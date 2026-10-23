package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"memona/internal/access"
	"memona/internal/memory"
)

func TestReviewMetadataMCPAndRetrieval(t *testing.T) {
	f := setup(t)
	ts := httptest.NewUnstartedServer(nil)
	app, err := New(Config{Listen: "127.0.0.1:8765", PublicURL: "http://" + ts.Listener.Addr().String()}, f.auth, f.notes)
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler = app
	ts.Start()
	defer ts.Close()
	_, token, err := f.auth.CreateKey(access.KeyInput{Name: "synthetic-review", Permissions: []string{"read", "write"}, Projects: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	session := connect(t, ts.URL, token)
	args := map[string]any{"project": "demo", "kind": "decision", "title": "Review fixture", "content": "Temporary conclusion pending source review", "status": "verified", "source": "synthetic evidence", "verified_at": "2026-10-23T09:00:00Z", "needs_review": true, "operation_id": "review-metadata-001"}
	r := call(t, session, "save_memory", args)
	var n memory.Note
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &n) != nil || !n.NeedsReview || n.VerifiedAt == "" {
		t.Fatal(resultText(t, r))
	}
	r = call(t, session, "save_memory", args)
	var replay memory.Note
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &replay) != nil || replay.ID != n.ID || replay.Revision != 1 {
		t.Fatal("replay failed", resultText(t, r))
	}
	r = call(t, session, "search_memory", map[string]any{"project": "demo", "query": "Review fixture"})
	var found results
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &found) != nil || len(found.Items) != 1 || !found.Items[0].NeedsReview || found.Items[0].VerifiedAt != n.VerifiedAt {
		t.Fatal(resultText(t, r))
	}
	r = call(t, session, "get_context", map[string]any{"project": "demo"})
	var out contextResult
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &out) != nil || len(out.Notes) != 1 || !out.Notes[0].NeedsReview || !strings.Contains(out.SelectionReasons[0].Reason, "WARNING") {
		t.Fatal(resultText(t, r))
	}
	// No truth inference: another contradictory decision does not replace this one.
	other := makeNote(t, f, "demo", "decision", "Review fixture disagreement", nil)
	r = call(t, session, "get_context", map[string]any{"project": "demo"})
	out = contextResult{}
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &out) != nil || len(out.Notes) != 2 {
		t.Fatal("implicit conflict resolution", resultText(t, r))
	}
	for _, item := range out.Notes {
		if item.ID != n.ID && item.ID != other.ID {
			t.Fatal("unexpected note")
		}
	}
	args["id"] = n.ID
	args["expected_revision"] = 1
	args["operation_id"] = "review-clear-001"
	args["verified_at"] = ""
	args["needs_review"] = false
	r = call(t, session, "revise_memory", args)
	n = memory.Note{}
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &n) != nil || n.NeedsReview || n.VerifiedAt != "" {
		t.Fatal("clear failed", resultText(t, r))
	}
}

func TestEditorReviewMetadataSecurityAndHistory(t *testing.T) {
	f := setup(t)
	cookie, csrf := login(t, f)
	flag := true
	stamp := "2026-10-23T09:00:00Z"
	input := memory.Input{Project: "demo", Kind: "knowledge", Title: "Review fixture", Content: "Synthetic uncertainty", Status: "verified", Source: "synthetic evidence", VerifiedAt: &stamp, NeedsReview: &flag}
	body := editorBody(t, input, -1)
	if w := request(f.app, "POST", "/api/notes", body, nil, ""); w.Code != 401 {
		t.Fatal("unauthenticated", w.Code)
	}
	if w := request(f.app, "POST", "/api/notes", body, cookie, ""); w.Code != 403 {
		t.Fatal("CSRF", w.Code)
	}
	n := editorNote(t, request(f.app, "POST", "/api/notes", body, cookie, csrf), 201)
	if !n.NeedsReview || n.VerifiedAt != stamp {
		t.Fatal(n)
	}
	input.VerifiedAt = nil
	input.NeedsReview = nil
	revised := editorNote(t, request(f.app, "POST", "/api/notes/"+n.ID+"/revise", editorBody(t, input, 1), cookie, csrf), 200)
	if !revised.NeedsReview || revised.VerifiedAt != stamp {
		t.Fatal("preservation", revised)
	}
	if w := request(f.app, "POST", "/api/notes/"+n.ID+"/revise", editorBody(t, input, 1), cookie, csrf); w.Code != 409 {
		t.Fatal("stale expected revision", w.Code)
	}
	history, err := f.notes.History(n.ID)
	if err != nil || len(history) != 2 || !history[0].NeedsReview || history[0].VerifiedAt != stamp {
		t.Fatal("history", err)
	}
}
