package gitscope

import (
	"errors"
	"strings"
	"testing"
)

// TestOpenOutsideAGitRepositoryIsTyped pins issue 86's contract at the seam
// that owns it: starting a run where no repository resolves returns an
// OpenError kinded OpenNoRepo, so a caller can put it in its document rather
// than exiting 1 with empty stdout. The code that kind maps onto is the
// caller's, pinned in gate/cmd/metric-gate and by gate/test's golden.
func TestOpenOutsideAGitRepositoryIsTyped(t *testing.T) {
	// git ships translations and sanitizedEnv passes the locale straight
	// through, so the assertion below is held to English the way
	// gate/test/harness_test.go holds its goldens to it. LANGUAGE is emptied
	// beside LC_ALL because it outranks LC_ALL for message translation.
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANGUAGE", "")
	t.Chdir(t.TempDir())

	_, err := Open()

	var open OpenError
	if !errors.As(err, &open) {
		t.Fatalf("Open() err = %v, want a gitscope.OpenError", err)
	}
	if open.Kind != OpenNoRepo {
		t.Errorf("Kind = %v, want OpenNoRepo", open.Kind)
	}
	// git's own complaint is quoted rather than the argv, so the message names
	// why no repository resolved.
	if !strings.HasPrefix(open.Message, "could not find a git repository: ") {
		t.Errorf("Message = %q, want it to open with the could-not-find prefix", open.Message)
	}
	if !strings.Contains(open.Message, "not a git repository") {
		t.Errorf("Message = %q, want git's own complaint inside it", open.Message)
	}
}
