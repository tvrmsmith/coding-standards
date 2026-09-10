using System.Collections.Immutable;
using System.Globalization;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.Diagnostics;
using Microsoft.CodeAnalysis.Text;

namespace Tvrmsmith.Analyzers;

/// <summary>
/// TVRM0006 <c>comment-block-length</c> — a run of non-documentation comments may not run past
/// the budget.
/// </summary>
/// <remarks>
/// The TypeScript half is the ESLint rule of the same name. Neither language had an off-the-shelf
/// home: StyleCop ships no length rule at all, and Sonar's S103 caps a physical line rather than a
/// comment, which flags long code lines and misses a long comment made of short ones.
/// </remarks>
[DiagnosticAnalyzer(LanguageNames.CSharp)]
public sealed class CommentBlockLengthAnalyzer : DiagnosticAnalyzer
{
    /// <summary>Default budget, in source lines.</summary>
    internal const int DefaultMaxLines = 10;

    /// <summary>
    /// The <c>.editorconfig</c> key overriding <see cref="DefaultMaxLines"/>.
    /// </summary>
    /// <remarks>
    /// A plain analyzer-config key rather than a <c>dotnet_diagnostic</c> one: those carry
    /// severity, not rule parameters. Roslyn has no parameter mechanism of its own, which is why
    /// SonarAnalyzer reaches for a SonarLint.xml AdditionalFile instead. A bare key is simpler and
    /// keeps the setting in the same file as the severities.
    /// </remarks>
    internal const string MaxLinesKey = "tvrmsmith_comment_block_max_lines";

    /// <inheritdoc />
    public override ImmutableArray<DiagnosticDescriptor> SupportedDiagnostics { get; } =
        ImmutableArray.Create(Descriptors.CommentBlockLength);

    /// <inheritdoc />
    public override void Initialize(AnalysisContext context)
    {
        context.ConfigureGeneratedCodeAnalysis(GeneratedCodeAnalysisFlags.None);
        context.EnableConcurrentExecution();
        context.RegisterSyntaxTreeAction(AnalyzeTree);
    }

    private static void AnalyzeTree(SyntaxTreeAnalysisContext context)
    {
        var options = context.Options.AnalyzerConfigOptionsProvider.GetOptions(context.Tree);
        var maxLines = ReadThreshold(options, MaxLinesKey, DefaultMaxLines);

        var text = context.Tree.GetText(context.CancellationToken);
        var root = context.Tree.GetRoot(context.CancellationToken);

        // The block currently being accumulated. `startPosition` of -1 means none is open.
        var startPosition = -1;
        var endPosition = -1;
        var firstLine = -1;
        var lastLine = -1;

        void ReportIfOverBudget()
        {
            if (startPosition < 0)
            {
                return;
            }

            var lines = lastLine - firstLine + 1;

            if (lines > maxLines)
            {
                context.ReportDiagnostic(Diagnostic.Create(
                    Descriptors.CommentBlockLength,
                    Location.Create(context.Tree, TextSpan.FromBounds(startPosition, endPosition)),
                    lines.ToString(CultureInfo.InvariantCulture),
                    maxLines.ToString(CultureInfo.InvariantCulture)));
            }

            startPosition = -1;
        }

        foreach (var trivia in root.DescendantTrivia())
        {
            if (!IsComment(trivia))
            {
                // Whitespace and newlines between two comment lines are not a break. A blank line
                // shows up as a line-number gap instead, which the adjacency check below catches.
                continue;
            }

            // A doc comment is exempt at any length, and it also ends the run rather than
            // absorbing it. Otherwise a two-line summary glued above thirty lines of prose would
            // exempt the prose, which is the shape the guard exists to catch.
            if (IsDocComment(trivia))
            {
                ReportIfOverBudget();
                continue;
            }

            // A trailing comment annotates the code on its line rather than standing on its own.
            // It neither opens nor extends a block, and a run of them is never one block: that
            // would be measuring the code.
            if (!StartsItsLine(text, trivia.SpanStart))
            {
                ReportIfOverBudget();
                continue;
            }

            var triviaFirstLine = text.Lines.GetLineFromPosition(trivia.SpanStart).LineNumber;
            var triviaLastLine = text.Lines.GetLineFromPosition(trivia.Span.End - 1).LineNumber;

            if (startPosition >= 0 && triviaFirstLine == lastLine + 1)
            {
                endPosition = trivia.Span.End;
                lastLine = triviaLastLine;
                continue;
            }

            ReportIfOverBudget();

            startPosition = trivia.SpanStart;
            endPosition = trivia.Span.End;
            firstLine = triviaFirstLine;
            lastLine = triviaLastLine;
        }

        ReportIfOverBudget();
    }

    /// <summary>
    /// Reads a positive integer threshold, falling back to the default when the key is absent or
    /// unparseable.
    /// </summary>
    /// <remarks>
    /// An unparseable value falls back silently, matching how Roslyn treats every other malformed
    /// analyzer-config entry. There is no diagnostic channel to report it on from here, and
    /// failing the build over a typo in a comment-length threshold would be worse than the typo.
    /// </remarks>
    private static int ReadThreshold(AnalyzerConfigOptions options, string key, int fallback) =>
        options.TryGetValue(key, out var raw)
        && int.TryParse(raw, NumberStyles.Integer, CultureInfo.InvariantCulture, out var parsed)
        && parsed > 0
            ? parsed
            : fallback;

    private static bool IsComment(SyntaxTrivia trivia) =>
        trivia.IsKind(SyntaxKind.SingleLineCommentTrivia)
        || trivia.IsKind(SyntaxKind.MultiLineCommentTrivia)
        || IsDocComment(trivia);

    private static bool IsDocComment(SyntaxTrivia trivia) =>
        trivia.IsKind(SyntaxKind.SingleLineDocumentationCommentTrivia)
        || trivia.IsKind(SyntaxKind.MultiLineDocumentationCommentTrivia);

    /// <summary>Whether nothing but whitespace precedes <paramref name="position"/> on its line.</summary>
    private static bool StartsItsLine(SourceText text, int position)
    {
        var line = text.Lines.GetLineFromPosition(position);

        for (var i = line.Start; i < position; i++)
        {
            if (!char.IsWhiteSpace(text[i]))
            {
                return false;
            }
        }

        return true;
    }
}
