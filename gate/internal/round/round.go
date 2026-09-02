// Package round holds the gate's one rounding rule. The metric and the
// encoder both round, and a run whose verdict and whose printed cell
// disagreed by a digit would be indefensible, so there is a single
// implementation rather than one per package.
package round

import (
	"strconv"
	"strings"
)

// HalfUp rounds f to precision decimal places, rounding half away from zero
// rather than Go's default round-half-to-even. It works from strconv's
// shortest round-tripping decimal representation of f, then rounds that
// decimal string, so a binary value like 2.675 (stored as
// 2.67499999999999982236431605997495353221893310546875) rounds as a human
// reading "2.675" would expect rather than picking up the binary noise a
// scaled math.Floor(f*scale+0.5) form would.
func HalfUp(f float64, precision int) float64 {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")

	intPart, fracPart, _ := strings.Cut(s, ".")
	if len(fracPart) > precision {
		roundUp := fracPart[precision] >= '5'
		fracPart = fracPart[:precision]
		if roundUp {
			var carry bool
			fracPart, carry = incrementDigits(fracPart)
			if carry {
				var overflow bool
				if intPart, overflow = incrementDigits(intPart); overflow {
					intPart = "1" + intPart
				}
			}
		}
	}

	rounded := intPart
	if fracPart != "" {
		rounded += "." + fracPart
	}
	if neg {
		rounded = "-" + rounded
	}
	// The rounded decimal string always round-trips exactly at this
	// precision, so the reparse below cannot fail.
	result, _ := strconv.ParseFloat(rounded, 64)
	return result
}

// incrementDigits adds one to the least-significant digit of a decimal digit
// string, propagating any carry leftward. It reports whether the carry
// propagated past the most significant digit (e.g. "99" -> "00", true).
func incrementDigits(digits string) (string, bool) {
	b := []byte(digits)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] != '9' {
			b[i]++
			return string(b), false
		}
		b[i] = '0'
	}
	return string(b), true
}
