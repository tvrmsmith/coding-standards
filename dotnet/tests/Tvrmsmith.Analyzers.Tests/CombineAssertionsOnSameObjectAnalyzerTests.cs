using System.Threading.Tasks;
using Xunit;
using Verify = Tvrmsmith.Analyzers.Tests.AssertionAnalyzerTest<
    Tvrmsmith.Analyzers.CombineAssertionsOnSameObjectAnalyzer>;

namespace Tvrmsmith.Analyzers.Tests;

/// <summary>TVRM0001 — <c>combine-assertions-on-same-object</c>.</summary>
public class CombineAssertionsOnSameObjectAnalyzerTests
{
    [Fact]
    public Task FiresOnBackToBackPropertyAssertionsAgainstOneObject() =>
        Verify.Fires(
            """
            public class PagingTests
            {
                public void Metadata(PagedResult result)
                {
                    {|#0:result.Page.Should().Be(2);|}
                    {|#1:result.PageSize.Should().Be(3);|}
                    {|#2:result.TotalResults.Should().Be(10);|}
                }
            }
            """,
            Expect.Diagnostic(Descriptors.CombineAssertionsOnSameObject)
                .WithLocation(0)
                .WithLocation(1)
                .WithLocation(2)
                .WithArguments("3", "result"));

    [Fact]
    public Task SilentOnTheSingleBeEquivalentToThatReplacesThem() =>
        Verify.Silent(
            """
            public class PagingTests
            {
                public void Metadata(PagedResult result)
                {
                    result.Should().BeEquivalentTo(
                        new { Page = 2, PageSize = 3, TotalResults = 10 },
                        o => o.ExcludingMissingMembers());
                }
            }
            """);

    [Fact]
    public Task FiresOnCountThenIndex() =>
        Verify.Fires(
            """
            public class CollectionTests
            {
                public void Contents(IList<Item> items)
                {
                    {|#0:items.Should().HaveCount(2);|}
                    {|#1:items[0].Name.Should().Be("Alice");|}
                    {|#2:items[1].Name.Should().Be("Bob");|}
                }
            }
            """,
            Expect.Diagnostic(Descriptors.CombineAssertionsOnSameObject)
                .WithLocation(0)
                .WithLocation(1)
                .WithLocation(2)
                .WithArguments("3", "items"));

    [Fact]
    public Task SilentOnHaveCountAloneWhenOnlyTheCountIsUnderTest() =>
        Verify.Silent(
            """
            public class CollectionTests
            {
                public void OnlyTheCount(PagedResult result)
                {
                    result.Items.Should().HaveCount(2);
                }
            }
            """);

    /// <summary>
    /// The widening. <c>StatusCode</c> and <c>Headers.Location</c> are reached through different
    /// intermediates but are rooted at the same <c>response</c>, and that is enough to group them.
    /// <c>body.Result.Name</c> is rooted elsewhere, so it stays out and the count is 2, not 3.
    /// </summary>
    [Fact]
    public Task FiresOnMembersReachedThroughDifferentIntermediates() =>
        Verify.Fires(
            """
            public class ScopeTests
            {
                public void StatusAndLocation(Response response, ApiResponse<Item> body)
                {
                    {|#0:response.StatusCode.Should().Be(201);|}
                    {|#1:response.Headers.Location.Should().Be(new Uri("https://example.test/items/1"));|}
                    body.Result.Name.Should().Be("Alice");
                }
            }
            """,
            Expect.Diagnostic(Descriptors.CombineAssertionsOnSameObject)
                .WithLocation(0)
                .WithLocation(1)
                .WithArguments("2", "response"));

    /// <summary>
    /// The counterweight to the widening, and the shape the reference documents as correct: under
    /// a scope the author has already said these cannot be combined, so the rule stands down.
    /// </summary>
    [Fact]
    public Task SilentOnThatSamePairInsideAnAssertionScope() =>
        Verify.Silent(
            """
            public class ScopeTests
            {
                public void StatusAndLocation(Response response, ApiResponse<Item> body)
                {
                    using (new AssertionScope())
                    {
                        response.StatusCode.Should().Be(201);
                        response.Headers.Location.Should().Be(new Uri("https://example.test/items/1"));
                        body.Result.Name.Should().Be("Alice");
                    }
                }
            }
            """);

    /// <summary>
    /// The spelling that opens no block of its own, so the assertions sit directly in the method
    /// body alongside the declaration rather than nested under it.
    /// </summary>
    [Fact]
    public Task SilentInsideAScopeOpenedByAUsingDeclaration() =>
        Verify.Silent(
            """
            public class ScopeTests
            {
                public void StatusAndLocation(Response response)
                {
                    using var scope = new AssertionScope();

                    response.StatusCode.Should().Be(201);
                    response.Headers.Location.Should().Be(new Uri("https://example.test/items/1"));
                }
            }
            """);

    /// <summary>
    /// A scope does not excuse count-then-index: <c>BeEquivalentTo</c> against an expected array
    /// always works, so this pair is never one a scope is legitimately holding apart.
    /// </summary>
    [Fact]
    public Task FiresOnCountThenIndexEvenInsideAnAssertionScope() =>
        Verify.Fires(
            """
            public class CollectionTests
            {
                public void Contents(IList<Item> items)
                {
                    using (new AssertionScope())
                    {
                        {|#0:items.Should().HaveCount(1);|}
                        {|#1:items[0].Name.Should().Be("Alice");|}
                    }
                }
            }
            """,
            Expect.Diagnostic(Descriptors.CombineAssertionsOnSameObject)
                .WithLocation(0)
                .WithLocation(1)
                .WithArguments("2", "items"));

    [Fact]
    public Task SilentWhenAStatementRunsBetweenTheAssertions() =>
        Verify.Silent(
            """
            public class SequencedTests
            {
                public void Reassigns(PagedResult result)
                {
                    result.Page.Should().Be(2);
                    result = Next(result);
                    result.PageSize.Should().Be(3);
                }

                private static PagedResult Next(PagedResult current) => current;
            }
            """);

    /// <summary>
    /// Two calls are not one object. Combining them would change how many times the method runs,
    /// so a repeated invocation in the receiver never opens a run.
    /// </summary>
    [Fact]
    public Task SilentWhenTheReceiverIsAMethodCall() =>
        Verify.Silent(
            """
            public class InvocationTests
            {
                public void TwoCalls()
                {
                    Load().Page.Should().Be(2);
                    Load().PageSize.Should().Be(3);
                }

                private static PagedResult Load() => new PagedResult();
            }
            """);

    [Fact]
    public Task SilentOnASingleExpressiveChain() =>
        Verify.Silent(
            """
            public class ChainTests
            {
                public void SingleItem(IList<Item> items)
                {
                    items.Should().ContainSingle()
                        .Which.Should().BeOfType<Item>()
                        .Which.Should().BeEquivalentTo(new { Name = "Alice" });
                }
            }
            """);
}
