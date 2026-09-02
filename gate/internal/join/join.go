// Package join runs the smallest-containing-span rule in both directions: it
// scopes the extractor's spans to the touched lines (ADR 0003), and it
// attributes coverage lines to those spans (ADR 0001).
package join

import (
	"sort"

	"github.com/tvrmsmith/coding-standards/gate/internal/coverage"
	"github.com/tvrmsmith/coding-standards/gate/internal/extract"
	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// Changed returns the spans holding at least one touched line, and the count
// of touched lines falling inside no span. A line outside every span is never
// an unknown method; it is a diagnostic that never gates. Only files an
// extractor claimed are counted, since a line in a file no extractor handles
// is outside the measurement rather than outside a span.
func Changed(extracted extract.Result, touched map[srcpath.Path][]int) (changed []extract.Span, outsideSpans int) {
	byFile := groupByFile(extracted.Spans)
	inside := map[key]extract.Span{}
	for _, file := range sortedFiles(touched) {
		if !extracted.Claimed[file] {
			continue
		}
		for _, line := range touched[file] {
			span, ok := smallestContaining(byFile[file], line)
			if !ok {
				outsideSpans++
				continue
			}
			inside[keyOf(span)] = span
		}
	}
	for _, span := range inside {
		changed = append(changed, span)
	}
	sortSpans(changed)
	return changed, outsideSpans
}

// key identifies a span by file and line range, never by name, which is the
// reading ADR 0001 rejects.
type key struct {
	file  srcpath.Path
	start int
	end   int
}

func keyOf(span extract.Span) key {
	return key{file: span.File, start: span.StartLine, end: span.EndLine}
}

// smallestContaining picks the narrowest span holding line. Where spans nest,
// a local function's own span wins over its container's, which is what stops
// the container absorbing lines that are not its own.
func smallestContaining(spans []extract.Span, line int) (extract.Span, bool) {
	var best extract.Span
	found := false
	for _, span := range spans {
		if line < span.StartLine || line > span.EndLine {
			continue
		}
		if !found || width(span) < width(best) {
			best, found = span, true
		}
	}
	return best, found
}

func width(span extract.Span) int { return span.EndLine - span.StartLine }

// groupByFile indexes spans by the file they were found in.
func groupByFile(spans []extract.Span) map[srcpath.Path][]extract.Span {
	byFile := map[srcpath.Path][]extract.Span{}
	for _, span := range spans {
		byFile[span.File] = append(byFile[span.File], span)
	}
	return byFile
}

// sortedFiles keeps iteration over a path-keyed map deterministic.
func sortedFiles[V any](byFile map[srcpath.Path]V) []srcpath.Path {
	files := make([]srcpath.Path, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i] < files[j] })
	return files
}

// sortSpans orders spans by file then start line.
func sortSpans(spans []extract.Span) {
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].File != spans[j].File {
			return spans[i].File < spans[j].File
		}
		return spans[i].StartLine < spans[j].StartLine
	})
}

// Method is one changed method with everything the join could establish about
// it: its span, its state, and the coverage fraction backing the score.
type Method struct {
	Span extract.Span
	// State is measured, structural_na, or unknown.
	State report.State
	// Coverage is the fraction of the span's instrumentable lines that were
	// hit, and is meaningless when State is unknown.
	Coverage float64
	// Reason is the typed reason for an unknown state, empty otherwise.
	Reason string
}

// Attribute joins each changed span to the coverage set. It needs every span
// of the file, not only the changed ones, because a covered line belongs to
// the smallest span containing it and a container must not absorb the lines
// of a local function nested inside it.
//
// A span whose file matched no report path is unknown; a span holding no
// instrumentable line is structural_na, treated as fully covered, which is
// what makes trivial members exclude themselves arithmetically.
func Attribute(all []extract.Span, changed []extract.Span, lines coverage.Set) []Method {
	byFile := groupByFile(all)
	methods := make([]Method, 0, len(changed))
	for _, span := range changed {
		fileLines, matched := lines[span.File]
		if !matched {
			methods = append(methods, Method{
				Span:   span,
				State:  report.StateUnknown,
				Reason: report.ReasonFileUnmatched,
			})
			continue
		}
		instrumentable, covered := attributedTo(span, byFile[span.File], fileLines)
		if instrumentable == 0 {
			methods = append(methods, Method{Span: span, State: report.StateStructuralNA, Coverage: 1})
			continue
		}
		methods = append(methods, Method{
			Span:     span,
			State:    report.StateMeasured,
			Coverage: float64(covered) / float64(instrumentable),
		})
	}
	return methods
}

// attributedTo counts the instrumentable lines the smallest-containing-span
// rule gives to span, and how many of them were hit.
func attributedTo(span extract.Span, siblings []extract.Span, lines coverage.Lines) (instrumentable, covered int) {
	for line, hit := range lines {
		owner, ok := smallestContaining(siblings, line)
		if !ok || keyOf(owner) != keyOf(span) {
			continue
		}
		instrumentable++
		if hit {
			covered++
		}
	}
	return instrumentable, covered
}
