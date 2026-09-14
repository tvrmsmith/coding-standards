package main

import (
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/coverage"
	"github.com/tvrmsmith/coding-standards/gate/internal/extract"
	"github.com/tvrmsmith/coding-standards/gate/internal/metric"
	"github.com/tvrmsmith/coding-standards/gate/internal/report"
)

// cancelSpan is the one changed method every case below attributes, and
// coveredLines is a coverage set that measures it: four instrumentable lines,
// all hit. They are built here rather than read off a repo, because what is
// under test is which of them the attribution consults, not how either is
// gathered.
var (
	cancelSpan = extract.Span{
		File: "src/Ordering/OrderService.cs", Name: "Cancel",
		StartLine: 10, EndLine: 20, Complexity: 4,
	}
	coveredLines = coverage.Set{
		cancelSpan.File: coverage.Lines{10: true, 12: true, 15: true, 20: true},
	}
)

// TestASelectionDeclaringNoCoverageScoresWithoutTheJoin is issue 19's second
// acceptance scenario at the seam that can observe it while CRAP is the one
// hosted metric: a metric needing no coverage runs in a repo where nobody ran
// the tests. loadCoverage hands such a selection no coverage set (ADR 0002),
// and the join reads an absent set as every file unmatched, so feeding it one
// would fail the run over an input nobody asked for.
func TestASelectionDeclaringNoCoverageScoresWithoutTheJoin(t *testing.T) {
	selected := []metric.Selection{declaresNothing}

	metrics, unknown := attribute(selected, extract.Result{Spans: []extract.Span{cancelSpan}}, []extract.Span{cancelSpan}, nil)

	if unknown != 0 {
		t.Errorf("unknown = %d, want 0: nothing was demanded, so nothing is unattributed", unknown)
	}
	if len(metrics) != 1 || len(metrics[0].Rows) != 1 {
		t.Fatalf("metrics = %+v, want one entry holding one row", metrics)
	}
	// Nothing is silently skipped: the method is still reported, carrying the
	// cells that come off the span alone.
	row := metrics[0].Rows[0]
	want := report.Row{
		File: cancelSpan.File, Start: cancelSpan.StartLine, End: cancelSpan.EndLine,
		Name: cancelSpan.Name, Complexity: cancelSpan.Complexity,
	}
	if row != want {
		t.Errorf("row = %+v, want %+v: every coverage-derived cell null", row, want)
	}
	if metrics[0].Measured() != 0 || metrics[0].Failed() != 0 {
		t.Errorf("measured = %d, failed = %d, want 0 and 0: no reading was taken",
			metrics[0].Measured(), metrics[0].Failed())
	}
}

// TestOneDeclaringMetricMakesTheJoinRunForAllOfThem pins the gate as the union
// of the declarations rather than each metric's own. A mixed selection has a
// coverage set loaded for it, so every metric in it is scored off that set.
func TestOneDeclaringMetricMakesTheJoinRunForAllOfThem(t *testing.T) {
	selected := []metric.Selection{declaresCoverage, declaresNothing}

	metrics, unknown := attribute(selected, extract.Result{Spans: []extract.Span{cancelSpan}}, []extract.Span{cancelSpan}, coveredLines)

	if unknown != 0 {
		t.Errorf("unknown = %d, want 0: the file matched the report", unknown)
	}
	if len(metrics) != 2 {
		t.Fatalf("metrics = %+v, want two entries", metrics)
	}
	for i, m := range metrics {
		if len(m.Rows) != 1 || m.Rows[0].State != report.StateMeasured || m.Rows[0].Score == nil {
			t.Errorf("metrics[%d] (%s) rows = %+v, want one measured, scored row", i, m.Name, m.Rows)
		}
	}
}

// TestEveryMetricScoresTheSameMethodSet pins the fan-out: a row per method per
// selection, in selection order, so a second metric cannot be left holding the
// first one's rows or none at all.
func TestEveryMetricScoresTheSameMethodSet(t *testing.T) {
	placeSpan := extract.Span{
		File: "src/Ordering/OrderService.cs", Name: "Place",
		StartLine: 30, EndLine: 40, Complexity: 2,
	}
	lines := coverage.Set{cancelSpan.File: coverage.Lines{10: true, 12: false, 30: true, 35: false}}
	selected := []metric.Selection{declaresCoverage, declaresCoverageToo}

	metrics, _ := attribute(selected, extract.Result{Spans: []extract.Span{cancelSpan, placeSpan}}, []extract.Span{cancelSpan, placeSpan}, lines)

	if len(metrics) != 2 {
		t.Fatalf("metrics = %+v, want two entries", metrics)
	}
	for i, want := range []string{declaresCoverage.Name, declaresCoverageToo.Name} {
		if metrics[i].Name != want {
			t.Errorf("metrics[%d].Name = %q, want %q", i, metrics[i].Name, want)
		}
		if len(metrics[i].Rows) != 2 {
			t.Fatalf("metrics[%d] rows = %+v, want one per changed method", i, metrics[i].Rows)
		}
		if metrics[i].Rows[0].Name != cancelSpan.Name || metrics[i].Rows[1].Name != placeSpan.Name {
			t.Errorf("metrics[%d] rows name %q then %q, want %q then %q",
				i, metrics[i].Rows[0].Name, metrics[i].Rows[1].Name, cancelSpan.Name, placeSpan.Name)
		}
	}
}
