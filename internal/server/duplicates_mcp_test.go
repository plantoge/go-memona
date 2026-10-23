package server

import (
	"encoding/json"
	"memona/internal/access"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDuplicateToolScopeAndNoWrite(t *testing.T) {
	f := setup(t)
	n := makeNote(t, f, "demo", "knowledge", "Alur pasien", nil)
	private := makeNote(t, f, "private", "knowledge", n.Title, nil)
	ts := httptest.NewUnstartedServer(nil)
	app, err := New(Config{Listen: "127.0.0.1:8765", PublicURL: "http://" + ts.Listener.Addr().String()}, f.auth, f.notes)
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = app
	ts.Start()
	defer ts.Close()
	for _, permission := range []string{"read", "write"} {
		_, token, err := f.auth.CreateKey(access.KeyInput{Name: "synthetic-duplicate", Permissions: []string{permission}, Projects: []string{"demo"}})
		if err != nil {
			t.Fatal(err)
		}
		session := connect(t, ts.URL, token)
		args := map[string]any{"project": "demo", "title": n.Title, "content": n.Content}
		r := call(t, session, "find_duplicate_candidates", args)
		if permission == "write" {
			if !r.IsError {
				t.Fatal("write-only read allowed")
			}
			continue
		}
		var out duplicateResult
		if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &out) != nil || len(out.Candidates) != 1 || out.Candidates[0].ID != n.ID {
			t.Fatal(resultText(t, r))
		}
		if strings.Contains(resultText(t, r), private.ID) || strings.Contains(resultText(t, r), n.Content) {
			t.Fatal("private/content leak")
		}
		args["exclude_id"] = n.ID
		r = call(t, session, "find_duplicate_candidates", args)
		if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &out) != nil || len(out.Candidates) != 0 {
			t.Fatal("exclusion ignored")
		}
		args["project"] = "private"
		r = call(t, session, "find_duplicate_candidates", args)
		if !r.IsError {
			t.Fatal("private scope accepted")
		}
	}
	history, err := f.notes.History(n.ID)
	if err != nil || len(history) != 1 {
		t.Fatal("tool wrote note", err)
	}
}
