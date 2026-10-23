package server

import (
	"errors"
	"memona/internal/access"
	"net/http"
	"strconv"
)

func (a *App) keyPageAPI(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			fail(w, 400, "Batas halaman harus 1–100.")
			return
		}
		limit = n
	}
	page, err := a.auth.KeysPageContext(r.Context(), limit, r.URL.Query().Get("cursor"))
	if err != nil {
		if errors.Is(err, access.ErrInvalid) {
			fail(w, 400, "Cursor halaman tidak valid.")
		} else {
			a.internal(w, err)
		}
		return
	}
	events, err := a.auth.ActivityContext(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	respond(w, 200, map[string]any{"items": page.Items, "next_cursor": page.NextCursor, "usage": recordedKeyUsage(page.Items, events), "audit_window": 200})
}
