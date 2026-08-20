using System.Collections.Generic;
using System.Collections.Immutable;
using System.Globalization;
using System.Threading;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;
using Microsoft.CodeAnalysis.Diagnostics;

namespace Tvrmsmith.Analyzers;

/// <summary>
/// TVRM0001 <c>combine-assertions-on-same-object</c> — consecutive assertions that pick apart one
/// object should be a single <c>BeEquivalentTo</c> against an anonymous object.
/// </summary>
/// <remarks>
/// <para>Two shapes are flagged, both straight out of the reference:</para>
/// <list type="number">
/// <item>
/// the property group — <c>result.Page.Should().Be(2); result.PageSize.Should().Be(3);</c>
/// </item>
/// <item>
/// count-then-index — <c>items.Should().HaveCount(2); items[0].Name.Should().Be("Alice");</c>
/// </item>
/// </list>
/// <para>
/// "The same object" is the *outermost* identifier of the receiver chain, not the immediate
/// receiver: <c>response.StatusCode</c> and <c>response.Headers.Location</c> group together
/// because both are rooted at <c>response</c>. That is deliberately wide, and it will sometimes
/// name a pair that cannot actually be combined — acceptable, because the rule is a warning and
/// never an error.
/// </para>
/// <para>
/// The counterweight is <c>AssertionScope</c>: under a scope, the property group is not reported
/// at all, on the reading that wrapping assertions in a scope is the author stating these
/// genuinely cannot be combined. Both spellings count, including the one that opens no block of
/// its own — <c>using (new AssertionScope())</c> and <c>using var scope = new AssertionScope();</c>.
/// </para>
/// <para>
/// Count-then-index keeps firing inside a scope. <c>BeEquivalentTo</c> against an expected array
/// always works, so that shape is never one of the uncombinable assertions a scope exists for,
/// and exempting it would turn the scope into a way to hide it.
/// </para>
/// </remarks>
[DiagnosticAnalyzer(LanguageNames.CSharp)]
public sealed class CombineAssertionsOnSameObjectAnalyzer : DiagnosticAnalyzer
{
    private const string HaveCountMethodName = "HaveCount";
    private const string AssertionScopeTypeName = "AssertionScope";

    /// <inheritdoc />
    public override ImmutableArray<DiagnosticDescriptor> SupportedDiagnostics { get; } =
        ImmutableArray.Create(Descriptors.CombineAssertionsOnSameObject);

    /// <inheritdoc />
    public override void Initialize(AnalysisContext context)
    {
        context.ConfigureGeneratedCodeAnalysis(GeneratedCodeAnalysisFlags.None);
        context.EnableConcurrentExecution();
        context.RegisterSyntaxNodeAction(AnalyzeBlock, SyntaxKind.Block);
    }

    private static void AnalyzeBlock(SyntaxNodeAnalysisContext context)
    {
        var block = (BlockSyntax)context.Node;
        var statements = block.Statements;
        if (statements.Count < 2)
        {
            return;
        }

        var underAssertionScope = IsUnderAssertionScope(
            block, context.SemanticModel, context.CancellationToken);

        // Assertions are only combinable when nothing runs between them, so the scan walks
        // adjacent statements and a non-assertion breaks the run.
        var assertions = new Assertion?[statements.Count];
        for (var i = 0; i < statements.Count; i++)
        {
            assertions[i] = Assertion.Describe(statements[i], context.SemanticModel, context.CancellationToken);
        }

        var index = 0;
        while (index < assertions.Length)
        {
            var first = assertions[index];
            if (first is null)
            {
                index++;
                continue;
            }

            var runEnd = first.IsHaveCount ? ExtendIndexedRun(assertions, index, first) : index;
            var isIndexedRun = runEnd > index;

            if (!isIndexedRun && !underAssertionScope)
            {
                runEnd = ExtendPropertyRun(assertions, index, first);
            }

            if (runEnd > index)
            {
                Report(context, assertions, index, runEnd, first, isIndexedRun);
                index = runEnd + 1;
                continue;
            }

            index++;
        }
    }

    /// <summary>
    /// Whether the statements in <paramref name="block"/> run under an <c>AssertionScope</c>.
    /// </summary>
    /// <remarks>
    /// The walk stops at the enclosing method, lambda or local function, so a scope opened in an
    /// unrelated sibling never reaches here. A using-*declaration* is treated as covering its whole
    /// block rather than only the statements that follow it: the ordering that would make the
    /// difference — assertions above the declaration — is not a shape worth the extra machinery.
    /// </remarks>
    private static bool IsUnderAssertionScope(
        SyntaxNode block,
        SemanticModel semanticModel,
        CancellationToken cancellationToken)
    {
        for (SyntaxNode? current = block; current is not null; current = current.Parent)
        {
            switch (current)
            {
                case UsingStatementSyntax usingStatement
                    when CreatesAssertionScope(usingStatement.Expression, semanticModel, cancellationToken)
                        || DeclaresAssertionScope(usingStatement.Declaration, semanticModel, cancellationToken):
                    return true;

                case BlockSyntax candidate
                    when HasAssertionScopeUsingDeclaration(candidate, semanticModel, cancellationToken):
                    return true;

                case BaseMethodDeclarationSyntax:
                case AnonymousFunctionExpressionSyntax:
                case LocalFunctionStatementSyntax:
                    return false;
            }
        }

        return false;
    }

    /// <summary>The <c>using var scope = new AssertionScope();</c> spelling, which opens no block.</summary>
    private static bool HasAssertionScopeUsingDeclaration(
        BlockSyntax block,
        SemanticModel semanticModel,
        CancellationToken cancellationToken)
    {
        foreach (var statement in block.Statements)
        {
            if (statement is LocalDeclarationStatementSyntax declaration
                && declaration.UsingKeyword.IsKind(SyntaxKind.UsingKeyword)
                && DeclaresAssertionScope(declaration.Declaration, semanticModel, cancellationToken))
            {
                return true;
            }
        }

        return false;
    }

    private static bool DeclaresAssertionScope(
        VariableDeclarationSyntax? declaration,
        SemanticModel semanticModel,
        CancellationToken cancellationToken)
    {
        if (declaration is null)
        {
            return false;
        }

        foreach (var variable in declaration.Variables)
        {
            if (CreatesAssertionScope(variable.Initializer?.Value, semanticModel, cancellationToken))
            {
                return true;
            }
        }

        return false;
    }

    /// <summary>
    /// Matched on the type name alone, so the rule reads FluentAssertions and AwesomeAssertions
    /// the same way — the namespaces differ, the type does not.
    /// </summary>
    private static bool CreatesAssertionScope(
        ExpressionSyntax? expression,
        SemanticModel semanticModel,
        CancellationToken cancellationToken) =>
        expression is not null
        && semanticModel.GetTypeInfo(expression, cancellationToken).Type?.Name == AssertionScopeTypeName;

    /// <summary>
    /// <c>items.Should().HaveCount(2)</c> followed by assertions that index into <c>items</c>.
    /// A bare <c>HaveCount</c> with nothing indexing after it is the documented good shape.
    /// </summary>
    private static int ExtendIndexedRun(Assertion?[] assertions, int start, Assertion first)
    {
        var end = start;

        for (var next = start + 1; next < assertions.Length; next++)
        {
            var candidate = assertions[next];
            if (candidate?.IndexedRoot is null
                || !SyntaxFactory.AreEquivalent(candidate.IndexedRoot, first.Receiver))
            {
                break;
            }

            end = next;
        }

        return end;
    }

    /// <summary>Consecutive assertions whose asserted members hang off the same object.</summary>
    private static int ExtendPropertyRun(Assertion?[] assertions, int start, Assertion first)
    {
        if (first.Target is null)
        {
            return start;
        }

        var end = start;

        for (var next = start + 1; next < assertions.Length; next++)
        {
            var candidate = assertions[next];
            if (candidate?.Target is null
                || !SyntaxFactory.AreEquivalent(candidate.Target, first.Target))
            {
                break;
            }

            end = next;
        }

        return end;
    }

    private static void Report(
        SyntaxNodeAnalysisContext context,
        Assertion?[] assertions,
        int start,
        int end,
        Assertion first,
        bool isIndexedRun)
    {
        var subject = isIndexedRun ? first.Receiver : first.Target!;

        var additionalLocations = new List<Location>();
        for (var i = start + 1; i <= end; i++)
        {
            additionalLocations.Add(assertions[i]!.Statement.GetLocation());
        }

        context.ReportDiagnostic(Diagnostic.Create(
            Descriptors.CombineAssertionsOnSameObject,
            first.Statement.GetLocation(),
            additionalLocations,
            (end - start + 1).ToString(CultureInfo.InvariantCulture),
            subject.ToString()));
    }

    /// <summary>One statement, reduced to the facts the grouping needs.</summary>
    private sealed class Assertion
    {
        private Assertion(
            ExpressionStatementSyntax statement,
            ExpressionSyntax receiver,
            ExpressionSyntax? target,
            ExpressionSyntax? indexedRoot,
            bool isHaveCount)
        {
            Statement = statement;
            Receiver = receiver;
            Target = target;
            IndexedRoot = indexedRoot;
            IsHaveCount = isHaveCount;
        }

        /// <summary>The whole statement, for reporting.</summary>
        public ExpressionStatementSyntax Statement { get; }

        /// <summary>What <c>.Should()</c> was called on.</summary>
        public ExpressionSyntax Receiver { get; }

        /// <summary>
        /// The object whose member is under assertion — the outermost identifier the receiver
        /// chain is rooted at. Null when the assertion is against a whole value rather than one
        /// of its members.
        /// </summary>
        public ExpressionSyntax? Target { get; }

        /// <summary>The collection this assertion indexes into, if it does.</summary>
        public ExpressionSyntax? IndexedRoot { get; }

        /// <summary>Whether the assertion is <c>.Should().HaveCount(...)</c>.</summary>
        public bool IsHaveCount { get; }

        public static Assertion? Describe(
            StatementSyntax statement,
            SemanticModel semanticModel,
            CancellationToken cancellationToken)
        {
            if (statement is not ExpressionStatementSyntax expressionStatement)
            {
                return null;
            }

            var should = AssertionSyntax.RootShouldInvocation(
                expressionStatement.Expression, semanticModel, cancellationToken);

            if (should?.ShouldReceiver()?.Unparenthesize() is not { } receiver
                || !AssertionSyntax.IsStableReference(receiver))
            {
                return null;
            }

            return new Assertion(
                expressionStatement,
                receiver,
                TargetOf(receiver),
                IndexedRootOf(receiver),
                IsHaveCountAssertion(should));
        }

        /// <summary>
        /// The identifier the receiver chain bottoms out at — <c>response</c> for both
        /// <c>response.StatusCode</c> and <c>response.Headers.Location</c>.
        /// </summary>
        /// <remarks>
        /// Null when the receiver *is* that identifier, which keeps
        /// <c>x.Should().Be(1); x.Should().BeGreaterThan(0);</c> out of the grouping: those are
        /// two assertions on one value, not two members that a single anonymous object replaces.
        /// </remarks>
        private static ExpressionSyntax? TargetOf(ExpressionSyntax receiver)
        {
            ExpressionSyntax? root = null;

            foreach (var node in AssertionSyntax.WalkReceiverSpine(receiver))
            {
                root = node;
            }

            return root is IdentifierNameSyntax or ThisExpressionSyntax or BaseExpressionSyntax
                && root != receiver
                ? root
                : null;
        }

        /// <summary>
        /// The collection an assertion reaches through an indexer — <c>items</c> for both
        /// <c>items[0]</c> and <c>items[0].Name</c>.
        /// </summary>
        private static ExpressionSyntax? IndexedRootOf(ExpressionSyntax receiver)
        {
            ExpressionSyntax? root = null;

            foreach (var node in AssertionSyntax.WalkReceiverSpine(receiver))
            {
                if (node is ElementAccessExpressionSyntax elementAccess)
                {
                    root = elementAccess.Expression.Unparenthesize();
                }
            }

            return root;
        }

        private static bool IsHaveCountAssertion(InvocationExpressionSyntax should) =>
            should.Parent is MemberAccessExpressionSyntax { Parent: InvocationExpressionSyntax } next
            && next.Name.Identifier.ValueText == HaveCountMethodName;
    }
}
