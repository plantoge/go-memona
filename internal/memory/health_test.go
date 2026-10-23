package memory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func healthSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		st, err := os.Lstat(path)
		if err != nil {
			return err
		}
		value := fmt.Sprint(st.Mode())
		if st.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf(" %x", sha256.Sum256(data))
		}
		if st.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += target
		}
		result[path] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestHealthReadOnlyHistoryAndOfflineLock(t *testing.T) {
	s, root := openTestStore(t)
	input := fixtureInput()
	n, err := s.Execute(testOperation("save_memory"), input, "synthetic-private-author")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(n.ID, 1, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Restore(n.ID, 2, "admin"); err != nil {
		t.Fatal(err)
	}
	before := healthSnapshot(t, root)
	r, err := s.Health(t.Context())
	if err != nil || !r.Integrity.Complete || len(r.Integrity.Issues) != 0 || r.Integrity.Revisions != 3 || !r.Disk.Available || !r.WriteFailuresObserved {
		t.Fatal(r, err)
	}
	data, _ := json.Marshal(r)
	for _, secret := range []string{root, n.ID, input.Title, input.Content, "synthetic-private-author", testOperation("save_memory").ID} {
		if strings.Contains(string(data), secret) {
			t.Fatal("diagnostics leaked private content")
		}
	}
	if _, err := InspectOffline(t.Context(), root); !errors.Is(err, ErrInspectBusy) {
		t.Fatal("offline did not refuse active writer", err)
	}
	s.mu.Lock()
	_, err = s.Health(t.Context())
	s.mu.Unlock()
	if !errors.Is(err, ErrInspectBusy) {
		t.Fatal("diagnostic queued behind writer")
	}
	if !reflect.DeepEqual(before, healthSnapshot(t, root)) {
		t.Fatal("online inspection changed data")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = InspectOffline(t.Context(), root)
	if err != nil || !r.Integrity.Complete || r.Status == "error" || r.WriteFailuresObserved {
		t.Fatal(r, err)
	}
	if !reflect.DeepEqual(before, healthSnapshot(t, root)) {
		t.Fatal("offline inspection changed data")
	}
	if _, err := s.Health(t.Context()); !errors.Is(err, errClosed) {
		t.Fatal(err)
	}
}

func TestHealthCorruptionAndStagingRetained(t *testing.T) {
	s, root := openTestStore(t)
	n := mustCreate(t, s, fixtureInput())
	if err := os.WriteFile(filepath.Join(root, "notes", n.ID, "000002.md"), []byte("PRIVATE_CORRUPT_BODY"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".staging", "PRIVATE_STAGE_NAME"), []byte("PRIVATE_STAGE_BODY"), 0600); err != nil {
		t.Fatal(err)
	}
	before := healthSnapshot(t, root)
	r, err := s.Health(t.Context())
	if err != nil || r.Status != "error" || r.Integrity.Issues["corrupt_revision"] != 1 || r.StagingEntries != 1 {
		t.Fatal(r, err)
	}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "PRIVATE") {
		t.Fatal("report leaked staging or corrupt contents")
	}
	s.Close()
	if reopened, err := Open(root); err == nil {
		reopened.Close()
		t.Fatal("corrupt history should prevent startup")
	}
	r, err = InspectOffline(t.Context(), root)
	if err != nil || r.Integrity.Issues["corrupt_revision"] != 1 {
		t.Fatal("cannot inspect a store which fails startup", r, err)
	}
	if !reflect.DeepEqual(before, healthSnapshot(t, root)) {
		t.Fatal("inspection repaired or removed evidence")
	}
}

func TestHealthUnsafeEntriesNeverFollowed(t *testing.T) {
	s, root := openTestStore(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("PRIVATE_OUTSIDE"), 0600); err != nil {
		t.Fatal(err)
	}
	for i, kind := range []string{"symlink", "fifo", "hardlink", "public", "oversize"} {
		n := mustCreate(t, s, fixtureInput())
		path := filepath.Join(root, "notes", n.ID, "000002.md")
		var err error
		switch kind {
		case "symlink":
			err = os.Symlink(outside, path)
		case "fifo":
			err = unix.Mkfifo(path, 0600)
		case "hardlink":
			err = os.Link(outside, path)
		case "public":
			err = os.WriteFile(path, []byte("x"), 0644)
		case "oversize":
			var f *os.File
			f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err == nil {
				err = f.Truncate(1<<20 + 1)
				f.Close()
			}
		}
		if err != nil {
			t.Fatalf("fixture %d: %v", i, err)
		}
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(root, "notes", strings.Repeat("f", 32))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "PRIVATE_NAME"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	r, err := s.Health(t.Context())
	if err != nil || r.Integrity.Issues["unsafe_revision"] != 5 || r.Integrity.Issues["note_directory_unreadable"] != 1 || r.Integrity.Issues["unexpected_note_entry"] != 1 || r.Integrity.Complete {
		t.Fatal(r, err)
	}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "PRIVATE") {
		t.Fatal("unsafe entry leaked")
	}
}

func TestHealthWriteFailuresValidationAndRestart(t *testing.T) {
	s, root := openTestStore(t)
	bad := fixtureInput()
	bad.Title = ""
	if _, err := s.Create(bad, "test"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	n := mustCreate(t, s, fixtureInput())
	if _, err := s.Archive(n.ID, 99, "test"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	r, _ := s.Health(t.Context())
	if r.WriteFailures.Count != 0 {
		t.Fatal("validation/conflict counted as I/O")
	}
	fd := s.stagingFD
	s.stagingFD = -1
	_, createErr := s.Create(fixtureInput(), "test")
	_, reviseErr := s.Revise(n.ID, 1, fixtureInput(), "test")
	_, executeErr := s.Execute(testOperation("save_memory"), fixtureInput(), "test")
	s.stagingFD = fd
	for _, err := range []error{createErr, reviseErr, executeErr} {
		if !errors.Is(err, errStorage) {
			t.Fatal(err)
		}
	}
	r, err := s.Health(t.Context())
	if err != nil || r.WriteFailures.Count != 3 || r.WriteFailures.LastCode != "storage_io" || r.WriteFailures.LastAt == "" || r.Status == "ok" {
		t.Fatal(r, err)
	}
	s.Close()
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	r, _ = reopened.Health(t.Context())
	if r.WriteFailures.Count != 0 {
		t.Fatal("counter should be process-local")
	}
}

func TestHealthCancellationAndBudgets(t *testing.T) {
	s, root := openTestStore(t)
	n := mustCreate(t, s, fixtureInput())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r, err := s.Health(ctx)
	if err != nil || r.Integrity.Complete || r.Integrity.Issues["cancelled_or_timeout"] != 1 {
		t.Fatal(r, err)
	}
	r = StorageHealth{Integrity: IntegrityHealth{Issues: map[string]int{}, Entries: InspectMaxEntries}}
	v := inspector{ctx: t.Context(), r: &r}
	v.names(s.notesFD)
	if !v.stopped || r.Integrity.Issues["entry_limit"] != 1 {
		t.Fatal(r)
	}
	r = StorageHealth{Integrity: IntegrityHealth{Issues: map[string]int{}, Bytes: InspectMaxBytes}}
	v = inspector{ctx: t.Context(), r: &r}
	fd, err := openDirectory(s.notesFD, n.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	v.note(fd, n.ID, map[string]Note{}, map[string]bool{})
	if !v.stopped || r.Integrity.Issues["byte_limit"] != 1 {
		t.Fatal(r)
	}
	missing := filepath.Join(root, "missing")
	if _, err := InspectOffline(t.Context(), missing); err == nil {
		t.Fatal("missing directory created")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("offline created path")
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectOffline(t.Context(), alias); err == nil {
		t.Fatal("symlink root followed")
	}
}

func TestHealthDiskThresholds(t *testing.T) {
	for _, tc := range []struct {
		bytes, available, inodes uint64
		low                      bool
	}{
		{10 << 30, 1 << 30, 1000, false}, {10 << 30, 100 << 20, 1000, true}, {100 << 30, 1 << 30, 1000, true}, {10 << 30, 1 << 30, 0, true},
	} {
		fs := unix.Statfs_t{Bsize: 1, Blocks: tc.bytes, Bavail: tc.available, Files: 10000, Ffree: tc.inodes}
		if got := diskHealth(fs); got.Low != tc.low || got.AvailableBytes != tc.available {
			t.Fatal(got, tc)
		}
	}
}

func TestHealthDiskAndDirectoryUnavailable(t *testing.T) {
	s, _ := openTestStore(t)
	r := inspect(t.Context(), -1, s.notesFD, s.stagingFD)
	if r.Disk.Available || r.Status != "error" || r.Integrity.Issues["disk_unavailable"] != 1 {
		t.Fatal(r)
	}
	r = inspect(t.Context(), s.rootFD, -1, s.stagingFD)
	if r.Integrity.Complete || r.Integrity.Issues["directory_unreadable"] != 1 {
		t.Fatal(r)
	}
}

func TestHealthCrossNoteAndHistoryCorruption(t *testing.T) {
	s, root := openTestStore(t)
	a := mustCreate(t, s, fixtureInput())
	b := mustCreate(t, s, fixtureInput())
	// Canonical bytes can still encode invalid relationships or transitions.
	a.Supersedes, b.Supersedes = []string{b.ID}, []string{a.ID}
	a.IsProjectSummary, b.IsProjectSummary, a.Kind, b.Kind = true, true, "knowledge", "knowledge"
	record, _ := testOperation("save_memory").record(fixtureInput())
	a.operation, b.operation = record, record
	for _, n := range []Note{a, b} {
		if err := os.WriteFile(filepath.Join(root, "notes", n.ID, "000001.md"), encode(n), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.Health(t.Context())
	if err != nil || r.Integrity.Issues["supersedes_cycle"] != 1 || r.Integrity.Issues["duplicate_summary"] != 1 || r.Integrity.Issues["duplicate_operation"] != 1 {
		t.Fatal(r, err)
	}
	b.RelatedTo = []string{strings.Repeat("f", 32)}
	if err := os.WriteFile(filepath.Join(root, "notes", b.ID, "000001.md"), encode(b), 0600); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Health(t.Context())
	if r.Integrity.Issues["invalid_relation"] != 1 {
		t.Fatal(r)
	}
	b.Revision, b.Project = 2, "other"
	if err := os.WriteFile(filepath.Join(root, "notes", b.ID, "000002.md"), encode(b), 0600); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Health(t.Context())
	if r.Integrity.Issues["history_transition"] != 1 {
		t.Fatal(r)
	}
}

func TestHealthMissingLockNeverCreated(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	for _, name := range []string{"notes", ".staging"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	before := healthSnapshot(t, root)
	if _, err := InspectOffline(t.Context(), root); err == nil {
		t.Fatal("missing lock accepted")
	}
	if !reflect.DeepEqual(before, healthSnapshot(t, root)) {
		t.Fatal("inspector created missing lock")
	}
}
