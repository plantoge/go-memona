package memory

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func flag(v bool) *bool { return &v }

func TestProjectSummaryLifecycle(t *testing.T) {
	s, _ := openTestStore(t)
	in := fixtureInput()
	in.Kind, in.IsProjectSummary = "knowledge", flag(true)
	first := mustCreate(t, s, in)
	if _, err := s.Create(in, "tester"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate summary: %v", err)
	}
	archived, err := s.Archive(first.ID, first.Revision, "tester")
	if err != nil {
		t.Fatal(err)
	}
	second := mustCreate(t, s, in)
	if _, err := s.Restore(first.ID, archived.Revision, "admin"); !errors.Is(err, ErrConflict) {
		t.Fatalf("restore duplicate summary: %v", err)
	}
	in.IsProjectSummary = flag(false)
	if _, err := s.Revise(second.ID, second.Revision, in, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Restore(first.ID, archived.Revision, "admin"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []Input{
		{Project: "global", Kind: "fact", Title: "wrong kind", Status: "unverified", IsProjectSummary: flag(true)},
		{Project: "other", Kind: "knowledge", Title: "oversize", Content: strings.Repeat("x", MaxSummaryBytes+1), Status: "unverified", IsProjectSummary: flag(true)},
	} {
		if _, err := s.Create(invalid, "tester"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid summary accepted: %v", err)
		}
	}
	// Omission must not bypass the summary size constraint on revision.
	in.IsProjectSummary, in.Content = nil, strings.Repeat("x", MaxSummaryBytes+1)
	if _, err := s.Revise(first.ID, archived.Revision+1, in, "tester"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("summary preservation bypass: %v", err)
	}
}

func TestConnectionRevisionPersistenceAndLegacy(t *testing.T) {
	s, root := openTestStore(t)
	in := fixtureInput()
	old := mustCreate(t, s, in)
	legacyBytes, err := os.ReadFile(filepath.Join(root, "notes", old.ID, "000001.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacyBytes), "related_to") || strings.Contains(string(legacyBytes), "is_project_summary") {
		t.Fatal("legacy format changed")
	}
	in.Kind, in.IsProjectSummary = "knowledge", flag(true)
	in.RelatedTo = []string{old.ID}
	n := mustCreate(t, s, in)
	in.IsProjectSummary, in.RelatedTo = nil, nil
	in.Content = "Updated without new metadata fields"
	n2, err := s.Revise(n.ID, 1, in, "legacy-client")
	if err != nil || !n2.IsProjectSummary || !reflect.DeepEqual(n2.RelatedTo, n.RelatedTo) {
		t.Fatalf("omission did not preserve: %#v %v", n2, err)
	}
	in.IsProjectSummary, in.RelatedTo, in.Supersedes = flag(false), []string{}, []string{old.ID}
	n3, err := s.Revise(n.ID, 2, in, "tester")
	if err != nil || n3.IsProjectSummary || len(n3.RelatedTo) != 0 || len(n3.Supersedes) != 1 {
		t.Fatalf("replace/clear: %#v %v", n3, err)
	}
	archived, err := s.Archive(n.ID, n3.Revision, "tester")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := s.Restore(n.ID, archived.Revision, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	h, err := reopened.History(n.ID)
	if err != nil || len(h) != 5 || !reflect.DeepEqual(h[0], n) || !reflect.DeepEqual(h[4], restored) {
		t.Fatalf("persistent relation history: %v %v", h, err)
	}
	got, err := reopened.Get(old.ID)
	if err != nil || !reflect.DeepEqual(got, old) {
		t.Fatal("legacy read changed", err)
	}
	again, err := os.ReadFile(filepath.Join(root, "notes", old.ID, "000001.md"))
	if err != nil || !reflect.DeepEqual(legacyBytes, again) {
		t.Fatal("legacy bytes rewritten", err)
	}
}

func TestConnectionValidationAndCycles(t *testing.T) {
	s, _ := openTestStore(t)
	in := fixtureInput()
	a, b, c := mustCreate(t, s, in), mustCreate(t, s, in), mustCreate(t, s, in)
	private := in
	private.Project = "other"
	other := mustCreate(t, s, private)
	for name, ids := range map[string][]string{
		"self": {a.ID}, "missing": {strings.Repeat("0", 32)}, "foreign": {other.ID}, "invalid": {"../bad"}, "duplicate": {b.ID, b.ID}, "too-many": make([]string, 21),
	} {
		t.Run(name, func(t *testing.T) {
			test := in
			test.RelatedTo = ids
			if _, err := s.Revise(a.ID, 1, test, "tester"); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid target accepted: %v", err)
			}
		})
	}
	overlap := in
	overlap.RelatedTo, overlap.Supersedes = []string{b.ID}, []string{b.ID}
	if _, err := s.Revise(a.ID, 1, overlap, "tester"); !errors.Is(err, ErrInvalid) {
		t.Fatal("overlap accepted", err)
	}
	// A replaces B, B replaces C. C must not replace A, even if A is archived.
	in.Supersedes = []string{b.ID}
	a2, err := s.Revise(a.ID, 1, in, "tester")
	if err != nil {
		t.Fatal(err)
	}
	in.Supersedes = []string{c.ID}
	if _, err := s.Revise(b.ID, 1, in, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(a.ID, a2.Revision, "tester"); err != nil {
		t.Fatal(err)
	}
	in.Supersedes = []string{a.ID}
	if _, err := s.Revise(c.ID, 1, in, "tester"); !errors.Is(err, ErrInvalid) {
		t.Fatal("cycle accepted", err)
	}
	latest, err := s.Get(c.ID)
	if err != nil || latest.Revision != 1 {
		t.Fatal("failed cycle wrote revision", err)
	}
	// Related-to cycles are harmless references, unlike supersedes cycles.
	in.Supersedes, in.RelatedTo = []string{}, []string{b.ID}
	if _, err := s.Revise(c.ID, 1, in, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Revise(c.ID, 1, in, "tester"); !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision accepted", err)
	}
}

func TestConcurrentSummaryAndSupersedes(t *testing.T) {
	s, _ := openTestStore(t)
	in := fixtureInput()
	in.Kind = "knowledge"
	in.IsProjectSummary = flag(true)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := s.Create(in, "tester"); errs <- err })
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("summary uniqueness raced", success)
	}
	base := fixtureInput()
	a, b := mustCreate(t, s, base), mustCreate(t, s, base)
	errs = make(chan error, 2)
	wg.Go(func() {
		x := base
		x.Supersedes = []string{b.ID}
		_, err := s.Revise(a.ID, 1, x, "tester")
		errs <- err
	})
	wg.Go(func() {
		x := base
		x.Supersedes = []string{a.ID}
		_, err := s.Revise(b.ID, 1, x, "tester")
		errs <- err
	})
	wg.Wait()
	close(errs)
	success = 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal("cycle validation raced", success)
	}
}
