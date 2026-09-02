// Command metric-gate scores the methods a change touched against a metric
// threshold. It takes no flags (ADR 0005): stdout is one TOON document,
// stderr is one summary line, and the exit code is 0 pass, 1 tool error, 2
// threshold exceeded.
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

	"github.com/tvrmsmith/coding-standards/gate/internal/coverage"
	"github.com/tvrmsmith/coding-standards/gate/internal/crap"
	"github.com/tvrmsmith/coding-standards/gate/internal/extract"
	"github.com/tvrmsmith/coding-standards/gate/internal/gitscope"
	"github.com/tvrmsmith/coding-standards/gate/internal/join"
	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

func main() {
	doc, err := measure()
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

// measure runs the gate over the repo containing the working directory. A
// typed exit-1 cause becomes the document's error block; only a problem the
// document cannot describe, such as not being in a repo at all, comes back as
// an error.
func measure() (report.Document, error) {
	var doc report.Document
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
	if err != nil {
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

	lines, skipped, err := loadCoverage(repo.Root())
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
// where they are the likeliest explanation for it.
func loadCoverage(root srcpath.Root) (coverage.Set, []string, error) {
	if !slices.Contains(crap.DeclaredInputs, inputCoverage) {
		return nil, nil, nil
	}
	reports, skipped, err := coverage.Discover(root)
	if err != nil {
		return nil, nil, err
	}
	if len(reports) == 0 {
		return nil, skipped, &report.Failure{
			Code:    report.CodeCoverageMissing,
			Message: crap.DisplayName + " requires a coverage report, none found",
		}
	}
	set, err := coverage.Load(root, reports)
	return set, skipped, err
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
