using System.Collections.Immutable;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;
using Microsoft.CodeAnalysis.Diagnostics;

namespace Tvrmsmith.Analyzers;

/// <summary>
/// TVRM0004 <c>no-assertion-without-matcher</c> — <c>x.Should();</c> as a whole statement.
/// </summary>
/// <remarks>
/// <para>
/// One of the three shapes under <em>Assertions Must Actually Execute</em>. The call compiles,
/// builds an assertions object, and throws it away. The test is green and checked nothing, which
/// is worse than having no assertion at all: a missing assertion is visible in review.
/// </para>
/// <para>
/// Nothing off the shelf sees this. CA1806 was measured against it and stays silent even when
/// explicitly enabled, because it knows a fixed list of pure framework methods and
/// <c>Should()</c> is not on it.
/// </para>
/// </remarks>
[DiagnosticAnalyzer(LanguageNames.CSharp)]
public sealed class NoAssertionWithoutMatcherAnalyzer : DiagnosticAnalyzer
{
    /// <inheritdoc />
    public override ImmutableArray<DiagnosticDescriptor> SupportedDiagnostics { get; } =
        ImmutableArray.Create(Descriptors.NoAssertionWithoutMatcher);

    /// <inheritdoc />
    public override void Initialize(AnalysisContext context)
    {
        context.ConfigureGeneratedCodeAnalysis(GeneratedCodeAnalysisFlags.None);
        context.EnableConcurrentExecution();
        context.RegisterSyntaxNodeAction(AnalyzeStatement, SyntaxKind.ExpressionStatement);
    }

    private static void AnalyzeStatement(SyntaxNodeAnalysisContext context)
    {
        var statement = (ExpressionStatementSyntax)context.Node;

        // The statement's own expression has to *be* the Should() call. A matcher after it makes
        // the statement an invocation of the matcher instead, which is the whole distinction.
        if (statement.Expression is not InvocationExpressionSyntax invocation)
        {
            return;
        }

        if (!invocation.IsShouldInvocation(context.SemanticModel, context.CancellationToken))
        {
            return;
        }

        var receiver = invocation.ShouldReceiver()?.Unparenthesize();

        context.ReportDiagnostic(Diagnostic.Create(
            Descriptors.NoAssertionWithoutMatcher,
            statement.GetLocation(),
            receiver?.ToString() ?? "value"));
    }
}
