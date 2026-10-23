package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"memona/internal/access"
	"memona/internal/memory"
)

func makeNote(t *testing.T, f *fixture, project, kind, title string, update func(*memory.Input)) memory.Note {
	t.Helper()
	in := memory.Input{Project: project, Kind: kind, Title: title, Content: "Synthetic fixture", Status: "unverified"}
	if update != nil {
		update(&in)
	}
	n, err := f.notes.Create(in, "tester")
	if err != nil {
		t.Fatal(err)
	}
	return n
}
func TestMCPConnectionsAndContext(t *testing.T) {
	f := setup(t)
	old := makeNote(t, f, "demo", "decision", "OLD-MARKER", nil)
	replacement := makeNote(t, f, "demo", "decision", "CURRENT-MARKER", func(in *memory.Input) { in.Supersedes = []string{old.ID} })
	flag := true
	overview := makeNote(t, f, "demo", "knowledge", "OVERVIEW-MARKER", func(in *memory.Input) { in.IsProjectSummary = &flag; in.RelatedTo = []string{replacement.ID} })
	private := makeNote(t, f, "private", "knowledge", "DO-NOT-LEAK", nil)
	_ = makeNote(t, f, "demo", "handoff", "NEWEST-MARKER", nil)
	_, token, err := f.auth.CreateKey(access.KeyInput{Name: "test-relations", Permissions: []string{"read", "write"}, Projects: []string{"demo"}})
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
	r := call(t, session, "get_context", map[string]any{"project": "demo", "limit": 1, "query": "no-match"})
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	var ctx contextResult
	if err := json.Unmarshal([]byte(resultText(t, r)), &ctx); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Notes) != 1 || ctx.Notes[0].ID != overview.ID {
		t.Fatalf("overview not prioritized: %#v", ctx)
	}
	r = call(t, session, "get_context", map[string]any{"project": "demo"})
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	if err := json.Unmarshal([]byte(resultText(t, r)), &ctx); err != nil {
		t.Fatal(err)
	}
	for _, n := range ctx.Notes {
		if n.ID == old.ID || n.ID == private.ID {
			t.Fatal("stale/private note in context")
		}
	}
	r = call(t, session, "get_context", map[string]any{"project": "demo", "kind": "decision"})
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &ctx) != nil || len(ctx.Notes) != 1 || ctx.Notes[0].ID != replacement.ID {
		t.Fatal("context kind filter ignored", resultText(t, r))
	}
	if strings.Contains(resultText(t, r), private.Title) {
		t.Fatal("private title leaked")
	}
	r = call(t, session, "read_memory", map[string]any{"id": old.ID})
	var view noteView
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &view) != nil || len(view.Connections.SupersededBy) != 1 || view.Connections.SupersededBy[0].ID != replacement.ID {
		t.Fatal("replacement warning absent", resultText(t, r))
	}
	r = call(t, session, "get_memory_relations", map[string]any{"id": replacement.ID})
	var links noteConnections
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &links) != nil || len(links.ReferencedBy) != 1 || links.ReferencedBy[0].ID != overview.ID || len(links.Supersedes) != 1 {
		t.Fatal("backlinks missing", resultText(t, r))
	}
	r = call(t, session, "search_memory", map[string]any{"project": "demo", "query": "OLD-MARKER"})
	var found results
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &found) != nil || len(found.Items) != 1 || len(found.Items[0].SupersededBy) != 1 {
		t.Fatal("search warning missing", resultText(t, r))
	}
	for _, name := range []string{"read_memory", "get_memory_relations"} {
		r = call(t, session, name, map[string]any{"id": private.ID})
		if !r.IsError || strings.Contains(resultText(t, r), private.Title) {
			t.Fatal("private relations exposed", name)
		}
	}
	for _, id := range []string{private.ID, strings.Repeat("0", 32)} {
		r = call(t, session, "save_memory", map[string]any{"project": "demo", "kind": "fact", "title": "invalid link", "content": "fixture", "status": "unverified", "source": "", "related_to": []string{id}})
		if !r.IsError || strings.Contains(resultText(t, r), private.Title) {
			t.Fatal("invalid target accepted/leaked")
		}
	}
	_, writeToken, err := f.auth.CreateKey(access.KeyInput{Name: "write-only-relations", Permissions: []string{"write"}, Projects: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	writer := connect(t, ts.URL, writeToken)
	r = call(t, writer, "save_memory", map[string]any{"project": "demo", "kind": "fact", "title": "no read", "content": "fixture", "status": "unverified", "source": "", "related_to": []string{old.ID}})
	if !r.IsError {
		t.Fatal("write-only set relations")
	}
	r = call(t, writer, "get_memory_relations", map[string]any{"id": old.ID})
	if !r.IsError {
		t.Fatal("write-only read relations")
	}
	// A legacy write-only revision preserves metadata without returning its IDs.
	r = call(t, writer, "revise_memory", map[string]any{"id": replacement.ID, "expected_revision": 1, "project": "demo", "kind": "decision", "title": replacement.Title, "content": replacement.Content, "status": "unverified", "source": ""})
	if r.IsError || strings.Contains(resultText(t, r), old.ID) {
		t.Fatal("write-only metadata response leaked", resultText(t, r))
	}
	kept, err := f.notes.Get(replacement.ID)
	if err != nil || len(kept.Supersedes) != 1 {
		t.Fatal("write-only revision lost metadata", err)
	}
	_, readToken, err := f.auth.CreateKey(access.KeyInput{Name: "read-only-relations", Permissions: []string{"read"}, Projects: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	reader := connect(t, ts.URL, readToken)
	r = call(t, reader, "save_memory", map[string]any{"project": "demo", "kind": "fact", "title": "no write", "content": "fixture", "status": "unverified", "source": "", "related_to": []string{old.ID}})
	if !r.IsError {
		t.Fatal("read-only set relations")
	}
	// Runtime discovery must expose optional new metadata, not require it of old clients.
	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name == "save_memory" {
			data, _ := json.Marshal(tool.InputSchema)
			var schema struct {
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			}
			json.Unmarshal(data, &schema)
			for _, field := range []string{"is_project_summary", "related_to", "supersedes"} {
				if _, ok := schema.Properties[field]; !ok {
					t.Fatal("missing schema", field)
				}
				for _, req := range schema.Required {
					if req == field {
						t.Fatal("metadata unexpectedly required")
					}
				}
			}
		}
	}
	// MCP revise: legacy omission preserves; explicit empty arrays clear.
	args := map[string]any{"id": overview.ID, "expected_revision": 1, "project": "demo", "kind": "knowledge", "title": overview.Title, "content": overview.Content, "status": "unverified", "source": ""}
	r = call(t, session, "revise_memory", args)
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	n, err := f.notes.Get(overview.ID)
	if err != nil || !n.IsProjectSummary || len(n.RelatedTo) != 1 {
		t.Fatal("MCP omission lost metadata", err)
	}
	args["expected_revision"], args["related_to"], args["is_project_summary"] = 2, []string{}, false
	r = call(t, session, "revise_memory", args)
	if r.IsError {
		t.Fatal(resultText(t, r))
	}
	n, err = f.notes.Get(overview.ID)
	if err != nil || n.IsProjectSummary || len(n.RelatedTo) != 0 {
		t.Fatal("MCP clear failed", err)
	}
	// Removing (archiving) an explicit replacement restores context eligibility.
	if _, err := f.notes.Archive(replacement.ID, 2, "admin"); err != nil {
		t.Fatal(err)
	}
	r = call(t, session, "get_context", map[string]any{"project": "demo", "query": "OLD-MARKER"})
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &ctx) != nil || len(ctx.Notes) != 1 || ctx.Notes[0].ID != old.ID {
		t.Fatal("archived replacement still suppresses", resultText(t, r))
	}
	if _, err := f.notes.Restore(replacement.ID, 3, "admin"); err != nil {
		t.Fatal(err)
	}
	r = call(t, session, "get_context", map[string]any{"project": "demo", "query": "OLD-MARKER"})
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &ctx) != nil || len(ctx.Notes) != 0 {
		t.Fatal("restored replacement failed to suppress", resultText(t, r))
	}
	// Bounded serialized note payload even when full notes or references are large.
	_ = makeNote(t, f, "demo", "fact", "HUGE-MARKER", func(in *memory.Input) { in.Content = strings.Repeat("x", 30000) })
	r = call(t, session, "get_context", map[string]any{"project": "demo"})
	if r.IsError || json.Unmarshal([]byte(resultText(t, r)), &ctx) != nil || !ctx.Truncated {
		t.Fatal("large context did not truncate")
	}
	bytes := 0
	for _, n := range ctx.Notes {
		data, _ := json.Marshal(n)
		bytes += len(data)
	}
	if bytes > 24000 {
		t.Fatal("context exceeded byte budget", bytes)
	}
}

func TestAdminConnectionRevision(t *testing.T) {
	f := setup(t)
	target := makeNote(t, f, "demo", "fact", "Target", nil)
	n := makeNote(t, f, "demo", "knowledge", "Overview", nil)
	path := "/api/notes/" + n.ID + "/connections"
	body := fmt.Sprintf(`{"expected_revision":1,"is_project_summary":true,"related_to":[%q],"supersedes":[]}`, target.ID)
	if w := request(f.app, "POST", path, body, nil, ""); w.Code != 401 {
		t.Fatal("unauthenticated metadata", w.Code)
	}
	cookie, csrf := login(t, f)
	if w := request(f.app, "POST", path, body, cookie, ""); w.Code != 403 {
		t.Fatal("missing CSRF accepted", w.Code)
	}
	if w := request(f.app, "POST", path, body, cookie, csrf); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request(f.app, "POST", path, body, cookie, csrf); w.Code != 409 {
		t.Fatal("stale metadata accepted", w.Code)
	}
	w := request(f.app, "GET", "/api/notes/"+target.ID, "", cookie, "")
	var detail struct {
		Note noteView `json:"note"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &detail) != nil || len(detail.Note.Connections.ReferencedBy) != 1 {
		t.Fatal("admin backlink missing", w.Body.String())
	}
	now, err := f.notes.Get(n.ID)
	if err != nil || now.Revision != 2 || now.Content != n.Content || !now.IsProjectSummary {
		t.Fatal("metadata mutated content", err)
	}
	bad := fmt.Sprintf(`{"expected_revision":2,"supersedes":[%q]}`, n.ID)
	if w := request(f.app, "POST", path, bad, cookie, csrf); w.Code != 400 {
		t.Fatal("self relation accepted", w.Code)
	}
	list := request(f.app, "GET", "/api/notes?project=demo", "", cookie, "")
	var notes []noteView
	if json.Unmarshal(list.Body.Bytes(), &notes) != nil || len(notes) != 2 || notes[0].ID != n.ID {
		t.Fatal("overview not first on dashboard")
	}
	_, err = f.notes.Archive(n.ID, 2, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if w := request(f.app, "POST", path, `{"expected_revision":3,"related_to":[]}`, cookie, csrf); w.Code != 409 {
		t.Fatal("archived edit accepted", w.Code)
	}
	skill := makeNote(t, f, "demo", "skill", "Skill metadata", func(in *memory.Input) { in.Status = "draft"; in.RelatedTo = []string{target.ID} })
	if _, err := f.notes.Publish(skill.ID, 1, "admin"); err != nil {
		t.Fatal(err)
	}
	w = request(f.app, "POST", "/api/notes/"+skill.ID+"/connections", `{"expected_revision":2,"related_to":[]}`, cookie, csrf)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	revised, err := f.notes.Get(skill.ID)
	if err != nil || revised.Status != "draft" || len(revised.RelatedTo) != 0 || revised.Content != skill.Content {
		t.Fatal("metadata bypassed skill review", err)
	}
}

func TestBoundedBacklinks(t *testing.T) {
	n := memory.Note{ID: "source", Project: "demo"}
	all := []memory.Note{n}
	for i := range 25 {
		all = append(all, memory.Note{ID: fmt.Sprintf("%032x", i+1), Project: "demo", RelatedTo: []string{n.ID}})
	}
	all = append(all, memory.Note{ID: "private", Project: "other", RelatedTo: []string{n.ID}})
	c := connectionsFor(n, all)
	if len(c.ReferencedBy) != 20 || !c.Truncated {
		t.Fatal("backlinks not bounded")
	}
	for _, ref := range c.ReferencedBy {
		if ref.ID == "private" {
			t.Fatal("cross-project backlink leaked")
		}
	}
}
