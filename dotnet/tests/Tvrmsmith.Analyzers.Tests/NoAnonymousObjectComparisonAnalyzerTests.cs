using System.Threading.Tasks;
using Xunit;
using Verify = Tvrmsmith.Analyzers.Tests.AssertionAnalyzerTest<
    Tvrmsmith.Analyzers.NoAnonymousObjectComparisonAnalyzer>;

namespace Tvrmsmith.Analyzers.Tests;

/// <summary>TVRM0007 <c>no-anonymous-object-comparison</c>.</summary>
public class NoAnonymousObjectComparisonAnalyzerTests
{
    /// <summary>The shape from the review: unrelated facts bundled so one call asserts them all.</summary>
    [Fact]
    public Task FiresOnUnrelatedFactsBundledIntoAnAnonymousSubject() =>
        Verify.Fires(
            """
            public class RejectionTests
            {
                public async Task Rejects(Response thrown, List<string> levels, Func<Task<int>> statusOf)
                {
                    {|#0:new { thrown.StatusCode, LoggedCritical = levels.Contains("Critical"), Status = await statusOf() }|}
                        .Should().BeEquivalentTo(new { StatusCode = 403, LoggedCritical = true, Status = 0 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison)
                .WithLocation(0)
                .WithArguments(3));

    /// <summary>
    /// Anonymous types override <c>Equals</c>, so <c>Be</c> compares the bundle too, and its
    /// failure message is worse than the structural diff.
    /// </summary>
    [Fact]
    public Task FiresOnAnyMatcherComparingTheBundleWithAnotherOne() =>
        Verify.Fires(
            """
            public class ItemTests
            {
                public void Compare(Item item, Response response)
                {
                    {|#0:new { item.Name, response.StatusCode }|}.Should().Be(new { Name = "Alice", StatusCode = 200 });
                    {|#1:new { item.Name, response.StatusCode }|}.Should().NotBeEquivalentTo(new { Name = "Bob", StatusCode = 500 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison).WithLocation(0).WithArguments(2),
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison).WithLocation(1).WithArguments(2));

    [Fact]
    public Task FiresThroughParenthesesAndEquivalencyOptions() =>
        Verify.Fires(
            """
            public class ItemTests
            {
                public void Compare(Item item, Response response)
                {
                    ({|#0:new { item.Name, response.StatusCode }|}).Should().BeEquivalentTo(
                        new { Name = "Alice", StatusCode = 200 },
                        o => o.WithStrictOrdering(),
                        "both facts describe the same call");
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison).WithLocation(0).WithArguments(2));

    /// <summary>The rewrite the diagnostic asks for.</summary>
    [Fact]
    public Task SilentOnOneAssertionPerFactInsideAnAssertionScope() =>
        Verify.Silent(
            """
            public class RejectionTests
            {
                public async Task Rejects(Response thrown, List<string> levels, Func<Task<int>> statusOf)
                {
                    using (new AssertionScope())
                    {
                        thrown.StatusCode.Should().Be(403);
                        levels.Should().Contain("Critical");
                        (await statusOf()).Should().Be(0);
                    }
                }
            }
            """);

    /// <summary>The pattern the combine-assertions guidance recommends: a real subject.</summary>
    [Fact]
    public Task SilentOnARealObjectComparedWithAnAnonymousExpectation() =>
        Verify.Silent(
            """
            public class PageTests
            {
                public void Page(PagedResult result)
                {
                    result.Should().BeEquivalentTo(
                        new { Page = 2, PageSize = 3, TotalResults = 10 },
                        o => o.ExcludingMissingMembers());
                }
            }
            """);

    [Fact]
    public Task SilentOnAnAnonymousRequestBodySentToAnHttpClient() =>
        Verify.Silent(
            """
            using System.Net;
            using System.Net.Http;
            using System.Net.Http.Json;

            public class CreateTests
            {
                public async Task Creates(HttpClient client)
                {
                    var response = await client.PostAsJsonAsync("/items", new { Name = "Alice", Age = 30 });

                    response.StatusCode.Should().Be(HttpStatusCode.Created);
                }
            }
            """);

    /// <summary>Only a comparison with two anonymous sides is the bundle; one side is a real value.</summary>
    [Fact]
    public Task SilentOnAnAnonymousSubjectComparedWithARealObject() =>
        Verify.Silent(
            """
            public class ItemTests
            {
                public void Compare(Item item, Item expected)
                {
                    new { item.Name, item.Age }.Should().BeEquivalentTo(expected);
                }
            }
            """);

    /// <summary>A projection of a real collection is a subject with its own identity, not a bundle.</summary>
    [Fact]
    public Task SilentOnAProjectedCollectionComparedWithAnonymousItems() =>
        Verify.Silent(
            """
            using System.Linq;

            public class ItemTests
            {
                public void Names(PagedResult result)
                {
                    result.Items.Select(i => new { i.Name }).Should().BeEquivalentTo(new[]
                    {
                        new { Name = "Alice" },
                        new { Name = "Bob" },
                    });
                }
            }
            """);

    /// <summary>An anonymous subject whose matcher takes no anonymous value compares nothing to a bundle.</summary>
    [Fact]
    public Task SilentOnAnAnonymousSubjectWithANonComparingMatcher() =>
        Verify.Silent(
            """
            public class ItemTests
            {
                public void Present(Item item)
                {
                    new { item.Name }.Should().NotBeNull();
                }
            }
            """);
}
