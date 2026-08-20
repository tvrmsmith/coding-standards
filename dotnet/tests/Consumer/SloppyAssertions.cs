using System.Collections.Generic;
using System.Linq;
using FluentAssertions;
using Xunit;

namespace Consumer;

/// <summary>
/// Assertions written the way the coding standards say not to. Each one is the trigger for a
/// curated severity; verify-severities.sh asserts they surface as warnings only when the
/// analyzers are injected.
/// </summary>
public class SloppyAssertions
{
    // FAA0001 "Simplify Assertion" — ships as Info, so it is invisible in a normal build.
    // Seeing it as a *warning* is the proof that the .globalconfig reached the compiler,
    // not merely that the analyzer DLL loaded.
    [Fact]
    public void CountCheckInsteadOfAnExpressiveChain()
    {
        var numbers = new List<int> { 1, 2, 3 };

        numbers.Count.Should().Be(3);
        numbers.Count().Should().BePositive();
        numbers.Where(n => n > 1).Should().NotBeEmpty();
    }

    // FAA0002 — a raw Xunit assertion where a FluentAssertions one belongs.
    [Fact]
    public void RawXunitAssert()
    {
        var value = "hello";

        Assert.Equal("hello", value);
        Assert.True(value.Length > 0);
        Assert.NotNull(value);
    }
}
