using System;
using System.Threading.Tasks;
using AwesomeAssertions;
using Xunit;

namespace Consumer;

/// <summary>
/// One violation per custom rule. Unlike the FAA diagnostics, these ship as warnings already —
/// what the verification proves here is delivery: that the analyzer DLL reaches the compiler
/// through the bare-DLL path and through the nupkg, against a project that references neither.
/// </summary>
public class CustomRuleViolations
{
    // TVRM0001 combine-assertions-on-same-object.
    [Fact]
    public void PropertiesAssertedOneAtATime()
    {
        var page = new Page { Number = 2, Size = 3, Total = 10 };

        page.Number.Should().Be(2);
        page.Size.Should().Be(3);
        page.Total.Should().Be(10);
    }

    // TVRM0002 no-suppression-before-assertion.
    [Fact]
    public void NullSuppressedOnTheValueUnderTest()
    {
        var envelope = new Envelope { Location = new Uri("https://example.test/items/123") };

        envelope.Location!.OriginalString.Should().Contain("/items/123");
    }

    // TVRM0003 no-assertion-escape-cast.
    [Fact]
    public void CastToObjectToEscapeTheCustomAssertionsType()
    {
        var body = new ApiResponse<Page> { StatusCode = 200, Result = new Page { Number = 2 } };

        ((object)body).Should().BeEquivalentTo(new { StatusCode = 200 });
    }

    // TVRM0004 no-assertion-without-matcher.
    [Fact]
    public void AssertionWithNoMatcherAfterIt()
    {
        var page = new Page { Number = 2 };

        page.Number.Should();
    }

    // TVRM0005 no-dropped-async-assertion. Synchronous body on purpose: inside an async method
    // CS4014 would report it, and CS4014 is not on the WarningsNotAsErrors allowlist, so the
    // TreatWarningsAsErrors section below would fail the build for a reason unrelated to delivery.
    [Fact]
    public void AsyncAssertionNobodyAwaits()
    {
        Func<Task> act = () => Task.FromException(new InvalidOperationException());

        act.Should().ThrowAsync<InvalidOperationException>();
    }

    // TVRM0006 comment-block-length. Twelve lines of plain prose, over the ten-line budget, and
    // deliberately the justifying-a-workaround shape the guideline is about rather than anything
    // a doc comment would carry. The block has to be plain // lines: a /// block is exempt at any
    // length, so writing the fixture as documentation would prove nothing.
    // It also has to sit on its own lines rather than trail a statement, because a trailing
    // comment annotates the code beside it and never joins a block.
    // The lines below pad the block past the budget without saying anything the rule cares about,
    // which is the point: the analyzer counts lines, and only the author can judge intent.
    // Padding line one.
    // Padding line two.
    // Padding line three.
    // Padding line four.
    [Fact]
    public void CommentBlockOverTheBudget()
    {
        var page = new Page { Number = 2 };

        page.Number.Should().Be(2);
    }
}

public sealed class Page
{
    public int Number { get; set; }
    public int Size { get; set; }
    public int Total { get; set; }
}

public sealed class Envelope
{
    public Uri? Location { get; set; }
}

/// <summary>Stands in for a framework wrapper type with its own <c>Should()</c>.</summary>
public sealed class ApiResponse<T>
{
    public int StatusCode { get; set; }

    public T Result { get; set; } = default!;
}

public sealed class ApiResponseAssertions<T>
{
    public ApiResponseAssertions(ApiResponse<T> subject) => Subject = subject;

    public ApiResponse<T> Subject { get; }

    /// <summary>Deliberately no <c>BeEquivalentTo</c> — that absence is what tempts the cast.</summary>
    public ApiResponseAssertions<T> HaveStatusCode(int expected) => this;
}

public static class ApiResponseExtensions
{
    public static ApiResponseAssertions<T> Should<T>(this ApiResponse<T> subject) => new(subject);
}
