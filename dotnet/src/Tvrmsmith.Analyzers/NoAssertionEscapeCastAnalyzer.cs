using System.Collections.Immutable;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;
using Microsoft.CodeAnalysis.Diagnostics;

namespace Tvrmsmith.Analyzers;

/// <summary>
/// TVRM0003 <c>no-assertion-escape-cast</c> — never cast to <c>object</c> to reach the general
/// <c>ObjectAssertions</c> overload.
/// </summary>
/// <remarks>
/// <para>
/// Generalized from the Atlas worked example. When a framework ships its own <c>Should()</c>
/// extension for its wrapper type — returning a bespoke assertions type with no
/// <c>BeEquivalentTo</c> — <c>((object)body).Should().BeEquivalentTo(...)</c> is the tempting way
/// out. It discards the type the framework deliberately gave you and reports failures about an
/// <c>object</c>. The fix is to assert on the inner DTO.
/// </para>
/// <para>
/// Only the cast is mechanical, so only the cast is a rule. Which target to assert on instead is
/// judgment and stays review-only in the skill text.
/// </para>
/// </remarks>
[DiagnosticAnalyzer(LanguageNames.CSharp)]
public sealed class NoAssertionEscapeCastAnalyzer : DiagnosticAnalyzer
{
    /// <inheritdoc />
    public override ImmutableArray<DiagnosticDescriptor> SupportedDiagnostics { get; } =
        ImmutableArray.Create(Descriptors.NoAssertionEscapeCast);

    /// <inheritdoc />
    public override void Initialize(AnalysisContext context)
    {
        context.ConfigureGeneratedCodeAnalysis(GeneratedCodeAnalysisFlags.None);
        context.EnableConcurrentExecution();
        context.RegisterSyntaxNodeAction(AnalyzeInvocation, SyntaxKind.InvocationExpression);
    }

    private static void AnalyzeInvocation(SyntaxNodeAnalysisContext context)
    {
        var invocation = (InvocationExpressionSyntax)context.Node;
        if (!invocation.IsShouldInvocation(context.SemanticModel, context.CancellationToken))
        {
            return;
        }

        if (invocation.ShouldReceiver()?.Unparenthesize() is not { } receiver)
        {
            return;
        }

        // A cast to any other type still selects an overload, but it keeps a meaningful type in
        // the failure message. Only the reach for ObjectAssertions is the escape hatch.
        var (castedValue, castedType) = receiver switch
        {
            CastExpressionSyntax cast => ((ExpressionSyntax?)cast.Expression, (TypeSyntax?)cast.Type),
            BinaryExpressionSyntax binary when binary.IsKind(SyntaxKind.AsExpression) =>
                (binary.Left, binary.Right as TypeSyntax),
            _ => (null, null),
        };

        if (castedValue is null || castedType is null || !IsObjectType(context, castedType))
        {
            return;
        }

        context.ReportDiagnostic(Diagnostic.Create(
            Descriptors.NoAssertionEscapeCast,
            receiver.GetLocation(),
            castedValue.ToString()));
    }

    private static bool IsObjectType(SyntaxNodeAnalysisContext context, TypeSyntax type) =>
        context.SemanticModel.GetTypeInfo(type, context.CancellationToken).Type
            is { SpecialType: SpecialType.System_Object };
}
