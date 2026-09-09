package extract_test

import (
	"slices"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/extract"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// TestClaimsAnswersMembership pins the query join.Changed asks per touched
// file. A Markdown file answering false is the point: a line in a file no
// extractor claims is outside the measurement entirely, not merely outside a
// span, and it is Claims that draws that line.
func TestClaimsAnswersMembership(t *testing.T) {
	result := extract.NewResult(nil, []srcpath.Path{"src/a.cs", "src/b.cs"})

	if !result.Claims("src/a.cs") {
		t.Errorf("Claims(%q) = false, want true", "src/a.cs")
	}
	if !result.Claims("src/b.cs") {
		t.Errorf("Claims(%q) = false, want true", "src/b.cs")
	}
	if result.Claims("docs/notes.md") {
		t.Errorf("Claims(%q) = true, want false", "docs/notes.md")
	}
}

// TestClaimedPathsSortsAndCollapsesDuplicates pins the order a --staged run
// promises: it checks these files for divergence and names them in a refusal
// message the goldens pin, so the order has to be a promise ClaimedPaths
// keeps, not an accident of map iteration. The duplicate comes from two
// extractors claiming the same path, which must collapse to one entry rather
// than naming a file twice.
func TestClaimedPathsSortsAndCollapsesDuplicates(t *testing.T) {
	result := extract.NewResult(nil, []srcpath.Path{"src/d.cs", "src/a.cs", "src/c.cs", "src/a.cs"})

	got := result.ClaimedPaths()

	want := []srcpath.Path{"src/a.cs", "src/c.cs", "src/d.cs"}
	if !slices.Equal(got, want) {
		t.Errorf("ClaimedPaths() = %v, want %v", got, want)
	}
}

// TestUnclaimedIsTheDifferenceSorted pins the set that becomes skipped_paths
// in the document: a named file no extractor claims is neither measured nor
// an error, so it has to be listed rather than silently dropped, and the sort
// keeps that list reproducible.
func TestUnclaimedIsTheDifferenceSorted(t *testing.T) {
	result := extract.NewResult(nil, []srcpath.Path{"src/a.cs"})

	got := result.Unclaimed([]srcpath.Path{"src/z.cs", "docs/notes.md", "src/a.cs"})

	want := []srcpath.Path{"docs/notes.md", "src/z.cs"}
	if !slices.Equal(got, want) {
		t.Errorf("Unclaimed(...) = %v, want %v", got, want)
	}
}

// TestZeroResultAnswersEveryQueryWithoutPanicking pins the case Extract's
// failure paths depend on: Extract returns Result{} on every one of them, so a
// caller reporting on a failed run holds a Result whose map is nil. Before
// claimed was unexported a caller indexed the map directly and got this
// answer for free from Go's nil-map read; the queries have to keep that true
// or a failed run panics instead of printing its typed cause.
func TestZeroResultAnswersEveryQueryWithoutPanicking(t *testing.T) {
	var result extract.Result

	if result.Claims("src/a.cs") {
		t.Errorf("Claims(%q) on the zero Result = true, want false", "src/a.cs")
	}
	if got := result.ClaimedPaths(); len(got) != 0 {
		t.Errorf("ClaimedPaths() on the zero Result = %v, want empty", got)
	}
	got := result.Unclaimed([]srcpath.Path{"src/b.cs", "src/a.cs"})
	want := []srcpath.Path{"src/a.cs", "src/b.cs"}
	if !slices.Equal(got, want) {
		t.Errorf("Unclaimed(...) on the zero Result = %v, want %v", got, want)
	}
}
