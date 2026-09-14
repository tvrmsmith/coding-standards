// Package assertions carries the shapes combineassertions must catch and the ones it must not.
//
// analysistest fails on an unexpected diagnostic as loudly as on a missing one, so every
// function here that has no `want` comment is a false positive the rule is being held to.
package assertions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type page struct {
	Title string
	Size  int
	Owner owner
}

type owner struct{ Name string }

func load() page { return page{} }

func TestPickedApart(t *testing.T) {
	got := load()

	assert.Equal(t, "home", got.Title) // want `2 assertions in a row pick apart "got"`
	assert.Equal(t, 3, got.Size)
}

func TestPickedApartAcrossAssertAndRequire(t *testing.T) {
	got := load()

	require.Equal(t, "home", got.Title) // want `3 assertions in a row pick apart "got"`
	assert.Equal(t, 3, got.Size)
	assert.Equal(t, "trevor", got.Owner.Name)
}

// TestPickedApartThroughAssertions covers the receiver spelling: the same method reached
// through *assert.Assertions rather than the package function, which is also how a suite's own
// s.Equal resolves.
func TestPickedApartThroughAssertions(t *testing.T) {
	a := assert.New(t)
	got := load()

	a.Equal("home", got.Title) // want `2 assertions in a row pick apart "got"`
	a.Equal(3, got.Size)
}

func TestCountThenIndex(t *testing.T) {
	pages := []page{}

	assert.Len(t, pages, 2) // want `the length of "pages" is asserted and then an element of it is indexed out`
	assert.Equal(t, "home", pages[0].Title)
}

// TestCountThenIndexApart is the two halves overlapping. The length assertion and the two
// element assertions are all reported, because the rewrite that fixes one fixes all three.
func TestCountThenIndexApart(t *testing.T) {
	pages := []page{}
	first := page{}

	assert.Len(t, pages, 2)              // want `the length of "pages" is asserted`
	assert.Equal(t, "home", first.Title) // want `2 assertions in a row pick apart "first"`
	assert.Equal(t, pages[0].Size, first.Size)
}

// --- the shapes that must stay silent ---

// separateObjects asserts one field each on two objects, which is not one object picked apart.
func separateObjects(t *testing.T) {
	got := load()
	other := load()

	assert.Equal(t, "home", got.Title)
	assert.Equal(t, 3, other.Size)
}

// interrupted has real work between the assertions, so the two are separate arrangements.
func interrupted(t *testing.T) {
	got := load()

	assert.Equal(t, "home", got.Title)
	got = load()
	assert.Equal(t, 3, got.Size)
}

// wholeValues asserts on values rather than fields, which is already the shape the rule asks for.
func wholeValues(t *testing.T) {
	got := load()
	err := error(nil)
	ok := true

	assert.Equal(t, page{}, got)
	assert.NoError(t, err)
	assert.True(t, ok)
}

// throughCalls reaches each field through its own call, so there is no one object to combine on.
func throughCalls(t *testing.T) {
	assert.Equal(t, "home", load().Title)
	assert.Equal(t, 3, load().Size)
}

// countAlone asserts a length and never indexes, which is a complete assertion on its own.
func countAlone(t *testing.T) {
	pages := []page{}

	assert.Len(t, pages, 2)
	assert.Empty(t, pages)
}

// indexAlone picks one element out without having asserted the length first.
func indexAlone(t *testing.T) {
	pages := []page{{}}

	assert.Equal(t, "home", pages[0].Title)
}
