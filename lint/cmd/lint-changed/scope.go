package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/tvrmsmith/coding-standards/internal/gitscope"
	"github.com/tvrmsmith/coding-standards/internal/srcpath"
	"github.com/tvrmsmith/coding-standards/lint/internal/lintfind"
)

// scopeSet answers whether a lintfind.Location falls inside the lines a
// commit changed.
type scopeSet interface {
	touches(loc lintfind.Location) bool
}

// lineScope is the ordinary case: a location is in scope when its span,
// inclusive of both ends, contains at least one line TouchedLines reported
// for its path.
type lineScope struct {
	touched map[srcpath.Path][]int
}

func (s lineScope) touches(loc lintfind.Location) bool {
	for _, line := range s.touched[loc.Path] {
		if line >= loc.StartLine && line <= loc.EndLine {
			return true
		}
	}
	return false
}

// fileScope is --files: every line of a named file is in scope, so a
// location is in scope whenever its path was one of the names given.
type fileScope struct {
	files map[srcpath.Path]bool
}

func (s fileScope) touches(loc lintfind.Location) bool {
	return s.files[loc.Path]
}

// divergentError is a --staged run finding a staged file whose working-tree
// copy differs from what is staged. Any divergence is a hard stop: the file
// was compiled from disk, so its content may not be what the diff's line
// numbers describe.
type divergentError struct{ paths []srcpath.Path }

func (e *divergentError) Error() string {
	msg := "staged file(s) differ from the index and must be re-staged: "
	for i, p := range e.paths {
		if i > 0 {
			msg += ", "
		}
		msg += string(p)
	}
	return msg
}

// resolveScope builds the scopeSet a filter run checks findings against.
// --files never asks git for a base at all, which is the point of the flag:
// a caller with no git base can still run.
func resolveScope(repo gitscope.Repo, fa FilterArgs) (scopeSet, error) {
	if fa.Mode == ScopeFiles {
		// Placed through the root rather than cast, because a location's path
		// is always the canonical repo-relative one and an absolute or
		// dot-prefixed --files entry would otherwise match nothing and drop
		// every finding silently. NamedFiles refuses a name it cannot place,
		// which is how an unresolvable path fails the run instead.
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("reading the working directory: %w", err)
		}
		named, err := repo.Root().NamedFiles(fa.Files, cwd)
		if err != nil {
			return nil, err
		}
		files := make(map[srcpath.Path]bool, len(named))
		for _, path := range named {
			files[path] = true
		}
		return fileScope{files: files}, nil
	}

	base, err := resolveBase(repo, fa)
	if err != nil {
		return nil, err
	}
	touched, err := repo.TouchedLines(base)
	if err != nil {
		return nil, err
	}
	if fa.Mode == ScopeStaged {
		if err := checkDivergence(repo, base); err != nil {
			return nil, err
		}
	}
	return lineScope{touched: touched}, nil
}

func resolveBase(repo gitscope.Repo, fa FilterArgs) (gitscope.Base, error) {
	if fa.Mode == ScopeSince {
		return repo.ResolveRef(fa.Ref)
	}
	return repo.ResolveStaged()
}

// checkDivergence asks about every staged path, not only the paths the report
// mentions and not only the ones TouchedLines returned: a staged-and-dirty
// file with no findings still got compiled from disk, so its divergence
// invalidates the whole run. TouchedLines is the wrong source for the list
// because it diffs with -w and --diff-filter=ACM and then drops pure moves, so
// a staged deletion, a pure rename and a whitespace-only edit all have no key
// in it and would go unchecked.
func checkDivergence(repo gitscope.Repo, base gitscope.Base) error {
	paths, err := stagedPaths(repo.Root(), base)
	if err != nil {
		return err
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i] < paths[j] })

	divergent, err := repo.DivergentFromIndex(paths)
	if err != nil {
		return err
	}
	if len(divergent) > 0 {
		return &divergentError{paths: divergent}
	}
	return nil
}

// stagedPaths is every path the index changes against base, named the way git
// names it. No -w and no --diff-filter, since the question here is which files
// the commit carries at all rather than which lines it wrote.
func stagedPaths(root srcpath.Root, base gitscope.Base) ([]srcpath.Path, error) {
	//nolint:gosec // G204: every argument but the last is a constant, and
	// base.Commit is a full sha gitscope resolved with rev-parse, so no
	// caller-supplied text reaches the argv.
	cmd := exec.Command("git", "diff", "--cached", "--name-only", "-z", base.Commit)
	cmd.Dir = root.Dir()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing the staged paths: %w", err)
	}
	var paths []srcpath.Path
	for _, name := range strings.Split(string(out), "\x00") {
		if name == "" {
			continue
		}
		paths = append(paths, srcpath.FromSlash(name))
	}
	return paths, nil
}
