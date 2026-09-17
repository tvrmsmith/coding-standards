package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/scope"
)

// The two gitscope.Open failures the black-box suite cannot stage. Running the
// gate outside a repository is the third and gate/test drives that one against
// a real git; these two need git itself replaced, which the built binary's own
// PATH is the only place to do. Each drives measure the way the run does and
// pins the whole document, so the code a caller branches on and the sentence a
// developer reads are both held.

func TestMeasureReportsAMissingGitBinaryInsideTheDocument(t *testing.T) {
	t.Setenv("PATH", "")

	doc, err := measure(scope.Scope{Mode: scope.ModeMergeBase}, func() (string, error) { return t.TempDir(), nil })
	if err != nil {
		t.Fatalf("measure: %v", err)
	}

	assertDocumentMatches(t, doc, "git_unavailable",
		"could not run git: git rev-parse --show-toplevel: exec: \"git\": executable file not found in $PATH\n")
}

func TestMeasureReportsARepoRootItCannotResolveInsideTheDocument(t *testing.T) {
	// A git that names a toplevel which is not on disk. Real git cannot be made
	// to do it, since it prints the physical directory it is already running
	// in, and the arm still has to answer for the filesystem losing the root
	// between the two syscalls.
	bin := t.TempDir()
	script := "#!/bin/sh\necho " + filepath.Join(t.TempDir(), "gone") + "\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	doc, err := measure(scope.Scope{Mode: scope.ModeMergeBase}, func() (string, error) { return t.TempDir(), nil })
	if err != nil {
		t.Fatalf("measure: %v", err)
	}

	assertDocumentMatches(t, doc, "repo_root_unresolvable",
		"could not resolve the repo root git named: no such file or directory\n")
}
