package join_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/extract"
	"github.com/tvrmsmith/coding-standards/gate/internal/join"
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
	extracted := extract.Result{Claimed: map[srcpath.Path]bool{}}
	touched := map[srcpath.Path][]int{}
	for _, file := range files {
		extracted.Spans = append(extracted.Spans,
			extract.Span{File: file, Name: "Second", StartLine: 10, EndLine: 15},
			extract.Span{File: file, Name: "First", StartLine: 1, EndLine: 5},
		)
		extracted.Claimed[file] = true
		touched[file] = []int{11, 2}
	}

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

// labels renders each span as the file and start line the order is asserted
// on, so a failure prints the sequence rather than a struct dump.
func labels(spans []extract.Span) []string {
	rendered := make([]string, 0, len(spans))
	for _, span := range spans {
		rendered = append(rendered, fmt.Sprintf("%s:%d", span.File, span.StartLine))
	}
	return rendered
}
