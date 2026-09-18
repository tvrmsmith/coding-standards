// Package waiver holds the release valve for the changed-lines lint gate: a
// one-shot waiver log an agent can record against and a human can audit.
//
// The log is append-only JSONL. Nothing already written is ever rewritten,
// so reading it top to bottom shows every waiver recorded and every one
// spent, with reasons. The threat model is an instruction-following agent,
// not an adversary, so the store does not attempt tamper-proofing.
package waiver

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// Waiver is permission for one rule to be suppressed on one path, once.
type Waiver struct {
	ID       string
	Language string
	Path     srcpath.Path
	Rule     string
	Reason   string
	Recorded time.Time
}

// Entry is a waiver plus what became of it, which is what an audit reads.
type Entry struct {
	Waiver
	SpentTree string    // empty if unspent
	SpentAt   time.Time // zero if unspent
}

// Store is the waiver log, held in memory alongside the file it was read
// from.
type Store struct {
	logPath string
	entries []Entry
}

// Open reads the log at logPath. A file that does not exist is an empty
// store; the file is created lazily on first append.
func Open(logPath string) (*Store, error) {
	s := &Store{logPath: logPath}

	f, err := os.Open(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening waiver log %s: %w", logPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		text := scanner.Text()
		if text == "" {
			continue
		}
		if err := s.readLine(text); err != nil {
			return nil, LineError{Path: logPath, Line: lineNum, Err: err}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading waiver log %s: %w", logPath, err)
	}

	return s, nil
}

// LineError is a waiver log line Open could not make sense of, a malformed
// record or a spend naming a waiver id the log never recorded. Hiding a bad
// line would defeat the audit the log exists for, so Open fails loudly and
// names exactly which line.
type LineError struct {
	Path string
	Line int
	Err  error
}

func (e LineError) Error() string {
	return fmt.Sprintf("%s:%d: %v", e.Path, e.Line, e.Err)
}

func (e LineError) Unwrap() error { return e.Err }

// readLine decodes one JSONL record and folds it into s.entries.
func (s *Store) readLine(text string) error {
	var l line
	if err := json.Unmarshal([]byte(text), &l); err != nil {
		return fmt.Errorf("malformed waiver log record: %w", err)
	}

	switch l.Kind {
	case kindWaiver:
		s.entries = append(s.entries, Entry{Waiver: Waiver{
			ID:       l.ID,
			Language: l.Language,
			Path:     l.Path,
			Rule:     l.Rule,
			Reason:   l.Reason,
			Recorded: l.Recorded,
		}})
	case kindSpend:
		idx, ok := s.index(l.ID)
		if !ok {
			return fmt.Errorf("spend record names unknown waiver id %s", l.ID)
		}
		s.entries[idx].SpentTree = l.Tree
		s.entries[idx].SpentAt = l.Spent
	default:
		return fmt.Errorf("waiver log record has unknown kind %q", l.Kind)
	}
	return nil
}

// List returns every waiver with its spend state, in the order recorded.
func (s *Store) List() []Entry {
	return s.entries
}

// line is one JSONL record, either a waiver or a spend, discriminated by
// Kind. One shape covers both so a reader need not guess which fields a
// line carries before it has decoded it.
type line struct {
	Kind     string       `json:"kind"`
	ID       string       `json:"id"`
	Language string       `json:"language,omitempty"`
	Path     srcpath.Path `json:"path,omitempty"`
	Rule     string       `json:"rule,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	Recorded time.Time    `json:"recorded,omitempty"`
	Tree     string       `json:"tree,omitempty"`
	Spent    time.Time    `json:"spent,omitempty"`
}

const (
	kindWaiver = "waiver"
	kindSpend  = "spend"
)

// Record appends a new waiver and returns it with its assigned ID.
func (s *Store) Record(w Waiver) (Waiver, error) {
	if w.Language == "" {
		return Waiver{}, fmt.Errorf("waiver: language is required")
	}
	if w.Path == "" {
		return Waiver{}, fmt.Errorf("waiver: path is required")
	}
	if w.Rule == "" {
		return Waiver{}, fmt.Errorf("waiver: rule is required")
	}
	if w.Reason == "" {
		return Waiver{}, fmt.Errorf("waiver: reason is required")
	}

	id, err := newID()
	if err != nil {
		return Waiver{}, fmt.Errorf("generating waiver id: %w", err)
	}
	w.ID = id
	if w.Recorded.IsZero() {
		w.Recorded = time.Now()
	}

	if err := s.append(line{
		Kind:     kindWaiver,
		ID:       w.ID,
		Language: w.Language,
		Path:     w.Path,
		Rule:     w.Rule,
		Reason:   w.Reason,
		Recorded: w.Recorded,
	}); err != nil {
		return Waiver{}, err
	}

	s.entries = append(s.entries, Entry{Waiver: w})
	return w, nil
}

// Match finds a waiver covering this rule on this path that is still
// usable: either never spent, or already spent against this same tree. It
// returns the oldest usable waiver when several match, so the store drains
// in recording order.
func (s *Store) Match(language string, path srcpath.Path, rule, tree string) (Waiver, bool) {
	for _, entry := range s.entries {
		if entry.Language != language || entry.Path != path || entry.Rule != rule {
			continue
		}
		if entry.SpentTree == "" || entry.SpentTree == tree {
			return entry.Waiver, true
		}
	}
	return Waiver{}, false
}

// Spend appends a spend record. Spending one already spent against the same
// tree is a no-op, so a retried commit does not burn a second waiver.
//
// Spending a waiver already spent against a different tree is an error.
// Match already refuses to hand that waiver back for any tree but the one it
// was first spent against, so a caller that reaches Spend on it anyway has a
// bug worth hearing about rather than a silent no-op to mask it.
func (s *Store) Spend(w Waiver, tree string) error {
	idx, ok := s.index(w.ID)
	if !ok {
		return fmt.Errorf("waiver: no waiver with id %s", w.ID)
	}
	if spentTree := s.entries[idx].SpentTree; spentTree != "" {
		if spentTree == tree {
			return nil
		}
		return fmt.Errorf("waiver: %s was already spent against tree %s, not %s", w.ID, spentTree, tree)
	}

	spent := time.Now()
	if err := s.append(line{
		Kind:  kindSpend,
		ID:    w.ID,
		Tree:  tree,
		Spent: spent,
	}); err != nil {
		return err
	}

	s.entries[idx].SpentTree = tree
	s.entries[idx].SpentAt = spent
	return nil
}

// index finds the entry recorded under id.
func (s *Store) index(id string) (int, bool) {
	for i, entry := range s.entries {
		if entry.ID == id {
			return i, true
		}
	}
	return 0, false
}

// newID generates a waiver ID: 16 random bytes, hex-encoded.
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// append writes one record as a single []byte ending in "\n", opened with
// os.O_APPEND|os.O_CREATE|os.O_WRONLY. This relies on O_APPEND atomicity for
// writes under PIPE_BUF, which is what a single JSONL record is, so two
// processes appending at once cannot interleave a partial line.
func (s *Store) append(l line) error {
	data, err := json.Marshal(l)
	if err != nil {
		return fmt.Errorf("encoding waiver log record: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(s.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening waiver log %s: %w", s.logPath, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("writing waiver log %s: %w", s.logPath, err)
	}
	return nil
}
