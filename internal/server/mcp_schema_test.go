package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"memona/internal/access"
	"memona/internal/memory"
)

// Validate the actual structured wire payload against tools/list, not merely
// against Go structs (whose JSON decoder ignores unknown properties by default).
func TestMCPDiscoveredOutputSchemas(t *testing.T) {
	f := setup(t)
	flag := true
	target := makeNote(t, f, "demo", "decision", "Original decision", nil)
	overview := makeNote(t, f, "demo", "knowledge", "Project summary", func(in *memory.Input) {
		in.IsProjectSummary = &flag
		in.RelatedTo = []string{target.ID}
	})
	replacement := makeNote(t, f, "demo", "decision", "Replacement", func(in *memory.Input) {
		in.Supersedes = []string{target.ID}
	})
	skill := makeNote(t, f, "demo", "skill", "Reviewed skill", func(in *memory.Input) {
		in.Status = "draft"
		in.SkillMetadata = &memory.SkillMetadata{Trigger: "Review synthetic fixtures", Version: "1.0", Compatibility: []string{"OpenCode"}, TestEvidence: "synthetic schema test"}
	})
	if _, err := f.notes.Publish(skill.ID, 1, "admin"); err != nil {
		t.Fatal(err)
	}
	_, token, err := f.auth.CreateKey(access.KeyInput{Name: "schema-fixture", Permissions: []string{"read", "write", "archive"}, Projects: []string{"demo"}})
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
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	schemas := map[string]*jsonschema.Resolved{}
	for _, tool := range listed.Tools {
		switch tool.Name {
		case "save_memory", "save_handoff", "revise_memory", "archive_memory":
			inputJSON, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			var input struct {
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			}
			if err := json.Unmarshal(inputJSON, &input); err != nil || input.Properties["operation_id"] == nil {
				t.Fatalf("%s missing operation_id in runtime schema: %v", tool.Name, err)
			}
			for _, field := range input.Required {
				if field == "operation_id" {
					t.Fatalf("%s requires optional operation_id", tool.Name)
				}
			}
		}
		encoded, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema jsonschema.Schema
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatalf("%s output schema: %v", tool.Name, err)
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			t.Fatalf("%s resolve: %v", tool.Name, err)
		}
		schemas[tool.Name] = resolved
	}
	if len(schemas) != 11 {
		t.Fatalf("expected eleven discovered output schemas, got %d", len(schemas))
	}
	validated := map[string]bool{}
	// This schema suite deliberately makes more calls than the production
	// per-key burst. Pace the real transport instead of disabling the limiter.
	var lastCall time.Time
	check := func(name string, args any) *mcp.CallToolResult {
		t.Helper()
		if remaining := 110*time.Millisecond - time.Since(lastCall); remaining > 0 {
			timer := time.NewTimer(remaining)
			select {
			case <-timer.C:
			case <-t.Context().Done():
				timer.Stop()
				t.Fatal(t.Context().Err())
			}
		}
		lastCall = time.Now()
		result := call(t, session, name, args)
		if result.IsError {
			t.Fatalf("%s: %s", name, resultText(t, result))
		}
		if result.StructuredContent == nil {
			t.Fatalf("%s missing structured output", name)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var instance map[string]any
		if err := json.Unmarshal(encoded, &instance); err != nil {
			t.Fatal(err)
		}
		if err := schemas[name].Validate(instance); err != nil {
			t.Fatalf("%s structured output violates discovered schema: %v", name, err)
		}
		instance["unexpected_schema_test_property"] = true
		if err := schemas[name].Validate(instance); err == nil {
			t.Fatalf("%s schema is not strict", name)
		}
		validated[name] = true
		return result
	}
	check("get_memory_guide", map[string]any{})
	check("get_context", map[string]any{"project": "demo"})
	check("get_context", map[string]any{"project": "demo", "kind": "not-found"})
	check("search_memory", map[string]any{"project": "demo"})
	check("search_memory", map[string]any{"project": "demo", "query": "not-found"})
	check("find_duplicate_candidates", map[string]any{"project": "demo", "title": target.Title, "content": target.Content})
	check("find_duplicate_candidates", map[string]any{"project": "demo", "title": "unrelated query", "content": "unrelated unique"})
	for _, n := range []memory.Note{target, overview, replacement, skill} {
		check("read_memory", map[string]any{"id": n.ID})
		check("get_memory_relations", map[string]any{"id": n.ID})
	}
	check("get_skill", map[string]any{"id": skill.ID})
	skillArgs := map[string]any{"project": "demo", "kind": "skill", "title": "Metadata roundtrip", "content": "Synthetic procedure", "status": "draft", "source": "synthetic", "skill_metadata": map[string]any{"trigger": "Review synthetic", "version": "1.0", "compatibility": []string{"OpenCode"}, "test_evidence": "schema test"}, "operation_id": "schema-skill-metadata-001"}
	skillCreated := check("save_memory", skillArgs)
	var skillNote memory.Note
	if err := json.Unmarshal([]byte(resultText(t, skillCreated)), &skillNote); err != nil || skillNote.SkillMetadata == nil || skillNote.SkillMetadata.Version != "1.0" {
		t.Fatal("skill metadata roundtrip failed", err)
	}
	check("save_memory", skillArgs)
	skillArgs["id"], skillArgs["expected_revision"] = skillNote.ID, 1
	skillArgs["skill_metadata"] = map[string]any{}
	cleared := check("revise_memory", skillArgs)
	// Decode into a fresh value: omitted metadata must not retain old Go pointer.
	var clearedNote memory.Note
	if err := json.Unmarshal([]byte(resultText(t, cleared)), &clearedNote); err != nil || clearedNote.SkillMetadata != nil {
		t.Fatal("clear failed", err)
	}
	args := map[string]any{"project": "demo", "kind": "fact", "title": "Schema round trip", "content": "Synthetic fixture", "status": "unverified", "source": "", "related_to": []string{target.ID}, "is_testing": true, "operation_id": "schema-operation-001"}
	created := check("save_memory", args)
	check("save_memory", args)
	var n memory.Note
	if err := json.Unmarshal([]byte(resultText(t, created)), &n); err != nil {
		t.Fatal(err)
	}
	args["id"], args["expected_revision"] = n.ID, 1
	check("revise_memory", args)
	check("revise_memory", args)
	for i := 0; i < 2; i++ {
		check("archive_memory", map[string]any{"id": n.ID, "expected_revision": 2, "operation_id": "schema-operation-001"})
		check("save_handoff", map[string]any{"project": "demo", "kind": "handoff", "title": "Handoff", "content": "Synthetic checkpoint", "status": "unverified", "source": "", "operation_id": "schema-operation-001"})
	}
	for name := range schemas {
		if !validated[name] {
			t.Errorf("%s output schema was not exercised", name)
		}
	}
}
