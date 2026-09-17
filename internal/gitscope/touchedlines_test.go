package gitscope

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TouchedLines is the boundary the whole package exists behind, and the only
// in-package cases reaching it read canned git output. These two drive it
// against a real repository, because what the move above gate/ changed is the
// type crossing this boundary: the error is the package's own
// UnreadableDiffError now rather than metric-gate's document failure, and
// nothing else executes the boundary to see which one comes back.
func TestTouchedLinesNamesTheEditedLines(t *testing.T) {
	repo, paths := dirtyRepo(t, 1)

	touched, err := repo.TouchedLines(Base{Ref: "HEAD", Commit: fixtureHead(t, repo)})

	if err != nil {
		t.Fatalf("TouchedLines against the fixture's own HEAD errored %v, want the edited lines", err)
	}
	// dirtyRepo commits two comment lines and rewrites the second on disk.
	if want := []int{2}; !slices.Equal(touched[paths[0]], want) {
		t.Errorf("TouchedLines named %v for %s, want %v", touched[paths[0]], paths[0], want)
	}
}

// A git that will not answer has to reach the caller as the type callers match
// on. Returning it bare would leave metric-gate's asFailure unable to type it,
// so the run exits 1 with no document at all, which is the shape ADR 0008
// refuses.
func TestTouchedLinesTypesAGitRefusalAsAnUnreadableDiff(t *testing.T) {
	repo, _ := dirtyRepo(t, 1)
	// A well-formed object name the store does not hold, so git fails inside
	// the diff rather than anywhere base resolution could have caught it.
	absent := strings.Repeat("0", 40)

	touched, err := repo.TouchedLines(Base{Ref: "absent", Commit: absent})

	var unreadable *UnreadableDiffError
	if touched != nil || !errors.As(err, &unreadable) {
		t.Fatalf("TouchedLines against a missing commit returned %v, erroring %v, want no paths and an UnreadableDiffError", touched, err)
	}
	if !strings.HasPrefix(unreadable.Message, "could not read the diff: ") {
		t.Errorf("the refusal reads %q, want it to open with the rendered diff-reading cause", unreadable.Message)
	}
}

// fixtureHead is the fixture repository's HEAD commit, which is what a
// resolver hands TouchedLines in production rather than the ref spelling.
func fixtureHead(t *testing.T, repo Repo) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo.Root().Dir()
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
