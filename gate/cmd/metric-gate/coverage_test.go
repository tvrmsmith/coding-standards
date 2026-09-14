package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/extract"
	"github.com/tvrmsmith/coding-standards/gate/internal/metric"
	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// The selections below are built here rather than read out of metric.Hosted,
// so the cases state the declaration they are testing instead of depending on
// what the catalogue happens to hold. Only the Inputs field differs between
// declaresCoverage and declaresNothing.
var (
	declaresCoverage = metric.Selection{
		Definition: metric.Definition{Name: "declares", Display: "DECLARES", Inputs: []metric.Input{metric.InputCoverage}},
		Threshold:  30,
	}
	declaresCoverageToo = metric.Selection{
		Definition: metric.Definition{Name: "declares-too", Display: "DECLARES-TOO", Inputs: []metric.Input{metric.InputCoverage}},
		Threshold:  30,
	}
	declaresNothing = metric.Selection{
		Definition: metric.Definition{Name: "declares-nothing", Display: "DECLARES-NOTHING"},
		Threshold:  30,
	}
)

// TestOnlyADeclaredCoverageInputIsDemanded is the reads-implies-declares half
// of issue 19's drift test, and ADR 0002's rule at the only seam that can
// observe it while CRAP is the one hosted metric. One root with no coverage
// report anywhere, two calls differing in nothing but the declaration: the
// declaration alone decides whether an absent report blocks the run.
func TestOnlyADeclaredCoverageInputIsDemanded(t *testing.T) {
	root, changed := rootWithNoCoverageReport(t)

	t.Run("a selection declaring no input skips coverage entirely", func(t *testing.T) {
		set, skipped, err := loadCoverage(root, nil, changed, []metric.Selection{declaresNothing})

		// Nothing was demanded, so nothing was skipped and the run is free to
		// pass in a repo where nobody ran the tests.
		if set != nil || skipped != nil || err != nil {
			t.Errorf("loadCoverage = (%v, %v, %v), want (nil, nil, nil)", set, skipped, err)
		}
	})

	t.Run("a selection declaring coverage fails on the absent report", func(t *testing.T) {
		_, _, err := loadCoverage(root, nil, changed, []metric.Selection{declaresCoverage})

		var failure *report.Failure
		if !errors.As(err, &failure) || failure.Code != report.CodeCoverageMissing {
			t.Fatalf("loadCoverage err = %v, want a report.Failure coded %s", err, report.CodeCoverageMissing)
		}
	})

	// ADR 0002 names every metric the absent report stops, not only the first,
	// so the developer learns what the one `dotnet test` run would unblock.
	t.Run("the failure names every metric stuck on the absent report", func(t *testing.T) {
		_, _, err := loadCoverage(root, nil, changed, []metric.Selection{declaresCoverage, declaresCoverageToo})

		var failure *report.Failure
		if !errors.As(err, &failure) {
			t.Fatalf("loadCoverage err = %v, want a report.Failure", err)
		}
		const want = "DECLARES, DECLARES-TOO requires a coverage report"
		if !strings.HasPrefix(failure.Message, want) {
			t.Errorf("failure.Message = %q, want it to begin %q", failure.Message, want)
		}
	})
}

// TestEveryHostedDeclarationIsImplemented is the third part of issue 19's
// drift test. A metric declaring an input this binary does not load would be
// ignored in silence, since nothing would go looking and nothing would fail,
// so the catalogue is checked against implementedInputs rather than against
// discipline (ADR 0002).
func TestEveryHostedDeclarationIsImplemented(t *testing.T) {
	for _, def := range metric.Hosted() {
		for _, input := range def.Inputs {
			if !slices.Contains(implementedInputs, input) {
				t.Errorf("metric %q declares input %q, which this binary does not load; implementedInputs = %v",
					def.Name, input, implementedInputs)
			}
		}
	}
}

// rootWithNoCoverageReport is a repo root holding the one source file the
// changed span sits in and no coverage report anywhere, which is the fixture
// both halves of the declaration case run against.
func rootWithNoCoverageReport(t *testing.T) (srcpath.Root, []extract.Span) {
	t.Helper()
	dir := t.TempDir()
	const rel = "src/Ordering/OrderService.cs"
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("// source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := srcpath.NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return root, []extract.Span{{File: rel, Name: "OrderService.Cancel", StartLine: 1, EndLine: 1, Complexity: 1}}
}
