// Package lint holds the black-box proof that the curated config does what it says.
//
// The peer of packages/eslint-config-tvrmsmith/test/cases.js: a preset is a list of promises,
// and a promise nobody exercised is a guess. Enabling a linter is not the same as it running,
// and the ways it can be enabled and silent are all quiet ones.
package lint_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	configPath = "../golangci.yml"
	binaryPath = "../bin/tvrmsmith-gcl"
)

// TestEveryEnabledLinterReports runs the personal binary over the fixtures and fails on any
// enabled linter that reported nothing.
//
// The expectation is read out of golangci.yml rather than listed here, so enabling a linter
// without writing a fixture for it fails this test rather than passing quietly.
func TestEveryEnabledLinterReports(t *testing.T) {
	reported := lintFixtures(t)

	for _, linter := range enabledLinters(t) {
		assert.Contains(t, reported, linter,
			"%s is enabled in golangci.yml but reported nothing in test/fixtures", linter)
	}
}

// TestFixturesCoverOnlyEnabledLinters is the same check the other way round. A fixture for a
// linter that was since turned off is dead weight that reads like coverage.
func TestFixturesCoverOnlyEnabledLinters(t *testing.T) {
	enabled := enabledLinters(t)

	for _, linter := range lintFixtures(t) {
		assert.Contains(t, enabled, linter,
			"test/fixtures triggers %s, which golangci.yml does not enable", linter)
	}
}

// TestNeitherOutputCapTruncates pins `max-issues-per-linter: 0` and `max-same-issues: 0`.
//
// Both cap by default, at 50 and at 3, and both drop the excess silently. That is not a cosmetic
// limit for this layer: harness/lint-changed-go.sh lints whole packages and filters down to the
// changed files afterwards, so a cap applied to the package total leaves the filter an arbitrary
// subset, and which findings survive shifts with the order parallel analysis finished in. The
// fixture calls one error-returning function 55 times, which is over both caps at once.
func TestNeitherOutputCapTruncates(t *testing.T) {
	const want = 55

	same := 0
	for _, issue := range runFixtures(t) {
		if issue.FromLinter == "errcheck" && issue.Text == "Error return value is not checked" {
			same++
		}
	}

	assert.GreaterOrEqual(t, same, want,
		"golangci-lint reported %d of the %d identical errcheck findings in fixtures/caps.go; "+
			"an output cap is truncating the report", same, want)
}

// TestGosecReportsInsideATestFile is the behavioral half of the rule that `gosec` stays armed
// over `_test.go`.
//
// The config half, gate/test/preset_test.go, can only assert that no key takes gosec off test
// files, and a key it does not know about, an upstream rename, or a future default that stops
// analysing tests would all pass it. This runs the binary and looks for the finding.
// fixtures/assertions_test.go reads through a variable path for it.
func TestGosecReportsInsideATestFile(t *testing.T) {
	var inTests []string
	for _, issue := range runFixtures(t) {
		if issue.FromLinter == "gosec" && strings.HasSuffix(issue.Pos.Filename, "_test.go") {
			inTests = append(inTests, issue.Pos.Filename)
		}
	}

	assert.NotEmpty(t, inTests,
		"gosec reported nothing in any _test.go under test/fixtures, so the preset is not running it over test files")
}

// lintFixtures returns the sorted set of linters that reported something in test/fixtures.
func lintFixtures(t *testing.T) []string {
	t.Helper()

	return sorted(func(add func(string)) {
		for _, issue := range runFixtures(t) {
			add(issue.FromLinter)
		}
	})
}

// issue is the part of golangci-lint's JSON report these cases read.
type issue struct {
	FromLinter string `json:"FromLinter"`
	Text       string `json:"Text"`
	Pos        struct {
		Filename string `json:"Filename"`
	} `json:"Pos"`
}

// runFixtures runs the personal binary over test/fixtures and returns every issue it reported.
func runFixtures(t *testing.T) []issue {
	t.Helper()

	binary, err := filepath.Abs(binaryPath)
	require.NoError(t, err)
	_, err = os.Stat(binary)
	require.NoErrorf(t, err, "build the binary first: go/build.sh")

	// The report goes to a file rather than to stdout, because the text output cannot be turned
	// off and the two would interleave on the same stream.
	reportPath := filepath.Join(t.TempDir(), "report.json")
	command := exec.Command(binary, "run",
		"--config", configPath,
		"--output.json.path", reportPath,
		"--issues-exit-code", "0",
		"./fixtures/...")
	_, err = command.Output()
	require.NoError(t, err, "the lint run itself failed: %s", stderrOf(err))

	output, err := os.ReadFile(reportPath)
	require.NoError(t, err)

	var report struct {
		Issues []issue `json:"Issues"`
	}
	require.NoError(t, json.Unmarshal(output, &report), "output was not the JSON report: %s", output)
	require.NotEmpty(t, report.Issues, "the fixtures reported nothing at all")

	return report.Issues
}

// enabledLinters returns the sorted `linters.enable` list from the curated config.
func enabledLinters(t *testing.T) []string {
	t.Helper()

	source, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var config struct {
		Linters struct {
			Enable []string `yaml:"enable"`
		} `yaml:"linters"`
	}
	require.NoError(t, yaml.Unmarshal(source, &config))
	require.NotEmpty(t, config.Linters.Enable)

	return sorted(func(add func(string)) {
		for _, linter := range config.Linters.Enable {
			add(linter)
		}
	})
}

func sorted(collect func(add func(string))) []string {
	seen := map[string]bool{}
	collect(func(value string) { seen[value] = true })

	unique := make([]string, 0, len(seen))
	for value := range seen {
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

// stderrOf pulls the captured stderr out of a failed exec, which is where golangci-lint puts
// the reason a run broke.
func stderrOf(err error) string {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return string(exit.Stderr)
	}
	return ""
}
