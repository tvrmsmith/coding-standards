package gitscope

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/srcpath"
)

// e2bigTarget is how much argv one invocation has to build before exec refuses
// it on the host running the case. macOS stops at a fixed 1 MiB. Linux takes
// max(min(6 MiB, RLIMIT_STACK/4), 128 KiB), so a container with a large or
// unlimited stack accepts far more than the 8 MiB stack a developer's shell
// hands out, and a target written down as a constant would exec fine there and
// leave the case green against the unbatched code it exists to refuse. It is
// read from the live rlimit instead, with a budget's slack on top, since argv
// and the environment share the ceiling. Batched, the same list is
// target/divergenceBudget invocations, a size no platform refuses.
func e2bigTarget(t *testing.T) int {
	t.Helper()
	var stack syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_STACK, &stack); err != nil {
		t.Fatal(err)
	}
	return int(max(min(uint64(6<<20), stack.Cur/4), 128<<10)) + divergenceBudget
}

// The defect issue 50 reports is an exec that never happens. Reproducing it
// takes more pathspec bytes than any black-box fixture can put on disk, which
// is what this case does without the files: a `:(literal)` pathspec matching
// nothing is not an error to git, it contributes no numstat record, so the
// bytes are free and the expected answer is unchanged by them.
func TestDivergentFromIndexFindsTheDirtyFileAmongMorePathspecsThanOneExecCarries(t *testing.T) {
	repo, dirty := dirtyRepo(t, 1)
	target := e2bigTarget(t)
	// The real path goes last, where a single unbounded argv puts it too, so
	// nothing about the ordering makes the answer easier to reach.
	paths := append(unmatchedPaths(target), dirty...)
	// Unbatched, this argv is one exec that fails with E2BIG and comes back as
	// diff_unparseable, which says git could not parse a diff it never ran. A
	// layout that stopped clearing the target would leave the case green
	// against exactly the code it exists to refuse.
	if argv := divergenceArgvBytes(paths); argv <= target {
		t.Fatalf("the argv over %d paths is %d bytes, want past the %d target no exec accepts", len(paths), argv, target)
	}

	divergent, err := repo.DivergentFromIndex(paths)

	// The paths are counted rather than printed. Printing them is two megabytes
	// of synthetic filler in the CI log for a case whose whole answer is the one
	// path it did or did not name.
	if err != nil || !slices.Equal(divergent, dirty) {
		t.Fatalf("DivergentFromIndex over %d paths named %d of them, erroring %v, want %s alone and no error", len(paths), len(divergent), err, dirty[0])
	}
}

// The answer is the union of what every batch named, and nothing else pins
// that. The two cases either side of this one put their single divergent path
// last, so a loop that kept only the final batch's records still returns it
// and still passes. This one spreads three divergent paths over three
// invocations, where keeping one batch loses two of them.
func TestDivergentFromIndexUnionsTheDirtyFilesEveryBatchNames(t *testing.T) {
	repo, dirty := dirtyRepo(t, 3)
	// Every divergent path is followed by a budget's worth of pathspecs that
	// match nothing, which forces a boundary before the next one, so no two of
	// the three can come back from the same invocation. The filler repeats
	// between them, which changes no answer, since a pathspec naming no file
	// contributes no record however many times git is handed it.
	var paths []srcpath.Path
	for _, path := range dirty {
		paths = append(paths, path)
		paths = append(paths, unmatchedPaths(divergenceBudget)...)
	}
	if naming := batchesNaming(paths, dirty); naming != len(dirty) {
		t.Fatalf("the %d divergent paths fall in %d batches, want one batch each", len(dirty), naming)
	}

	divergent, err := repo.DivergentFromIndex(paths)

	if err != nil || !slices.Equal(divergent, dirty) {
		t.Fatalf("DivergentFromIndex over %d paths named %v, erroring %v, want all of %v in that order", len(paths), divergent, err, dirty)
	}
}

// A batch that git refuses has to end the call, not be skipped. The union
// makes a swallowed failure look like an answer: the earlier batches already
// named paths, so returning what they found hands the caller a shorter
// divergent list than the truth and the gate passes a changeset it never
// finished asking about. The first batch failing is a shape the unbatched code
// already had, so the case drives the failure out of the second one.
func TestDivergentFromIndexFailsWhenALaterBatchDoes(t *testing.T) {
	repo, dirty := dirtyRepo(t, 1)
	// git refuses a pathspec leading out of the repository, which is a failure
	// of the invocation carrying it rather than of the paths beside it.
	outside := srcpath.Path("../outside.cs")
	paths := slices.Concat(dirty, unmatchedPaths(divergenceBudget), []srcpath.Path{outside})
	batches := divergenceBatches(paths)
	if len(batches) < 2 || slices.Contains(batches[0], outside) {
		t.Fatalf("the layout split into %d batches with the refused pathspec in the first, want it in a later one", len(batches))
	}

	divergent, err := repo.DivergentFromIndex(paths)

	if err == nil || divergent != nil {
		t.Fatalf("DivergentFromIndex with a refused second batch named %v, erroring %v, want no paths and an error rather than the first batch's union", divergent, err)
	}
}

// batchesNaming counts the batches holding at least one of wanted. The union
// case asserts on it rather than on the budget arithmetic, because what the
// case needs is that its divergent paths really did arrive from different
// invocations.
func batchesNaming(paths, wanted []srcpath.Path) int {
	naming := 0
	for _, batch := range divergenceBatches(paths) {
		if slices.ContainsFunc(batch, func(path srcpath.Path) bool { return slices.Contains(wanted, path) }) {
			naming++
		}
	}
	return naming
}

// unmatchedPaths lays out synthetic paths naming no file, enough of them that
// the argv over them passes target. The count is derived rather than written
// down, so the case still reproduces E2BIG after someone changes how long a
// synthetic path is.
func unmatchedPaths(target int) []srcpath.Path {
	const absent = "src/Absent/%s%05d.cs"
	filler := strings.Repeat("m", 500)
	perPath := len(pathspec(srcpath.Path(fmt.Sprintf(absent, filler, 0)))) + 1
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

// dirtyRepo is a repository holding count source files, each staged in one
// state and left in another on disk, which is the divergence the guard exists
// to find. This package tests parsers against canned git output everywhere
// else, so the fixture is the smallest real repository that can answer a
// question about what git itself reports, one commit, one staged edit per
// file, one edit on disk after it.
func dirtyRepo(t *testing.T, count int) (Repo, []srcpath.Path) {
	t.Helper()
	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--quiet")
	paths := make([]srcpath.Path, 0, count)
	for i := range count {
		rel := fmt.Sprintf("src/Order%02d.cs", i)
		paths = append(paths, srcpath.Path(rel))
		writeFixtureFile(t, dir, rel, "// one\n// two\n")
	}
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "initial")
	for _, path := range paths {
		writeFixtureFile(t, dir, path.String(), "// one\n// staged\n")
	}
	fixtureGit(t, dir, "add", "--all")
	for _, path := range paths {
		writeFixtureFile(t, dir, path.String(), "// one\n// on disk\n")
	}

	root, err := srcpath.NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return Repo{root: root}, paths
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
