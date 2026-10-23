package server

import (
	"net/http"
	"strconv"
	"strings"
)

func (a *App) historyPageAPI(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/notes/") || !strings.HasSuffix(r.URL.Path, "/history-page") {
		return false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/notes/"), "/history-page")
	limit, before := 20, 0
	for name, target := range map[string]*int{"limit": &limit, "before": &before} {
		if raw := r.URL.Query().Get(name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				fail(w, 400, "Parameter halaman tidak valid.")
				return true
			}
			*target = n
		}
	}
	page, err := a.notes.HistoryPageContext(r.Context(), id, before, limit)
	if err != nil {
		a.noteError(w, err)
		return true
	}
	respond(w, 200, page)
	return true
}
