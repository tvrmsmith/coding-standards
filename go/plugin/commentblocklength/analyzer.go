// Package commentblocklength holds the Go half of the comment-block-length rule.
//
// Guideline "Comments" (coding-standards/SKILL.md) — "a paragraph justifying a workaround means
// the code is wrong". The ESLint half is `tvrmsmith/comment-block-length` and the C# half is the
// Roslyn analyzer TVRM0006; all three carry the same 10-line budget and the same doc-comment
// exemption.
//
// The skill states the judgement is whether the comment is *justifying* something, not how long
// it is, and no lint rule can read intent. Length is the one mechanical proxy available, and it
// is a weak one: a long comment is a prompt to re-read the code, not a verdict on it.
package commentblocklength

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// DefaultMax is the budget the preset ships: a block spanning more lines than this is reported.
const DefaultMax = 10

// NewAnalyzer builds the analyzer with a caller-supplied budget, because golangci-lint hands
// plugin settings to the plugin rather than to the analyzer's own flag set.
func NewAnalyzer(budget int) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "commentblocklength",
		Doc:  "reports a run of non-documentation comments longer than the budget",
		URL:  "https://github.com/tvrmsmith/coding-standards/blob/main/packages/eslint-plugin-tvrmsmith/docs/rules/comment-block-length.md",
		Run: func(pass *analysis.Pass) (any, error) {
			return run(pass, budget)
		},
	}
}

func run(pass *analysis.Pass, budget int) (any, error) {
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		src, err := pass.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("reading %s to find the start of each comment's line: %w", filename, err)
		}

		for _, block := range blocks(file, pass.Fset, src) {
			lines := block.lines(pass.Fset)
			if lines <= budget {
				continue
			}
			pass.Report(analysis.Diagnostic{
				Pos: block.first.Pos(),
				End: block.last.End(),
				Message: fmt.Sprintf(
					"%d-line comment block, over the %d-line budget. Is it justifying the code below it? "+
						"Then fix the code. Is it documenting a contract or an invariant? Then move it above "+
						"the declaration it documents, where a doc comment is exempt.",
					lines, budget),
			})
		}
	}
	return nil, nil
}

// A block is a maximal run of own-line, non-documentation comments on consecutive lines,
// measured from the first comment's opening line to the last comment's closing line.
type block struct {
	first, last *ast.Comment
}

func (b block) lines(fset *token.FileSet) int {
	return fset.Position(b.last.End()).Line - fset.Position(b.first.Pos()).Line + 1
}

// blocks returns the file's comment blocks in source order.
//
// Four things end a block: a blank line, a line of code, a trailing comment (`x++ // why`, which
// annotates the code on its line rather than standing on its own), and a doc comment. The parser
// already breaks a group on the first two, so only the last two are decided here.
func blocks(file *ast.File, fset *token.FileSet, src []byte) []block {
	exempt := exemptGroups(file)

	var found []block
	open := -1 // index in found of the block still being extended, or -1 for none

	for _, group := range file.Comments {
		if exempt[group] {
			open = -1
			continue
		}
		for _, comment := range group.List {
			if !startsItsLine(fset, src, comment) {
				open = -1
				continue
			}
			if open >= 0 && line(fset, comment.Pos()) == line(fset, found[open].last.End())+1 {
				found[open].last = comment
				continue
			}
			found = append(found, block{first: comment, last: comment})
			open = len(found) - 1
		}
	}

	return found
}

// exemptGroups collects the comment groups the rule never measures.
//
// A doc comment is exempt at any length: the same skill section requires documenting public API
// contracts, invariants, units and side effects, so a rule that punished a thorough doc comment
// would contradict the standard it enforces.
//
// Go marks a doc comment structurally rather than with its own syntax — it is the group the
// parser attached to a declaration — so the exemption is read off the AST rather than off the
// comment text. Two consequences differ from the ESLint and Roslyn halves. Prose sitting
// immediately above a `var`, `const` or `type` inside a function body is that declaration's doc
// comment and is exempt, where in the other two languages it would be measured. And the
// "a doc comment ends the run beside it" case those halves need cannot arise here: contiguous
// comment lines above a declaration are one group, which the parser attaches whole.
//
// Anything above the `package` clause is exempt too. That region holds licence headers and
// `//go:build` constraints, neither of which justifies code, and only the group the parser
// attached as the package doc would otherwise escape.
func exemptGroups(file *ast.File) map[*ast.CommentGroup]bool {
	exempt := map[*ast.CommentGroup]bool{}
	add := func(group *ast.CommentGroup) {
		if group != nil {
			exempt[group] = true
		}
	}

	for _, group := range file.Comments {
		if group.End() < file.Package {
			add(group)
		}
	}

	add(file.Doc)
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.FuncDecl:
			add(node.Doc)
		case *ast.GenDecl:
			add(node.Doc)
		case *ast.TypeSpec:
			add(node.Doc)
		case *ast.ValueSpec:
			add(node.Doc)
		case *ast.ImportSpec:
			add(node.Doc)
		case *ast.Field:
			add(node.Doc)
		}
		return true
	})
	return exempt
}

// startsItsLine reports whether nothing but whitespace precedes the comment on its opening line.
func startsItsLine(fset *token.FileSet, src []byte, comment *ast.Comment) bool {
	pos := fset.Position(comment.Pos())
	lineStart := pos.Offset - (pos.Column - 1)
	return strings.TrimSpace(string(src[lineStart:pos.Offset])) == ""
}

func line(fset *token.FileSet, pos token.Pos) int {
	return fset.Position(pos).Line
}
