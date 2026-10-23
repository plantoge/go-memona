package memory

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func testOperation(tool string) Operation {
	return Operation{Principal: strings.Repeat("a", 24), Tool: tool, ID: "synthetic-operation-001"}
}

func TestOperationsReplayOriginalHistoryAndRestart(t *testing.T) {
	s, root := openTestStore(t)
	input := fixtureInput()
	op := testOperation("save_memory")
	first, err := s.Execute(op, input, "writer")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		replay, err := s.Execute(op, input, "renamed-writer")
		if err != nil || !reflect.DeepEqual(first, replay) {
			t.Fatal("create replay changed the original result", err)
		}
	}
	input.Content = "Second revision"
	revise := testOperation("revise_memory")
	revise.NoteID, revise.ExpectedRevision = first.ID, 1
	second, err := s.Execute(revise, input, "writer")
	if err != nil {
		t.Fatal(err)
	}
	archive := testOperation("archive_memory")
	archive.NoteID, archive.ExpectedRevision = first.ID, 2
	third, err := s.Execute(archive, Input{}, "writer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Restore(first.ID, 3, "admin"); err != nil {
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
	for _, tc := range []struct {
		op    Operation
		input Input
		want  Note
	}{{op, fixtureInput(), first}, {revise, input, second}, {archive, Input{}, third}} {
		replay, err := reopened.Execute(tc.op, tc.input, "writer")
		if err != nil || !reflect.DeepEqual(replay, tc.want) {
			t.Fatalf("%s restart replay: %v", tc.op.Tool, err)
		}
	}
	history, err := reopened.History(first.ID)
	if err != nil || len(history) != 4 || history[3].operation != nil {
		t.Fatal("replay added revision or admin restore inherited receipt", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "notes", first.ID, "000001.md"))
	if err != nil || !bytes.Contains(data, []byte(`"_operation"`)) || bytes.Contains(data, []byte(op.ID)) || bytes.Contains(data, []byte(op.Principal)) {
		t.Fatal("receipt missing or raw ID/principal stored", err)
	}
	// Public note JSON must not expose the storage-only receipt.
	public, err := json.Marshal(first)
	if err != nil || bytes.Contains(public, []byte("_operation")) {
		t.Fatal("internal receipt leaked to public JSON")
	}
}

func TestOperationsScopeConflictAndHandoff(t *testing.T) {
	s, _ := openTestStore(t)
	op, input := testOperation("save_memory"), fixtureInput()
	first, err := s.Execute(op, input, "writer")
	if err != nil {
		t.Fatal(err)
	}
	other := input
	other.Content = "Different arguments"
	if _, err := s.Execute(op, other, "writer"); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("different payload accepted", err)
	}
	op.Principal = strings.Repeat("b", 24)
	second, err := s.Execute(op, other, "writer")
	if err != nil || second.ID == first.ID {
		t.Fatal("principal namespace collided", err)
	}
	op = testOperation("save_handoff")
	handoff, err := s.Execute(op, input, "writer")
	if err != nil || handoff.ID == first.ID || handoff.Kind != "handoff" {
		t.Fatal("tool namespace or forced kind failed", err)
	}
	replayed, err := s.Execute(op, input, "writer")
	if err != nil || !reflect.DeepEqual(handoff, replayed) {
		t.Fatal("handoff duplicated", err)
	}
	// Hash the original typed request even when handoff normalizes kind.
	input.Kind = "handoff"
	if _, err := s.Execute(op, input, "writer"); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("changed original kind did not conflict", err)
	}
}

func TestOperationsMetadataOmissionAndClearAreDifferent(t *testing.T) {
	s, _ := openTestStore(t)
	input, op := fixtureInput(), testOperation("save_memory")
	if _, err := s.Execute(op, input, "writer"); err != nil {
		t.Fatal(err)
	}
	input.RelatedTo = []string{}
	if _, err := s.Execute(op, input, "writer"); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("omitted metadata and explicit clear have same hash", err)
	}
	input.RelatedTo = nil
	flag := false
	input.IsProjectSummary = &flag
	if _, err := s.Execute(op, input, "writer"); !errors.Is(err, ErrOperationConflict) {
		t.Fatal("omitted flag and false have same hash", err)
	}
}

func TestOperationsConcurrentRetryOneCommit(t *testing.T) {
	s, _ := openTestStore(t)
	for _, tool := range []string{"save_memory", "save_handoff", "revise_memory", "archive_memory"} {
		t.Run(tool, func(t *testing.T) {
			op, input := testOperation(tool), fixtureInput()
			if tool == "revise_memory" || tool == "archive_memory" {
				n := mustCreate(t, s, input)
				op.NoteID, op.ExpectedRevision = n.ID, 1
			}
			if tool == "archive_memory" {
				input = Input{}
			}
			const workers = 12
			var wg sync.WaitGroup
			start := make(chan struct{})
			results := make(chan Note, workers)
			errors := make(chan error, workers)
			for i := 0; i < workers; i++ {
				wg.Go(func() { <-start; n, err := s.Execute(op, input, "writer"); results <- n; errors <- err })
			}
			close(start)
			wg.Wait()
			close(results)
			close(errors)
			for err := range errors {
				if err != nil {
					t.Fatal(err)
				}
			}
			var first Note
			for n := range results {
				if first.ID != "" && !reflect.DeepEqual(first, n) {
					t.Fatal("concurrent retries returned different results")
				}
				first = n
			}
			history, err := s.History(first.ID)
			if err != nil || len(history) != first.Revision {
				t.Fatal("retry wrote extra revisions", err)
			}
		})
	}
}

func TestOperationsFailedValidationDoesNotReserveID(t *testing.T) {
	s, _ := openTestStore(t)
	op, input := testOperation("save_memory"), fixtureInput()
	input.Title = ""
	if _, err := s.Execute(op, input, "writer"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	input.Title = "Corrected"
	if _, err := s.Execute(op, input, "writer"); err != nil {
		t.Fatal("validation reserved operation ID", err)
	}
	for _, id := range []string{"short", "bad space operation", "../traversal", "-invalid-first", strings.Repeat("x", 129)} {
		op.ID = id
		if _, err := s.Execute(op, input, "writer"); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid ID accepted", err)
		}
	}
}

func TestOperationsRebuildAfterAmbiguousCommit(t *testing.T) {
	s, _ := openTestStore(t)
	op, input := testOperation("save_memory"), fixtureInput()
	record, err := op.record(input)
	if err != nil {
		t.Fatal(err)
	}
	// Model the window after canonical rename, before updating the derived index
	// or delivering a response. Only committed Markdown may determine replay.
	s.mu.Lock()
	n, err := s.create(input, "writer", record)
	s.operationsDirty = true
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Execute(op, input, "writer")
	if err != nil || !reflect.DeepEqual(n, replay) {
		t.Fatal("committed receipt not recovered from Markdown", err)
	}
	// A replay must not report durable success if its directory fsync fails.
	fd := s.stagingFD
	s.stagingFD = -1
	_, err = s.Execute(op, input, "writer")
	s.stagingFD = fd
	if !errors.Is(err, errStorage) {
		t.Fatal("fsync error hidden", err)
	}
	if _, err := s.Execute(op, input, "writer"); err != nil {
		t.Fatal("retry failed after transient sync error", err)
	}
}

func TestOperationsAbruptProcessExit(t *testing.T) {
	if root := os.Getenv("MEMONA_TEST_OPERATION_ROOT"); root != "" {
		s, err := Open(root)
		if err != nil {
			os.Exit(2)
		}
		if _, err := s.Execute(testOperation("save_memory"), fixtureInput(), "writer"); err != nil {
			os.Exit(3)
		}
		// No Close/graceful shutdown; the parent deliberately receives no result.
		os.Exit(0)
	}
	root := filepath.Join(t.TempDir(), "crashed-writer")
	cmd := exec.Command(os.Args[0], "-test.run=^TestOperationsAbruptProcessExit$")
	cmd.Env = append(os.Environ(), "MEMONA_TEST_OPERATION_ROOT="+root)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	notes, err := s.List()
	if err != nil || len(notes) != 1 {
		t.Fatal("lost committed write", err)
	}
	n, err := s.Execute(testOperation("save_memory"), fixtureInput(), "writer")
	if err != nil || !reflect.DeepEqual(n, notes[0]) {
		t.Fatal("restart retry duplicated committed write", err)
	}
}

func TestOperationsLegacyRevisionsDoNotInheritReceipt(t *testing.T) {
	s, root := openTestStore(t)
	n, err := s.Execute(testOperation("save_memory"), fixtureInput(), "writer")
	if err != nil {
		t.Fatal(err)
	}
	n, err = s.Revise(n.ID, 1, fixtureInput(), "legacy-writer")
	if err != nil || n.operation != nil {
		t.Fatal("legacy revision inherited receipt", err)
	}
	if _, err := s.Archive(n.ID, 2, "legacy-archiver"); err != nil {
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
	if _, err := reopened.Execute(testOperation("save_memory"), fixtureInput(), "writer"); err != nil {
		t.Fatal(err)
	}
}

func TestOperationsCorruptAndDuplicateReceiptRefused(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "malformed", true: "duplicate"}[duplicate], func(t *testing.T) {
			s, root := openTestStore(t)
			op, input := testOperation("save_memory"), fixtureInput()
			record, err := op.record(input)
			if err != nil {
				t.Fatal(err)
			}
			if duplicate {
				if _, err := s.Execute(op, input, "writer"); err != nil {
					t.Fatal(err)
				}
			} else {
				record.Scope = "invalid"
			}
			// Commit deliberately invalid synthetic fixtures using internal helpers.
			s.mu.Lock()
			_, err = s.create(input, "writer", record)
			s.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			s.Close()
			if reopened, err := Open(root); !errors.Is(err, errCorrupt) {
				if reopened != nil {
					reopened.Close()
				}
				t.Fatal("invalid receipt accepted on startup", err)
			}
		})
	}
}

func TestOperationsStagingIsNotACommittedReceipt(t *testing.T) {
	s, _ := openTestStore(t)
	op, input := testOperation("save_memory"), fixtureInput()
	record, _ := op.record(input)
	// Simulate an abandoned file before rename; no canonical note references it.
	n := Note{ID: strings.Repeat("f", 32), operation: record}
	fd, err := unix.Openat(s.stagingFD, "retained-operation", unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Write(fd, encode(n)); err != nil {
		t.Fatal(err)
	}
	unix.Close(fd)
	s.operationsDirty = true
	if _, err := s.Execute(op, input, "writer"); err != nil {
		t.Fatal("abandoned staging prevented new attempt", err)
	}
	if names, err := directoryNames(s.stagingFD); err != nil || len(names) != 1 {
		t.Fatal("staging fixture removed", err)
	}
}

func TestOperationsConcurrentDifferentPayloadOneWinner(t *testing.T) {
	s, _ := openTestStore(t)
	op := testOperation("save_memory")
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, content := range []string{"Payload one", "Payload two"} {
		go func(content string) {
			<-start
			input := fixtureInput()
			input.Content = content
			_, err := s.Execute(op, input, "writer")
			results <- err
		}(content)
	}
	close(start)
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrOperationConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	notes, err := s.List()
	if err != nil || successes != 1 || conflicts != 1 || len(notes) != 1 {
		t.Fatal("different-payload race did not have one commit and one conflict", err)
	}
}

func TestOperationsSummaryReplayAfterUniqueSummaryAndSkillPublication(t *testing.T) {
	s, _ := openTestStore(t)
	op, input := testOperation("save_memory"), fixtureInput()
	flag := true
	input.Kind, input.IsProjectSummary = "knowledge", &flag
	n, err := s.Execute(op, input, "writer")
	if err != nil {
		t.Fatal(err)
	}
	if replay, err := s.Execute(op, input, "writer"); err != nil || !reflect.DeepEqual(n, replay) {
		t.Fatal("summary uniqueness rejected replay", err)
	}
	op.ID = "skill-operation-001"
	input.Kind, input.Status, input.IsProjectSummary = "skill", "draft", nil
	skill, err := s.Execute(op, input, "writer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(skill.ID, 1, "admin"); err != nil {
		t.Fatal(err)
	}
	if replay, err := s.Execute(op, input, "writer"); err != nil || !reflect.DeepEqual(skill, replay) {
		t.Fatal("published skill replay did not return original draft", err)
	}
	if latest, err := s.Get(skill.ID); err != nil || latest.Status != "active" || latest.operation != nil {
		t.Fatal("replay reverted published skill or publish inherited receipt", err)
	}
}
