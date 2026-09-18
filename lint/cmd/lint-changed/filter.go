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
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	scope, err := resolveScope(repo, fa)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	findings, dropped, err := lintfind.ParseSARIF(stdin, repo.Root())
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
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
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	trees := &indexTree{root: repo.Root()}

	kept := make([]survivor, 0, len(survivors))
	var matched []matchedWaiver
	claimed := map[string]bool{}
	for _, s := range survivors {
		path, waivable := s.waivablePath()
		if !waivable {
			kept = append(kept, s)
			continue
		}
		tree, err := trees.sha()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		w, ok := store.Match(fa.Language, path, s.finding.Rule, tree, claimed)
		if !ok {
			kept = append(kept, s)
			continue
		}
		claimed[w.ID] = true
		matched = append(matched, matchedWaiver{waiver: w, rule: s.finding.Rule, path: path})
	}

	if dropped > 0 {
		_, _ = fmt.Fprintf(stderr, "%d result(s) could not be placed inside the repo and were dropped\n", dropped)
	}

	if len(kept) > 0 {
		for _, m := range matched {
			_, _ = fmt.Fprintf(stderr, "waiver %s matched %s on %s but was not spent: the run blocked on another finding\n",
				m.waiver.ID, m.rule, m.path)
		}
		reportKept(stdout, fa.Language, kept)
		return 2
	}

	// The spends land only now, because a waiver's one use is one commit that
	// actually went through. Spending during the loop would burn a waiver on a
	// run that blocked anyway, and fixing the blocking finding changes the
	// index tree, so the burnt waiver would no longer match.
	for _, m := range matched {
		tree, err := trees.sha()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		if err := store.Spend(m.waiver, tree); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintf(stderr, "waiver %s suppressed %s on %s\n", m.waiver.ID, m.rule, m.path)
	}
	return 0
}

// matchedWaiver is a waiver that covered a survivor, held until the run knows
// whether anything else blocked it.
type matchedWaiver struct {
	waiver waiver.Waiver
	rule   string
	path   srcpath.Path
}

// waivablePath is the path a waiver for this survivor names. A finding that
// ignores scope can carry no locations at all, since Roslyn reports an
// analyzer that failed to load at Location.None: there is no path to waive it
// on, and the load failure is breakage to fix rather than a false positive.
func (s survivor) waivablePath() (srcpath.Path, bool) {
	if len(s.locations) == 0 {
		return "", false
	}
	return s.locations[0].Path, true
}

// reportKept prints every finding that blocked the commit, with the command
// that would waive it where one exists.
func reportKept(stdout io.Writer, language string, kept []survivor) {
	for _, s := range kept {
		_, _ = fmt.Fprintf(stdout, "%s: %s\n", s.finding.Rule, s.finding.Message)
		for _, loc := range s.locations {
			_, _ = fmt.Fprintf(stdout, "  %s:%d\n", loc.Path, loc.StartLine)
		}
		path, waivable := s.waivablePath()
		if !waivable {
			_, _ = fmt.Fprintf(stdout, "  no location, so no waiver can name it: fix the analyzer load failure\n\n")
			continue
		}
		_, _ = fmt.Fprintf(stdout, "  lint-changed waive --language %s --path %s --rule %s --reason \"<why>\"\n\n",
			language, path, s.finding.Rule)
	}
}

// indexTree is the index tree sha every waiver is matched and spent against,
// so a waiver recorded against one staged state never silently covers
// another. It is read on first use rather than up front: a run with no waiver
// to look up, including any --files run in a tree whose index git refuses to
// write, never needs it.
type indexTree struct {
	root srcpath.Root
	sum  string
}

func (t *indexTree) sha() (string, error) {
	if t.sum != "" {
		return t.sum, nil
	}
	cmd := exec.Command("git", "write-tree")
	cmd.Dir = t.root.Dir()
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("computing the index tree: %w", err)
	}
	t.sum = strings.TrimSpace(string(out))
	return t.sum, nil
}
