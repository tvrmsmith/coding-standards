package lintfind

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tvrmsmith/coding-standards/internal/srcpath"
)

// TestParseGolangCITwoIssues pins G1: the two-issue skeleton example, both
// Pos.Filename rewritten under the test root, parses to two Findings with no
// Dropped and no error, one per issue's own linter and severity.
func TestParseGolangCITwoIssues(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "main.go")
	filename := root.Abs(srcpath.Path("main.go"))

	doc := `{"Issues":[
		{"FromLinter":"forbidigo","Text":"use of fmt.Println forbidden","Severity":"","SourceLines":["\tfmt.Println(\"hi\")"],"Pos":{"Filename":"` + jsonEscape(filename) + `","Offset":43,"Line":6,"Column":2}},
		{"FromLinter":"revive","Text":"package-comments: should have a package comment","Severity":"warning","SourceLines":["package main"],"Pos":{"Filename":"` + jsonEscape(filename) + `","Offset":0,"Line":1,"Column":1},"LineRange":{"From":1,"To":1}}
	],"Report":{"Warnings":[],"Linters":[{"Name":"errcheck","Enabled":true}]}}`

	findings, dropped, err := ParseGolangCI(strings.NewReader(doc), root)

	if err != nil {
		t.Fatalf("ParseGolangCI() err = %v, want nil", err)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %v, want none", dropped)
	}
	want := []Finding{
		{
			Rule:      "forbidigo",
			Message:   "use of fmt.Println forbidden",
			Severity:  "warning",
			Locations: []Location{{Path: srcpath.Path("main.go"), StartLine: 6, StartColumn: 2, EndLine: 6}},
		},
		{
			Rule:      "revive",
			Message:   "package-comments: should have a package comment",
			Severity:  "warning",
			Locations: []Location{{Path: srcpath.Path("main.go"), StartLine: 1, StartColumn: 1, EndLine: 1}},
		},
	}
	if !reflect.DeepEqual(findings, want) {
		t.Errorf("findings = %#v, want %#v", findings, want)
	}
}

// TestParseGolangCILineRange pins G2: an issue carrying a LineRange parses to
// EndLine from LineRange.To, and an otherwise identical issue with no
// LineRange parses to EndLine equal to StartLine.
func TestParseGolangCILineRange(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "main.go")
	filename := jsonEscape(root.Abs(srcpath.Path("main.go")))

	withRange := `{"Issues":[{"FromLinter":"revive","Text":"a finding","Pos":{"Filename":"` + filename + `","Line":4,"Column":2},"LineRange":{"From":4,"To":9}}]}`
	findings, _, err := ParseGolangCI(strings.NewReader(withRange), root)
	if err != nil {
		t.Fatalf("ParseGolangCI() err = %v, want nil", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	got := findings[0].Locations[0]
	if got.StartLine != 4 || got.EndLine != 9 {
		t.Errorf("Location = %+v, want StartLine 4, EndLine 9", got)
	}

	withoutRange := `{"Issues":[{"FromLinter":"revive","Text":"a finding","Pos":{"Filename":"` + filename + `","Line":4,"Column":2}}]}`
	findings, _, err = ParseGolangCI(strings.NewReader(withoutRange), root)
	if err != nil {
		t.Fatalf("ParseGolangCI() err = %v, want nil", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	got = findings[0].Locations[0]
	if got.StartLine != 4 || got.EndLine != 4 {
		t.Errorf("Location = %+v, want StartLine 4, EndLine 4", got)
	}
}

// TestParseGolangCIZeroColumnDefaultsToOne pins G3: the real typecheck shape,
// Pos.Column of 0, parses to StartColumn 1.
func TestParseGolangCIZeroColumnDefaultsToOne(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "main.go")
	filename := jsonEscape(root.Abs(srcpath.Path("main.go")))

	doc := `{"Issues":[{"FromLinter":"typecheck","Text":"a finding","Pos":{"Filename":"` + filename + `","Line":1,"Column":0}}]}`
	findings, _, err := ParseGolangCI(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseGolangCI() err = %v, want nil", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if got := findings[0].Locations[0].StartColumn; got != 1 {
		t.Errorf("StartColumn = %d, want 1", got)
	}
}

// TestParseGolangCIKeepsOwnErrorSeverity pins G4: an issue's own "error"
// Severity is carried through rather than defaulted to "warning".
func TestParseGolangCIKeepsOwnErrorSeverity(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "main.go")
	filename := jsonEscape(root.Abs(srcpath.Path("main.go")))

	doc := `{"Issues":[{"FromLinter":"revive","Text":"a finding","Severity":"error","Pos":{"Filename":"` + filename + `","Line":1,"Column":1}}]}`
	findings, _, err := ParseGolangCI(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseGolangCI() err = %v, want nil", err)
	}
	if len(findings) != 1 || findings[0].Severity != "error" {
		t.Fatalf("findings = %#v, want Severity error", findings)
	}
}

// TestParseGolangCIDropsIssueOutsideRoot pins G5: an issue whose Pos.Filename
// names a path outside root parses to no Finding and one Dropped naming the
// filename as the report wrote it.
func TestParseGolangCIDropsIssueOutsideRoot(t *testing.T) {
	root := testRoot(t)
	outside := t.TempDir() + "/main.go"

	doc := `{"Issues":[{"FromLinter":"forbidigo","Text":"a finding","Pos":{"Filename":"` + jsonEscape(outside) + `","Line":1,"Column":1}}]}`
	findings, dropped, err := ParseGolangCI(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseGolangCI() err = %v, want nil", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
	want := []Dropped{{Rule: "forbidigo", URI: outside, Outside: true}}
	if !reflect.DeepEqual(dropped, want) {
		t.Errorf("dropped = %#v, want %#v", dropped, want)
	}
}

// TestParseGolangCIEmptyFromLinterIsUnparsed pins G6: an issue with an empty
// FromLinter, at a resolvable path and line, parses to one UNPARSED Finding
// carrying its Location. Nothing is dropped.
func TestParseGolangCIEmptyFromLinterIsUnparsed(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "main.go")
	filename := jsonEscape(root.Abs(srcpath.Path("main.go")))

	doc := `{"Issues":[{"FromLinter":"","Text":"unnamed","Pos":{"Filename":"` + filename + `","Line":1,"Column":1}}]}`
	findings, dropped, err := ParseGolangCI(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseGolangCI() err = %v, want nil", err)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %v, want none", dropped)
	}
	if len(findings) != 1 || findings[0].Rule != "UNPARSED" || !findings[0].IgnoresScope {
		t.Fatalf("findings = %#v, want one UNPARSED, IgnoresScope true", findings)
	}
	if len(findings[0].Locations) != 1 {
		t.Errorf("Locations = %v, want one", findings[0].Locations)
	}
}

// TestParseGolangCIZeroLineIsUnparsed pins G7: an issue with Pos.Line 0
// parses to one UNPARSED Finding that still names the file it came from. The
// path is what keys a waiver, so throwing it away would leave the reader a
// finding at the repo root and only a path-less waiver to clear it.
func TestParseGolangCIZeroLineIsUnparsed(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "main.go")
	filename := jsonEscape(root.Abs(srcpath.Path("main.go")))

	doc := `{"Issues":[{"FromLinter":"revive","Text":"a finding","Pos":{"Filename":"` + filename + `","Line":0,"Column":0}}]}`
	findings, _, err := ParseGolangCI(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseGolangCI() err = %v, want nil", err)
	}
	if len(findings) != 1 || findings[0].Rule != "UNPARSED" || !findings[0].IgnoresScope {
		t.Fatalf("findings = %#v, want one UNPARSED, IgnoresScope true", findings)
	}
	want := Location{Path: srcpath.Path("main.go"), StartLine: 1, StartColumn: 1, EndLine: 1}
	if len(findings[0].Locations) != 1 || findings[0].Locations[0] != want {
		t.Errorf("Locations = %#v, want [%#v]", findings[0].Locations, want)
	}
}

// TestParseGolangCILineRangeBelowPosIsRefused pins the floor on LineRange.To.
// A To below Pos.Line spans no line at all, so the scope filter would match
// it against nothing and the finding would vanish with neither a Dropped
// entry nor an UNPARSED marker, which is the silent drop the whole design
// refuses. The span falls back to the issue's own line instead.
func TestParseGolangCILineRangeBelowPosIsRefused(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "main.go")
	filename := jsonEscape(root.Abs(srcpath.Path("main.go")))

	for _, to := range []int{0, 3} {
		doc := fmt.Sprintf(
			`{"Issues":[{"FromLinter":"revive","Text":"a finding","Pos":{"Filename":"%s","Line":4,"Column":2},"LineRange":{"From":4,"To":%d}}]}`,
			filename, to)
		findings, _, err := ParseGolangCI(strings.NewReader(doc), root)
		if err != nil {
			t.Fatalf("ParseGolangCI(To %d) err = %v, want nil", to, err)
		}
		if len(findings) != 1 {
			t.Fatalf("To %d: len(findings) = %d, want 1", to, len(findings))
		}
		got := findings[0].Locations[0]
		if got.StartLine != 4 || got.EndLine != 4 {
			t.Errorf("To %d: Location = %+v, want StartLine 4, EndLine 4", to, got)
		}
	}
}

// TestParseGolangCICleanRunIsValid pins G8: an empty and a null Issues array
// both parse to no findings, no dropped and no error, since a clean run must
// not read as a broken parse.
func TestParseGolangCICleanRunIsValid(t *testing.T) {
	root := testRoot(t)

	for _, doc := range []string{`{"Issues":[]}`, `{"Issues":null}`} {
		findings, dropped, err := ParseGolangCI(strings.NewReader(doc), root)
		if err != nil {
			t.Fatalf("doc %s: ParseGolangCI() err = %v, want nil", doc, err)
		}
		if len(findings) != 0 || len(dropped) != 0 {
			t.Errorf("doc %s: findings = %v, dropped = %v, want none", doc, findings, dropped)
		}
	}
}

// TestParseGolangCIRejectsMalformedJSON pins G9: a report that is not even
// JSON is refused as UnreadableReportError whose Format is "golangci".
func TestParseGolangCIRejectsMalformedJSON(t *testing.T) {
	root := testRoot(t)

	_, _, err := ParseGolangCI(strings.NewReader("not json at all"), root)

	var unreadable UnreadableReportError
	if !errors.As(err, &unreadable) {
		t.Fatalf("ParseGolangCI() err = %v, want an UnreadableReportError", err)
	}
	if unreadable.Format != "golangci" {
		t.Errorf("Format = %q, want %q", unreadable.Format, "golangci")
	}
}

// TestParseGolangCIEmptyFilenameIsUnparsed pins G10: an issue naming no path
// at all is checked before placement and parses to one UNPARSED Finding with
// no Locations, and no Dropped entry. An empty filename does not describe
// another tree, which Outside: true would claim, and dropping it silently is
// exactly what UNPARSED exists to prevent.
func TestParseGolangCIEmptyFilenameIsUnparsed(t *testing.T) {
	root := testRoot(t)

	doc := `{"Issues":[{"FromLinter":"forbidigo","Text":"use of ` + "`fmt.Println`" + ` forbidden","Severity":"","Pos":{"Filename":"","Line":6,"Column":2}}]}`
	findings, dropped, err := ParseGolangCI(strings.NewReader(doc), root)
	if err != nil {
		t.Fatalf("ParseGolangCI() err = %v, want nil", err)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %v, want none", dropped)
	}
	if len(findings) != 1 || findings[0].Rule != "UNPARSED" || !findings[0].IgnoresScope {
		t.Fatalf("findings = %#v, want one UNPARSED, IgnoresScope true", findings)
	}
	if len(findings[0].Locations) != 0 {
		t.Errorf("Locations = %v, want none", findings[0].Locations)
	}
}

// TestParseGolangCITypecheckIgnoresScope pins G11: the real shape golangci
// 2.13.2 emits when a package does not compile, one issue from "typecheck" at
// Pos.Line 1, Pos.Column 0, and no LineRange. It parses to one ordinary
// Finding, Rule "typecheck", Message the Text verbatim, except IgnoresScope
// is true: a package that failed to compile was never actually analysed, so
// a clean scope result under it proves nothing, the same reasoning AD0001
// carries in the SARIF parser.
func TestParseGolangCITypecheckIgnoresScope(t *testing.T) {
	root := testRoot(t)
	writeSource(t, root, "bad.go")
	filename := jsonEscape(root.Abs(srcpath.Path("bad.go")))

	text := ": # gcltest\n./bad.go:1:28: syntax error: unexpected {, expected )"
	doc := `{"Issues":[{"FromLinter":"typecheck","Text":` + fmt.Sprintf("%q", text) + `,"Pos":{"Filename":"` + filename + `","Line":1,"Column":0}}]}`

	findings, dropped, err := ParseGolangCI(strings.NewReader(doc), root)

	if err != nil {
		t.Fatalf("ParseGolangCI() err = %v, want nil", err)
	}
	if len(dropped) != 0 {
		t.Errorf("dropped = %v, want none", dropped)
	}
	want := []Finding{
		{
			Rule:         "typecheck",
			Message:      text,
			Severity:     "warning",
			IgnoresScope: true,
			Locations:    []Location{{Path: srcpath.Path("bad.go"), StartLine: 1, StartColumn: 1, EndLine: 1}},
		},
	}
	if !reflect.DeepEqual(findings, want) {
		t.Errorf("findings = %#v, want %#v", findings, want)
	}
}

// jsonEscape escapes s for embedding inside a JSON string literal, since the
// absolute paths these fixtures build can carry a backslash on Windows.
func jsonEscape(s string) string {
	return strings.ReplaceAll(s, `\`, `\\`)
}
