package server

import (
	"memona/internal/memory"
	"net/http"
	"strings"
)

// Copy historical content into a fresh draft. Current relationships remain
// untouched; publication is a separate existing admin action after review.
func (a *App) skillRevisionAPI(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, "/api/notes/") || !strings.HasSuffix(r.URL.Path, "/restore-skill-content") {
		return false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/notes/"), "/restore-skill-content")
	var in struct {
		Expected int `json:"expected_revision"`
		Source   int `json:"source_revision"`
	}
	if decode(r, &in) != nil || in.Expected < 1 || in.Source < 1 || in.Source > in.Expected {
		fail(w, 400, "Revisi sumber dan revisi terbaru diperlukan.")
		return true
	}
	latest, err := a.notes.GetContext(r.Context(), id)
	if err != nil {
		a.noteError(w, err)
		return true
	}
	if latest.Kind != "skill" || latest.Archived || latest.Revision != in.Expected {
		a.noteError(w, memory.ErrConflict)
		return true
	}
	page, err := a.notes.HistoryPageContext(r.Context(), id, in.Source+1, 1)
	if err != nil {
		a.noteError(w, err)
		return true
	}
	if len(page.Items) != 1 || page.Items[0].Revision != in.Source {
		a.noteError(w, memory.ErrInvalid)
		return true
	}
	source := page.Items[0]
	n, err := a.notes.ReviseContext(r.Context(), id, in.Expected, memory.Input{Project: latest.Project, Kind: "skill", Title: source.Title, Content: source.Content, Source: source.Source, Status: "draft"}, "admin")
	if err != nil {
		a.noteError(w, err)
		return true
	}
	a.audit("admin", "skill.restore-content", id)
	respond(w, 201, n)
	return true
}
