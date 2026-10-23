package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"memona/internal/access"
	"memona/internal/memory"
)

func TestContextBudgetIncludesReasonsAndEnvelope(t *testing.T) {
	f := setup(t)
	for i := 0; i < 12; i++ {
		makeNote(t, f, "demo", "knowledge", "Shared context fixture", func(in *memory.Input) { in.Content = strings.Repeat("x", 1750) })
	}
	ts := httptest.NewUnstartedServer(nil)
	app, err := New(Config{Listen: "127.0.0.1:8765", PublicURL: "http://" + ts.Listener.Addr().String()}, f.auth, f.notes)
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = app
	ts.Start()
	defer ts.Close()
	_, token, err := f.auth.CreateKey(access.KeyInput{Name: "synthetic-budget", Permissions: []string{"read"}, Projects: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	session := connect(t, ts.URL, token)
	r := call(t, session, "get_context", map[string]any{"project": "demo", "limit": 12})
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	raw := resultText(t, r)
	var out contextResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	if len(raw) > 24000 || !out.Truncated || len(out.Notes) == 0 || len(out.SelectionReasons) != len(out.Notes) {
		t.Fatalf("budget=%d notes=%d reasons=%d truncated=%v", len(raw), len(out.Notes), len(out.SelectionReasons), out.Truncated)
	}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > 24000 {
		t.Fatal("structured result exceeded budget", len(encoded), err)
	}
	for i, n := range out.Notes {
		if out.SelectionReasons[i].ID != n.ID {
			t.Fatal("reason alignment")
		}
	}
}
