using System.Collections.Generic;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.CodeAnalysis.CSharp.Testing;
using Microsoft.CodeAnalysis.Testing;
using Xunit;

namespace Tvrmsmith.Analyzers.Tests;

/// <summary>TVRM0006 — <c>comment-block-length</c>.</summary>
/// <remarks>
/// Every comment line the helpers emit is the same width, so a diagnostic's end column is a
/// constant and the expected spans stay readable.
/// </remarks>
public class CommentBlockLengthAnalyzerTests
{
    /// <summary>Width of one generated comment line, and so the end column of any span.</summary>
    private const int LineWidth = 10;

    /// <summary><paramref name="count"/> own-line <c>//</c> comments on consecutive lines.</summary>
    private static string LineComments(int count) =>
        string.Join("\n", Enumerable.Range(1, count).Select(i => $"// note {i:00}"));

    /// <summary><paramref name="count"/> own-line <c>///</c> doc comments on consecutive lines.</summary>
    private static string DocComments(int count) =>
        string.Join("\n", Enumerable.Range(1, count).Select(i => $"/// <remarks>{i:00}</remarks>"));

    /// <summary>One <c>/* ... *&#47;</c> block comment spanning <paramref name="count"/> lines.</summary>
    private static string BlockComment(int count) =>
        string.Join("\n", new[] { "/*" }
            .Concat(Enumerable.Range(1, count - 2).Select(i => $" * body {i:00}"))
            .Concat(new[] { " */" }));

    private static DiagnosticResult OverBudget(int lines, int budget) =>
        new DiagnosticResult(Descriptors.CommentBlockLength)
            .WithSpan(1, 1, lines, LineWidth + 1)
            .WithArguments(lines.ToString(), budget.ToString());

    private static Task Run(string source, params DiagnosticResult[] expected) =>
        Run(source, editorConfig: null, expected);

    private static Task Run(string source, string? editorConfig, params DiagnosticResult[] expected)
    {
        var test = new Test { TestCode = source + "\nclass C { }\n" };

        if (editorConfig is not null)
        {
            test.TestState.AnalyzerConfigFiles.Add(("/.editorconfig", editorConfig));
        }

        test.ExpectedDiagnostics.AddRange(expected);
        return test.RunAsync(CancellationToken.None);
    }

    // ---- inside the budget ----

    [Fact]
    public Task SilentOnABlockExactlyAtTheBudget() => Run(LineComments(10));

    [Fact]
    public Task SilentOnAShortBlockComment() => Run(BlockComment(4));

    [Fact]
    public Task SilentOnNoCommentsAtAll() => Run("");

    // ---- over the budget ----

    [Fact]
    public Task WarnsOneLineOverTheBudget() => Run(LineComments(11), OverBudget(11, 10));

    /// <summary>A much longer block is still one report, not one per line over.</summary>
    [Fact]
    public Task WarnsOnceOnAMuchLongerBlock() => Run(LineComments(30), OverBudget(30, 10));

    [Fact]
    public Task MeasuresAMultiLineBlockCommentByItsLineSpan() =>
        Run(
            BlockComment(12),
            new DiagnosticResult(Descriptors.CommentBlockLength)
                .WithSpan(1, 1, 12, 4)
                .WithArguments("12", "10"));

    // ---- doc comments are exempt ----

    /// <summary>
    /// The comments guideline requires documenting public API contracts, so a rule that punished
    /// a thorough doc comment would contradict the standard it enforces.
    /// </summary>
    [Fact]
    public Task SilentOnALongSingleLineDocComment() => Run(DocComments(40));

    /// <summary>
    /// A doc comment ends the run rather than absorbing it. Otherwise a short summary glued above
    /// a long stretch of prose would exempt the prose, which is the shape the guard exists to
    /// catch. The plain run below is measured on its own.
    /// </summary>
    [Fact]
    public Task ReportsThePlainRunBelowADocCommentOnItsOwn() =>
        Run(
            DocComments(12) + "\n" + LineComments(12),
            new DiagnosticResult(Descriptors.CommentBlockLength)
                .WithSpan(13, 1, 24, LineWidth + 1)
                .WithArguments("12", "10"));

    [Fact]
    public Task ReportsProseThatAShortSummaryTriesToExempt() =>
        Run(
            DocComments(2) + "\n" + LineComments(14),
            new DiagnosticResult(Descriptors.CommentBlockLength)
                .WithSpan(3, 1, 16, LineWidth + 1)
                .WithArguments("14", "10"));

    /// <summary>The same break in the other direction: the run above a doc comment ends at it.</summary>
    [Fact]
    public Task SilentOnAShortRunEndedByADocComment() =>
        Run(LineComments(6) + "\n" + DocComments(6));

    // ---- what ends a block ----

    [Fact]
    public Task SilentOnTwoShortParagraphsSplitByABlankLine() =>
        Run(LineComments(8) + "\n\n" + LineComments(8));

    /// <summary>
    /// A trailing comment annotates the code on its line. Gluing a run of them together would be
    /// measuring the code, not the comment.
    /// </summary>
    [Fact]
    public Task SilentOnALongRunOfTrailingComments() =>
        Run(string.Join("\n", Enumerable.Range(1, 15).Select(i => $"class C{i:00} {{ }} // note {i:00}")));

    [Fact]
    public Task SilentWhenCodeSplitsTwoShortBlocks() =>
        Run(LineComments(8) + "\nclass A { }\n" + LineComments(8));

    // ---- the budget is configurable ----

    /// <summary>
    /// A plain analyzer-config key rather than a <c>dotnet_diagnostic</c> one, which carries
    /// severity and not rule parameters.
    /// </summary>
    [Fact]
    public Task HonoursALoweredBudgetFromEditorConfig() =>
        Run(
            LineComments(4),
            """
            root = true

            [*.cs]
            tvrmsmith_comment_block_max_lines = 3
            """,
            OverBudget(4, 3));

    [Fact]
    public Task SilentInsideARaisedBudgetFromEditorConfig() =>
        Run(
            LineComments(14),
            """
            root = true

            [*.cs]
            tvrmsmith_comment_block_max_lines = 20
            """);

    /// <summary>
    /// An unparseable threshold falls back to the default, matching how Roslyn treats every other
    /// malformed analyzer-config entry. Failing the build over a typo here would be worse.
    /// </summary>
    [Fact]
    public Task FallsBackToTheDefaultOnAnUnparseableThreshold() =>
        Run(
            LineComments(11),
            """
            root = true

            [*.cs]
            tvrmsmith_comment_block_max_lines = plenty
            """,
            OverBudget(11, 10));

    private sealed class Test : CSharpAnalyzerTest<CommentBlockLengthAnalyzer, DefaultVerifier>
    {
        public Test() => ReferenceAssemblies = ReferenceAssemblies.Net.Net80;
    }
}
