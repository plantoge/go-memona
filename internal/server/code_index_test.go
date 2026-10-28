package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"memona/internal/access"
	"memona/internal/codeindex"
	"memona/internal/memory"
)

func TestCodeIndexProjectScopeAndNoPathArgument(t *testing.T) {
	f := setup(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "feature.go"), []byte("package fixture\nfunc SecretFunction() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := codeindex.Build(context.Background(), []codeindex.Repository{{Name: "fixture", Project: "private-project", Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	f.app.code = idx
	f.app.cfg.CodeRepositories = []codeindex.Repository{{Name: "fixture", Project: "private-project", Path: root}}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	connect := func(project string) *mcp.ClientSession {
		t.Helper()
		key, _, err := f.auth.CreateKey(access.KeyInput{Name: "code-test", Permissions: []string{"read"}, Projects: []string{project}})
		if err != nil {
			t.Fatal(err)
		}
		left, right := mcp.NewInMemoryTransports()
		server, err := f.app.mcpServer(key).Connect(t.Context(), right, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { server.Close() })
		session, err := client.Connect(t.Context(), left, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { session.Close() })
		return session
	}
	allowed := connect("private-project")
	result := call(t, allowed, "search_code_index", map[string]any{"project": "private-project", "query": "SecretFunction"})
	if result.IsError || !strings.Contains(resultText(t, result), "fixture:feature.go#L2-L2") || strings.Contains(resultText(t, result), root) {
		t.Fatalf("bad code result: %s", resultText(t, result))
	}
	denied := connect("public-project")
	result = call(t, denied, "search_code_index", map[string]any{"project": "private-project", "query": "SecretFunction"})
	if !result.IsError || strings.Contains(resultText(t, result), "SecretFunction") {
		t.Fatalf("cross-project code access: %s", resultText(t, result))
	}
	result = call(t, denied, "search_code_index", map[string]any{"project": "public-project", "query": "SecretFunction"})
	if result.IsError || strings.Contains(resultText(t, result), "SecretFunction") || strings.Contains(resultText(t, result), "indexed_at") {
		t.Fatalf("cross-project code results: %s", resultText(t, result))
	}
}

func TestConfiguredRepositoryIndexedAtStartup(t *testing.T) {
	f := setup(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "startup.go"), []byte("package fixture\nfunc StartupSymbol() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app, err := New(Config{Listen: "127.0.0.1:8765", PublicURL: "http://localhost:8765", CodeRepositories: []codeindex.Repository{{Name: "fixture", Project: "demo", Path: root}}}, f.auth, f.notes)
	if err != nil || app.code == nil || len(app.code.Symbols) != 1 || app.code.Symbols[0].Name != "StartupSymbol" {
		t.Fatalf("configured index not loaded: %v", err)
	}
}

func TestCodeReferenceMCPWriteOnlyReceiptDoesNotLeak(t *testing.T) {
	f := setup(t)
	ref := memory.CodeReference{Repository: "fixture", Path: "main.go", Symbol: "Example", Line: 2}
	writer, _, err := f.auth.CreateKey(access.KeyInput{Name: "writer", Permissions: []string{"write"}, Projects: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}
	left, right := mcp.NewInMemoryTransports()
	server, err := f.app.mcpServer(writer).Connect(t.Context(), right, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), left, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	args := map[string]any{"project": "demo", "kind": "decision", "title": "Fixture", "content": "Synthetic", "status": "unverified", "source": "synthetic", "code_references": []memory.CodeReference{ref}}
	created := call(t, session, "save_memory", args)
	if created.IsError {
		t.Fatalf("create failed: %s", resultText(t, created))
	}
	var n memory.Note
	if err := json.Unmarshal([]byte(resultText(t, created)), &n); err != nil || len(n.CodeReferences) != 1 {
		t.Fatalf("create citation missing: %v", err)
	}
	delete(args, "code_references")
	args["id"], args["expected_revision"] = n.ID, 1
	revised := call(t, session, "revise_memory", args)
	if revised.IsError || strings.Contains(resultText(t, revised), "code_references") {
		t.Fatalf("write-only response leaked citation: %s", resultText(t, revised))
	}
	stored, err := f.notes.Get(n.ID)
	if err != nil || len(stored.CodeReferences) != 1 || stored.CodeReferences[0] != ref {
		t.Fatalf("citation not preserved on disk: %v", err)
	}
}
