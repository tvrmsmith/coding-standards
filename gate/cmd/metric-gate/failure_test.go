package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/tvrmsmith/coding-standards/gate/internal/report"
	"github.com/tvrmsmith/coding-standards/internal/gitscope"
)

// gitscope stopped naming report's codes when it moved above gate/ to be
// shared, so the code an unreadable diff carries into the document is chosen
// here and nowhere else. A black-box case would have to make a real git fail
// to reach it, which is the reason the mapping is driven directly instead.
func TestUnreadableDiffReachesTheDocumentAsDiffUnparseable(t *testing.T) {
	failure, ok := asFailure(gitscope.UnreadableDiffError{Message: "could not read the diff: git printed nothing"})

	if !ok {
		t.Fatalf("asFailure did not type an UnreadableDiffError, so the run would exit 1 with no document")
	}
	if failure.Code != report.CodeDiffUnparseable {
		t.Errorf("the code is %q, want %q", failure.Code, report.CodeDiffUnparseable)
	}
	if failure.Message != "could not read the diff: git printed nothing" {
		t.Errorf("the message is %q, want the one gitscope rendered", failure.Message)
	}
}

// An UnreadableDiffError arrives wrapped wherever a caller added context on the
// way up, and errors.As is what keeps the wrapping from costing the document
// its typed code.
func TestAWrappedUnreadableDiffStillReachesTheDocument(t *testing.T) {
	wrapped := fmt.Errorf("resolving the diff: %w", gitscope.UnreadableDiffError{Message: "could not read the diff: exit status 128"})

	failure, ok := asFailure(wrapped)

	if !ok || failure.Code != report.CodeDiffUnparseable {
		t.Errorf("asFailure on a wrapped UnreadableDiffError returned %v, %v, want a diff_unparseable failure", failure, ok)
	}
}

// Anything else stays an error, which is what makes the document's error block
// a list of causes the gate recognises rather than a place every stray failure
// lands with a code that does not describe it.
func TestAnUnrecognisedCauseIsNotTypedAsAFailure(t *testing.T) {
	if failure, ok := asFailure(errors.New("the extractor process was killed")); ok {
		t.Errorf("asFailure typed an unrecognised cause as %v, want it left as an error", failure)
	}
}

// TestAnUnmappedOpenKindStillReachesTheDocument pins issue 122. A fifth
// OpenKind added to gitscope used to reach asFailure by way of a panic, which
// exits with no document, the one shape ADR 0008 refuses. asFailure types it
// as report.CodeInternalError instead, with a message naming the kind that had
// no code.
func TestAnUnmappedOpenKindStillReachesTheDocument(t *testing.T) {
	failure, ok := asFailure(gitscope.OpenError{
		Kind:    gitscope.OpenKind(99),
		Message: "could not run git: exec: \"git\": executable file not found in $PATH",
	})

	if !ok {
		t.Fatalf("asFailure did not type an unmapped OpenKind, so the run would exit 1 with no document")
	}
	if failure.Code != report.CodeInternalError {
		t.Errorf("the code is %q, want %q", failure.Code, report.CodeInternalError)
	}
	want := "the gate has no error code for gitscope.OpenKind 99. could not run git: exec: \"git\": executable file not found in $PATH"
	if failure.Message != want {
		t.Errorf("the message is %q, want %q", failure.Message, want)
	}
}

// TestOpenCodeMapsEveryDeclaredKind walks gitscope.OpenKinds rather than
// naming the four kinds again here, so a fifth kind appended to that list
// without a case added to openCode reds this test instead of silently
// falling through to report.CodeInternalError.
func TestOpenCodeMapsEveryDeclaredKind(t *testing.T) {
	registered := map[string]bool{}
	for _, code := range report.Codes() {
		registered[code] = true
	}

	for _, kind := range gitscope.OpenKinds() {
		code, ok := openCode(kind)
		if !ok {
			t.Errorf("openCode(%v) reported no mapping, want every declared kind mapped", kind)
			continue
		}
		if !registered[code] {
			t.Errorf("openCode(%v) = %q, which report.Codes() does not list", kind, code)
		}
	}
}
