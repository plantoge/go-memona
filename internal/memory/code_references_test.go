package memory

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCodeReferencesPersistPreserveAndClear(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ref := CodeReference{Repository: "example", Path: "internal/handler.go", Commit: "aabbccd", Symbol: "Serve", Line: 12}
	in := Input{Project: "demo", Kind: "decision", Title: "Use handler", Content: "Because synthetic.", Status: "unverified", Source: "synthetic evidence", CodeReferences: []CodeReference{ref}}
	n, err := s.Create(in, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(n.CodeReferences, []CodeReference{ref}) {
		t.Fatal("citation missing")
	}
	in.CodeReferences = nil
	in.Content = "After review"
	n, err = s.Revise(n.ID, n.Revision, in, "tester")
	if err != nil || !reflect.DeepEqual(n.CodeReferences, []CodeReference{ref}) {
		t.Fatalf("revision lost citation: %v", err)
	}
	history, err := s.History(n.ID)
	if err != nil || len(history) != 2 || !reflect.DeepEqual(history[0].CodeReferences, []CodeReference{ref}) {
		t.Fatalf("history invalid: %v", err)
	}
	in.CodeReferences = []CodeReference{}
	n, err = s.Revise(n.ID, n.Revision, in, "tester")
	if err != nil || len(n.CodeReferences) != 0 {
		t.Fatalf("clear failed: %v", err)
	}
}

func TestCodeReferencesRejectTraversalAndMalformed(t *testing.T) {
	cases := []CodeReference{
		{Repository: "repo", Path: "../secret"},
		{Repository: "repo", Path: "/absolute/file.go"},
		{Repository: "repo", Path: "src\\file.go"},
		{Repository: "repo", Path: "a//b"},
		{Repository: "repo", Path: "a/./b"},
		{Repository: "repo", Path: "file.go", Commit: "branch-name"},
		{Repository: "repo", Path: "file.go", Line: -1},
		{Repository: "INVALID", Path: "file.go"},
	}
	for _, ref := range cases {
		if err := validateCodeReferences([]CodeReference{ref}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %+v: %v", ref, err)
		}
	}
}
