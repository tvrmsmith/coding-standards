// Package metric is the catalogue of metrics the binary hosts and ADR 0002's
// declared-input rule. Pure data: no git, no filesystem, and no import of
// scope or report, so scope can import this package without breaking its own
// rule of depending on nothing that touches the world (issue 19). It imports
// the metric packages themselves, which are pure arithmetic, so the catalogue
// reads each metric's own spelling of its name and bar rather than restating
// them.
package metric

import "github.com/tvrmsmith/coding-standards/gate/internal/crap"

// Input is one thing a metric needs the gate to go and get, spelled once in
// a metric's Definition so the gate never demands what nothing declared and
// never silently skips what something did (ADR 0002).
type Input string

// InputCoverage is the one input a hosted metric declares today: a coverage
// report, which only CRAP needs.
const InputCoverage Input = "coverage"

// Definition is one metric the binary hosts. Name is the document key its
// table appears under; it is deliberately not the prose Display name, so a
// run cannot select a table the document does not print by typing the name
// it reads in prose.
type Definition struct {
	Name             string
	Display          string
	DefaultThreshold int
	Inputs           []Input
}

// Hosted lists every metric the binary hosts, in the order their tables
// appear in the document. CRAP is the only entry until a second metric earns
// one; see the design at issue 19 for why none is added here yet.
//
// The entry reads CRAP's name, prose name and default bar off package crap
// rather than restating them, so a rename there cannot leave the catalogue
// advertising a key the document does not print. Only the ADR 0002
// declaration is spelled here, because the declaration is about what the gate
// must go and get, which is this package's subject and not the formula's.
func Hosted() []Definition {
	return []Definition{
		{
			Name:             crap.Name,
			Display:          crap.DisplayName,
			DefaultThreshold: crap.DefaultThreshold,
			Inputs:           []Input{InputCoverage},
		},
	}
}

// Names lists a catalogue's document keys, for a usage message. The
// catalogue is a parameter rather than Hosted() read here, so a caller
// already driving itself from an injected catalogue asks the same question of
// the catalogue it is using rather than of the binary's real one (issue 19).
func Names(hosted []Definition) []string {
	names := make([]string, len(hosted))
	for i, def := range hosted {
		names[i] = def.Name
	}
	return names
}

// Lookup finds a metric in hosted by document key. The match is exact, not
// case-insensitive: the key is the table key the document prints, and a
// case-insensitive match would let a run name a key the document does not
// have. The catalogue is a parameter for the reason Names gives.
func Lookup(hosted []Definition, name string) (Definition, bool) {
	for _, def := range hosted {
		if def.Name == name {
			return def, true
		}
	}
	return Definition{}, false
}

// Selection is one metric a run selected, with the threshold in force for
// it.
type Selection struct {
	Definition
	Threshold int
}

// Declaring lists the selections that declared input, in selection order.
// The gate goes looking for an input only when this is non-empty (ADR 0002),
// and the failure names these metrics rather than the file that is absent.
func Declaring(selected []Selection, input Input) []Selection {
	declaring := []Selection{}
	for _, sel := range selected {
		for _, declared := range sel.Inputs {
			if declared == input {
				declaring = append(declaring, sel)
				break
			}
		}
	}
	return declaring
}
