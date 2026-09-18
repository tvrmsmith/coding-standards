package waiver_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
	"github.com/tvrmsmith/coding-standards/lint/internal/waiver"
)

func TestOpen_missingFile_isEmptyStore(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "waivers.jsonl")

	store, err := waiver.Open(logPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if got := store.List(); len(got) != 0 {
		t.Fatalf("List() = %v, want empty", got)
	}
}

func TestRecord_thenMatch_findsIt(t *testing.T) {
	store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	w, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if w.ID == "" {
		t.Fatalf("Record did not assign an ID")
	}

	got, ok := store.Match("csharp", srcpath.FromSlash("src/OrderService.cs"), "TVRM0001", "abc123", nil)
	if !ok {
		t.Fatalf("Match: not found")
	}
	if got.ID != w.ID {
		t.Fatalf("Match ID = %q, want %q", got.ID, w.ID)
	}
}

// A waiver with no path is how an analyzer load failure is waived: Roslyn
// reports AD0001 at Location.None, so there is no path to name and the empty
// one is the value Match keys on rather than a field the record is missing.
func TestRecordAndMatch_pathlessWaiver(t *testing.T) {
	store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	w, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Rule:     "AD0001",
		Reason:   "the analyzer is broken upstream",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, ok := store.Match("csharp", "", "AD0001", "abc123", nil)
	if !ok {
		t.Fatalf("Match found no waiver for a pathless finding")
	}
	if got.ID != w.ID {
		t.Fatalf("Match returned %s, want %s", got.ID, w.ID)
	}
	if _, ok := store.Match("csharp", srcpath.FromSlash("src/Order.cs"), "AD0001", "abc123", nil); ok {
		t.Fatalf("a pathless waiver matched a finding that named a path")
	}
}

func TestMatch_requiresLanguagePathAndRule(t *testing.T) {
	store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if _, ok := store.Match("csharp", srcpath.FromSlash("src/Other.cs"), "TVRM0001", "abc123", nil); ok {
		t.Fatalf("Match matched on a different path")
	}
	if _, ok := store.Match("go", srcpath.FromSlash("src/OrderService.cs"), "TVRM0001", "abc123", nil); ok {
		t.Fatalf("Match matched on a different language")
	}
	if _, ok := store.Match("csharp", srcpath.FromSlash("src/OrderService.cs"), "TVRM0002", "abc123", nil); ok {
		t.Fatalf("Match matched on a different rule")
	}
}

func TestRecord_rejectsEmptyFields(t *testing.T) {
	valid := waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	}

	cases := []struct {
		name string
		w    waiver.Waiver
	}{
		{"empty reason", func() waiver.Waiver { w := valid; w.Reason = ""; return w }()},
		{"empty language", func() waiver.Waiver { w := valid; w.Language = ""; return w }()},
		{"empty rule", func() waiver.Waiver { w := valid; w.Rule = ""; return w }()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if _, err := store.Record(c.w); err == nil {
				t.Fatalf("Record(%+v) = nil error, want an error", c.w)
			}
		})
	}
}

func TestRecord_recordedTime(t *testing.T) {
	store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	before := time.Now()
	w, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	after := time.Now()
	if w.Recorded.Before(before) || w.Recorded.After(after) {
		t.Fatalf("Recorded = %v, want between %v and %v", w.Recorded, before, after)
	}

	pinned := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	w2, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0002",
		Reason:   "second waiver",
		Recorded: pinned,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if !w2.Recorded.Equal(pinned) {
		t.Fatalf("Recorded = %v, want %v", w2.Recorded, pinned)
	}
}

func TestRecord_ignoresCallerSuppliedID(t *testing.T) {
	store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	w, err := store.Record(waiver.Waiver{
		ID:       "caller-chosen-id",
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if w.ID == "caller-chosen-id" {
		t.Fatalf("Record kept the caller-supplied ID")
	}
}

func TestSpend_wornExampleFromAssignment(t *testing.T) {
	store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	w, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	if err := store.Spend(w, "abc123"); err != nil {
		t.Fatalf("Spend: %v", err)
	}

	// Same tree: the waiver is still usable, since a retried commit against
	// the same tree must not burn a second waiver.
	if _, ok := store.Match("csharp", srcpath.FromSlash("src/OrderService.cs"), "TVRM0001", "abc123", nil); !ok {
		t.Fatalf("Match after same-tree spend: not found")
	}

	// A different tree: the waiver is used up.
	if _, ok := store.Match("csharp", srcpath.FromSlash("src/OrderService.cs"), "TVRM0001", "def456", nil); ok {
		t.Fatalf("Match after spend matched a different tree")
	}
}

func TestSpend_alreadySpentAgainstDifferentTree_isError(t *testing.T) {
	store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	w, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Spend(w, "abc123"); err != nil {
		t.Fatalf("Spend: %v", err)
	}

	err = store.Spend(w, "def456")
	if err == nil {
		t.Fatalf("Spend(different tree) = nil error, want an error")
	}
}

func TestSpend_unknownID_isError(t *testing.T) {
	store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	err = store.Spend(waiver.Waiver{ID: "does-not-exist"}, "abc123")
	if err == nil {
		t.Fatalf("Spend(unknown id) = nil error, want an error")
	}
}

func TestSpend_sameTreeAgain_addsNoLine(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "waivers.jsonl")
	store, err := waiver.Open(logPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	w, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Spend(w, "abc123"); err != nil {
		t.Fatalf("Spend: %v", err)
	}
	linesAfterFirstSpend := fileLineCount(t, logPath)

	if err := store.Spend(w, "abc123"); err != nil {
		t.Fatalf("Spend again: %v", err)
	}
	if got := fileLineCount(t, logPath); got != linesAfterFirstSpend {
		t.Fatalf("line count after repeat spend = %d, want %d", got, linesAfterFirstSpend)
	}
}

// A waiver the caller already took for another finding in the same run is not
// handed back for a second one: one waiver, one finding.
func TestMatch_skipsClaimedWaivers(t *testing.T) {
	store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Two waivers for the same rule on the same path, so the claim has to skip
	// past the first and land on the second. With only one recorded, a Match
	// that refused every lookup whenever anything was claimed would pass too.
	first, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "the claimed one",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	second, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "the one left",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	claimed := map[string]bool{first.ID: true}
	got, ok := store.Match("csharp", srcpath.FromSlash("src/OrderService.cs"), "TVRM0001", "abc123", claimed)
	if !ok {
		t.Fatalf("Match returned nothing, so a claimed waiver blocked the unclaimed one behind it")
	}
	if got.ID != second.ID {
		t.Fatalf("Match returned %s, want %s: the claimed waiver was handed back again", got.ID, second.ID)
	}
}

func TestMatch_returnsOldestUsable(t *testing.T) {
	store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	first, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "first waiver",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	second, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "second waiver",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, ok := store.Match("csharp", srcpath.FromSlash("src/OrderService.cs"), "TVRM0001", "abc123", nil)
	if !ok {
		t.Fatalf("Match: not found")
	}
	if got.ID != first.ID {
		t.Fatalf("Match returned %q, want the oldest waiver %q (not %q)", got.ID, first.ID, second.ID)
	}

	// Spending the oldest drains the store toward the next one, in recording
	// order.
	if err := store.Spend(first, "def456"); err != nil {
		t.Fatalf("Spend: %v", err)
	}
	got, ok = store.Match("csharp", srcpath.FromSlash("src/OrderService.cs"), "TVRM0001", "abc123", nil)
	if !ok {
		t.Fatalf("Match: not found")
	}
	if got.ID != second.ID {
		t.Fatalf("Match returned %q, want the next waiver %q", got.ID, second.ID)
	}
}

func TestList_reportsSpendState(t *testing.T) {
	store, err := waiver.Open(filepath.Join(t.TempDir(), "waivers.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	w, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Spend(w, "abc123"); err != nil {
		t.Fatalf("Spend: %v", err)
	}

	entries := store.List()
	if len(entries) != 1 {
		t.Fatalf("List() has %d entries, want 1", len(entries))
	}
	got := entries[0]
	if got.ID != w.ID {
		t.Fatalf("Entry.ID = %q, want %q", got.ID, w.ID)
	}
	if got.SpentTree != "abc123" {
		t.Fatalf("Entry.SpentTree = %q, want %q", got.SpentTree, "abc123")
	}
	if got.SpentAt.IsZero() {
		t.Fatalf("Entry.SpentAt is zero, want non-zero")
	}
}

func TestOpen_readsExistingLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "waivers.jsonl")

	store, err := waiver.Open(logPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	w, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Spend(w, "abc123"); err != nil {
		t.Fatalf("Spend: %v", err)
	}

	reopened, err := waiver.Open(logPath)
	if err != nil {
		t.Fatalf("Open (reopen): %v", err)
	}

	entries := reopened.List()
	if len(entries) != 1 {
		t.Fatalf("List() has %d entries, want 1", len(entries))
	}
	if entries[0].ID != w.ID {
		t.Fatalf("Entry.ID = %q, want %q", entries[0].ID, w.ID)
	}
	if entries[0].SpentTree != "abc123" {
		t.Fatalf("Entry.SpentTree = %q, want %q", entries[0].SpentTree, "abc123")
	}
	if entries[0].SpentAt.IsZero() {
		t.Fatalf("Entry.SpentAt is zero, want non-zero")
	}

	if _, ok := reopened.Match("csharp", srcpath.FromSlash("src/OrderService.cs"), "TVRM0001", "def456", nil); ok {
		t.Fatalf("Match after reopen matched a different tree, the waiver should be used up")
	}
}

func TestOpen_malformedLine_namesLineNumber(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "waivers.jsonl")
	content := "{\"kind\":\"waiver\",\"id\":\"a\",\"language\":\"go\",\"path\":\"x.go\",\"rule\":\"R\",\"reason\":\"r\"}\n" +
		"not json\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := waiver.Open(logPath)
	if err == nil {
		t.Fatalf("Open(malformed line) = nil error, want an error")
	}
	var lineErr waiver.LineError
	if !errors.As(err, &lineErr) {
		t.Fatalf("Open error = %v, want a waiver.LineError", err)
	}
	if lineErr.Line != 2 {
		t.Fatalf("LineError.Line = %d, want 2", lineErr.Line)
	}
}

func TestOpen_spendForUnknownID_isError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "waivers.jsonl")
	content := "{\"kind\":\"spend\",\"id\":\"does-not-exist\",\"tree\":\"abc123\",\"spent\":\"2020-01-01T00:00:00Z\"}\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := waiver.Open(logPath)
	if err == nil {
		t.Fatalf("Open(spend for unknown id) = nil error, want an error")
	}
	var lineErr waiver.LineError
	if !errors.As(err, &lineErr) {
		t.Fatalf("Open error = %v, want a waiver.LineError", err)
	}
	if lineErr.Line != 1 {
		t.Fatalf("LineError.Line = %d, want 1", lineErr.Line)
	}
}

func TestRecord_createsLogPrivate(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "waivers.jsonl")
	store, err := waiver.Open(logPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("waiver log mode = %o, want %o", got, 0o600)
	}
}

func fileLineCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return len(bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n")))
}

// TestLogOmitsTimesARecordDoesNotHave reads the log as the audit artifact it
// is. A time.Time is a struct, so encoding/json's omitempty never fires on
// one, and a value field would write "0001-01-01T00:00:00Z" as the spend time
// of every unspent waiver and the recorded time of every spend. A reader
// cannot tell that from a real timestamp, and the file is append-only, so a
// record written in the wrong shape stays in the wrong shape.
func TestLogOmitsTimesARecordDoesNotHave(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "waivers.jsonl")
	store, err := waiver.Open(logPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	w, err := store.Record(waiver.Waiver{
		Language: "csharp",
		Path:     srcpath.FromSlash("src/OrderService.cs"),
		Rule:     "TVRM0001",
		Reason:   "analyzer misreads the builder chain",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Spend(w, "tree-sha"); err != nil {
		t.Fatalf("Spend: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading %s: %v", logPath, err)
	}
	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("log holds %d lines, want 2", len(lines))
	}

	for i, want := range []struct {
		kind    string
		present []string
		absent  []string
	}{
		{kind: "waiver", present: []string{"recorded"}, absent: []string{"spent", "tree"}},
		{kind: "spend", present: []string{"spent", "tree"}, absent: []string{"recorded"}},
	} {
		var record map[string]any
		if err := json.Unmarshal(lines[i], &record); err != nil {
			t.Fatalf("line %d is not JSON: %v", i+1, err)
		}
		if record["kind"] != want.kind {
			t.Fatalf("line %d kind = %v, want %s", i+1, record["kind"], want.kind)
		}
		for _, field := range want.present {
			if _, ok := record[field]; !ok {
				t.Errorf("line %d (%s) omits %q, which it is the record of", i+1, want.kind, field)
			}
		}
		for _, field := range want.absent {
			if got, ok := record[field]; ok {
				t.Errorf("line %d (%s) carries %q = %v, want the field absent. A zero time here reads as a real timestamp from the year one", i+1, want.kind, field, got)
			}
		}
	}
}
