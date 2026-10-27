package access

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Workspace projects use a reserved namespace. Legacy wildcard keys must not
// authorize this namespace; Key.Allows enforces that boundary.
type Workspace struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Role    string `json:"role"`
}
type Invitation struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	Email         string `json:"email"`
	Status        string `json:"status"`
}

// Initial per-account limits constrain unmetered public registration. They are
// not a substitute for storage quotas or operational abuse controls.
var ErrWorkspaceLimit = errors.New("workspace capacity limit reached")

const (
	maxTeamsPerOwner         = 10
	maxPendingInvitesPerTeam = 100
	maxKeysPerUser           = 100
)

// InitWorkspaces is idempotent and is called after initUsers during startup.
func (s *Store) InitWorkspaces() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS workspaces (
 id TEXT PRIMARY KEY, project TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('personal','team')),
 owner_user_id TEXT NOT NULL REFERENCES users(id));
 CREATE UNIQUE INDEX IF NOT EXISTS personal_workspace_owner ON workspaces(owner_user_id) WHERE kind='personal';
 CREATE TABLE IF NOT EXISTS workspace_members (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), user_id TEXT NOT NULL REFERENCES users(id),
 role TEXT NOT NULL CHECK(role IN ('owner','member')),
 PRIMARY KEY(workspace_id,user_id));
 CREATE TABLE IF NOT EXISTS workspace_invitations (
 id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 email TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('pending','accepted')),
 invited_by TEXT NOT NULL REFERENCES users(id), created_at TEXT NOT NULL);
 CREATE UNIQUE INDEX IF NOT EXISTS pending_workspace_invite ON workspace_invitations(workspace_id,email) WHERE status='pending';`)
	if err != nil {
		return err
	}
	// SQLite's ALTER TABLE ADD COLUMN has no portable IF NOT EXISTS. Inspect first;
	// failed migrations abort startup rather than silently weakening key checks.
	for _, col := range []string{"owner_user_id", "workspace_id"} {
		rows, e := s.db.Query("PRAGMA table_info(api_keys)")
		if e != nil {
			return e
		}
		found := false
		for rows.Next() {
			var cid, notnull, pk int
			var name, typ string
			var def sql.NullString
			if e = rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); e != nil {
				break
			}
			if name == col {
				found = true
			}
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		if !found {
			if _, e = s.db.Exec("ALTER TABLE api_keys ADD COLUMN " + col + " TEXT"); e != nil {
				return e
			}
		}
	}
	return nil
}

func (s *Store) EnsurePersonalWorkspace(ctx context.Context, userID string) (Workspace, error) {
	if userID == "" {
		return Workspace{}, ErrDenied
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return Workspace{}, e
	}
	defer tx.Rollback()
	var w Workspace
	e = tx.QueryRowContext(ctx, `SELECT id,project,name,kind FROM workspaces WHERE kind='personal' AND owner_user_id=?`, userID).Scan(&w.ID, &w.Project, &w.Name, &w.Kind)
	if errors.Is(e, sql.ErrNoRows) {
		seed, err := Random()
		if err != nil {
			return Workspace{}, err
		}
		w = Workspace{ID: seed[:24], Project: "ws-" + seed[:32], Name: "Personal", Kind: "personal"}
		_, e = tx.ExecContext(ctx, `INSERT INTO workspaces(id,project,name,kind,owner_user_id) VALUES(?,?,?,'personal',?)`, w.ID, w.Project, w.Name, userID)
		if e != nil {
			return Workspace{}, e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES(?,?,'owner')`, w.ID, userID)
	} else if e == nil {
		w.Role = "owner"
	}
	if e != nil {
		return Workspace{}, e
	}
	if e = tx.Commit(); e != nil {
		return Workspace{}, e
	}
	w.Role = "owner"
	return w, nil
}
func (s *Store) UserWorkspaces(ctx context.Context, userID string) ([]Workspace, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT w.id,w.project,w.name,w.kind,m.role FROM workspaces w JOIN workspace_members m ON m.workspace_id=w.id WHERE m.user_id=? ORDER BY w.kind,w.name,w.id`, userID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Workspace{}
	for rows.Next() {
		var w Workspace
		if e = rows.Scan(&w.ID, &w.Project, &w.Name, &w.Kind, &w.Role); e != nil {
			return nil, e
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
func (s *Store) UserWorkspace(ctx context.Context, userID, id string) (Workspace, error) {
	var w Workspace
	e := s.db.QueryRowContext(ctx, `SELECT w.id,w.project,w.name,w.kind,m.role FROM workspaces w JOIN workspace_members m ON m.workspace_id=w.id WHERE m.user_id=? AND w.id=?`, userID, id).Scan(&w.ID, &w.Project, &w.Name, &w.Kind, &w.Role)
	if errors.Is(e, sql.ErrNoRows) {
		return w, ErrDenied
	}
	return w, e
}
func (s *Store) CreateTeam(ctx context.Context, userID, name string) (Workspace, error) {
	name = strings.TrimSpace(name)
	if len(name) == 0 || len(name) > 100 || strings.ContainsAny(name, "\x00\r\n") {
		return Workspace{}, ErrInvalid
	}
	seed, e := Random()
	if e != nil {
		return Workspace{}, e
	}
	w := Workspace{ID: seed[:24], Project: "ws-" + seed[:32], Name: name, Kind: "team", Role: "owner"}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return Workspace{}, e
	}
	defer tx.Rollback()
	result, e := tx.ExecContext(ctx, `INSERT INTO workspaces(id,project,name,kind,owner_user_id)
	SELECT ?,?,?,'team',id FROM users WHERE id=?
	AND (SELECT COUNT(*) FROM workspaces WHERE kind='team' AND owner_user_id=?) < ?`, w.ID, w.Project, name, userID, userID, maxTeamsPerOwner)
	if e != nil {
		return Workspace{}, e
	}
	if affected, err := result.RowsAffected(); err != nil {
		return Workspace{}, err
	} else if affected != 1 {
		return Workspace{}, ErrWorkspaceLimit
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES(?,?,'owner')`, w.ID, userID)
	if e != nil {
		return Workspace{}, e
	}
	return w, tx.Commit()
}
func normalizeInviteEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }
func (s *Store) InviteUser(ctx context.Context, userID, workspaceID, email string) (Invitation, error) {
	email = normalizeInviteEmail(email)
	if len(email) > 320 || len(email) < 3 || strings.ContainsAny(email, "\x00\r\n ") || strings.Count(email, "@") != 1 {
		return Invitation{}, ErrInvalid
	}
	seed, e := Random()
	if e != nil {
		return Invitation{}, e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return Invitation{}, e
	}
	defer tx.Rollback()
	var name string
	e = tx.QueryRowContext(ctx, `SELECT w.name FROM workspaces w JOIN workspace_members m ON m.workspace_id=w.id WHERE w.id=? AND w.kind='team' AND m.user_id=? AND m.role='owner'`, workspaceID, userID).Scan(&name)
	if errors.Is(e, sql.ErrNoRows) {
		return Invitation{}, ErrDenied
	}
	if e != nil {
		return Invitation{}, e
	}
	var member int
	e = tx.QueryRowContext(ctx, `SELECT count(*) FROM workspace_members m JOIN users u ON u.id=m.user_id WHERE m.workspace_id=? AND lower(u.email)=?`, workspaceID, email).Scan(&member)
	if e != nil {
		return Invitation{}, e
	}
	if member > 0 {
		return Invitation{}, ErrInvalid
	}
	i := Invitation{ID: seed[:24], WorkspaceID: workspaceID, WorkspaceName: name, Email: email, Status: "pending"}
	result, e := tx.ExecContext(ctx, `INSERT INTO workspace_invitations(id,workspace_id,email,status,invited_by,created_at)
	SELECT ?,?,?,'pending',?,? WHERE (SELECT COUNT(*) FROM workspace_invitations WHERE workspace_id=? AND status='pending') < ?`, i.ID, workspaceID, email, userID, time.Now().UTC().Format(time.RFC3339), workspaceID, maxPendingInvitesPerTeam)
	if e != nil {
		return Invitation{}, ErrInvalid
	}
	if affected, err := result.RowsAffected(); err != nil {
		return Invitation{}, err
	} else if affected != 1 {
		return Invitation{}, ErrWorkspaceLimit
	}
	return i, tx.Commit()
}
func (s *Store) UserInvitations(ctx context.Context, email string) ([]Invitation, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT i.id,i.workspace_id,w.name,i.email,i.status FROM workspace_invitations i JOIN workspaces w ON w.id=i.workspace_id WHERE i.email=? AND i.status='pending' ORDER BY i.created_at DESC`, normalizeInviteEmail(email))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Invitation{}
	for rows.Next() {
		var i Invitation
		if e = rows.Scan(&i.ID, &i.WorkspaceID, &i.WorkspaceName, &i.Email, &i.Status); e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *Store) AcceptInvitation(ctx context.Context, userID, invitationID string) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var workspaceID string
	e = tx.QueryRowContext(ctx, `SELECT i.workspace_id FROM workspace_invitations i JOIN users u ON lower(u.email)=i.email WHERE i.id=? AND i.status='pending' AND u.id=?`, invitationID, userID).Scan(&workspaceID)
	if errors.Is(e, sql.ErrNoRows) {
		return ErrDenied
	}
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES(?,?,'member') ON CONFLICT(workspace_id,user_id) DO NOTHING`, workspaceID, userID)
	if e != nil {
		return e
	}
	res, e := tx.ExecContext(ctx, `UPDATE workspace_invitations SET status='accepted' WHERE id=? AND status='pending'`, invitationID)
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return ErrDenied
	}
	return tx.Commit()
}

// CreateUserKey never trusts client-supplied Projects. A key belongs to one
// workspace and can be managed only by its creator while still a member.
func (s *Store) CreateUserKey(ctx context.Context, userID, workspaceID string, in KeyInput) (Key, string, error) {
	w, e := s.UserWorkspace(ctx, userID, workspaceID)
	if e != nil {
		return Key{}, "", e
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 || len(in.Permissions) == 0 || len(in.Permissions) > 2 || len(in.Projects) != 0 {
		return Key{}, "", ErrInvalid
	}
	for _, p := range in.Permissions {
		if p != "read" && p != "write" {
			return Key{}, "", ErrInvalid
		}
	}
	if in.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, in.ExpiresAt)
		if err != nil || !t.After(time.Now()) {
			return Key{}, "", ErrInvalid
		}
		in.ExpiresAt = t.UTC().Format(time.RFC3339)
	}
	id, e := Random()
	if e != nil {
		return Key{}, "", e
	}
	secret, e := Random()
	if e != nil {
		return Key{}, "", e
	}
	token := "mm_" + secret
	k := Key{ID: id[:24], Name: in.Name, Prefix: token[:11], Permissions: in.Permissions, Projects: []string{w.Project}, CreatedAt: time.Now().UTC().Format(time.RFC3339), ExpiresAt: in.ExpiresAt, WorkspaceID: w.ID, OwnerUserID: userID}
	perms, _ := json.Marshal(k.Permissions)
	projects, _ := json.Marshal(k.Projects)
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return Key{}, "", e
	}
	defer tx.Rollback()
	// Recheck membership inside the write transaction to close the revocation race.
	var existing int
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE owner_user_id=?`, userID).Scan(&existing); e != nil {
		return Key{}, "", e
	}
	if existing >= maxKeysPerUser {
		return Key{}, "", ErrWorkspaceLimit
	}
	res, e := tx.ExecContext(ctx, `INSERT INTO api_keys(id,name,prefix,hash,permissions,projects,created_at,expires_at,owner_user_id,workspace_id)
	SELECT ?,?,?,?,?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM workspace_members WHERE workspace_id=? AND user_id=?)
	AND (SELECT COUNT(*) FROM api_keys WHERE owner_user_id=?) < ?`, k.ID, k.Name, k.Prefix, digest(token), string(perms), string(projects), k.CreatedAt, k.ExpiresAt, userID, w.ID, w.ID, userID, userID, maxKeysPerUser)
	if e != nil {
		return Key{}, "", e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return Key{}, "", e
	}
	if n != 1 {
		return Key{}, "", ErrDenied
	}
	if e = tx.Commit(); e != nil {
		return Key{}, "", e
	}
	return k, token, nil
}
func (s *Store) UserKeys(ctx context.Context, userID string) ([]Key, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT `+keyColumns+`,workspace_id FROM api_keys WHERE owner_user_id=? AND EXISTS(SELECT 1 FROM workspace_members WHERE workspace_id=api_keys.workspace_id AND user_id=?) ORDER BY created_at DESC,id`, userID, userID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Key{}
	for rows.Next() {
		var k Key
		var perms, projects string
		if e = rows.Scan(&k.ID, &k.Name, &k.Prefix, &perms, &projects, &k.CreatedAt, &k.ExpiresAt, &k.Revoked, &k.WorkspaceID); e != nil {
			return nil, e
		}
		k.OwnerUserID = userID
		if e = json.Unmarshal([]byte(perms), &k.Permissions); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(projects), &k.Projects); e != nil {
			return nil, e
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
func (s *Store) RevokeUserKey(ctx context.Context, userID, id string) error {
	res, e := s.db.ExecContext(ctx, `UPDATE api_keys SET revoked=1 WHERE id=? AND owner_user_id=? AND EXISTS(SELECT 1 FROM workspace_members WHERE workspace_id=api_keys.workspace_id AND user_id=?)`, id, userID, userID)
	if e != nil {
		return e
	}
	n, e := res.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return ErrDenied
	}
	return nil
}

// AuthenticateWorkspaceKeyContext adds live membership and project checks to
// normal token authentication. secureMCP must call this instead of AuthenticateContext.
func (s *Store) AuthenticateWorkspaceKeyContext(ctx context.Context, token string) (Key, error) {
	k, e := s.AuthenticateContext(ctx, token)
	if e != nil {
		return k, e
	}
	var owner, workspace sql.NullString
	e = s.db.QueryRowContext(ctx, `SELECT owner_user_id,workspace_id FROM api_keys WHERE id=?`, k.ID).Scan(&owner, &workspace)
	if e != nil {
		return Key{}, e
	}
	if !owner.Valid && !workspace.Valid {
		return k, nil
	}
	if !owner.Valid || !workspace.Valid {
		return Key{}, ErrDenied
	}
	var project string
	e = s.db.QueryRowContext(ctx, `SELECT w.project FROM workspaces w JOIN workspace_members m ON m.workspace_id=w.id WHERE w.id=? AND m.user_id=?`, workspace.String, owner.String).Scan(&project)
	if errors.Is(e, sql.ErrNoRows) {
		return Key{}, ErrDenied
	}
	if e != nil {
		return Key{}, e
	}
	if len(k.Projects) != 1 || k.Projects[0] != project || !strings.HasPrefix(project, "ws-") {
		return Key{}, ErrDenied
	}
	k.WorkspaceID = workspace.String
	k.OwnerUserID = owner.String
	return k, nil
}
