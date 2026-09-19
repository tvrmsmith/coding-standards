package gate_test

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// presetConfig is the path, relative to this package, to the preset the CI
// lint step names with --config. TestCIDeclaresTheBlockingLintStep pins that
// the step reads this file; this one pins that reading it still reports the
// rules the step exists to enforce.
const presetConfig = "../../go/golangci.yml"

// gosecLinter is the entry in linters.enable that arms the security rules, and
// justifiedRules are the gosec ids the sweep's findings carried. Each was
// answered at the site, by tightening the code or by a //nolint that names
// gosec and gives a reason true of that call, rather than by an exclude. The
// ids are not expected to appear as text in the directives, which say
// //nolint:gosec without spelling out which rule they silence. Every answer
// goes dead the moment its id lands in gosec.excludes, and nolintlint will not
// say so, because allow-unused: true is set for the unrelated reason that the
// target repository's own live directives would all read as dead under this
// preset.
const gosecLinter = "gosec"

var justifiedRules = []string{"G204", "G301", "G302", "G304", "G306", "G702", "G703"}

// exclusionRule is one entry of linters.exclusions.rules. All four selectors
// are decoded so a failure can name the one the rule actually sets.
type exclusionRule struct {
	Path       string   `yaml:"path"`
	PathExcept string   `yaml:"path-except"`
	Text       string   `yaml:"text"`
	Source     string   `yaml:"source"`
	Linters    []string `yaml:"linters"`
}

// selector renders the rule's match condition for a failure message.
func (r exclusionRule) selector() string {
	var set []string
	for _, s := range []struct{ key, value string }{
		{"path", r.Path},
		{"path-except", r.PathExcept},
		{"text", r.Text},
		{"source", r.Source},
	} {
		if s.value != "" {
			set = append(set, fmt.Sprintf("%s: %q", s.key, s.value))
		}
	}
	if len(set) == 0 {
		return "every finding, naming no selector at all"
	}
	return strings.Join(set, ", ")
}

// preset is the part of go/golangci.yml this package asserts on.
//
// IssuesExitCode and Tests are raw nodes because their absence is what the
// cases assert, and each key's disarming value, 0 and false, is also the zero
// value of its type, which would make the two indistinguishable.
type preset struct {
	Run struct {
		IssuesExitCode yaml.Node `yaml:"issues-exit-code"`
		Tests          yaml.Node `yaml:"tests"`
	} `yaml:"run"`
	Linters struct {
		Enable   []string `yaml:"enable"`
		Settings struct {
			Gosec struct {
				Excludes []string `yaml:"excludes"`
			} `yaml:"gosec"`
		} `yaml:"settings"`
		Exclusions struct {
			Rules       []exclusionRule `yaml:"rules"`
			Paths       []string        `yaml:"paths"`
			PathsExcept []string        `yaml:"paths-except"`
			Presets     []string        `yaml:"presets"`
		} `yaml:"exclusions"`
	} `yaml:"linters"`
}

// gosecReachDisarms returns one message per key that takes gosec off code it
// would otherwise see, and nothing when the preset still arms it over every
// file. gosec.excludes decides which rules are armed; every key here decides
// how much code they run over, so each one leaves the CI step green while the
// sites it covers answer nothing.
func gosecReachDisarms(p preset) []string {
	var disarms []string

	for i, rule := range p.Linters.Exclusions.Rules {
		if len(rule.Linters) == 0 {
			disarms = append(disarms, fmt.Sprintf(
				"linters.exclusions.rules[%d] lists no linters, which applies it to every one of them, and drops %s",
				i, rule.selector()))
			continue
		}
		if slices.Contains(rule.Linters, gosecLinter) {
			disarms = append(disarms, fmt.Sprintf(
				"linters.exclusions.rules[%d] names %q and drops %s",
				i, gosecLinter, rule.selector()))
		}
	}

	for i, path := range p.Linters.Exclusions.Paths {
		disarms = append(disarms, fmt.Sprintf(
			"linters.exclusions.paths[%d] is %q, and that key drops every finding from a matching file for every linter, so it takes gosec off those files exactly as a rule naming gosec would",
			i, path))
	}

	for i, path := range p.Linters.Exclusions.PathsExcept {
		disarms = append(disarms, fmt.Sprintf(
			"linters.exclusions.paths-except[%d] is %q, and that key reports findings only from matching files, so gosec goes quiet on every file outside it",
			i, path))
	}

	if len(p.Linters.Exclusions.Presets) != 0 {
		disarms = append(disarms, fmt.Sprintf(
			"linters.exclusions.presets is %v, and the shipped presets silence whole rule families in test files and generated code: common-false-positives and legacy alone carry EXC0007 (G204), EXC0009 (G301/G302/G306) and EXC0010 (G304). The list is empty in the preset for that reason and this is what holds it there",
			p.Linters.Exclusions.Presets))
	}

	if tests := p.Run.Tests.Value; tests != "" {
		disarms = append(disarms, fmt.Sprintf(
			"run.tests is %s, and that key decides whether _test.go is analysed at all, so setting it takes gosec off more code than any exclusion can",
			tests))
	}

	return disarms
}

// decodePreset parses the subset of golangci.yml the cases here assert on.
func decodePreset(body []byte) (preset, error) {
	var p preset
	err := yaml.Unmarshal(body, &p)
	return p, err
}

// TestThePresetStillReportsTheRulesThisRepoJustified reads go/golangci.yml as
// the machine-consumed config it is and asserts the edits that would leave the
// CI lint step running, green, and reporting nothing it is there to catch:
// dropping gosec from the enable list, excluding a rule some site in this
// repository claims to answer, any of the four keys that decide how much code
// gosec runs over rather than which rules it holds, and run.issues-exit-code,
// which golangci-lint honours exactly like the command-line flag the step
// declines to pass.
func TestThePresetStillReportsTheRulesThisRepoJustified(t *testing.T) {
	body, err := os.ReadFile(presetConfig)
	if err != nil {
		t.Fatalf("reading the preset the CI lint step runs with, at %s: %v", presetConfig, err)
	}

	p, err := decodePreset(body)
	if err != nil {
		t.Fatalf("parsing the preset the CI lint step runs with, at %s: %v", presetConfig, err)
	}

	if !slices.Contains(p.Linters.Enable, gosecLinter) {
		t.Errorf("%s: linters.enable is %v, want it to hold %q. With the entry gone the CI step still runs and still exits 0, while every //nolint:gosec in this repository suppresses a rule nothing reports",
			presetConfig, p.Linters.Enable, gosecLinter)
	}

	for _, rule := range justifiedRules {
		if slices.Contains(p.Linters.Settings.Gosec.Excludes, rule) {
			t.Errorf("%s: linters.settings.gosec.excludes holds %s, which this repository answered by tightening the code or writing a //nolint that says why the site is safe. An exclude also disarms the rule in every repository this preset visits",
				presetConfig, rule)
		}
	}

	for _, disarm := range gosecReachDisarms(p) {
		t.Errorf("%s: %s", presetConfig, disarm)
	}

	if code := p.Run.IssuesExitCode.Value; code != "" {
		t.Errorf("%s: run.issues-exit-code is %s. The CI lint step blocks by leaving the exit code at its default, and this key overrides it from inside the config the step reads",
			presetConfig, code)
	}
}

// TestEveryWayOfTakingGosecOffTestFilesIsReported feeds gosecReachDisarms the
// configs the committed preset does not carry, so the check the case above
// runs is exercised by inputs that are not in the tree. The first case is the
// block PR #127 shipped, which this branch removed.
func TestEveryWayOfTakingGosecOffTestFilesIsReported(t *testing.T) {
	cases := []struct {
		name   string
		config string
		want   string
	}{
		{
			name: "the exclusion rule PR #127 shipped",
			config: `
linters:
  exclusions:
    rules:
      - path: _test\.go
        linters:
          - gosec
`,
			want: "linters.exclusions.rules[0] names \"gosec\"",
		},
		{
			name: "a rule listing no linters, which applies to all of them",
			config: `
linters:
  exclusions:
    rules:
      - path: _test\.go
`,
			want: "linters.exclusions.rules[0] lists no linters",
		},
		{
			name: "a rule matching on text rather than path",
			config: `
linters:
  exclusions:
    rules:
      - text: "G304"
        linters:
          - gosec
`,
			want: `text: "G304"`,
		},
		{
			name: "an exclusions.paths entry",
			config: `
linters:
  exclusions:
    paths:
      - _test\.go
`,
			want: "linters.exclusions.paths[0]",
		},
		{
			name: "an exclusions.paths-except entry",
			config: `
linters:
  exclusions:
    paths-except:
      - cmd/
`,
			want: "linters.exclusions.paths-except[0]",
		},
		{
			name: "a non-empty presets list",
			config: `
linters:
  exclusions:
    presets:
      - common-false-positives
      - legacy
`,
			want: "linters.exclusions.presets",
		},
		{
			name: "run.tests turned off",
			config: `
run:
  tests: false
`,
			want: "run.tests is false",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := decodePreset([]byte(tc.config))
			if err != nil {
				t.Fatalf("parsing the case config: %v", err)
			}

			disarms := gosecReachDisarms(p)
			if len(disarms) != 1 {
				t.Fatalf("gosecReachDisarms reported %d disarms, want 1: %v", len(disarms), disarms)
			}
			if !strings.Contains(disarms[0], tc.want) {
				t.Errorf("gosecReachDisarms reported %q, want it to name %q", disarms[0], tc.want)
			}
		})
	}
}

// TestAnExclusionRuleLeavingGosecArmedIsNotReported holds the check to the
// keys that reach gosec, so it does not grow into a ban on exclusions.
func TestAnExclusionRuleLeavingGosecArmedIsNotReported(t *testing.T) {
	p, err := decodePreset([]byte(`
run:
  issues-exit-code: 1
linters:
  exclusions:
    rules:
      - path: _test\.go
        linters:
          - errcheck
    presets: []
`))
	if err != nil {
		t.Fatalf("parsing the case config: %v", err)
	}

	if disarms := gosecReachDisarms(p); len(disarms) != 0 {
		t.Errorf("gosecReachDisarms reported %v for a rule that names only errcheck, want nothing", disarms)
	}
}
