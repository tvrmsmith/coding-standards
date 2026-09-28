using System.Collections.Immutable;
using System.Threading;
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
/// The subject is what makes it a bundle, so both sides have to be anonymous objects someone
/// built with <c>new { ... }</c>, inline or in a <c>var</c> local the assertion names. C# gives an
/// anonymous type no name to write, so a local is the only other place one can be held. A real
/// object against an anonymous expectation is the shape <c>TVRM0001</c> steers toward, and a
/// projection such as <c>items.Select(i =&gt; new { i.Name }).First()</c> has a subject of its
/// own, even held in a local. When every value is read from one object, the advice changes from
/// a scope to a single <c>BeEquivalentTo</c> on that object. The matcher is not named, because anonymous types override
/// <c>Equals</c> and <c>Be</c> compares the bundle as surely as <c>BeEquivalentTo</c> does.
/// </para>
/// </remarks>
[DiagnosticAnalyzer(LanguageNames.CSharp)]
public sealed class NoAnonymousObjectComparisonAnalyzer : DiagnosticAnalyzer
{
    /// <inheritdoc />
    public override ImmutableArray<DiagnosticDescriptor> SupportedDiagnostics { get; } =
        ImmutableArray.Create(
            Descriptors.NoAnonymousObjectComparison,
            Descriptors.NoAnonymousObjectComparisonOfOneObject);

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
        if (invocation.ShouldReceiver()?.Unparenthesize() is not { } subject
            || !CanHoldAnAnonymousObject(subject)
            || invocation.Parent is not MemberAccessExpressionSyntax { Parent: InvocationExpressionSyntax matcher }
            || !invocation.IsShouldInvocation(context.SemanticModel, context.CancellationToken))
        {
            return;
        }

        if (AnonymousObjectBehind(subject, context.SemanticModel, context.CancellationToken) is not { } bundle
            || !IsComparedWithAnAnonymousObject(matcher, context.SemanticModel, context.CancellationToken))
        {
            return;
        }

        context.ReportDiagnostic(SharedRoot(bundle) is { } root
            ? Diagnostic.Create(
                Descriptors.NoAnonymousObjectComparisonOfOneObject,
                subject.GetLocation(),
                root.ToString())
            : Diagnostic.Create(
                Descriptors.NoAnonymousObjectComparison,
                subject.GetLocation(),
                bundle.Initializers.Count));
    }

    /// <summary>
    /// The one object every value in <paramref name="bundle"/> is read from, or
    /// <see langword="null"/> when the values come from more than one place.
    /// </summary>
    /// <remarks>
    /// Values copied out of one object are that object's members, and a single BeEquivalentTo on
    /// the object checks them without the copy. Anything gathered from several places, or
    /// reached through a call, has no subject to compare and belongs in an AssertionScope.
    /// </remarks>
    private static ExpressionSyntax? SharedRoot(AnonymousObjectCreationExpressionSyntax bundle)
    {
        ExpressionSyntax? shared = null;

        foreach (var member in bundle.Initializers)
        {
            var value = member.Expression.Unparenthesize();
            if (!AssertionSyntax.IsStableReference(value)
                || AssertionSyntax.MemberChainRoot(value) is not { } root
                || (shared is not null && !SyntaxFactory.AreEquivalent(shared, root)))
            {
                return null;
            }

            shared = root;
        }

        return shared;
    }

    /// <summary>Does <paramref name="matcher"/> take an anonymous object, inline or through a local?</summary>
    private static bool IsComparedWithAnAnonymousObject(
        InvocationExpressionSyntax matcher,
        SemanticModel semanticModel,
        CancellationToken cancellationToken)
    {
        foreach (var argument in matcher.ArgumentList.Arguments)
        {
            if (AnonymousObjectBehind(argument.Expression, semanticModel, cancellationToken) is not null)
            {
                return true;
            }
        }

        return false;
    }

    private static bool CanHoldAnAnonymousObject(ExpressionSyntax expression) =>
        expression is AnonymousObjectCreationExpressionSyntax or IdentifierNameSyntax;

    /// <summary>
    /// The <c>new { ... }</c> that <paramref name="expression"/> is, or that the local it names
    /// was declared with.
    /// </summary>
    /// <remarks>
    /// Only the declaration is read, so a local declared as a bundle and later reassigned from a
    /// projection of the same shape still reports. Rare enough to accept at warning severity.
    /// </remarks>
    private static AnonymousObjectCreationExpressionSyntax? AnonymousObjectBehind(
        ExpressionSyntax expression,
        SemanticModel semanticModel,
        CancellationToken cancellationToken)
    {
        expression = expression.Unparenthesize();
        if (expression is AnonymousObjectCreationExpressionSyntax creation)
        {
            return creation;
        }

        if (expression is not IdentifierNameSyntax
            || semanticModel.GetSymbolInfo(expression, cancellationToken).Symbol is not ILocalSymbol local)
        {
            return null;
        }

        foreach (var reference in local.DeclaringSyntaxReferences)
        {
            if (reference.GetSyntax(cancellationToken) is VariableDeclaratorSyntax { Initializer.Value: var value }
                && value.Unparenthesize() is AnonymousObjectCreationExpressionSyntax declared)
            {
                return declared;
            }
        }

        return null;
    }
}
