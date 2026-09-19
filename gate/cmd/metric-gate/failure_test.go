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
		Kind:    gitscope.OpenKind(openKindBeyondEveryDeclaredKind),
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

// TestAMappedOpenKindKeepsItsCodeAndMessage is the other half of issue 122's
// fix: making an unmapped kind carry a composed message must not change what a
// mapped kind carries. Every kind gitscope declares goes through asFailure
// here, and each one keeps its own ADR 0008 code and gitscope's message
// verbatim, with none of the "the gate has no error code" prefix.
func TestAMappedOpenKindKeepsItsCodeAndMessage(t *testing.T) {
	want := map[gitscope.OpenKind]string{
		gitscope.OpenGitUnavailable:   report.CodeGitUnavailable,
		gitscope.OpenRepoUnreadable:   report.CodeGitRepoUnreadable,
		gitscope.OpenNoRepo:           report.CodeNoGitRepo,
		gitscope.OpenRootUnresolvable: report.CodeRepoRootUnresolvable,
	}

	for _, kind := range gitscope.OpenKinds() {
		message := fmt.Sprintf("gitscope rendered this for kind %d", kind)

		failure, ok := asFailure(gitscope.OpenError{Kind: kind, Message: message})

		if !ok {
			t.Errorf("asFailure did not type an OpenError of kind %v, so the run would exit 1 with no document", kind)
			continue
		}
		if failure.Code != want[kind] {
			t.Errorf("the code for kind %v is %q, want %q", kind, failure.Code, want[kind])
		}
		if failure.Message != message {
			t.Errorf("the message for kind %v is %q, want gitscope's own %q", kind, failure.Message, message)
		}
	}
}

// TestTheUnmappedOpenKindDocumentMatchesTheInternalErrorGolden is issue 122's
// outcome rather than its mechanism. The other cases read the Failure struct;
// this one starts from an OpenError, runs asFailure's real composition, and
// holds the whole document to gate/test/golden/internal_error.toon, its exit
// code and its stderr line. Before the fix openCode panicked here, so the
// process exited with no document at all, the one output shape ADR 0008
// refuses. This case is the golden's only owner, so a change to openCode's
// fallback code or asFailure's sentence reds here instead of shipping a
// document no golden describes. measure takes no gitscope seam, so an unmapped
// kind cannot be driven through it.
func TestTheUnmappedOpenKindDocumentMatchesTheInternalErrorGolden(t *testing.T) {
	failure, ok := asFailure(gitscope.OpenError{
		Kind:    gitscope.OpenKind(openKindBeyondEveryDeclaredKind),
		Message: `could not run git: exec: "git": executable file not found in $PATH`,
	})
	if !ok {
		t.Fatalf("asFailure did not type an unmapped OpenKind, so there is no document at all")
	}

	assertDocumentMatches(t, report.Document{Scope: "merge-base", Failure: failure}, "internal_error",
		"the gate has no error code for gitscope.OpenKind 99. could not run git: exec: \"git\": executable file not found in $PATH\n")
}

// TestTheFallbackCodeIsRegistered closes the half the mapped cases cannot
// reach. An unmapped kind's code has to be a code report registered, not a
// bare string openCode spells itself: ADR 0008's golden check walks
// report.Codes(), so a string missing from the registry ships a document
// naming a code nothing pins and nothing a caller branches on knows.
func TestTheFallbackCodeIsRegistered(t *testing.T) {
	code, mapped := openCode(gitscope.OpenKind(openKindBeyondEveryDeclaredKind))

	if mapped {
		t.Fatalf("openCode reported kind %d as mapped, want the fallback", openKindBeyondEveryDeclaredKind)
	}
	for _, registered := range report.Codes() {
		if registered == code {
			return
		}
	}
	t.Errorf("openCode fell back to %q, which report.Codes() does not list", code)
}

// openKindBeyondEveryDeclaredKind is a kind gitscope does not declare, which
// is the only way to reach the fallback: every kind gitscope.OpenKinds lists
// is mapped, pinned by TestOpenCodeMapsEveryDeclaredKind.
const openKindBeyondEveryDeclaredKind = 99

// TestOpenCodeMapsEveryDeclaredKind walks gitscope.OpenKinds rather than
// naming the four kinds again here. OpenKinds derives its list from gitscope's
// own count, so a fifth kind declared there without a case added to openCode
// reds this test instead of silently falling through to
// report.CodeInternalError.
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
