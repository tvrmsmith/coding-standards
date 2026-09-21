package plugin

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"

	"github.com/tvrmsmith/coding-standards/go/plugin/commentblocklength"
)

// A three-line comment block: silent under the shipped budget, reported under a budget of one,
// which is the smallest value the fallback guard lets through.
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
		name       string
		settings   any
		wantBudget int
		want       []string
	}{
		{name: "omitted setting falls back to the default", settings: nil, wantBudget: commentblocklength.DefaultMax},
		{name: "zero falls back to the default", settings: map[string]any{"max": 0}, wantBudget: commentblocklength.DefaultMax},
		{name: "negative falls back to the default", settings: map[string]any{"max": -1}, wantBudget: commentblocklength.DefaultMax},
		{
			name:       "a positive setting is honoured",
			settings:   map[string]any{"max": 1},
			wantBudget: 1,
			want:       []string{"3-line comment block, over the 1-line budget"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			linter, err := newCommentBlockLength(test.settings)
			if err != nil {
				t.Fatalf("newCommentBlockLength(%v): %v", test.settings, err)
			}
			if budget := linter.(*commentBlockLength).budget; budget != test.wantBudget {
				t.Errorf("got budget %d, want %d", budget, test.wantBudget)
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

	const filename = "fixture.go"
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
