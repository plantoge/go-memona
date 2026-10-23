package access

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrProjectInvalid  = errors.New("invalid project settings")
	ErrProjectConflict = errors.New("project revision conflict")
	ErrProjectLimit    = errors.New("project registry limit exceeded")
)

const projectLimit = 1000

// Project is optional display metadata. Slug is the immutable registry identity;
// aliases never resolve authorization scopes or change immutable note projects.
type Project struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases"`
	Archived    bool     `json:"archived"`
	Revision    int      `json:"revision"`
}

type ProjectInput struct {
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Aliases          []string `json:"aliases"`
	Archived         bool     `json:"archived"`
	ExpectedRevision int      `json:"expected_revision"`
}

// initProjects adds metadata only: it neither scans notes/private paths nor
// migrates existing projects, keys, or scopes into this optional registry.
func (s *Store) initProjects() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS project_registry (
 slug TEXT PRIMARY KEY,
 name TEXT NOT NULL,
 description TEXT NOT NULL,
 aliases TEXT NOT NULL,
 archived INTEGER NOT NULL DEFAULT 0 CHECK(archived IN (0,1)),
 revision INTEGER NOT NULL CHECK(revision > 0)
)`)
	return err
}

// ProjectListContext includes archived entries and orders deterministically by
// slug. An oversized registry fails explicitly rather than hiding entries.
func (s *Store) ProjectListContext(ctx context.Context) ([]Project, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT slug,name,description,aliases,archived,revision FROM project_registry ORDER BY slug LIMIT ?`, projectLimit+1)
	if err != nil {
		return nil, requestContextError(ctx, err)
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(out) == projectLimit {
			return nil, ErrProjectLimit
		}
		var p Project
		var aliases string
		if err := rows.Scan(&p.Slug, &p.Name, &p.Description, &aliases, &p.Archived, &p.Revision); err != nil {
			return nil, requestContextError(ctx, err)
		}
		if err := json.Unmarshal([]byte(aliases), &p.Aliases); err != nil {
			return nil, err
		}
		if p.Aliases == nil {
			p.Aliases = []string{}
		}
		out = append(out, p)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, requestContextError(ctx, err)
	}
	return out, nil
}

// SaveProjectContext creates only at expected_revision=0, otherwise replacing
// metadata at exactly that revision. It cannot rename a slug. Audit and metadata
// share the auth database transaction, so an audit failure rolls back the save.
func (s *Store) SaveProjectContext(ctx context.Context, in ProjectInput) (Project, error) {
	if err := ctx.Err(); err != nil {
		return Project{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if !slug.MatchString(in.Slug) || in.Name == "" || len(in.Name) > 200 || len(in.Description) > 2000 || len(in.Aliases) > 20 || in.ExpectedRevision < 0 || in.ExpectedRevision == int(^uint(0)>>1) {
		return Project{}, ErrProjectInvalid
	}
	// Copy before normalizing so callers retain ownership of their input slice.
	aliases := make([]string, 0, len(in.Aliases))
	seen := make(map[string]bool, len(in.Aliases))
	for _, alias := range in.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || len(alias) > 200 || seen[alias] {
			return Project{}, ErrProjectInvalid
		}
		seen[alias] = true
		aliases = append(aliases, alias)
	}
	p := Project{Slug: in.Slug, Name: in.Name, Description: in.Description, Aliases: aliases, Archived: in.Archived, Revision: in.ExpectedRevision + 1}
	encoded, err := json.Marshal(aliases)
	if err != nil {
		return Project{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Project{}, requestContextError(ctx, err)
	}
	defer tx.Rollback()
	action := "project.update"
	if in.ExpectedRevision == 0 {
		action = "project.create"
		// The count guard and insert are a single SQL write: concurrent creators
		// cannot exceed the cap, including through separate database connections.
		r, err := tx.ExecContext(ctx, `INSERT INTO project_registry(slug,name,description,aliases,archived,revision)
 SELECT ?,?,?,?,?,1 WHERE (SELECT COUNT(*) FROM project_registry) < ?
 ON CONFLICT(slug) DO NOTHING`, p.Slug, p.Name, p.Description, string(encoded), p.Archived, projectLimit)
		if err != nil {
			return Project{}, requestContextError(ctx, err)
		}
		n, err := r.RowsAffected()
		if err != nil {
			return Project{}, requestContextError(ctx, err)
		}
		if n == 0 {
			var exists bool
			if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM project_registry WHERE slug=?)", p.Slug).Scan(&exists); err != nil {
				return Project{}, requestContextError(ctx, err)
			}
			if exists {
				return Project{}, ErrProjectConflict
			}
			return Project{}, ErrProjectLimit
		}
	} else {
		r, err := tx.ExecContext(ctx, `UPDATE project_registry SET name=?,description=?,aliases=?,archived=?,revision=revision+1 WHERE slug=? AND revision=?`, p.Name, p.Description, string(encoded), p.Archived, p.Slug, in.ExpectedRevision)
		if err != nil {
			return Project{}, requestContextError(ctx, err)
		}
		n, err := r.RowsAffected()
		if err != nil {
			return Project{}, requestContextError(ctx, err)
		}
		if n != 1 {
			return Project{}, ErrProjectConflict
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO activity(time,actor,action,target) VALUES(?,?,?,?)", time.Now().UTC().Format(time.RFC3339), "admin", action, p.Slug); err != nil {
		return Project{}, requestContextError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return Project{}, err
	}
	if err := tx.Commit(); err != nil {
		return Project{}, requestContextError(ctx, err)
	}
	return p, nil
}
