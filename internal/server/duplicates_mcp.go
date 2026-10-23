package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"memona/internal/access"
	"memona/internal/memory"
)

type duplicateInput struct {
	Project   string `json:"project"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	ExcludeID string `json:"exclude_id,omitempty"`
}
type duplicateResult struct {
	Candidates []duplicateCandidate `json:"candidates"`
	Guidance   string               `json:"guidance"`
}

func addDuplicateTool(requestCtx context.Context, s *mcp.Server, a *App, k access.Key) {
	toolContext(requestCtx, s, "find_duplicate_candidates", "Find up to ten lexical similarity candidates in one explicit authorized project before saving. Requires read permission. Advisory metadata only; no merge, save or duplicate certainty. Optional exclude_id omits the note being revised.", true, func(ctx context.Context, r *mcp.CallToolRequest, in duplicateInput) (*mcp.CallToolResult, duplicateResult, error) {
		out := duplicateResult{Candidates: []duplicateCandidate{}, Guidance: "Token similarity is not semantic identity or probability. Review candidates manually; choose revision or new note yourself. No writes or merge performed; at most ten nonarchived same-project candidates."}
		if !k.Allows("read", in.Project) {
			return nil, out, access.ErrDenied
		}
		if !validRetrievalProject(in.Project) || !validRetrievalText(in.Title, 200, false, true) || !validRetrievalText(in.Content, 65536, true, false) {
			return nil, out, memory.ErrInvalid
		}
		notes, err := a.readableSnapshotContext(ctx, k)
		if err != nil {
			return nil, out, err
		}
		eligible := []memory.Note{}
		for _, n := range notes {
			if n.Project == in.Project && n.ID != in.ExcludeID {
				eligible = append(eligible, n)
			}
		}
		out.Candidates, err = duplicateCandidates(ctx, eligible, in.Project, in.Title, in.Content)
		if err != nil {
			return nil, out, publicReadError(err)
		}
		a.audit(k.Actor(), "memory.duplicates", in.Project)
		return nil, out, nil
	})
}
