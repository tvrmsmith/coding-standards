// Package lintchanged_test is lint-changed's black-box suite. Every case
// builds the real binary once, drives it against a throwaway git repo built
// in t.TempDir(), feeds it a hand-written SARIF document on stdin, and
// asserts stdout and the exit code. Nothing here reaches inside the binary.
package lintchanged_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binDir holds the lint-changed binary for the whole run.
var binDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "lint-changed-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binDir = dir
	//nolint:gosec // the literal "go" building this module's own ./cmd/lint-changed; the only non-constant argv element is the MkdirTemp path in dir above
	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "lint-changed"), "./cmd/lint-changed")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building lint-changed: %v\n%s", err, out)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// gitEnv pins the identity and dates git commits with, and cuts the
// machine's own config out, so a fixture repo behaves the same everywhere.
var gitEnv = []string{
	"LC_ALL=C",
	"LANGUAGE=",
	"GIT_AUTHOR_NAME=Fixture Author",
	"GIT_AUTHOR_EMAIL=author@fixture.invalid",
	"GIT_COMMITTER_NAME=Fixture Committer",
	"GIT_COMMITTER_EMAIL=committer@fixture.invalid",
	"GIT_AUTHOR_DATE=2026-01-01T00:00:00+00:00",
	"GIT_COMMITTER_DATE=2026-01-01T00:00:00+00:00",
	"GIT_CONFIG_GLOBAL=/dev/null",
	"GIT_CONFIG_SYSTEM=/dev/null",
}

// scrubbedEnv is the process environment with git's own namespace taken out.
// gitEnv pins the few GIT_ variables a fixture wants and nothing pins the rest,
// so without this a run under `git rebase --exec` or any hook inherits
// GIT_DIR, GIT_INDEX_FILE and GIT_WORK_TREE and every fixture reads the wrong
// repository. That is a failure the developer's own shell decides, which is the
// one thing a fixture must never depend on.
func scrubbedEnv() []string {
	env := os.Environ()
	kept := make([]string, 0, len(env))
	for _, entry := range env {
		if name, _, _ := strings.Cut(entry, "="); strings.HasPrefix(name, "GIT_") {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// fixture is a throwaway git repo one case runs lint-changed against.
type fixture struct {
	t          *testing.T
	root       string
	waiverFile string
}

// newFixture creates an empty git repo on branch "main", with its own
// waiver log path so no case's waivers reach another's.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, root: t.TempDir(), waiverFile: filepath.Join(t.TempDir(), "waivers.jsonl")}
	f.git("-c", "init.defaultBranch=main", "init", "--quiet")
	return f
}

// git runs one git command in the fixture and returns its trimmed stdout.
func (f *fixture) git(args ...string) string {
	f.t.Helper()
	//nolint:gosec // the literal "git" with argv the calling case wrote, run in the fixture repo under t.TempDir and with the machine's own git config scrubbed out by gitEnv
	cmd := exec.Command("git", args...)
	cmd.Dir = f.root
	cmd.Env = append(scrubbedEnv(), gitEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitTry runs one git command that is expected to fail sometimes, returning
// its exit code and combined output. It is how a case drives a commit a
// pre-commit hook may refuse.
func (f *fixture) gitTry(args ...string) (int, string) {
	f.t.Helper()
	//nolint:gosec // same literal "git" and same scrubbed fixture repo as git above; this variant exists only to return the exit code instead of failing the test, which is how a case drives a commit the pre-commit hook refuses
	cmd := exec.Command("git", args...)
	cmd.Dir = f.root
	cmd.Env = append(scrubbedEnv(), gitEnv...)
	out, err := cmd.CombinedOutput()
	exitCode := 0
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		exitCode = exit.ExitCode()
	default:
		f.t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return exitCode, string(out)
}

// installPreCommitHook writes a real pre-commit hook that runs lint-changed
// over report, the way harness/hooks/pre-commit runs it after a build.
func (f *fixture) installPreCommitHook(report string) {
	f.t.Helper()
	hooks := filepath.Join(f.root, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o750); err != nil {
		f.t.Fatal(err)
	}
	script := fmt.Sprintf("#!/bin/sh\nexport TVRMSMITH_WAIVERS=%q\nexec %q --format sarif --language csharp --staged --report %q\n",
		f.waiverFile, filepath.Join(binDir, "lint-changed"), report)
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte(script), 0o700); err != nil { //nolint:gosec // G306: a hook git will not run is no hook.
		f.t.Fatal(err)
	}
}

// head is the sha the branch points at, so a case can tell a refused commit
// from one that went through.
func (f *fixture) head() string {
	f.t.Helper()
	return f.git("rev-parse", "HEAD")
}

// write puts content at the repo-relative path rel, creating parents.
func (f *fixture) write(rel, content string) {
	f.t.Helper()
	full := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// commitAll stages everything in the tree and commits it.
func (f *fixture) commitAll(message string) {
	f.t.Helper()
	f.git("add", "-A")
	f.git("commit", "--quiet", "-m", message)
}

// stage stages a single path without committing it, which is how a case
// builds the state a --staged run reads.
func (f *fixture) stage(rel string) {
	f.t.Helper()
	f.git("add", rel)
}

// absPath is the resolved absolute path of a file already written into the
// fixture, the form golangci and eslint name their own paths in. t.TempDir()
// hands back a path under /var that is on macOS a symlink to /private/var,
// and srcpath.Root resolves symlinks when it places a candidate path, so a
// report naming the raw filepath.Join(f.root, rel) fails to place inside the
// root: it names a real file, but not at the resolved path the root compares
// against. Resolving here, the same way srcpath.Root.Place does internally,
// is what makes the report describe a path the parser can place.
func (f *fixture) absPath(rel string) string {
	f.t.Helper()
	resolved, err := filepath.EvalSymlinks(filepath.Join(f.root, filepath.FromSlash(rel)))
	if err != nil {
		f.t.Fatal(err)
	}
	return resolved
}

// writeTree is the index tree sha a case expects a waiver to be matched and
// spent against, read back out of the fixture rather than out of the
// binary's own output.
func (f *fixture) writeTree() string {
	f.t.Helper()
	return f.git("write-tree")
}

// runResult is one lint-changed invocation.
type runResult struct {
	exitCode int
	stdout   string
	stderr   string
}

// run executes lint-changed in the fixture repo with sarif on stdin.
func (f *fixture) run(sarif string, args ...string) runResult {
	f.t.Helper()
	return f.runInDir(f.root, sarif, args...)
}

// runInDir is run started in dir rather than the fixture root, which is how
// a case drives lint-changed outside any git repository at all.
func (f *fixture) runInDir(dir, sarif string, args ...string) runResult {
	f.t.Helper()
	//nolint:gosec // binDir holds the lint-changed TestMain built from this module's own source, so the executable is this package's build output rather than anything a case can name
	cmd := exec.Command(filepath.Join(binDir, "lint-changed"), args...)
	cmd.Dir = dir
	cmd.Env = append(append(scrubbedEnv(), gitEnv...), "TVRMSMITH_WAIVERS="+f.waiverFile)
	cmd.Stdin = strings.NewReader(sarif)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		exitCode = exit.ExitCode()
	default:
		f.t.Fatalf("running lint-changed: %v", err)
	}
	return runResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

// waiverLogLines is the raw lines of the fixture's waiver log, used to pin
// how many records a run appended.
func (f *fixture) waiverLogLines() []string {
	f.t.Helper()
	body, err := os.ReadFile(f.waiverFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		f.t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(string(body), "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// sarifDoc wraps one or more sarifResult strings into a minimal SARIF 2.1
// log, the shape lintfind.ParseSARIF reads.
func sarifDoc(results ...string) string {
	return `{"version":"2.1.0","runs":[{"results":[` + strings.Join(results, ",") + `]}]}`
}

// sarifLoc is one SARIF physicalLocation, a span inclusive of both ends.
func sarifLoc(path string, start, end int) string {
	return fmt.Sprintf(`{"physicalLocation":{"artifactLocation":{"uri":%q},"region":{"startLine":%d,"endLine":%d}}}`, path, start, end)
}

// sarifLocCol is sarifLoc's sibling carrying an explicit startColumn, for the
// cases that pin the column threading through the filter.
func sarifLocCol(path string, start, end, col int) string {
	return fmt.Sprintf(`{"physicalLocation":{"artifactLocation":{"uri":%q},"region":{"startLine":%d,"startColumn":%d,"endLine":%d}}}`, path, start, col, end)
}

// sarifLocNoRegion is a SARIF physicalLocation naming a file and no region,
// which is what Roslyn writes for a diagnostic about a whole document.
func sarifLocNoRegion(path string) string {
	return fmt.Sprintf(`{"physicalLocation":{"artifactLocation":{"uri":%q}}}`, path)
}

// sarifResult is one SARIF result with its primary locations.
func sarifResult(rule, message string, locs ...string) string {
	return fmt.Sprintf(`{"ruleId":%q,"level":"warning","message":{"text":%q},"locations":[%s]}`,
		rule, message, strings.Join(locs, ","))
}

// sarifResultNoLocation is a SARIF result with no locations array, which is
// what Roslyn writes for a diagnostic reported at Location.None.
func sarifResultNoLocation(rule, message string) string {
	return fmt.Sprintf(`{"ruleId":%q,"level":"warning","message":{"text":%q}}`, rule, message)
}

// sarifResultRelated is a SARIF result whose primary location is separate
// from its related location, for pinning behaviour 5.
func sarifResultRelated(rule, message string, primary, related string) string {
	return fmt.Sprintf(`{"ruleId":%q,"level":"warning","message":{"text":%q},"locations":[%s],"relatedLocations":[%s]}`,
		rule, message, primary, related)
}

// golangciDoc is a minimal golangci-lint JSON report with one issue, the
// shape lintfind.ParseGolangCI reads. path is the absolute path the case
// resolved for its own fixture file: golangci reports Pos.Filename absolute,
// since harness/linters/go.sh runs it with --path-mode abs.
func golangciDoc(path, fromLinter, text string, line, col int) string {
	return fmt.Sprintf(`{"Issues":[{"FromLinter":%q,"Text":%q,"Severity":"","Pos":{"Filename":%q,"Line":%d,"Column":%d}}]}`,
		fromLinter, text, path, line, col)
}

// eslintDoc is a minimal ESLint JSON report with one file result and one
// message, the shape lintfind.ParseESLint reads. path is the absolute path
// the case resolved for its own fixture file: ESLint resolves
// --stdin-filename against its own cwd before reporting it, so the
// staged-content run harness/linters/ts.sh makes lands on a real repo path
// too.
func eslintDoc(path, ruleID, message string, severity, line, col int) string {
	return fmt.Sprintf(`[{"filePath":%q,"messages":[{"ruleId":%q,"message":%q,"severity":%d,"line":%d,"column":%d}]}]`,
		path, ruleID, message, severity, line, col)
}
