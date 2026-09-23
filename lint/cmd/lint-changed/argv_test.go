package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/tvrmsmith/coding-standards/lint/internal/lintfind"
)

func TestParseFilterRequiresExactlyOneScope(t *testing.T) {
	_, err := Parse([]string{"--format", "sarif"})
	if err == nil {
		t.Fatal("got nil, want an error naming the missing scope")
	}

	_, err = Parse([]string{"--format", "sarif", "--staged", "--since", "main"})
	if err == nil {
		t.Fatal("got nil, want an error naming two scopes")
	}
}

func TestParseFilterStaged(t *testing.T) {
	cmd, err := Parse([]string{"--format", "sarif", "--staged"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Kind != KindFilter || cmd.Filter.Mode != ScopeStaged {
		t.Fatalf("got %+v, want a staged filter command", cmd)
	}
}

func TestParseFilterSince(t *testing.T) {
	cmd, err := Parse([]string{"--format", "sarif", "--since", "main"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Filter.Mode != ScopeSince || cmd.Filter.Ref != "main" {
		t.Fatalf("got %+v, want since main", cmd.Filter)
	}
}

func TestParseFilterFilesSplitsOnComma(t *testing.T) {
	cmd, err := Parse([]string{"--format", "sarif", "--files", "a.cs,b.cs"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"a.cs", "b.cs"}
	if len(cmd.Filter.Files) != len(want) || cmd.Filter.Files[0] != want[0] || cmd.Filter.Files[1] != want[1] {
		t.Fatalf("got %v, want %v", cmd.Filter.Files, want)
	}
}

func TestParseWaiveRequiresEveryField(t *testing.T) {
	cases := [][]string{
		{"waive", "--path", "p", "--rule", "r", "--reason", "why"},
		{"waive", "--language", "csharp", "--path", "p", "--reason", "why"},
		{"waive", "--language", "csharp", "--path", "p", "--rule", "r"},
	}
	for _, args := range cases {
		if _, err := Parse(args); err == nil {
			t.Errorf("Parse(%v): got nil error, want a usage error", args)
		}
	}
}

// --path is the one waive flag that may be left out. A diagnostic reported at
// Location.None has no path to name, so requiring one would leave an analyzer
// load failure with no route out at all.
func TestParseWaiveWithoutAPath(t *testing.T) {
	cmd, err := Parse([]string{"waive", "--language", "csharp", "--rule", "AD0001", "--reason", "the analyzer is broken upstream"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Waive.Path != "" {
		t.Fatalf("got path %q, want empty", cmd.Waive.Path)
	}
	if cmd.Waive.Rule != "AD0001" {
		t.Fatalf("got rule %q, want AD0001", cmd.Waive.Rule)
	}
}

func TestParseWaive(t *testing.T) {
	cmd, err := Parse([]string{"waive", "--language", "csharp", "--path", "Foo.cs", "--rule", "CA1822", "--reason", "known false positive"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Kind != KindWaive {
		t.Fatalf("got kind %v, want KindWaive", cmd.Kind)
	}
	want := WaiveArgs{Language: "csharp", Path: "Foo.cs", Rule: "CA1822", Reason: "known false positive"}
	if cmd.Waive != want {
		t.Fatalf("got %+v, want %+v", cmd.Waive, want)
	}
}

// A waiver keys on the language a finding carries, and findings only ever
// carry csharp, go or ts. Any other --language records a waiver that can
// never match, so waive refuses it instead.
func TestParseWaiveRejectsUnknownLanguage(t *testing.T) {
	_, err := Parse([]string{"waive", "--language", "typescript", "--path", "a.ts", "--rule", "no-console", "--reason", "why"})
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("got %v, want *UsageError", err)
	}
	want := "waive: unknown --language 'typescript', want one of csharp, go, ts"
	if ue.Problem != want {
		t.Fatalf("Problem = %q, want %q", ue.Problem, want)
	}
}

func TestParseWaiveAcceptsGoAndTS(t *testing.T) {
	for _, language := range []string{"go", "ts"} {
		cmd, err := Parse([]string{"waive", "--language", language, "--path", "a", "--rule", "r", "--reason", "why"})
		if err != nil {
			t.Fatalf("Parse --language %s: %v", language, err)
		}
		if cmd.Waive.Language != language {
			t.Fatalf("Language = %q, want %q", cmd.Waive.Language, language)
		}
	}
}

func TestParseWaivers(t *testing.T) {
	cmd, err := Parse([]string{"waivers"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Kind != KindWaivers {
		t.Fatalf("got kind %v, want KindWaivers", cmd.Kind)
	}
}

func TestParseWaiversRejectsArguments(t *testing.T) {
	if _, err := Parse([]string{"waivers", "extra"}); err == nil {
		t.Fatal("got nil, want a usage error")
	}
}

func TestParseUnknownFlag(t *testing.T) {
	if _, err := Parse([]string{"--format", "sarif", "--staged", "--bogus"}); err == nil {
		t.Fatal("got nil, want a usage error")
	}
}

// An unknown --format is refused at parse time, the same way a --report with
// no --format before it is, rather than reaching readReports and failing
// against whatever the first report happens to look like.
//
// Scenario 8.
func TestParseFilterRejectsUnknownFormat(t *testing.T) {
	_, err := Parse([]string{"--format", "bogus", "--staged", "--report", "a.json"})
	if err == nil {
		t.Fatalf("got nil, want an error")
	}
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("got %T, want *UsageError", err)
	}
	want := "unknown --format 'bogus', want one of sarif, golangci, eslint"
	if ue.Problem != want {
		t.Fatalf("Problem = %q, want %q", ue.Problem, want)
	}
}

// Scenario 5: --format is repeatable and scopes every --report that follows
// it, so a mixed run of two formats parses to two reports paired with their
// own parser.
func TestParseFilterEachFormatScopesItsOwnReports(t *testing.T) {
	cmd, err := Parse([]string{"--staged", "--format", "eslint", "--report", "a.json", "--format", "sarif", "--report", "b.sarif"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cmd.Filter.Reports) != 2 {
		t.Fatalf("got %d reports, want 2: %+v", len(cmd.Filter.Reports), cmd.Filter.Reports)
	}
	if cmd.Filter.Reports[0].Path != "a.json" || funcPointer(cmd.Filter.Reports[0].Parser) != funcPointer(lintfind.ParseESLint) {
		t.Errorf("Reports[0] = %+v, want a.json parsed as eslint", cmd.Filter.Reports[0])
	}
	if cmd.Filter.Reports[1].Path != "b.sarif" || funcPointer(cmd.Filter.Reports[1].Parser) != funcPointer(lintfind.ParseSARIF) {
		t.Errorf("Reports[1] = %+v, want b.sarif parsed as sarif", cmd.Filter.Reports[1])
	}
}

// Scenario 6: --format is sticky. Two --report flags after one --format both
// take that format's parser.
func TestParseFilterFormatIsSticky(t *testing.T) {
	cmd, err := Parse([]string{"--staged", "--format", "eslint", "--report", "a.json", "--report", "b.json"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cmd.Filter.Reports) != 2 {
		t.Fatalf("got %d reports, want 2: %+v", len(cmd.Filter.Reports), cmd.Filter.Reports)
	}
	for i, r := range cmd.Filter.Reports {
		if funcPointer(r.Parser) != funcPointer(lintfind.ParseESLint) {
			t.Errorf("Reports[%d].Parser is not ParseESLint", i)
		}
	}
}

// Scenario 7: a --report with no --format before it is a usage error, since
// a report no longer has to exist and "--format is required" no longer
// applies to a run with zero reports.
func TestParseFilterReportWithNoFormatIsUsageError(t *testing.T) {
	_, err := Parse([]string{"--staged", "--report", "a.json"})
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("got %T (%v), want *UsageError", err, err)
	}
	want := "--report a.json has no --format before it"
	if ue.Problem != want {
		t.Fatalf("Problem = %q, want %q", ue.Problem, want)
	}
}

// Scenario 9: --language is no longer a filter flag. The parser supplies the
// language now, so asserting it from the command line is gone.
func TestParseFilterLanguageIsUnknownArgument(t *testing.T) {
	_, err := Parse([]string{"--staged", "--language", "csharp"})
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("got %T (%v), want *UsageError", err, err)
	}
	want := "unknown argument '--language'"
	if ue.Problem != want {
		t.Fatalf("Problem = %q, want %q", ue.Problem, want)
	}
}

// Scenario 10: --staged alone parses cleanly to zero reports. A report no
// longer has to exist for a run to be legal.
func TestParseFilterStagedAloneIsZeroReports(t *testing.T) {
	cmd, err := Parse([]string{"--staged"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Kind != KindFilter || cmd.Filter.Mode != ScopeStaged {
		t.Fatalf("got %+v, want a staged filter command", cmd)
	}
	if len(cmd.Filter.Reports) != 0 {
		t.Fatalf("got %d reports, want 0", len(cmd.Filter.Reports))
	}
}

// Scenario 11: a bare --format with no --report after it parses cleanly too.
// It legally means "this branch ran and found nothing".
func TestParseFilterFormatWithNoReportIsZeroReports(t *testing.T) {
	cmd, err := Parse([]string{"--staged", "--format", "eslint"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(cmd.Filter.Reports) != 0 {
		t.Fatalf("got %d reports, want 0", len(cmd.Filter.Reports))
	}
}

// Scenario 12: spend parses its repeated --waiver flags in order.
func TestParseSpend(t *testing.T) {
	cmd, err := Parse([]string{"spend", "--waiver", "abc", "--waiver", "def"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Kind != KindSpend {
		t.Fatalf("got kind %v, want KindSpend", cmd.Kind)
	}
	want := []string{"abc", "def"}
	if !reflect.DeepEqual(cmd.Spend.IDs, want) {
		t.Fatalf("got %v, want %v", cmd.Spend.IDs, want)
	}
}

// Scenario 13: spend with no --waiver at all is a usage error.
func TestParseSpendRequiresAtLeastOneWaiver(t *testing.T) {
	_, err := Parse([]string{"spend"})
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("got %T (%v), want *UsageError", err, err)
	}
	want := "spend: at least one --waiver is required"
	if ue.Problem != want {
		t.Fatalf("Problem = %q, want %q", ue.Problem, want)
	}
}

// Scenario 14: waive is unchanged by any of this.
func TestParseWaiveUnchanged(t *testing.T) {
	cmd, err := Parse([]string{"waive", "--language", "csharp", "--rule", "CS0219", "--reason", "why"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := WaiveArgs{Language: "csharp", Rule: "CS0219", Reason: "why"}
	if cmd.Kind != KindWaive || cmd.Waive != want {
		t.Fatalf("got %+v, want KindWaive %+v", cmd, want)
	}
}

// funcPointer compares two lintfind.Parser values by function identity: a
// non-nil check would pass just as well if --format golangci resolved to the
// SARIF parser, and reading golangci JSON with the wrong parser is the one
// mistake parse-time resolution exists to prevent.
func funcPointer(p lintfind.Parser) uintptr {
	return reflect.ValueOf(p).Pointer()
}
