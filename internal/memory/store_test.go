package memory

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func fixtureInput() Input {
	return Input{Project: "global", Kind: "fact", Title: "A durable note", Content: "Body\nwith **Markdown**.\n---\n", Status: "unverified"}
}

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "memory")
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s, root
}

func mustCreate(t *testing.T, s *Store, input Input) Note {
	t.Helper()
	n, err := s.Create(input, "tester")
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPersistenceHistoryAndArchive(t *testing.T) {
	s, root := openTestStore(t)
	list, err := s.List()
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty List: %v %v", list, err)
	}
	input := fixtureInput()
	first := mustCreate(t, s, input)
	if first.Revision != 1 || first.CreatedAt == "" || first.CreatedAt != first.UpdatedAt {
		t.Fatalf("bad initial metadata: %#v", first)
	}
	path := filepath.Join(root, "notes", first.ID, "000001.md")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(original), "---\n{\"id\":") || !strings.HasSuffix(string(original), input.Content+"\n") {
		t.Fatal("not documented Markdown format")
	}
	for _, p := range []string{root, filepath.Join(root, "notes"), filepath.Dir(path), path, filepath.Join(root, ".writer.lock")} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Fatalf("non-private permissions on %s", p)
		}
	}
	input.Content = "Revised body"
	input.Status, input.Source = "verified", "https://example.test/reference"
	second, err := s.Revise(first.ID, 1, input, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision != 2 || second.CreatedAt != first.CreatedAt || second.Author != "reviewer" {
		t.Fatalf("bad revision: %#v", second)
	}
	if _, err := s.Revise(first.ID, 1, input, "tester"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revise: %v", err)
	}
	if _, err := s.Archive(first.ID, 1, "tester"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale archive: %v", err)
	}
	third, err := s.Archive(first.ID, 2, "archiver")
	if err != nil || !third.Archived || third.Revision != 3 {
		t.Fatalf("archive: %#v %v", third, err)
	}
	if _, err := s.Revise(first.ID, 3, input, "tester"); !errors.Is(err, ErrConflict) {
		t.Fatalf("archived revise: %v", err)
	}
	if _, err := s.Archive(first.ID, 3, "tester"); !errors.Is(err, ErrConflict) {
		t.Fatalf("archived archive: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	history, err := reopened.History(first.ID)
	if err != nil || !reflect.DeepEqual(history, []Note{first, second, third}) {
		t.Fatalf("history: %#v %v", history, err)
	}
	latest, err := reopened.Get(first.ID)
	if err != nil || !reflect.DeepEqual(latest, third) {
		t.Fatalf("Get: %#v %v", latest, err)
	}
	list, err = reopened.List()
	if err != nil || !reflect.DeepEqual(list, []Note{third}) {
		t.Fatalf("List: %#v %v", list, err)
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || string(unchanged) != string(original) {
		t.Fatal("original history overwritten")
	}
	history[0].Title = "caller modification"
	again, err := reopened.History(first.ID)
	if err != nil || !reflect.DeepEqual(again[0], first) {
		t.Fatal("history not isolated")
	}
}

func TestConcurrentReviseOneWinner(t *testing.T) {
	s, _ := openTestStore(t)
	input := fixtureInput()
	n := mustCreate(t, s, input)
	const count = 20
	results := make(chan error, count)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, err := s.Revise(n.ID, 1, input, "writer"); results <- err }()
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("got %d winners", winners)
	}
	h, err := s.History(n.ID)
	if err != nil || len(h) != 2 {
		t.Fatalf("history: %v %v", h, err)
	}
}

func TestValidation(t *testing.T) {
	s, _ := openTestStore(t)
	cases := []struct {
		name   string
		mutate func(*Input)
	}{
		{"empty project", func(i *Input) { i.Project = "" }},
		{"project traversal", func(i *Input) { i.Project = "../secret" }},
		{"project uppercase", func(i *Input) { i.Project = "Project" }},
		{"project leading hyphen", func(i *Input) { i.Project = "-project" }},
		{"project too long", func(i *Input) { i.Project = strings.Repeat("p", 65) }},
		{"unknown kind", func(i *Input) { i.Kind = "script" }},
		{"unknown status", func(i *Input) { i.Status = "published" }},
		{"empty title", func(i *Input) { i.Title = "  " }},
		{"title too long", func(i *Input) { i.Title = strings.Repeat("t", 201) }},
		{"title control", func(i *Input) { i.Title = "bad\ntitle" }},
		{"content too long", func(i *Input) { i.Content = strings.Repeat("x", 64*1024+1) }},
		{"content NUL", func(i *Input) { i.Content = "bad\x00content" }},
		{"invalid UTF8", func(i *Input) { i.Content = string([]byte{0xff}) }},
		{"source too long", func(i *Input) { i.Source = strings.Repeat("s", 2001) }},
		{"source control", func(i *Input) { i.Source = "bad\nsource" }},
		{"verified without source", func(i *Input) { i.Status = "verified" }},
		{"verified blank source", func(i *Input) { i.Status, i.Source = "verified", " " }},
		{"active fact", func(i *Input) { i.Status = "active" }},
		{"active skill", func(i *Input) { i.Kind, i.Status = "skill", "active" }},
		{"unverified skill", func(i *Input) { i.Kind = "skill" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := fixtureInput()
			tc.mutate(&input)
			if _, err := s.Create(input, "tester"); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
	for _, author := range []string{"", " ", "bad\nauthor", strings.Repeat("a", 201)} {
		if _, err := s.Create(fixtureInput(), author); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid author: %v", err)
		}
	}
	boundary := fixtureInput()
	boundary.Project, boundary.Title, boundary.Content, boundary.Source = strings.Repeat("p", 64), strings.Repeat("t", 200), strings.Repeat("x", 64*1024), strings.Repeat("s", 2000)
	mustCreate(t, s, boundary)
	for _, kind := range []string{"fact", "decision", "knowledge", "handoff", "skill"} {
		input := fixtureInput()
		input.Kind, input.Status = kind, "draft"
		mustCreate(t, s, input)
	}
	n := mustCreate(t, s, fixtureInput())
	bad := fixtureInput()
	bad.Title = ""
	if _, err := s.Revise(n.ID, 1, bad, "tester"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid revise: %v", err)
	}
	if _, err := s.Archive(n.ID, 1, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid archive author: %v", err)
	}
	if _, err := s.Revise(n.ID, 0, fixtureInput(), "tester"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid expected revision: %v", err)
	}
}

func TestInvalidPathsAndNotFound(t *testing.T) {
	s, root := openTestStore(t)
	for _, id := range []string{"", "..", "../secret", "/etc/passwd", strings.Repeat("a", 32) + "/..", "000001.md", strings.Repeat("A", 32), "\x00"} {
		if _, err := s.Get(id); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Get(%q): %v", id, err)
		}
		if _, err := s.History(id); !errors.Is(err, ErrInvalid) {
			t.Fatalf("History(%q): %v", id, err)
		}
		if _, err := s.Archive(id, 1, "tester"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Archive(%q): %v", id, err)
		}
		if _, err := s.Revise(id, 1, fixtureInput(), "tester"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Revise(%q): %v", id, err)
		}
		if _, err := s.Publish(id, 1, "tester"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Publish(%q): %v", id, err)
		}
	}
	missing := strings.Repeat("0", 32)
	if _, err := s.Get(missing); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.History(missing); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", root + "/../other", "bad\x00path"} {
		if opened, err := Open(invalid); !errors.Is(err, ErrInvalid) {
			if opened != nil {
				opened.Close()
			}
			t.Fatalf("Open invalid path: %v", err)
		}
	}
}

func TestWriterLeaseAndClose(t *testing.T) {
	s, root := openTestStore(t)
	if duplicate, err := Open(root); !errors.Is(err, ErrConflict) {
		if duplicate != nil {
			duplicate.Close()
		}
		t.Fatalf("duplicate writer: %v", err)
	}
	// A failed duplicate Open must not release the first writer's lease.
	if duplicate, err := Open(root); !errors.Is(err, ErrConflict) {
		if duplicate != nil {
			duplicate.Close()
		}
		t.Fatalf("lost original lease: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err == nil {
		t.Fatal("closed List succeeded")
	}
	if _, err := s.Create(fixtureInput(), "tester"); err == nil {
		t.Fatal("closed Create succeeded")
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
}

func TestSkillsPromotion(t *testing.T) {
	s, _ := openTestStore(t)
	input := fixtureInput()
	input.Kind, input.Status = "skill", "draft"
	first := mustCreate(t, s, input)
	active, err := s.Publish(first.ID, 1, "reviewer")
	if err != nil || active.Status != "active" || active.Revision != 2 {
		t.Fatalf("publish: %#v %v", active, err)
	}
	if _, err := s.Publish(first.ID, 1, "reviewer"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale publish: %v", err)
	}
	if _, err := s.Publish(first.ID, 2, "reviewer"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate publish: %v", err)
	}
	input.Status, input.Content = "active", "Changed, requires review"
	draft, err := s.Revise(first.ID, 2, input, "editor")
	if err != nil || draft.Status != "draft" || draft.Revision != 3 {
		t.Fatalf("revise active skill: %#v %v", draft, err)
	}
	if _, err := s.Revise(first.ID, 3, input, "editor"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Revise bypassed publish: %v", err)
	}
	active, err = s.Publish(first.ID, 3, "reviewer")
	if err != nil || active.Status != "active" {
		t.Fatalf("republish: %#v %v", active, err)
	}
	if _, err := s.Archive(first.ID, 4, "archiver"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(first.ID, 5, "reviewer"); !errors.Is(err, ErrConflict) {
		t.Fatalf("publish archived: %v", err)
	}
	h, err := s.History(first.ID)
	if err != nil || len(h) != 5 {
		t.Fatalf("skill history: %v %v", h, err)
	}
	fact := mustCreate(t, s, fixtureInput())
	if _, err := s.Publish(fact.ID, 1, "reviewer"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("publish fact: %v", err)
	}
}

func TestSymlinkIsolation(t *testing.T) {
	t.Run("root", func(t *testing.T) {
		base, outside := t.TempDir(), t.TempDir()
		link := filepath.Join(base, "linked")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		if s, err := Open(filepath.Join(link, "child")); err == nil {
			s.Close()
			t.Fatal("followed root symlink")
		}
		entries, err := os.ReadDir(outside)
		if err != nil || len(entries) != 0 {
			t.Fatal("modified external directory")
		}
	})
	for _, target := range []string{"notes", ".writer.lock", ".staging"} {
		t.Run(target, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			if err := os.Symlink(outside, filepath.Join(root, target)); err != nil {
				t.Fatal(err)
			}
			if s, err := Open(root); err == nil {
				s.Close()
				t.Fatal("followed storage symlink")
			}
		})
	}
	t.Run("note directory", func(t *testing.T) {
		s, root := openTestStore(t)
		outside := t.TempDir()
		id := strings.Repeat("a", 32)
		if err := os.Symlink(outside, filepath.Join(root, "notes", id)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Get(id); err == nil {
			t.Fatal("followed note symlink")
		}
		if _, err := s.List(); err == nil {
			t.Fatal("silently skipped note symlink")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		if reopened, err := Open(root); err == nil {
			reopened.Close()
			t.Fatal("opened corrupt store")
		}
	})
	t.Run("revision", func(t *testing.T) {
		s, root := openTestStore(t)
		n := mustCreate(t, s, fixtureInput())
		outside := filepath.Join(t.TempDir(), "external.md")
		if err := os.WriteFile(outside, []byte("PRIVATE EXTERNAL CONTENT"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "notes", n.ID, "000002.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := s.History(n.ID); err == nil || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatalf("unsafe symlink result: %v", err)
		}
		if _, err := s.Revise(n.ID, 1, fixtureInput(), "tester"); err == nil {
			t.Fatal("wrote through revision symlink")
		}
		data, err := os.ReadFile(outside)
		if err != nil || string(data) != "PRIVATE EXTERNAL CONTENT" {
			t.Fatal("external file changed")
		}
	})
}

func TestProjectAndKindImmutable(t *testing.T) {
	s, _ := openTestStore(t)
	input := fixtureInput()
	n := mustCreate(t, s, input)
	for _, field := range []string{"project", "kind"} {
		changed := input
		if field == "project" {
			changed.Project = "another-project"
		} else {
			changed.Kind = "knowledge"
		}
		if _, err := s.Revise(n.ID, 1, changed, "editor"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("changed %s: %v", field, err)
		}
	}
	latest, err := s.Get(n.ID)
	if err != nil || !reflect.DeepEqual(latest, n) {
		t.Fatalf("invalid revisions changed note: %v %v", latest, err)
	}
}

func TestRestore(t *testing.T) {
	for _, skill := range []bool{false, true} {
		s, root := openTestStore(t)
		input := fixtureInput()
		if skill {
			input.Kind, input.Status = "skill", "draft"
		}
		first := mustCreate(t, s, input)
		latest := first
		var err error
		if skill {
			latest, err = s.Publish(first.ID, 1, "reviewer")
			if err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.Restore(first.ID, latest.Revision, "admin"); !errors.Is(err, ErrConflict) {
			t.Fatalf("restored live note: %v", err)
		}
		archived, err := s.Archive(first.ID, latest.Revision, "admin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Restore(first.ID, latest.Revision, "admin"); !errors.Is(err, ErrConflict) {
			t.Fatalf("stale restore: %v", err)
		}
		if _, err := s.Revise(first.ID, archived.Revision, input, "editor"); !errors.Is(err, ErrConflict) {
			t.Fatalf("revised archived note: %v", err)
		}
		restored, err := s.Restore(first.ID, archived.Revision, "admin")
		if err != nil || restored.Archived || restored.Revision != archived.Revision+1 || !sameInput(noteInput(restored), noteInput(archived)) || restored.CreatedAt != first.CreatedAt {
			t.Fatalf("restore: %#v %v", restored, err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(root)
		if err != nil {
			t.Fatal(err)
		}
		h, err := reopened.History(first.ID)
		if err != nil || len(h) != restored.Revision || !reflect.DeepEqual(h[0], first) || !reflect.DeepEqual(h[len(h)-2], archived) || !reflect.DeepEqual(h[len(h)-1], restored) {
			t.Fatalf("restore history: %v %v", h, err)
		}
		if _, err := reopened.Revise(first.ID, restored.Revision, input, "editor"); err != nil {
			t.Fatalf("cannot revise restored note: %v", err)
		}
		reopened.Close()
	}
}

func TestStagingAndExclusiveWrites(t *testing.T) {
	s, root := openTestStore(t)
	n := mustCreate(t, s, fixtureInput())
	staging := filepath.Join(root, ".staging")
	// Simulate interrupted revision and interrupted first-note staging. Neither
	// becomes visible and neither is deleted, including across a reopen.
	partial := filepath.Join(staging, "revision-interrupted")
	if err := os.WriteFile(partial, []byte("---\n{partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(staging, "note-interrupted"), 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := openDirectory(s.notesFD, n.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	modified := n
	modified.Content = "must not overwrite"
	if err := writeRevision(s.stagingFD, dir, modified); !errors.Is(err, ErrConflict) {
		t.Fatalf("overwrote revision: %v", err)
	}
	unix.Close(dir)
	// Entire staged note directories likewise cannot replace existing notes.
	if err := moveExclusive(s.stagingFD, "note-interrupted", s.notesFD, n.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("overwrote note directory: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	list, err := reopened.List()
	if err != nil || !reflect.DeepEqual(list, []Note{n}) {
		t.Fatalf("staging exposed: %v %v", list, err)
	}
	if data, err := os.ReadFile(partial); err != nil || string(data) != "---\n{partial" {
		t.Fatal("staging cleaned up")
	}
	if _, err := reopened.Revise(n.ID, 1, fixtureInput(), "editor"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "notes", n.ID))
	if err != nil || len(entries) != 2 {
		t.Fatalf("unexpected note files: %v %v", entries, err)
	}
}

func TestInvalidRestoreHistory(t *testing.T) {
	s, root := openTestStore(t)
	n := mustCreate(t, s, fixtureInput())
	archived, err := s.Archive(n.ID, 1, "admin")
	if err != nil {
		t.Fatal(err)
	}
	bad := archived
	bad.Revision++
	bad.Archived = false
	bad.Content = "changed while restoring"
	if err := os.WriteFile(filepath.Join(root, "notes", n.ID, "000003.md"), encode(bad), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.History(n.ID); err == nil {
		t.Fatal("accepted mutated restore")
	}
}

func TestCorruptRecordsFailClosed(t *testing.T) {
	for _, corruption := range []string{"partial", "metadata", "body", "gap", "unexpected", "empty note", "hardlink", "directory revision"} {
		t.Run(corruption, func(t *testing.T) {
			s, root := openTestStore(t)
			n := mustCreate(t, s, fixtureInput())
			dir := filepath.Join(root, "notes", n.ID)
			path := filepath.Join(dir, "000002.md")
			switch corruption {
			case "partial":
				if err := os.WriteFile(path, []byte("---\n{\"content\":\"PRIVATE"), 0600); err != nil {
					t.Fatal(err)
				}
			case "metadata":
				bad := n
				bad.Revision = 2
				bad.Project = "../secret"
				if err := os.WriteFile(path, encode(bad), 0600); err != nil {
					t.Fatal(err)
				}
			case "body":
				bad := n
				bad.Revision = 2
				if err := os.WriteFile(path, append(encode(bad), []byte("PRIVATE")...), 0600); err != nil {
					t.Fatal(err)
				}
			case "gap":
				bad := n
				bad.Revision = 3
				if err := os.WriteFile(filepath.Join(dir, "000003.md"), encode(bad), 0600); err != nil {
					t.Fatal(err)
				}
			case "unexpected":
				if err := os.WriteFile(filepath.Join(dir, "unexpected.md"), []byte("PRIVATE"), 0600); err != nil {
					t.Fatal(err)
				}
			case "empty note":
				if err := os.Mkdir(filepath.Join(root, "notes", strings.Repeat("b", 32)), 0700); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(filepath.Join(dir, "000001.md"), path); err != nil {
					t.Fatal(err)
				}
			case "directory revision":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.List(); err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("corruption hidden or leaked: %v", err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			if reopened, err := Open(root); err == nil {
				reopened.Close()
				t.Fatal("corruption accepted on reopen")
			}
		})
	}
}
