package server

import (
	"encoding/json"
	"memona/internal/memory"
	"testing"
)

func TestEditorCurationMetadataAndDraftReview(t *testing.T) {
	f := setup(t)
	cookie, csrf := login(t, f)
	testingFlag := true
	input := memory.Input{Project: "demo", Kind: "skill", Title: "Synthetic review", Content: "Synthetic procedure", Status: "draft", Source: "synthetic", IsTesting: &testingFlag, SkillMetadata: &memory.SkillMetadata{Trigger: "Review fixtures", Version: "1.0", Compatibility: []string{"OpenCode"}, TestEvidence: "unit fixture"}}
	created := editorNote(t, request(f.app, "POST", "/api/notes", editorBody(t, input, -1), cookie, csrf), 201)
	if !created.IsTesting || created.SkillMetadata == nil || created.SkillMetadata.Version != "1.0" {
		t.Fatal(created)
	}
	published, err := f.notes.Publish(created.ID, created.Revision, "admin")
	if err != nil {
		t.Fatal(err)
	}
	input.IsTesting = nil
	input.SkillMetadata = nil
	input.Content = "Revised procedure"
	revised := editorNote(t, request(f.app, "POST", "/api/notes/"+created.ID+"/revise", editorBody(t, input, published.Revision), cookie, csrf), 200)
	if revised.Status != "draft" || !revised.IsTesting || revised.SkillMetadata == nil {
		t.Fatal("omission/review failed", revised)
	}
	testingFlag = false
	input.IsTesting = &testingFlag
	input.SkillMetadata = &memory.SkillMetadata{}
	cleared := editorNote(t, request(f.app, "POST", "/api/notes/"+created.ID+"/revise", editorBody(t, input, revised.Revision), cookie, csrf), 200)
	if cleared.IsTesting || cleared.SkillMetadata != nil {
		t.Fatal("clear failed", cleared)
	}
	// API detail output advertises both metadata fields using the same note shape.
	w := request(f.app, "GET", "/api/notes/"+created.ID+"?history=paged", "", cookie, "")
	var out struct {
		History []memory.Note `json:"history"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.History) != 4 || out.History[3].SkillMetadata == nil {
		t.Fatal(w.Code, w.Body.String())
	}
}
