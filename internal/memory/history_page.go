package memory

import (
	"context"
	"golang.org/x/sys/unix"
	"strconv"
	"strings"
)

type HistoryPage struct {
	Items  []Note `json:"items"`
	Before int    `json:"next_before"`
	Latest int    `json:"latest_revision"`
}

// HistoryPageContext validates the entire immutable chain but retains only one
// page plus the preceding revision. Cursor is exclusive; zero starts at latest.
// Pages are newest-first. Corruption anywhere fails closed, without partial data.
func (s *Store) HistoryPageContext(ctx context.Context, id string, before, limit int) (HistoryPage, error) {
	out := HistoryPage{Items: []Note{}}
	if !idPattern.MatchString(id) || before < 0 || limit < 1 || limit > 50 {
		return out, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()
	if err := s.lockRead(ctx); err != nil {
		return out, err
	}
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if s.closed {
		return out, errClosed
	}
	fd, err := openDirectory(s.notesFD, id, false)
	if err != nil {
		return out, err
	}
	defer unix.Close(fd)
	b := readBudget{ctx: ctx}
	names, err := b.names(fd)
	if err != nil {
		return out, err
	}
	if len(names) == 0 {
		return out, errCorrupt
	}
	out.Latest = len(names)
	end := len(names)
	if before > 0 && before-1 < end {
		end = before - 1
	}
	start := max(1, end-limit+1)
	var previous []Note
	for i, name := range names {
		if !revisionPattern.MatchString(name) {
			return HistoryPage{}, errCorrupt
		}
		revision, _ := strconv.Atoi(strings.TrimSuffix(name, ".md"))
		if revision != i+1 {
			return HistoryPage{}, errCorrupt
		}
		n, err := b.revision(fd, name)
		if err != nil {
			return HistoryPage{}, err
		}
		if n.ID != id || n.Revision != revision || !validHistoryTransition(previous, n) {
			return HistoryPage{}, errCorrupt
		}
		previous = []Note{n}
		if revision >= start && revision <= end {
			out.Items = append(out.Items, n)
		}
	}
	for i, j := 0, len(out.Items)-1; i < j; i, j = i+1, j-1 {
		out.Items[i], out.Items[j] = out.Items[j], out.Items[i]
	}
	if start > 1 && len(out.Items) > 0 {
		out.Before = start
	}
	if err := ctx.Err(); err != nil {
		return HistoryPage{}, err
	}
	return out, nil
}
