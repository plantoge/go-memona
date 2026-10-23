package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadContextMatchesLegacyAndValidatesHistory(t *testing.T) {
	s, root := openTestStore(t)
	ctx := t.Context()
	empty, err := s.ListContext(ctx)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatal("invalid empty snapshot", err)
	}
	n := mustCreate(t, s, fixtureInput())
	for i := 0; i < 3; i++ {
		n, err = s.Revise(n.ID, n.Revision, fixtureInput(), "tester")
		if err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListContext(ctx)
	legacy, oldErr := s.List()
	if err != nil || oldErr != nil || !reflect.DeepEqual(list, legacy) {
		t.Fatal("snapshot differs", err, oldErr)
	}
	h, err := s.HistoryContext(ctx, n.ID)
	oldHistory, oldErr := s.History(n.ID)
	if err != nil || oldErr != nil || !reflect.DeepEqual(h, oldHistory) {
		t.Fatal("history differs", err, oldErr)
	}
	got, err := s.GetContext(ctx, n.ID)
	if err != nil || !reflect.DeepEqual(got, n) {
		t.Fatal("latest differs", err)
	}
	// Latest-only cannot skip corruption in an earlier revision.
	path := filepath.Join(root, "notes", n.ID, "000001.md")
	if err := os.WriteFile(path, []byte("synthetic corruption"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := s.ListContext(ctx); !errors.Is(err, errCorrupt) || out != nil {
		t.Fatal("partial snapshot or skipped corruption", err)
	}
}

func TestReadContextCancellationDoesNotWaitForMutex(t *testing.T) {
	s, _ := openTestStore(t)
	n := mustCreate(t, s, fixtureInput())
	for _, method := range []string{"list", "get", "history"} {
		t.Run(method, func(t *testing.T) {
			s.mu.Lock()
			defer s.mu.Unlock()
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() {
				var err error
				switch method {
				case "list":
					_, err = s.ListContext(ctx)
				case "get":
					_, err = s.GetContext(ctx, n.ID)
				case "history":
					_, err = s.HistoryContext(ctx, n.ID)
				}
				done <- err
			}()
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("canceled read waited on held mutex")
			}
		})
	}
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	if out, err := s.ListContext(ctx); !errors.Is(err, context.DeadlineExceeded) || out != nil {
		t.Fatal(err)
	}
	if _, err := s.GetContext(t.Context(), n.ID); err != nil {
		t.Fatal("subsequent read failed", err)
	}
}

func TestReadBudgetBoundariesAndCancellation(t *testing.T) {
	s, _ := openTestStore(t)
	n := mustCreate(t, s, fixtureInput())
	fd, err := openDirectory(s.notesFD, n.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstatat(fd, "000001.md", &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		t.Fatal(err)
	}
	b := readBudget{ctx: t.Context(), entries: ReadMaxEntries - 1}
	if names, err := b.names(fd); err != nil || len(names) != 1 || b.entries != ReadMaxEntries {
		t.Fatal("exact entry budget failed", err)
	}
	b = readBudget{ctx: t.Context(), entries: ReadMaxEntries}
	if names, err := b.names(fd); !errors.Is(err, ErrReadLimit) || names != nil {
		t.Fatal("entry budget bypass", err)
	}
	b = readBudget{ctx: t.Context(), bytes: ReadMaxBytes - st.Size}
	if _, err := b.revision(fd, "000001.md"); err != nil || b.bytes != ReadMaxBytes {
		t.Fatal("exact byte budget failed", err)
	}
	b.bytes = ReadMaxBytes - st.Size + 1
	if got, err := b.revision(fd, "000001.md"); !errors.Is(err, ErrReadLimit) || got.ID != "" {
		t.Fatal("byte budget bypass", err)
	}
	b = readBudget{ctx: t.Context()}
	if _, err := b.revision(fd, "000001.md"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	b.ctx = ctx
	if _, err := b.revision(fd, "000001.md"); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled revision read", err)
	}
	if out, err := b.history(s, n.ID, false); !errors.Is(err, context.Canceled) || out != nil {
		t.Fatal("canceled history read", err)
	}
}

func TestReadContextClosedAndInvalid(t *testing.T) {
	s, _ := openTestStore(t)
	if _, err := s.GetContext(t.Context(), "../escape"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if out, err := s.ListContext(t.Context()); !errors.Is(err, errClosed) || out != nil {
		t.Fatal(err)
	}
}
