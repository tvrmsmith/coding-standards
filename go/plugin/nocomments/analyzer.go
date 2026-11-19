// Package nocomments holds the Go half of the no-comments rule.
//
// Guideline "Comments" (coding-standards/SKILL.md): code documents itself, and what another
// agent needs to know is written as a test. The ESLint half is `tvrmsmith/no-comments` and the
// C# half is the Roslyn analyzer TVRM0008; all three report every comment except a doc comment,
// a directive or suppression a tool reads, and anything in a generated file.
package nocomments

import (
	"go/ast"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Message is the text every report carries. It is written for the coding agent that will read
// it, so it names the alternatives rather than only the violation.
const Message = "Remove this comment. Code should document itself: carry the meaning in a name, a type " +
	"or an extracted function. A comment is often there to justify a workaround, and the fix for a " +
	"workaround is the code. If another agent needs to know this, write a test that documents the " +
	"behavior instead. Doc comments, suppressions, tool directives and generated files are exempt."

// NewAnalyzer builds the analyzer. It takes no settings: there is nothing to tune, since a budget
// on comments would only be a way to keep some.
func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "nocomments",
		Doc:  "reports every comment that is not a doc comment, a tool directive or in a generated file",
		URL:  "https://github.com/tvrmsmith/coding-standards/blob/main/packages/eslint-plugin-tvrmsmith/docs/rules/no-comments.md",
		Run:  run,
	}
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		documentation := docGroups(file)
		for _, group := range file.Comments {
			if documentation[group] {
				continue
			}
			for _, comment := range group.List {
				if isDirective(comment.Text) {
					continue
				}
				pass.Report(analysis.Diagnostic{Pos: comment.Pos(), End: comment.End(), Message: Message})
			}
		}
	}
	return nil, nil
}

// docGroups collects the comment groups that are documentation.
//
// Go marks a doc comment structurally rather than with its own syntax: it is the group the parser
// attached to a declaration. So the exemption is read off the AST, and prose immediately above a
// `var`, `const` or `type` inside a function body counts as that declaration's documentation.
//
// The line comment trailing a struct field, a `const` or `var` spec or a type spec is exempt too,
// because godoc renders it as that declaration's documentation. The one trailing an import is not,
// because nothing renders it.
//
// The group above `import "C"` is the cgo preamble, which is C source rather than prose. It is the
// import's doc group, so the same rule keeps it.
func docGroups(file *ast.File) map[*ast.CommentGroup]bool {
	docs := map[*ast.CommentGroup]bool{}
	add := func(group *ast.CommentGroup) {
		if group != nil {
			docs[group] = true
		}
	}

	add(file.Doc)
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.FuncDecl:
			add(node.Doc)
		case *ast.GenDecl:
			add(node.Doc)
		case *ast.ImportSpec:
			add(node.Doc)
		case *ast.TypeSpec:
			add(node.Doc)
			add(node.Comment)
		case *ast.ValueSpec:
			add(node.Doc)
			add(node.Comment)
		case *ast.Field:
			add(node.Doc)
			add(node.Comment)
		}
		return true
	})
	return docs
}

// toolDirective is the form the Go toolchain reserves for comments a tool reads: `//` with no
// space, then a lowercase name, a colon and an argument, as in `//go:build`, `//go:generate`,
// `//nolint:errcheck` and staticcheck's `//lint:ignore`. It is the pattern go/ast uses itself.
var toolDirective = regexp.MustCompile(`^//[a-z0-9]+:[a-z0-9]`)

// isDirective reports whether a tool reads the comment, so deleting it would change what a tool
// does. A suppression's reason rides on the same comment (`//nolint:gosec // reason`), so it is
// kept with the suppression.
func isDirective(text string) bool {
	switch {
	case toolDirective.MatchString(text):
		return true
	case strings.HasPrefix(text, "//nolint"):
		return true
	case strings.HasPrefix(text, "//line "), strings.HasPrefix(text, "//export "), strings.HasPrefix(text, "//extern "):
		return true
	case strings.HasPrefix(text, "// +build "):
		return true
	}

	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(text, "//"), "/*"), "*/"))
	return strings.HasPrefix(body, "#nosec")
}
