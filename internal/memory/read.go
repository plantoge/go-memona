package memory

import (
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Request reads fail as a whole when a budget is exhausted. They never return
// a partial snapshot that could be mistaken for complete authorization/relations.
// Legacy reads and mutations (including startup) retain their unbounded behavior.
const ReadMaxEntries = 20000
const ReadMaxBytes int64 = 256 << 20
const ReadTimeout = 5 * time.Second

var ErrReadLimit = errors.New("memory: read resource limit exceeded")

type readBudget struct {
	ctx     context.Context
	entries int
	bytes   int64
}

func (b *readBudget) check() error {
	if b == nil {
		return nil // Legacy operations do not acquire request budgets.
	}
	return b.ctx.Err()
}

func (b *readBudget) names(fd int) ([]string, error) {
	if b == nil {
		return directoryNames(fd)
	}
	if err := b.check(); err != nil {
		return nil, err
	}
	reader, err := openDirectory(fd, ".", false)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(reader), "request-read")
	defer file.Close()
	names := []string{}
	for {
		if err := b.check(); err != nil {
			return nil, err
		}
		// At most one entry beyond the budget is observed, including names
		// that turn out to be invalid. Never allocate an unbounded directory.
		count := min(128, ReadMaxEntries-b.entries+1)
		batch, err := file.Readdirnames(count)
		b.entries += len(batch)
		if b.entries > ReadMaxEntries {
			return nil, ErrReadLimit
		}
		names = append(names, batch...)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errStorage
		}
	}
	sort.Strings(names)
	if err := b.check(); err != nil {
		return nil, err
	}
	return names, nil
}

func (b *readBudget) revision(dir int, name string) (Note, error) {
	if err := b.check(); err != nil {
		return Note{}, err
	}
	return readRevisionBudget(dir, name, b)
}

func (b *readBudget) history(s *Store, id string, latestOnly bool) ([]Note, error) {
	if b == nil {
		return s.history(id)
	}
	if err := b.check(); err != nil {
		return nil, err
	}
	if !idPattern.MatchString(id) {
		return nil, ErrInvalid
	}
	fd, err := openDirectory(s.notesFD, id, false)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	names, err := b.names(fd)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, errCorrupt
	}
	history := []Note{}
	for i, name := range names {
		if err := b.check(); err != nil {
			return nil, err
		}
		if !revisionPattern.MatchString(name) {
			return nil, errCorrupt
		}
		revision, _ := strconv.Atoi(strings.TrimSuffix(name, ".md"))
		if revision != i+1 {
			return nil, errCorrupt
		}
		n, err := b.revision(fd, name)
		if err != nil {
			return nil, err
		}
		if n.ID != id || n.Revision != revision || !validHistoryTransition(history, n) {
			return nil, errCorrupt
		}
		if latestOnly && len(history) != 0 {
			history[0] = n // Validation needs only the preceding revision.
		} else {
			history = append(history, n)
		}
	}
	if err := b.check(); err != nil {
		return nil, err
	}
	return history, nil
}

// lockRead waits cooperatively without a worker goroutine stranded on a mutex.
// A blocked disk syscall still cannot be forcibly interrupted by a context.
func (s *Store) lockRead(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.mu.TryLock() {
		return nil
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := ctx.Err(); err != nil {
				return err
			}
			if s.mu.TryLock() {
				return nil
			}
		}
	}
}

func (s *Store) read(ctx context.Context, id string, list, history bool) ([]Note, error) {
	ctx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()
	if err := s.lockRead(ctx); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.closed {
		return nil, errClosed
	}
	b := readBudget{ctx: ctx}
	if !list {
		return b.history(s, id, !history)
	}
	return b.list(s)
}

func (b *readBudget) list(s *Store) ([]Note, error) {
	if b == nil {
		return s.list()
	}
	names, err := b.names(s.notesFD)
	if err != nil {
		return nil, err
	}
	notes := make([]Note, 0, len(names))
	for _, id := range names {
		// Unlike Get, an invalid ID in the directory is storage corruption.
		if !idPattern.MatchString(id) {
			return nil, errCorrupt
		}
		h, err := b.history(s, id, true)
		if err != nil {
			return nil, err
		}
		notes = append(notes, h[0])
	}
	if err := b.check(); err != nil {
		return nil, err
	}
	return notes, nil
}

// ListContext returns a complete ID-sorted latest snapshot or an error, never
// partial results. Budgets include all historical revisions validated en route.
func (s *Store) ListContext(ctx context.Context) ([]Note, error) {
	return s.read(ctx, "", true, false)
}

func (s *Store) GetContext(ctx context.Context, id string) (Note, error) {
	h, err := s.read(ctx, id, false, false)
	if err != nil {
		return Note{}, err
	}
	return h[0], nil
}

func (s *Store) HistoryContext(ctx context.Context, id string) ([]Note, error) {
	return s.read(ctx, id, false, true)
}
