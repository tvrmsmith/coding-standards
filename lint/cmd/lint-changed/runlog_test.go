package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

//nolint:gosec // G101: a fixture origin with a fake userinfo, there to prove the log strips it.
const logRunOrigin = "https://user:secret@example.test/o/r.git"

// logRunRepo makes a repo with one commit and the given origin (none when
// empty), chdirs into it, and pins git's environment. It returns the repo dir.
func logRunRepo(t *testing.T, origin string) string {
	t.Helper()
	dir := pinGitEnv(t)
	logRunGit(t, "init", "--quiet", "--initial-branch=main")
	logRunGit(t, "commit", "--quiet", "--allow-empty", "-m", "base")
	if origin != "" {
		logRunGit(t, "remote", "add", "origin", origin)
	}
	return dir
}

// pinGitEnv chdirs into a fresh dir and pins git's environment so neither the
// machine's own config nor a GIT_DIR inherited from a hook or `git rebase
// --exec` can reach it. It returns the dir.
func pinGitEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	for _, k := range []string{"GIT_DIR", "GIT_INDEX_FILE", "GIT_WORK_TREE"} {
		t.Setenv(k, "") // registers the restore; git rejects an empty GIT_DIR, so unset it too
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GIT_AUTHOR_NAME", "A")
	t.Setenv("GIT_AUTHOR_EMAIL", "a@x.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "A")
	t.Setenv("GIT_COMMITTER_EMAIL", "a@x.invalid")
	return dir
}

func logRunGit(t *testing.T, args ...string) string {
	t.Helper()
	//nolint:gosec // G204: the literal "git" with argv the calling case wrote, run in a fixture repo under t.TempDir.
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// logRunEnv gives the case its own state dir, with NM_GATE unset.
func logRunEnv(t *testing.T) string {
	t.Helper()
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("NM_GATE", "")
	return state
}

func writeKept(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "kept")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func logRunLines(t *testing.T, state string) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(state, "coding-standards", "lint-runs.jsonl")) //nolint:gosec // G304: path under this test's own TempDir
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
}

func callLogRun(args LogRunArgs) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(Command{Kind: KindLogRun, LogRun: args}, &out, &errb)
	return code, out.String(), errb.String()
}

// jsonKeys is the keys of one JSON object in document order.
func jsonKeys(t *testing.T, line string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(line))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

func TestLogRunAppendsOneLinePerLang(t *testing.T) {
	logRunRepo(t, logRunOrigin)
	state := logRunEnv(t)
	head := logRunGit(t, "rev-parse", "HEAD")
	kept := writeKept(t, "ts\tno-unused-vars\nts\tno-unused-vars\ncsharp\tCS0219\ngo\tstub\n")

	before := time.Now().Unix()
	code, stdout, stderr := callLogRun(LogRunArgs{Mode: "since", Status: 2, Kept: kept, Langs: []string{"ts", "dotnet"}})
	after := time.Now().Unix()

	if code != 0 || stdout != "" {
		t.Fatalf("exit %d stdout %q stderr %q, want exit 0 and empty stdout", code, stdout, stderr)
	}
	lines := logRunLines(t, state)
	if len(lines) != 2 {
		t.Fatalf("log has %d lines, want 2: %q", len(lines), lines)
	}
	wantKeys := []string{"ts", "repo", "branch", "head", "lang", "mode", "gate", "blocked", "findings"}
	for i, line := range lines {
		if got := jsonKeys(t, line); !reflect.DeepEqual(got, wantKeys) {
			t.Fatalf("line %d keys = %v, want %v", i+1, got, wantKeys)
		}
	}
	var first struct {
		TS       int64          `json:"ts"`
		Repo     string         `json:"repo"`
		Branch   string         `json:"branch"`
		Head     string         `json:"head"`
		Lang     string         `json:"lang"`
		Mode     string         `json:"mode"`
		Gate     bool           `json:"gate"`
		Blocked  bool           `json:"blocked"`
		Findings map[string]int `json:"findings"`
	}
	second := first
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}
	if first.TS < before || first.TS > after {
		t.Fatalf("ts = %d, want within [%d, %d]", first.TS, before, after)
	}
	if first.Repo != "https://example.test/o/r.git" || first.Branch != "main" || first.Head != head ||
		first.Lang != "ts" || first.Mode != "since" || first.Gate || !first.Blocked ||
		!reflect.DeepEqual(first.Findings, map[string]int{"no-unused-vars": 2}) {
		t.Fatalf("line 1 = %+v", first)
	}
	if second.Repo != first.Repo || second.Branch != "main" || second.Head != head ||
		second.Lang != "dotnet" || second.Mode != "since" || second.Gate || !second.Blocked ||
		!reflect.DeepEqual(second.Findings, map[string]int{"CS0219": 1}) {
		t.Fatalf("line 2 = %+v", second)
	}
}

func logRunOne(t *testing.T, state string, status int, kept, lang string) string {
	t.Helper()
	code, _, stderr := callLogRun(LogRunArgs{Mode: "staged", Status: status, Kept: writeKept(t, kept), Langs: []string{lang}})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	lines := logRunLines(t, state)
	return lines[len(lines)-1]
}

func TestLogRunEmptyKeptWritesEmptyFindingsObject(t *testing.T) {
	logRunRepo(t, logRunOrigin)
	state := logRunEnv(t)
	line := logRunOne(t, state, 0, "", "go")
	if !strings.Contains(line, `"findings":{}`) || !strings.Contains(line, `"blocked":false`) {
		t.Fatalf("line = %s", line)
	}
	if n := len(logRunLines(t, state)); n != 1 {
		t.Fatalf("log has %d lines, want 1", n)
	}
}

func TestLogRunStatusOneIsNotBlocked(t *testing.T) {
	logRunRepo(t, logRunOrigin)
	state := logRunEnv(t)
	line := logRunOne(t, state, 1, "go\tstub\n", "go")
	if !strings.Contains(line, `"blocked":false`) || !strings.Contains(line, `"findings":{"stub":1}`) {
		t.Fatalf("line = %s", line)
	}
}

func TestLogRunGateFollowsNMGate(t *testing.T) {
	logRunRepo(t, logRunOrigin)
	state := logRunEnv(t)
	t.Setenv("NM_GATE", "1")
	if line := logRunOne(t, state, 0, "", "go"); !strings.Contains(line, `"gate":true`) {
		t.Fatalf("NM_GATE=1: %s", line)
	}
	t.Setenv("NM_GATE", "")
	if line := logRunOne(t, state, 0, "", "go"); !strings.Contains(line, `"gate":false`) {
		t.Fatalf("NM_GATE unset: %s", line)
	}
}

func TestLogRunAppendsAcrossCalls(t *testing.T) {
	logRunRepo(t, logRunOrigin)
	state := logRunEnv(t)
	kept := writeKept(t, "")
	callLogRun(LogRunArgs{Mode: "staged", Kept: kept, Langs: []string{"ts", "go"}})
	firstTwo := logRunLines(t, state)
	callLogRun(LogRunArgs{Mode: "staged", Kept: kept, Langs: []string{"dotnet"}})
	got := logRunLines(t, state)
	if len(got) != 3 || !reflect.DeepEqual(got[:2], firstTwo) {
		t.Fatalf("log = %q, want 3 lines with the first two unchanged", got)
	}
}

func TestLogRunGitLookupsThatFailYieldEmptyFields(t *testing.T) {
	pinGitEnv(t)
	logRunGit(t, "init", "--quiet", "--initial-branch=main")
	state := logRunEnv(t)
	line := logRunOne(t, state, 0, "", "go")
	for _, want := range []string{`"repo":""`, `"branch":"main"`, `"head":""`} {
		if !strings.Contains(line, want) {
			t.Fatalf("line %s lacks %s", line, want)
		}
	}
}

func TestLogRunDetachedHeadHasNoBranch(t *testing.T) {
	logRunRepo(t, logRunOrigin)
	state := logRunEnv(t)
	logRunGit(t, "checkout", "--quiet", "--detach")
	if line := logRunOne(t, state, 0, "", "go"); !strings.Contains(line, `"branch":""`) {
		t.Fatalf("line = %s", line)
	}
}

func TestLogRunScpOriginIsKeptVerbatim(t *testing.T) {
	logRunRepo(t, "git@github.com:o/r.git")
	state := logRunEnv(t)
	if line := logRunOne(t, state, 0, "", "go"); !strings.Contains(line, `"repo":"git@github.com:o/r.git"`) {
		t.Fatalf("line = %s", line)
	}
}

func TestLogRunDefaultsToHomeLocalState(t *testing.T) {
	logRunRepo(t, logRunOrigin)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("NM_GATE", "")
	code, _, stderr := callLogRun(LogRunArgs{Mode: "staged", Kept: writeKept(t, ""), Langs: []string{"go"}})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "state", "coding-standards", "lint-runs.jsonl")); err != nil {
		t.Fatal(err)
	}
}

func TestLogRunUnwritableStateFailsLoudly(t *testing.T) {
	logRunRepo(t, logRunOrigin)
	file := filepath.Join(t.TempDir(), "regular")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", file)
	code, stdout, stderr := callLogRun(LogRunArgs{Mode: "staged", Kept: writeKept(t, ""), Langs: []string{"go"}})
	if code != 1 || stderr == "" || stdout != "" {
		t.Fatalf("exit %d stdout %q stderr %q, want exit 1, stderr set, stdout empty", code, stdout, stderr)
	}
}

func TestLogRunMissingKeptFailsAndWritesNothing(t *testing.T) {
	logRunRepo(t, logRunOrigin)
	state := logRunEnv(t)
	code, _, stderr := callLogRun(LogRunArgs{Mode: "staged", Kept: filepath.Join(t.TempDir(), "absent"), Langs: []string{"go"}})
	if code != 1 || stderr == "" {
		t.Fatalf("exit %d stderr %q, want exit 1 and stderr set", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(state, "coding-standards", "lint-runs.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("log file exists after a failed call: %v", err)
	}
}
