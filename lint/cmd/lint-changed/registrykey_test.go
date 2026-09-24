package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRegistryKeyTakesNoArguments(t *testing.T) {
	cmd, err := Parse([]string{"registry-key"})
	if err != nil || cmd.Kind != KindRegistryKey {
		t.Errorf("Parse(registry-key) = %+v, %v, want the registry-key form", cmd, err)
	}
	if _, err := Parse([]string{"registry-key", "--staged"}); err == nil {
		t.Error("Parse(registry-key --staged) succeeded, want a usage error")
	}
}

func TestRegistryKeyIsTheRepoItself(t *testing.T) {
	repo := committedRepo(t)
	chdirWithoutRegistryKey(t, filepath.Join(repo, "sub"))

	assertRegistryKey(t, resolved(t, repo))
}

// A linked worktree is the same adoption as the checkout it was made from.
func TestRegistryKeyOfALinkedWorktreeIsItsMainCheckout(t *testing.T) {
	repo := committedRepo(t)
	worktree := filepath.Join(t.TempDir(), "linked")
	repoGit(t, repo, "worktree", "add", "--quiet", worktree)
	chdirWithoutRegistryKey(t, worktree)

	assertRegistryKey(t, resolved(t, repo))
}

// The .NET props condition matches the resolved path, so a checkout entered
// through a symlink keys on where the link points.
func TestRegistryKeyOfASymlinkedCheckoutIsResolved(t *testing.T) {
	repo := committedRepo(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	chdirWithoutRegistryKey(t, link)

	assertRegistryKey(t, resolved(t, repo))
}

// A relative value is read against the working directory, the way the shell's
// cd read it, and resolved like every other key.
func TestRegistryKeyEnvNamesTheKeyResolved(t *testing.T) {
	repo := committedRepo(t)
	named := t.TempDir()
	if err := os.Symlink(named, filepath.Join(repo, "named")); err != nil {
		t.Fatal(err)
	}
	chdirWithoutRegistryKey(t, repo)
	t.Setenv(registryKeyEnv, "named")

	assertRegistryKey(t, resolved(t, named))
}

// A key naming no directory matches no registry, so every branch would skip
// and the gate would pass linting nothing.
func TestRegistryKeyEnvNamingNoDirectoryFails(t *testing.T) {
	repo := committedRepo(t)
	chdirWithoutRegistryKey(t, repo)
	for _, named := range []string{filepath.Join(repo, "typo"), filepath.Join(repo, "one.go")} {
		t.Setenv(registryKeyEnv, named)
		var stdout, stderr strings.Builder

		code := run(Command{Kind: KindRegistryKey}, &stdout, &stderr)

		want := "TVRMSMITH_REGISTRY_KEY names '" + named + "', which is not a directory"
		if code != 1 || stdout.String() != "" || !strings.Contains(stderr.String(), want) {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 1 saying %q", named, code, stdout.String(), stderr.String(), want)
		}
	}
}

func TestRegistryKeyOutsideARepositoryFails(t *testing.T) {
	chdirWithoutRegistryKey(t, t.TempDir())
	var stdout, stderr strings.Builder

	code := run(Command{Kind: KindRegistryKey}, &stdout, &stderr)

	if code != 1 || stdout.String() != "" || stderr.String() == "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 1 with gitscope's message", code, stdout.String(), stderr.String())
	}
}

// committedRepo is a repository with one commit, which `git worktree add`
// needs, and a subdirectory to run from.
func committedRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	repoGit(t, repo, "init", "--quiet")
	writeRepoFile(t, repo, "one.go")
	if err := os.Mkdir(filepath.Join(repo, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	repoGit(t, repo, "add", "--all")
	repoGit(t, repo, "commit", "--quiet", "-m", "base")
	return repo
}

// chdirWithoutRegistryKey clears an ambient TVRMSMITH_REGISTRY_KEY, which
// would otherwise answer every case.
func chdirWithoutRegistryKey(t *testing.T, dir string) {
	t.Helper()
	t.Setenv(registryKeyEnv, "")
	t.Chdir(dir)
}

// resolved is dir through its symlinks. t.TempDir on macOS sits under /var,
// itself a link to /private/var.
func resolved(t *testing.T, dir string) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func assertRegistryKey(t *testing.T, want string) {
	t.Helper()
	var stdout, stderr strings.Builder

	code := run(Command{Kind: KindRegistryKey}, &stdout, &stderr)

	if code != 0 || stdout.String() != want+"\n" {
		t.Errorf("exit %d, stdout %q, stderr %q, want exit 0 and %q", code, stdout.String(), stderr.String(), want)
	}
}
