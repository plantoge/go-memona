package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryPageOrderCursorAndValidation(t *testing.T) {
	s, root := openTestStore(t)
	n := mustCreate(t, s, fixtureInput())
	for i := 0; i < 6; i++ {
		var err error
		n, err = s.Revise(n.ID, n.Revision, fixtureInput(), "tester")
		if err != nil {
			t.Fatal(err)
		}
	}
	before := 0
	expected := 7
	for expected > 0 {
		p, err := s.HistoryPageContext(t.Context(), n.ID, before, 3)
		if err != nil {
			t.Fatal(err)
		}
		if p.Latest != 7 || len(p.Items) > 3 {
			t.Fatal(p)
		}
		for _, n := range p.Items {
			if n.Revision != expected {
				t.Fatal(n.Revision, expected)
			}
			expected--
		}
		before = p.Before
		if expected > 0 && before == 0 {
			t.Fatal("missing cursor")
		}
	}
	for _, before := range []int{1} {
		p, err := s.HistoryPageContext(t.Context(), n.ID, before, 3)
		if err != nil || len(p.Items) != 0 {
			t.Fatal(p, err)
		}
	}
	if _, err := s.HistoryPageContext(t.Context(), n.ID, -1, 3); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.HistoryPageContext(t.Context(), n.ID, 0, 51); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.HistoryPageContext(ctx, n.ID, 0, 3); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", n.ID, "000001.md"), []byte("synthetic corruption"), 0600); err != nil {
		t.Fatal(err)
	}
	if p, err := s.HistoryPageContext(t.Context(), n.ID, 0, 1); err == nil || len(p.Items) != 0 {
		t.Fatal("hidden corruption accepted", err)
	}
}
