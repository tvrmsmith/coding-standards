package fixtures

// The test-shaped fixtures. testifylint, thelper, tparallel, usetesting and the personal
// assertion rule all key on a test file, so these cannot live beside the others.
//
// They are written to pass when run, even though nothing runs them: `go test ./...` in this
// module is off the table anyway, because the printf fixture in errors.go would fail the vet
// pass `go test` does on its own. cases_test.go at the module root is the only test here.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type page struct {
	Title string
	Size  int
}

// TestCombineAssertions picks one object apart, two assertions where one would do.
func TestCombineAssertions(t *testing.T) {
	got := page{Title: "home", Size: 3}

	assert.Equal(t, "home", got.Title)
	assert.Equal(t, 3, got.Size)
}

// TestTestifylint asserts a length through len() instead of assert.Len, so a failure reports
// "2 != 3" without saying what was in the slice.
func TestTestifylint(t *testing.T) {
	pages := []page{{}, {}}

	assert.Equal(t, 2, len(pages))
}

// TestUsetesting builds a context by hand rather than taking the one the test already has, so
// it is not cancelled when the test ends.
func TestUsetesting(t *testing.T) {
	ctx := context.Background()

	assert.NotNil(t, ctx)
}

// TestTparallel runs its subtests in parallel without being parallel itself, so the subtests
// wait for every other sequential test in the package before they start.
func TestTparallel(t *testing.T) {
	t.Run("first", func(t *testing.T) {
		t.Parallel()
		assertReady(t, true)
	})
}

// assertReady is a helper that never marks itself one, so a failure inside it is reported at
// this line rather than at the call that caused it.
func assertReady(t *testing.T, ready bool) {
	assert.True(t, ready)
}
