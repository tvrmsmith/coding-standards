package main

import (
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// Both callers hand dirtyMessage a sorted list today, so a black-box case
// cannot tell its sort from theirs and cannot go red when it is deleted. The
// rule the goldens depend on is held here, where the unsorted pair the callers
// never produce can be handed over directly.
func TestDirtyMessageSortsTheNamesItWasHanded(t *testing.T) {
	message := dirtyMessage([]srcpath.Path{"src/Ordering/Other.cs", "src/Ordering/OrderService.cs"})

	const want = "refusing to score src/Ordering/OrderService.cs, src/Ordering/Other.cs: staged in one state and on disk in another"
	if message != want {
		t.Errorf("dirtyMessage on an unsorted pair returned\n%q\nwant\n%q", message, want)
	}
}
