package server

import (
	"context"
	"encoding/json"
	"memona/internal/access"
	"memona/internal/memory"
	"strings"
	"testing"
)

func TestDuplicatesAdminBoundaryAndNoMutation(t *testing.T) {
	f := setup(t)
	in := memory.Input{Project: "demo", Kind: "knowledge", Title: "Alur pasien", Content: "loket pemeriksaan farmasi kasir", Status: "unverified", Source: "synthetic"}
	n, err := f.notes.Create(in, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	other := in
	other.Project = "private"
	if _, err = f.notes.Create(other, "synthetic"); err != nil {
		t.Fatal(err)
	}
	body := `{"project":"demo","title":"Alur pasien","content":"loket pemeriksaan farmasi kasir"}`
	if w := request(f.app, "POST", "/api/duplicate-candidates", body, nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	cookie, csrf := login(t, f)
	if w := request(f.app, "POST", "/api/duplicate-candidates", body, cookie, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	w := request(f.app, "POST", "/api/duplicate-candidates", body, cookie, csrf)
	var out struct {
		Candidates []duplicateCandidate `json:"candidates"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Candidates) != 1 || out.Candidates[0].ID != n.ID {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), in.Content) || strings.Contains(w.Body.String(), "private") {
		t.Fatal("content/scope leak")
	}
	excluded := strings.TrimSuffix(body, "}") + `,"exclude_id":"` + n.ID + `"}`
	w = request(f.app, "POST", "/api/duplicate-candidates", excluded, cookie, csrf)
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Candidates) != 0 {
		t.Fatal(w.Body.String())
	}
	latest, err := f.notes.Get(n.ID)
	if err != nil || latest.Revision != 1 {
		t.Fatal("read mutated note", err)
	}
	if w = request(f.app, "POST", "/api/duplicate-candidates", `{"project":"demo","title":""}`, cookie, csrf); w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func TestQuerySnapshotUsesLexicalRankingAfterScope(t *testing.T) {
	key := access.Key{Permissions: []string{"read"}, Projects: []string{"demo"}}
	all := []memory.Note{
		{ID: "body", Project: "demo", Title: "Other", Content: "pasien", UpdatedAt: "2026"},
		{ID: "title", Project: "demo", Title: "Pasien", UpdatedAt: "2025"},
		{ID: "hidden", Project: "private", Title: "Pasien"},
		{ID: "archived", Project: "demo", Title: "Pasien", Archived: true},
	}
	got, err := querySnapshot(context.Background(), key, queryInput{Query: "pasien kasir"}, all)
	if err != nil || len(got) != 2 || got[0].ID != "title" || got[1].ID != "body" {
		t.Fatal(got, err)
	}
	if _, err = querySnapshot(context.Background(), key, queryInput{Project: "private"}, all); err == nil {
		t.Fatal("scope accepted")
	}
	if _, err = querySnapshot(context.Background(), key, queryInput{Query: strings.Repeat("x", 2001)}, all); err == nil {
		t.Fatal("oversized query accepted")
	}
}

func TestAdminRankedSearchPreservesArchivesAndPartialIdentifiers(t *testing.T) {
	f := setup(t)
	cookie, _ := login(t, f)
	n := makeNote(t, f, "demo", "knowledge", "expected_revision", nil)
	if _, err := f.notes.Archive(n.ID, n.Revision, "synthetic"); err != nil {
		t.Fatal(err)
	}
	w := request(f.app, "GET", "/api/notes?q=expected_rev", "", cookie, "")
	var out []noteView
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out) != 1 || !out[0].Archived || out[0].ID != n.ID {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(f.app, "GET", "/api/notes?q="+strings.Repeat("x", 2001), "", cookie, "")
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
}
