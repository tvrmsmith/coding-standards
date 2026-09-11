package main

import (
	"bytes"
	"errors"
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
	code, err := emit(shortWriter{}, &bytes.Buffer{}, passingDocument())

	if code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if err == nil {
		t.Error("err = nil, want a short-write error")
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
