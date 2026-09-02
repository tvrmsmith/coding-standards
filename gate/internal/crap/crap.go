// Package crap is the CRAP metric: the inputs it declares (ADR 0002), the
// formula, and the two typed fix cells ADR 0005 requires beside it.
package crap

import "math"

// Name is the key the metric's table appears under in the document.
const Name = "crap"

// DisplayName is how the metric is named in prose, which is what the stderr
// line and the missing-input failure both say.
const DisplayName = "CRAP"

// Threshold is the CRAP score a changed method may not exceed. It is a
// constant; making it configurable is another issue's work.
const Threshold = 30

// DeclaredInputs names the inputs a run selecting CRAP demands. The gate
// looks for a coverage report only because this names it, and the failure
// message names the metric rather than the file.
var DeclaredInputs = []string{"coverage"}

// The two fix instructions and their absence, as ADR 0005 types them.
const (
	ActionSplitMethod   = "split_method"
	ActionRaiseCoverage = "raise_coverage"
	ActionNone          = "none"
)

// Measurement is one method's complexity and coverage fraction, which is
// everything CRAP needs to score it and to say which fix applies.
type Measurement struct {
	Complexity int
	// Coverage is the raw fraction, 0 to 1, not the rounded cell value.
	Coverage float64
}

// Score is comp² × (1 − cov)³ + comp, rounded half up at two decimals. The
// rounded value is what the document shows and what the verdict compares, so
// a run can never fail on a digit the reader cannot see.
func (m Measurement) Score() float64 {
	comp := float64(m.Complexity)
	uncovered := 1 - m.Coverage
	return roundHalfUp(comp*comp*uncovered*uncovered*uncovered+comp, 2)
}

// Action is the one fix instruction that applies. split_method is emitted
// exactly when complexity exceeds the threshold, because at full coverage
// CRAP reduces to comp and no test can rescue the method.
func (m Measurement) Action() string {
	switch {
	case m.Complexity > Threshold:
		return ActionSplitMethod
	case m.Score() > Threshold:
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
	target := ceilAt(1-math.Cbrt((Threshold-comp)/(comp*comp)), 3)
	return &target
}

// roundHalfUp rounds f to precision decimal places, half away from zero.
func roundHalfUp(f float64, precision int) float64 {
	scale := math.Pow(10, float64(precision))
	return math.Floor(f*scale+0.5) / scale
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
