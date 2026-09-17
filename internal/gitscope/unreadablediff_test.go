package gitscope

import (
	"fmt"
	"testing"
)

// A cause that already named itself an unreadable diff passes the boundary
// untouched. Re-wrapping it would prefix "could not read the diff: " onto a
// message that opens with those words, which is what the caller prints, so the
// document would carry the sentence twice.
func TestUnreadableDiffKeepsACauseThatAlreadyTypedItself(t *testing.T) {
	typed := UnreadableDiffError{Message: `could not read the diff: git printed the numstat record "1 2 src/a.cs"`}

	got := unreadableDiff(fmt.Errorf("reading the pure moves: %w", typed))

	// Compared as any rather than with errors.Is or errors.As, because those
	// unwrap and so would also pass if unreadableDiff had wrapped the cause a
	// second time. Identity of the returned value is the whole assertion.
	if any(got) != any(typed) {
		t.Fatalf("unreadableDiff returned %v, want the cause's own UnreadableDiffError back", got)
	}
}
