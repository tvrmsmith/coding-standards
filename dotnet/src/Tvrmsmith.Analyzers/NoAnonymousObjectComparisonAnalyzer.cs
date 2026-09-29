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
/// own, even held in a local. When every value is a member read one level deep from one object,
/// the advice changes from a scope to a single <c>BeEquivalentTo</c> on that object, and a
/// bundle of one other value is told to assert that value directly. An empty bundle holds no
/// value to advise on. The matcher is not named, because anonymous types override
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
            Descriptors.NoAnonymousObjectComparisonOfOneObject,
            Descriptors.NoAnonymousObjectComparisonOfOneValue);

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

        // The syntactic tests run before IsShouldInvocation asks the semantic model.
        if (invocation.ShouldReceiver()?.Unparenthesize() is not { } subject
            || !CanHoldAnAnonymousObject(subject)
            || invocation.Parent is not MemberAccessExpressionSyntax { Parent: InvocationExpressionSyntax matcher }
            || !invocation.IsShouldInvocation(context.SemanticModel, context.CancellationToken))
        {
            return;
        }

        if (AnonymousObjectBehind(subject, context.SemanticModel, context.CancellationToken) is not { } bundle
            || bundle.Initializers.Count == 0
            || !IsComparedWithAnAnonymousObject(matcher, context.SemanticModel, context.CancellationToken))
        {
            return;
        }

        context.ReportDiagnostic(
            AdviceFor(bundle, invocation, subject.GetLocation(), context.SemanticModel, context.CancellationToken));
    }

    /// <summary>The diagnostic whose fix suits <paramref name="bundle"/>.</summary>
    private static Diagnostic AdviceFor(
        AnonymousObjectCreationExpressionSyntax bundle,
        InvocationExpressionSyntax assertion,
        Location location,
        SemanticModel semanticModel,
        CancellationToken cancellationToken)
    {
        if (SharedRoot(bundle, assertion, semanticModel, cancellationToken) is { } root)
        {
            return Diagnostic.Create(Descriptors.NoAnonymousObjectComparisonOfOneObject, location, root.ToString());
        }

        return bundle.Initializers.Count == 1
            ? Diagnostic.Create(
                Descriptors.NoAnonymousObjectComparisonOfOneValue,
                location,
                bundle.Initializers[0].Expression.ToString())
            : Diagnostic.Create(Descriptors.NoAnonymousObjectComparison, location, bundle.Initializers.Count);
    }

    /// <summary>
    /// The one object every value in <paramref name="bundle"/> is read from, or
    /// <see langword="null"/> when the values come from more than one place.
    /// </summary>
    /// <remarks>
    /// Each value has to be <c>root.Member</c>, one level deep, for a single BeEquivalentTo on the
    /// root to check it, and the member has to be a public instance field or property, the only
    /// kind BeEquivalentTo compares by default. A renamed member only changes the name the
    /// expectation writes. A nested or indexed value belongs to some other object, a type or
    /// <c>base</c> is no value to assert, and a root whose <c>Should()</c> cannot take an anonymous
    /// object has no single BeEquivalentTo to offer, so those bundles belong in an AssertionScope.
    /// </remarks>
    private static ExpressionSyntax? SharedRoot(
        AnonymousObjectCreationExpressionSyntax bundle,
        InvocationExpressionSyntax assertion,
        SemanticModel semanticModel,
        CancellationToken cancellationToken)
    {
        ExpressionSyntax? shared = null;

        foreach (var member in bundle.Initializers)
        {
            if (member.Expression.Unparenthesize() is not MemberAccessExpressionSyntax { Expression: var root } access
                || (shared is not null && !SyntaxFactory.AreEquivalent(shared, root))
                || semanticModel.GetSymbolInfo(access, cancellationToken).Symbol
                    is not ((IPropertySymbol or IFieldSymbol)
                        and { IsStatic: false, DeclaredAccessibility: Accessibility.Public }))
            {
                return null;
            }

            shared = root;
        }

        return shared is not null
            && IsAssertableValue(shared, semanticModel, cancellationToken)
            && IsEquivalentToAnAnonymousObject(shared, assertion, semanticModel)
            ? shared
            : null;
    }

    /// <summary>Can <paramref name="root"/> itself be the subject of a <c>Should()</c>?</summary>
    private static bool IsAssertableValue(
        ExpressionSyntax root,
        SemanticModel semanticModel,
        CancellationToken cancellationToken) =>
        root is ThisExpressionSyntax
        || (root is IdentifierNameSyntax
            && semanticModel.GetSymbolInfo(root, cancellationToken).Symbol
                is ILocalSymbol or IParameterSymbol or IFieldSymbol or IPropertySymbol);

    /// <summary>
    /// Would <c><paramref name="root"/>.Should()</c>, written where <paramref name="assertion"/>
    /// is, offer a BeEquivalentTo that takes an anonymous object?
    /// </summary>
    /// <remarks>
    /// Only a generic <c>BeEquivalentTo&lt;T&gt;(T)</c> accepts one. A collection's assertions want
    /// another collection, a string's want a string, and some have no BeEquivalentTo at all.
    /// Asking the assertions type keeps the check free of any list of library types.
    /// </remarks>
    private static bool IsEquivalentToAnAnonymousObject(
        ExpressionSyntax root,
        InvocationExpressionSyntax assertion,
        SemanticModel semanticModel)
    {
        var should = SyntaxFactory.InvocationExpression(
            SyntaxFactory.MemberAccessExpression(
                SyntaxKind.SimpleMemberAccessExpression,
                root.WithoutTrivia(),
                SyntaxFactory.IdentifierName(AssertionSyntax.ShouldMethodName)));

        if (semanticModel.GetSpeculativeSymbolInfo(assertion.SpanStart, should, SpeculativeBindingOption.BindAsExpression)
                .Symbol is not IMethodSymbol { ReturnType: var assertions })
        {
            return false;
        }

        for (var type = assertions; type is not null; type = type.BaseType)
        {
            foreach (var member in type.GetMembers("BeEquivalentTo"))
            {
                if (member is IMethodSymbol { Parameters.Length: > 0 } method
                    && method.Parameters[0].Type is ITypeParameterSymbol { TypeParameterKind: TypeParameterKind.Method })
                {
                    return true;
                }
            }
        }

        return false;
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
