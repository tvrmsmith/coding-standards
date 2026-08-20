using System;
using System.Threading;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;

namespace Tvrmsmith.Analyzers;

/// <summary>
/// Shared recognition of the FluentAssertions shape all three analyzers key off: a
/// <c>.Should()</c> entry point and the receiver chain feeding it.
/// </summary>
/// <remarks>
/// Deliberately library-neutral. FluentAssertions 6.x is the assumed baseline, but the shape
/// — a zero-argument <c>Should</c> extension returning a type whose name ends in
/// <c>Assertions</c> — is identical in AwesomeAssertions and in the bespoke <c>Should()</c>
/// extensions frameworks ship for their own wrapper types. Matching on the shape rather than on
/// a namespace is what lets <c>TVRM0003</c> see both sides of an escape cast.
/// </remarks>
internal static class AssertionSyntax
{
    private const string ShouldMethodName = "Should";
    private const string AssertionsTypeSuffix = "Assertions";

    /// <summary>
    /// Is <paramref name="invocation"/> a <c>.Should()</c> entry point into an assertion chain?
    /// </summary>
    public static bool IsShouldInvocation(
        this InvocationExpressionSyntax invocation,
        SemanticModel semanticModel,
        CancellationToken cancellationToken)
    {
        if (invocation.ArgumentList.Arguments.Count != 0)
        {
            return false;
        }

        // Cheap syntactic gate first — the vast majority of invocations are not named Should,
        // and asking the semantic model is the expensive half.
        var name = invocation.Expression switch
        {
            MemberAccessExpressionSyntax memberAccess => memberAccess.Name,
            MemberBindingExpressionSyntax memberBinding => memberBinding.Name,
            _ => null,
        };

        if (name?.Identifier.ValueText != ShouldMethodName)
        {
            return false;
        }

        return semanticModel.GetSymbolInfo(invocation, cancellationToken).Symbol is IMethodSymbol
        {
            IsExtensionMethod: true,
            Name: ShouldMethodName,
        } method && method.ReturnType.Name.EndsWith(AssertionsTypeSuffix, StringComparison.Ordinal);
    }

    /// <summary>
    /// The expression <c>.Should()</c> was called on, or <see langword="null"/> when the call is
    /// a <c>?.</c> member binding and so has no receiver of its own.
    /// </summary>
    public static ExpressionSyntax? ShouldReceiver(this InvocationExpressionSyntax shouldInvocation) =>
        (shouldInvocation.Expression as MemberAccessExpressionSyntax)?.Expression;

    /// <summary>
    /// Walks down the receiver spine of <paramref name="expression"/>, yielding every node from
    /// the outside in and stopping at the first thing that is not part of the chain.
    /// </summary>
    /// <remarks>
    /// Arguments are never entered. That is what keeps <c>Client.BaseAddress!</c> inside an
    /// expected-value argument out of <c>TVRM0002</c> while still catching a suppression on the
    /// value under test.
    /// </remarks>
    public static SpineWalker WalkReceiverSpine(ExpressionSyntax? expression) => new(expression);

    /// <summary>
    /// The leftmost <c>.Should()</c> in a statement's chain — the one whose receiver is the
    /// object under test, rather than a <c>.Which</c> further along.
    /// </summary>
    public static InvocationExpressionSyntax? RootShouldInvocation(
        ExpressionSyntax expression,
        SemanticModel semanticModel,
        CancellationToken cancellationToken)
    {
        InvocationExpressionSyntax? root = null;

        foreach (var node in WalkReceiverSpine(expression))
        {
            if (node is InvocationExpressionSyntax invocation
                && invocation.IsShouldInvocation(semanticModel, cancellationToken))
            {
                root = invocation;
            }
        }

        return root;
    }

    /// <summary>
    /// Can this expression be named twice in a row and mean the same thing both times?
    /// Identifiers, <c>this</c>/<c>base</c>, member access over those, and indexing by a literal.
    /// </summary>
    /// <remarks>
    /// Invocations are excluded on purpose: <c>Repo.Load().Name</c> and <c>Repo.Load().Age</c>
    /// are not assertions on one object, they are two calls, and combining them would change
    /// behaviour rather than tidy it.
    /// </remarks>
    public static bool IsStableReference(ExpressionSyntax? expression)
    {
        while (true)
        {
            switch (expression)
            {
                case IdentifierNameSyntax:
                case ThisExpressionSyntax:
                case BaseExpressionSyntax:
                case PredefinedTypeSyntax:
                    return true;

                case MemberAccessExpressionSyntax memberAccess:
                    expression = memberAccess.Expression;
                    continue;

                case ElementAccessExpressionSyntax elementAccess:
                    foreach (var argument in elementAccess.ArgumentList.Arguments)
                    {
                        if (argument.Expression is not LiteralExpressionSyntax)
                        {
                            return false;
                        }
                    }

                    expression = elementAccess.Expression;
                    continue;

                case ParenthesizedExpressionSyntax parenthesized:
                    expression = parenthesized.Expression;
                    continue;

                default:
                    return false;
            }
        }
    }

    /// <summary>Strips redundant parentheses without changing what the expression denotes.</summary>
    public static ExpressionSyntax Unparenthesize(this ExpressionSyntax expression)
    {
        while (expression is ParenthesizedExpressionSyntax parenthesized)
        {
            expression = parenthesized.Expression;
        }

        return expression;
    }

    /// <summary>
    /// An allocation-free walk down a receiver chain. A struct enumerator so the per-node
    /// analyzer hot path does not allocate.
    /// </summary>
    internal readonly struct SpineWalker
    {
        private readonly ExpressionSyntax? _start;

        public SpineWalker(ExpressionSyntax? start) => _start = start;

        public Enumerator GetEnumerator() => new(_start);

        internal struct Enumerator
        {
            private ExpressionSyntax? _next;

            public Enumerator(ExpressionSyntax? start)
            {
                _next = start;
                Current = null!;
            }

            public ExpressionSyntax Current { get; private set; }

            public bool MoveNext()
            {
                if (_next is null)
                {
                    return false;
                }

                Current = _next;

                _next = Current switch
                {
                    InvocationExpressionSyntax invocation => invocation.Expression,
                    MemberAccessExpressionSyntax memberAccess => memberAccess.Expression,
                    ElementAccessExpressionSyntax elementAccess => elementAccess.Expression,
                    ParenthesizedExpressionSyntax parenthesized => parenthesized.Expression,
                    CastExpressionSyntax cast => cast.Expression,
                    PostfixUnaryExpressionSyntax postfix
                        when postfix.IsKind(SyntaxKind.SuppressNullableWarningExpression) => postfix.Operand,

                    // MemberBindingExpression is the root of a `?.` continuation and has no
                    // receiver of its own; IdentifierName and friends terminate the chain.
                    _ => null,
                };

                return true;
            }
        }
    }
}
