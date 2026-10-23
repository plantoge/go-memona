package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerInfoRequiresAdminSession(t *testing.T) {
	f := setup(t)
	for _, cookie := range []*http.Cookie{nil, {Name: "memona_session", Value: "invalid-session"}} {
		w := request(f.app, "GET", "/api/server-info", "", cookie, "")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("without valid session: got %d", w.Code)
		}
	}

	cookie, csrf := login(t, f)
	w := request(f.app, "GET", "/api/server-info", "", cookie, "")
	if w.Code != http.StatusOK {
		t.Fatalf("authenticated GET: got %d", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("server metadata should not be cached")
	}
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out["mcp_endpoint"] != "http://localhost/mcp" || out["transport"] != "Streamable HTTP" {
		t.Fatal("unexpected public metadata fields")
	}
	if w := request(f.app, "POST", "/api/server-info", `{}`, cookie, csrf); w.Code != http.StatusNotFound {
		t.Fatalf("metadata endpoint must be read-only: got %d", w.Code)
	}
}

func TestServerInfoUsesConfiguredPublicURL(t *testing.T) {
	f := setup(t)
	cookie, _ := login(t, f)
	cases := []struct {
		name     string
		cfg      Config
		host     string
		endpoint string
	}{
		{"https", Config{Listen: "0.0.0.0:8765", PublicURL: "https://memory.example", SecureCookies: true}, "memory.example", "https://memory.example/mcp"},
		{"trailing slash", Config{Listen: "127.0.0.1:8765", PublicURL: "http://localhost/"}, "localhost", "http://localhost/mcp"},
		{"private host alias", Config{Listen: "0.0.0.0:8765", PublicURL: "http://192.168.1.20:8765", AllowPrivateHTTP: true}, "100.64.1.5:8765", "http://192.168.1.20:8765/mcp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, err := New(tc.cfg, f.auth, f.notes)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "http://"+tc.host+"/api/server-info", nil)
			r.AddCookie(cookie)
			w := httptest.NewRecorder()
			app.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("metadata: got %d", w.Code)
			}
			var out map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out["mcp_endpoint"] != tc.endpoint {
				t.Fatalf("endpoint must come from configured public URL, got %q", out["mcp_endpoint"])
			}

			r = httptest.NewRequest("GET", "http://attacker.example/api/server-info", nil)
			r.AddCookie(cookie)
			w = httptest.NewRecorder()
			app.ServeHTTP(w, r)
			if w.Code != http.StatusForbidden {
				t.Fatalf("unexpected Host accepted: %d", w.Code)
			}
		})
	}
}
