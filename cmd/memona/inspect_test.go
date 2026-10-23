package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"memona/internal/memory"
)

func TestInspectCommandOfflineAndFindings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	s, err := memory.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := inspectCommand(t.Context(), []string{"-data-dir", root}, &out); err == nil || !strings.Contains(err.Error(), "writer active") || out.Len() != 0 {
		t.Fatal(err, out.String())
	}
	s.Close()
	err = inspectCommand(t.Context(), []string{"-data-dir", root}, &out)
	if err != nil && !errors.Is(err, errInspectionFindings) {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"complete":true`) || strings.Contains(out.String(), root) {
		t.Fatal(out.String())
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "PRIVATE_UNEXPECTED_NAME"), []byte("PRIVATE_BODY"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := inspectCommand(t.Context(), []string{"-data-dir", root}, &out); !errors.Is(err, errInspectionFindings) || strings.Contains(out.String(), "PRIVATE") {
		t.Fatal(err, out.String())
	}
	for _, args := range [][]string{{}, {"-data-dir", filepath.Join(root, "missing")}, {"-data-dir", root, "unexpected"}} {
		out.Reset()
		if err := inspectCommand(t.Context(), args, &out); err == nil || out.Len() != 0 {
			t.Fatal("invalid inspect invocation", err)
		}
	}
}
