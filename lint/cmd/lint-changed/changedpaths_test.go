package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseChangedPathsTakesEachScope(t *testing.T) {
	cases := []struct {
		args []string
		want Scope
	}{
		{[]string{"changed-paths", "--staged"}, Scope{Mode: ScopeStaged}},
		{[]string{"changed-paths", "--since", "main"}, Scope{Mode: ScopeSince, Ref: "main"}},
		{[]string{"changed-paths", "--files", "a,b.cs", "--files", "c.cs"}, Scope{Mode: ScopeFiles, Files: []string{"a,b.cs", "c.cs"}}},
	}
	for _, c := range cases {
		cmd, err := Parse(c.args)
		if err != nil {
			t.Fatalf("Parse(%v): %v", c.args, err)
		}
		if cmd.Kind != KindChangedPaths || !reflect.DeepEqual(cmd.ChangedPaths, c.want) {
			t.Errorf("Parse(%v) = %+v, want changed-paths over %+v", c.args, cmd, c.want)
		}
	}
}

// A filter flag is not a changed-paths flag, so a caller that mixes the two
// forms learns that rather than having the flag ignored.
func TestParseChangedPathsRefusesWhatItCannotRead(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"changed-paths"}, "exactly one of --staged, --since, --files is required"},
		{[]string{"changed-paths", "--staged", "--since", "main"}, "only one of --staged, --since, --files may be given"},
		{[]string{"changed-paths", "--staged", "--format", "sarif"}, "changed-paths: unknown argument '--format'"},
		{[]string{"changed-paths", "--since"}, "--since needs a value"},
	}
	for _, c := range cases {
		_, err := Parse(c.args)
		var ue *UsageError
		if !errors.As(err, &ue) || ue.Problem != c.want {
			t.Errorf("Parse(%v) = %v, want the usage error %q", c.args, err, c.want)
		}
	}
}

func TestChangedPathsPassesFilesThroughNulTerminated(t *testing.T) {
	var stdout, stderr strings.Builder

	code := run(Command{Kind: KindChangedPaths, ChangedPaths: Scope{Mode: ScopeFiles, Files: []string{"a b.ts", "gone.cs"}}}, &stdout, &stderr)

	if code != 0 || stdout.String() != "a b.ts\x00gone.cs\x00" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and both names as given", code, stdout.String(), stderr.String())
	}
}

func TestChangedPathsListsTheStagedFiles(t *testing.T) {
	dir := t.TempDir()
	repoGit(t, dir, "init", "--quiet")
	writeRepoFile(t, dir, "one.go")
	writeRepoFile(t, dir, "two.ts")
	repoGit(t, dir, "add", "--all")
	chdirOutsideAnyHook(t, dir)
	var stdout, stderr strings.Builder

	code := run(Command{Kind: KindChangedPaths, ChangedPaths: Scope{Mode: ScopeStaged}}, &stdout, &stderr)

	if code != 0 || stdout.String() != "one.go\x00two.ts\x00" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and both staged files", code, stdout.String(), stderr.String())
	}
}

// Under git commit -a the pre-commit hook names a temporary index in
// GIT_INDEX_FILE and .git/index still matches HEAD, so reading the default
// index would list nothing and lint nothing.
func TestChangedPathsListsTheIndexTheHookNames(t *testing.T) {
	dir := t.TempDir()
	repoGit(t, dir, "init", "--quiet")
	writeRepoFile(t, dir, "one.go")
	repoGit(t, dir, "add", "--all")
	repoGit(t, dir, "commit", "--quiet", "-m", "base")
	writeRepoFile(t, dir, "hooked.go")
	repoGit(t, dir, "add", "hooked.go")
	hookIndex := filepath.Join(t.TempDir(), "index")
	//nolint:gosec // G304: the index of the fixture repo this case just built under t.TempDir
	staged, err := os.ReadFile(filepath.Join(dir, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // G703: hookIndex is a fixed name under this case's own t.TempDir
	if err := os.WriteFile(hookIndex, staged, 0o600); err != nil {
		t.Fatal(err)
	}
	repoGit(t, dir, "reset", "--quiet")
	chdirOutsideAnyHook(t, dir)
	t.Setenv("GIT_INDEX_FILE", hookIndex)
	var stdout, stderr strings.Builder

	code := run(Command{Kind: KindChangedPaths, ChangedPaths: Scope{Mode: ScopeStaged}}, &stdout, &stderr)

	if code != 0 || stdout.String() != "hooked.go\x00" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and the file staged only in the hook's index", code, stdout.String(), stderr.String())
	}
}

// An empty stdout would read to the dispatcher as nothing changed, so a ref
// that names nothing has to fail the run.
func TestChangedPathsFailsOnASinceRefThatDoesNotResolve(t *testing.T) {
	dir := t.TempDir()
	repoGit(t, dir, "init", "--quiet")
	writeRepoFile(t, dir, "one.go")
	repoGit(t, dir, "add", "--all")
	repoGit(t, dir, "commit", "--quiet", "-m", "base")
	chdirOutsideAnyHook(t, dir)
	var stdout, stderr strings.Builder

	code := run(Command{Kind: KindChangedPaths, ChangedPaths: Scope{Mode: ScopeSince, Ref: "no-such-ref"}}, &stdout, &stderr)

	if code != 1 || stdout.String() != "" || !strings.Contains(stderr.String(), "no-such-ref") {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 naming the ref", code, stdout.String(), stderr.String())
	}
}

func TestChangedPathsOutsideARepositoryFails(t *testing.T) {
	chdirOutsideAnyHook(t, t.TempDir())
	var stdout, stderr strings.Builder

	code := run(Command{Kind: KindChangedPaths, ChangedPaths: Scope{Mode: ScopeStaged}}, &stdout, &stderr)

	if code != 1 || stdout.String() != "" || stderr.String() == "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 with gitscope's message", code, stdout.String(), stderr.String())
	}
}

// chdirOutsideAnyHook starts the case in dir with no hook index named, since
// OpenHook reads GIT_INDEX_FILE and a suite run from a hook would hand it one
// for another repository.
func chdirOutsideAnyHook(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("GIT_INDEX_FILE", "")
	t.Chdir(dir)
}

// repoGit runs git in a fixture repository with the identity pinned and the
// developer's own config and git namespace kept out, since a run under a hook
// inherits GIT_DIR and GIT_INDEX_FILE.
func repoGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	//nolint:gosec // a test helper running the literal "git" with argv this file writes, against a t.TempDir repo
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_AUTHOR_NAME=Fixture Author",
		"GIT_AUTHOR_EMAIL=author@fixture.invalid",
		"GIT_COMMITTER_NAME=Fixture Committer",
		"GIT_COMMITTER_EMAIL=committer@fixture.invalid",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeRepoFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
