package server

import (
	"net/http/httptest"
	"testing"
)

func TestNetworkListenRequiresHTTPS(t *testing.T) {
	cases := []struct {
		name, listen, origin string
		secure, valid        bool
	}{
		{"local", "127.0.0.1:8765", "http://127.0.0.1:8765", false, true},
		{"wildcard behind proxy", "0.0.0.0:8765", "https://memona.example.com", true, true},
		{"tailscale behind proxy", "100.64.0.10:8765", "https://memona.example.com", true, true},
		{"IPv6 wildcard", "[::]:8765", "https://memona.example.com", true, true},
		{"wildcard plain HTTP", "0.0.0.0:8765", "http://100.64.0.10:8765", false, false},
		{"wildcard local origin", "0.0.0.0:8765", "http://localhost:8765", false, false},
		{"insecure cookie", "0.0.0.0:8765", "https://memona.example.com", false, false},
		{"bad bind", "not-an-ip:8765", "https://memona.example.com", true, false},
		{"bad port", "0.0.0.0:0", "https://memona.example.com", true, false},
		{"wildcard origin", "0.0.0.0:8765", "https://0.0.0.0:8765", true, false},
		{"bad origin", "0.0.0.0:8765", "https://memona.example.com/mcp", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateConfig(Config{Listen: c.listen, PublicURL: c.origin, SecureCookies: c.secure})
			if (err == nil) != c.valid {
				t.Fatalf("valid=%v err=%v", c.valid, err)
			}
		})
	}
}

func TestNetworkOriginStillEnforcesHost(t *testing.T) {
	f := setup(t)
	app, e := New(Config{Listen: "0.0.0.0:8765", PublicURL: "https://memona.example.com", SecureCookies: true}, f.auth, f.notes)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		host, origin string
		status       int
	}{
		{"memona.example.com", "https://memona.example.com", 200},
		{"100.64.0.10:8765", "", 403},
		{"memona.example.com", "https://attacker.example", 403},
	} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8765/healthz", nil)
		r.Host = c.host
		r.Header.Set("Origin", c.origin)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != c.status {
			t.Fatalf("host=%s origin=%s status=%d", c.host, c.origin, w.Code)
		}
	}
}
