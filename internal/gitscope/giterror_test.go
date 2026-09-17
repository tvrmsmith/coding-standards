package gitscope

import (
	"errors"
	"os/exec"
	"testing"
)

// A git that never launched captured nothing on stderr, which is what
// exec.ErrNotFound means: a missing git, or a blanked filter driver binary git
// tried to run. Every fixture in the black-box suite makes git print something,
// so this arm is only reachable from here.
func TestGitErrorLeavesNoDanglingSeparatorWhenGitPrintedNothing(t *testing.T) {
	err := &gitError{args: []string{"diff", "--cached"}, err: exec.ErrNotFound}

	if got, want := err.Error(), "git diff --cached: "+exec.ErrNotFound.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestGitErrorQuotesTheComplaintGitPrinted(t *testing.T) {
	cause := errors.New("exit status 128")
	err := &gitError{args: []string{"merge-base", "HEAD", "main"}, err: cause, stderr: "fatal: not a valid object name"}

	want := "git merge-base HEAD main: exit status 128: fatal: not a valid object name"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
