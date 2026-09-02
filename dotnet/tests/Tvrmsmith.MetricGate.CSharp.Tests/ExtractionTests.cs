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
    public void ANestedTypesMethodNameIsQualifiedByItsDeclaringType()
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
        result.Spans.Should().OnlyContain(s => s.File == "fixtures/Plain.cs");
        result.Spans.Should().HaveCount(2);
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
}
