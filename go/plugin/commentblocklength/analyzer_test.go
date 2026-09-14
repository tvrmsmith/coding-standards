package commentblocklength_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tvrmsmith/coding-standards/go/plugin/commentblocklength"
)

// One fixture carries every shape the rule has to separate: the over-budget block it exists to
// find, and beside it the four exempt or non-forming shapes that would each be a false positive.
// analysistest fails on an unexpected diagnostic as loudly as on a missing one, so the silent
// cases are asserted by the same run.
func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), commentblocklength.NewAnalyzer(commentblocklength.DefaultMax), "blocks")
}
