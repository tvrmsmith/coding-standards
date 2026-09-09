using System.Threading.Tasks;
using Xunit;
using Verify = Tvrmsmith.Analyzers.Tests.AssertionAnalyzerTest<
    Tvrmsmith.Analyzers.NoAssertionWithoutMatcherAnalyzer>;

namespace Tvrmsmith.Analyzers.Tests;

/// <summary>TVRM0004 — <c>no-assertion-without-matcher</c>.</summary>
public class NoAssertionWithoutMatcherAnalyzerTests
{
    [Fact]
    public Task FiresOnAShouldCallWithNothingAfterIt() =>
        Verify.Fires(
            """
            public class ItemTests
            {
                public void Name(Item item)
                {
                    {|#0:item.Name.Should();|}
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAssertionWithoutMatcher)
                .WithLocation(0)
                .WithArguments("item.Name"));

    /// <summary>A custom assertions type is the same shape, and the same silent pass.</summary>
    [Fact]
    public Task FiresOnACustomAssertionsTypeWithNoMatcher() =>
        Verify.Fires(
            """
            public class ResponseTests
            {
                public void Body(ApiResponse<Item> body)
                {
                    {|#0:body.Should();|}
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAssertionWithoutMatcher)
                .WithLocation(0)
                .WithArguments("body"));

    [Fact]
    public Task SilentOnceAMatcherFollows() =>
        Verify.Silent(
            """
            public class ItemTests
            {
                public void Name(Item item)
                {
                    item.Name.Should().Be("Alice");
                }
            }
            """);

    /// <summary>
    /// The assertions object handed to a variable or an argument is still live, so the rule
    /// only looks at the case where the statement itself is the whole expression.
    /// </summary>
    [Fact]
    public Task SilentWhenTheAssertionsObjectIsKept() =>
        Verify.Silent(
            """
            public class ItemTests
            {
                public void Name(Item item)
                {
                    var assertions = item.Name.Should();
                    assertions.Be("Alice");
                }
            }
            """);

    /// <summary>An ordinary no-argument call that happens not to be an assertion.</summary>
    [Fact]
    public Task SilentOnANonAssertionStatement() =>
        Verify.Silent(
            """
            public class ItemTests
            {
                public void Name(Item item)
                {
                    item.Name.Trim();
                }
            }
            """);
}
