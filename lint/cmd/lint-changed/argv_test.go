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
		{"waive", "--language", "csharp", "--rule", "r", "--reason", "why"},
		{"waive", "--language", "csharp", "--path", "p", "--reason", "why"},
		{"waive", "--language", "csharp", "--path", "p", "--rule", "r"},
	}
	for _, args := range cases {
		if _, err := Parse(args); err == nil {
			t.Errorf("Parse(%v): got nil error, want a usage error", args)
		}
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
