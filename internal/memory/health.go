package memory

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Reports contain counts/codes only: never filenames, paths, note text or errors
// from the OS. Limits apply to each inspection, not to the writer's data model.
const InspectMaxEntries = 20000
const InspectMaxBytes = 256 << 20
const InspectTimeout = 5 * time.Second
const LowDiskBytes = 256 << 20

var ErrInspectBusy = errors.New("memory: inspection busy; retry later")

type WriteFailures struct {
	Count    uint64 `json:"count"`
	LastAt   string `json:"last_at,omitempty"`
	LastCode string `json:"last_code,omitempty"`
}

type DiskHealth struct {
	Available       bool   `json:"available"`
	TotalBytes      uint64 `json:"total_bytes"`
	AvailableBytes  uint64 `json:"available_bytes"`
	AvailableInodes uint64 `json:"available_inodes"`
	Low             bool   `json:"low"`
}

type IntegrityHealth struct {
	Complete  bool           `json:"complete"`
	Entries   int            `json:"entries"`
	Notes     int            `json:"notes"`
	Revisions int            `json:"revisions"`
	Bytes     int64          `json:"bytes"`
	Issues    map[string]int `json:"issues"`
}

type StorageHealth struct {
	CheckedAt             string          `json:"checked_at"`
	Status                string          `json:"status"`
	Disk                  DiskHealth      `json:"disk"`
	Integrity             IntegrityHealth `json:"integrity"`
	StagingEntries        int             `json:"staging_entries"`
	WriteFailures         WriteFailures   `json:"write_failures"`
	WriteFailuresObserved bool            `json:"write_failures_observed"`
}

func (s *Store) trackWriteFailure(err error) {
	code := ""
	switch {
	case errors.Is(err, errStorage):
		code = "storage_io"
	case errors.Is(err, errCorrupt):
		code = "storage_corrupt"
	}
	if code != "" {
		s.writeFailures.Count++
		s.writeFailures.LastAt = time.Now().UTC().Format(time.RFC3339Nano)
		s.writeFailures.LastCode = code
	}
}

// Health performs no probes/writes/repairs. TryLock avoids queueing diagnostics
// behind a writer. A scan holds the same mutex as mutations for a stable view.
func (s *Store) Health(ctx context.Context) (StorageHealth, error) {
	if !s.mu.TryLock() {
		return StorageHealth{}, ErrInspectBusy
	}
	defer s.mu.Unlock()
	if s.closed {
		return StorageHealth{}, errClosed
	}
	report := inspect(ctx, s.rootFD, s.notesFD, s.stagingFD)
	report.WriteFailures, report.WriteFailuresObserved = s.writeFailures, true
	if s.writeFailures.Count > 0 && report.Status == "ok" {
		report.Status = "warning"
	}
	return report, nil
}

// InspectOffline opens existing directories and an existing writer lock only.
// LOCK_SH conflicts with the active LOCK_EX writer. Never call Open here: it
// creates directories, chmods the lock and refuses corrupt history too early.
func InspectOffline(ctx context.Context, root string) (StorageHealth, error) {
	if root == "" || strings.ContainsRune(root, 0) {
		return StorageHealth{}, ErrInvalid
	}
	for _, part := range strings.Split(root, "/") {
		if part == ".." {
			return StorageHealth{}, ErrInvalid
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return StorageHealth{}, ErrInvalid
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return StorageHealth{}, errStorage
	}
	for _, part := range strings.Split(strings.TrimPrefix(abs, "/"), "/") {
		if part == "" {
			continue
		}
		next, e := openDirectory(fd, part, false)
		unix.Close(fd)
		if e != nil {
			return StorageHealth{}, e
		}
		fd = next
	}
	defer unix.Close(fd)
	lock, err := unix.Openat(fd, ".writer.lock", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return StorageHealth{}, errStorage
	}
	defer unix.Close(lock)
	var stat unix.Stat_t
	if unix.Fstat(lock, &stat) != nil || !privateRegular(stat) {
		return StorageHealth{}, errCorrupt
	}
	if unix.Flock(lock, unix.LOCK_SH|unix.LOCK_NB) != nil {
		return StorageHealth{}, ErrInspectBusy
	}
	notes, err := openDirectory(fd, "notes", false)
	if err != nil {
		return StorageHealth{}, err
	}
	defer unix.Close(notes)
	staging, err := openDirectory(fd, ".staging", false)
	if err != nil {
		return StorageHealth{}, err
	}
	defer unix.Close(staging)
	return inspect(ctx, fd, notes, staging), nil
}

func privateRegular(st unix.Stat_t) bool {
	return st.Mode&unix.S_IFMT == unix.S_IFREG && st.Nlink == 1 && st.Uid == uint32(os.Geteuid()) && st.Mode&0077 == 0
}

type inspector struct {
	ctx     context.Context
	r       *StorageHealth
	stopped bool
}

func (v *inspector) issue(code string) { v.r.Integrity.Issues[code]++ }
func (v *inspector) allowed() bool {
	if v.stopped {
		return false
	}
	if v.ctx.Err() != nil {
		v.issue("cancelled_or_timeout")
		v.stopped = true
	}
	return !v.stopped
}

// A separately opened fd does not share the writer's directory cursor. Read at
// most budget+1 names, including unexpected entries; never follow those names.
func (v *inspector) names(fd int) []string {
	if !v.allowed() {
		return nil
	}
	reader, err := openDirectory(fd, ".", false)
	if err != nil {
		v.issue("directory_unreadable")
		return nil
	}
	file := os.NewFile(uintptr(reader), "inspection")
	defer file.Close()
	names := []string{}
	for v.allowed() {
		batch, err := file.Readdirnames(128)
		for _, name := range batch {
			if v.r.Integrity.Entries >= InspectMaxEntries {
				v.issue("entry_limit")
				v.stopped = true
				break
			}
			v.r.Integrity.Entries++
			names = append(names, name)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			v.issue("directory_unreadable")
			break
		}
	}
	sort.Strings(names)
	return names
}

func inspect(ctx context.Context, root, notes, staging int) StorageHealth {
	ctx, cancel := context.WithTimeout(ctx, InspectTimeout)
	defer cancel()
	r := StorageHealth{CheckedAt: time.Now().UTC().Format(time.RFC3339Nano), Status: "ok", Integrity: IntegrityHealth{Issues: map[string]int{}}}
	v := inspector{ctx: ctx, r: &r}
	var fs unix.Statfs_t
	if unix.Fstatfs(root, &fs) == nil && fs.Bsize > 0 {
		r.Disk = diskHealth(fs)
	} else {
		v.issue("disk_unavailable")
	}
	for _, fd := range []int{root, notes, staging} {
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || st.Mode&0077 != 0 || st.Uid != uint32(os.Geteuid()) {
			v.issue("directory_permissions")
		}
	}
	r.StagingEntries = len(v.names(staging)) // top-level only; never traverse staging
	latest := map[string]Note{}
	receipts := map[string]bool{}
	for _, id := range v.names(notes) {
		if !v.allowed() {
			break
		}
		if !idPattern.MatchString(id) {
			v.issue("unexpected_note_entry")
			continue
		}
		fd, err := openDirectory(notes, id, false)
		if err != nil {
			v.issue("note_directory_unreadable")
			continue
		}
		v.note(fd, id, latest, receipts)
		unix.Close(fd)
	}
	if v.allowed() {
		v.relations(latest)
	}
	r.Integrity.Complete = !v.stopped && r.Integrity.Issues["directory_unreadable"] == 0 && r.Integrity.Issues["note_directory_unreadable"] == 0
	if len(r.Integrity.Issues) > 0 {
		r.Status = "error"
	} else if r.Disk.Low || r.StagingEntries > 0 {
		r.Status = "warning"
	}
	return r
}

func diskHealth(fs unix.Statfs_t) DiskHealth {
	total, available := fs.Blocks*uint64(fs.Bsize), fs.Bavail*uint64(fs.Bsize)
	return DiskHealth{Available: true, TotalBytes: total, AvailableBytes: available, AvailableInodes: fs.Ffree,
		Low: available < LowDiskBytes || (total > 0 && available < total/20) || (fs.Files > 0 && fs.Ffree < 100)}
}

func (v *inspector) note(fd int, id string, latest map[string]Note, receipts map[string]bool) {
	v.r.Integrity.Notes++
	var dirStat unix.Stat_t
	if unix.Fstat(fd, &dirStat) != nil || dirStat.Uid != uint32(os.Geteuid()) || dirStat.Mode&0077 != 0 {
		v.issue("directory_permissions")
	}
	names := v.names(fd)
	if len(names) == 0 && v.allowed() {
		v.issue("empty_note_directory")
	}
	var previous []Note
	valid := true
	for i, name := range names {
		if !v.allowed() {
			return
		}
		if name != revisionName(i+1) {
			v.issue("revision_sequence")
			valid = false
		}
		if !revisionPattern.MatchString(name) {
			continue
		}
		var st unix.Stat_t
		if unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil || !privateRegular(st) || st.Size < 0 || st.Size > 1<<20 {
			v.issue("unsafe_revision")
			valid = false
			continue
		}
		if v.r.Integrity.Bytes+st.Size > InspectMaxBytes {
			v.issue("byte_limit")
			v.stopped = true
			return
		}
		v.r.Integrity.Bytes += st.Size
		n, err := readRevision(fd, name)
		if err != nil || n.ID != id || name != revisionName(n.Revision) {
			v.issue("corrupt_revision")
			valid = false
			previous = nil
			continue
		}
		v.r.Integrity.Revisions++
		if valid && !validHistoryTransition(previous, n) {
			v.issue("history_transition")
			valid = false
		}
		if n.operation != nil {
			if receipts[n.operation.Scope] {
				v.issue("duplicate_operation")
			}
			receipts[n.operation.Scope] = true
		}
		previous = []Note{n}
	}
	if valid && len(previous) > 0 {
		n := previous[0]
		n.Content, n.Title, n.Source, n.Author = "", "", "", ""
		latest[id] = n
	}
}

func revisionName(n int) string { return fmt.Sprintf("%06d.md", n) }

func (v *inspector) relations(latest map[string]Note) {
	summaries := map[string]bool{}
	for id, n := range latest {
		if !v.allowed() {
			return
		}
		if n.IsProjectSummary && !n.Archived {
			if summaries[n.Project] {
				v.issue("duplicate_summary")
			}
			summaries[n.Project] = true
		}
		for _, group := range [][]string{n.RelatedTo, n.Supersedes} {
			for _, target := range group {
				t, ok := latest[target]
				if !ok || target == id || t.Project != n.Project {
					v.issue("invalid_relation")
				}
			}
		}
	}
	// Kahn's algorithm avoids recursion on untrusted deep graphs; archived edges
	// participate, exactly as in writer validation. Related-to cycles are allowed.
	degree := map[string]int{}
	for id, n := range latest {
		degree[id] += 0
		for _, target := range n.Supersedes {
			if _, ok := latest[target]; ok {
				degree[target]++
			}
		}
	}
	queue := []string{}
	for id, d := range degree {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	visited := 0
	for len(queue) > 0 && v.allowed() {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		visited++
		for _, target := range latest[id].Supersedes {
			if _, ok := latest[target]; ok {
				degree[target]--
				if degree[target] == 0 {
					queue = append(queue, target)
				}
			}
		}
	}
	if v.allowed() && visited != len(latest) {
		v.issue("supersedes_cycle")
	}
}
