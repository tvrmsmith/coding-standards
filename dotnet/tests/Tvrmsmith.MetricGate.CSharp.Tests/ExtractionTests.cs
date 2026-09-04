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
        result.Spans.Should().Contain(span => span.Name == "Overloads.I<T>")
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
    /// Every construct <c>docs/csharp-decision-points.md</c> enumerates, one per method, so a
    /// construct that stops scoring fails under its own name. A construct dropped from the walker
    /// while it stays in the doc, or added to the walker while the doc says it is out of scope,
    /// has to move a number here. <c>DefaultLabel</c> through <c>BitwiseOrOperator</c> are the
    /// constructs that document says score nothing, the last four of them being deltas where
    /// Roslyn does score. A guard rides its case label, which is why <c>CaseGuard</c> and
    /// <c>CatchFilter</c> carry two points on top of the base.
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
            new { Name = "Enumerated.DeconstructingForeach", Complexity = 2 },
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
            new { Name = "Enumerated.Goto", Complexity = 1 },
            new { Name = "Enumerated.ConditionalAccess", Complexity = 1 },
            new { Name = "Enumerated.BitwiseAndOperator", Complexity = 1 },
            new { Name = "Enumerated.BitwiseOrOperator", Complexity = 1 },
            new { Name = "Enumerated.Folded", Complexity = 1 },
            new { Name = "Enumerated.Folded.Inner", Complexity = 2 },
        }, o => o.WithStrictOrdering());
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

    /// <summary>
    /// Every declaration kind that carries a body or an expression body gets a span, named
    /// metadata style: constructors as <c>.ctor</c>/<c>.cctor</c>, the finalizer as
    /// <c>Finalize</c>, accessors prefixed by their kind, and operators by their metadata name,
    /// which is arity-sensitive for <c>+</c> and <c>-</c> and takes a <c>Checked</c> infix when
    /// the declaration is <c>checked</c>. An explicit interface implementation carries the
    /// interface it implements between the type and the member, so two of them cannot collide.
    /// An auto-property accessor gets a span too, spanning its own line, even though it carries
    /// neither a body nor an expression body. Every conversion operator on a type shares one
    /// metadata name, so its signature carries the target type and the last two, written on one
    /// line, stay distinguishable.
    /// </summary>
    [Fact]
    public void EveryMemberKindThatCarriesLinesGetsASpanNamedMetadataStyle()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Widened.cs");

        exitCode.Should().Be(0);
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { Name = "Widened..cctor", Signature = "()", StartLine = 20, EndLine = 23, Complexity = 1 },
            new { Name = "Widened..ctor", Signature = "(int)", StartLine = 25, EndLine = 31, Complexity = 2 },
            new { Name = "Widened.Finalize", Signature = "()", StartLine = 33, EndLine = 36, Complexity = 1 },
            new { Name = "Widened.get_Id", Signature = "()", StartLine = 38, EndLine = 38, Complexity = 1 },
            new { Name = "Widened.set_Id", Signature = "()", StartLine = 38, EndLine = 38, Complexity = 1 },
            new { Name = "Widened.get_Started", Signature = "()", StartLine = 40, EndLine = 40, Complexity = 1 },
            new { Name = "Widened.init_Started", Signature = "()", StartLine = 40, EndLine = 40, Complexity = 1 },
            new { Name = "Widened.get_Count", Signature = "()", StartLine = 44, EndLine = 52, Complexity = 2 },
            new { Name = "Widened.set_Count", Signature = "()", StartLine = 53, EndLine = 53, Complexity = 1 },
            new { Name = "Widened.get_Total", Signature = "()", StartLine = 56, EndLine = 56, Complexity = 1 },
            new { Name = "Widened.get_Item", Signature = "(int)", StartLine = 58, EndLine = 58, Complexity = 2 },
            new { Name = "Widened.get_Item", Signature = "(string)", StartLine = 62, EndLine = 65, Complexity = 1 },
            new { Name = "Widened.set_Item", Signature = "(string)", StartLine = 67, EndLine = 70, Complexity = 1 },
            new { Name = "Widened.add_Changed", Signature = "()", StartLine = 75, EndLine = 81, Complexity = 2 },
            new { Name = "Widened.remove_Changed", Signature = "()", StartLine = 82, EndLine = 82, Complexity = 1 },
            new { Name = "Widened.IShifted.get_Described", Signature = "()", StartLine = 85, EndLine = 85, Complexity = 1 },
            new { Name = "Widened.IShifted.Describe", Signature = "()", StartLine = 87, EndLine = 87, Complexity = 1 },
            new { Name = "Widened.op_Addition", Signature = "(Widened, Widened)", StartLine = 89, EndLine = 89, Complexity = 1 },
            new { Name = "Widened.op_CheckedAddition", Signature = "(Widened, Widened)", StartLine = 91, EndLine = 91, Complexity = 1 },
            new { Name = "Widened.op_UnaryPlus", Signature = "(Widened)", StartLine = 93, EndLine = 93, Complexity = 1 },
            new { Name = "Widened.op_UnaryNegation", Signature = "(Widened)", StartLine = 95, EndLine = 95, Complexity = 1 },
            new { Name = "Widened.op_Subtraction", Signature = "(Widened, Widened)", StartLine = 97, EndLine = 97, Complexity = 1 },
            new { Name = "Widened.op_Implicit", Signature = "(Widened):int", StartLine = 99, EndLine = 99, Complexity = 1 },
            new { Name = "Widened.op_Explicit", Signature = "(Widened):long", StartLine = 101, EndLine = 101, Complexity = 1 },
            new { Name = "Widened.op_CheckedExplicit", Signature = "(Widened):short", StartLine = 103, EndLine = 103, Complexity = 1 },
            new { Name = "Widened.op_Explicit", Signature = "(Widened):byte", StartLine = 106, EndLine = 106, Complexity = 1 },
            new { Name = "Widened.op_Explicit", Signature = "(Widened):sbyte", StartLine = 106, EndLine = 106, Complexity = 1 },
        }, o => o.WithStrictOrdering());
    }

    /// <summary>
    /// The compiler synthesizes a body for an auto-property accessor, and that is the whole reason
    /// one gets a span. So a partial property's defining half, which promises an accessor rather
    /// than declaring one, gets none while its implementing half gets exactly one pair, whether it
    /// writes accessor bodies or stays an auto-property with an initializer. The file has to report
    /// <c>parsed</c> for any of that to be reachable, since a partial property is C# 13 and an
    /// older parser would call the whole file unparseable and emit nothing.
    /// A <c>static</c> auto-property on an interface gets a pair too, even though
    /// its bodyless neighbour gets neither.
    ///
    /// <para>Which half is which is read off the declaration, never off the order the two halves
    /// appear in. <c>Half</c> and <c>Whole</c> declare the promise first, <c>Early</c> declares its
    /// implementing half first, and its rows land on line 15 rather than line 30, so a rule keyed
    /// on declaration order fails here.</para>
    /// </summary>
    [Fact]
    public void OnlyAnAutoAccessorTheCompilerFillsInGetsASpan()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Synthesized.cs");

        exitCode.Should().Be(0);
        result.Files.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/Synthesized.cs", Status = "parsed" },
        });
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { Name = "Held.get_Early", Signature = "()", StartLine = 15, EndLine = 15, Complexity = 1 },
            new { Name = "Held.set_Early", Signature = "()", StartLine = 15, EndLine = 15, Complexity = 1 },
            new { Name = "Held.get_Half", Signature = "()", StartLine = 24, EndLine = 24, Complexity = 1 },
            new { Name = "Held.set_Half", Signature = "()", StartLine = 25, EndLine = 25, Complexity = 1 },
            new { Name = "Held.get_Whole", Signature = "()", StartLine = 28, EndLine = 28, Complexity = 1 },
            new { Name = "Held.set_Whole", Signature = "()", StartLine = 28, EndLine = 28, Complexity = 1 },
            new { Name = "ICounted.get_Counter", Signature = "()", StartLine = 35, EndLine = 35, Complexity = 1 },
            new { Name = "ICounted.set_Counter", Signature = "()", StartLine = 35, EndLine = 35, Complexity = 1 },
        }, o => o.WithStrictOrdering());
    }

    /// <summary>
    /// A local function declared where no member encloses it, which is what top-level statements
    /// produce, is still a span. It takes its bare local name, and the run reports the file the
    /// same way it reports any other, rather than dying with no JSON at all. The global statements
    /// around it are the documented deferral: they belong to a synthesized <c>&lt;Main&gt;$</c>
    /// with no declaration syntax, so the branch on the file's last <c>if</c> is scored nowhere and
    /// the file yields that one span and no other.
    /// </summary>
    [Fact]
    public void ALocalFunctionWithNoEnclosingSpanTakesItsBareNameAndGlobalStatementsGetNone()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/TopLevel.cs");

        exitCode.Should().Be(0);
        result.Files.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/TopLevel.cs", Status = "parsed" },
        });
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { Name = "Helper", Signature = "(int)@1", StartLine = 5, EndLine = 11, Complexity = 2 },
        });
    }

    /// <summary>
    /// Two sibling scopes declaring the same local name with the same parameters on one line agree
    /// on name, parameter list and line range, so without the start column in the signature they
    /// would be one span reported twice and the gate would abort the run on valid C#. Each is still
    /// scored on its own, which is why the two complexities differ.
    /// </summary>
    [Fact]
    public void TwoSameNamedLocalFunctionsOnOneLineAreTwoSpansToldApartByTheirColumns()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Siblings.cs");

        exitCode.Should().Be(0);
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { Name = "Siblings.Twice", Signature = "(int)", StartLine = 8, EndLine = 13, Complexity = 1 },
            new { Name = "Siblings.Twice.L", Signature = "(int)@11", StartLine = 10, EndLine = 10, Complexity = 2 },
            new { Name = "Siblings.Twice.L", Signature = "(int)@57", StartLine = 10, EndLine = 10, Complexity = 1 },
        }, o => o.WithStrictOrdering());
    }

    /// <summary>
    /// The C# 13 ceiling that <c>docs/csharp-decision-points.md</c> names, pinned from both sides
    /// so it cannot move without a test saying so. <c>Recent.cs</c> holds C# 13 syntax, an
    /// <c>allows ref struct</c> constraint and an <c>\e</c> escape, both of which need the
    /// <c>Microsoft.CodeAnalysis.CSharp</c> reference at 4.12.0 or later and the parse pinned to
    /// the highest language version that reference exposes; a downgrade of either turns it into a
    /// failed row with no spans. <c>Beyond.cs</c> holds C# 14 extension members, which the current
    /// reference cannot parse, so it comes back failed even though a consumer's own compiler
    /// accepts it, which is the gap the ceiling section records. Raising the reference past C# 14
    /// flips that row to parsed and reddens this test, which is the signal to move the ceiling in
    /// the doc rather than a regression.
    /// </summary>
    [Fact]
    public void TheParserAcceptsCSharp13AndNothingPastTheDocumentedCeiling()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Recent.cs", "fixtures/Beyond.cs");

        exitCode.Should().Be(0);
        result.Files.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/Recent.cs", Status = "parsed" },
            new { File = "fixtures/Beyond.cs", Status = "failed" },
        }, o => o.WithStrictOrdering());
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { Name = "Recent.Pick<T>", Signature = "`1(T, bool)", StartLine = 9, EndLine = 9, Complexity = 2 },
            new { Name = "Recent.Reset", Signature = "()", StartLine = 11, EndLine = 11, Complexity = 1 },
        }, o => o.WithStrictOrdering());
    }

    /// <summary>
    /// An interface member with no default implementation, an abstract member or auto-property, an
    /// extern member or auto-property, a field-like event, a bodyless partial method and a primary
    /// constructor on a record or a class carry no lines a developer can branch in, so none of them
    /// gets a span, and the file still parses. An <c>extern</c> local function is the same case, so
    /// the only span the whole file yields is the ordinary method declaring it.
    /// </summary>
    [Fact]
    public void ADeclarationCarryingNoBodyGetsNoSpanAndTheFileStillParses()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Bodyless.cs");

        exitCode.Should().Be(0);
        result.Files.Should().BeEquivalentTo(new[]
        {
            new { File = "fixtures/Bodyless.cs", Status = "parsed" },
        });
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { Name = "Holder.Hold", Signature = "()", StartLine = 43, EndLine = 46, Complexity = 1 },
        });
    }

    /// <summary>
    /// A lambda's branches score against the method holding it, so <c>WithLambda</c>'s <c>&amp;&amp;</c>
    /// counts against the method itself. A local function is its own span, so <c>WithLocal</c> does
    /// not absorb <c>Running</c>'s branches, and a local function declared inside a lambda still
    /// takes the name of the span holding the lambda, not the lambda itself. Nesting a local
    /// function inside another appends again and keeps each one's branches to itself, and a local
    /// function inside a field-initializer lambda has no enclosing span to prefix, so it takes its
    /// bare local name.
    /// </summary>
    [Fact]
    public void ALambdaFoldsIntoItsMethodWhileALocalFunctionIsItsOwnSpan()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Nesting.cs");

        exitCode.Should().Be(0);
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { Name = "Nesting.WithLambda", Signature = "(int[])", StartLine = 9, EndLine = 13, Complexity = 2 },
            new { Name = "Nesting.WithLocal", Signature = "(int[])", StartLine = 15, EndLine = 34, Complexity = 2 },
            new { Name = "Nesting.WithLocal.Running", Signature = "(int)@9", StartLine = 17, EndLine = 25, Complexity = 2 },
            new { Name = "Nesting.LocalInsideLambda", Signature = "(int[])", StartLine = 36, EndLine = 43, Complexity = 1 },
            new { Name = "Nesting.LocalInsideLambda.Twice", Signature = "(int)@13", StartLine = 40, EndLine = 40, Complexity = 2 },
            new { Name = "Nesting.get_Accessed", Signature = "()", StartLine = 47, EndLine = 60, Complexity = 1 },
            new { Name = "Nesting.get_Accessed.Sign", Signature = "(int)@13", StartLine = 49, EndLine = 57, Complexity = 2 },
            new { Name = "Nesting.ThreeDeep", Signature = "(int)", StartLine = 63, EndLine = 96, Complexity = 1 },
            new { Name = "Nesting.ThreeDeep.Outer", Signature = "(int)@9", StartLine = 65, EndLine = 93, Complexity = 2 },
            new { Name = "Nesting.ThreeDeep.Outer.Middle", Signature = "(int)@13", StartLine = 67, EndLine = 85, Complexity = 2 },
            new { Name = "Nesting.ThreeDeep.Outer.Middle.Innermost", Signature = "(int)@17", StartLine = 69, EndLine = 77, Complexity = 2 },
            new { Name = "Detached", Signature = "(int)@9", StartLine = 102, EndLine = 108, Complexity = 2 },
        }, o => o.WithStrictOrdering());
    }

    /// <summary>
    /// An async method and an iterator are scored on the lines they are written on, and a local
    /// function declared inside an async method is still its own span.
    /// </summary>
    [Fact]
    public void AnAsyncMethodAndAnIteratorAreScoredOnTheirOwnSourceLines()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Asynchrony.cs");

        exitCode.Should().Be(0);
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { Name = "Asynchrony.LoadAsync", Signature = "(int)", StartLine = 10, EndLine = 19, Complexity = 2 },
            new { Name = "Asynchrony.Evens", Signature = "(int[])", StartLine = 21, EndLine = 30, Complexity = 3 },
            new { Name = "Asynchrony.StreamAsync", Signature = "(int[])", StartLine = 32, EndLine = 39, Complexity = 2 },
            new { Name = "Asynchrony.RunAsync", Signature = "()", StartLine = 41, EndLine = 52, Complexity = 1 },
            new { Name = "Asynchrony.RunAsync.InnerAsync", Signature = "(int)@9", StartLine = 43, EndLine = 49, Complexity = 2 },
        }, o => o.WithStrictOrdering());
    }

    /// <summary>
    /// A generic method's qualified name carries its own type parameters, and a method on a
    /// generic type carries the type's parameters too, both comma-space separated.
    /// </summary>
    [Fact]
    public void AGenericMethodsQualifiedNameCarriesItsTypeParameters()
    {
        var (exitCode, result) = ExtractorRun.Run("fixtures/Generics.cs");

        exitCode.Should().Be(0);
        result.Spans.Should().BeEquivalentTo(new[]
        {
            new { Name = "Generics<TKey, TValue>.Map<TA, TB>", Signature = "`2(TA, TB)", StartLine = 9, EndLine = 9, Complexity = 1 },
            new { Name = "Generics<TKey, TValue>.Single<T>", Signature = "`1(T)", StartLine = 11, EndLine = 11, Complexity = 1 },
            new { Name = "Generics<TKey, TValue>.Plain", Signature = "(int)", StartLine = 13, EndLine = 13, Complexity = 1 },
        }, o => o.WithStrictOrdering());
    }
}
