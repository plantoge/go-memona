package server

import (
	"memona/internal/memory"
	"net/http"
)

// Advisory read only, under the existing admin session and POST CSRF boundary.
// Never writes a note, merges data, or sends content in the response.
func (a *App) duplicatesAPI(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/duplicate-candidates" || r.Method != http.MethodPost {
		return false
	}
	var in struct {
		Project   string `json:"project"`
		Title     string `json:"title"`
		Content   string `json:"content"`
		ExcludeID string `json:"exclude_id"`
	}
	if decode(r, &in) != nil || !validRetrievalProject(in.Project) || !validRetrievalText(in.Title, 200, false, true) || !validRetrievalText(in.Content, 65536, true, false) {
		a.noteError(w, memory.ErrInvalid)
		return true
	}
	all, err := a.notes.ListContext(r.Context())
	if err != nil {
		a.internal(w, err)
		return true
	}
	eligible := make([]memory.Note, 0)
	for _, n := range all {
		if n.Project == in.Project && n.ID != in.ExcludeID {
			eligible = append(eligible, n)
		}
	}
	candidates, err := duplicateCandidates(r.Context(), eligible, in.Project, in.Title, in.Content)
	if err != nil {
		a.noteError(w, err)
		return true
	}
	respond(w, http.StatusOK, map[string]any{"candidates": candidates, "guidance": "Kemiripan leksikal, bukan keputusan duplikat. Tinjau manual; tidak ada merge atau perubahan otomatis."})
	return true
}
