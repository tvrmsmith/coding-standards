package combineassertions_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tvrmsmith/coding-standards/go/plugin/combineassertions"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), combineassertions.NewAnalyzer(), "assertions")
}
