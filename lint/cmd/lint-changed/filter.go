package main

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/tvrmsmith/coding-standards/internal/gitscope"
	"github.com/tvrmsmith/coding-standards/internal/srcpath"
	"github.com/tvrmsmith/coding-standards/lint/internal/lintfind"
	"github.com/tvrmsmith/coding-standards/lint/internal/waiver"
)

// survivor is a finding that outlasted diff scoping, with the locations
// that justify it. An IgnoresScope finding carries every location it has,
// since nothing scoped it: it survives regardless of the diff.
type survivor struct {
	finding   lintfind.Finding
	locations []lintfind.Location
}

// scopeFinding checks one finding against scope, returning the survivor and
// true when at least one of its locations, primary or related alike since
// lintfind.ParseSARIF already merges the two, is in scope.
func scopeFinding(f lintfind.Finding, scope scopeSet) (survivor, bool) {
	if f.IgnoresScope {
		return survivor{finding: f, locations: f.Locations}, true
	}
	var locs []lintfind.Location
	for _, loc := range f.Locations {
		if scope.touches(loc) {
			locs = append(locs, loc)
		}
	}
	if len(locs) == 0 {
		return survivor{}, false
	}
	return survivor{finding: f, locations: locs}, true
}

// runFilter is the blocking form: it reads a report off stdin, scopes it to
// the diff, lets a waiver suppress a survivor, and reports the rest. Exit 0
// is nothing survived, 1 is the tool breaking, 2 is a finding surviving.
func runFilter(fa FilterArgs, stdin io.Reader, stdout, stderr io.Writer) int {
	repo, err := gitscope.Open()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	scope, err := resolveScope(repo, fa)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	findings, dropped, err := lintfind.ParseSARIF(stdin, repo.Root())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	var survivors []survivor
	for _, f := range findings {
		if s, ok := scopeFinding(f, scope); ok {
			survivors = append(survivors, s)
		}
	}

	store, err := waiver.Open(waiverLogPath())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	tree, err := writeTree(repo.Root())
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	kept := make([]survivor, 0, len(survivors))
	for _, s := range survivors {
		path := s.locations[0].Path
		w, ok := store.Match(fa.Language, path, s.finding.Rule, tree)
		if !ok {
			kept = append(kept, s)
			continue
		}
		if err := store.Spend(w, tree); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stderr, "waiver %s suppressed %s on %s\n", w.ID, s.finding.Rule, path)
	}

	if dropped > 0 {
		fmt.Fprintf(stderr, "%d result(s) could not be placed inside the repo and were dropped\n", dropped)
	}

	if len(kept) == 0 {
		return 0
	}

	for _, s := range kept {
		fmt.Fprintf(stdout, "%s: %s\n", s.finding.Rule, s.finding.Message)
		for _, loc := range s.locations {
			fmt.Fprintf(stdout, "  %s:%d\n", loc.Path, loc.StartLine)
		}
		fmt.Fprintf(stdout, "  lint-changed waive --language %s --path %s --rule %s --reason \"<why>\"\n\n",
			fa.Language, s.locations[0].Path, s.finding.Rule)
	}
	return 2
}

// writeTree is the index tree sha every waiver is matched and spent
// against, so a waiver recorded against one staged state never silently
// covers another.
func writeTree(root srcpath.Root) (string, error) {
	cmd := exec.Command("git", "write-tree")
	cmd.Dir = root.Dir()
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("computing the index tree: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
