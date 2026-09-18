package main

import (
	"strings"
	"testing"
)

func TestParseFilterRequiresFormat(t *testing.T) {
	_, err := Parse([]string{"--language", "csharp", "--staged"})
	if err == nil || !strings.Contains(err.Error(), "--format") {
		t.Fatalf("got %v, want an error naming --format", err)
	}
}

func TestParseFilterRequiresLanguage(t *testing.T) {
	_, err := Parse([]string{"--format", "sarif", "--staged"})
	if err == nil || !strings.Contains(err.Error(), "--language") {
		t.Fatalf("got %v, want an error naming --language", err)
	}
}

func TestParseFilterRequiresExactlyOneScope(t *testing.T) {
	_, err := Parse([]string{"--format", "sarif", "--language", "csharp"})
	if err == nil {
		t.Fatal("got nil, want an error naming the missing scope")
	}

	_, err = Parse([]string{"--format", "sarif", "--language", "csharp", "--staged", "--since", "main"})
	if err == nil {
		t.Fatal("got nil, want an error naming two scopes")
	}
}

func TestParseFilterStaged(t *testing.T) {
	cmd, err := Parse([]string{"--format", "sarif", "--language", "csharp", "--staged"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Kind != KindFilter || cmd.Filter.Mode != ScopeStaged {
		t.Fatalf("got %+v, want a staged filter command", cmd)
	}
}

func TestParseFilterSince(t *testing.T) {
	cmd, err := Parse([]string{"--format", "sarif", "--language", "csharp", "--since", "main"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cmd.Filter.Mode != ScopeSince || cmd.Filter.Ref != "main" {
		t.Fatalf("got %+v, want since main", cmd.Filter)
	}
}

func TestParseFilterFilesSplitsOnComma(t *testing.T) {
	cmd, err := Parse([]string{"--format", "sarif", "--language", "csharp", "--files", "a.cs,b.cs"})
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

// --report is repeatable, so one process reads every report a build wrote and
// makes one waiver-spend decision across all of them.
func TestParseFilterCollectsEveryReport(t *testing.T) {
	cmd, err := Parse([]string{"--format", "sarif", "--language", "csharp", "--staged",
		"--report", "one.sarif", "--report", "two.sarif"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"one.sarif", "two.sarif"}
	if len(cmd.Filter.Reports) != len(want) || cmd.Filter.Reports[0] != want[0] || cmd.Filter.Reports[1] != want[1] {
		t.Fatalf("got %v, want %v", cmd.Filter.Reports, want)
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
	if _, err := Parse([]string{"--format", "sarif", "--language", "csharp", "--staged", "--bogus"}); err == nil {
		t.Fatal("got nil, want a usage error")
	}
}
