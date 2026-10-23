package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"golang.org/x/sys/unix"
)

var ErrOperationConflict = errors.New("memory: operation ID already used with a different payload")

var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,127}$`)
var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var principalPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)

// Operation identifies a retry, not an authorization grant. Callers must check
// current credentials, permissions and project scope BEFORE every Execute call.
// Principal must be the stable key ID, never the token or display name.
type Operation struct {
	Principal        string
	Tool             string
	ID               string
	NoteID           string
	ExpectedRevision int
}

// Receipts live inside the same canonical revision/atomic rename as the result.
// No raw operation ID, token, or second copy of the input is stored. The index is
// derived on startup and can be discarded; Markdown remains the source of truth.
type operationRecord struct {
	Version int    `json:"version"`
	Scope   string `json:"scope"`
	Payload string `json:"payload"`
}

type storedNote struct {
	Note
	Operation *operationRecord `json:"_operation,omitempty"`
}

type operationLocation struct {
	ID       string
	Revision int
	Payload  string
}

func validOperationRecord(record *operationRecord) bool {
	return record == nil || (record.Version == 1 && hashPattern.MatchString(record.Scope) && hashPattern.MatchString(record.Payload))
}

func hashJSON(value any) string {
	data, _ := json.Marshal(value)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (op Operation) record(input Input) (*operationRecord, error) {
	if !operationIDPattern.MatchString(op.ID) {
		return nil, fmt.Errorf("%w: operation_id must be 8-128 ASCII letters/digits/._:- with an alphanumeric first character", ErrInvalid)
	}
	if !principalPattern.MatchString(op.Principal) {
		return nil, ErrInvalid
	}
	switch op.Tool {
	case "save_memory", "save_handoff":
		if op.NoteID != "" || op.ExpectedRevision != 0 {
			return nil, ErrInvalid
		}
	case "revise_memory", "archive_memory":
		if !idPattern.MatchString(op.NoteID) || op.ExpectedRevision < 1 || op.ExpectedRevision >= 999999 {
			return nil, ErrInvalid
		}
	default:
		return nil, ErrInvalid
	}
	// Preserve the original field order and null/empty encoding: old immutable
	// receipts must still replay the exact original typed request. New metadata
	// is appended and omitted ONLY when absent, keeping legacy payload bytes
	// unchanged while distinguishing explicit false (clear) from nil (preserve).
	// Do not use Input's encoding: it conflates nil and empty relation slices.
	payload := struct {
		NoteID, Project, Kind, Title, Content, Status, Source string
		ExpectedRevision                                      int
		IsProjectSummary                                      *bool
		RelatedTo, Supersedes                                 []string
		IsTesting                                             *bool          `json:",omitempty"`
		SkillMetadata                                         *SkillMetadata `json:",omitempty"`
	}{op.NoteID, input.Project, input.Kind, input.Title, input.Content, input.Status, input.Source,
		op.ExpectedRevision, input.IsProjectSummary, input.RelatedTo, input.Supersedes, input.IsTesting, input.SkillMetadata}
	return &operationRecord{Version: 1,
		Scope: hashJSON([]string{op.Principal, op.Tool, op.ID}), Payload: hashJSON(payload)}, nil
}

// Execute supports the four MCP mutation tools. A replay returns the original
// committed revision, NOT the latest note, even after later revisions/archives.
// Failed validation/conflicts do not reserve the ID. No TTL/automatic deletion:
// receipts live as long as their immutable revision, including after restore.
func (s *Store) Execute(op Operation, input Input, author string) (note Note, resultErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() { s.trackWriteFailure(resultErr) }()
	return s.executeLocked(op, input, author, nil)
}

// ExecuteContext shares one request budget across receipt recovery, replay and
// mutation validation. Cancellation never reserves an operation ID. After the
// commit boundary (or replay fsyncs), cancellation cannot interrupt durability.
func (s *Store) ExecuteContext(ctx context.Context, op Operation, input Input, author string) (Note, error) {
	return s.mutationContext(ctx, func(b *readBudget) (Note, error) {
		return s.executeLocked(op, input, author, b)
	})
}

func (s *Store) executeLocked(op Operation, input Input, author string, budget *readBudget) (Note, error) {
	if s.closed {
		return Note{}, errClosed
	}
	if !validText(author, 200, true) {
		return Note{}, ErrInvalid
	}
	record, err := op.record(input)
	if err != nil {
		return Note{}, err
	}
	if s.operationsDirty {
		if err := s.rebuildOperationsBudget(budget); err != nil {
			return Note{}, err
		}
	}
	if location, ok := s.operations[record.Scope]; ok {
		if location.Payload != record.Payload {
			return Note{}, ErrOperationConflict
		}
		history, err := budget.history(s, location.ID, false)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrReadLimit) {
				return Note{}, err
			}
			return Note{}, errCorrupt
		}
		if location.Revision > len(history) {
			return Note{}, errCorrupt
		}
		n := history[location.Revision-1]
		if n.operation == nil || *n.operation != *record {
			return Note{}, errCorrupt
		}
		// Also retry directory fsyncs after a prior ambiguous post-rename error.
		fd, err := openDirectory(s.notesFD, n.ID, false)
		if err != nil {
			return Note{}, err
		}
		if err := budget.check(); err != nil {
			unix.Close(fd)
			return Note{}, err
		}
		syncErr := unix.Fsync(fd)
		unix.Close(fd)
		if syncErr != nil || unix.Fsync(s.notesFD) != nil || unix.Fsync(s.stagingFD) != nil {
			return Note{}, errStorage
		}
		return n, nil
	}
	var n Note
	switch op.Tool {
	case "save_memory", "save_handoff":
		if op.Tool == "save_handoff" {
			input.Kind = "handoff"
		}
		n, err = s.createBudget(input, author, record, budget)
	case "revise_memory":
		n, err = s.changeLockedBudget(op.NoteID, op.ExpectedRevision, author, false, record, reviseNoteBudget(s, input, author, budget), budget)
	case "archive_memory":
		n, err = s.changeLockedBudget(op.NoteID, op.ExpectedRevision, author, false, record, func(n *Note) error { n.Archived = true; return nil }, budget)
	}
	if err != nil {
		// A rename can succeed before fsync fails. Before the next operation,
		// recover from committed Markdown rather than executing the write again.
		s.operationsDirty = true
		return Note{}, err
	}
	s.operations[record.Scope] = operationLocation{n.ID, n.Revision, record.Payload}
	return n, nil
}

// Called under the store mutex (or during Open before publication).
func (s *Store) rebuildOperations() error {
	return s.rebuildOperationsBudget(nil)
}

func (s *Store) rebuildOperationsBudget(budget *readBudget) error {
	ids, err := budget.names(s.notesFD)
	if err != nil {
		return err
	}
	index := map[string]operationLocation{}
	for _, id := range ids {
		history, err := budget.history(s, id, false)
		if err != nil {
			return err
		}
		for _, n := range history {
			if record := n.operation; record != nil {
				if _, exists := index[record.Scope]; exists {
					return errCorrupt
				}
				index[record.Scope] = operationLocation{n.ID, n.Revision, record.Payload}
			}
		}
	}
	if err := budget.check(); err != nil {
		return err
	}
	s.operations, s.operationsDirty = index, false
	return nil
}
