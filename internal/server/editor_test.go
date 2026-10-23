package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"memona/internal/memory"
)

// Use the parent handler when delegation is wired. Until then, exercise the
// isolated delegate behind exactly the existing boundary/admin middleware.
func editorHandler(t *testing.T, f *fixture, cookie *http.Cookie, csrf string) http.Handler {
	t.Helper()
	probe := request(f.app, "POST", "/api/notes", `{}`, cookie, "")
	if probe.Code != http.StatusForbidden {
		t.Fatalf("parent CSRF guard: %d", probe.Code)
	}
	if w := request(f.app, "POST", "/api/notes", `{}`, cookie, csrf); w.Code == http.StatusBadRequest {
		return f.app
	}
	return f.app.boundary(f.app.admin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !f.app.editorAPI(w, r) {
			f.app.api(w, r)
		}
	})))
}

func editorBody(t *testing.T, input memory.Input, revision int) string {
	t.Helper()
	var value any = input
	if revision >= 0 {
		value = struct {
			ExpectedRevision int `json:"expected_revision"`
			memory.Input
		}{revision, input}
	}
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func editorNote(t *testing.T, w *httptest.ResponseRecorder, status int) memory.Note {
	t.Helper()
	if w.Code != status {
		t.Fatalf("response: %d %s", w.Code, w.Body.String())
	}
	var n memory.Note
	if err := json.Unmarshal(w.Body.Bytes(), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEditorCreateReviseAndGuards(t *testing.T) {
	f := setup(t)
	cookie, csrf := login(t, f)
	h := editorHandler(t, f, cookie, csrf)
	input := memory.Input{Project: "demo", Kind: "knowledge", Title: "Editor fixture", Content: "# Original", Status: "unverified", Source: "test fixture"}
	body := editorBody(t, input, -1)
	for _, handler := range []http.Handler{f.app, h} {
		if w := request(handler, "POST", "/api/notes", body, nil, ""); w.Code != 401 {
			t.Fatalf("auth: %d", w.Code)
		}
		if w := request(handler, "POST", "/api/notes", body, cookie, ""); w.Code != 403 {
			t.Fatalf("CSRF: %d", w.Code)
		}
	}
	created := editorNote(t, request(h, "POST", "/api/notes", body, cookie, csrf), 201)
	if created.Revision != 1 || created.Author != "admin" || created.Title != input.Title {
		t.Fatal("unexpected created note", created)
	}
	// Verify the integrated parent handler when the parallel wiring is present.
	probe := request(f.app, "POST", "/api/notes/"+created.ID+"/revise", `{}`, cookie, csrf)
	if probe.Code != 404 && probe.Code != 400 {
		t.Fatalf("parent delegation: %d", probe.Code)
	}
	if probe.Code == 404 {
		t.Log("parent delegation not yet wired; isolated handler exercised behind admin/boundary")
	}
	input.Title, input.Content = "Revised fixture", "<script>alert(1)</script>\nNew content"
	for _, handler := range []http.Handler{f.app, h} {
		if w := request(handler, "POST", "/api/notes/"+created.ID+"/revise", editorBody(t, input, 1), cookie, ""); w.Code != 403 {
			t.Fatalf("revise CSRF: %d", w.Code)
		}
	}
	revised := editorNote(t, request(h, "POST", "/api/notes/"+created.ID+"/revise", editorBody(t, input, 1), cookie, csrf), 200)
	if revised.ID != created.ID || revised.Revision != 2 || revised.Content != input.Content {
		t.Fatal("unexpected revised note", revised)
	}
	if w := request(h, "POST", "/api/notes/"+created.ID+"/revise", editorBody(t, input, 1), cookie, csrf); w.Code != 409 {
		t.Fatalf("stale revision accepted: %d", w.Code)
	}
	history, err := f.notes.History(created.ID)
	if err != nil || len(history) != 2 || history[0].Content != "# Original" {
		t.Fatalf("immutable history: %v %v", history, err)
	}
	activity, err := f.auth.ActivityContext(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range activity {
		if entry.Target == created.ID {
			seen[entry.Action] = true
		}
	}
	if !seen["note.create"] || !seen["note.revise"] {
		t.Fatal("missing editor audit actions", seen)
	}
}

func TestEditorValidationAndMetadataPreservation(t *testing.T) {
	f := setup(t)
	cookie, csrf := login(t, f)
	h := editorHandler(t, f, cookie, csrf)
	input := memory.Input{Project: "demo", Kind: "knowledge", Title: "Metadata fixture", Content: "content", Status: "unverified", Source: "test"}
	for _, body := range []string{
		`{"project":"demo","kind":"fact","title":"x","content":"x","source":"test","status":"active"}`,
		`{"project":"demo","kind":"skill","title":"x","content":"x","source":"test","status":"verified"}`,
		`{"project":"demo","kind":"fact","title":"x","content":"x","source":"","status":"verified"}`,
		strings.TrimSuffix(editorBody(t, input, -1), "}") + `,"unexpected":true}`,
		editorBody(t, input, -1) + `{}`,
	} {
		if w := request(h, "POST", "/api/notes", body, cookie, csrf); w.Code != 400 {
			t.Fatalf("invalid input accepted: %d %s", w.Code, w.Body.String())
		}
	}
	target := editorNote(t, request(h, "POST", "/api/notes", editorBody(t, input, -1), cookie, csrf), 201)
	flag := true
	input.IsProjectSummary, input.RelatedTo = &flag, []string{target.ID}
	note := editorNote(t, request(h, "POST", "/api/notes", editorBody(t, input, -1), cookie, csrf), 201)
	input.IsProjectSummary, input.RelatedTo = nil, nil
	input.Title = "Updated content, same metadata"
	path := "/api/notes/" + note.ID + "/revise"
	revised := editorNote(t, request(h, "POST", path, editorBody(t, input, 1), cookie, csrf), 200)
	if !revised.IsProjectSummary || len(revised.RelatedTo) != 1 || revised.RelatedTo[0] != target.ID {
		t.Fatal("omitted metadata was not preserved", revised)
	}
	for _, mutate := range []func(*memory.Input){
		func(in *memory.Input) { in.Project = "other" },
		func(in *memory.Input) { in.Kind = "fact" },
	} {
		invalid := input
		mutate(&invalid)
		if w := request(h, "POST", path, editorBody(t, invalid, 2), cookie, csrf); w.Code != 400 {
			t.Fatalf("immutable field accepted: %d", w.Code)
		}
	}
	if w := request(h, "POST", path, strings.TrimSuffix(editorBody(t, input, 2), "}")+`,"revision":2}`, cookie, csrf); w.Code != 400 {
		t.Fatalf("unknown revision property accepted: %d", w.Code)
	}
	if _, err := f.notes.Archive(note.ID, 2, "test"); err != nil {
		t.Fatal(err)
	}
	if w := request(h, "POST", path, editorBody(t, input, 3), cookie, csrf); w.Code != 409 {
		t.Fatalf("archived revision accepted: %d", w.Code)
	}
}

func TestEditorActiveSkillReturnsToDraftAndNonmatches(t *testing.T) {
	f := setup(t)
	cookie, csrf := login(t, f)
	h := editorHandler(t, f, cookie, csrf)
	input := memory.Input{Project: "demo", Kind: "skill", Title: "Skill fixture", Content: "procedure", Status: "draft", Source: "test"}
	note := editorNote(t, request(h, "POST", "/api/notes", editorBody(t, input, -1), cookie, csrf), 201)
	if _, err := f.notes.Publish(note.ID, 1, "admin"); err != nil {
		t.Fatal(err)
	}
	input.Status = "active"
	revised := editorNote(t, request(h, "POST", "/api/notes/"+note.ID+"/revise", editorBody(t, input, 2), cookie, csrf), 200)
	if revised.Status != "draft" {
		t.Fatal("active skill edit did not require review")
	}
	for _, path := range []string{"/api/notes", "/api/notes/abc", "/api/notes/abc/revise/extra", "/api/keys"} {
		method := "POST"
		if path == "/api/notes" {
			method = "GET"
		}
		if f.app.editorAPI(httptest.NewRecorder(), httptest.NewRequest(method, path, nil)) {
			t.Fatalf("unrelated route handled: %s %s", method, path)
		}
	}
}
