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
	typed := &UnreadableDiffError{Message: `could not read the diff: git printed the numstat record "1 2 src/a.cs"`}

	got := unreadableDiff(fmt.Errorf("reading the pure moves: %w", typed))

	if got != error(typed) {
		t.Fatalf("unreadableDiff returned %v, want the cause's own UnreadableDiffError back", got)
	}
}
