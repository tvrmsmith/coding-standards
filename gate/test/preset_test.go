package gate_test

import (
	"os"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// presetConfig is the path, relative to this package, to the preset the CI
// lint step names with --config. TestCIDeclaresTheBlockingLintStep pins that
// the step reads this file; this one pins that reading it still reports the
// rules the step exists to enforce.
const presetConfig = "../../go/golangci.yml"

// gosecLinter is the entry in linters.enable that arms the security rules, and
// justifiedRules are the gosec ids this repository answered at the site, either
// by tightening the code or by a //nolint carrying a reason true of that call,
// rather than by an exclude. Each answer goes dead the moment its id lands in
// gosec.excludes, and nolintlint will not say so, because allow-unused: true is
// set for the unrelated reason that the target repository's own live directives
// would all read as dead under this preset.
const gosecLinter = "gosec"

var justifiedRules = []string{"G204", "G301", "G302", "G304", "G306", "G702", "G703"}

// TestThePresetStillReportsTheRulesThisRepoJustified reads go/golangci.yml as
// the machine-consumed config it is and asserts the four edits that would leave
// the CI lint step running, green, and reporting nothing it is there to catch:
// dropping gosec from the enable list, excluding a rule some site in this
// repository claims to answer, sending gosec through linters.exclusions.rules,
// which decides how much code the linter sees rather than which rules it holds,
// and setting run.issues-exit-code, which golangci-lint honours exactly like
// the command-line flag the step declines to pass.
func TestThePresetStillReportsTheRulesThisRepoJustified(t *testing.T) {
	body, err := os.ReadFile(presetConfig)
	if err != nil {
		t.Fatalf("reading the preset the CI lint step runs with, at %s: %v", presetConfig, err)
	}

	// IssuesExitCode is a raw node because its absence is what this case
	// asserts, and 0 is both the value that disarms the run and the zero value
	// of an int, which would make the two indistinguishable.
	var preset struct {
		Run struct {
			IssuesExitCode yaml.Node `yaml:"issues-exit-code"`
		} `yaml:"run"`
		Linters struct {
			Enable   []string `yaml:"enable"`
			Settings struct {
				Gosec struct {
					Excludes []string `yaml:"excludes"`
				} `yaml:"gosec"`
			} `yaml:"settings"`
			Exclusions struct {
				Rules []struct {
					Path    string   `yaml:"path"`
					Linters []string `yaml:"linters"`
				} `yaml:"rules"`
			} `yaml:"exclusions"`
		} `yaml:"linters"`
	}
	if err := yaml.Unmarshal(body, &preset); err != nil {
		t.Fatalf("parsing the preset the CI lint step runs with, at %s: %v", presetConfig, err)
	}

	if !slices.Contains(preset.Linters.Enable, gosecLinter) {
		t.Errorf("%s: linters.enable is %v, want it to hold %q. With the entry gone the CI step still runs and still exits 0, while every //nolint:gosec in this repository suppresses a rule nothing reports",
			presetConfig, preset.Linters.Enable, gosecLinter)
	}

	for _, rule := range justifiedRules {
		if slices.Contains(preset.Linters.Settings.Gosec.Excludes, rule) {
			t.Errorf("%s: linters.settings.gosec.excludes holds %s, which this repository answered by tightening the code or writing a //nolint that says why the site is safe. An exclude also disarms the rule in every repository this preset visits",
				presetConfig, rule)
		}
	}

	// An exclusion rule listing no linters applies to all of them, so the empty
	// list silences gosec as surely as naming it does.
	for i, rule := range preset.Linters.Exclusions.Rules {
		if len(rule.Linters) != 0 && !slices.Contains(rule.Linters, gosecLinter) {
			continue
		}
		t.Errorf("%s: linters.exclusions.rules[%d] takes %q out over path %q. gosec.excludes decides which rules are armed and this decides which code they run on, so a rule here leaves the CI step green while the sites it covers answer nothing",
			presetConfig, i, gosecLinter, rule.Path)
	}

	if code := preset.Run.IssuesExitCode.Value; code != "" {
		t.Errorf("%s: run.issues-exit-code is %s. The CI lint step blocks by leaving the exit code at its default, and this key overrides it from inside the config the step reads",
			presetConfig, code)
	}
}
