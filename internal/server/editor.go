package server

import (
	"net/http"
	"strings"

	"memona/internal/memory"
)

// editorAPI is delegated to by api after the existing admin authentication and
// CSRF middleware. It deliberately uses the same store validators as MCP.
func (a *App) editorAPI(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	if r.URL.Path == "/api/notes" {
		var in memory.Input
		if decode(r, &in) != nil {
			fail(w, http.StatusBadRequest, "Isi catatan tidak valid.")
			return true
		}
		n, err := a.notes.CreateContext(r.Context(), in, "admin")
		if err != nil {
			a.noteError(w, err)
			return true
		}
		a.audit("admin", "note.create", n.ID)
		respond(w, http.StatusCreated, n)
		return true
	}
	if !strings.HasPrefix(r.URL.Path, "/api/notes/") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/notes/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "revise" {
		return false
	}
	var in struct {
		ExpectedRevision int `json:"expected_revision"`
		memory.Input
	}
	if decode(r, &in) != nil {
		fail(w, http.StatusBadRequest, "Isi catatan dan revisi diperlukan.")
		return true
	}
	n, err := a.notes.ReviseContext(r.Context(), parts[0], in.ExpectedRevision, in.Input, "admin")
	if err != nil {
		a.noteError(w, err)
		return true
	}
	a.audit("admin", "note.revise", n.ID)
	respond(w, http.StatusOK, n)
	return true
}
