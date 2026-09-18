package main

import (
	"sort"

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
		files := make(map[srcpath.Path]bool, len(fa.Files))
		for _, name := range fa.Files {
			files[srcpath.FromSlash(name)] = true
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
		if err := checkDivergence(repo, touched); err != nil {
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

// checkDivergence asks about every path TouchedLines named, not only the
// paths the report mentions: a staged-and-dirty file with no findings still
// got compiled from disk, so its divergence invalidates the whole run.
func checkDivergence(repo gitscope.Repo, touched map[srcpath.Path][]int) error {
	paths := make([]srcpath.Path, 0, len(touched))
	for p := range touched {
		paths = append(paths, p)
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
