// Package crap is the CRAP metric: the inputs it declares (ADR 0002), the
// formula, and the two typed fix cells ADR 0008 requires beside it.
package crap

import (
	"math"

	"github.com/tvrmsmith/coding-standards/gate/internal/round"
)

// Name is the key the metric's table appears under in the document.
const Name = "crap"

// DisplayName is how the metric is named in prose, which is what the stderr
// line and the missing-input failure both say.
const DisplayName = "CRAP"

// DefaultThreshold is the CRAP score a changed method may not exceed absent a
// run naming its own bar (issue 19). The catalogue entry in package metric is
// what a run actually reads; this stays for the caller building that entry.
const DefaultThreshold = 30

// Action is the fix instruction the document's `action` cell carries. ADR
// 0008 requires that cell to be typed rather than prose and enumerates the
// three tokens below, so the gate spells them as a type, the same way
// report.State spells its own three, rather than as bare strings in a field
// any string at all can be assigned to.
//
// Go will still convert an untyped constant, so `Action("split-method")`
// and a literal in a composite literal both compile. What the type buys is
// the rest: a `string` variable no longer assigns into the field, the three
// constants are the discoverable vocabulary, and the asymmetry issue 94
// found inside report.Row is gone.
//
// It lives here rather than beside report.State, which it sits next to in
// report.Row, because report already depends on this package through scope
// and metric, so the named type could only travel in this direction.
type Action string

// The two fix instructions and their absence, the whole of what `action` can
// be.
const (
	ActionSplitMethod   Action = "split_method"
	ActionRaiseCoverage Action = "raise_coverage"
	ActionNone          Action = "none"
)

// Measurement is one method's complexity and coverage fraction judged against
// one threshold, which is everything CRAP needs to score it and to say which
// fix applies. Threshold is a field rather than a package constant (issue 19):
// the same complexity and coverage can pass one run's bar and fail another's,
// so the verdict has to travel with the bar it was judged against, not with
// the package.
type Measurement struct {
	Complexity int
	// Coverage is the raw fraction, 0 to 1, not the rounded cell value.
	Coverage  float64
	Threshold int
}

// Score is comp² × (1 − cov)³ + comp, rounded half up at two decimals. The
// rounded value is what the document shows and what the verdict compares, so
// a run can never fail on a digit the reader cannot see.
func (m Measurement) Score() float64 {
	comp := float64(m.Complexity)
	uncovered := 1 - m.Coverage
	return round.HalfUp(comp*comp*uncovered*uncovered*uncovered+comp, 2)
}

// Action is the one fix instruction that applies. split_method is emitted
// exactly when complexity exceeds the threshold, because at full coverage
// CRAP reduces to comp and no test can rescue the method.
func (m Measurement) Action() Action {
	switch {
	case m.Complexity > m.Threshold:
		return ActionSplitMethod
	case m.Score() > float64(m.Threshold):
		return ActionRaiseCoverage
	default:
		return ActionNone
	}
}

// TargetCoverage is the coverage that would bring the method under the
// threshold at its current complexity, 1 − cbrt((T − comp) / comp²), or nil
// when raising coverage is not the fix. It rounds **up** at three decimals:
// at the half-up value the score can still exceed the threshold, and a target
// the developer can hit and still fail defeats the reason the cell exists.
func (m Measurement) TargetCoverage() *float64 {
	if m.Action() != ActionRaiseCoverage {
		return nil
	}
	comp := float64(m.Complexity)
	threshold := float64(m.Threshold)
	target := ceilAt(1-math.Cbrt((threshold-comp)/(comp*comp)), 3)
	return &target
}

// ceilAt rounds f up to precision decimal places. It snaps the scaled value
// to the nearest integer first when it is within binary representation noise
// of one, so a value already exact at this precision does not gain a whole
// step.
func ceilAt(f float64, precision int) float64 {
	scale := math.Pow(10, float64(precision))
	scaled := f * scale
	if nearest := math.Round(scaled); math.Abs(scaled-nearest) < 1e-9 {
		scaled = nearest
	}
	return math.Ceil(scaled) / scale
}
