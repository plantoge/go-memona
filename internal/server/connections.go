package server

import (
	"context"
	"errors"

	"memona/internal/access"
	"memona/internal/memory"
)

type noteReference struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Project  string `json:"project"`
	Kind     string `json:"kind"`
	Archived bool   `json:"archived"`
}
type noteConnections struct {
	RelatedTo    []noteReference `json:"related_to"`
	Supersedes   []noteReference `json:"supersedes"`
	ReferencedBy []noteReference `json:"referenced_by"`
	SupersededBy []noteReference `json:"superseded_by"`
	Truncated    bool            `json:"truncated"`
}
type noteView struct {
	memory.Note
	Connections noteConnections `json:"connections"`
}

func reference(n memory.Note) noteReference {
	return noteReference{n.ID, n.Title, n.Project, n.Kind, n.Archived}
}
func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}
func connectionSize(c noteConnections) int {
	size := 0
	for _, group := range [][]noteReference{c.RelatedTo, c.Supersedes, c.ReferencedBy, c.SupersededBy} {
		for _, n := range group {
			size += len(n.ID) + len(n.Title) + len(n.Project) + len(n.Kind) + 80
		}
	}
	return size
}

// Derive backlinks from current Markdown only, within the source project. The
// caller must filter authorization before deriving counts, titles or references.
func connectionsFor(n memory.Note, all []memory.Note) noteConnections {
	c, _ := connectionsForContext(context.Background(), n, all)
	return c
}

func connectionsForContext(ctx context.Context, n memory.Note, all []memory.Note) (noteConnections, error) {
	c := noteConnections{RelatedTo: []noteReference{}, Supersedes: []noteReference{}, ReferencedBy: []noteReference{}, SupersededBy: []noteReference{}}
	for _, other := range all {
		if err := ctx.Err(); err != nil {
			return noteConnections{}, err
		}
		if other.Project != n.Project || other.ID == n.ID {
			continue
		}
		ref := reference(other)
		if containsID(n.RelatedTo, other.ID) {
			appendReference(&c.RelatedTo, ref, &c.Truncated)
		}
		if containsID(n.Supersedes, other.ID) {
			appendReference(&c.Supersedes, ref, &c.Truncated)
		}
		if !other.Archived && containsID(other.RelatedTo, n.ID) {
			appendReference(&c.ReferencedBy, ref, &c.Truncated)
		}
		if !other.Archived && containsID(other.Supersedes, n.ID) {
			appendReference(&c.SupersededBy, ref, &c.Truncated)
		}
	}
	if err := ctx.Err(); err != nil {
		return noteConnections{}, err
	}
	return c, nil
}

// Retain only the 20 smallest IDs, independent of snapshot ordering. At no
// point does a high-fan-in backlink allocate a slice for the whole collection.
func appendReference(group *[]noteReference, ref noteReference, truncated *bool) {
	i := 0
	for i < len(*group) && (*group)[i].ID < ref.ID {
		i++
	}
	if len(*group) == 20 {
		*truncated = true
		if i == 20 {
			return
		}
		copy((*group)[i+1:], (*group)[i:19])
		(*group)[i] = ref
		return
	}
	*group = append(*group, noteReference{})
	copy((*group)[i+1:], (*group)[i:])
	(*group)[i] = ref
}
func (a *App) readableSnapshot(k access.Key) ([]memory.Note, error) {
	return a.readableSnapshotContext(context.Background(), k)
}
func (a *App) readableSnapshotContext(ctx context.Context, k access.Key) ([]memory.Note, error) {
	if err := authorizeQuery(k, queryInput{}); err != nil {
		return nil, err
	}
	all, err := a.notes.ListContext(ctx)
	if err != nil {
		return nil, publicReadError(err)
	}
	out := []memory.Note{}
	for _, n := range all {
		if err := ctx.Err(); err != nil {
			return nil, publicReadError(err)
		}
		if k.Allows("read", n.Project) {
			out = append(out, n)
		}
	}
	return out, nil
}
func (a *App) readView(k access.Key, id string) (noteView, error) {
	return a.readViewContext(context.Background(), k, id)
}
func (a *App) readViewContext(ctx context.Context, k access.Key, id string) (noteView, error) {
	all, err := a.readableSnapshotContext(ctx, k)
	if err != nil {
		return noteView{}, err
	}
	for _, n := range all {
		if err := ctx.Err(); err != nil {
			return noteView{}, publicReadError(err)
		}
		if n.ID == id {
			links, err := connectionsForContext(ctx, n, all)
			if err != nil {
				return noteView{}, publicReadError(err)
			}
			return noteView{n, links}, nil
		}
	}
	return noteView{}, errors.New("note unavailable or access denied")
}
func (a *App) authorizeMetadata(k access.Key, in memory.Input) error {
	return a.authorizeMetadataContext(context.Background(), k, in)
}
func (a *App) authorizeMetadataContext(ctx context.Context, k access.Key, in memory.Input) error {
	if in.IsProjectSummary != nil || in.RelatedTo != nil || in.Supersedes != nil {
		if !k.Allows("read", in.Project) {
			return access.ErrDenied
		}
	}
	// Do not distinguish nonexistent targets from private targets.
	for _, group := range [][]string{in.RelatedTo, in.Supersedes} {
		for _, id := range group {
			n, err := a.authorizedNoteContext(ctx, k, "read", id)
			if err != nil || n.Project != in.Project {
				return errors.New("relation target unavailable or outside this project")
			}
		}
	}
	return nil
}
