package join_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/coverage"
	"github.com/tvrmsmith/coding-standards/gate/internal/extract"
	"github.com/tvrmsmith/coding-standards/gate/internal/join"
	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// TestChangedReturnsSpansInAscendingOrder pins the order Changed promises its
// callers. Changed collects its spans in a Go map, whose iteration order the
// runtime randomises per run, so the returned order is either the sort's doing
// or nothing at all. newestEdit in cmd/metric-gate reads the first span of an
// equal-times tie as the smallest path, and the file it names in the
// coverage_stale message is what a dropped sort would silently get wrong.
func TestChangedReturnsSpansInAscendingOrder(t *testing.T) {
	files := []srcpath.Path{"src/d.cs", "src/c.cs", "src/b.cs", "src/a.cs"}
	var spans []extract.Span
	touched := map[srcpath.Path][]int{}
	for _, file := range files {
		spans = append(spans,
			extract.Span{File: file, Name: "Second", StartLine: 10, EndLine: 15},
			extract.Span{File: file, Name: "First", StartLine: 1, EndLine: 5},
		)
		touched[file] = []int{11, 2}
	}
	extracted := extract.NewResult(spans, files)

	changed, outside := join.Changed(extracted, touched)

	if outside != 0 {
		t.Fatalf("touched lines outside every span: got %d, want 0", outside)
	}
	want := []string{
		"src/a.cs:1", "src/a.cs:10",
		"src/b.cs:1", "src/b.cs:10",
		"src/c.cs:1", "src/c.cs:10",
		"src/d.cs:1", "src/d.cs:10",
	}
	if got := labels(changed); !slices.Equal(got, want) {
		t.Errorf("Changed returned spans out of order\ngot:  %s\nwant: %s",
			strings.Join(got, " "), strings.Join(want, " "))
	}
}

// TestAllSpansMeasuresEveryExtractedSpanInAscendingOrder pins --files' half of
// the ADR 0007 rule: with no touched line to narrow against, every span the
// extractor found is changed, nested spans included, in the same ascending
// order Changed promises.
//
// The span in src/c.cs, which the claimed list does not name, is the asserted
// half of "claim membership is irrelevant here": AllSpans measures it anyway,
// because extract.collect is what refuses a path the extractor was not handed
// and a second filter here would only hide a regression in that check.
//
// The final check pins the clone. AllSpans sorts in place, so returning
// extracted.Spans itself would reorder the caller's slice, and join.Attribute
// later reads that same slice through smallestContaining, whose tie-break is
// first-wins over input order.
func TestAllSpansMeasuresEveryExtractedSpanInAscendingOrder(t *testing.T) {
	spans := []extract.Span{
		{File: "src/b.cs", Name: "Outer", StartLine: 1, EndLine: 20},
		{File: "src/a.cs", Name: "Second", StartLine: 30, EndLine: 40},
		{File: "src/c.cs", Name: "Unclaimed", StartLine: 1, EndLine: 4},
		{File: "src/b.cs", Name: "Local", StartLine: 5, EndLine: 8},
		{File: "src/a.cs", Name: "First", StartLine: 1, EndLine: 10},
	}
	original := labels(slices.Clone(spans))
	extracted := extract.NewResult(spans, []srcpath.Path{"src/b.cs", "src/a.cs"})

	changed := join.AllSpans(extracted)

	want := []string{"src/a.cs:1", "src/a.cs:30", "src/b.cs:1", "src/b.cs:5", "src/c.cs:1"}
	if got := labels(changed); !slices.Equal(got, want) {
		t.Errorf("AllSpans returned spans out of order\ngot:  %s\nwant: %s",
			strings.Join(got, " "), strings.Join(want, " "))
	}
	if got := labels(extracted.Spans); !slices.Equal(got, original) {
		t.Errorf("AllSpans reordered the caller's spans\ngot:  %s\nwant: %s",
			strings.Join(got, " "), strings.Join(original, " "))
	}
}

// TestChangedSkipsATouchedFileNoExtractorClaimed pins the other half of
// Changed's doc comment: a line in a file no extractor handles is outside the
// measurement, not outside a span, so it never inflates outsideSpans. Three
// touched README lines exercise that a whole unclaimed file, not just a
// single stray line, is excluded rather than counted.
func TestChangedSkipsATouchedFileNoExtractorClaimed(t *testing.T) {
	extracted := extract.NewResult([]extract.Span{
		{File: "src/a.cs", Name: "First", StartLine: 1, EndLine: 10},
	}, []srcpath.Path{"src/a.cs"})
	touched := map[srcpath.Path][]int{
		"src/a.cs":  {5},
		"README.md": {1, 2, 3},
	}

	changed, outside := join.Changed(extracted, touched)

	if outside != 0 {
		t.Fatalf("touched lines outside every span: got %d, want 0", outside)
	}
	want := []string{"src/a.cs:1"}
	if got := labels(changed); !slices.Equal(got, want) {
		t.Errorf("Changed returned unexpected spans\ngot:  %s\nwant: %s",
			strings.Join(got, " "), strings.Join(want, " "))
	}
}

// TestAttributeReportsFileUninstrumentedForAFileWithNoInstrumentableLines
// pins the case coverage.mergeInto's zero-line entry creates: a report that
// lists the file but names no instrumentable line inside it never
// instrumented the file at all, so the span cannot be read as trivially
// covered the way an instrumented file's empty span is. It must come back
// unknown, not structural_na, and carry no coverage.
func TestAttributeReportsFileUninstrumentedForAFileWithNoInstrumentableLines(t *testing.T) {
	span := extract.Span{File: "src/a.cs", Name: "Empty", StartLine: 1, EndLine: 5}
	lines := coverage.Set{"src/a.cs": coverage.Lines{}}

	methods := join.Attribute([]extract.Span{span}, []extract.Span{span}, lines)

	if len(methods) != 1 {
		t.Fatalf("Attribute returned %d methods, want 1", len(methods))
	}
	want := join.Method{Span: span, State: report.StateUnknown, Reason: report.ReasonFileUninstrumented}
	if methods[0] != want {
		t.Errorf("Attribute returned %+v, want %+v", methods[0], want)
	}
}

// labels renders each span as the file and start line the order is asserted
// on, so a failure prints the sequence rather than a struct dump.
func labels(spans []extract.Span) []string {
	rendered := make([]string, 0, len(spans))
	for _, span := range spans {
		rendered = append(rendered, fmt.Sprintf("%s:%d", span.File, span.StartLine))
	}
	return rendered
}
