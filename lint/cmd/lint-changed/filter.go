package main

import (
	"errors"
	"fmt"
	"io"
	"os"

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

	findings, dropped, err := readReports(fa, stdin, repo.Root())
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
	trees := &indexTree{repo: repo}

	kept := make([]survivor, 0, len(survivors))
	var matched []matchedWaiver
	claimed := map[string]bool{}
	// An empty log can match nothing, so the index tree is never computed on the
	// overwhelmingly common run that has no waiver recorded at all.
	waivable := len(store.List()) > 0
	for _, s := range survivors {
		if !waivable {
			kept = append(kept, s)
			continue
		}
		tree, err := trees.sha()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		path := s.waivePath()
		w, ok := store.Match(fa.Language, path, s.finding.Rule, tree, claimed)
		if !ok {
			kept = append(kept, s)
			continue
		}
		claimed[w.ID] = true
		matched = append(matched, matchedWaiver{waiver: w, rule: s.finding.Rule, path: path})
	}

	if len(dropped) > 0 {
		_, _ = fmt.Fprintf(stderr, "%d result(s) could not be placed inside the repo and were dropped\n", len(dropped))
	}

	if len(kept) > 0 {
		for _, m := range matched {
			_, _ = fmt.Fprintf(stderr, "waiver %s matched %s on %s but was not spent: the run blocked on another finding\n",
				m.waiver.ID, m.rule, m.path)
		}
		reportKept(stdout, waiveBinary(), fa.Language, kept)
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

// waivePath is the path a waiver for this survivor names, empty when the
// finding has no location. Roslyn reports an analyzer that failed to load at
// Location.None, and a waiver keyed on language and rule alone is the route out
// of one, so an empty path is a value the store matches rather than a refusal.
func (s survivor) waivePath() srcpath.Path {
	if len(s.locations) == 0 {
		return ""
	}
	return s.locations[0].Path
}

// reportKept prints every finding that blocked the commit, with the command
// that would waive it.
func reportKept(stdout io.Writer, bin, language string, kept []survivor) {
	for _, s := range kept {
		_, _ = fmt.Fprintf(stdout, "%s: %s\n", s.finding.Rule, s.finding.Message)
		for _, loc := range s.locations {
			_, _ = fmt.Fprintf(stdout, "  %s:%d\n", loc.Path, loc.StartLine)
		}
		_, _ = fmt.Fprintf(stdout, "  %s waive --language %s%s --rule %s --reason \"<why>\"\n\n",
			bin, language, pathFlag(s.waivePath()), s.finding.Rule)
	}
}

// pathFlag is the --path the printed command carries, or nothing at all for a
// finding with no location to name.
func pathFlag(path srcpath.Path) string {
	if path == "" {
		return ""
	}
	return " --path " + string(path)
}

// waiveBinary is the absolute path of the running binary, so the command the
// report prints runs as printed. lint-changed is built into a cache directory
// and invoked by full path from the harness, never from PATH, so the bare name
// would name nothing.
func waiveBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return "lint-changed"
	}
	return exe
}

// indexTree is the index tree sha every waiver is matched and spent against,
// so a waiver recorded against one staged state never silently covers
// another. It is read on first use rather than up front: a run with no waiver
// to look up, including any --files run in a tree whose index git refuses to
// write, never needs it.
type indexTree struct {
	repo gitscope.Repo
	sum  string
}

func (t *indexTree) sha() (string, error) {
	if t.sum != "" {
		return t.sum, nil
	}
	sum, err := t.repo.WriteTree()
	if err != nil {
		return "", fmt.Errorf("computing the index tree: %w", err)
	}
	t.sum = sum
	return t.sum, nil
}

// readReports parses every --report the caller named, or stdin when it named
// none. One process reads them all so the whole commit's findings meet the
// waiver log once: a process per report would spend a waiver against findings
// the next process had not seen yet.
func readReports(fa FilterArgs, stdin io.Reader, root srcpath.Root) ([]lintfind.Finding, []lintfind.Dropped, error) {
	if len(fa.Reports) == 0 {
		findings, dropped, err := lintfind.ParseSARIF(stdin, root)
		if err != nil {
			return nil, nil, err
		}
		if err := checkPlaced("the report on stdin", findings, dropped); err != nil {
			return nil, nil, err
		}
		return dedup(findings), dropped, nil
	}
	var findings []lintfind.Finding
	var dropped []lintfind.Dropped
	for _, name := range fa.Reports {
		//nolint:gosec // G304: the report path is the caller's whole point. The
		// harness names the files its own build just wrote, in a directory it
		// created, so there is no trust line for a variable path to cross.
		f, err := os.Open(name)
		if err != nil {
			return nil, nil, fmt.Errorf("opening report %s: %w", name, err)
		}
		got, gotDropped, err := lintfind.ParseSARIF(f, root)
		_ = f.Close()
		if err != nil {
			return nil, nil, err
		}
		if err := checkPlaced(name, got, gotDropped); err != nil {
			return nil, nil, err
		}
		findings = append(findings, got...)
		dropped = append(dropped, gotDropped...)
	}
	return dedup(findings), dropped, nil
}

// checkPlaced refuses a report that placed nothing while dropping something.
// That report checked no code at all, and passing the commit on it would be the
// gate reporting clean on a report it never read. The question is asked of each
// report on its own, because a commit spanning two projects where only one
// resolves its URIs inside the repo would otherwise ride on the other's results.
// A report with no results at all is a genuine clean pass.
func checkPlaced(source string, findings []lintfind.Finding, dropped []lintfind.Dropped) error {
	if len(findings) > 0 || len(dropped) == 0 {
		return nil
	}
	msg := "every result in " + source + " fell outside the repo, so nothing was checked"
	for _, d := range dropped {
		msg += fmt.Sprintf("\n  %s at %s", d.Rule, d.URI)
	}
	return errors.New(msg)
}

// dedup keeps one copy of each distinct finding. A multi-targeted project
// compiles once per framework and writes one report per framework, and a .cs
// linked into two projects is compiled by both, so the same source-level warning
// arrives two or more times. The framework and the owning project are not part
// of the identity the gate judges, and a duplicate would print twice and demand
// a second waiver for a single line of code.
func dedup(findings []lintfind.Finding) []lintfind.Finding {
	seen := make(map[string]bool, len(findings))
	unique := make([]lintfind.Finding, 0, len(findings))
	for _, f := range findings {
		key := f.Rule + "\x00" + f.Message
		for _, loc := range f.Locations {
			key += fmt.Sprintf("\x00%s:%d-%d", loc.Path, loc.StartLine, loc.EndLine)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, f)
	}
	return unique
}
