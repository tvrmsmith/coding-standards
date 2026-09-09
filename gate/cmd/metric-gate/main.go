// Command metric-gate scores the methods a change touched against a metric
// threshold. Its scope defaults to the merge base of HEAD and the default
// branch, and --staged, --since <ref> or --files <path>... pick another
// (ADR 0008, issue 14). --coverage <path>, repeatable, names the coverage
// reports to read in place of discovery. The rest of ADR 0008's command line
// lands with its own issues, and any other argument is a usage error. Package
// scope owns the whole of argv, so the usage block one prints lists every
// flag. Stdout is one TOON document, stderr is the human summary, one line
// except on the unknown_changed_method path, which prints the cause above the
// counts, and the exit code is 0 pass, 1 tool error, 2 threshold exceeded.
//
// Any error that is not typed as a report.Failure writes its cause to stderr,
// leaves stdout empty, and exits 1. ADR 0008 sanctions that shape for one
// failure only, a malformed command line, which "exits 1 with empty stdout and
// no typed code, because argv failed before the run had a shape to report".
//
// Four other errors take that shape, and 0008 sanctions none of them. Failing
// to open a git repository lands upstream of the document, before there is a
// base or a scope to report. Failing to read the working directory while
// resolving a relative --files name lands after that and before any method is
// counted. Failing to stat a changed file and failing to read the working
// directory a --coverage path resolves against land after the changed methods
// are counted, so the gate did examine a repository and still emits nothing.
// All four are known deviations from the contract rather than part of it, and
// issue 31 gives the last two typed codes and moves them inside the document.
//
// A fifth error stays outside issue 31's scope and is the one exception to the
// empty stdout above: stdout refusing the write that carries the document, a
// full disk or a closed descriptor among the causes. It happens after the
// document is built, so the gate did produce one, and it still exits 1 with no
// typed code, because the document is the thing that could not be delivered.
// A write that came up short leaves a truncated document behind, so on this
// cause alone stdout may hold part of a document rather than nothing, and the
// exit code is the only signal a caller can trust. Issue 31 has nowhere to move
// it to.
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
	"github.com/tvrmsmith/coding-standards/gate/internal/gitscope"
	"github.com/tvrmsmith/coding-standards/gate/internal/join"
	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/scope"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

func main() {
	sc, err := scope.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	doc, err := measure(sc)
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
// This is a known deviation from ADR 0008: the failure carries no typed
// error.code, because the code would have nowhere to be printed, stdout
// being the thing that is unusable. See the package doc above.
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
	// writing the human summary has nowhere left to complain. It is dropped
	// rather than turned into a second exit-1 cause.
	io.WriteString(stderr, doc.Stderr())
	return doc.ExitCode(), nil
}

// measure runs the gate over the repo containing the working directory,
// scoped as sc names. A typed exit-1 cause becomes the document's error
// block; anything not typed as a report.Failure comes back as an error, which
// the package doc above enumerates.
func measure(sc scope.Scope) (report.Document, error) {
	var doc report.Document
	doc.Scope = sc.Mode
	repo, err := gitscope.Open()
	if err != nil {
		return doc, err
	}

	var selected selection
	if sc.Mode == scope.ModeFiles {
		selected, err = selectFiles(repo, sc.Files)
	} else {
		selected, err = selectDiff(repo, sc)
	}
	// A selection that failed part-way still carries what it did establish,
	// so a base resolved before the extractor broke is still the base the
	// document names.
	doc.Base = selected.Base
	doc.TouchedLinesOutsideSpans = selected.TouchedLinesOutsideSpans
	doc.SkippedPaths = selected.SkippedPaths
	if err != nil {
		return doc, err
	}
	if selected.Failure != nil {
		doc.Failure = selected.Failure
		return doc, nil
	}

	extracted, changed := selected.Extracted, selected.Changed
	doc.ChangedMethods = len(changed)
	metric := report.Metric{Name: crap.Name, Display: crap.DisplayName, Threshold: crap.Threshold}
	// ADR 0007: an empty changed-method set exits 0 before resolving any
	// input, because a metric with nothing to compute is not asking for one.
	if len(changed) == 0 {
		doc.Metric = &metric
		return doc, nil
	}

	lines, skipped, err := loadCoverage(repo.Root(), sc.Coverage, changed)
	// The append is what enforces ADR 0008's order, the paths --files named
	// first and coverage discovery's skips after them. Merging the two lists
	// and sorting the result would read as tidier and would break it.
	doc.SkippedPaths = append(doc.SkippedPaths, skipped...)
	if failure, ok := asFailure(err); ok {
		doc.Failure = failure
		return doc, nil
	} else if err != nil {
		return doc, err
	}

	for _, method := range join.Attribute(extracted.Spans, changed, lines) {
		metric.Rows = append(metric.Rows, rowFor(method))
	}
	doc.Metric = &metric
	// Any single unknown fails the run: there is no tolerated fraction, and
	// the table is still present on that failure.
	if unknown := len(metric.Rows) - metric.Measured(); unknown > 0 {
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
			selected.Failure = &report.Failure{Code: report.CodeNoDiffBase, Message: noBase.Error()}
			return selected, nil
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
			selected.Failure = dirty
			return selected, nil
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
			selected.Failure = dirty
			return selected, nil
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
// is the shape ADR 0007 gives --files instead of a diff: every method in a
// listed file is changed, there is no base to record, and
// touched_lines_outside_spans has nothing to count.
func selectFiles(repo gitscope.Repo, names []string) (selection, error) {
	var selected selection
	// --files is the first path list a developer writes by hand rather than
	// one built from sorted map keys, so it is the first that can name one
	// file twice. srcpath owns both the resolution and that de-duplication,
	// since both turn on which file a typed name landed on.
	resolved, err := repo.Root().NamedFiles(names)
	if err != nil {
		// Only a refusal about the path itself carries the typed code, whose
		// message the reader expects to name a path. Anything else, a lost
		// working directory for instance, is upstream of the document.
		var unresolved *srcpath.UnresolvedError
		if errors.As(err, &unresolved) {
			selected.Failure = &report.Failure{Code: report.CodeFileUnresolved, Message: unresolved.Error()}
			return selected, nil
		}
		return failing(selected, err)
	}

	extracted, err := extract.Extract(repo.Root(), resolved)
	if err != nil {
		return failing(selected, err)
	}
	// ADR 0008 names --files as a second producer of skipped_paths: a named
	// file no extractor claims is neither measured nor an error, so it is
	// listed rather than silently dropped.
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
// already carrying a cause it would rather keep.
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

// loadCoverage resolves the coverage input, and only because
// crap.DeclaredInputs names it. The failure names the metric that is stuck
// rather than the file that is absent (ADR 0002). The paths discovery could
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
func loadCoverage(root srcpath.Root, named []string, changed []extract.Span) (coverage.Set, []string, error) {
	if !slices.Contains(crap.DeclaredInputs, inputCoverage) {
		return nil, nil, nil
	}
	now := time.Now()
	newest, err := newestEdit(root, changed)
	if err != nil {
		return nil, nil, err
	}
	if len(named) > 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, nil, fmt.Errorf("resolving --coverage paths against the working directory: %w", err)
		}
		set, _, err := coverage.Load(root, coverage.Named(root, cwd, named), newest, now)
		return set, nil, err
	}
	sources, skipped, err := coverage.Discover(root)
	if err != nil {
		return nil, nil, err
	}
	if len(sources) == 0 {
		return nil, skipped, &report.Failure{
			Code: report.CodeCoverageMissing,
			Message: crap.DisplayName + " requires a coverage report, none found matching " +
				coverage.Glob + " under the repo root",
		}
	}
	set, superseded, err := coverage.Load(root, sources, newest, now)
	if err != nil {
		return nil, nil, err
	}
	// discovery's own skips and Load's superseded skips are two different
	// reasons a path is missing from the score, a walk that could not read a
	// directory and a report a fresher run replaced, but skipped_paths does
	// not distinguish them, so they append into one sorted list rather than
	// two fields the document would have to carry separately.
	skipped = append(skipped, superseded...)
	slices.Sort(skipped)
	return set, skipped, nil
}

// newestEdit stats the working-tree file of every distinct span file among
// the changed methods, and returns the greatest modification time, truncated
// to whole seconds, along with the file that holds it. A tie on equal times
// resolves to the lexicographically smallest path, and keeping the first file
// seen is enough for that because join.Changed returns its spans in ascending
// file order, which its doc comment promises and its own test holds it to.
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
			return coverage.Newest{}, fmt.Errorf("stat %s: %w", span.File, err)
		}
		at := info.ModTime().Truncate(time.Second)
		if newest.File == "" || at.After(newest.At) {
			newest = coverage.Newest{File: span.File, At: at}
		}
	}
	return newest, nil
}

// inputCoverage is the declared input name a coverage report answers.
const inputCoverage = "coverage"

// rowFor renders one joined method as a document row. An unknown method
// carries no numbers, only its typed reason.
func rowFor(method join.Method) report.Row {
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
	measurement := crap.Measurement{Complexity: method.Span.Complexity, Coverage: method.Coverage}
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
func asFailure(err error) (*report.Failure, bool) {
	var failure *report.Failure
	if errors.As(err, &failure) {
		return failure, true
	}
	return nil, false
}
