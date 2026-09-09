using System.Collections.Immutable;
using System.Linq;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;
using Microsoft.CodeAnalysis.Diagnostics;

namespace Tvrmsmith.Analyzers;

/// <summary>
/// TVRM0005 <c>no-dropped-async-assertion</c> — an assertion statement whose Task nobody awaits.
/// </summary>
/// <remarks>
/// <para>
/// One of the three shapes under <em>Assertions Must Actually Execute</em>.
/// <c>act.Should().ThrowAsync&lt;T&gt;();</c> hands back a Task carrying the result. Drop it and
/// the test method returns before the assertion resolves.
/// </para>
/// <para>
/// Scoped to what the compiler cannot already see. Inside an <c>async</c> method CS4014 reports
/// this, so the rule stays quiet there and would otherwise double-report; in a synchronous test
/// body there is no <c>await</c> for CS4014 to key off, and nothing warns at all. That
/// synchronous case is the entire reason this rule exists.
/// </para>
/// </remarks>
[DiagnosticAnalyzer(LanguageNames.CSharp)]
public sealed class NoDroppedAsyncAssertionAnalyzer : DiagnosticAnalyzer
{
    /// <inheritdoc />
    public override ImmutableArray<DiagnosticDescriptor> SupportedDiagnostics { get; } =
        ImmutableArray.Create(Descriptors.NoDroppedAsyncAssertion);

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

        // An awaited assertion is an AwaitExpression statement, so reaching an invocation here
        // already means the result went nowhere.
        if (statement.Expression is not InvocationExpressionSyntax invocation)
        {
            return;
        }

        if (AssertionSyntax.RootShouldInvocation(invocation, context.SemanticModel, context.CancellationToken) is null)
        {
            return;
        }

        if (EnclosingFunctionIsAsync(statement))
        {
            return;
        }

        if (!IsAwaitable(context, invocation))
        {
            return;
        }

        context.ReportDiagnostic(Diagnostic.Create(
            Descriptors.NoDroppedAsyncAssertion,
            statement.GetLocation(),
            invocation.ToString()));
    }

    /// <summary>
    /// Walks out to the nearest enclosing function, which is where CS4014's own scope ends too.
    /// </summary>
    private static bool EnclosingFunctionIsAsync(SyntaxNode node)
    {
        foreach (var ancestor in node.Ancestors())
        {
            switch (ancestor)
            {
                case MethodDeclarationSyntax method:
                    return method.Modifiers.Any(SyntaxKind.AsyncKeyword);
                case LocalFunctionStatementSyntax local:
                    return local.Modifiers.Any(SyntaxKind.AsyncKeyword);
                case AnonymousFunctionExpressionSyntax anonymous:
                    return anonymous.AsyncKeyword.IsKind(SyntaxKind.AsyncKeyword);
                case AccessorDeclarationSyntax:
                case ConstructorDeclarationSyntax:
                    return false;
            }
        }

        return false;
    }

    /// <summary>
    /// Awaitability is asked of the type rather than matched against a list of Task names, so a
    /// framework's own awaitable assertion type counts the same as <c>Task</c>.
    /// </summary>
    private static bool IsAwaitable(SyntaxNodeAnalysisContext context, ExpressionSyntax expression)
    {
        if (context.SemanticModel.GetTypeInfo(expression, context.CancellationToken).Type is not { } type)
        {
            return false;
        }

        return HasGetAwaiter(type) || type.AllInterfaces.Any(HasGetAwaiter);
    }

    private static bool HasGetAwaiter(ITypeSymbol type) =>
        type.GetMembers("GetAwaiter").Any(member => member is IMethodSymbol { Parameters.IsEmpty: true });
}
