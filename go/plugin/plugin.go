// Package plugin registers the personal Go lint rules with golangci-lint.
//
// One `register.Plugin` call per rule, rather than one plugin holding every analyzer: the
// registered name *is* the linter name in `.golangci.yml`, so a rule per name is what lets a
// consumer enable, disable and configure them one at a time.
//
// golangci-lint imports this package for its side effects when it builds the custom binary; see
// ../.custom-gcl.yml and ../README.md.
package plugin

import (
	"fmt"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/tvrmsmith/coding-standards/go/plugin/combineassertions"
	"github.com/tvrmsmith/coding-standards/go/plugin/commentblocklength"
)

func init() {
	register.Plugin("tvrmsmith-comment-block-length", newCommentBlockLength)
	register.Plugin("tvrmsmith-combine-assertions", newCombineAssertions)
}

type commentBlockLengthSettings struct {
	// Max is the budget: a block spanning more lines than this is reported. Anything
	// non-positive means unset, which is how an omitted setting arrives, so it reads as the
	// default rather than as a budget every comment block busts.
	Max int `json:"max"`
}

type commentBlockLength struct {
	budget int
}

var _ register.LinterPlugin = (*commentBlockLength)(nil)

func newCommentBlockLength(input any) (register.LinterPlugin, error) {
	settings, err := register.DecodeSettings[commentBlockLengthSettings](input)
	if err != nil {
		return nil, fmt.Errorf("decoding tvrmsmith-comment-block-length settings: %w", err)
	}

	budget := settings.Max
	if budget <= 0 {
		budget = commentblocklength.DefaultMax
	}
	return &commentBlockLength{budget: budget}, nil
}

func (c *commentBlockLength) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{commentblocklength.NewAnalyzer(c.budget)}, nil
}

// LoadModeSyntax: the rule reads comments and declaration positions, never a type, so
// golangci-lint can skip type-checking the packages it runs over.
func (c *commentBlockLength) GetLoadMode() string {
	return register.LoadModeSyntax
}

type combineAssertions struct{}

var _ register.LinterPlugin = (*combineAssertions)(nil)

// newCombineAssertions takes no settings. The rule has nothing to tune: two consecutive
// assertions are the shape, and a threshold on it would only be a way to keep the shape.
func newCombineAssertions(any) (register.LinterPlugin, error) {
	return &combineAssertions{}, nil
}

func (c *combineAssertions) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{combineassertions.NewAnalyzer()}, nil
}

// LoadModeTypesInfo: the rule recognises an assertion by the callee's package rather than by an
// `assert.` prefix in the source, which is what covers the package functions, the
// `*assert.Assertions` methods and a suite's promoted `s.Equal` with one check. That answer only
// exists in the type checker's output.
func (c *combineAssertions) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
