package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestGoogleConfigRequiresPairAndSafeOrigin(t *testing.T) {
	base := Config{Listen: "127.0.0.1:8765", PublicURL: "http://localhost:8765"}
	base.GoogleClientID = "example"
	if ValidateConfig(base) == nil {
		t.Fatal("missing client secret accepted")
	}
	base.GoogleClientSecret = "synthetic"
	if err := ValidateConfig(base); err != nil {
		t.Fatalf("loopback development denied: %v", err)
	}
	base.PublicURL = "http://192.168.1.12:8765"
	base.AllowPrivateHTTP = true
	if ValidateConfig(base) == nil {
		t.Fatal("private HTTP Google login accepted")
	}
	base = Config{Listen: "127.0.0.1:8765", PublicURL: "https://example.org", SecureCookies: true, GoogleClientID: "example", GoogleClientSecret: "synthetic"}
	if err := ValidateConfig(base); err != nil {
		t.Fatalf("HTTPS Google login denied: %v", err)
	}
}

func TestPrivateHTTPWithTunnel(t *testing.T) {
	cfg := Config{Listen: "192.168.1.5:8765", PublicURL: "http://192.168.1.5:8765", AllowPrivateHTTP: true, TunnelURL: "https://memona.example.com"}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.GoogleClientID, cfg.GoogleClientSecret = "example", "synthetic"
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("Google login over HTTPS tunnel denied: %v", err)
	}
	cfg.GoogleClientID, cfg.GoogleClientSecret = "", ""
	for _, tunnel := range []string{"http://memona.example.com", "https://memona.example.com:8765", "https://memona.example.com/mcp", "https://memona.example.com?x=1", "https://192.168.1.5", "https://user@memona.example.com"} {
		cfg.TunnelURL = tunnel
		if err := ValidateConfig(cfg); err == nil {
			t.Fatalf("accepted tunnel URL %q", tunnel)
		}
	}
	cfg.TunnelURL = "https://memona.example.com"
	cfg.AllowPrivateHTTP = false
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("accepted tunnel without private HTTP")
	}
}

func TestPrivateHTTPAndTunnelOrigins(t *testing.T) {
	f := setup(t)
	app, err := New(Config{Listen: "192.168.1.5:8765", PublicURL: "http://192.168.1.5:8765", AllowPrivateHTTP: true, TunnelURL: "https://memona.example.com"}, f.auth, f.notes)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		host, origin string
		status       int
		sts          bool
	}{
		{"192.168.1.5:8765", "http://192.168.1.5:8765", 200, false},
		{"127.0.0.1:8765", "http://127.0.0.1:8765", 200, false},
		{"memona.example.com", "https://memona.example.com", 200, true},
		{"memona.example.com", "http://memona.example.com", 403, true},
		{"192.168.1.5:8765", "https://memona.example.com", 403, false},
		{"memona.example.com", "http://192.168.1.5:8765", 403, true},
		{"evil.example.com", "", 403, false},
		{"memona.example.com:8765", "", 403, false},
	} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8765/healthz", nil)
		r.Host = c.host
		r.Header.Set("Origin", c.origin)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != c.status || (w.Header().Get("Strict-Transport-Security") != "") != c.sts {
			t.Errorf("host=%s origin=%s status=%d HSTS=%q", c.host, c.origin, w.Code, w.Header().Get("Strict-Transport-Security"))
		}
	}
}

func TestTunnelAndPrivateSessions(t *testing.T) {
	f := setup(t)
	app, err := New(Config{Listen: "192.168.1.5:8765", PublicURL: "http://192.168.1.5:8765", AllowPrivateHTTP: true, TunnelURL: "https://memona.example.com"}, f.auth, f.notes)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host, origin, endpoint string
		secure                 bool
	}{
		{"192.168.1.5:8765", "http://192.168.1.5:8765", "http://192.168.1.5:8765/mcp", false},
		{"memona.example.com", "https://memona.example.com", "https://memona.example.com/mcp", true},
	} {
		send := func(method, path, body, csrf string, cookie *http.Cookie) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, "http://127.0.0.1:8765"+path, strings.NewReader(body))
			r.Host = tc.host
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", "application/json")
			if cookie != nil {
				r.AddCookie(cookie)
			}
			if csrf != "" {
				r.Header.Set("X-CSRF-Token", csrf)
			}
			w := httptest.NewRecorder()
			app.ServeHTTP(w, r)
			return w
		}
		w := send("POST", "/api/login", `{"password":"test-admin-password-12345"}`, "", nil)
		if w.Code != 200 {
			t.Fatalf("%s login status %d", tc.host, w.Code)
		}
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Secure != tc.secure || !cookies[0].HttpOnly || cookies[0].Domain != "" {
			t.Fatalf("%s incorrect session cookie", tc.host)
		}
		var auth struct {
			CSRF string `json:"csrf"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &auth); err != nil {
			t.Fatal(err)
		}
		if w := send("GET", "/api/session", "", "", cookies[0]); w.Code != 200 {
			t.Fatalf("%s session status %d", tc.host, w.Code)
		}
		if w := send("POST", "/api/logout", `{}`, "wrong", cookies[0]); w.Code != 403 {
			t.Fatalf("%s CSRF status %d", tc.host, w.Code)
		}
		w = send("GET", "/api/server-info", "", "", cookies[0])
		if w.Code != 200 || !strings.Contains(w.Body.String(), tc.endpoint) {
			t.Fatalf("%s endpoint status %d: %s", tc.host, w.Code, w.Body.String())
		}
		w = send("POST", "/api/logout", `{}`, auth.CSRF, cookies[0])
		if w.Code != 200 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].Secure != tc.secure {
			t.Fatalf("%s logout cookie status %d", tc.host, w.Code)
		}
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
