package gitscope

import "testing"

// TestOpenKindsListsEveryDeclaredKind pairs with
// TestOpenCodeMapsEveryDeclaredKind in gate/cmd/metric-gate: that test catches
// a kind appended to OpenKinds without a case added to openCode, and this one
// catches the other half, a kind added to the const block and never appended
// to OpenKinds. The count assertion is what catches it. Contiguity alone does
// not: a fifth const left out of the list leaves the list contiguous from
// zero, and only openKindCount moves.
func TestOpenKindsListsEveryDeclaredKind(t *testing.T) {
	kinds := OpenKinds()

	if len(kinds) != int(openKindCount) {
		t.Errorf("OpenKinds() lists %d kinds, want the %d the const block declares", len(kinds), openKindCount)
	}
	for i, kind := range kinds {
		if kind != OpenKind(i) {
			t.Errorf("OpenKinds()[%d] = %v, want %v: the list is not contiguous from zero", i, kind, OpenKind(i))
		}
	}
}
