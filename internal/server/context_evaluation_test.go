package server

import (
	"encoding/json"
	"fmt"
	"memona/internal/access"
	"memona/internal/memory"
	"net/http/httptest"
	"testing"
)

// Synthetic retrieval evaluation through actual MCP transport, not a helper-only
// ranking assertion. Expected IDs are explicit judgments for this small corpus.
func TestContextRetrievalEvaluation(t *testing.T) {
	f := setup(t)
	flag := true
	overview := makeNote(t, f, "demo", "knowledge", "Ringkasan proyek", func(in *memory.Input) { in.IsProjectSummary = &flag })
	decision := makeNote(t, f, "demo", "decision", "Pilihan penyimpanan", func(in *memory.Input) { in.Content = "alur pasien expected_revision" })
	global := makeNote(t, f, "global", "knowledge", "Aturan bersama", func(in *memory.Input) { in.Content = "alur pasien expected_revision" })
	private := makeNote(t, f, "private", "decision", "alur pasien expected_revision", nil)
	testingNote := makeNote(t, f, "demo", "knowledge", "alur pasien expected_revision synthetic", func(in *memory.Input) { flag := true; in.IsTesting = &flag })
	old := makeNote(t, f, "demo", "decision", "alur pasien expected_revision usang", nil)
	replacement := makeNote(t, f, "demo", "decision", "alur pasien expected_revision keputusan terbaru", func(in *memory.Input) { in.Supersedes = []string{old.ID} })
	for i := 0; i < 8; i++ {
		makeNote(t, f, "demo", "handoff", fmt.Sprintf("alur pasien expected_revision checkpoint %d", i), nil)
	}
	_, token, err := f.auth.CreateKey(access.KeyInput{Name: "synthetic-evaluation", Permissions: []string{"read"}, Projects: []string{"demo", "global"}})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(nil)
	app, err := New(Config{Listen: "127.0.0.1:8765", PublicURL: "http://" + ts.Listener.Addr().String()}, f.auth, f.notes)
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = app
	ts.Start()
	defer ts.Close()
	session := connect(t, ts.URL, token)
	for _, query := range []string{"alur pasien", "expected_revision"} {
		t.Run(query, func(t *testing.T) {
			r := call(t, session, "get_context", map[string]any{"project": "demo", "query": query, "limit": 6})
			var out contextResult
			if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &out) != nil {
				t.Fatal(resultText(t, r))
			}
			ids := map[string]bool{}
			handoffs := 0
			for _, n := range out.Notes {
				ids[n.ID] = true
				if n.Kind == "handoff" {
					handoffs++
				}
			}
			for _, id := range []string{overview.ID, decision.ID, global.ID, replacement.ID} {
				if !ids[id] {
					t.Fatalf("expected relevant note missing %s: %s", id, resultText(t, r))
				}
			}
			if ids[private.ID] || ids[old.ID] || ids[testingNote.ID] || handoffs != 2 || !out.Truncated || out.Notes[0].ID != overview.ID {
				t.Fatal("scope/supersession/quota/overview contract", resultText(t, r))
			}
			encoded, _ := json.Marshal(out.Notes)
			if len(encoded) > 24000 {
				t.Fatal("context notes budget exceeded")
			}
		})
	}
	r := call(t, session, "get_context", map[string]any{"project": "demo", "kind": "knowledge", "include_testing": true})
	var testingOut contextResult
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &testingOut) != nil {
		t.Fatal(resultText(t, r))
	}
	foundTesting := false
	for _, n := range testingOut.Notes {
		if n.ID == testingNote.ID {
			foundTesting = true
		}
	}
	if !foundTesting {
		t.Fatal("explicit testing inclusion ignored")
	}
	// Testing is curation metadata, not authorization: search still surfaces it.
	search := call(t, session, "search_memory", map[string]any{"project": "demo", "query": "synthetic"})
	var searchOut results
	if search.IsError || json.Unmarshal([]byte(resultText(t, search)), &searchOut) != nil {
		t.Fatal(resultText(t, search))
	}
	foundSearch := false
	for _, item := range searchOut.Items {
		if item.ID == testingNote.ID && item.IsTesting {
			foundSearch = true
		}
	}
	if !foundSearch {
		t.Fatal("testing label hidden from explicit search")
	}
	if len(testingOut.SelectionReasons) != len(testingOut.Notes) {
		t.Fatal("missing selection explanations")
	}
	r = call(t, session, "get_context", map[string]any{"project": "demo", "kind": "handoff", "limit": 6})
	var out contextResult
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &out) != nil || len(out.Notes) != 6 || !out.Truncated {
		t.Fatal("explicit handoff request unexpectedly capped", resultText(t, r))
	}
	for _, n := range out.Notes {
		if n.Kind != "handoff" {
			t.Fatal("kind filter ignored")
		}
	}
}
