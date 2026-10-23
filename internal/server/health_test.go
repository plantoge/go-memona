package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"memona/internal/access"
	"memona/internal/memory"
)

func TestStorageHealthAdminBoundary(t *testing.T) {
	f := setup(t)
	if w := request(f.app, "GET", "/healthz", "", nil, ""); w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"status":"ok"}` {
		t.Fatal(w.Code, w.Body.String())
	}
	_, token, err := f.auth.CreateKey(access.KeyInput{Name: "synthetic", Permissions: []string{"read", "write"}, Projects: []string{"*"}})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://localhost/api/storage-health", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	f.app.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("bearer grants admin diagnostics", w.Code)
	}
	c, csrf := login(t, f)
	n := makeNote(t, f, "demo", "fact", "PRIVATE_HEALTH_TITLE", nil)
	w = request(f.app, "GET", "/api/storage-health", "", c, "")
	var report memory.StorageHealth
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &report) != nil || !report.Integrity.Complete || report.Integrity.Notes != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, value := range []string{n.ID, n.Title, token, ".writer.lock", "data_dir"} {
		if strings.Contains(w.Body.String(), value) {
			t.Fatal("diagnostic leak")
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("diagnostics cached")
	}
	r = httptest.NewRequest("GET", "http://localhost/api/storage-health", nil)
	r.AddCookie(c)
	r.Header.Set("Origin", "https://attacker.example")
	w = httptest.NewRecorder()
	f.app.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross origin diagnostic allowed")
	}
	if w := request(f.app, "POST", "/api/storage-health", "{}", c, ""); w.Code != 403 {
		t.Fatal("CSRF bypass")
	}
	if w := request(f.app, "POST", "/api/storage-health", "{}", c, csrf); w.Code != 404 {
		t.Fatal("unexpected mutation endpoint", w.Code)
	}
	f.notes.Close()
	if w := request(f.app, "GET", "/api/storage-health", "", c, ""); w.Code != 500 || strings.Contains(w.Body.String(), "closed") {
		t.Fatal("internal diagnostic error leaked", w.Code, w.Body.String())
	}
	if w := request(f.app, "GET", "/healthz", "", nil, ""); w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"status":"ok"}` {
		t.Fatal("liveness became readiness")
	}
	if w := request(f.app, "POST", "/api/logout", "{}", c, csrf); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request(f.app, "GET", "/api/storage-health", "", c, ""); w.Code != 401 {
		t.Fatal("stale session allowed")
	}
}
