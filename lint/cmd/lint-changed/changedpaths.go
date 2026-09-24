package main

import (
	"fmt"
	"io"

	"github.com/tvrmsmith/coding-standards/internal/gitscope"
	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// runChangedPaths prints every file a run under scope covers, each followed by
// a NUL, since git quotes a path holding a space or a non-ASCII byte in its
// newline-delimited output. Exit 0 is the list, however short, and 1 is the
// list failing to come back. The dispatcher exits 1 on that 1 rather than
// reading an empty stdout as nothing changed, which is how a bad --since ref
// would otherwise pass as a clean run.
func runChangedPaths(scope Scope, stdout, stderr io.Writer) int {
	paths, err := changedPaths(scope)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	for _, path := range paths {
		if _, err := fmt.Fprint(stdout, path, "\x00"); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	return 0
}

// changedPaths passes --files through as given rather than placing each name
// the way the filter does. A path in the change but not on disk is the
// language branch's question, since the C# branch stops a commit over one.
func changedPaths(scope Scope) ([]string, error) {
	if scope.Mode == ScopeFiles {
		return scope.Files, nil
	}
	// OpenHook for the reason runFilter gives: under a pre-commit hook the index
	// git names is the one the commit will write.
	repo, err := gitscope.OpenHook()
	if err != nil {
		return nil, err
	}
	var files []srcpath.Path
	if scope.Mode == ScopeSince {
		files, err = repo.FilesSince(scope.Ref)
	} else {
		files, err = repo.StagedFiles()
	}
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.String())
	}
	return paths, nil
}
