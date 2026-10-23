package server

import (
	"net/http/httptest"
	"testing"
)

func TestPrivateHTTP(t *testing.T) {
	cfg := Config{Listen: "0.0.0.0:8765", PublicURL: "http://100.76.35.38:8765", AllowPrivateHTTP: true}
	if e := ValidateConfig(cfg); e != nil {
		t.Fatal(e)
	}
	f := setup(t)
	app, e := New(cfg, f.auth, f.notes)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		host, origin string
		status       int
	}{
		{"100.76.35.38:8765", "http://100.76.35.38:8765", 200},
		{"192.168.1.6:8765", "http://192.168.1.6:8765", 200},
		{"127.0.0.1:8765", "", 200},
		{"8.8.8.8:8765", "", 403},
		{"evil.example:8765", "", 403},
		{"100.76.35.38:9999", "", 403},
		{"100.76.35.38:8765", "http://evil.example:8765", 403},
	} {
		r := httptest.NewRequest("GET", "http://"+c.host+"/healthz", nil)
		r.Header.Set("Origin", c.origin)
		w := httptest.NewRecorder()
		app.ServeHTTP(w, r)
		if w.Code != c.status {
			t.Fatalf("%s %s status=%d", c.host, c.origin, w.Code)
		}
	}
	for _, origin := range []string{"http://8.8.8.8:8765", "http://example.com:8765", "http://0.0.0.0:8765", "http://100.76.35.38:9999"} {
		cfg.PublicURL = origin
		if ValidateConfig(cfg) == nil {
			t.Fatal("unsafe origin accepted", origin)
		}
	}
}
