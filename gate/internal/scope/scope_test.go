package scope

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/metric"
)

// fakeAlpha and fakeBeta are a two-entry catalogue standing in for the
// binary's real one, which hosts CRAP alone. With only one metric hosted,
// several rules parse enforces are unreachable through Parse: a threshold
// naming a metric --metric did not select has no second metric to name, and
// "defaults to every metric the binary hosts" cannot be told apart from
// "defaults to crap". Driving parse directly with this catalogue is what
// makes those rules real tests rather than dead branches.
var (
	fakeAlpha = metric.Definition{Name: "alpha", Display: "ALPHA", DefaultThreshold: 10, Inputs: []metric.Input{metric.InputCoverage}}
	fakeBeta  = metric.Definition{Name: "beta", Display: "BETA", DefaultThreshold: 5}
)

var fakeCatalogue = []metric.Definition{fakeAlpha, fakeBeta}

// TestParseSelectsMetricsAtTheirThreshold pins Scope.Metrics against the
// fake catalogue: which metrics a run selects, the threshold in force for
// each, and the order they come out in. The third and sixth cases are what
// pin metric.Hosted order against typed order, and the fifth pins that a
// threshold with no --metric still lands on the default selection.
func TestParseSelectsMetricsAtTheirThreshold(t *testing.T) {
	cases := map[string]struct {
		argv []string
		want []metric.Selection
	}{
		"no --metric selects every hosted metric at its default": {
			argv: nil,
			want: []metric.Selection{
				{Definition: fakeAlpha, Threshold: 10},
				{Definition: fakeBeta, Threshold: 5},
			},
		},
		"--metric selects only the named metric": {
			argv: []string{"--metric", "beta"},
			want: []metric.Selection{
				{Definition: fakeBeta, Threshold: 5},
			},
		},
		"two --metric flags come out in hosted order, not typed order": {
			argv: []string{"--metric", "beta", "--metric", "alpha"},
			want: []metric.Selection{
				{Definition: fakeAlpha, Threshold: 10},
				{Definition: fakeBeta, Threshold: 5},
			},
		},
		"--threshold overrides the default for a selected metric": {
			argv: []string{"--metric", "beta", "--threshold", "beta=3"},
			want: []metric.Selection{
				{Definition: fakeBeta, Threshold: 3},
			},
		},
		// The joined form is the one place two equals signs meet, so it pins
		// that the split takes the first and leaves the assignment intact.
		"--threshold=<name>=<n> splits at the first equals": {
			argv: []string{"--metric=beta", "--threshold=beta=3"},
			want: []metric.Selection{
				{Definition: fakeBeta, Threshold: 3},
			},
		},
		"--threshold with no --metric still lands on the default selection": {
			argv: []string{"--threshold", "alpha=7"},
			want: []metric.Selection{
				{Definition: fakeAlpha, Threshold: 7},
				{Definition: fakeBeta, Threshold: 5},
			},
		},
		"--threshold typed before --metric still resolves in hosted order": {
			argv: []string{"--threshold", "beta=3", "--metric", "beta", "--metric", "alpha"},
			want: []metric.Selection{
				{Definition: fakeAlpha, Threshold: 10},
				{Definition: fakeBeta, Threshold: 3},
			},
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			sc, err := parse(tt.argv, fakeCatalogue)
			if err != nil {
				t.Fatalf("parse(%v) = _, %v, want no error", tt.argv, err)
			}
			if !reflect.DeepEqual(sc.Metrics, tt.want) {
				t.Errorf("parse(%v).Metrics = %#v, want %#v", tt.argv, sc.Metrics, tt.want)
			}
		})
	}
}

// TestParseMetricUsageErrorsReachableOnlyWithTwoMetricsHosted covers the two
// rules the binary's one hosted metric cannot exercise through Parse: a
// threshold naming a metric the run did not select, and the hosted-names
// list in an unknown-metric message actually listing more than one name.
func TestParseMetricUsageErrorsReachableOnlyWithTwoMetricsHosted(t *testing.T) {
	cases := map[string]struct {
		argv    []string
		problem string
	}{
		"a threshold naming a hosted metric --metric did not select": {
			argv:    []string{"--metric", "beta", "--threshold", "alpha=7"},
			problem: "--threshold names alpha, which --metric did not select",
		},
		"an unknown metric lists every hosted name": {
			argv:    []string{"--metric", "gamma"},
			problem: "unknown metric 'gamma'; this binary hosts: alpha, beta",
		},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parse(tt.argv, fakeCatalogue)
			if err == nil {
				t.Fatalf("parse(%v) = _, nil, want a usage error", tt.argv)
			}
			var usageErr *UsageError
			if !errors.As(err, &usageErr) {
				t.Fatalf("parse(%v) returned %T, want *UsageError", tt.argv, err)
			}
			if usageErr.Problem != tt.problem {
				t.Errorf("parse(%v) problem = %q, want %q", tt.argv, usageErr.Problem, tt.problem)
			}
		})
	}
}

// TestParseDefaultsTheRealCatalogueToItsOneMetric asserts through the
// exported Parse, against the binary's real catalogue, that a run naming its
// one hosted metric selects it at its default threshold, and that bare argv
// yields the same selection: the positive case the design says belongs at
// the unit level, since asserting it black-box would need a whole fixture
// repo for a fact that does not depend on one.
func TestParseDefaultsTheRealCatalogueToItsOneMetric(t *testing.T) {
	want := []metric.Selection{{Definition: metric.Definition{Name: "crap", Display: "CRAP", DefaultThreshold: 30, Inputs: []metric.Input{metric.InputCoverage}}, Threshold: 30}}

	cases := map[string][]string{
		"bare argv":     nil,
		"--metric crap": {"--metric", "crap"},
		"--metric=crap": {"--metric=crap"},
	}
	for name, argv := range cases {
		t.Run(name, func(t *testing.T) {
			sc, err := Parse(argv)
			if err != nil {
				t.Fatalf("Parse(%v) = _, %v, want no error", argv, err)
			}
			if !reflect.DeepEqual(sc.Metrics, want) {
				t.Errorf("Parse(%v).Metrics = %#v, want %#v", argv, sc.Metrics, want)
			}
		})
	}
}
