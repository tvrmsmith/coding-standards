package main

import (
	"os"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/scope"
)

// TestMeasureFailsInsideTheDocumentWhenTheWorkingDirectoryCannotBeRead pins
// working_directory_unreadable (issue 31). The black-box suite cannot reach
// this: anything that breaks os.Getwd has already made git rev-parse fail in
// the same directory, since gitscope.Open shells out to git in that same
// directory. So the seam is the only way to drive it, a stub getwd standing
// in for the real read the way emit_test.go stubs a broken stdout.
func TestMeasureFailsInsideTheDocumentWhenTheWorkingDirectoryCannotBeRead(t *testing.T) {
	getwd := func() (string, error) { return "", os.ErrPermission }

	doc, err := measure(scope.Scope{Mode: scope.ModeMergeBase}, getwd)
	if err != nil {
		t.Fatalf("measure: %v", err)
	}

	body, renderErr := doc.Stdout()
	if renderErr != nil {
		t.Fatalf("doc.Stdout(): %v", renderErr)
	}
	want := readGolden(t, "working_directory_unreadable")
	if string(body) != want {
		t.Errorf("stdout =\n%s\nwant\n%s", body, want)
	}
	if doc.ExitCode() != 1 {
		t.Errorf("ExitCode() = %d, want 1", doc.ExitCode())
	}
	const wantStderr = "could not read the working directory that --files and --coverage paths resolve against: permission denied\n"
	if doc.Stderr() != wantStderr {
		t.Errorf("Stderr() = %q, want %q", doc.Stderr(), wantStderr)
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
