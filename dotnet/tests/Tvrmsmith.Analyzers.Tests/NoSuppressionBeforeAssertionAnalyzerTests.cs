using System.Threading.Tasks;
using Xunit;
using Verify = Tvrmsmith.Analyzers.Tests.AssertionAnalyzerTest<
    Tvrmsmith.Analyzers.NoSuppressionBeforeAssertionAnalyzer>;

namespace Tvrmsmith.Analyzers.Tests;

/// <summary>TVRM0002 — <c>no-suppression-before-assertion</c>.</summary>
public class NoSuppressionBeforeAssertionAnalyzerTests
{
    [Fact]
    public Task FiresOnNullForgivingInTheReceiverChain() =>
        Verify.Fires(
            """
            public class LocationTests
            {
                public void Header(Response response)
                {
                    {|#0:response.Headers.Location!|}.OriginalString.Should().Contain("/items/123");
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoSuppressionBeforeAssertion)
                .WithLocation(0)
                .WithArguments("response.Headers.Location!"));

    /// <summary>
    /// The worse of the two: the whole chain short-circuits when <c>Location</c> is null, so the
    /// assertion never runs and the test reports a pass.
    /// </summary>
    [Fact]
    public Task FiresOnConditionalAccessInTheReceiverChain() =>
        Verify.Fires(
            """
            public class LocationTests
            {
                public void Header(Response response)
                {
                    response.Headers.Location{|#0:?|}.OriginalString.Should().Contain("/items/123");
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoSuppressionBeforeAssertion)
                .WithLocation(0)
                .WithArguments("?."));

    [Fact]
    public Task SilentOnTheConstructedExpectedValueThatReplacesThem() =>
        Verify.Silent(
            """
            public class LocationTests
            {
                public void Header(Response response, Item item)
                {
                    response.Headers.Location.Should().Be(new Uri("https://example.test/items/123"));
                }
            }
            """);

    /// <summary>
    /// The exemption the reference calls out by name. <c>BaseAddress</c> is set during setup and
    /// is a precondition of the test, not the value under test — and it sits in the expected
    /// value, not the receiver chain. Flagging every suppression in the file would catch it.
    /// </summary>
    [Fact]
    public Task SilentOnASetupPreconditionSuppressionInsideTheExpectedValue() =>
        Verify.Silent(
            """
            public class LocationTests
            {
                public void Header(Response response, Client client, Item item)
                {
                    response.Headers.Location.Should().Be(new Uri(client.BaseAddress!, "/items/1"));
                }
            }
            """);

    [Fact]
    public Task SilentOnASuppressionOutsideAnyAssertion() =>
        Verify.Silent(
            """
            public class SetupTests
            {
                public void Arrange(Client client, Response response)
                {
                    var baseAddress = client.BaseAddress!;
                    response.StatusCode.Should().Be(200);
                }
            }
            """);

    /// <summary>
    /// A chained assertion sits on top of an earlier <c>Should()</c>. The suppression belongs to
    /// the first link and must be reported once, not once per link.
    /// </summary>
    [Fact]
    public Task FiresOnceWhenTheAssertionChainContinues() =>
        Verify.Fires(
            """
            public class ChainTests
            {
                public void SingleItem(IList<Item>? items)
                {
                    {|#0:items!|}.Should().ContainSingle()
                        .Which.Should().BeEquivalentTo(new { Name = "Alice" });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoSuppressionBeforeAssertion)
                .WithLocation(0)
                .WithArguments("items!"));
}
