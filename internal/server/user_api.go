package server

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"memona/internal/access"
	"memona/internal/memory"
)

// userAPI is registered as /api/user/ by the application router, before the
// catch-all admin /api/ route. It does not grant admin privileges.
func (a *App) userAPI(w http.ResponseWriter, r *http.Request) {
	cookie, e := r.Cookie("memona_user_session")
	if e != nil {
		fail(w, 401, "Login diperlukan.")
		return
	}
	user, csrf, e := a.auth.UserSessionContext(r.Context(), cookie.Value)
	if e != nil {
		if !readFailure(w, e) {
			fail(w, 401, "Sesi berakhir.")
		}
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" && subtle.ConstantTimeCompare([]byte(csrf), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
		fail(w, 403, "Token CSRF tidak valid.")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/user/")
	switch {
	case path == "workspaces" && r.Method == "GET":
		if _, e = a.auth.EnsurePersonalWorkspace(r.Context(), user.ID); e != nil {
			a.internal(w, e)
			return
		}
		v, err := a.auth.UserWorkspaces(r.Context(), user.ID)
		if err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, v)
	case path == "teams" && r.Method == "POST":
		var in struct {
			Name string `json:"name"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "Nama tim tidak valid.")
			return
		}
		v, err := a.auth.CreateTeam(r.Context(), user.ID, in.Name)
		if err != nil {
			a.userAccessError(w, err)
			return
		}
		a.audit("user:"+user.ID, "team.create", v.ID)
		respond(w, 201, v)
	case strings.HasPrefix(path, "teams/") && strings.HasSuffix(path, "/invite") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "teams/"), "/invite")
		if strings.Contains(id, "/") || id == "" {
			fail(w, 404, "Tidak ditemukan.")
			return
		}
		var in struct {
			Email string `json:"email"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "Email tidak valid.")
			return
		}
		v, err := a.auth.InviteUser(r.Context(), user.ID, id, in.Email)
		if err != nil {
			a.userAccessError(w, err)
			return
		}
		a.audit("user:"+user.ID, "team.invite", id)
		respond(w, 201, v)
	case path == "invitations" && r.Method == "GET":
		v, err := a.auth.UserInvitations(r.Context(), user.Email)
		if err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, v)
	case strings.HasPrefix(path, "invitations/") && strings.HasSuffix(path, "/accept") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "invitations/"), "/accept")
		if id == "" || strings.Contains(id, "/") {
			fail(w, 404, "Tidak ditemukan.")
			return
		}
		if err := a.auth.AcceptInvitation(r.Context(), user.ID, id); err != nil {
			a.userAccessError(w, err)
			return
		}
		a.audit("user:"+user.ID, "team.accept", id)
		respond(w, 200, map[string]bool{"ok": true})
	case path == "keys" && r.Method == "GET":
		v, err := a.auth.UserKeys(r.Context(), user.ID)
		if err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, v)
	case path == "keys" && r.Method == "POST":
		var in struct {
			WorkspaceID string   `json:"workspace_id"`
			Name        string   `json:"name"`
			Permissions []string `json:"permissions"`
			ExpiresAt   string   `json:"expires_at"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "Key tidak valid.")
			return
		}
		k, token, err := a.auth.CreateUserKey(r.Context(), user.ID, in.WorkspaceID, access.KeyInput{Name: in.Name, Permissions: in.Permissions, ExpiresAt: in.ExpiresAt})
		if err != nil {
			a.userAccessError(w, err)
			return
		}
		a.audit("user:"+user.ID, "key.create", k.ID)
		respond(w, 201, map[string]any{"key": k, "token": token})
	case strings.HasPrefix(path, "keys/") && strings.HasSuffix(path, "/revoke") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "keys/"), "/revoke")
		if id == "" || strings.Contains(id, "/") {
			fail(w, 404, "Tidak ditemukan.")
			return
		}
		if err := a.auth.RevokeUserKey(r.Context(), user.ID, id); err != nil {
			a.userAccessError(w, err)
			return
		}
		a.audit("user:"+user.ID, "key.revoke", id)
		respond(w, 200, map[string]bool{"ok": true})
	case path == "graph" && r.Method == "GET":
		workspaceID := r.URL.Query().Get("workspace_id")
		ws, err := a.auth.UserWorkspace(r.Context(), user.ID, workspaceID)
		if err != nil {
			a.userAccessError(w, err)
			return
		}
		limit := 200
		if raw := r.URL.Query().Get("limit"); raw != "" {
			limit, err = strconv.Atoi(raw)
			if err != nil || limit < 1 || limit > 500 {
				fail(w, 400, "Batas graph harus 1–500.")
				return
			}
		}
		query := r.URL.Query().Get("q")
		if len(query) > 2000 {
			fail(w, 400, "Filter graph terlalu panjang.")
			return
		}
		// Never accept a caller-supplied project or archive flag for user graphs.
		if r.URL.Query().Has("project") || r.URL.Query().Has("archived") {
			fail(w, 400, "Filter graph tidak valid.")
			return
		}
		all, err := a.notes.ListContext(r.Context())
		if err != nil {
			a.internal(w, err)
			return
		}
		authorized := make([]memory.Note, 0)
		for _, n := range all {
			if n.Project == ws.Project && !n.Archived {
				authorized = append(authorized, n)
			}
		}
		out, err := buildGraph(r.Context(), authorized, ws.Project, query, false, limit)
		if err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, out)
	case path == "notes" && r.Method == "GET":
		workspaceID := r.URL.Query().Get("workspace_id")
		ws, err := a.auth.UserWorkspace(r.Context(), user.ID, workspaceID)
		if err != nil {
			a.userAccessError(w, err)
			return
		}
		all, err := a.notes.ListContext(r.Context())
		if err != nil {
			a.internal(w, err)
			return
		}
		out := []memory.Note{}
		for _, n := range all {
			if n.Project == ws.Project && !n.Archived {
				out = append(out, n)
			}
		}
		sortNotes(out)
		if len(out) > 500 {
			out = out[:500]
		}
		respond(w, 200, out)
	case path == "notes" && r.Method == "POST":
		var in struct {
			WorkspaceID    string                 `json:"workspace_id"`
			Kind           string                 `json:"kind"`
			Title          string                 `json:"title"`
			Content        string                 `json:"content"`
			Status         string                 `json:"status"`
			Source         string                 `json:"source"`
			CodeReferences []memory.CodeReference `json:"code_references"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "Catatan tidak valid.")
			return
		}
		ws, err := a.auth.UserWorkspace(r.Context(), user.ID, in.WorkspaceID)
		if err != nil {
			a.userAccessError(w, err)
			return
		}
		n, err := a.notes.CreateContext(r.Context(), memory.Input{Project: ws.Project, Kind: in.Kind, Title: in.Title, Content: in.Content, Status: in.Status, Source: in.Source, CodeReferences: in.CodeReferences}, "user:"+user.ID)
		if err != nil {
			a.noteError(w, err)
			return
		}
		a.audit("user:"+user.ID, "memory.create", n.ID)
		respond(w, 201, n)
	case strings.HasPrefix(path, "notes/") && (r.Method == "GET" || r.Method == "POST"):
		id := strings.TrimPrefix(path, "notes/")
		if id == "" || strings.Contains(id, "/") {
			fail(w, 404, "Tidak ditemukan.")
			return
		}
		// Resolve live membership before reading an ID, and return the same 404
		// for inaccessible and unknown notes to avoid an enumeration oracle.
		ws, err := a.auth.UserWorkspace(r.Context(), user.ID, r.URL.Query().Get("workspace_id"))
		if err != nil {
			if readFailure(w, err) {
				return
			}
			if !errors.Is(err, access.ErrDenied) {
				a.internal(w, err)
				return
			}
			fail(w, 404, "Catatan tidak ditemukan.")
			return
		}
		n, err := a.notes.GetContext(r.Context(), id)
		if errors.Is(err, memory.ErrNotFound) || errors.Is(err, memory.ErrInvalid) || (err == nil && (ws.Project != n.Project || n.Archived)) {
			fail(w, 404, "Catatan tidak ditemukan.")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		if r.Method == "GET" {
			if n.Archived {
				fail(w, 404, "Catatan tidak ditemukan.")
				return
			}
			respond(w, 200, n)
			return
		}
		var in struct {
			ExpectedRevision int                    `json:"expected_revision"`
			Kind             string                 `json:"kind"`
			Title            string                 `json:"title"`
			Content          string                 `json:"content"`
			Status           string                 `json:"status"`
			Source           string                 `json:"source"`
			CodeReferences   []memory.CodeReference `json:"code_references"`
		}
		if decode(r, &in) != nil {
			fail(w, 400, "Catatan tidak valid.")
			return
		}
		if n.Archived || in.Kind != n.Kind {
			fail(w, 400, "Jenis catatan tidak dapat diubah.")
			return
		}
		updated, err := a.notes.ReviseContext(r.Context(), id, in.ExpectedRevision, memory.Input{Project: ws.Project, Kind: n.Kind, Title: in.Title, Content: in.Content, Status: in.Status, Source: in.Source, CodeReferences: in.CodeReferences}, "user:"+user.ID)
		if err != nil {
			a.noteError(w, err)
			return
		}
		a.audit("user:"+user.ID, "memory.revise", updated.ID)
		respond(w, 200, updated)
	default:
		fail(w, 404, "Tidak ditemukan.")
	}
}
func (a *App) userAccessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, access.ErrDenied):
		fail(w, 403, "Akses workspace ditolak.")
	case errors.Is(err, access.ErrInvalid):
		fail(w, 400, "Masukan tidak valid.")
	case errors.Is(err, access.ErrWorkspaceLimit):
		fail(w, 429, "Batas ruang kerja, undangan, atau key tercapai.")
	default:
		a.internal(w, err)
	}
}
