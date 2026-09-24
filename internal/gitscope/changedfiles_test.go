package gitscope

import (
	"errors"
	"slices"
	"testing"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

func TestStagedFilesListsWhatTheCommitAddsOrModifies(t *testing.T) {
	repo, dir := changedFilesRepo(t)
	writeFixtureFile(t, dir, "Edited.cs", "// edited\n")
	writeFixtureFile(t, dir, "Added File.cs", "// added\n")
	fixtureGit(t, dir, "rm", "--quiet", "Deleted.cs")
	fixtureGit(t, dir, "add", "--all")

	got, err := repo.StagedFiles()

	want := []srcpath.Path{"Added File.cs", "Edited.cs"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("StagedFiles returned %v, %v, want %v", got, err, want)
	}
}

// Rename detection would make the moved file an R, which ACM drops, so the edit
// made on the way would reach no linter.
func TestStagedFilesKeepsTheNewSideOfARenameAndEdit(t *testing.T) {
	repo, dir := changedFilesRepo(t)
	fixtureGit(t, dir, "mv", "Kept.cs", "Moved.cs")
	writeFixtureFile(t, dir, "Moved.cs", "// one\n// two\n// three\n// four\n// five\n// six\n")
	fixtureGit(t, dir, "add", "--all")

	got, err := repo.StagedFiles()

	want := []srcpath.Path{"Moved.cs"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("StagedFiles returned %v, %v, want %v", got, err, want)
	}
}

func TestStagedFilesListsTheFirstCommitOfAnUnbornBranch(t *testing.T) {
	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--quiet")
	writeFixtureFile(t, dir, "First.cs", "// first\n")
	fixtureGit(t, dir, "add", "--all")

	got, err := fixtureRepo(t, dir).StagedFiles()

	want := []srcpath.Path{"First.cs"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("StagedFiles returned %v, %v, want %v", got, err, want)
	}
}

// The merge brings Incoming.cs untouched, combines both sides' edits to
// Shared.cs, and Edited.cs is changed while merging. Only the combination and
// the edit differ from both parents. Branch.cs, committed on the branch before
// the merge, differs from MERGE_HEAD alone.
func TestStagedFilesDuringAMergeListsOnlyWhatDiffersFromBothParents(t *testing.T) {
	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--quiet", "--initial-branch=main")
	writeFixtureFile(t, dir, "Shared.cs", "// one\n// two\n// three\n// four\n")
	writeFixtureFile(t, dir, "Edited.cs", "// base\n")
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "base")
	fixtureGit(t, dir, "checkout", "--quiet", "-b", "incoming")
	writeFixtureFile(t, dir, "Shared.cs", "// incoming\n// two\n// three\n// four\n")
	writeFixtureFile(t, dir, "Incoming.cs", "// incoming\n")
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "incoming")
	fixtureGit(t, dir, "checkout", "--quiet", "main")
	writeFixtureFile(t, dir, "Shared.cs", "// one\n// two\n// three\n// branch\n")
	writeFixtureFile(t, dir, "Branch.cs", "// branch\n")
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "branch")
	fixtureGit(t, dir, "merge", "--quiet", "--no-commit", "--no-ff", "incoming")
	writeFixtureFile(t, dir, "Edited.cs", "// edited while merging\n")
	fixtureGit(t, dir, "add", "--all")

	got, err := fixtureRepo(t, dir).StagedFiles()

	want := []srcpath.Path{"Edited.cs", "Shared.cs"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("StagedFiles returned %v, %v, want %v", got, err, want)
	}
}

// Edited.cs changes only on main after the branch point. Against main's tip it
// would differ, against the merge base it does not.
func TestFilesSinceDiffsTheWorkingTreeAgainstTheMergeBase(t *testing.T) {
	repo, dir := changedFilesRepo(t)
	fixtureGit(t, dir, "branch", "topic")
	writeFixtureFile(t, dir, "Edited.cs", "// changed on main\n")
	fixtureGit(t, dir, "commit", "--quiet", "-am", "main moves on")
	fixtureGit(t, dir, "checkout", "--quiet", "topic")
	writeFixtureFile(t, dir, "Kept.cs", "// edited on disk, never staged\n")

	got, err := repo.FilesSince("main")

	want := []srcpath.Path{"Kept.cs"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("FilesSince returned %v, %v, want %v", got, err, want)
	}
}

func TestFilesSinceARefThatDoesNotResolveIsNoBase(t *testing.T) {
	repo, _ := changedFilesRepo(t)

	got, err := repo.FilesSince("no-such-ref")

	var noBase NoBaseError
	if got != nil || !errors.As(err, &noBase) || noBase.Ref != "no-such-ref" {
		t.Errorf("FilesSince returned %v, %v, want no paths and a NoBaseError naming the ref", got, err)
	}
}

// changedFilesRepo is one commit of three files on main, the starting point
// each case edits.
func changedFilesRepo(t *testing.T) (Repo, string) {
	t.Helper()
	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--quiet", "--initial-branch=main")
	writeFixtureFile(t, dir, "Edited.cs", "// base\n")
	writeFixtureFile(t, dir, "Deleted.cs", "// base\n")
	writeFixtureFile(t, dir, "Kept.cs", "// one\n// two\n// three\n// four\n// five\n")
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "base")
	return fixtureRepo(t, dir), dir
}

func fixtureRepo(t *testing.T, dir string) Repo {
	t.Helper()
	root, err := srcpath.NewRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return Repo{root: root}
}
