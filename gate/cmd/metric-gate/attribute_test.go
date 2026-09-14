package main

import (
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/coverage"
	"github.com/tvrmsmith/coding-standards/gate/internal/crap"
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
	// uncoveredLines instruments the same four lines and records a hit on
	// none, so cancelSpan measures at coverage 0 and scores 20.
	uncoveredLines = coverage.Set{
		cancelSpan.File: coverage.Lines{10: false, 12: false, 15: false, 20: false},
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

	metrics, unknown := attribute(selected, extract.Result{Spans: []extract.Span{cancelSpan}}, []extract.Span{cancelSpan}, nil, false)

	if unknown != 0 {
		t.Errorf("unknown = %d, want 0: the join never ran, so nothing is unattributed", unknown)
	}
	if len(metrics) != 1 {
		t.Fatalf("metrics = %+v, want the one selected entry", metrics)
	}
	// The metric is still named, at its own bar, having read nothing. What a
	// row looks like for it waits on the formula that would fill one.
	if len(metrics[0].Rows) != 0 {
		t.Errorf("rows = %+v, want none", metrics[0].Rows)
	}
	if metrics[0].Measured() != 0 || metrics[0].Failed() != 0 {
		t.Errorf("measured = %d, failed = %d, want 0 and 0", metrics[0].Measured(), metrics[0].Failed())
	}
	// The run passes rather than dying on unknown_changed_method, which is the
	// whole of the scenario.
	doc := report.Document{ChangedMethods: len([]extract.Span{cancelSpan}), Metrics: metrics}
	if code := doc.ExitCode(); code != 0 {
		t.Errorf("ExitCode() = %d, want 0", code)
	}
}

// TestADeclaredRunScoresEveryMetricOffTheOneSet pins what attribute does once
// the declaration is in: it reads declared and never sel.Inputs, so a mixed
// selection is scored off the one loaded set rather than the metric that
// declared nothing being left rowless. Whether the mixed selection loads a set
// at all is loadCoverage's call, pinned in coverage_test.go.
func TestADeclaredRunScoresEveryMetricOffTheOneSet(t *testing.T) {
	selected := []metric.Selection{declaresCoverage, declaresNothing}

	metrics, unknown := attribute(selected, extract.Result{Spans: []extract.Span{cancelSpan}}, []extract.Span{cancelSpan}, coveredLines, true)

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

// TestOneUnattributableMethodCountsOnceWhateverIsSelected is why the count
// comes off the join rather than off the rows: a method nothing could
// attribute is unknown to the run, not once to every metric reading it. Two
// selections over one unmatched method must report one, not two.
func TestOneUnattributableMethodCountsOnceWhateverIsSelected(t *testing.T) {
	selected := []metric.Selection{declaresCoverage, declaresCoverageToo}
	// The set names some other file, so cancelSpan's file matched no report
	// path at all, which is ADR 0004's file_unmatched.
	elsewhere := coverage.Set{"src/Billing/InvoiceService.cs": coverage.Lines{1: true}}

	metrics, unknown := attribute(selected, extract.Result{Spans: []extract.Span{cancelSpan}}, []extract.Span{cancelSpan}, elsewhere, true)

	if unknown != 1 {
		t.Errorf("unknown = %d, want 1: one method, counted once however many metrics read it", unknown)
	}
	if len(metrics) != 2 {
		t.Fatalf("metrics = %+v, want two entries", metrics)
	}
	for i, m := range metrics {
		if len(m.Rows) != 1 || m.Rows[0].State != report.StateUnknown {
			t.Errorf("metrics[%d] (%s) rows = %+v, want one unknown row", i, m.Name, m.Rows)
		}
	}
}

// TestOneMethodIsJudgedAtEachSelectionsOwnBar is why rowFor takes a selection
// rather than a bare threshold: one method, one score, two verdicts. The score
// is a property of the method and must not move between the entries, while the
// action, the target and the failure count are readings of that score against
// a bar and must.
func TestOneMethodIsJudgedAtEachSelectionsOwnBar(t *testing.T) {
	selected := []metric.Selection{declaresCoverage, declaresCoverageToo}

	metrics, unknown := attribute(selected, extract.Result{Spans: []extract.Span{cancelSpan}}, []extract.Span{cancelSpan}, uncoveredLines, true)

	if unknown != 0 {
		t.Fatalf("unknown = %d, want 0: the method measured at coverage 0", unknown)
	}
	if len(metrics) != 2 || len(metrics[0].Rows) != 1 || len(metrics[1].Rows) != 1 {
		t.Fatalf("metrics = %+v, want two entries holding one row each", metrics)
	}
	loose, tight := metrics[0].Rows[0], metrics[1].Rows[0]

	if loose.Score == nil || tight.Score == nil || *loose.Score != 20 || *tight.Score != 20 {
		t.Fatalf("scores = %v and %v, want 20 for both: the bar does not move the score",
			loose.Score, tight.Score)
	}
	if loose.Action != crap.ActionNone || loose.TargetCoverage != nil {
		t.Errorf("at bar %d: action = %q, target = %v, want %q and nil",
			declaresCoverage.Threshold, loose.Action, loose.TargetCoverage, crap.ActionNone)
	}
	if tight.Action != crap.ActionRaiseCoverage || tight.TargetCoverage == nil || *tight.TargetCoverage != 0.207 {
		t.Errorf("at bar %d: action = %q, target = %v, want %q and 0.207",
			declaresCoverageToo.Threshold, tight.Action, tight.TargetCoverage, crap.ActionRaiseCoverage)
	}
	if metrics[0].Failed() != 0 || metrics[1].Failed() != 1 {
		t.Errorf("failed = %d and %d, want 0 and 1: the same method clears one bar and not the other",
			metrics[0].Failed(), metrics[1].Failed())
	}
	// One metric failing fails the run, so the document exits 2 rather than
	// reading only the entry that passed.
	doc := report.Document{ChangedMethods: 1, Metrics: metrics}
	if code := doc.ExitCode(); code != 2 {
		t.Errorf("ExitCode() = %d, want 2", code)
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

	metrics, _ := attribute(selected, extract.Result{Spans: []extract.Span{cancelSpan, placeSpan}}, []extract.Span{cancelSpan, placeSpan}, lines, true)

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
