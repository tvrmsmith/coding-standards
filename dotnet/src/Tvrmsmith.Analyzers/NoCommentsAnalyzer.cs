using System;
using System.Collections.Immutable;
using System.Linq;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;
using Microsoft.CodeAnalysis.Diagnostics;
using Microsoft.CodeAnalysis.Text;

namespace Tvrmsmith.Analyzers;

/// <summary>
/// TVRM0008 <c>no-comments</c>: every comment is reported except an XML doc comment on a
/// declaration, a suppression or directive a tool reads, and anything in generated code.
/// </summary>
/// <remarks>
/// <para>
/// The TypeScript half is the ESLint rule <c>tvrmsmith/no-comments</c> and the Go half is the
/// <c>tvrmsmith-no-comments</c> golangci-lint plugin. The guideline behind all three is that code
/// documents itself, and what another agent needs to know is written as a test.
/// </para>
/// <para>
/// Generated code is skipped by Roslyn itself through <see cref="GeneratedCodeAnalysisFlags.None"/>:
/// an <c>&lt;auto-generated&gt;</c> header, a <c>.g.cs</c> or <c>.designer.cs</c> name, or
/// <c>generated_code = true</c> in an analyzer config.
/// </para>
/// <para>
/// A suppression's own reason needs no exemption here. <c>#pragma warning disable CA1234 // why</c>
/// holds its comment inside the directive's structured trivia, which the walk does not descend
/// into, so the comment stays with the suppression it explains.
/// </para>
/// </remarks>
[DiagnosticAnalyzer(LanguageNames.CSharp)]
public sealed class NoCommentsAnalyzer : DiagnosticAnalyzer
{
    /// <summary>
    /// Comment prefixes a tool reads, matched against the text after the <c>//</c> or <c>/*</c>.
    /// </summary>
    /// <remarks>
    /// ReSharper and Rider suppress inspections and pause the formatter through comments, and
    /// SonarAnalyzer honours <c>NOSONAR</c>. The compiler's own suppressions are
    /// <c>#pragma</c> directives and attributes, which are not comments at all.
    /// </remarks>
    private static readonly ImmutableArray<string> DirectivePrefixes = ImmutableArray.Create(
        "ReSharper disable",
        "ReSharper restore",
        "NOSONAR",
        "@formatter:off",
        "@formatter:on");

    /// <inheritdoc />
    public override ImmutableArray<DiagnosticDescriptor> SupportedDiagnostics { get; } =
        ImmutableArray.Create(Descriptors.NoComments);

    /// <inheritdoc />
    public override void Initialize(AnalysisContext context)
    {
        context.ConfigureGeneratedCodeAnalysis(GeneratedCodeAnalysisFlags.None);
        context.EnableConcurrentExecution();
        context.RegisterSyntaxTreeAction(AnalyzeTree);
    }

    private static void AnalyzeTree(SyntaxTreeAnalysisContext context)
    {
        var root = context.Tree.GetRoot(context.CancellationToken);

        foreach (var trivia in root.DescendantTrivia())
        {
            if (IsComment(trivia) && !IsExempt(trivia))
            {
                context.ReportDiagnostic(Diagnostic.Create(Descriptors.NoComments, LocationOf(context.Tree, trivia)));
            }
        }
    }

    /// <summary>
    /// The comment's whole text, its delimiter included. A doc comment's own span starts after
    /// its <c>///</c> and runs through the line break, so it is trimmed back to the text instead.
    /// </summary>
    private static Location LocationOf(SyntaxTree tree, SyntaxTrivia trivia)
    {
        var text = trivia.ToFullString();
        var end = trivia.FullSpan.Start + text.TrimEnd('\r', '\n').Length;
        return Location.Create(tree, TextSpan.FromBounds(trivia.FullSpan.Start, end));
    }

    private static bool IsComment(SyntaxTrivia trivia) =>
        trivia.IsKind(SyntaxKind.SingleLineCommentTrivia)
        || trivia.IsKind(SyntaxKind.MultiLineCommentTrivia)
        || trivia.IsKind(SyntaxKind.SingleLineDocumentationCommentTrivia)
        || trivia.IsKind(SyntaxKind.MultiLineDocumentationCommentTrivia);

    private static bool IsExempt(SyntaxTrivia trivia) =>
        IsDirective(trivia) || (IsWrittenAsDocComment(trivia) && DocumentsADeclaration(trivia));

    /// <summary>
    /// Whether the comment is written as an XML doc comment, <c>///</c> or <c>/** ... */</c>.
    /// </summary>
    /// <remarks>
    /// Read off the text as well as the kind. A project that does not generate a documentation
    /// file parses with <see cref="DocumentationMode.None"/>, and there the compiler hands a
    /// <c>///</c> line over as an ordinary single-line comment.
    /// </remarks>
    private static bool IsWrittenAsDocComment(SyntaxTrivia trivia)
    {
        if (trivia.IsKind(SyntaxKind.SingleLineDocumentationCommentTrivia)
            || trivia.IsKind(SyntaxKind.MultiLineDocumentationCommentTrivia))
        {
            return true;
        }

        var text = trivia.ToFullString();
        return (text.StartsWith("///", StringComparison.Ordinal) && !text.StartsWith("////", StringComparison.Ordinal))
            || (text.StartsWith("/**", StringComparison.Ordinal) && !text.StartsWith("/**/", StringComparison.Ordinal));
    }

    /// <summary>
    /// Whether the comment sits in the leading trivia of the first token of a declaration, the
    /// only place the compiler reads a doc comment from.
    /// </summary>
    /// <remarks>
    /// A doc comment above a statement documents nothing, so it is reported like any other
    /// comment. That keeps the exemption from being the easy way around the rule. A top-level
    /// statement is a <see cref="MemberDeclarationSyntax"/> by type but declares nothing, so it is
    /// left out.
    /// </remarks>
    private static bool DocumentsADeclaration(SyntaxTrivia trivia)
    {
        var token = trivia.Token;
        if (!token.LeadingTrivia.Contains(trivia))
        {
            return false;
        }

        return token.Parent?
            .AncestorsAndSelf()
            .TakeWhile(node => node.GetFirstToken() == token)
            .Any(IsDeclaration) == true;
    }

    private static bool IsDeclaration(SyntaxNode node) =>
        node is MemberDeclarationSyntax and not GlobalStatementSyntax
            or AccessorDeclarationSyntax
            or LocalFunctionStatementSyntax;

    private static bool IsDirective(SyntaxTrivia trivia)
    {
        if (!trivia.IsKind(SyntaxKind.SingleLineCommentTrivia) && !trivia.IsKind(SyntaxKind.MultiLineCommentTrivia))
        {
            return false;
        }

        var body = trivia.ToString().Substring(2).TrimStart();
        return DirectivePrefixes.Any(prefix => body.StartsWith(prefix, StringComparison.Ordinal));
    }
}
