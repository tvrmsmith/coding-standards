package crap_test

import (
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/crap"
)

// TestMeasurement covers the eight cases the design pins in the assignment,
// including the two pairs that hold complexity and coverage fixed and change
// only the threshold: the score must not move while the verdict does, which
// is the whole point of moving Threshold onto Measurement.
func TestMeasurement(t *testing.T) {
	f := func(v float64) *float64 { return &v }

	cases := []struct {
		name           string
		complexity     int
		coverage       float64
		threshold      int
		score          float64
		action         string
		targetCoverage *float64
	}{
		{"low complexity zero coverage clears a generous threshold", 4, 0, 30, 20, crap.ActionNone, nil},
		{"same measurement fails a tight threshold", 4, 0, 12, 20, crap.ActionRaiseCoverage, f(0.207)},
		{"partial coverage still needs more against the default threshold", 9, 0.1, 30, 68.05, crap.ActionRaiseCoverage, f(0.363)},
		{"high complexity exceeds the threshold outright", 34, 0.55, 30, 139.34, crap.ActionSplitMethod, nil},
		{"well covered method clears the default threshold", 3, 0.667, 30, 3.33, crap.ActionNone, nil},
		{"fully covered trivial method clears the default threshold", 1, 1, 30, 1, crap.ActionNone, nil},
		{"complexity above a tight threshold splits even at full coverage", 13, 1, 12, 13, crap.ActionSplitMethod, nil},
		{"same complexity clears the default threshold", 13, 1, 30, 13, crap.ActionNone, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := crap.Measurement{Complexity: c.complexity, Coverage: c.coverage, Threshold: c.threshold}

			if got := m.Score(); got != c.score {
				t.Errorf("Score() = %v, want %v", got, c.score)
			}
			if got := m.Action(); got != c.action {
				t.Errorf("Action() = %v, want %v", got, c.action)
			}
			gotTarget := m.TargetCoverage()
			switch {
			case c.targetCoverage == nil && gotTarget != nil:
				t.Errorf("TargetCoverage() = %v, want nil", *gotTarget)
			case c.targetCoverage != nil && gotTarget == nil:
				t.Errorf("TargetCoverage() = nil, want %v", *c.targetCoverage)
			case c.targetCoverage != nil && gotTarget != nil && *gotTarget != *c.targetCoverage:
				t.Errorf("TargetCoverage() = %v, want %v", *gotTarget, *c.targetCoverage)
			}
		})
	}
}

// TestScoreIgnoresThreshold pins seam 2's other half: two Measurements
// differing only in Threshold score identically, since Score answers a
// question about the method alone.
func TestScoreIgnoresThreshold(t *testing.T) {
	loose := crap.Measurement{Complexity: 4, Coverage: 0, Threshold: 30}
	tight := crap.Measurement{Complexity: 4, Coverage: 0, Threshold: 12}

	if loose.Score() != tight.Score() {
		t.Errorf("Score() moved with Threshold: %v vs %v", loose.Score(), tight.Score())
	}
}
