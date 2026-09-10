// Package join runs the smallest-containing-span rule in both directions: it
// scopes the extractor's spans to the touched lines (ADR 0003), and it
// attributes coverage lines to those spans (ADR 0001).
package join

import (
	"maps"
	"slices"
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
//
// The spans come back in the total order sortSpans defines, ascending by file
// first. That is a promise to callers rather than only the row determinism
// the printed table needs. cmd/metric-gate reads the first span of an
// equal-mtime tie as the smallest path when it names a file in the
// coverage_stale message, and nothing else tells it what it is holding.
func Changed(extracted extract.Result, touched map[srcpath.Path][]int) (changed []extract.Span, outsideSpans int) {
	byFile := groupByFile(extracted.Spans)
	inside := map[extract.Span]bool{}
	for _, file := range sortedFiles(touched) {
		if !extracted.Claims(file) {
			continue
		}
		for _, line := range touched[file] {
			narrowest := narrowestContaining(byFile[file], line)
			if len(narrowest) == 0 {
				outsideSpans++
				continue
			}
			for _, span := range narrowest {
				inside[span] = true
			}
		}
	}
	for span := range inside {
		changed = append(changed, span)
	}
	sortSpans(changed)
	return changed, outsideSpans
}

// AllSpans returns every span extracted holds, in ascending order. This is the
// --files rule: the flag carries no line information, so ADR 0007 makes every
// method in a listed file changed, nested spans included, since with no touched
// line there is nothing to attribute to the smallest container the way Changed's
// narrowing would.
//
// The file list is already established upstream: extract.collect refuses any
// path the extractor echoed but was not handed, exiting 1 with
// extractor_path_mismatch.
func AllSpans(extracted extract.Result) []extract.Span {
	changed := slices.Clone(extracted.Spans)
	sortSpans(changed)
	return changed
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

// narrowestContaining returns every span of minimal width holding line. Where
// spans nest, a local function's own span wins over its container's, which is
// what stops the container absorbing lines that are not its own. Where two
// distinct methods share the same range, as two one-line methods declared on
// one line do, both win and both are scored.
func narrowestContaining(spans []extract.Span, line int) []extract.Span {
	var best []extract.Span
	for _, span := range spans {
		if line < span.StartLine || line > span.EndLine {
			continue
		}
		switch {
		case len(best) == 0 || width(span) < width(best[0]):
			best = []extract.Span{span}
		case width(span) == width(best[0]):
			best = append(best, span)
		}
	}
	return best
}

// smallestContaining picks one narrowest span holding line, in extractor
// order. Its callers compare ranges, and the tie this resolves is between two
// methods declared on the same lines, so the pick does not change the range.
func smallestContaining(spans []extract.Span, line int) (extract.Span, bool) {
	narrowest := narrowestContaining(spans, line)
	if len(narrowest) == 0 {
		return extract.Span{}, false
	}
	return narrowest[0], true
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
	return slices.Sorted(maps.Keys(byFile))
}

// sortSpans puts spans in a total order, so the set Changed drains out of a
// map lands in the table the same way every run. File, start and end alone
// leave two methods declared on the same lines tied, and two overloads tie on
// the name as well, so the signature settles it. The name is the arm that keeps
// two same-range methods from swapping rows between runs; two spans that get
// past it share every cell the table prints, since the coverage join keys on
// the range alone, so what the signature buys is a total order for sort.Slice
// rather than a visible one.
func sortSpans(spans []extract.Span) {
	sort.Slice(spans, func(i, j int) bool {
		left, right := spans[i], spans[j]
		switch {
		case left.File != right.File:
			return left.File < right.File
		case left.StartLine != right.StartLine:
			return left.StartLine < right.StartLine
		case left.EndLine != right.EndLine:
			return left.EndLine < right.EndLine
		case left.Name != right.Name:
			return left.Name < right.Name
		default:
			return left.Signature < right.Signature
		}
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
// A span whose file matched no report path is unknown. A span whose file
// matched but whose entry lists no instrumentable line at all is unknown too,
// because the report never instrumented that file rather than having found
// it trivial, and reading it as covered would be the same broken join issue
// 17 exists to close. Only once the file is confirmed instrumented does an
// empty span turn structural_na, treated as fully covered, which is what
// makes trivial members exclude themselves arithmetically.
//
// Under default coverlet options a file of nothing but trivial members does
// not reach the second arm. Measured against a real `dotnet test
// --collect:"XPlat Code Coverage"` run over a record with only positional
// members and a class with two auto-implemented properties, none of them
// touched by a test, coverlet emits a <class> per type carrying class-level
// <lines> with one hits="0" entry per auto-property getter. So such a file is
// instrumented and its entry is never empty, and its getters read measured at
// coverage 0.
//
// Two shapes still reach the arm. Coverlet run with SkipAutoProps=true
// suppresses exactly the auto-property getter lines that evidence rests on.
// And a producer that writes lines solely under <methods>, leaving the
// class-level element empty, which coverlet, the producer this gate reads,
// does not do. Under either one a changed method in such a file exits 1
// rather than scoring as covered. That is the accepted cost of refusing to
// read an uninstrumented file as covered. A file whose only type carries
// [ExcludeFromCodeCoverage] is not one of them: coverlet leaves its <class>
// out of the report altogether, so the set holds no entry for the file and
// its spans take the first arm, file_unmatched, which also exits 1.
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
		if len(fileLines) == 0 {
			methods = append(methods, Method{
				Span:   span,
				State:  report.StateUnknown,
				Reason: report.ReasonFileUninstrumented,
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
