package gitscope

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// The numstat parser is the one rule in this package a black-box case cannot
// reach, because a real git prints only the records it knows how to print. The
// refusal exists for the release that changes the shape, so it is driven from
// here with the wire text git would have written.
func TestNumstatPathsReadsThePathAfterTheTwoCounts(t *testing.T) {
	named, err := parseNumstatPaths("1\t2\tsrc/a.cs\x00")

	if err != nil || !slices.Equal(sortedPaths(named), []srcpath.Path{"src/a.cs"}) {
		t.Errorf("parseNumstatPaths on one record returned %v, %v, want src/a.cs and no error", named, err)
	}
}

func TestNumstatPathsReadsABinaryRecordLikeAnyOther(t *testing.T) {
	// git counts no lines for a binary difference and writes "-" for both, so a
	// parser reading the counts as numbers would refuse a path that did diverge.
	named, err := parseNumstatPaths("-\t-\tsrc/bin.dat\x00")

	if err != nil || !slices.Equal(sortedPaths(named), []srcpath.Path{"src/bin.dat"}) {
		t.Errorf("parseNumstatPaths on a binary record returned %v, %v, want src/bin.dat and no error", named, err)
	}
}

func TestNumstatPathsKeepsATabInsideAPath(t *testing.T) {
	// core.quotePath is pinned false, so a filename holding a tab arrives raw.
	// Split on every tab rather than the first two, the path would come back
	// truncated and the file it names would pass the divergence guard.
	named, err := parseNumstatPaths("1\t2\tsrc/od\td.cs\x001\t0\tsrc/b.cs\x00")

	want := []srcpath.Path{"src/b.cs", "src/od\td.cs"}
	if err != nil || !slices.Equal(sortedPaths(named), want) {
		t.Errorf("parseNumstatPaths on a path holding a tab returned %v, %v, want %v and no error", named, err, want)
	}
}

func TestNumstatPathsRefusesARecordItDoesNotKnow(t *testing.T) {
	named, err := parseNumstatPaths("1 2 src/a.cs\x00")

	var failure *report.Failure
	if named != nil || !errors.As(err, &failure) {
		t.Fatalf("parseNumstatPaths on a malformed record returned %v, %v, want no paths and a typed failure", named, err)
	}
	const wantMessage = `could not read the diff: git printed the numstat record "1 2 src/a.cs"`
	if failure.Code != report.CodeDiffUnparseable || failure.Message != wantMessage {
		t.Errorf("the refusal is %s %q, want %s %q", failure.Code, failure.Message, report.CodeDiffUnparseable, wantMessage)
	}
}

// sortedPaths is the map read back as the ordered list a case can compare.
func sortedPaths(named map[srcpath.Path]bool) []srcpath.Path {
	return slices.Sorted(maps.Keys(named))
}
