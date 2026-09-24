// Command metric-gate scores the methods a change touched against a metric
// threshold. Its scope defaults to the merge base of HEAD and the default
// branch, and --staged, --since <ref> or --files <path>... pick another
// (ADR 0008, issue 14). --coverage <path>, repeatable, names the coverage
// reports to read in place of discovery. --metric <name> picks which of the
// metrics the binary hosts a run measures and --threshold <name>=<n> sets one
// metric's bar, both repeatable, both defaulting to what the catalogue in
// package metric declares (issue 19). The rest of ADR 0008's command line
// lands with its own issues, and any other argument is a usage error. Package
// scope owns the whole of argv, so the usage block one prints lists every
// flag. Stdout is one TOON document, stderr is the human summary, one line
// except on the unknown_changed_method path, which prints the cause above the
// counts, and the exit code is 0 pass, 1 tool error, 2 threshold exceeded.
//
// Any error asFailure cannot type writes its cause to stderr and exits 1 with
// no typed code. Exactly two exits have that shape, and this command reaches
// no other. Two more untyped returns exist, in the encoder refusing to render
// the document and in coverage discovery, but no input reaches them: emit
// reports the first and nothing the gate builds makes the encoder refuse, and
// discovery's own walk hands back no error.
//
// ADR 0008 carves out both. The first is a malformed command line, which exits
// before measure ever runs, empty stdout and all, because argv failed before
// the run had a shape to report. Every cause measure can reach, by contrast,
// carries a typed code and a document: failing to run git, failing to find a
// git repository, a repository git will not answer about, failing to resolve
// the root git named, failing to read the process working directory, failing
// to stat a changed file to date it against the coverage report, all land
// inside the document rather than beside it.
// Issue 86 and issue 31 moved these in, one at a time; this is the one place
// that says none are left outside it, so a case in gate/test pinning such an
// exit points here rather than restate it.
//
// The second is reachable only after the document is built, stdout refusing
// the write that carries it, a full disk or a closed descriptor among the
// causes. 0008 named it as a carve-out on its 2026-09-17 consolidation. The
// gate did produce a document, and it still exits 1 with no typed code,
// because the document is the thing that could not be delivered. A write that
// came up short leaves a truncated document behind, so on this cause alone
// stdout may hold part of a document rather than nothing, and the exit code is
// the only signal a caller can trust. There is nowhere to move this one to:
// the failure is stdout itself, which is where a typed code would have to be
// printed.
package main

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/tvrmsmith/coding-standards/gate/internal/coverage"
	"github.com/tvrmsmith/coding-standards/gate/internal/crap"
	"github.com/tvrmsmith/coding-standards/gate/internal/extract"
	"github.com/tvrmsmith/coding-standards/gate/internal/join"
	"github.com/tvrmsmith/coding-standards/gate/internal/metric"
	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/scope"
	"github.com/tvrmsmith/coding-standards/internal/gitscope"
	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

func main() {
	sc, err := scope.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	doc, err := measure(sc, os.Getwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code, err := emit(os.Stdout, os.Stderr, doc)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(code)
}

// emit writes doc to stdout and stderr and reports the exit code that goes
// with them. A caller reads exit 0 as approval, so this is the one place a
// failure to deliver the document has to turn into something other than
// doc.ExitCode(). A run that could not render its document, and a run that
// rendered one and could not deliver it, must both look like something other
// than a pass.
//
// ADR 0008 carves this out. The failure carries no typed error.code, because
// the code would have nowhere to be printed, stdout being the thing that is
// unusable. See the package doc above.
func emit(stdout, stderr io.Writer, doc report.Document) (int, error) {
	body, err := doc.Stdout()
	if err != nil {
		return 1, fmt.Errorf("rendering the document: %w", err)
	}
	n, err := stdout.Write(body)
	if err != nil {
		return 1, fmt.Errorf("writing the document to stdout: %w", err)
	}
	// io.Writer obliges a short write to report an error and os.File honours
	// that, so this branch only fires for a writer that breaks the contract.
	// A caller must never read exit 0 as approval, so emit checks the count
	// itself rather than trusting the writer.
	if n != len(body) {
		return 1, fmt.Errorf("writing the document to stdout: wrote %d of %d bytes", n, len(body))
	}
	// The document already reached the caller by this point, so a failure
	// writing the human summary has nowhere left to complain: the summary is
	// what a complaint would be printed to. Dropped explicitly rather than
	// turned into a second exit-1 cause, which would contradict the document
	// the caller is already holding.
	_, _ = io.WriteString(stderr, doc.Stderr())
	return doc.ExitCode(), nil
}

// measure runs the gate over the repo containing the working directory,
// scoped as sc names. A typed exit-1 cause becomes the document's error
// block; anything asFailure cannot type comes back as an error, which the
// package doc above enumerates.
//
// getwd is the one read of the process working directory the whole run
// performs, taken here rather than deeper down where --files and --coverage
// resolution used to each call os.Getwd for themselves. It runs before
// gitscope.Open, for two reasons. It needs nothing, so a test can drive this
// failure without standing up a fixture repo the way every other case in this
// package does. And it adds no failure to a run that would otherwise pass:
// Open shells out to git in this same working directory, so a working
// directory the gate cannot read had already doomed the run before Open ever
// got to it.
func measure(sc scope.Scope, getwd func() (string, error)) (report.Document, error) {
	var doc report.Document
	doc.Scope = sc.Mode
	cwd, err := getwd()
	if err != nil {
		doc.Failure = &report.Failure{
			Code:    report.CodeWorkingDirectoryUnreadable,
			Message: "could not read the process working directory: " + cause(err).Error(),
		}
		return doc, nil
	}
	repo, err := gitscope.Open()
	if err != nil {
		if failure, ok := asFailure(err); ok {
			doc.Failure = failure
			return doc, nil
		}
		return doc, err
	}

	var selected selection
	if sc.Mode == scope.ModeFiles {
		selected, err = selectFiles(repo, sc.Files, cwd)
	} else {
		selected, err = selectDiff(repo, sc)
	}
	// A selection that failed part-way still carries what it did establish,
	// so a base resolved before the extractor broke is still the base the
	// document names. Failure travels with them: a selection returning an
	// error carries no typed cause, since failing is the only thing that sets
	// the field and it keeps the error instead whenever it cannot type it.
	doc.Base = selected.Base
	doc.TouchedLinesOutsideSpans = selected.TouchedLinesOutsideSpans
	doc.SkippedPaths = selected.SkippedPaths
	doc.Failure = selected.Failure
	if err != nil {
		return doc, err
	}
	// A typed cause the selection established is already the whole verdict.
	// No metric runs behind it, so the document is finished here.
	if doc.Failed() {
		return doc, nil
	}

	extracted, changed := selected.Extracted, selected.Changed
	doc.ChangedMethods = len(changed)
	// ADR 0014: an empty changed-method set exits 0 before resolving any
	// input, because a metric with nothing to compute is not asking for one.
	// Every selected metric is still emitted, with no rows, so the document
	// carries the same keys it would have carried had there been work.
	if len(changed) == 0 {
		doc.Metrics = entriesFor(sc.Metrics)
		return doc, nil
	}

	lines, declared, skipped, err := loadCoverage(repo.Root(), sc.Coverage, changed, sc.Metrics, cwd)
	// The append is what enforces ADR 0008's skipped_paths order, which
	// gate/test/golden/files_skip_before_discovery_skip.toon pins.
	doc.SkippedPaths = append(doc.SkippedPaths, skipped...)
	if failure, ok := asFailure(err); ok {
		doc.Failure = failure
		return doc, nil
	} else if err != nil {
		return doc, err
	}

	metrics, unknown := attribute(sc.Metrics, extracted, changed, lines, declared)
	doc.Metrics = metrics
	// Any single unknown fails the run: there is no tolerated fraction, and
	// the table is still present on that failure. The count comes off the join
	// rather than off a metric's rows, because a method nothing could
	// attribute is unknown to the run, not to one metric's reading of it.
	if unknown > 0 {
		doc.Failure = &report.Failure{
			Code:    report.CodeUnknownChangedMethod,
			Message: unknownMessage(unknown),
		}
	}
	return doc, nil
}

// selection is everything a scope establishes before any metric runs: which
// spans exist, which of them the scope calls changed, and what the document
// has to say about how they were reached.
//
// Failure is a cause the selection itself discovered, a file staged in one
// state and dirty in another, a base that will not resolve, or a --files path
// the gate cannot place on a source file. It is a field rather than a returned
// error because the other fields are still meant for the document on the first
// of those, where a dirty staged file still has a base to name. The other two
// have nothing else to preserve, since a base that will not resolve is the
// first thing the diff scopes ask for and --files resolves no base at all and
// stops on the first path it will not take. They travel the same way so every
// caller reads one channel.
type selection struct {
	Base                     *string
	Extracted                extract.Result
	Changed                  []extract.Span
	TouchedLinesOutsideSpans int
	SkippedPaths             []string
	Failure                  *report.Failure
}

// selectDiff resolves sc's git diff into a selection, the shape --staged,
// --since and the merge-base default all share.
func selectDiff(repo gitscope.Repo, sc scope.Scope) (selection, error) {
	var selected selection
	base, err := resolveBase(repo, sc)
	if err != nil {
		var noBase gitscope.NoBaseError
		if errors.As(err, &noBase) {
			return refusing(selected, &report.Failure{Code: report.CodeNoDiffBase, Message: noBase.Error()})
		}
		return failing(selected, err)
	}
	label := base.Label()
	selected.Base = &label

	touched, err := repo.TouchedLines(base)
	if err != nil {
		return failing(selected, err)
	}

	files := changedFiles(touched)
	extracted, err := extract.Extract(repo.Root(), files)
	if err != nil {
		if dirty := dirtyBehindExtraction(repo, base, files); dirty != nil {
			return refusing(selected, dirty)
		}
		return failing(selected, err)
	}

	if base.Staged {
		// A --staged run checks the claimed files for divergence, not the
		// diff's own file list: a dirty staged Markdown file the diff never
		// claimed cannot be misattributed, so it does not refuse the run.
		dirty, err := stagedDirty(repo, extracted.ClaimedPaths())
		if err != nil {
			return failing(selected, err)
		}
		if dirty != nil {
			return refusing(selected, dirty)
		}
	}

	selected.Extracted = extracted
	selected.Changed, selected.TouchedLinesOutsideSpans = join.Changed(extracted, touched)
	return selected, nil
}

// dirtyBehindExtraction is the staged_file_dirty refusal hiding behind an
// extraction that failed, and nil whenever the extractor's own cause is the
// one to keep.
//
// The widest divergence, a file staged and then deleted from disk, reaches the
// extractor as a path with nothing to read and comes back as the extractor's
// own failure. Nothing was claimed, so selectDiff's ordinary check cannot see
// it, and a caller branching on the code would be told its source does not
// parse. The paths the gate could hand to an extractor by the static extension
// table answer that here. They are asked about rather than the whole diff,
// because a dirty staged Markdown file the extractor never saw would otherwise
// replace the extractor's own cause with one about a file nothing was going to
// read. stagedDirty names no file when it could not ask git, and that answer
// is the one to keep here for the same reason: the extractor's cause is the one
// the gate did establish.
//
// The check spans every routable changed path rather than only the paths
// extraction failed on, so a dirty staged source file unrelated to the crash
// still overrides the extractor's cause. extract.Extract fails atomically, so
// there is no per-path failure set to narrow to. Narrowing further would mean
// parsing paths out of the extractor's cause text, which the ADR 0009
// extractor contract does not promise, and the override would then stop
// firing for every cause that carries no path.
func dirtyBehindExtraction(repo gitscope.Repo, base gitscope.Base, files []srcpath.Path) *report.Failure {
	if !base.Staged {
		return nil
	}
	dirty, _ := stagedDirty(repo, extract.Routable(files))
	return dirty
}

// selectFiles resolves names, --files' argument list, into a selection. This
// is the shape ADR 0017 gives --files instead of a diff: every method in a
// listed file is changed, there is no base to record, and
// touched_lines_outside_spans has nothing to count.
func selectFiles(repo gitscope.Repo, names []string, cwd string) (selection, error) {
	var selected selection
	// --files is the first path list a developer writes by hand rather than
	// one built from sorted map keys, so it is the first that can name one
	// file twice. srcpath owns both the resolution and that de-duplication,
	// since both turn on which file a typed name landed on.
	resolved, err := repo.Root().NamedFiles(names, cwd)
	if err != nil {
		// Only a refusal about the path itself carries the typed code, whose
		// message the reader expects to name a path. NamedFiles no longer reads
		// the working directory itself, measure already has, so nothing else
		// reaches this branch today; it stays a fallback for a future refusal
		// srcpath types some other way.
		var unresolved *srcpath.UnresolvedError
		if errors.As(err, &unresolved) {
			return refusing(selected, &report.Failure{Code: report.CodeFileUnresolved, Message: unresolved.Error()})
		}
		return failing(selected, err)
	}

	extracted, err := extract.Extract(repo.Root(), resolved)
	if err != nil {
		return failing(selected, err)
	}
	// ADR 0008's 2026-09-11 amendment names --files as the first of the two
	// producers of skipped_paths. A named file no extractor claims is neither
	// measured nor an error, so it is listed rather than silently dropped.
	selected.SkippedPaths = pathStrings(extracted.Unclaimed(resolved))
	selected.Extracted = extracted
	selected.Changed = join.AllSpans(extracted)
	return selected, nil
}

// pathStrings renders paths as the plain strings SkippedPaths holds.
func pathStrings(paths []srcpath.Path) []string {
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, path.String())
	}
	return names
}

// resolveBase picks the git resolution matching sc.Mode. --files never
// reaches it, because a file list names no commit to diff against.
func resolveBase(repo gitscope.Repo, sc scope.Scope) (gitscope.Base, error) {
	switch sc.Mode {
	case scope.ModeSince:
		return repo.ResolveRef(sc.Ref)
	case scope.ModeStaged:
		return repo.ResolveStaged()
	default:
		return repo.ResolveBase()
	}
}

// stagedDirty renders the refusal when any of paths is staged in one state and
// on disk in another, and nil when none is. The failure to ask comes back
// separately from the answer, because one caller reports it and the other is
// already carrying a cause it would rather keep. Both are typed causes by the
// time they get here, DivergentFromIndex having typed its own, so the two
// channels are the only thing telling them apart.
func stagedDirty(repo gitscope.Repo, paths []srcpath.Path) (*report.Failure, error) {
	dirty, err := repo.DivergentFromIndex(paths)
	if err != nil {
		return nil, err
	}
	if len(dirty) == 0 {
		return nil, nil
	}
	return &report.Failure{Code: report.CodeStagedFileDirty, Message: dirtyMessage(dirty)}, nil
}

// dirtyMessage names the files staged in one state and on disk in another,
// comma-space separated in sorted order. The sort happens here rather than
// being left to the callers, both of which hand over a sorted list today, so
// the order the goldens pin is held where the sentence is built.
func dirtyMessage(dirty []srcpath.Path) string {
	names := make([]string, 0, len(dirty))
	for _, path := range dirty {
		names = append(names, path.String())
	}
	slices.Sort(names)
	return "refusing to score " + strings.Join(names, ", ") + ": staged in one state and on disk in another"
}

// implementedInputs names every input this binary knows how to go and get.
// It is the other half of ADR 0002's declaration: a metric declaring an input
// nothing here resolves would be ignored in silence, because the resolution
// below asks only about the inputs it implements, so nothing would go looking
// and the run would score the metric as though it had everything it asked for.
//
// Nothing reads this list at run time. Adding an Input to it makes the gate
// fetch nothing: loadCoverage asks about metric.InputCoverage by name, and
// every future input needs its own resolution written by hand beside it. The
// list exists only so TestEveryHostedDeclarationIsImplemented fails when a
// catalogue entry declares an input no such code went and got.
var implementedInputs = []metric.Input{metric.InputCoverage}

// loadCoverage resolves the coverage input, and only because a selected
// metric declared it. Whether one did comes back as declared, so the caller
// reads the decision this function already made rather than asking
// metric.Declaring the same question again and risking a different answer.
// The failure names the metrics that are stuck rather
// than the file that is absent (ADR 0002). The paths discovery could
// not read come back alongside, including on the missing-report failure,
// where they are the likeliest explanation for it. When the developer named
// any report, discovery does not run at all: skipped_paths stays empty, and
// the named sources go straight to Load. That branch always returns a nil
// skipped list of its own too, since a named report is the one Source.Origin
// coverage.Load never skips as superseded. The newest edit is computed here,
// behind the declared-input guard, so a metric asking for no coverage pays no
// stat for one. The clock is read once here and passed down, so every report
// in a run is judged against the same instant rather than against a clock that
// advanced between them, which would let two reports stamped alike land on
// opposite sides of the tolerance.
//
// A Load failure returns no skipped paths, discovery's own included. Nothing
// was scored, so no skipped path explains a short score, and listing a
// directory the walk could not enter beside an unrelated coverage_unparseable
// reads as though both were why. That is the rule coverage.Load already
// applies to the names it collects itself, and applying it to only one of the
// two producers would put the same field under two rules on one path. The
// missing-report failure above is the one exception, and it earns it: there
// the unreadable paths are candidate reports, so they are the likeliest
// explanation for finding none.
//
// cwd is what a relative --coverage path resolves against, read once by
// measure rather than here; see measure's doc comment for why.
func loadCoverage(root srcpath.Root, named []string, changed []extract.Span, selected []metric.Selection, cwd string) (lines coverage.Set, declared bool, skipped []string, err error) {
	declaring := metric.Declaring(selected, metric.InputCoverage)
	if len(declaring) == 0 {
		return nil, false, nil, nil
	}
	now := time.Now()
	newest, err := newestEdit(root, changed)
	if err != nil {
		return nil, true, nil, err
	}
	if len(named) > 0 {
		set, _, err := coverage.Load(root, coverage.Named(root, cwd, named), newest, now)
		return set, true, nil, err
	}
	sources, skipped, err := coverage.Discover(root)
	if err != nil {
		return nil, true, nil, err
	}
	if len(sources) == 0 {
		return nil, true, skipped, &report.Failure{
			Code: report.CodeCoverageMissing,
			Message: displayNames(declaring) + " requires a coverage report, none found matching " +
				coverage.Glob + " under the repo root",
		}
	}
	set, superseded, err := coverage.Load(root, sources, newest, now)
	if err != nil {
		return nil, true, nil, err
	}
	// discovery's own skips and Load's superseded skips are two different
	// reasons a path is missing from the score, a walk that could not read a
	// directory and a report a fresher run replaced, but skipped_paths does
	// not distinguish them, so they append into one sorted list rather than
	// two fields the document would have to carry separately.
	skipped = append(skipped, superseded...)
	slices.Sort(skipped)
	return set, true, skipped, nil
}

// newestEdit stats the working-tree file of every distinct span file among
// the changed methods, and returns the greatest modification time, truncated
// to whole seconds, along with the file that holds it. A tie on equal times
// resolves to the lexicographically smallest path, and keeping the first file
// seen is enough for that because join.Changed returns its spans in ascending
// file order, which its doc comment promises and its own test holds it to.
//
// An unreadable file fails the run rather than being dropped from the set the
// way coverage.Discover tolerates an unreadable path under skipped_paths. The
// stat feeds the staleness comparison, so dropping a file would compute the
// newest edit over a smaller set, which can date a stale report as fresh and
// report a method as covered against coverage that predates it. Discover's
// skipped path costs a missing score for that path alone; a skipped stat here
// costs a wrong verdict across every method the run scores.
func newestEdit(root srcpath.Root, changed []extract.Span) (coverage.Newest, error) {
	var newest coverage.Newest
	seen := map[srcpath.Path]bool{}
	for _, span := range changed {
		if seen[span.File] {
			continue
		}
		seen[span.File] = true
		info, err := os.Stat(root.Abs(span.File))
		if err != nil {
			return coverage.Newest{}, &report.Failure{
				Code: report.CodeChangedFileUnreadable,
				Message: fmt.Sprintf("could not read %s to date it against the coverage report: %s",
					span.File, cause(err)),
			}
		}
		at := info.ModTime().Truncate(time.Second)
		if newest.File == "" || at.After(newest.At) {
			newest = coverage.Newest{File: span.File, At: at}
		}
	}
	return newest, nil
}

// displayNames renders the prose names of selected, comma-space separated in
// selection order, which is how a failure about an input names every metric
// that is stuck on it rather than only the first (ADR 0002).
func displayNames(selected []metric.Selection) string {
	names := make([]string, len(selected))
	for i, sel := range selected {
		names[i] = sel.Display
	}
	return strings.Join(names, ", ")
}

// cause unwraps an *os.PathError to its syscall error, so the message names
// only what actually went wrong rather than the absolute path os.Stat's own
// Error() carries. ADR 0011 names the source path already in span.File, and
// a document naming an absolute path defeats that.
func cause(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	// os.Getwd reports its failure as an *os.SyscallError, which prefixes the
	// syscall's name onto what the operating system said. The sentence the
	// code carries already names the read that failed, so the name would be
	// said twice.
	var syscallErr *os.SyscallError
	if errors.As(err, &syscallErr) {
		return syscallErr.Err
	}
	return err
}

// entriesFor opens one document entry per selected metric, carrying the bar
// the run reads it against and no rows yet.
func entriesFor(selected []metric.Selection) []report.Metric {
	metrics := make([]report.Metric, 0, len(selected))
	for _, sel := range selected {
		metrics = append(metrics, report.Metric{Name: sel.Name, Display: sel.Display, Threshold: sel.Threshold})
	}
	return metrics
}

// attribute reads changed against every selection, and reports the entries
// beside the count of methods nothing could attribute. The join runs once
// whatever is selected. Every metric scores the same method set; only the bar
// it is read against differs, so attributing the set per metric would repeat
// the expensive half of the run to reach the same answer.
//
// declared is loadCoverage's own answer about whether any selection asked for
// coverage. When nothing did there is no coverage set, and the join reads an
// absent set as every file unmatched, which would fail the run with
// unknown_changed_method over an input nobody asked for (ADR 0002). So the
// join does not run and the selection contributes no rows; its summary row
// still names it, at its bar, measured 0 and failed 0, so the run exits 0.
//
// What a row looks like for a metric that takes no coverage reading is the
// second metric's to settle, alongside the formula that would fill it. That is
// the same deferral rowFor's comment makes, and inventing a row shape, or a
// state and action token for it, before there is a formula to fit would be
// guessing at both.
//
// The gate is the union of the declarations, not each metric's own: one
// declaring metric is enough to make the set worth loading and the join worth
// running for every metric in the selection.
func attribute(selected []metric.Selection, extracted extract.Result, changed []extract.Span, lines coverage.Set, declared bool) ([]report.Metric, int) {
	metrics := entriesFor(selected)
	if !declared {
		return metrics, 0
	}

	unknown := 0
	for _, method := range join.Attribute(extracted.Spans, changed, lines) {
		if method.State == report.StateUnknown {
			unknown++
		}
		for i, sel := range selected {
			metrics[i].Rows = append(metrics[i].Rows, rowFor(method, sel))
		}
	}
	return metrics, unknown
}

// rowFor renders one joined method as a document row, scored at sel's own
// threshold: the same method can pass one selection's bar and fail another's,
// so the row is a reading of the method against a bar rather than a property
// of the method. An unknown method carries no numbers, only its typed reason.
//
// The formula is CRAP's whatever sel names, which is correct only while CRAP
// is the one metric the catalogue hosts. A second entry needs the formula to
// be dispatched off sel rather than fixed here, and that dispatch is that
// metric's work rather than issue 19's: the selection machinery this row sits
// in is what makes the dispatch possible, and choosing its shape before there
// is a second formula to fit would be guessing at one.
func rowFor(method join.Method, sel metric.Selection) report.Row {
	row := report.Row{
		File:       method.Span.File,
		Start:      method.Span.StartLine,
		End:        method.Span.EndLine,
		Name:       method.Span.Name,
		Complexity: method.Span.Complexity,
		State:      method.State,
		Action:     crap.ActionNone,
		Reason:     method.Reason,
	}
	if method.State == report.StateUnknown {
		return row
	}
	measurement := crap.Measurement{Complexity: method.Span.Complexity, Coverage: method.Coverage, Threshold: sel.Threshold}
	score := measurement.Score()
	row.Coverage = &method.Coverage
	row.Score = &score
	row.Action = measurement.Action()
	row.TargetCoverage = measurement.TargetCoverage()
	return row
}

// unknownMessage names how many changed methods the join could not attribute.
func unknownMessage(unknown int) string {
	noun := "changed methods"
	if unknown == 1 {
		noun = "changed method"
	}
	return fmt.Sprintf("%d %s could not be attributed to a coverage report", unknown, noun)
}

// changedFiles lists the files the diff touched, in a fixed order so the
// extractor sees the same stdin on every run.
func changedFiles(touched map[srcpath.Path][]int) []srcpath.Path {
	return slices.Sorted(maps.Keys(touched))
}

// refusing is the selection carrying a typed cause it established as an
// answer rather than caught as an error: the file it was asked about is
// staged in one state and on disk in another, or the base will not resolve.
// The selection is done, and the run exits 1 off the Failure field, so there
// is no error left to return. Sharing failing's shape keeps every exit from a
// selection on one of two named calls, neither of which reads as a check that
// found a problem and then said nothing.
func refusing(selected selection, failure *report.Failure) (selection, error) {
	selected.Failure = failure
	return selected, nil
}

// failing folds a typed exit-1 cause into the selection that was carrying it
// as an error, so every cause a selection discovered reaches its caller on the
// one Failure field. A cause the document cannot describe stays an error.
func failing(selected selection, err error) (selection, error) {
	if failure, ok := asFailure(err); ok {
		selected.Failure = failure
		return selected, nil
	}
	return selected, err
}

// asFailure unwraps a typed exit-1 cause out of an error.
//
// gitscope has no vocabulary for a document code, so the mapping from its
// diff-reading error onto CodeDiffUnparseable, and from each of Open's kinds
// onto the code ADR 0008 gives it, lives here. Doing it at this single site
// rather than at each gitscope call keeps the property the package doc claims:
// a cause added under TouchedLines reaches the error block without anything
// here being extended to let it.
func asFailure(err error) (*report.Failure, bool) {
	var failure *report.Failure
	if errors.As(err, &failure) {
		return failure, true
	}
	var unreadable gitscope.UnreadableDiffError
	if errors.As(err, &unreadable) {
		return &report.Failure{Code: report.CodeDiffUnparseable, Message: unreadable.Message}, true
	}
	var open gitscope.OpenError
	if errors.As(err, &open) {
		code, mapped := openCode(open.Kind)
		if !mapped {
			return &report.Failure{
				Code:    code,
				Message: fmt.Sprintf("the gate has no error code for gitscope.OpenKind %d. %s", open.Kind, open.Message),
			}, true
		}
		return &report.Failure{Code: code, Message: open.Message}, true
	}
	return nil, false
}

// openCode is ADR 0008's code for each way Open can fail. The bool reports
// whether kind had a mapping: true for the four kinds gitscope defines today,
// false for a kind added to that package without a case added here, which
// returns report.CodeInternalError rather than panicking so the run still
// exits 1 with a document.
func openCode(kind gitscope.OpenKind) (string, bool) {
	switch kind {
	case gitscope.OpenGitUnavailable:
		return report.CodeGitUnavailable, true
	case gitscope.OpenRepoUnreadable:
		return report.CodeGitRepoUnreadable, true
	case gitscope.OpenNoRepo:
		return report.CodeNoGitRepo, true
	case gitscope.OpenRootUnresolvable:
		return report.CodeRepoRootUnresolvable, true
	}
	return report.CodeInternalError, false
}
