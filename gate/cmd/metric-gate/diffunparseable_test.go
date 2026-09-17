package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/metric"
	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/gate/internal/scope"
)

// gitscope stopped returning report.Failure when it moved above gate/, so the
// only thing carrying a diff it could not read into the document is the
// asFailure mapping. failure_test.go drives that mapping directly; this case
// runs the whole gate over a repository whose base tree object is gone, which
// is the one path where an UnreadableDiffError is raised by git rather than by
// the test, and pins that it still lands in the document as diff_unparseable
// with exit 1 instead of escaping as an untyped error and leaving no document.
func TestARepositoryGitCannotDiffIsMeasuredAsDiffUnparseable(t *testing.T) {
	dir := unreadableDiffRepo(t)
	t.Chdir(dir)

	doc, err := measure(scope.Scope{
		Mode:    scope.ModeSince,
		Ref:     "HEAD",
		Metrics: []metric.Selection{declaresNothing},
	})

	if err != nil {
		t.Fatalf("measure returned the error %v, want a document carrying the cause", err)
	}
	if doc.Failure == nil {
		t.Fatalf("measure returned a document with no error block, want the unreadable diff in one")
	}
	if doc.Failure.Code != report.CodeDiffUnparseable {
		t.Errorf("the document's code is %q, want %q", doc.Failure.Code, report.CodeDiffUnparseable)
	}
	if !strings.HasPrefix(doc.Failure.Message, "could not read the diff: ") {
		t.Errorf("the document's message is %q, want the sentence gitscope rendered", doc.Failure.Message)
	}
	if doc.ExitCode() != 1 {
		t.Errorf("the run exits %d, want 1", doc.ExitCode())
	}
}

// unreadableDiffRepo is a repository whose HEAD commit resolves but whose tree
// cannot be read, so base resolution succeeds and the diff behind it is what
// fails. Deleting the tree object is the smallest way to get there: git still
// verifies the commit, then refuses the diff that has to walk into it.
func unreadableDiffRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--quiet")
	writeFixtureFile(t, dir, "src/Order.cs", "// one\n// two\n")
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "initial")

	tree := strings.TrimSpace(fixtureGitOut(t, dir, "rev-parse", "HEAD^{tree}"))
	if err := os.Remove(filepath.Join(dir, ".git", "objects", tree[:2], tree[2:])); err != nil {
		t.Fatalf("removing the base tree object: %v", err)
	}
	// The working tree has to differ from the base, so the diff is asked for
	// rather than answered out of an unchanged index.
	writeFixtureFile(t, dir, "src/Order.cs", "// one\n// on disk\n")
	return dir
}

// fixtureGit runs one repository-building git command. The identity and both
// config files are pinned, because a runner with no user.email cannot commit
// and a developer's own global config is not the fixture's to inherit. The
// gate's own invocations scrub the same environment themselves.
func fixtureGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	fixtureGitOut(t, dir, args...)
}

func fixtureGitOut(t *testing.T, dir string, args ...string) string {
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
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

func writeFixtureFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
