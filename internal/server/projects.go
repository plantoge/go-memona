package server

import (
	"errors"
	"net/http"

	"memona/internal/access"
)

// projectsAPI is called inside the existing admin session/CSRF middleware. The
// optional registry is display metadata, not a source of note/key authorization.
func (a *App) projectsAPI(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/project-registry" {
		return false
	}
	switch r.Method {
	case http.MethodGet:
		projects, err := a.auth.ProjectListContext(r.Context())
		if err != nil {
			a.projectError(w, err)
			return true
		}
		respond(w, http.StatusOK, projects)
	case http.MethodPost:
		var in access.ProjectInput
		if err := decode(r, &in); err != nil {
			fail(w, http.StatusBadRequest, "Metadata proyek tidak valid.")
			return true
		}
		project, err := a.auth.SaveProjectContext(r.Context(), in)
		if err != nil {
			a.projectError(w, err)
			return true
		}
		status := http.StatusOK
		if in.ExpectedRevision == 0 {
			status = http.StatusCreated
		}
		respond(w, status, project)
	default:
		w.Header().Set("Allow", "GET, POST")
		fail(w, http.StatusMethodNotAllowed, "Metode tidak didukung.")
	}
	return true
}

func (a *App) projectError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, access.ErrProjectInvalid):
		fail(w, http.StatusBadRequest, "Slug, nama, deskripsi, alias, atau revisi proyek tidak valid.")
	case errors.Is(err, access.ErrProjectConflict):
		fail(w, http.StatusConflict, "Revisi proyek sudah berubah. Muat ulang sebelum mencoba.")
	case errors.Is(err, access.ErrProjectLimit):
		fail(w, http.StatusConflict, "Registry proyek melampaui batas 1000 entri; tidak ada hasil parsial atau perubahan yang disimpan.")
	default:
		a.internal(w, err)
	}
}
