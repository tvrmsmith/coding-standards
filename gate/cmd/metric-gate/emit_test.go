package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/crap"
	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/scope"
)

// passingDocument is a document that renders and would exit 0 against a
// healthy writer, so every case below is exercising emit's own handling of
// the write rather than a cause the document already carried.
func passingDocument() report.Document {
	return report.Document{Scope: scope.ModeMergeBase, Metric: &report.Metric{
		Name: crap.Name, Display: crap.DisplayName, Threshold: crap.Threshold,
	}}
}

// exceedingDocument is a document that renders and carries a score over the
// threshold, so its own exit code is 2 rather than 0. It is what pins emit
// passing doc.ExitCode() through instead of returning a constant that happens
// to match the passing case.
func exceedingDocument() report.Document {
	measurement := crap.Measurement{Complexity: crap.Threshold + 10, Coverage: 0.1}
	score := measurement.Score()
	return report.Document{Scope: scope.ModeMergeBase, ChangedMethods: 1, Metric: &report.Metric{
		Name: crap.Name, Display: crap.DisplayName, Threshold: crap.Threshold,
		Rows: []report.Row{{
			File: "src/Ordering/OrderService.cs", Start: 1, End: 20, Name: "Place",
			Complexity: measurement.Complexity, Coverage: &measurement.Coverage, Score: &score,
			State: report.StateMeasured, Action: measurement.Action(),
			TargetCoverage: measurement.TargetCoverage(),
		}},
	}}
}

// erroringWriter always fails, which black-box coverage cannot force onto
// os.Stdout without a real broken descriptor.
type erroringWriter struct{ err error }

func (w erroringWriter) Write(p []byte) (int, error) { return 0, w.err }

// shortWriter reports success for one byte fewer than it was handed, the
// case a broken io.Writer produces without ever returning an error, which is
// exactly the shape emit is not allowed to trust.
type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestEmitReturnsExitOneWhenStdoutWriteFails(t *testing.T) {
	cause := errors.New("no space left on device")
	code, err := emit(erroringWriter{err: cause}, &bytes.Buffer{}, passingDocument())

	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), cause.Error()) {
		t.Errorf("err = %v, want it to mention %q", err, cause)
	}
}

func TestEmitReturnsExitOneOnAShortWriteThatReportsNoError(t *testing.T) {
	doc := passingDocument()
	body, err := doc.Stdout()
	if err != nil {
		t.Fatalf("doc.Stdout(): %v", err)
	}

	code, err := emit(shortWriter{}, &bytes.Buffer{}, doc)

	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	// The counts are asserted, not just a non-nil error, so a render failure
	// or any other exit-1 cause cannot satisfy this case in the short write's
	// place.
	want := fmt.Sprintf("wrote %d of %d bytes", len(body)-1, len(body))
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to report %q", err, want)
	}
}

func TestEmitKeepsTheDocumentsCodeWhenOnlyTheStderrSummaryFails(t *testing.T) {
	doc := exceedingDocument()
	wantStdout, err := doc.Stdout()
	if err != nil {
		t.Fatalf("doc.Stdout(): %v", err)
	}

	var stdout bytes.Buffer
	code, err := emit(&stdout, erroringWriter{err: errors.New("closed")}, doc)

	// The document reached the caller before the summary failed, so the
	// summary going nowhere is dropped rather than becoming a second exit-1
	// cause, and the document's own 2 is still what comes back. A document
	// that exits 2 is what separates that from returning a constant.
	if code != 2 {
		t.Errorf("code = %d, want 2", code)
	}
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if stdout.String() != string(wantStdout) {
		t.Errorf("stdout = %q, want %q", stdout.String(), wantStdout)
	}
}

func TestEmitWritesTheDocumentAndSummaryOnAHealthyWriter(t *testing.T) {
	doc := passingDocument()
	wantStdout, err := doc.Stdout()
	if err != nil {
		t.Fatalf("doc.Stdout(): %v", err)
	}

	var stdout, stderr bytes.Buffer
	code, err := emit(&stdout, &stderr, doc)

	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	// 0 and the summary line are written out rather than read back off doc,
	// so the case still fails if emit starts sourcing either from somewhere
	// else.
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}
	const wantStderr = "no changed methods, nothing to measure\n"
	if stdout.String() != string(wantStdout) {
		t.Errorf("stdout = %q, want %q", stdout.String(), wantStdout)
	}
	if stderr.String() != wantStderr {
		t.Errorf("stderr = %q, want %q", stderr.String(), wantStderr)
	}
}
