package nocomments_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tvrmsmith/coding-standards/go/plugin/nocomments"
)

// The fixture's `// want` lines are comments the rule reports too, so each one expects the
// diagnostic it raises itself, and a line holding two comments expects two.
func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), nocomments.NewAnalyzer(), "comments")
}
