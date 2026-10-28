package codeindex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildExplicitGoRepository(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package demo\nfunc A(){ B() }\nfunc B(){}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "vendor"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vendor", "ignored.go"), []byte("package demo\nfunc Hidden(){}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "main.go"), filepath.Join(root, "linked.go")); err != nil {
		t.Fatal(err)
	}
	idx, err := Build(context.Background(), []Repository{{Name: "demo", Project: "project-a", Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Symbols) != 2 || idx.Symbols[0].Name != "A" || idx.Symbols[0].Reference != "demo:main.go#L2-L2" || len(idx.Symbols[0].Calls) != 1 || idx.Symbols[0].Calls[0] != "B" {
		t.Fatalf("unexpected symbols: %+v", idx.Symbols)
	}
}

func TestBuildRejectsUnsafeRoots(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "repo")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), []Repository{{Name: "repo", Project: "project-a", Path: link}}); err == nil {
		t.Fatal("symlink root accepted")
	}
	if err := Validate([]Repository{{Name: "repo", Project: "project-a", Path: "."}}); err == nil {
		t.Fatal("relative root accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "large.go"), []byte(strings.Repeat("a", maxFileBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), []Repository{{Name: "repo", Project: "project-a", Path: root}}); err == nil {
		t.Fatal("oversized file accepted")
	}
}
