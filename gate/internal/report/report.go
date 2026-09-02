// Package report assembles the one TOON document metric-gate writes on
// stdout (ADR 0005), the single summary line it writes on stderr, and the
// exit code that goes with them. The field list is fixed and never varies
// with the outcome.
package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
	"github.com/tvrmsmith/coding-standards/gate/internal/toon"
)

// Version is the gate's own version, pinned in the document header beside
// the spec version so a reader knows what produced the document.
const Version = "0.1.0"

// Scope is the only diff scope this gate has. ADR 0005 makes it a document
// field; the flags that would vary it are other issues' work.
const Scope = "merge-base"

// The typed error codes for the exit-1 causes this gate can reach. An agent
// branches on these rather than parsing the message.
const (
	CodeNoDiffBase                    = "no_diff_base"
	CodeExtractorFailed               = "extractor_failed"
	CodeExtractorPathMismatch         = "extractor_path_mismatch"
	CodeExtractorCapabilitiesMismatch = "extractor_capabilities_mismatch"
	CodeExtractorDuplicateSpan        = "extractor_duplicate_span"
	CodeParseFailed                   = "parse_failed"
	CodeCoverageMissing               = "coverage_missing"
	CodeCoverageUnparseable           = "coverage_unparseable"
	CodeUnknownChangedMethod          = "unknown_changed_method"
)

// ReasonFileUnmatched is the typed reason on an unknown row: the changed
// method's file matched no report path. ADR 0004 narrows it to that one
// meaning.
const ReasonFileUnmatched = "file_unmatched"

// Failure is one typed exit-1 cause: the `error` block of the document, and
// the stderr line when there is no table.
type Failure struct {
	Code    string
	Message string
}

func (f Failure) Error() string { return f.Message }

// State is what the join could establish about one method span.
type State string

const (
	StateMeasured     State = "measured"
	StateStructuralNA State = "structural_na"
	StateUnknown      State = "unknown"
)

// Row is one changed method as the document reports it.
type Row struct {
	File           srcpath.Path
	Start          int
	End            int
	Name           string
	Complexity     int
	Coverage       *float64
	Score          *float64
	State          State
	Action         string
	TargetCoverage *float64
	Reason         string
}

// cells renders the row in the fixed column order. A nil pointer and an
// empty reason both become the bare `null` token, never the string "null".
func (r Row) cells() []any {
	return []any{
		r.File.String(), r.Start, r.End, r.Name, r.Complexity,
		nullable(r.Coverage), nullable(r.Score), string(r.State),
		r.Action, nullable(r.TargetCoverage), reasonCell(r.Reason),
	}
}

// Metric is one metric's threshold and its whole changed-method set, failures
// and passes alike.
type Metric struct {
	// Name is the document key the metric's table appears under.
	Name string
	// Display is how the metric is named in the stderr line.
	Display   string
	Threshold int
	Rows      []Row
}

// orderedRows sorts by descending score, then file, then start line, with the
// rows that carry no score last.
func (m Metric) orderedRows() []Row {
	ordered := append([]Row{}, m.Rows...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if (left.Score == nil) != (right.Score == nil) {
			return right.Score == nil
		}
		if left.Score != nil && *left.Score != *right.Score {
			return *left.Score > *right.Score
		}
		if left.File != right.File {
			return left.File < right.File
		}
		return left.Start < right.Start
	})
	return ordered
}

// Measured counts the rows the join could attribute, which is every row whose
// state is not unknown. A structural_na row counts as measured.
func (m Metric) Measured() int {
	measured := 0
	for _, row := range m.Rows {
		if row.State != StateUnknown {
			measured++
		}
	}
	return measured
}

// Failed counts the rows whose score exceeds the threshold.
func (m Metric) Failed() int {
	failed := 0
	for _, row := range m.Rows {
		if row.Score != nil && *row.Score > float64(m.Threshold) {
			failed++
		}
	}
	return failed
}

// WorstScore is the highest score among the scored rows, and zero when
// nothing scored.
func (m Metric) WorstScore() float64 {
	worst := 0.0
	for _, row := range m.Rows {
		if row.Score != nil && *row.Score > worst {
			worst = *row.Score
		}
	}
	return worst
}

// Document is one whole gate run's output.
type Document struct {
	// Base is the resolved "<ref>@<sha>" label, or nil when resolution is
	// what failed.
	Base                     *string
	ChangedMethods           int
	TouchedLinesOutsideSpans int
	SkippedPaths             []string
	// Failure is the typed exit-1 cause, or nil.
	Failure *Failure
	// Metric is present exactly when the join ran.
	Metric *Metric
}

// Status is the document's verdict: error when a typed cause fired, fail when
// a scored method is over threshold, pass otherwise.
func (d Document) Status() string {
	switch {
	case d.Failure != nil:
		return "error"
	case d.Metric != nil && d.Metric.Failed() > 0:
		return "fail"
	default:
		return "pass"
	}
}

// ExitCode is 0 pass, 1 tool error, 2 threshold exceeded. There is no fourth
// code.
func (d Document) ExitCode() int {
	switch d.Status() {
	case "error":
		return 1
	case "fail":
		return 2
	default:
		return 0
	}
}

// Stderr is the single summary line a human reads, so nobody is asked to
// parse the machine document. A run that scored methods reports the counts; a
// run that failed before scoring reports its message verbatim; the one cause
// that fails with a table reports both.
func (d Document) Stderr() string {
	var b strings.Builder
	if d.Failure != nil {
		b.WriteString(d.Failure.Message + "\n")
	}
	if d.Metric == nil {
		return b.String()
	}
	if d.ChangedMethods == 0 {
		b.WriteString("no changed methods, nothing to measure\n")
		return b.String()
	}
	fmt.Fprintf(&b, "%d of %d changed methods over %s threshold %d, worst score %.2f\n",
		d.Metric.Failed(), d.ChangedMethods, d.Metric.Display, d.Metric.Threshold, d.Metric.WorstScore())
	return b.String()
}

// Stdout renders the document as TOON.
func (d Document) Stdout() ([]byte, error) {
	fields := []toon.Field{
		{Key: "status", Value: d.Status()},
		{Key: "tool", Value: "metric-gate/" + Version},
		{Key: "spec", Value: "toon/" + toon.SpecVersion},
		{Key: "scope", Value: Scope},
		{Key: "base", Value: nullable(d.Base)},
		{Key: "changed_methods", Value: d.ChangedMethods},
		{Key: "touched_lines_outside_spans", Value: d.TouchedLinesOutsideSpans},
		{Key: "skipped_paths", Value: append([]string{}, d.SkippedPaths...)},
	}
	if d.Failure != nil {
		fields = append(fields, toon.Field{Key: "error", Value: &toon.Doc{Fields: []toon.Field{
			{Key: "code", Value: d.Failure.Code},
			{Key: "message", Value: d.Failure.Message},
		}}})
	}
	if d.Metric != nil {
		fields = append(fields,
			toon.Field{Key: "metrics", Value: metricsTable(*d.Metric)},
			toon.Field{Key: d.Metric.Name, Value: rowsTable(d.Metric.orderedRows())},
		)
	}
	return toon.Doc{Fields: fields}.Encode()
}

// metricsTable is the one-row summary of each metric the run computed.
func metricsTable(m Metric) *toon.Table {
	return &toon.Table{
		Columns: []toon.Column{{Name: "name"}, {Name: "threshold"}, {Name: "measured"}, {Name: "failed"}},
		Rows:    [][]any{{m.Name, m.Threshold, m.Measured(), m.Failed()}},
	}
}

// rowsTable is the changed-method table. Precision is a rounding rule, so the
// cells carry raw float64 values and the encoder renders them canonically.
func rowsTable(rows []Row) *toon.Table {
	table := &toon.Table{Columns: []toon.Column{
		{Name: "file"}, {Name: "start"}, {Name: "end"}, {Name: "name"},
		{Name: "complexity"}, {Name: "coverage", Precision: 3}, {Name: "score", Precision: 2},
		{Name: "state"}, {Name: "action"}, {Name: "target_coverage", Precision: 3}, {Name: "reason"},
	}}
	for _, row := range rows {
		table.Rows = append(table.Rows, row.cells())
	}
	return table
}

// nullable unwraps an optional value into a cell, so an absent one renders as
// the bare `null` token rather than the string "null".
func nullable[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// reasonCell renders the typed reason, where no reason is `null`.
func reasonCell(reason string) any {
	if reason == "" {
		return nil
	}
	return reason
}
