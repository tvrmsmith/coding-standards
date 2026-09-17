package main

import (
	"os"
	"syscall"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/scope"
)

// TestMeasureFailsInsideTheDocumentWhenTheWorkingDirectoryCannotBeRead pins
// working_directory_unreadable (issue 31). The black-box suite cannot reach
// this: anything that breaks os.Getwd has already made git rev-parse fail in
// the same directory, since gitscope.Open shells out to git in that same
// directory. So the seam is the only way to drive it, a stub getwd standing
// in for the real read the way emit_test.go stubs a broken stdout.
//
// The stub answers in the shape os.Getwd answers in, an *os.SyscallError over
// the errno, so the golden is the document a real refused getwd produces
// rather than one only a stub can make.
func TestMeasureFailsInsideTheDocumentWhenTheWorkingDirectoryCannotBeRead(t *testing.T) {
	getwd := func() (string, error) { return "", os.NewSyscallError("getwd", syscall.EACCES) }

	doc, err := measure(scope.Scope{Mode: scope.ModeMergeBase}, getwd)
	if err != nil {
		t.Fatalf("measure: %v", err)
	}

	assertDocumentMatches(t, doc, "working_directory_unreadable",
		"could not read the process working directory: permission denied\n")
}

// assertDocumentMatches holds a typed exit-1 document to its golden, its exit
// code and its stderr line. The golden is named rather than the code, since
// the golden is what carries both.
func assertDocumentMatches(t *testing.T, doc report.Document, golden, stderr string) {
	t.Helper()
	body, err := doc.Stdout()
	if err != nil {
		t.Fatalf("doc.Stdout(): %v", err)
	}
	if want := readGolden(t, golden); string(body) != want {
		t.Errorf("stdout =\n%s\nwant\n%s", body, want)
	}
	if doc.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1", doc.ExitCode())
	}
	if doc.Stderr() != stderr {
		t.Errorf("Stderr() = %q, want %q", doc.Stderr(), stderr)
	}
}

// readGolden loads a golden document from gate/test/golden, byte for byte.
// cmd/metric-gate hosts no goldens of its own; this reaches into the
// black-box suite's so the two exit-1 seams pin the same file rather than
// each carrying a private copy.
func readGolden(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile("../../test/golden/" + name + ".toon")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
