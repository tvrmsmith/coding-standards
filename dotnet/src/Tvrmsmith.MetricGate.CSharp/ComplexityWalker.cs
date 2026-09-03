using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;

namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>
/// McCabe cyclomatic complexity for a single method's full syntax, base 1 plus one point per
/// decision point. The scored set, spelled out here so a future widening (issue 18) amends this
/// list deliberately rather than by omission:
///
///   if, while, do, for, foreach, a case label (default scores nothing), a switch expression arm,
///   catch, a when filter clause, a pattern combinator (and, or), &amp;&amp;, ||, ?:, ??
///
/// A pattern combinator scores for the reason &amp;&amp; and || do: each side is a test the method
/// can branch on, and `o is int or string` is the same control flow as `o is int || o is string`.
/// Both combinators count, so the pattern spelling of a condition never scores under the operator
/// spelling of it. A `not` pattern adds no branch and does not count.
///
/// An arm counts even when its pattern is the discard `_`, which is where this list parts company
/// with `default:`. An arm is always a pattern matched in turn, and the catch-all arm is written
/// as one; a `default:` label is not a pattern and scores nothing.
///
/// Nothing else scores. In particular ??= is not on this list and does not count. Walks the whole
/// node it is handed, but stops at a nested declaration that carries a span of its own: a local
/// function's decision points score against the local function alone, never against whatever
/// declares it. A lambda or anonymous method is not such a declaration, so its decision points do
/// fold into whichever span holds it, which is what keeps a method from hiding its branches in one.
/// </summary>
internal sealed class ComplexityWalker : CSharpSyntaxWalker
{
    private SyntaxNode? _root;

    public int Complexity { get; private set; } = 1;

    /// <summary>
    /// Records the node the walk started from, so <see cref="VisitLocalFunctionStatement"/> can
    /// tell "the local function this walker was asked to score" (the root, when a caller scores a
    /// local function's own span) from "a local function nested inside whatever this walker was
    /// asked to score" (never the root, always skipped).
    /// </summary>
    public override void Visit(SyntaxNode? node)
    {
        _root ??= node;
        base.Visit(node);
    }

    public override void VisitIfStatement(IfStatementSyntax node)
    {
        Complexity++;
        base.VisitIfStatement(node);
    }

    public override void VisitWhileStatement(WhileStatementSyntax node)
    {
        Complexity++;
        base.VisitWhileStatement(node);
    }

    public override void VisitDoStatement(DoStatementSyntax node)
    {
        Complexity++;
        base.VisitDoStatement(node);
    }

    public override void VisitForStatement(ForStatementSyntax node)
    {
        Complexity++;
        base.VisitForStatement(node);
    }

    public override void VisitForEachStatement(ForEachStatementSyntax node)
    {
        Complexity++;
        base.VisitForEachStatement(node);
    }

    public override void VisitCaseSwitchLabel(CaseSwitchLabelSyntax node)
    {
        Complexity++;
        base.VisitCaseSwitchLabel(node);
    }

    public override void VisitCasePatternSwitchLabel(CasePatternSwitchLabelSyntax node)
    {
        Complexity++;
        base.VisitCasePatternSwitchLabel(node);
    }

    public override void VisitSwitchExpressionArm(SwitchExpressionArmSyntax node)
    {
        Complexity++;
        base.VisitSwitchExpressionArm(node);
    }

    public override void VisitBinaryPattern(BinaryPatternSyntax node)
    {
        Complexity++;
        base.VisitBinaryPattern(node);
    }

    public override void VisitCatchClause(CatchClauseSyntax node)
    {
        Complexity++;
        base.VisitCatchClause(node);
    }

    public override void VisitCatchFilterClause(CatchFilterClauseSyntax node)
    {
        Complexity++;
        base.VisitCatchFilterClause(node);
    }

    public override void VisitWhenClause(WhenClauseSyntax node)
    {
        Complexity++;
        base.VisitWhenClause(node);
    }

    public override void VisitConditionalExpression(ConditionalExpressionSyntax node)
    {
        Complexity++;
        base.VisitConditionalExpression(node);
    }

    public override void VisitBinaryExpression(BinaryExpressionSyntax node)
    {
        if (node.IsKind(SyntaxKind.LogicalAndExpression)
            || node.IsKind(SyntaxKind.LogicalOrExpression)
            || node.IsKind(SyntaxKind.CoalesceExpression))
        {
            Complexity++;
        }

        base.VisitBinaryExpression(node);
    }

    /// <summary>
    /// A local function carries a span of its own, so its decision points must not fold into
    /// whatever span is currently being scored, unless this walker was asked to score the local
    /// function itself, in which case <paramref name="node"/> is the root and must be walked. Not
    /// calling <c>base</c> for any other local function stops the walk there rather than
    /// descending, which also keeps a local function nested inside another local function from
    /// folding into either of its ancestors.
    /// </summary>
    public override void VisitLocalFunctionStatement(LocalFunctionStatementSyntax node)
    {
        if (ReferenceEquals(node, _root))
        {
            base.VisitLocalFunctionStatement(node);
        }
    }
}
