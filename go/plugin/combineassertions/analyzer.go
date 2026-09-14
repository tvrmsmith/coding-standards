// Package combineassertions holds the Go half of combine-assertions-on-same-object.
//
// Guideline A1, "Combine Assertions on the Same Object" (test-best-practices/SKILL.md): back-to-
// back assertions picking apart one object are wasteful and fragile, because the first failure
// hides every later one. The ESLint half is `tvrmsmith/combine-assertions-on-same-object` and
// the C# half is the Roslyn analyzer TVRM0001. A1 is the one guideline with no off-the-shelf
// rule in any of the three languages.
//
// Two shapes, the same two the Roslyn half takes:
//
//   - consecutive assertions whose value under test is a field of one object, and
//   - a length assertion on a collection followed by indexing into it.
//
// Both warn rather than error: the rewrite is a restructure, and which expected value to
// compare against is the author's call.
package combineassertions

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

func NewAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "combineassertions",
		Doc:  "reports consecutive testify assertions that pick one object apart",
		URL:  "https://github.com/tvrmsmith/coding-standards/blob/main/go/README.md",
		Run:  run,
	}
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			block, ok := node.(*ast.BlockStmt)
			if !ok {
				return true
			}
			reportPickedApart(pass, block)
			reportCountThenIndex(pass, block)
			return true
		})
	}
	return nil, nil
}

// reportPickedApart walks the block for runs of consecutive assertions whose subjects are
// fields of one object.
//
// Any other statement ends a run. That is deliberately stricter than the C# half, which groups
// across an intervening statement: a run broken by real work is two arrangements, and calling
// it one would propose a rewrite that changes what the test does.
func reportPickedApart(pass *analysis.Pass, block *ast.BlockStmt) {
	var run []subject

	flush := func() {
		if len(run) >= 2 {
			pass.Report(analysis.Diagnostic{
				Pos: run[0].call.Pos(),
				End: run[len(run)-1].call.End(),
				Message: fmt.Sprintf(
					"%d assertions in a row pick apart %q. One assertion against a whole expected "+
						"value checks them together and reports every difference, where this stops "+
						"at the first that fails.",
					len(run), run[0].object.Name()),
			})
		}
		run = nil
	}

	for _, stmt := range block.List {
		found, ok := subjectOf(pass, stmt)
		if !ok {
			flush()
			continue
		}
		if len(run) > 0 && run[0].object != found.object {
			flush()
		}
		run = append(run, found)
	}
	flush()
}

// reportCountThenIndex walks the block for a length assertion on a collection followed by an
// assertion that indexes into the same collection.
//
// Unlike the run above this looks ahead through the whole block rather than at the next
// statement, because the shape survives statements in between: the length is asserted, some
// work happens, and an element is picked out later. One equivalence assertion against the
// expected collection covers both, whatever sits between them.
func reportCountThenIndex(pass *analysis.Pass, block *ast.BlockStmt) {
	for i, stmt := range block.List {
		call, name, args, ok := assertionCall(pass, stmt)
		if !ok || name != "Len" || len(args) == 0 {
			continue
		}
		ident, ok := args[0].(*ast.Ident)
		if !ok {
			continue
		}
		collection := pass.TypesInfo.ObjectOf(ident)
		if collection == nil || !indexedLater(pass, block.List[i+1:], collection) {
			continue
		}
		pass.Report(analysis.Diagnostic{
			Pos: call.Pos(),
			End: call.End(),
			Message: fmt.Sprintf(
				"the length of %q is asserted and then an element of it is indexed out. One "+
					"assertion against the expected collection checks the length and the elements "+
					"together.", collection.Name()),
		})
	}
}

// indexedLater reports whether any later assertion in the block indexes into the collection.
func indexedLater(pass *analysis.Pass, rest []ast.Stmt, collection types.Object) bool {
	for _, stmt := range rest {
		call, _, _, ok := assertionCall(pass, stmt)
		if !ok {
			continue
		}
		indexed := false
		ast.Inspect(call, func(node ast.Node) bool {
			index, ok := node.(*ast.IndexExpr)
			if !ok {
				return true
			}
			if ident, ok := index.X.(*ast.Ident); ok && pass.TypesInfo.ObjectOf(ident) == collection {
				indexed = true
			}
			return true
		})
		if indexed {
			return true
		}
	}
	return false
}

// A subject is one assertion's value under test, reduced to the object it reaches into.
type subject struct {
	call   *ast.CallExpr
	object types.Object
}

// subjectOf reports the object an assertion statement reaches into, and whether the statement
// is an assertion that reaches into one at all.
//
// The root is resolved to a types.Object rather than compared by name, so two assertions on the
// same spelling in different scopes are not one group.
func subjectOf(pass *analysis.Pass, stmt ast.Stmt) (subject, bool) {
	call, name, args, ok := assertionCall(pass, stmt)
	if !ok {
		return subject{}, false
	}

	index := subjectIndex(name)
	if index >= len(args) {
		return subject{}, false
	}

	root, ok := fieldRoot(args[index])
	if !ok {
		return subject{}, false
	}
	object := pass.TypesInfo.ObjectOf(root)
	if object == nil {
		return subject{}, false
	}
	if _, isPackage := object.(*types.PkgName); isPackage {
		return subject{}, false // `config.Timeout` is a package member, not an object picked apart
	}
	return subject{call: call, object: object}, true
}

// fieldRoot walks a field selection back to the identifier it starts from.
//
// Three shapes are rejected, and each rejection is what keeps the rule honest. A bare
// identifier is not picking an object apart. A chain containing a call is not one object
// either: `load().Page` and `load().Age` are two calls, the same reason the C# half opens no
// group on a receiver holding one. And an index makes the subject an element rather than a
// field, which the count-then-index half owns.
func fieldRoot(expr ast.Expr) (*ast.Ident, bool) {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}

	for {
		switch inner := selector.X.(type) {
		case *ast.Ident:
			return inner, true
		case *ast.SelectorExpr:
			selector = inner
		default:
			return nil, false
		}
	}
}

// subjectIndex is the argument holding the value under test, once the TestingT is dropped.
//
// testify's expected-first family is the exception and everything else puts the subject first.
// Getting this wrong on a function not listed here costs a missed report rather than a false
// one, because a report also needs the neighbouring assertion to reach into the same object.
func subjectIndex(name string) int {
	switch name {
	case "Equal", "NotEqual", "EqualValues", "NotEqualValues", "Exactly",
		"InDelta", "InEpsilon", "JSONEq", "YAMLEq", "Same", "NotSame", "IsType":
		return 1
	default:
		return 0
	}
}

// assertionCall recognises a testify assertion statement and strips the TestingT argument.
//
// Recognition is by the callee's package, read from the type checker, rather than by a
// `assert.` prefix in the source. That is what covers all three spellings at once: the package
// functions, the `*assert.Assertions` methods, and a suite's own `s.Equal`, which is the same
// method promoted through an embedded `*assert.Assertions`.
func assertionCall(pass *analysis.Pass, stmt ast.Stmt) (*ast.CallExpr, string, []ast.Expr, bool) {
	expr, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return nil, "", nil, false
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok {
		return nil, "", nil, false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, "", nil, false
	}
	fn, ok := pass.TypesInfo.Uses[selector.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || !isTestify(fn.Pkg().Path()) {
		return nil, "", nil, false
	}

	args := call.Args
	if signature, ok := fn.Type().(*types.Signature); ok && signature.Recv() == nil {
		if len(args) == 0 {
			return nil, "", nil, false
		}
		args = args[1:] // the package functions take the TestingT the methods already hold
	}
	return call, fn.Name(), args, true
}

func isTestify(path string) bool {
	return path == "github.com/stretchr/testify/assert" ||
		path == "github.com/stretchr/testify/require"
}
