package plugin

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// A three-line comment block: silent under the shipped 10-line budget, reported under a budget
// of two. Which of the two a setting produces is the whole observable difference between the
// default and a honoured value.
const threeLineBlock = `package fixture

func f() {
	// one
	// two
	// three
	g()
}
`

func TestCommentBlockLengthBudget(t *testing.T) {
	for _, test := range []struct {
		name     string
		settings any
		want     []string
	}{
		{name: "omitted setting falls back to the default", settings: map[string]any{}},
		{name: "zero falls back to the default", settings: map[string]any{"max": 0}},
		{name: "negative falls back to the default", settings: map[string]any{"max": -1}},
		{
			name:     "a positive setting is honoured",
			settings: map[string]any{"max": 2},
			want:     []string{"3-line comment block, over the 2-line budget"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			linter, err := newCommentBlockLength(test.settings)
			if err != nil {
				t.Fatalf("newCommentBlockLength(%v): %v", test.settings, err)
			}
			analyzers, err := linter.BuildAnalyzers()
			if err != nil {
				t.Fatalf("BuildAnalyzers: %v", err)
			}
			if len(analyzers) != 1 {
				t.Fatalf("got %d analyzers, want 1", len(analyzers))
			}

			got := reportsOver(t, analyzers[0], threeLineBlock)
			if len(got) != len(test.want) {
				t.Fatalf("got %d diagnostics %q, want %d %q", len(got), got, len(test.want), test.want)
			}
			for i, want := range test.want {
				if !strings.Contains(got[i], want) {
					t.Errorf("diagnostic %d is %q, want it to contain %q", i, got[i], want)
				}
			}
		})
	}
}

// reportsOver runs the analyzer over one source file and returns the messages it reported.
func reportsOver(t *testing.T, analyzer *analysis.Analyzer, src string) []string {
	t.Helper()

	filename := filepath.Join(t.TempDir(), "fixture.go")
	if err := os.WriteFile(filename, []byte(src), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}

	var messages []string
	pass := &analysis.Pass{
		Analyzer: analyzer,
		Fset:     fset,
		Files:    []*ast.File{file},
		ReadFile: func(name string) ([]byte, error) {
			if name != filename {
				return nil, fmt.Errorf("unexpected read of %s", name)
			}
			return []byte(src), nil
		},
		Report: func(d analysis.Diagnostic) { messages = append(messages, d.Message) },
	}
	if _, err := analyzer.Run(pass); err != nil {
		t.Fatalf("running the analyzer: %v", err)
	}
	return messages
}
