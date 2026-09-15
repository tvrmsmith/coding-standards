package metric_test

import (
	"reflect"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/metric"
)

// TestHosted pins the one metric the binary hosts today, spelled exactly as
// its document table, prose name, default bar, and ADR 0002 declaration.
func TestHosted(t *testing.T) {
	want := []metric.Definition{
		{Name: "crap", Display: "CRAP", DefaultThreshold: 30, Inputs: []metric.Input{metric.InputCoverage}},
	}

	got := metric.Hosted()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Hosted() = %+v, want %+v", got, want)
	}
}

// TestNames pins the usage-message spelling: the document key, not the
// prose name. The second case is what holds Names to the catalogue it was
// handed rather than to the binary's own.
func TestNames(t *testing.T) {
	cases := map[string]struct {
		hosted []metric.Definition
		want   []string
	}{
		"the real catalogue": {metric.Hosted(), []string{"crap"}},
		"an injected catalogue answers for itself": {
			[]metric.Definition{{Name: "alpha"}, {Name: "beta"}},
			[]string{"alpha", "beta"},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := metric.Names(c.hosted)

			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Names() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestLookup covers the case-sensitive contract: the key is the document
// key the metric's table prints, so a run naming a different case names a
// table key the document does not have.
func TestLookup(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		found bool
	}{
		{"the hosted document key is found", "crap", true},
		{"the prose name is not the key", "CRAP", false},
		{"no case-insensitive fallback", "Crap", false},
		{"the empty string names nothing", "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			def, ok := metric.Lookup(metric.Hosted(), c.key)

			if ok != c.found {
				t.Fatalf("Lookup(%q) ok = %v, want %v", c.key, ok, c.found)
			}
			if c.found && def.Name != c.key {
				t.Errorf("Lookup(%q) = %+v, want Name %q", c.key, def, c.key)
			}
		})
	}
}

// TestDeclaring covers ADR 0002's declaration rule at the seam that can
// observe it without any second real metric existing: fake Definition
// values standing in for one that declares an input, one that declares
// nothing, and a mixed list that must keep the declaring entries in their
// original order.
func TestDeclaring(t *testing.T) {
	declares := metric.Selection{Definition: metric.Definition{Name: "declares", Inputs: []metric.Input{metric.InputCoverage}}}
	declaresNothing := metric.Selection{Definition: metric.Definition{Name: "declares-nothing", Inputs: nil}}
	declaresToo := metric.Selection{Definition: metric.Definition{Name: "declares-too", Inputs: []metric.Input{metric.InputCoverage}}}

	cases := []struct {
		name     string
		selected []metric.Selection
		want     []metric.Selection
	}{
		{"a declaring selection comes back", []metric.Selection{declares}, []metric.Selection{declares}},
		{"a non-declaring selection comes back empty", []metric.Selection{declaresNothing}, []metric.Selection{}},
		{"a mixed list keeps only the declaring entries, in order", []metric.Selection{declaresNothing, declares, declaresToo}, []metric.Selection{declares, declaresToo}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := metric.Declaring(c.selected, metric.InputCoverage)

			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Declaring() = %+v, want %+v", got, c.want)
			}
		})
	}
}
