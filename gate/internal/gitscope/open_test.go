package gitscope

import (
	"errors"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/report"
)

// TestOpenOutsideAGitRepositoryIsTyped pins issue 86's contract at the seam
// that owns it: starting the gate where no repository resolves returns a
// report.Failure coded no_git_repo, so measure can put it in the document
// rather than exiting 1 with empty stdout. The black-box suite runs the built
// binary, so this is the only place the typed error itself is observable.
func TestOpenOutsideAGitRepositoryIsTyped(t *testing.T) {
	// git ships translations and sanitizedEnv passes the locale straight
	// through, so the assertion below is held to English the way
	// gate/test/harness_test.go holds its goldens to it. LANGUAGE is emptied
	// beside LC_ALL because it outranks LC_ALL for message translation.
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANGUAGE", "")
	t.Chdir(t.TempDir())

	_, err := Open()

	var failure *report.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("Open() err = %v, want a *report.Failure", err)
	}
	if failure.Code != report.CodeNoGitRepo {
		t.Errorf("Code = %q, want %q", failure.Code, report.CodeNoGitRepo)
	}
	// git's own complaint is quoted rather than the argv, so the message names
	// why no repository resolved.
	if !strings.HasPrefix(failure.Message, "could not find a git repository: ") {
		t.Errorf("Message = %q, want it to open with the could-not-find prefix", failure.Message)
	}
	if !strings.Contains(failure.Message, "not a git repository") {
		t.Errorf("Message = %q, want git's own complaint inside it", failure.Message)
	}
}
