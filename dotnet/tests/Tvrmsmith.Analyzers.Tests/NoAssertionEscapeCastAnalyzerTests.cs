using System.Threading.Tasks;
using Xunit;
using Verify = Tvrmsmith.Analyzers.Tests.AssertionAnalyzerTest<
    Tvrmsmith.Analyzers.NoAssertionEscapeCastAnalyzer>;

namespace Tvrmsmith.Analyzers.Tests;

/// <summary>TVRM0003 — <c>no-assertion-escape-cast</c>.</summary>
/// <remarks>
/// <c>ApiResponse&lt;T&gt;</c> in the fixtures carries its own <c>Should()</c> returning a bespoke
/// assertions type with no <c>BeEquivalentTo</c> — the Atlas shape the cast exists to escape.
/// </remarks>
public class NoAssertionEscapeCastAnalyzerTests
{
    [Fact]
    public Task FiresOnAnObjectCastReachingTheGeneralOverload() =>
        Verify.Fires(
            """
            public class ResponseTests
            {
                public void Body(ApiResponse<Item> body)
                {
                    ({|#0:(object)body|}).Should().BeEquivalentTo(
                        new { StatusCode = 200, Result = new { Name = "Alice" } },
                        o => o.ExcludingMissingMembers());
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAssertionEscapeCast)
                .WithLocation(0)
                .WithArguments("body"));

    [Fact]
    public Task FiresOnTheAsObjectSpellingOfTheSameEscape() =>
        Verify.Fires(
            """
            public class ResponseTests
            {
                public void Body(ApiResponse<Item> body)
                {
                    ({|#0:body as object|}).Should().BeEquivalentTo(new { StatusCode = 200 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAssertionEscapeCast)
                .WithLocation(0)
                .WithArguments("body"));

    /// <summary>The fix: assert on the inner DTO, and check the transport concern separately.</summary>
    [Fact]
    public Task SilentOnAssertingTheInnerDtoInstead() =>
        Verify.Silent(
            """
            public class ResponseTests
            {
                public void Body(ApiResponse<Item> body)
                {
                    using (new AssertionScope())
                    {
                        body.Should().HaveStatusCode(200);
                        body.Result.Should().BeEquivalentTo(
                            new { Name = "Alice" },
                            o => o.ExcludingMissingMembers());
                    }
                }
            }
            """);

    /// <summary>
    /// A cast to a real type still leaves a meaningful type in the failure message and is a
    /// legitimate way to pick an overload. Only the reach for <c>ObjectAssertions</c> is banned.
    /// </summary>
    [Fact]
    public Task SilentOnACastToSomethingOtherThanObject() =>
        Verify.Silent(
            """
            public class CastTests
            {
                public void Element(IList<Item> items)
                {
                    ((IEnumerable<Item>)items).Should().NotBeEmpty();
                }
            }
            """);

    [Fact]
    public Task SilentOnAnObjectCastThatIsNotFeedingAnAssertion() =>
        Verify.Silent(
            """
            public class CastTests
            {
                public void Boxed(Item item)
                {
                    var boxed = (object)item;
                    boxed.Should().NotBeNull();
                }
            }
            """);
}
