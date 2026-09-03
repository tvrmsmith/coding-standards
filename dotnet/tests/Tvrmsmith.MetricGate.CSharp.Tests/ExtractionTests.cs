using System.Linq;
using FluentAssertions;
using Xunit;

namespace Tvrmsmith.MetricGate.CSharp.Tests;

public class ExtractionTests
{
    [Fact]
    public void AFileWithNoMethodsParsesWithZeroSpans()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/NoMethods.cs");

        exitCode.Should().Be(0);
        result.Files.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/NoMethods.cs", Status = "parsed" },
        });
        result.Spans.Should().BeEmpty();
    }

    [Fact]
    public void APlainFileYieldsOneSpanPerMethodWithNameLinesAndComplexity()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Plain.cs");

        exitCode.Should().Be(0);
        result.Files.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/Plain.cs", Status = "parsed" },
        });
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/Plain.cs", Name = "Plain.Simple", StartLine = 5, EndLine = 8, Complexity = 1 },
            new { File = "fixtures/Plain.cs", Name = "Plain.Branchy", StartLine = 10, EndLine = 23, Complexity = 4 },
        }, o => o.WithStrictOrdering());
    }

    /// <summary>
    /// Every scored decision point in one method: base 1, ?? , ?:, for, do, case 1, case 2
    /// (default scores nothing), catch, when — nine total, per the assignment's worked count.
    /// </summary>
    [Fact]
    public void EveryScoredDecisionPointContributesOnePoint()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Points.cs");

        exitCode.Should().Be(0);
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/Points.cs", Name = "Points.All", StartLine = 5, EndLine = 36, Complexity = 9 },
        });
    }

    [Fact]
    public void AFileThatFailsToParseContributesNoSpansAndStillExitsZero()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Broken.cs");

        exitCode.Should().Be(0);
        result.Files.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/Broken.cs", Status = "failed" },
        });
        result.Spans.Should().BeEmpty();
    }

    [Fact]
    public void AMethodInASubdirectoryIsQualifiedByItsDeclaringTypeAlone()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/sub/Nested.cs");

        exitCode.Should().Be(0);
        result.Files.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/sub/Nested.cs", Status = "parsed" },
        });
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/sub/Nested.cs", Name = "Nested.One", StartLine = 5, EndLine = 13, Complexity = 2 },
        });
    }

    [Fact]
    public void AMethodInsideANestedTypeIsQualifiedOutermostTypeFirst()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/OuterInner.cs");

        exitCode.Should().Be(0);
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/OuterInner.cs", Name = "Outer.Inner.M", StartLine = 7, EndLine = 15, Complexity = 2 },
        });
    }

    [Fact]
    public void APathIsEchoedByteIdenticalNeverRewrittenOrAbsolutized()
    {
        var (_, result) = ExtractorRun.Run("fixtures/sub/Nested.cs");

        result.Files.Should().ContainSingle().Which.File.Should().Be("fixtures/sub/Nested.cs");
    }

    [Fact]
    public void MixingParsedFailedAndSilentFilesOnOneRunReportsAllThreeAndSpansOnlyFromTheParsedOne()
    {
        var (exitCode, result) = ExtractorRun.Run(
            "fixtures/Plain.cs", "fixtures/Broken.cs", "fixtures/NoMethods.cs");

        exitCode.Should().Be(0);
        result.Files.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/Plain.cs", Status = "parsed" },
            new { File = "fixtures/Broken.cs", Status = "failed" },
            new { File = "fixtures/NoMethods.cs", Status = "parsed" },
        }, o => o.WithStrictOrdering());
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/Plain.cs", Name = "Plain.Simple" },
            new { File = "fixtures/Plain.cs", Name = "Plain.Branchy" },
        }, o => o.WithStrictOrdering());
    }

    [Fact]
    public void TwoOverloadsOnOneLineAreTwoSpansToldApartByTheirSignatures()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Overloads.cs");

        exitCode.Should().Be(0);
        result.Spans.Where(span => span.Name == "Overloads.F").Should().BeEquivalentTo(new[]
        {
            new { Name = "Overloads.F", Signature = "(int)", StartLine = 7, EndLine = 7 },
            new { Name = "Overloads.F", Signature = "(string)", StartLine = 7, EndLine = 7 },
        }, o => o.WithStrictOrdering());
    }

    /// <summary>
    /// The signature spelling <c>MethodSpanResult</c> documents, so an extractor for another
    /// language can match it. Modifiers in order, the source spelling of the type, generic arity as
    /// a backtick count, and never a parameter name.
    /// </summary>
    [Fact]
    public void ASignatureSpellsModifiersSourceTypesAndGenericArityButNeverParameterNames()
    {
        var (_, result) = ExtractorRun.Run("fixtures/Overloads.cs");

        result.Spans.Should().Contain(span => span.Name == "Overloads.G")
            .Which.Signature.Should().Be("(ref int, out string, params int[])");
        result.Spans.Should().Contain(span => span.Name == "Overloads.H")
            .Which.Signature.Should().Be("(Dictionary<string, int>, int[]?, (int x, string y))");
        result.Spans.Should().Contain(span => span.Name == "Overloads.I")
            .Which.Signature.Should().Be("`1(T)");
    }

    /// <summary>
    /// One point each for a foreach, an ||, a pattern case label and a case guard. Guarded is a
    /// constant case label plus its when clause, so its delta over Constant is the guard alone.
    /// </summary>
    [Fact]
    public void ForeachOrPatternCaseAndCaseGuardEachScoreOnePoint()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Constructs.cs");

        exitCode.Should().Be(0);
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { Name = "Constructs.Each", Complexity = 2 },
            new { Name = "Constructs.Either", Complexity = 2 },
            new { Name = "Constructs.Constant", Complexity = 2 },
            new { Name = "Constructs.Typed", Complexity = 2 },
            new { Name = "Constructs.Guarded", Complexity = 3 },
        }, o => o.WithStrictOrdering());
    }

    /// <summary>
    /// One point per switch expression arm, the discard arm included, so a method built from a
    /// switch expression is no cheaper than the same logic written as a switch statement.
    /// </summary>
    [Fact]
    public void EverySwitchExpressionArmScoresOnePoint()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Patterns.cs");

        exitCode.Should().Be(0);
        result.Spans.Where(span => span.Name == "Patterns.Arms").Should().BeEquivalentTo(new[]
        {
            new { Name = "Patterns.Arms", Complexity = 4 },
        });
    }

    /// <summary>
    /// An arm's guard scores on top of the arm, the way a case guard scores on top of its case
    /// label. Two arms plus one guard over base 1.
    /// </summary>
    [Fact]
    public void ASwitchExpressionArmGuardScoresOnTopOfItsArm()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Patterns.cs");

        exitCode.Should().Be(0);
        result.Spans.Where(span => span.Name == "Patterns.GuardedArm").Should().BeEquivalentTo(new[]
        {
            new { Name = "Patterns.GuardedArm", Complexity = 4 },
        });
    }

    /// <summary>
    /// A pattern combinator scores what the operator spelling of the same condition scores, so
    /// rewriting <c>o is int || o is string</c> as <c>o is int or string</c> cannot lower a score.
    /// <c>not</c> adds no branch and does not count.
    /// </summary>
    [Fact]
    public void APatternCombinatorScoresLikeTheOperatorSpellingOfTheSameCondition()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Patterns.cs");

        exitCode.Should().Be(0);
        var combinators = new[]
        {
            "Patterns.OrPattern", "Patterns.AndPattern", "Patterns.OrOperator", "Patterns.NotPattern",
        };
        result.Spans.Where(span => combinators.Contains(span.Name)).Should().BeEquivalentTo(new[]
        {
            new { Name = "Patterns.OrPattern", Complexity = 2 },
            new { Name = "Patterns.AndPattern", Complexity = 2 },
            new { Name = "Patterns.OrOperator", Complexity = 2 },
            new { Name = "Patterns.NotPattern", Complexity = 1 },
        });
    }

    /// <summary>
    /// Every construct <c>ComplexityWalker</c>'s contract enumerates, one per method, so a
    /// construct that stops scoring fails under its own name. A construct dropped from the walker
    /// while it stays in the doc, or added to the walker while the doc says it is out of scope,
    /// has to move a number here. The last three are the contract's stated non-points, and a
    /// guard rides its case label, so <c>CaseGuard</c> is the only one carrying two points.
    /// </summary>
    [Fact]
    public void EveryEnumeratedConstructScoresExactlyThePointsTheContractGivesIt()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Enumerated.cs");

        exitCode.Should().Be(0);
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { Name = "Enumerated.If", Complexity = 2 },
            new { Name = "Enumerated.While", Complexity = 2 },
            new { Name = "Enumerated.Do", Complexity = 2 },
            new { Name = "Enumerated.For", Complexity = 2 },
            new { Name = "Enumerated.Foreach", Complexity = 2 },
            new { Name = "Enumerated.CaseLabel", Complexity = 2 },
            new { Name = "Enumerated.CasePatternLabel", Complexity = 2 },
            new { Name = "Enumerated.CaseGuard", Complexity = 3 },
            new { Name = "Enumerated.SwitchExpressionArm", Complexity = 2 },
            new { Name = "Enumerated.Catch", Complexity = 2 },
            new { Name = "Enumerated.CatchFilter", Complexity = 3 },
            new { Name = "Enumerated.AndPattern", Complexity = 2 },
            new { Name = "Enumerated.OrPattern", Complexity = 2 },
            new { Name = "Enumerated.AndAlso", Complexity = 2 },
            new { Name = "Enumerated.OrElse", Complexity = 2 },
            new { Name = "Enumerated.Conditional", Complexity = 2 },
            new { Name = "Enumerated.Coalesce", Complexity = 2 },
            new { Name = "Enumerated.DefaultLabel", Complexity = 1 },
            new { Name = "Enumerated.NotPattern", Complexity = 1 },
            new { Name = "Enumerated.CoalesceAssign", Complexity = 1 },
            new { Name = "Enumerated.Folded", Complexity = 2 },
        }, o => o.WithStrictOrdering());
    }

    /// <summary>
    /// A local function is not a span of its own, and the decision points inside it fold into the
    /// method that declares it, which is what keeps a method from hiding its branches in one.
    /// </summary>
    [Fact]
    public void ALocalFunctionIsNoSpanOfItsOwnAndItsBranchesFoldIntoItsMethod()
    {
        var (_, result) = ExtractorRun.Run("fixtures/Enumerated.cs");

        result.Spans.Should().ContainSingle(span => span.Name.StartsWith("Enumerated.Folded"))
            .Which.Complexity.Should().Be(2);
        result.Spans.Should().NotContain(span => span.Name.Contains("Inner"));
    }

    [Fact]
    public void ANonExistentPathIsAFailedRowNotACrash()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/does-not-exist.cs");

        exitCode.Should().Be(0);
        result.Files.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/does-not-exist.cs", Status = "failed" },
        });
        result.Spans.Should().BeEmpty();
    }

    /// <summary>
    /// A NUL byte in a path is rejected before any I/O, with an ArgumentException rather than the
    /// IOException a missing file raises. It is still a file the extractor could not read, so it
    /// has to come back as the one failed row rather than as a crashed process that leaves the
    /// gate no JSON to read at all.
    /// </summary>
    [Fact]
    public void APathTheFilesystemCannotExpressIsAFailedRowNotACrash()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/\0.cs");

        exitCode.Should().Be(0);
        result.Files.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/\0.cs", Status = "failed" },
        });
        result.Spans.Should().BeEmpty();
    }
}
