package extract

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// onPath locates a stand-in extractor, because the suite runs on both macOS
// and Linux and the two disagree about where cat and false live.
func onPath(t *testing.T, name string) string {
	t.Helper()
	full, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("locating %s: %v", name, err)
	}
	return full
}

// TestInvokeHandsOnePathPerLineOnStdin pins the wire the extractor protocol
// rests on: invoke spawns the located binary in the repo root and feeds it the
// file list, one path per line, then returns that binary's stdout verbatim.
// /bin/cat stands in for an extractor so the case asserts the stream itself
// rather than any one language's parsing of it.
func TestInvokeHandsOnePathPerLineOnStdin(t *testing.T) {
	dir := t.TempDir()
	root, err := srcpath.NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := extractor{language: "stub", binary: onPath(t, "cat"), root: root}

	out, err := e.invoke([]srcpath.Path{"src/a.cs", "src/b.cs"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	if want := "src/a.cs\nsrc/b.cs\n"; string(out) != want {
		t.Errorf("stdin the extractor saw = %q, want %q", out, want)
	}
}

// TestExecReportsANonZeroExitAsAnExtractorFailure pins the other half of the
// same call: a binary that exits non-zero becomes the extractor-failed refusal
// naming the language, not a bare os error the caller would have to translate.
func TestExecReportsANonZeroExitAsAnExtractorFailure(t *testing.T) {
	dir := t.TempDir()
	root, err := srcpath.NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := extractor{language: "stub", binary: onPath(t, "false"), root: root}

	out, err := e.exec(nil)
	if err == nil {
		t.Fatalf("exec returned %q and no error, want an extractor failure", out)
	}
	var failure *report.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %v (%T), want a *report.Failure, which is what carries a refusal code to the caller and into the rendered document", err, err)
	}
	if failure.Code != report.CodeExtractorFailed {
		t.Errorf("refusal code = %q, want %q", failure.Code, report.CodeExtractorFailed)
	}
	if want := "stub extractor exited 1"; failure.Message != want {
		t.Errorf("message = %q, want %q", failure.Message, want)
	}
}
