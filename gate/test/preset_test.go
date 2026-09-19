package gate_test

import (
	"fmt"
	"maps"
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

// pinnedExcludes are the two ids gosec.excludes carries and the reason the
// justifiedRules loop below is not a no-op. That loop only ever asserts
// absence, so a wrong struct tag, or an upstream move of the key, would decode
// nil and pass forever. Asserting these two are present proves the decode path
// reaches the real list.
var pinnedExcludes = []string{"G104", "G115"}

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
// The banned keys are raw nodes because their absence is what the cases
// assert, and each one's disarming value, 0 and false and the empty string, is
// also the zero value of its type, which would make the two indistinguishable.
type preset struct {
	Run struct {
		IssuesExitCode yaml.Node `yaml:"issues-exit-code"`
		Tests          yaml.Node `yaml:"tests"`
	} `yaml:"run"`
	Issues struct {
		New              yaml.Node `yaml:"new"`
		NewFromRev       yaml.Node `yaml:"new-from-rev"`
		NewFromMergeBase yaml.Node `yaml:"new-from-merge-base"`
		NewFromPatch     yaml.Node `yaml:"new-from-patch"`
	} `yaml:"issues"`
	Linters struct {
		Enable   []string `yaml:"enable"`
		Disable  []string `yaml:"disable"`
		Settings struct {
			Gosec struct {
				Includes   []string       `yaml:"includes"`
				Excludes   []string       `yaml:"excludes"`
				Severity   string         `yaml:"severity"`
				Confidence string         `yaml:"confidence"`
				Config     map[string]any `yaml:"config"`
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

// bannedKeys returns the keys no value belongs under, each with the node it
// decoded to. A node's Kind is zero only when the key was absent: a key with
// nothing after it decodes to a null scalar, which golangci-lint then reads as
// the type's zero value, so emptiness is not the question.
func (p preset) bannedKeys() map[string]yaml.Node {
	return map[string]yaml.Node{
		"run.issues-exit-code":       p.Run.IssuesExitCode,
		"run.tests":                  p.Run.Tests,
		"issues.new":                 p.Issues.New,
		"issues.new-from-rev":        p.Issues.NewFromRev,
		"issues.new-from-merge-base": p.Issues.NewFromMergeBase,
		"issues.new-from-patch":      p.Issues.NewFromPatch,
	}
}

// bannedKeyReason says why no value at all belongs under each banned key. Each
// one has a harmless-looking value, and pinning that value here would still
// leave a one-word edit between this config and a disarmed run, so the key is
// banned rather than pinned.
var bannedKeyReason = map[string]string{
	"run.issues-exit-code":       "the CI lint step blocks by leaving the exit code at its default, and 0 here overrides it from inside the config the step reads rather than on the command line",
	"run.tests":                  "the preset states golangci-lint's default, analysing _test.go, by omission, and false is the one value that drops every _test.go from analysis, which takes gosec off more code than any exclusion can",
	"issues.new":                 "it narrows the report to the diff from inside the config, and the CI step chooses whole-tree or changed-lines scope on its own command line",
	"issues.new-from-rev":        "it narrows the report to the diff from inside the config, and the CI step chooses whole-tree or changed-lines scope on its own command line",
	"issues.new-from-merge-base": "it narrows the report to the diff from inside the config, and the CI step chooses whole-tree or changed-lines scope on its own command line",
	"issues.new-from-patch":      "it narrows the report to the diff from inside the config, and the CI step chooses whole-tree or changed-lines scope on its own command line",
}

// presetDisarms returns one message per key that would leave the CI lint step
// running and green while reporting less than it does today, and nothing when
// the preset still arms every justified gosec id over every file.
//
// The keys it reads, enumerated rather than claimed complete:
//
//   - linters.enable and linters.disable, which decide whether gosec runs at
//     all. disable wins over enable, so the enable entry alone proves nothing.
//   - linters.settings.gosec.excludes, includes, severity, confidence and
//     config, which decide which gosec rules report. A non-empty includes list
//     runs only the ids it names, and severity or confidence above low drops
//     every Medium finding, which is all seven justified ids.
//   - linters.exclusions.rules, paths, paths-except and presets, which decide
//     how much code the armed rules run over. paths-except is here because it
//     is the same class of disarm as paths, reading as an allowlist rather than
//     a denylist.
//   - run.tests and the four issues.new* keys, which decide whether _test.go
//     and unchanged code are analysed at all.
//
// Known and not guarded here: the output caps issues.max-issues-per-linter and
// issues.max-same-issues, which silently drop findings over a threshold and are
// pinned instead by TestNeitherOutputCapTruncates in go/test; the CI step's own
// flags, pinned by TestCIDeclaresTheBlockingLintStep; and //nolint directives
// in the code being linted, which are the point rather than a bypass.
//
// None of this proves gosec fires in a _test.go, which is a property of the
// linter rather than of the YAML. TestGosecReportsInsideATestFile in go/test
// runs the binary over a fixture for that.
func presetDisarms(p preset) []string {
	var disarms []string

	if !slices.Contains(p.Linters.Enable, gosecLinter) {
		disarms = append(disarms, fmt.Sprintf(
			"linters.enable is %v and does not hold %q, so the CI step still runs and still exits 0 while every //nolint:gosec in this repository suppresses a rule nothing reports",
			p.Linters.Enable, gosecLinter))
	}

	if slices.Contains(p.Linters.Disable, gosecLinter) {
		disarms = append(disarms, fmt.Sprintf(
			"linters.disable names %q, and disable is applied after enable, so the entry in linters.enable reports nothing",
			gosecLinter))
	}

	gosec := p.Linters.Settings.Gosec

	for _, rule := range justifiedRules {
		if slices.Contains(gosec.Excludes, rule) {
			disarms = append(disarms, fmt.Sprintf(
				"linters.settings.gosec.excludes holds %s, which this repository answered by tightening the code or writing a //nolint that says why the site is safe. An exclude also disarms the rule in every repository this preset visits",
				rule))
		}
	}

	if len(gosec.Includes) != 0 {
		disarms = append(disarms, fmt.Sprintf(
			"linters.settings.gosec.includes is %v, and a non-empty includes list runs only the ids it names, so it disarms every rule this repository answered while gosec.excludes stays clean",
			gosec.Includes))
	}

	for _, score := range []struct{ key, value string }{
		{"severity", gosec.Severity},
		{"confidence", gosec.Confidence},
	} {
		if score.value != "" && !strings.EqualFold(score.value, "low") {
			disarms = append(disarms, fmt.Sprintf(
				"linters.settings.gosec.%s is %q, and gosec keeps only findings at or above it. G204, G301, G302, G304 and G306 are all Medium, so anything above low empties the sweep in one word",
				score.key, score.value))
		}
	}

	if len(gosec.Config) != 0 {
		disarms = append(disarms, fmt.Sprintf(
			"linters.settings.gosec.config sets %v, and those are the per-rule arguments, so a mode threshold here loosens a rule as surely as excluding it",
			slices.Sorted(maps.Keys(gosec.Config))))
	}

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
			"linters.exclusions.presets is %v, and the shipped presets silence whole rule families in test files and generated code. common-false-positives and legacy alone carry EXC0007 (G204), EXC0009 (G301/G302/G306) and EXC0010 (G304)",
			p.Linters.Exclusions.Presets))
	}

	nodes := p.bannedKeys()
	for _, key := range slices.Sorted(maps.Keys(nodes)) {
		node := nodes[key]
		if node.Kind == 0 {
			continue
		}
		disarms = append(disarms, fmt.Sprintf(
			"%s is written here, as %s. No value for the key belongs in this preset: %s",
			key, scalarOf(node), bannedKeyReason[key]))
	}

	return disarms
}

// scalarOf renders a node for a failure message, naming the null a bare key
// decodes to rather than printing nothing.
func scalarOf(node yaml.Node) string {
	if node.Value == "" {
		return "null"
	}
	return node.Value
}

// decodePreset parses the subset of golangci.yml the cases here assert on.
func decodePreset(body []byte) (preset, error) {
	var p preset
	err := yaml.Unmarshal(body, &p)
	return p, err
}

// TestThePresetStillReportsTheRulesThisRepoJustified reads go/golangci.yml as
// the machine-consumed config it is and asserts that none of the keys
// presetDisarms enumerates would leave the CI lint step running, green, and
// reporting less than it does today.
func TestThePresetStillReportsTheRulesThisRepoJustified(t *testing.T) {
	body, err := os.ReadFile(presetConfig)
	if err != nil {
		t.Fatalf("reading the preset the CI lint step runs with, at %s: %v", presetConfig, err)
	}

	p, err := decodePreset(body)
	if err != nil {
		t.Fatalf("parsing the preset the CI lint step runs with, at %s: %v", presetConfig, err)
	}

	for _, rule := range pinnedExcludes {
		if !slices.Contains(p.Linters.Settings.Gosec.Excludes, rule) {
			t.Errorf("%s: linters.settings.gosec.excludes decoded as %v, want it to hold %s. Either the two deliberate excludes are gone, or the key moved and this file is reading nothing, which would pass the absence checks below forever",
				presetConfig, p.Linters.Settings.Gosec.Excludes, rule)
		}
	}

	for _, disarm := range presetDisarms(p) {
		t.Errorf("%s: %s", presetConfig, disarm)
	}
}

// TestEveryDisarmingKeyIsReported feeds presetDisarms the configs the committed
// preset does not carry, so every branch is exercised by inputs that are not in
// the tree. The first case is the block PR #127 shipped, which this branch
// removed.
func TestEveryDisarmingKeyIsReported(t *testing.T) {
	// armed is the shape of the committed preset, which each case edits one
	// key at a time so the reported disarm is the one the case introduced.
	const armed = `
linters:
  enable:
    - gosec
  settings:
    gosec:
      excludes:
        - G104
        - G115
`

	cases := []struct {
		name   string
		config string
		want   string
	}{
		{
			name: "gosec dropped from the enable list",
			config: `
linters:
  enable:
    - errcheck
`,
			want: "linters.enable is [errcheck]",
		},
		{
			name: "gosec disabled after being enabled",
			config: armed + `
  disable:
    - gosec
`,
			want: "linters.disable names \"gosec\"",
		},
		{
			name: "an id this repository answered at the site added to excludes",
			config: `
linters:
  enable:
    - gosec
  settings:
    gosec:
      excludes:
        - G104
        - G304
`,
			want: "gosec.excludes holds G304",
		},
		{
			name: "an includes list, which runs only the ids it names",
			config: armed + `
      includes:
        - G101
`,
			want: "gosec.includes is [G101]",
		},
		{
			name: "severity raised above the Medium the justified ids carry",
			config: armed + `
      severity: high
`,
			want: `gosec.severity is "high"`,
		},
		{
			name: "confidence raised above the Medium the justified ids carry",
			config: armed + `
      confidence: high
`,
			want: `gosec.confidence is "high"`,
		},
		{
			name: "a per-rule config argument loosening a mode threshold",
			config: armed + `
      config:
        G306: "0777"
`,
			want: "gosec.config sets [G306]",
		},
		{
			name: "the exclusion rule PR #127 shipped",
			config: armed + `
  exclusions:
    rules:
      - path: _test\.go
        linters:
          - gosec
`,
			want: `linters.exclusions.rules[0] names "gosec"`,
		},
		{
			name: "a rule listing no linters, which applies to all of them",
			config: armed + `
  exclusions:
    rules:
      - path: _test\.go
`,
			want: "linters.exclusions.rules[0] lists no linters",
		},
		{
			name: "a rule matching on text rather than path",
			config: armed + `
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
			config: armed + `
  exclusions:
    paths:
      - _test\.go
`,
			want: "linters.exclusions.paths[0]",
		},
		{
			name: "an exclusions.paths-except entry",
			config: armed + `
  exclusions:
    paths-except:
      - cmd/
`,
			want: "linters.exclusions.paths-except[0]",
		},
		{
			name: "a non-empty presets list",
			config: armed + `
  exclusions:
    presets:
      - common-false-positives
      - legacy
`,
			want: "linters.exclusions.presets",
		},
		{
			name:   "run.tests turned off",
			config: armed + "\nrun:\n  tests: false\n",
			want:   "run.tests is written here, as false",
		},
		{
			name:   "run.tests written with no value, which decodes to false",
			config: armed + "\nrun:\n  tests:\n",
			want:   "run.tests is written here, as null",
		},
		{
			name:   "run.issues-exit-code written with no value, which decodes to 0",
			config: armed + "\nrun:\n  issues-exit-code:\n",
			want:   "run.issues-exit-code is written here, as null",
		},
		{
			name:   "issues.new-from-rev narrowing the run to the diff",
			config: armed + "\nissues:\n  new-from-rev: main\n",
			want:   "issues.new-from-rev is written here, as main",
		},
		{
			name:   "issues.new narrowing the run to the diff",
			config: armed + "\nissues:\n  new: true\n",
			want:   "issues.new is written here, as true",
		},
		{
			name:   "issues.new-from-merge-base narrowing the run to the diff",
			config: armed + "\nissues:\n  new-from-merge-base: origin/main\n",
			want:   "issues.new-from-merge-base is written here",
		},
		{
			name:   "issues.new-from-patch narrowing the run to the diff",
			config: armed + "\nissues:\n  new-from-patch: /tmp/changes.patch\n",
			want:   "issues.new-from-patch is written here",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := decodePreset([]byte(tc.config))
			if err != nil {
				t.Fatalf("parsing the case config: %v", err)
			}

			disarms := presetDisarms(p)
			if len(disarms) != 1 {
				t.Fatalf("presetDisarms reported %d disarms, want 1: %v", len(disarms), disarms)
			}
			if !strings.Contains(disarms[0], tc.want) {
				t.Errorf("presetDisarms reported %q, want it to name %q", disarms[0], tc.want)
			}
		})
	}
}

// TestAnExclusionRuleLeavingGosecArmedIsNotReported holds the check to the keys
// that reach gosec, so it does not grow into a ban on exclusions.
func TestAnExclusionRuleLeavingGosecArmedIsNotReported(t *testing.T) {
	p, err := decodePreset([]byte(`
linters:
  enable:
    - gosec
  settings:
    gosec:
      excludes:
        - G104
        - G115
      severity: low
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

	if disarms := presetDisarms(p); len(disarms) != 0 {
		t.Errorf("presetDisarms reported %v for a rule that names only errcheck, want nothing", disarms)
	}
}
