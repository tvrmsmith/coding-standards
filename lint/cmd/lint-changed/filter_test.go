package main

import (
	"testing"

	"github.com/tvrmsmith/coding-standards/lint/internal/lintfind"
)

// One filter run reads every language's reports, so two findings alike in
// rule, message and location but reported by two languages are two findings,
// each needing its own waiver under its own language key.
func TestDedupKeepsFindingsThatDifferOnlyInLanguage(t *testing.T) {
	loc := []lintfind.Location{{Path: "a.go", StartLine: 3, StartColumn: 1, EndLine: 3}}
	findings := []lintfind.Finding{
		{Language: lintfind.LanguageGo, Rule: "R1", Message: "m", Locations: loc},
		{Language: lintfind.LanguageTS, Rule: "R1", Message: "m", Locations: loc},
		{Language: lintfind.LanguageGo, Rule: "R1", Message: "m", Locations: loc},
	}

	got := dedup(findings)

	if len(got) != 2 {
		t.Fatalf("dedup kept %d finding(s), want 2: one per language", len(got))
	}
	if got[0].Language != lintfind.LanguageGo || got[1].Language != lintfind.LanguageTS {
		t.Fatalf("languages = %q, %q, want go then ts", got[0].Language, got[1].Language)
	}
}
