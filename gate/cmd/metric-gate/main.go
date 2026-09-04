// Command metric-gate scores the methods a change touched against a metric
// threshold. It takes no flags (ADR 0005): stdout is one TOON document,
// stderr is the human summary, one line except on the unknown_changed_method
// path, which prints the cause above the counts, and the exit code is 0 pass,
// 1 tool error, 2 threshold exceeded.
//
// The one exception to "stdout is one TOON document" is a failure upstream of
// the document itself, such as not being in a git repo at all. Those write the
// cause to stderr, leave stdout empty, and exit 1.
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
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

func main() {
	doc, err := measure(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	stdout, err := doc.Stdout()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Stdout.Write(stdout)
	io.WriteString(os.Stderr, doc.Stderr())
	os.Exit(doc.ExitCode())
}

// usageLine is the second line of every usage error this gate can print.
const usageLine = "usage: metric-gate [--coverage <path>]..."

// parseArgs reads the command line for the one flag this issue owns,
// --coverage, repeatable and accepted as either "--coverage <path>" or
// "--coverage=<path>". Anything else is a usage error, returned as a plain
// error so main prints it to stderr and writes no document (ADR 0005: a
// failure upstream of the document has no document).
func parseArgs(args []string) ([]string, error) {
	var coveragePaths []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--coverage":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--coverage needs a path\n%s", usageLine)
			}
			coveragePaths = append(coveragePaths, args[i])
		case strings.HasPrefix(arg, "--coverage="):
			value := strings.TrimPrefix(arg, "--coverage=")
			if value == "" {
				return nil, fmt.Errorf("--coverage needs a path\n%s", usageLine)
			}
			coveragePaths = append(coveragePaths, value)
		default:
			return nil, fmt.Errorf("unknown argument: %s\n%s", arg, usageLine)
		}
	}
	return coveragePaths, nil
}

// measure runs the gate over the repo containing the working directory. A
// typed exit-1 cause becomes the document's error block; only a problem the
// document cannot describe, such as not being in a repo at all, comes back as
// an error.
func measure(args []string) (report.Document, error) {
	var doc report.Document
	coveragePaths, err := parseArgs(args)
	if err != nil {
		return doc, err
	}

	repo, err := gitscope.Open()
	if err != nil {
		return doc, err
	}

	base, err := repo.ResolveBase()
	if err != nil {
		var noBase gitscope.NoBaseError
		if errors.As(err, &noBase) {
			doc.Failure = &report.Failure{Code: report.CodeNoDiffBase, Message: noBase.Error()}
			return doc, nil
		}
		return doc, err
	}
	label := base.Label()
	doc.Base = &label

	touched, err := repo.TouchedLines(base)
	if failure, ok := asFailure(err); ok {
		doc.Failure = failure
		return doc, nil
	} else if err != nil {
		return doc, err
	}

	extracted, err := extract.Extract(repo.Root(), changedFiles(touched))
	if failure, ok := asFailure(err); ok {
		doc.Failure = failure
		return doc, nil
	} else if err != nil {
		return doc, err
	}

	changed, outsideSpans := join.Changed(extracted, touched)
	doc.ChangedMethods = len(changed)
	doc.TouchedLinesOutsideSpans = outsideSpans
	metric := report.Metric{Name: crap.Name, Display: crap.DisplayName, Threshold: crap.Threshold}
	// ADR 0003: an empty changed-method set exits 0 before resolving any
	// input, because a metric with nothing to compute is not asking for one.
	if len(changed) == 0 {
		doc.Metric = &metric
		return doc, nil
	}

	newest, err := newestEdit(repo.Root(), changed)
	if err != nil {
		return doc, err
	}

	lines, skipped, err := loadCoverage(repo.Root(), coveragePaths, newest)
	doc.SkippedPaths = skipped
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

// loadCoverage resolves the coverage input, and only because
// crap.DeclaredInputs names it. The failure names the metric that is stuck
// rather than the file that is absent (ADR 0002). The paths discovery could
// not read come back alongside, including on the missing-report failure,
// where they are the likeliest explanation for it. When the developer named
// any report, discovery does not run at all: skipped_paths stays empty, and
// the named sources go straight to Load.
func loadCoverage(root srcpath.Root, named []string, newest coverage.Newest) (coverage.Set, []string, error) {
	if !slices.Contains(crap.DeclaredInputs, inputCoverage) {
		return nil, nil, nil
	}
	if len(named) > 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, nil, err
		}
		set, err := coverage.Load(root, coverage.Named(cwd, named), newest)
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
	set, err := coverage.Load(root, sources, newest)
	return set, skipped, err
}

// newestEdit stats the working-tree file of every distinct span file among
// the changed methods, and returns the greatest modification time, truncated
// to whole seconds, along with the file that holds it. A tie on equal times
// is broken on the lexicographically smallest path, so the message a stale
// report gets is deterministic.
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
		switch {
		case newest.File == "", at.After(newest.At):
			newest = coverage.Newest{File: span.File, At: at}
		case at.Equal(newest.At) && span.File < newest.File:
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

// asFailure unwraps a typed exit-1 cause out of an error.
func asFailure(err error) (*report.Failure, bool) {
	var failure *report.Failure
	if errors.As(err, &failure) {
		return failure, true
	}
	return nil, false
}
