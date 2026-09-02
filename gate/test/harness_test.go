// Package gate_test is the gate's black-box suite. Every case builds the
// real metric-gate binary and a stub extractor into one directory, builds a
// fixture git repo in another, runs the binary there, and asserts the exit
// code and the whole of stdout against a golden file.
//
// Nothing here reaches inside the gate. The golden files are the expected
// values; a case that disagrees with one is a bug in the gate.
package gate_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// binDir holds the metric-gate binary and the stub extractor for the whole
// run. The gate locates its extractor in filepath.Dir(os.Executable()), so
// the two must sit side by side under the name the language map expects.
var binDir string

// extractorName is the binary name the gate's language map looks for.
const extractorName = "metric-gate-csharp"

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "metric-gate-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binDir = dir
	if err := buildBinaries(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// buildBinaries compiles the gate and the stub extractor into dir.
func buildBinaries(dir string) error {
	targets := map[string]string{
		"metric-gate": "./cmd/metric-gate",
		// The stub takes the real tool's name so os.Executable() lookup finds it.
		extractorName: "./test/stub",
	}
	for name, pkg := range targets {
		cmd := exec.Command("go", "build", "-o", filepath.Join(dir, name), pkg)
		cmd.Dir = ".."
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("building %s: %v\n%s", pkg, err, out)
		}
	}
	return nil
}

// gateOnlyDir builds a metric-gate binary into a directory of its own with no
// extractor beside it, which is the deployment a docs-only change has to pass
// in.
func gateOnlyDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "metric-gate"), "./cmd/metric-gate")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building metric-gate: %v\n%s", err, out)
	}
	return dir
}

// fixture is a throwaway git repo a case runs the gate against.
type fixture struct {
	t    *testing.T
	root string
	stub stubConfig
}

// stubConfig is the canned extractor behaviour for one case, in the shape
// the stub reads from METRIC_GATE_STUB.
type stubConfig struct {
	Language           string   `json:"language"`
	Extensions         []string `json:"extensions"`
	CapabilitiesStdout string   `json:"capabilitiesStdout"`
	ExitCode           int      `json:"exitCode"`
	Stdout             string   `json:"stdout"`
}

// gitEnv pins the identity and dates git commits with, and cuts the
// machine's own config out, so a fixture repo is the same everywhere.
var gitEnv = []string{
	"GIT_AUTHOR_NAME=Fixture Author",
	"GIT_AUTHOR_EMAIL=author@fixture.invalid",
	"GIT_COMMITTER_NAME=Fixture Committer",
	"GIT_COMMITTER_EMAIL=committer@fixture.invalid",
	"GIT_AUTHOR_DATE=2026-01-01T00:00:00+00:00",
	"GIT_COMMITTER_DATE=2026-01-01T00:00:00+00:00",
	"GIT_CONFIG_GLOBAL=/dev/null",
	"GIT_CONFIG_SYSTEM=/dev/null",
}

// newFixture creates an empty git repo on the named default branch.
func newFixture(t *testing.T, defaultBranch string) *fixture {
	t.Helper()
	f := &fixture{t: t, root: t.TempDir()}
	f.git("-c", "init.defaultBranch="+defaultBranch, "init", "--quiet")
	return f
}

// git runs one git command in the fixture and returns its trimmed stdout.
func (f *fixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.root
	cmd.Env = append(os.Environ(), gitEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// write puts content at the repo-relative path rel, creating parents.
func (f *fixture) write(rel, content string) {
	f.t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// commitAll stages everything in the tree and commits it.
func (f *fixture) commitAll(message string) {
	f.t.Helper()
	f.git("add", "-A")
	f.git("commit", "--quiet", "-m", message)
}

// baseLabel is the "<ref>@<7-char sha>" the run is expected to resolve, read
// back out of the fixture rather than out of the gate's own output.
func (f *fixture) baseLabel(ref string) string {
	f.t.Helper()
	sha := f.git("rev-parse", f.git("merge-base", "HEAD", ref))
	return ref + "@" + sha[:7]
}

// runResult is one gate invocation.
type runResult struct {
	exitCode int
	stdout   string
	stderr   string
}

// run executes the gate in the fixture repo with the case's stub config.
func (f *fixture) run() runResult {
	f.t.Helper()
	cfgPath := filepath.Join(f.t.TempDir(), "stub.json")
	body, err := json.Marshal(f.stub)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, body, 0o644); err != nil {
		f.t.Fatal(err)
	}

	cmd := exec.Command(filepath.Join(binDir, "metric-gate"))
	cmd.Dir = f.root
	cmd.Env = append(os.Environ(), append(gitEnv, "METRIC_GATE_STUB="+cfgPath)...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	result := runResult{stdout: stdout.String(), stderr: stderr.String()}
	switch e := err.(type) {
	case nil:
	case *exec.ExitError:
		result.exitCode = e.ExitCode()
	default:
		f.t.Fatalf("running metric-gate: %v", err)
	}
	return result
}

// assertMatches checks the run against a golden file and the expected exit
// code and stderr in one place, so a case carries one assertion.
func (r runResult) assertMatches(t *testing.T, golden string, exitCode int, base, stderr string) {
	t.Helper()
	want := readGolden(t, golden, base)
	if r.stdout == want && r.exitCode == exitCode && r.stderr == stderr {
		return
	}
	t.Errorf("gate run did not match golden %s.toon\n"+
		"exit code: got %d, want %d\n"+
		"stderr:    got %q, want %q\n"+
		"stdout diff:\n%s",
		golden, r.exitCode, exitCode, r.stderr, stderr, lineDiff(want, r.stdout))
}

// readGolden loads a golden document, substituting the resolved base.
func readGolden(t *testing.T, name, base string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("golden", name+".toon"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(body), "{{BASE}}", base)
}

// lineDiff renders the first differing lines of want against got.
func lineDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	var b strings.Builder
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		w, g := lineAt(wantLines, i), lineAt(gotLines, i)
		marker := "  "
		if w != g {
			marker = "->"
		}
		fmt.Fprintf(&b, "%s %s want %s\n%s %s got  %s\n", marker, pad(i), strconv.Quote(w), marker, pad(i), strconv.Quote(g))
	}
	return b.String()
}

func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "<absent>"
}

func pad(i int) string {
	return fmt.Sprintf("%3d", i+1)
}
