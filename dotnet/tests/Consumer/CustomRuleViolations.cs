using System;
using FluentAssertions;
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
