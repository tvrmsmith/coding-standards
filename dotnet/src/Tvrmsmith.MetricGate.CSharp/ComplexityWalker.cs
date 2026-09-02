using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;

namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>
/// McCabe cyclomatic complexity for a single method's full syntax, base 1 plus one point per
/// decision point. The scored set, spelled out here so a future widening (issue 18) amends this
/// list deliberately rather than by omission:
///
///   if, while, do, for, foreach, a case label (default scores nothing), catch,
///   a when filter clause, &amp;&amp;, ||, ?:, ??
///
/// Nothing else scores. In particular ??= is not on this list and does not count, and a switch
/// expression arm is out of scope for this slice (spans come only from MethodDeclarationSyntax).
/// Walks the whole method node, so a nested lambda or local function's decision points fold into
/// the enclosing method's score, matching how spans exist only at the method-declaration level.
/// </summary>
internal sealed class ComplexityWalker : CSharpSyntaxWalker
{
    public int Complexity { get; private set; } = 1;

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
}
