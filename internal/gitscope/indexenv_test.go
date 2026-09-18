package gitscope

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// `git commit -a` and `git commit -- <pathspec>` build a temporary index out of
// the working tree and point the pre-commit hook at it with GIT_INDEX_FILE.
// Scrubbing that variable with the rest of git's namespace sends WriteTree to
// .git/index instead, so a waiver is matched and spent against a tree the
// commit never writes: the developer spends a waiver and the next attempt at
// the same commit asks for another.
func TestWriteTreeHashesTheIndexTheEnvironmentNames(t *testing.T) {
	opened, _ := dirtyRepo(t, 1)
	dir := opened.root.Dir()
	alternate := filepath.Join(t.TempDir(), "next-index")

	indexGit(t, dir, alternate, "read-tree", "HEAD")
	indexGit(t, dir, alternate, "add", "--all")
	want := indexGit(t, dir, alternate, "write-tree")
	if other := indexGit(t, dir, "", "write-tree"); other == want {
		t.Fatalf("the alternate index hashes to the same tree as .git/index (%s), so the case proves nothing", want)
	}

	t.Setenv("GIT_INDEX_FILE", alternate)
	t.Chdir(dir)
	repo, err := OpenHook()
	if err != nil {
		t.Fatalf("OpenHook() err = %v", err)
	}
	got, err := repo.WriteTree()

	if err != nil {
		t.Fatalf("WriteTree() err = %v", err)
	}
	if got != want {
		t.Errorf("WriteTree() = %s, want %s, the tree the named index holds", got, want)
	}
}

// A run nothing handed an index reads the repository's own, whatever the shell
// that started it left in GIT_INDEX_FILE. The gate is that run: it measures the
// repository it was started in, and an inherited variable naming another
// repository's index would answer every question there instead and report no
// changed methods.
func TestOpenReadsTheRepositorysOwnIndexWhateverTheEnvironmentNames(t *testing.T) {
	opened, _ := dirtyRepo(t, 1)
	dir := opened.root.Dir()
	alternate := filepath.Join(t.TempDir(), "next-index")

	indexGit(t, dir, alternate, "read-tree", "HEAD")
	indexGit(t, dir, alternate, "add", "--all")
	want := indexGit(t, dir, "", "write-tree")
	if other := indexGit(t, dir, alternate, "write-tree"); other == want {
		t.Fatalf("the alternate index hashes to the same tree as .git/index (%s), so the case proves nothing", want)
	}

	t.Setenv("GIT_INDEX_FILE", alternate)
	t.Chdir(dir)
	repo, err := Open()
	if err != nil {
		t.Fatalf("Open() err = %v", err)
	}
	got, err := repo.WriteTree()

	if err != nil {
		t.Fatalf("WriteTree() err = %v", err)
	}
	if got != want {
		t.Errorf("WriteTree() = %s, want %s, the tree .git/index holds", got, want)
	}
}

// indexGit runs one git command against an explicit index, or the repository's
// own when index is empty, and returns its trimmed stdout.
func indexGit(t *testing.T, dir, index string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
	)
	if index != "" {
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+index)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
