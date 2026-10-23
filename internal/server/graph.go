package server

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"memona/internal/memory"
)

type graphNode struct {
	ID               string `json:"id"`
	Project          string `json:"project"`
	Kind             string `json:"kind"`
	Title            string `json:"title"`
	Status           string `json:"status"`
	Archived         bool   `json:"archived"`
	IsProjectSummary bool   `json:"is_project_summary"`
	UpdatedAt        string `json:"updated_at"`
	Revision         int    `json:"revision"`
}
type graphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}
type graphResult struct {
	Nodes     []graphNode `json:"nodes"`
	Edges     []graphEdge `json:"edges"`
	Truncated bool        `json:"truncated"`
}

// all must be an authorized latest snapshot. Edges are explicit only and both
// endpoints must be included; omitted endpoints are reported as truncation.
func buildGraph(ctx context.Context, all []memory.Note, project, query string, archived bool, limit int) (graphResult, error) {
	out := graphResult{Nodes: []graphNode{}, Edges: []graphEdge{}}
	if limit < 1 || limit > 500 {
		limit = 200
	}
	candidates := []memory.Note{}
	for _, n := range all {
		if err := ctx.Err(); err != nil {
			return graphResult{}, err
		}
		if (project == "" || n.Project == project) && (archived || !n.Archived) && (query == "" || matches(n, strings.ToLower(query))) {
			candidates = append(candidates, n)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	if len(candidates) > limit {
		out.Truncated = true
		candidates = candidates[:limit]
	}
	included := map[string]memory.Note{}
	for _, n := range candidates {
		included[n.ID] = n
		out.Nodes = append(out.Nodes, graphNode{n.ID, n.Project, n.Kind, n.Title, n.Status, n.Archived, n.IsProjectSummary, n.UpdatedAt, n.Revision})
	}
	for _, n := range candidates {
		for _, group := range []struct {
			kind string
			ids  []string
		}{{"related_to", n.RelatedTo}, {"supersedes", n.Supersedes}} {
			for _, id := range group.ids {
				if err := ctx.Err(); err != nil {
					return graphResult{}, err
				}
				other, ok := included[id]
				if !ok {
					out.Truncated = true
					continue
				}
				if id == n.ID || other.Project != n.Project {
					continue
				}
				out.Edges = append(out.Edges, graphEdge{n.ID, id, group.kind})
			}
		}
	}
	sort.Slice(out.Edges, func(i, j int) bool {
		a, b := out.Edges[i], out.Edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.To < b.To
	})
	if err := ctx.Err(); err != nil {
		return graphResult{}, err
	}
	return out, nil
}
func (a *App) graphAPI(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			fail(w, 400, "Batas graph harus 1–500.")
			return
		}
		limit = n
	}
	project, query := r.URL.Query().Get("project"), r.URL.Query().Get("q")
	if len(project) > 64 || len(query) > 2000 {
		fail(w, 400, "Filter graph terlalu panjang.")
		return
	}
	archive := r.URL.Query().Get("archived")
	if archive != "" && archive != "true" && archive != "false" {
		fail(w, 400, "Filter arsip tidak valid.")
		return
	}
	all, err := a.notes.ListContext(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	out, err := buildGraph(r.Context(), all, project, query, archive == "true", limit)
	if err != nil {
		a.internal(w, err)
		return
	}
	respond(w, 200, out)
}
