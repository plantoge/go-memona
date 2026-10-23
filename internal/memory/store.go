// Package memory stores private, immutable note revisions on a local filesystem.
// Each revision is canonical Markdown: a JSON object (valid YAML) between ---
// frontmatter delimiters, a blank line, the content, and a final newline. Content
// is also recorded in the JSON so truncated or mismatched bodies are detected.
// Source is a citation/reference string; it is never opened or executed.
package memory

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

var (
	ErrNotFound     = errors.New("memory: not found")
	ErrConflict     = errors.New("memory: revision conflict")
	ErrInvalid      = errors.New("memory: invalid input")
	errCorrupt      = errors.New("memory: corrupt storage")
	errStorage      = errors.New("memory: storage operation failed")
	errClosed       = errors.New("memory: store is closed")
	slugPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	idPattern       = regexp.MustCompile(`^[0-9a-f]{32}$`)
	revisionPattern = regexp.MustCompile(`^[0-9]{6}\.md$`)
)

type Note struct {
	ID               string         `json:"id"`
	Project          string         `json:"project"`
	Kind             string         `json:"kind"`
	Title            string         `json:"title"`
	Content          string         `json:"content"`
	Status           string         `json:"status"`
	Source           string         `json:"source"`
	Author           string         `json:"author"`
	Revision         int            `json:"revision"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at"`
	Archived         bool           `json:"archived"`
	IsProjectSummary bool           `json:"is_project_summary,omitempty"`
	IsTesting        bool           `json:"is_testing,omitempty"`
	RelatedTo        []string       `json:"related_to,omitempty"`
	Supersedes       []string       `json:"supersedes,omitempty"`
	SkillMetadata    *SkillMetadata `json:"skill_metadata,omitempty"`
	VerifiedAt       string         `json:"verified_at,omitempty"`
	NeedsReview      bool           `json:"needs_review,omitempty"`
	operation        *operationRecord
}

type Input struct {
	Project          string         `json:"project"`
	Kind             string         `json:"kind"`
	Title            string         `json:"title"`
	Content          string         `json:"content"`
	Status           string         `json:"status" jsonschema:"For fact/decision/knowledge/handoff use unverified or verified (verified requires source evidence). New skills must use draft. NEVER use active for ordinary notes; it is reserved for admin-published skills."`
	Source           string         `json:"source"`
	IsProjectSummary *bool          `json:"is_project_summary,omitempty" jsonschema:"Optional project overview flag, knowledge notes only. Omit on revise to preserve; false clears. One nonarchived summary per project, content max 8000 bytes."`
	IsTesting        *bool          `json:"is_testing,omitempty" jsonschema:"Optional explicit testing/example note flag. Omit on revise to preserve; false clears. Does not change status or permissions."`
	RelatedTo        []string       `json:"related_to,omitempty" jsonschema:"Optional related note IDs in the SAME project, max 20. Omit on revise to preserve; empty array clears. Requires read and write when supplied."`
	Supersedes       []string       `json:"supersedes,omitempty" jsonschema:"Optional IDs this note explicitly replaces, SAME project, max 20. Omit on revise to preserve; empty array clears. Requires read and write when supplied. No self links or cycles."`
	SkillMetadata    *SkillMetadata `json:"skill_metadata,omitempty" jsonschema:"Optional structured metadata for skills only. Omit on revise to preserve; empty object clears. Descriptive text only, not executable or a publish approval."`
	VerifiedAt       *string        `json:"verified_at,omitempty" jsonschema:"Optional self-declared verification time for non-skills, canonical UTC RFC3339Nano (Z). Requires verified status and source evidence. Omit on revise to preserve while verified; empty string clears. Not a server verification."`
	NeedsReview      *bool          `json:"needs_review,omitempty" jsonschema:"Optional explicit review flag for non-skills. Omit on revise to preserve; false clears. Does not change status, permissions, or supersedes relations."`
}

// Store holds an exclusive process-wide writer lease until Close. Operations on
// one Store are serialized, including reads. Writes are fsynced in .staging then
// atomically moved without replacement; new notes are staged as complete dirs.
// Interrupted staging entries are ignored and retained, never automatically
// deleted. A failure after rename may mean the mutation committed: use Execute
// with the same operation ID/arguments to replay, or inspect state before a
// non-idempotent retry. Requires Linux and a local filesystem
// supporting flock, fsync, and renameat2(RENAME_NOREPLACE).
type Store struct {
	mu              sync.Mutex
	rootFD          int
	notesFD         int
	stagingFD       int
	lockFD          int
	closed          bool
	operations      map[string]operationLocation
	operationsDirty bool
	writeFailures   WriteFailures
}

// Open rejects symlinks in every component of root and beneath notes. All later
// access is relative to pinned directory descriptors with O_NOFOLLOW, not paths
// supplied by callers. Missing root components are created with mode 0700.
func Open(root string) (*Store, error) {
	if root == "" || strings.ContainsRune(root, 0) {
		return nil, ErrInvalid
	}
	// Reject traversal even though filepath.Abs would otherwise clean it away.
	for _, part := range strings.Split(filepath.ToSlash(root), "/") {
		if part == ".." {
			return nil, ErrInvalid
		}
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, ErrInvalid
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errStorage
	}
	for _, part := range strings.Split(strings.TrimPrefix(absolute, "/"), "/") {
		if part == "" {
			continue
		}
		next, openErr := openDirectory(fd, part, true)
		unix.Close(fd)
		if openErr != nil {
			return nil, openErr
		}
		fd = next
	}
	s := &Store{rootFD: fd, notesFD: -1, stagingFD: -1, lockFD: -1}
	fail := func(err error) (*Store, error) { s.Close(); return nil, err }
	s.lockFD, err = unix.Openat(fd, ".writer.lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		return fail(errStorage)
	}
	var stat unix.Stat_t
	if unix.Fstat(s.lockFD, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return fail(errCorrupt)
	}
	if err = unix.Flock(s.lockFD, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return fail(ErrConflict)
		}
		return fail(errStorage)
	}
	if unix.Fchmod(s.lockFD, 0600) != nil {
		return fail(errStorage)
	}
	s.notesFD, err = openDirectory(fd, "notes", true)
	if err != nil {
		return fail(err)
	}
	s.stagingFD, err = openDirectory(fd, ".staging", true)
	if err != nil {
		return fail(err)
	}
	// Refuse an existing shared staging directory rather than changing its
	// permissions or trusting files another OS user can replace.
	if unix.Fstat(s.stagingFD, &stat) != nil || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0077 != 0 {
		return fail(errCorrupt)
	}
	if err = s.rebuildOperations(); err != nil {
		return fail(err)
	}
	return s, nil
}

func openDirectory(parent int, name string, create bool) (int, error) {
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) && create {
		if err = unix.Mkdirat(parent, name, 0700); err != nil && !errors.Is(err, unix.EEXIST) {
			return -1, errStorage
		}
		if unix.Fsync(parent) != nil {
			return -1, errStorage
		}
		fd, err = unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	}
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return -1, ErrNotFound
		}
		return -1, errCorrupt
	}
	return fd, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var failed bool
	for _, fd := range []int{s.notesFD, s.stagingFD, s.lockFD, s.rootFD} {
		// Closing the lock descriptor releases flock, including on Open failure.
		if fd >= 0 && unix.Close(fd) != nil {
			failed = true
		}
	}
	if failed {
		return errStorage
	}
	return nil
}

func validText(value string, max int, required bool) bool {
	if !utf8.ValidString(value) || len(value) > max || (required && strings.TrimSpace(value) == "") {
		return false
	}
	return !strings.ContainsFunc(value, unicode.IsControl)
}

func validate(input Input, author string, allowActive bool) error {
	if !slugPattern.MatchString(input.Project) || !validText(input.Title, 200, true) ||
		!validText(input.Source, 2000, false) || !validText(author, 200, true) ||
		!utf8.ValidString(input.Content) || len(input.Content) > 64*1024 || strings.ContainsRune(input.Content, 0) {
		return ErrInvalid
	}
	switch input.Kind {
	case "fact", "decision", "knowledge", "handoff", "skill":
	default:
		return ErrInvalid
	}
	switch input.Status {
	case "unverified", "verified", "draft":
	case "active":
		if !allowActive || input.Kind != "skill" {
			return fmt.Errorf("%w: status active is reserved for admin-published skills; ordinary notes use unverified or verified", ErrInvalid)
		}
	default:
		return ErrInvalid
	}
	if input.Kind == "skill" && input.Status != "draft" && !(allowActive && input.Status == "active") {
		return fmt.Errorf("%w: new or revised skills must use status draft", ErrInvalid)
	}
	if input.Status == "verified" && strings.TrimSpace(input.Source) == "" {
		return fmt.Errorf("%w: status verified requires evidence in source; otherwise use unverified", ErrInvalid)
	}
	if err := validateMetadata(input); err != nil {
		return err
	}
	return nil
}

func noteInput(n Note) Input {
	summary, testing := n.IsProjectSummary, n.IsTesting
	var verifiedAt *string
	var needsReview *bool
	// Skills cannot receive even explicit clears. Preserve nil for valid skills,
	// but expose illegal stored values so history validation rejects them.
	if n.Kind != "skill" || n.VerifiedAt != "" {
		value := n.VerifiedAt
		verifiedAt = &value
	}
	if n.Kind != "skill" || n.NeedsReview {
		value := n.NeedsReview
		needsReview = &value
	}
	return Input{Project: n.Project, Kind: n.Kind, Title: n.Title, Content: n.Content, Status: n.Status, Source: n.Source,
		IsProjectSummary: &summary, IsTesting: &testing, RelatedTo: n.RelatedTo, Supersedes: n.Supersedes, SkillMetadata: cloneSkillMetadata(n.SkillMetadata),
		VerifiedAt: verifiedAt, NeedsReview: needsReview}
}

func sameInput(a, b Input) bool { return reflect.DeepEqual(a, b) }

func (s *Store) Create(input Input, author string) (note Note, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() { s.trackWriteFailure(err) }()
	return s.create(input, author, nil)
}

// mutationContext bounds admission and validation with the same request-local
// budget as reads. The mutex stays held through validation and commit. Helpers
// stop consulting the budget immediately before staging, so cancellation cannot
// interrupt a commit or turn its success into an ambiguous context error.
func (s *Store) mutationContext(ctx context.Context, mutate func(*readBudget) (Note, error)) (note Note, err error) {
	ctx, cancel := context.WithTimeout(ctx, ReadTimeout)
	defer cancel()
	if err := s.lockRead(ctx); err != nil {
		return Note{}, err
	}
	defer s.mu.Unlock()
	defer func() { s.trackWriteFailure(err) }()
	if err := ctx.Err(); err != nil {
		return Note{}, err
	}
	return mutate(&readBudget{ctx: ctx})
}

// CreateContext may cancel before staging; after staging starts it finishes the
// existing atomic write protocol even if ctx is canceled.
func (s *Store) CreateContext(ctx context.Context, input Input, author string) (Note, error) {
	return s.mutationContext(ctx, func(b *readBudget) (Note, error) {
		return s.createBudget(input, author, nil, b)
	})
}

func (s *Store) create(input Input, author string, operation *operationRecord) (Note, error) {
	return s.createBudget(input, author, operation, nil)
}

func (s *Store) createBudget(input Input, author string, operation *operationRecord, budget *readBudget) (Note, error) {
	if s.closed {
		return Note{}, errClosed
	}
	if err := validate(input, author, false); err != nil {
		return Note{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Note{}, errStorage
	}
	id := hex.EncodeToString(random[:])
	n := Note{ID: id, Project: input.Project, Kind: input.Kind, Title: input.Title, Content: input.Content,
		Status: input.Status, Source: input.Source, Author: author, Revision: 1, operation: operation}
	applyMetadata(&n, input)
	if err := s.validateConnectionsBudget(n, budget); err != nil {
		return Note{}, err
	}
	if err := budget.check(); err != nil {
		return Note{}, err
	}
	// Commit boundary: no context checks after the first staging operation.
	stage := "note-" + id
	if err := unix.Mkdirat(s.stagingFD, stage, 0700); err != nil {
		return Note{}, errStorage
	}
	if unix.Fsync(s.stagingFD) != nil {
		return Note{}, errStorage
	}
	fd, err := openDirectory(s.stagingFD, stage, false)
	if err != nil {
		return Note{}, err
	}
	defer unix.Close(fd)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	n.CreatedAt, n.UpdatedAt = now, now
	if err := writeRevision(s.stagingFD, fd, n); err != nil {
		return Note{}, err
	}
	if err := moveExclusive(s.stagingFD, stage, s.notesFD, id); err != nil {
		return Note{}, err
	}
	return n, nil
}

func (s *Store) Revise(id string, expected int, input Input, author string) (Note, error) {
	return s.change(id, expected, author, false, reviseNote(s, input, author))
}

func (s *Store) ReviseContext(ctx context.Context, id string, expected int, input Input, author string) (Note, error) {
	return s.mutationContext(ctx, func(b *readBudget) (Note, error) {
		return s.changeLockedBudget(id, expected, author, false, nil, reviseNoteBudget(s, input, author, b), b)
	})
}

func reviseNote(s *Store, input Input, author string) func(*Note) error {
	return reviseNoteBudget(s, input, author, nil)
}

func reviseNoteBudget(s *Store, input Input, author string, budget *readBudget) func(*Note) error {
	return func(n *Note) error {
		if input.Project != n.Project || input.Kind != n.Kind {
			return ErrInvalid
		}
		// Editing an active skill always requires a new review, even when a UI
		// submits the previous active status with the rest of the note.
		if n.Kind == "skill" && n.Status == "active" && input.Kind == "skill" && (input.Status == "active" || input.Status == "draft") {
			input.Status = "draft"
		}
		if err := validate(input, author, false); err != nil {
			return err
		}
		n.Project, n.Kind, n.Title, n.Content, n.Status, n.Source = input.Project, input.Kind, input.Title, input.Content, input.Status, input.Source
		applyMetadata(n, input)
		if err := validate(noteInput(*n), author, false); err != nil {
			return err
		}
		if err := s.validateConnectionsBudget(*n, budget); err != nil {
			return err
		}
		return nil
	}
}

func (s *Store) Archive(id string, expected int, author string) (Note, error) {
	return s.change(id, expected, author, false, func(n *Note) error { n.Archived = true; return nil })
}

func (s *Store) ArchiveContext(ctx context.Context, id string, expected int, author string) (Note, error) {
	return s.mutationContext(ctx, func(b *readBudget) (Note, error) {
		return s.changeLockedBudget(id, expected, author, false, nil, func(n *Note) error { n.Archived = true; return nil }, b)
	})
}

func (s *Store) PublishContext(ctx context.Context, id string, expected int, author string) (Note, error) {
	return s.mutationContext(ctx, func(b *readBudget) (Note, error) {
		return s.changeLockedBudget(id, expected, author, false, nil, publishNote, b)
	})
}

func publishNote(n *Note) error {
	if n.Kind != "skill" || n.Status != "draft" {
		return ErrInvalid
	}
	n.Status = "active"
	return nil
}

func (s *Store) Publish(id string, expected int, author string) (Note, error) {
	return s.change(id, expected, author, false, publishNote)
}

// Restore appends an unarchived copy of an archived note. Authorization belongs
// to the caller; this storage API does not establish admin identity.
func (s *Store) Restore(id string, expected int, author string) (Note, error) {
	return s.change(id, expected, author, true, func(n *Note) error {
		n.Archived = false
		return s.validateConnections(*n)
	})
}

func (s *Store) RestoreContext(ctx context.Context, id string, expected int, author string) (Note, error) {
	return s.mutationContext(ctx, func(b *readBudget) (Note, error) {
		return s.changeLockedBudget(id, expected, author, true, nil, func(n *Note) error {
			n.Archived = false
			return s.validateConnectionsBudget(*n, b)
		}, b)
	})
}

func (s *Store) change(id string, expected int, author string, restoring bool, update func(*Note) error) (note Note, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() { s.trackWriteFailure(err) }()
	return s.changeLocked(id, expected, author, restoring, nil, update)
}

func (s *Store) changeLocked(id string, expected int, author string, restoring bool, operation *operationRecord, update func(*Note) error) (Note, error) {
	return s.changeLockedBudget(id, expected, author, restoring, operation, update, nil)
}

func (s *Store) changeLockedBudget(id string, expected int, author string, restoring bool, operation *operationRecord, update func(*Note) error, budget *readBudget) (Note, error) {
	if s.closed {
		return Note{}, errClosed
	}
	if !idPattern.MatchString(id) || expected < 1 || !validText(author, 200, true) {
		return Note{}, ErrInvalid
	}
	history, err := budget.history(s, id, true)
	if err != nil {
		return Note{}, err
	}
	n := history[len(history)-1]
	// An operation receipt belongs only to the revision that committed it.
	n.operation = operation
	if n.Revision != expected || n.Archived != restoring {
		return Note{}, ErrConflict
	}
	if n.Revision >= 999999 {
		return Note{}, ErrInvalid
	}
	if err := update(&n); err != nil {
		return Note{}, err
	}
	n.Revision++
	n.Author = author
	now := time.Now().UTC()
	previous, _ := time.Parse(time.RFC3339Nano, n.UpdatedAt)
	if now.Before(previous) {
		now = previous
	}
	n.UpdatedAt = now.Format(time.RFC3339Nano)
	fd, err := openDirectory(s.notesFD, id, false)
	if err != nil {
		return Note{}, err
	}
	defer unix.Close(fd)
	if err := budget.check(); err != nil {
		return Note{}, err
	}
	// Commit boundary: writeRevision and its fsync/rename path are uninterrupted.
	if err := writeRevision(s.stagingFD, fd, n); err != nil {
		return Note{}, err
	}
	return n, nil
}

func encode(n Note) []byte {
	n.SkillMetadata = cloneSkillMetadata(n.SkillMetadata)
	metadata, _ := json.Marshal(storedNote{Note: n, Operation: n.operation})
	return []byte("---\n" + string(metadata) + "\n---\n\n" + n.Content + "\n")
}

func writeRevision(staging, dir int, n Note) error {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return errStorage
	}
	stage := "revision-" + hex.EncodeToString(random[:])
	fd, err := unix.Openat(staging, stage, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return errStorage
	}
	file := os.NewFile(uintptr(fd), stage)
	data := encode(n)
	written, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || written != len(data) || syncErr != nil || closeErr != nil {
		return errStorage
	}
	return moveExclusive(staging, stage, dir, fmt.Sprintf("%06d.md", n.Revision))
}

func moveExclusive(source int, from string, destination int, to string) error {
	if err := unix.Renameat2(source, from, destination, to, unix.RENAME_NOREPLACE); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return ErrConflict
		}
		return errStorage
	}
	// Sync both directory entries after the atomic move. Even if a sync fails,
	// the destination contains a whole revision rather than a partial file.
	destinationErr := unix.Fsync(destination)
	sourceErr := unix.Fsync(source)
	if destinationErr != nil || sourceErr != nil {
		return errStorage
	}
	return nil
}

func (s *Store) Get(id string) (Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Note{}, errClosed
	}
	h, err := s.history(id)
	if err != nil {
		return Note{}, err
	}
	return h[len(h)-1], nil
}

func (s *Store) History(id string) ([]Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errClosed
	}
	return s.history(id)
}

func directoryNames(fd int) ([]string, error) {
	reader, err := openDirectory(fd, ".", false)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(reader), "directory")
	defer file.Close()
	names, err := file.Readdirnames(-1)
	if err != nil {
		return nil, errStorage
	}
	sort.Strings(names)
	return names, nil
}

func (s *Store) history(id string) ([]Note, error) {
	if !idPattern.MatchString(id) {
		return nil, ErrInvalid
	}
	fd, err := openDirectory(s.notesFD, id, false)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	names, err := directoryNames(fd)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, errCorrupt
	}
	history := make([]Note, 0, len(names))
	for i, name := range names {
		if !revisionPattern.MatchString(name) {
			return nil, errCorrupt
		}
		revision, _ := strconv.Atoi(strings.TrimSuffix(name, ".md"))
		if revision != i+1 {
			return nil, errCorrupt
		}
		n, err := readRevision(fd, name)
		if err != nil {
			return nil, err
		}
		if n.ID != id || n.Revision != revision {
			return nil, errCorrupt
		}
		if !validHistoryTransition(history, n) {
			return nil, errCorrupt
		}
		history = append(history, n)
	}
	return history, nil
}

// Only the previous revision is needed, shared with the bounded inspector.
func validHistoryTransition(history []Note, n Note) bool {
	i := len(history)
	if i == 0 {
		if n.Archived || n.CreatedAt != n.UpdatedAt || (n.Kind == "skill" && n.Status != "draft") {
			return false
		}
	} else {
		previous := history[i-1]
		updated, _ := time.Parse(time.RFC3339Nano, n.UpdatedAt)
		oldUpdated, _ := time.Parse(time.RFC3339Nano, previous.UpdatedAt)
		if n.CreatedAt != previous.CreatedAt || n.Project != previous.Project || n.Kind != previous.Kind || updated.Before(oldUpdated) {
			return false
		}
		// Archived records can only transition to an identical, restored
		// record; archiving likewise preserves all user-editable fields.
		if previous.Archived && (n.Archived || !sameInput(noteInput(n), noteInput(previous))) {
			return false
		}
		if n.Archived && !sameInput(noteInput(n), noteInput(previous)) {
			return false
		}
		if n.Status == "active" && !n.Archived && !previous.Archived {
			oldInput := noteInput(previous)
			oldInput.Status = "active"
			if previous.Kind != "skill" || previous.Status != "draft" || !sameInput(noteInput(n), oldInput) {
				return false
			}
		}
	}
	return true
}

func readRevision(dir int, name string) (Note, error) {
	return readRevisionBudget(dir, name, nil)
}

func readRevisionBudget(dir int, name string, budget *readBudget) (Note, error) {
	fd, err := unix.Openat(dir, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return Note{}, errCorrupt
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Size > 1024*1024 {
		return Note{}, errCorrupt
	}
	if budget != nil {
		if err := budget.check(); err != nil {
			return Note{}, err
		}
		if stat.Size < 0 || stat.Size > ReadMaxBytes-budget.bytes {
			return Note{}, ErrReadLimit
		}
		budget.bytes += stat.Size
	}
	data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return Note{}, errCorrupt
	}
	if budget != nil {
		// Managed files are immutable; a change in size signals an external
		// writer and must not bypass the pre-read byte budget.
		if int64(len(data)) != stat.Size {
			return Note{}, errCorrupt
		}
		if err := budget.check(); err != nil {
			return Note{}, err
		}
	}
	lines := bytes.SplitN(data, []byte("\n"), 3)
	var stored storedNote
	if len(lines) != 3 || string(lines[0]) != "---" || json.Unmarshal(lines[1], &stored) != nil {
		return Note{}, errCorrupt
	}
	n := stored.Note
	n.operation = stored.Operation
	if !validOperationRecord(n.operation) || !bytes.Equal(data, encode(n)) {
		return Note{}, errCorrupt
	}
	if validate(noteInput(n), n.Author, true) != nil || !idPattern.MatchString(n.ID) || n.Revision < 1 || n.Revision > 999999 {
		return Note{}, errCorrupt
	}
	created, err := time.Parse(time.RFC3339Nano, n.CreatedAt)
	if err != nil {
		return Note{}, errCorrupt
	}
	updated, err := time.Parse(time.RFC3339Nano, n.UpdatedAt)
	if err != nil || updated.Before(created) {
		return Note{}, errCorrupt
	}
	return n, nil
}

// List returns the latest revision of every note, including archived notes,
// sorted by ID. History returns revisions oldest first. Empty lists are non-nil.
func (s *Store) List() ([]Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errClosed
	}
	return s.list()
}

func (s *Store) list() ([]Note, error) {
	names, err := directoryNames(s.notesFD)
	if err != nil {
		return nil, err
	}
	notes := make([]Note, 0, len(names))
	for _, id := range names {
		if !idPattern.MatchString(id) {
			return nil, errCorrupt
		}
		h, err := s.history(id)
		if err != nil {
			return nil, err
		}
		notes = append(notes, h[len(h)-1])
	}
	return notes, nil
}
