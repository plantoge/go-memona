package server

import (
	"fmt"
	"memona/internal/memory"
	"testing"
)

func TestSkillRestoreContentNewDraftAndGuards(t *testing.T) {
	f := setup(t)
	in := memory.Input{Project: "demo", Kind: "skill", Title: "Original skill", Content: "Original procedure", Source: "synthetic evidence", Status: "draft"}
	n, err := f.notes.Create(in, "tester")
	if err != nil {
		t.Fatal(err)
	}
	original := n
	in.Title = "Changed skill"
	in.Content = "Changed procedure"
	n, err = f.notes.Revise(n.ID, n.Revision, in, "tester")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/notes/" + n.ID + "/restore-skill-content"
	body := fmt.Sprintf(`{"expected_revision":%d,"source_revision":1}`, n.Revision)
	if w := request(f.app, "POST", path, body, nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	cookie, csrf := login(t, f)
	if w := request(f.app, "POST", path, body, cookie, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	restored := editorNote(t, request(f.app, "POST", path, body, cookie, csrf), 201)
	if restored.Revision != 3 || restored.Status != "draft" || restored.Content != original.Content || restored.Title != original.Title || restored.Source != original.Source {
		t.Fatal(restored)
	}
	if w := request(f.app, "POST", path, body, cookie, csrf); w.Code != 409 {
		t.Fatal("stale accepted", w.Code)
	}
	history, err := f.notes.History(n.ID)
	if err != nil || len(history) != 3 || history[0].Content != original.Content || history[1].Content != in.Content {
		t.Fatal("history changed", err)
	}
	if w := request(f.app, "POST", path, `{"expected_revision":3,"source_revision":4}`, cookie, csrf); w.Code != 400 {
		t.Fatal(w.Code)
	}
	ordinary := makeNote(t, f, "demo", "knowledge", "not a skill", nil)
	if w := request(f.app, "POST", "/api/notes/"+ordinary.ID+"/restore-skill-content", `{"expected_revision":1,"source_revision":1}`, cookie, csrf); w.Code != 409 {
		t.Fatal(w.Code)
	}
}
