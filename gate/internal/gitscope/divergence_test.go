package gitscope

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// e2bigTarget is how much argv one invocation has to build before exec refuses
// it wherever the gate runs. macOS stops at 1 MiB, and Linux at a quarter of
// the stack rlimit, which is 2 MiB on the 8 MiB stack CI's runners set, and the
// environment rides on the same limit, so an argv past 2 MiB clears both.
// Batched, the same list is e2bigTarget/divergenceBudget invocations, a size no
// platform refuses.
const e2bigTarget = 2 << 20

// The defect issue 50 reports is an exec that never happens. Reproducing it
// takes more pathspec bytes than any black-box fixture can put on disk, which
// is what this case does without the files: a `:(literal)` pathspec matching
// nothing is not an error to git, it contributes no numstat record, so the
// bytes are free and the expected answer is unchanged by them.
func TestDivergentFromIndexFindsTheDirtyFileAmongMorePathspecsThanOneExecCarries(t *testing.T) {
	repo, dirty := dirtyRepo(t)
	// The real path goes last, where a single unbounded argv puts it too, so
	// nothing about the ordering makes the answer easier to reach.
	paths := append(unmatchedPaths(e2bigTarget), dirty)
	// Unbatched, this argv is one exec that fails with E2BIG and comes back as
	// diff_unparseable, which says git could not parse a diff it never ran. A
	// layout that stopped clearing the target would leave the case green
	// against exactly the code it exists to refuse.
	if argv := divergenceArgvBytes(paths); argv <= e2bigTarget {
		t.Fatalf("the argv over %d paths is %d bytes, want past the %d target no exec accepts", len(paths), argv, e2bigTarget)
	}

	divergent, err := repo.DivergentFromIndex(paths)

	// The paths are counted rather than printed. Printing them is two megabytes
	// of synthetic filler in the CI log for a case whose whole answer is the one
	// path it did or did not name.
	if err != nil || !slices.Equal(divergent, []srcpath.Path{dirty}) {
		t.Fatalf("DivergentFromIndex over %d paths named %d of them, erroring %v, want %s alone and no error", len(paths), len(divergent), err, dirty)
	}
}

// unmatchedPaths lays out synthetic paths naming no file, enough of them that
// the argv over them passes target. The count is derived rather than written
// down, so the case still reproduces E2BIG after someone changes how long a
// synthetic path is.
func unmatchedPaths(target int) []srcpath.Path {
	const absent = "src/Absent/%s%05d.cs"
	filler := strings.Repeat("m", 500)
	perPath := len(":(literal)") + len(fmt.Sprintf(absent, filler, 0)) + 1
	paths := make([]srcpath.Path, 0, target/perPath+1)
	for i := range target/perPath + 1 {
		paths = append(paths, srcpath.Path(fmt.Sprintf(absent, filler, i)))
	}
	return paths
}

// divergenceArgvBytes is what one invocation over paths costs in the kernel's
// argv block, every argument plus the NUL terminating it. The argv comes from
// the production builder, so the accounting is over what the gate execs and
// not over a second guess at its flags.
func divergenceArgvBytes(paths []srcpath.Path) int {
	argv := 0
	for _, arg := range DivergenceArgs(paths) {
		argv += len(arg) + 1
	}
	return argv
}

// dirtyRepo is a repository holding one source file staged in one state and
// left in another on disk, which is the divergence the guard exists to find.
// This package tests parsers against canned git output everywhere else, so the
// fixture is the smallest real repository that can answer a question about
// what git itself reports: one commit, one staged edit, one edit after it.
func dirtyRepo(t *testing.T) (Repo, srcpath.Path) {
	t.Helper()
	dir := t.TempDir()
	const rel = "src/Order.cs"
	fixtureGit(t, dir, "init", "--quiet")
	writeFixtureFile(t, dir, rel, "// one\n// two\n")
	fixtureGit(t, dir, "add", rel)
	fixtureGit(t, dir, "commit", "--quiet", "-m", "initial")
	writeFixtureFile(t, dir, rel, "// one\n// staged\n")
	fixtureGit(t, dir, "add", rel)
	writeFixtureFile(t, dir, rel, "// one\n// on disk\n")

	root, err := srcpath.NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return Repo{root: root}, srcpath.Path(rel)
}

// fixtureGit runs one git command against the fixture repository. The identity
// and both config files are pinned, because a runner with no user.email cannot
// commit and a developer's own global config is not the fixture's to inherit.
// Repo's own invocations scrub the same environment themselves, so what is
// pinned here reaches only the commands that build the repository.
func fixtureGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
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

// writeFixtureFile puts content at the repo-relative rel, creating parents.
func writeFixtureFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
