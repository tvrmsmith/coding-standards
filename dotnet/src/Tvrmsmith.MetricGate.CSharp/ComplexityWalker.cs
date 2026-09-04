using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;

namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>
/// McCabe cyclomatic complexity for a single span's full syntax, base 1 plus one point per decision
/// point. Which constructs score, which do not, and why each is on the side it is on live in
/// <c>docs/csharp-decision-points.md</c>, along with every point where this list departs from
/// Roslyn's own; the overrides below are that document in code.
///
/// Walks the whole node it is handed, but stops at a nested declaration that carries a span of its
/// own: a local function's decision points score against the local function alone, never against
/// whatever declares it. A lambda or anonymous method is not such a declaration, so its decision
/// points do fold into whichever span holds it, which is what keeps a method from hiding its
/// branches in one. The root is the node <see cref="Score"/> was handed, so that
/// <see cref="VisitLocalFunctionStatement"/> tells a local function being scored in its own right
/// from one nested inside the span under measure.
/// </summary>
internal sealed class ComplexityWalker : CSharpSyntaxWalker
{
    private readonly SyntaxNode _root;

    private ComplexityWalker(SyntaxNode root)
    {
        _root = root;
    }

    private int Complexity { get; set; } = 1;

    /// <summary>
    /// Scores <paramref name="node"/>. The only entry point, so the root the walk starts from and
    /// the root <see cref="VisitLocalFunctionStatement"/> compares against are the same node by
    /// construction rather than by the caller remembering to pass it twice.
    /// </summary>
    public static int Score(SyntaxNode node)
    {
        var walker = new ComplexityWalker(node);
        walker.Visit(node);

        return walker.Complexity;
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

    /// <summary>
    /// A deconstructing <c>foreach (var (a, b) in pairs)</c> parses to its own node type rather than
    /// to <see cref="ForEachStatementSyntax"/>, and it is the same loop, so it scores the same point.
    /// </summary>
    public override void VisitForEachVariableStatement(ForEachVariableStatementSyntax node)
    {
        Complexity++;
        base.VisitForEachVariableStatement(node);
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
