package report

import (
	"slices"
	"strings"
	"testing"
)

// Every list-shaped behaviour of a document is a fan-out over Metrics, and
// with CRAP the one hosted metric no black-box case can hand it two. These
// build the two-entry document directly, since Document is plain data.

// scoredRow is one measured method at score, which is what makes an entry pass
// or fail its own threshold.
func scoredRow(name string, score float64) Row {
	coverage := 0.0
	return Row{
		File: "src/Ordering/OrderService.cs", Start: 1, End: 20, Name: name,
		Complexity: 4, Coverage: &coverage, Score: &score,
		State: StateMeasured, Action: "raise_coverage",
	}
}

// twoMetrics is one passing entry and one failing one over the same method,
// the shape that separates "any metric failing fails the run" from "the first
// metric decides".
func twoMetrics(firstThreshold, secondThreshold int) []Metric {
	return []Metric{
		{Name: "first", Display: "FIRST", Threshold: firstThreshold, Rows: []Row{scoredRow("Cancel", 20)}},
		{Name: "second", Display: "SECOND", Threshold: secondThreshold, Rows: []Row{scoredRow("Cancel", 20)}},
	}
}

func TestStatusFailsWhenAnyMetricFails(t *testing.T) {
	cases := []struct {
		name                            string
		firstThreshold, secondThreshold int
		want                            string
		wantExit                        int
	}{
		{name: "neither entry is over its bar", firstThreshold: 30, secondThreshold: 25, want: "pass", wantExit: 0},
		{name: "the last entry is over its bar", firstThreshold: 30, secondThreshold: 12, want: "fail", wantExit: 2},
		{name: "the first entry is over its bar", firstThreshold: 12, secondThreshold: 30, want: "fail", wantExit: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := Document{ChangedMethods: 1, Metrics: twoMetrics(tc.firstThreshold, tc.secondThreshold)}

			if got := doc.Status(); got != tc.want {
				t.Errorf("Status() = %q, want %q", got, tc.want)
			}
			if got := doc.ExitCode(); got != tc.wantExit {
				t.Errorf("ExitCode() = %d, want %d", got, tc.wantExit)
			}
		})
	}
}

func TestStderrReportsOneLinePerMetricInOrder(t *testing.T) {
	doc := Document{ChangedMethods: 1, Metrics: twoMetrics(30, 12)}

	want := "0 of 1 changed methods over FIRST threshold 30, worst score 20.00\n" +
		"1 of 1 changed methods over SECOND threshold 12, worst score 20.00\n"
	if got := doc.Stderr(); got != want {
		t.Errorf("Stderr() = %q, want %q", got, want)
	}
}

// TestStderrSaysTheChangedSetIsEmptyOnceForTheWholeRun pins the sentence as a
// statement about the set, not about any metric's reading of it, so it appears
// once however many metrics were selected.
func TestStderrSaysTheChangedSetIsEmptyOnceForTheWholeRun(t *testing.T) {
	doc := Document{ChangedMethods: 0, Metrics: twoMetrics(30, 12)}

	const want = "no changed methods, nothing to measure\n"
	if got := doc.Stderr(); got != want {
		t.Errorf("Stderr() = %q, want %q", got, want)
	}
}

// TestStdoutPutsEveryRowsTableAfterTheWholeSummary pins ADR 0008's key order
// in the document the gate publishes: the summary table carries a row per
// metric, and only then does each metric's own table follow, in the same
// order, so a reader reaches the verdict for the run before any detail.
func TestStdoutPutsEveryRowsTableAfterTheWholeSummary(t *testing.T) {
	doc := Document{ChangedMethods: 1, Metrics: twoMetrics(30, 12)}

	body, err := doc.Stdout()
	if err != nil {
		t.Fatalf("Stdout(): %v", err)
	}

	rendered := string(body)
	keys := topLevelKeys(rendered)
	want := []string{
		"status", "tool", "spec", "scope", "base", "changed_methods",
		"touched_lines_outside_spans", "skipped_paths", "metrics", "first", "second",
	}
	if !slices.Equal(keys, want) {
		t.Errorf("top-level keys = %v, want %v\n%s", keys, want, rendered)
	}
	// The summary table holds both metrics, so the two keys above are not two
	// documents' worth of rows under one summary row.
	wantSummary := "metrics[2|]{name|threshold|measured|failed}:\n  first|30|1|0\n  second|12|1|1\n"
	if !strings.Contains(rendered, wantSummary) {
		t.Errorf("document is missing the summary table\n%s\nwant\n%s", rendered, wantSummary)
	}
}

// topLevelKeys reads the document's own keys off the rendered TOON, which is
// the byte contract ADR 0008 fixes and gate/test/golden pins: a key sits at
// column zero and ends at the first `:` or `[`.
func topLevelKeys(rendered string) []string {
	var keys []string
	for _, line := range strings.Split(rendered, "\n") {
		if line == "" || strings.HasPrefix(line, " ") {
			continue
		}
		keys = append(keys, strings.FieldsFunc(line, func(r rune) bool { return r == ':' || r == '[' })[0])
	}
	return keys
}
