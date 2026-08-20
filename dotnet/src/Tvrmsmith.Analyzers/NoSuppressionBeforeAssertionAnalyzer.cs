using System.Collections.Immutable;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;
using Microsoft.CodeAnalysis.Diagnostics;

namespace Tvrmsmith.Analyzers;

/// <summary>
/// TVRM0002 <c>no-suppression-before-assertion</c> — no <c>!</c> and no <c>?.</c> in the receiver
/// chain feeding <c>.Should()</c>.
/// </summary>
/// <remarks>
/// <para>
/// <c>!</c> turns the null the assertion exists to catch into a NullReferenceException carrying
/// no expectation; <c>?.</c> is worse, because the whole chain short-circuits and the test passes.
/// </para>
/// <para>
/// Scoped to the receiver chain, not the file. <c>Client.BaseAddress!</c> in test setup, or inside
/// the expected value handed to <c>Be(...)</c>, is a precondition rather than the value under test
/// and is deliberately left alone — the reference calls that out explicitly.
/// </para>
/// </remarks>
[DiagnosticAnalyzer(LanguageNames.CSharp)]
public sealed class NoSuppressionBeforeAssertionAnalyzer : DiagnosticAnalyzer
{
    /// <inheritdoc />
    public override ImmutableArray<DiagnosticDescriptor> SupportedDiagnostics { get; } =
        ImmutableArray.Create(Descriptors.NoSuppressionBeforeAssertion);

    /// <inheritdoc />
    public override void Initialize(AnalysisContext context)
    {
        context.ConfigureGeneratedCodeAnalysis(GeneratedCodeAnalysisFlags.None);
        context.EnableConcurrentExecution();
        context.RegisterSyntaxNodeAction(AnalyzeInvocation, SyntaxKind.InvocationExpression);
        context.RegisterSyntaxNodeAction(AnalyzeConditionalAccess, SyntaxKind.ConditionalAccessExpression);
    }

    /// <summary>Catches <c>value!.Member.Should()</c>.</summary>
    private static void AnalyzeInvocation(SyntaxNodeAnalysisContext context)
    {
        var invocation = (InvocationExpressionSyntax)context.Node;
        if (!invocation.IsShouldInvocation(context.SemanticModel, context.CancellationToken))
        {
            return;
        }

        foreach (var node in AssertionSyntax.WalkReceiverSpine(invocation.ShouldReceiver()))
        {
            // A chained assertion (`.ContainSingle().Which.Should()`) sits on top of an earlier
            // Should(); anything below that belongs to the earlier one and is its business.
            if (node is InvocationExpressionSyntax inner
                && inner.IsShouldInvocation(context.SemanticModel, context.CancellationToken))
            {
                return;
            }

            if (node is PostfixUnaryExpressionSyntax suppression
                && suppression.IsKind(SyntaxKind.SuppressNullableWarningExpression))
            {
                context.ReportDiagnostic(Diagnostic.Create(
                    Descriptors.NoSuppressionBeforeAssertion,
                    suppression.GetLocation(),
                    suppression.ToString()));
            }
        }
    }

    /// <summary>Catches <c>value?.Member.Should()</c>, where the call is a member binding.</summary>
    private static void AnalyzeConditionalAccess(SyntaxNodeAnalysisContext context)
    {
        var conditionalAccess = (ConditionalAccessExpressionSyntax)context.Node;

        // The `?.` only matters if an assertion actually hangs off it. Walking the continuation's
        // spine — rather than searching it — is what keeps `x.Should().Be(y?.Z)` out of scope.
        var should = AssertionSyntax.RootShouldInvocation(
            conditionalAccess.WhenNotNull, context.SemanticModel, context.CancellationToken);

        if (should is null)
        {
            return;
        }

        context.ReportDiagnostic(Diagnostic.Create(
            Descriptors.NoSuppressionBeforeAssertion,
            conditionalAccess.OperatorToken.GetLocation(),
            // The token itself is just '?' — the '.' belongs to the member binding that follows.
            // The message names the operator the way it is written.
            "?."));
    }
}
