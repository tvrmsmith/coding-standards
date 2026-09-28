using System.Collections.Immutable;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;
using Microsoft.CodeAnalysis.Diagnostics;

namespace Tvrmsmith.Analyzers;

/// <summary>
/// TVRM0007 <c>no-anonymous-object-comparison</c>. An assertion whose subject and expectation
/// are both anonymous objects.
/// </summary>
/// <remarks>
/// <para>
/// <c>new { a.X, B = b.Y }.Should().BeEquivalentTo(new { X = 1, B = true })</c> bundles values
/// that share no object into a throwaway shape so one call asserts them all. A failure reports a
/// structural diff of that shape rather than which value broke, and the expected side grows casts
/// to match member types. One <c>Should()</c> per value inside an <c>AssertionScope</c> still
/// reports every failure at once.
/// </para>
/// <para>
/// The subject is what makes it a bundle, so both sides have to be anonymous-object creations. A
/// real object against an anonymous expectation is the shape <c>TVRM0001</c> steers toward, and
/// a projection such as <c>items.Select(i =&gt; new { i.Name })</c> has a subject of its own. The
/// matcher is not named: anonymous types override <c>Equals</c>, so <c>Be</c> compares the bundle
/// as surely as <c>BeEquivalentTo</c> does.
/// </para>
/// </remarks>
[DiagnosticAnalyzer(LanguageNames.CSharp)]
public sealed class NoAnonymousObjectComparisonAnalyzer : DiagnosticAnalyzer
{
    /// <inheritdoc />
    public override ImmutableArray<DiagnosticDescriptor> SupportedDiagnostics { get; } =
        ImmutableArray.Create(Descriptors.NoAnonymousObjectComparison);

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

        // Both syntactic tests run before IsShouldInvocation asks the semantic model.
        if (invocation.ShouldReceiver()?.Unparenthesize() is not AnonymousObjectCreationExpressionSyntax subject
            || !IsComparedWithAnAnonymousObject(invocation)
            || !invocation.IsShouldInvocation(context.SemanticModel, context.CancellationToken))
        {
            return;
        }

        context.ReportDiagnostic(Diagnostic.Create(
            Descriptors.NoAnonymousObjectComparison,
            subject.GetLocation(),
            subject.Initializers.Count));
    }

    /// <summary>Does the matcher called on <paramref name="shouldInvocation"/> take an anonymous object?</summary>
    private static bool IsComparedWithAnAnonymousObject(InvocationExpressionSyntax shouldInvocation)
    {
        if (shouldInvocation.Parent is not MemberAccessExpressionSyntax { Parent: InvocationExpressionSyntax matcher })
        {
            return false;
        }

        foreach (var argument in matcher.ArgumentList.Arguments)
        {
            if (argument.Expression.Unparenthesize() is AnonymousObjectCreationExpressionSyntax)
            {
                return true;
            }
        }

        return false;
    }
}
