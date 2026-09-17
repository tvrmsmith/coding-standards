package main

import (
	"errors"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/extract"
	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// TestDatingAnUnreadableChangedFileIsTyped pins issue 31's half that dates the
// changed files against the coverage report: a file the stat cannot read comes
// back as a report.Failure coded changed_file_unreadable, so measure lands it
// in the document instead of exiting 1 with empty stdout. It also pins what
// cause strips, the operating system's words without the absolute path
// os.PathError carries, since ADR 0004 names the source path already.
func TestDatingAnUnreadableChangedFileIsTyped(t *testing.T) {
	dir := t.TempDir()
	root, err := srcpath.NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	const rel = "src/Ordering/OrderService.cs"
	changed := []extract.Span{{File: rel, Name: "OrderService.Cancel", StartLine: 1, EndLine: 1, Complexity: 1}}

	_, err = newestEdit(root, changed)

	var failure *report.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("newestEdit err = %v, want a *report.Failure", err)
	}
	if failure.Code != report.CodeChangedFileUnreadable {
		t.Errorf("Code = %q, want %q", failure.Code, report.CodeChangedFileUnreadable)
	}
	want := "could not read " + rel + " to date it against the coverage report: no such file or directory"
	if failure.Message != want {
		t.Errorf("Message = %q, want %q", failure.Message, want)
	}
}
