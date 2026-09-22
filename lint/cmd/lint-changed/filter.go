package main

import (
	"fmt"
	"io"
	"os"
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

// runFilter is the blocking form: it reads every --report named, scopes them
// to the diff, lets a waiver suppress a survivor, and reports the rest. Exit
// 0 is nothing survived, 1 is the tool breaking, 2 is a finding surviving.
//
// It never spends a matched waiver. Only the dispatcher that calls this
// filter across every language branch of one commit knows whether the whole
// commit went through, so spending is the separate KindSpend form.
func runFilter(fa FilterArgs, stdout, stderr io.Writer) int {
	// OpenHook rather than Open, because git runs this filter from a pre-commit
	// hook and the index it names there is the index the commit will write.
	repo, err := gitscope.OpenHook()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	scope, err := resolveScope(repo, fa)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	reports := readReports(fa, repo.Root())

	var survivors []survivor
	for _, f := range reports.findings {
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
		w, ok := store.Match(s.finding.Language, path, s.finding.Rule, tree, claimed)
		if !ok {
			kept = append(kept, s)
			continue
		}
		claimed[w.ID] = true
		matched = append(matched, matchedWaiver{waiver: w, rule: s.finding.Rule, path: path})
	}

	// Written unconditionally once matching has finished, whatever the
	// verdict below: the file records what matched, and the decision to
	// spend belongs to the dispatcher alone.
	if fa.MatchedWaivers != "" {
		if err := writeMatchedWaivers(fa.MatchedWaivers, matched); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if len(reports.dropped) > 0 {
		_, _ = fmt.Fprintf(stderr, "%d result(s) could not be placed inside the repo and were dropped\n", len(reports.dropped))
	}
	for _, msg := range reports.unchecked {
		_, _ = fmt.Fprintln(stderr, msg)
	}

	// Printed for every matched waiver on every verdict, not only when the
	// run blocks: the filter never spends, so it never knows why a waiver
	// went unspent, only that it matched.
	for _, m := range matched {
		_, _ = fmt.Fprintf(stderr, "waiver %s matched %s on %s\n", m.waiver.ID, m.rule, m.path)
	}

	if len(kept) > 0 {
		reportKept(stdout, stderr, waiveBinary(), kept)
		return 2
	}

	// A report that checked nothing is the tool breaking rather than a finding,
	// so it is answered only once no finding has blocked.
	if len(reports.unchecked) > 0 {
		return 1
	}

	return 0
}

// matchedWaiver is a waiver that covered a survivor.
type matchedWaiver struct {
	waiver waiver.Waiver
	rule   string
	path   srcpath.Path
}

// writeMatchedWaivers writes the id of every matched waiver, one per line, to
// path, truncating whatever was there. An empty matched list still writes an
// empty file: the caller asked to know what matched, and nothing matching is
// itself an answer.
func writeMatchedWaivers(path string, matched []matchedWaiver) error {
	var b strings.Builder
	for _, m := range matched {
		b.WriteString(m.waiver.ID)
		b.WriteByte('\n')
	}
	//nolint:gosec // G306: this file is read straight back by the dispatcher
	// that invoked this run, never executed, so 0644 costs nothing here.
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("writing matched waivers to %s: %w", path, err)
	}
	return nil
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

// reportKept prints stdout's one machine-readable line per surviving
// finding, at its first in-scope location, and nothing else. The consumer is
// no-mistakes' lint.extra_linters, which reads this stream with one regex
// across every language, so one line per finding and no second shape is what
// makes that one regex possible. The waive command that would clear each one
// goes to stderr instead, the stream everything that is not a finding
// already lives on.
func reportKept(stdout, stderr io.Writer, bin string, kept []survivor) {
	for _, s := range kept {
		loc := firstLocation(s)
		_, _ = fmt.Fprintf(stdout, "%s:%d:%d: %s: %s\n", loc.Path, loc.StartLine, loc.StartColumn, s.finding.Rule, flatten(s.finding.Message))
		_, _ = fmt.Fprintf(stderr, "%s waive --language %s%s --rule %s --reason \"<why>\"\n",
			bin, s.finding.Language, pathFlag(s.waivePath()), s.finding.Rule)
	}
}

// firstLocation is the location a survivor's porcelain line reports at: its
// first in-scope location, the one scopeFinding kept first, when it has one,
// or a placeholder when it has none at all. AD0001 and its kin are reported
// at Location.None, and stdout must never drop a finding for lack of
// somewhere to point at it.
func firstLocation(s survivor) lintfind.Location {
	if len(s.locations) == 0 {
		return lintfind.Location{Path: ".", StartLine: 1, StartColumn: 1}
	}
	return s.locations[0]
}

// flatten collapses a finding's message onto the one line the porcelain
// shape allows. golangci's typecheck linter genuinely emits a multi-line
// Text, and a message that spilled onto a second line would make that line
// unparseable as a finding of its own.
func flatten(msg string) string {
	return strings.Join(strings.Fields(msg), " ")
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

// parsedReports is every report one run read: the findings they placed, the
// results they dropped, and the reports that could not be read at all or
// described another tree entirely.
type parsedReports struct {
	findings  []lintfind.Finding
	dropped   []lintfind.Dropped
	unchecked []string
}

// readReports parses every --report the caller named, one process reading
// them all so a waiver is matched against the whole commit's findings at
// once, not just the one report a process per language would have seen.
// Zero reports is legal and parses to no findings at all, which is what a
// branch that ran and found nothing reports as.
//
// A report that cannot be opened or parsed does not stop the reading: it is
// recorded into unchecked and the rest are still read, so one unreadable
// report never hides what another found.
func readReports(fa FilterArgs, root srcpath.Root) parsedReports {
	var out parsedReports
	for _, r := range fa.Reports {
		//nolint:gosec // G304: the report path is the caller's whole point. The
		// harness names the files its own build just wrote, in a directory it
		// created, so there is no trust line for a variable path to cross.
		f, err := os.Open(r.Path)
		if err != nil {
			out.unchecked = append(out.unchecked, fmt.Sprintf("could not read report %s: %v", r.Path, err))
			continue
		}
		got, gotDropped, err := r.Parser(f, root)
		_ = f.Close()
		if err != nil {
			out.unchecked = append(out.unchecked, fmt.Sprintf("could not read report %s: %v", r.Path, err))
			continue
		}
		out.collect(r.Path, got, gotDropped)
	}
	out.findings = dedup(out.findings)
	return out
}

// collect takes one report's results in and records whether that report
// checked this repo at all. A report that placed nothing while every artifact
// it named sat outside the root describes another tree, so passing the commit
// on it would be the tool reporting clean on a report it never read. The
// question is asked of each report on its own, because a commit spanning two
// projects where only one resolves its URIs inside the repo would otherwise
// ride on the other's results. A report with no results, or one whose results
// were dropped for any other reason, is a genuine clean pass.
func (p *parsedReports) collect(source string, findings []lintfind.Finding, dropped []lintfind.Dropped) {
	p.findings = append(p.findings, findings...)
	p.dropped = append(p.dropped, dropped...)
	if len(findings) > 0 {
		return
	}
	var outside []lintfind.Dropped
	for _, d := range dropped {
		if d.Outside {
			outside = append(outside, d)
		}
	}
	if len(outside) == 0 {
		return
	}
	msg := "every result in " + source + " fell outside the repo, so nothing was checked"
	for _, d := range outside {
		msg += fmt.Sprintf("\n  %s at %s", d.Rule, d.URI)
	}
	p.unchecked = append(p.unchecked, msg)
}

// dedup keeps one copy of each distinct finding. A multi-targeted project
// compiles once per framework and writes one report per framework, and a .cs
// linked into two projects is compiled by both, so the same source-level warning
// arrives two or more times. The framework and the owning project are not part
// of the identity the gate judges, and a duplicate would print twice and demand
// a second waiver for a single line of code.
//
// Language is part of the key: one process now reads findings from every
// language branch of a commit, and the same rule string can mean two
// unrelated things in two languages (UNPARSED, which both the ESLint and
// golangci parsers emit for an entry they cannot read), so a Go and a
// TypeScript finding sharing a rule and message stay two findings.
func dedup(findings []lintfind.Finding) []lintfind.Finding {
	seen := make(map[string]bool, len(findings))
	unique := make([]lintfind.Finding, 0, len(findings))
	for _, f := range findings {
		key := f.Language + "\x00" + f.Rule + "\x00" + f.Message
		for _, loc := range f.Locations {
			key += fmt.Sprintf("\x00%s:%d:%d-%d", loc.Path, loc.StartLine, loc.StartColumn, loc.EndLine)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, f)
	}
	return unique
}
