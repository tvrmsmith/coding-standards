using System.Threading.Tasks;
using Xunit;
using Verify = Tvrmsmith.Analyzers.Tests.AssertionAnalyzerTest<
    Tvrmsmith.Analyzers.NoAnonymousObjectComparisonAnalyzer>;

namespace Tvrmsmith.Analyzers.Tests;

/// <summary>TVRM0007 <c>no-anonymous-object-comparison</c>.</summary>
public class NoAnonymousObjectComparisonAnalyzerTests
{
    /// <summary>The shape from the review. Unrelated facts are bundled so one call asserts them all.</summary>
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

    /// <summary>
    /// A <c>var</c> local is the only other way to hold an anonymous type, so the rule follows
    /// the local back to the object it was created as.
    /// </summary>
    [Fact]
    public Task FiresOnTheBundleHeldInALocal() =>
        Verify.Fires(
            """
            public class ItemTests
            {
                public void Compare(Item item, Response response)
                {
                    var actual = new { item.Name, response.StatusCode };

                    {|#0:actual|}.Should().BeEquivalentTo(new { Name = "Alice", StatusCode = 200 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison).WithLocation(0).WithArguments(2));

    [Fact]
    public Task FiresOnTheExpectationHeldInALocal() =>
        Verify.Fires(
            """
            public class ItemTests
            {
                public void Compare(Item item, Response response)
                {
                    var expected = new { Name = "Alice", StatusCode = 200 };

                    {|#0:new { item.Name, response.StatusCode }|}.Should().BeEquivalentTo(expected);
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison).WithLocation(0).WithArguments(2));

    [Fact]
    public Task FiresWhenBothSidesAreHeldInLocals() =>
        Verify.Fires(
            """
            public class ItemTests
            {
                public void Compare(Item item, Response response)
                {
                    var actual = new { item.Name, response.StatusCode };
                    var expected = new { Name = "Alice", StatusCode = 200 };

                    {|#0:actual|}.Should().BeEquivalentTo(expected);
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison).WithLocation(0).WithArguments(2));

    /// <summary>
    /// Values copied out of one object need no scope. The object itself is the subject, so the
    /// advice is a single BeEquivalentTo on it, and a renamed member only changes the name the
    /// expectation writes.
    /// </summary>
    [Fact]
    public Task AdvisesOneBeEquivalentToWhenEveryValueIsAMemberOfOneObject() =>
        Verify.Fires(
            """
            public class PageTests
            {
                public void Page(PagedResult result)
                {
                    {|#0:new { result.Page, Size = result.PageSize }|}
                        .Should().BeEquivalentTo(new { Page = 2, Size = 3 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparisonOfOneObject)
                .WithLocation(0)
                .WithArguments("result"));

    /// <summary>
    /// A nested value is not a member of the root, so a BeEquivalentTo on the root would compare
    /// a member it does not have.
    /// </summary>
    [Fact]
    public Task AdvisesAScopeWhenAValueIsNestedInsideTheObject() =>
        Verify.Fires(
            """
            public class PageTests
            {
                public void Page(PagedResult result)
                {
                    {|#0:new { result.Page, Size = result.PageSize, Count = result.Items.Count }|}
                        .Should().BeEquivalentTo(new { Page = 2, Size = 3, Count = 10 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison)
                .WithLocation(0)
                .WithArguments(3));

    /// <summary>Values read through an indexer belong to an element, not to the collection.</summary>
    [Fact]
    public Task AdvisesAScopeWhenValuesAreReadThroughAnIndexer() =>
        Verify.Fires(
            """
            public class ItemTests
            {
                public void First(IList<Item> items)
                {
                    {|#0:new { items[0].Name, items[0].Age }|}.Should().BeEquivalentTo(new { Name = "Alice", Age = 30 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison)
                .WithLocation(0)
                .WithArguments(2));

    [Fact]
    public Task AdvisesAScopeWhenOnlySomeValuesComeFromTheObject() =>
        Verify.Fires(
            """
            public class PageTests
            {
                public void Page(PagedResult result, int count)
                {
                    {|#0:new { result.Page, Count = count }|}.Should().BeEquivalentTo(new { Page = 2, Count = 3 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison)
                .WithLocation(0)
                .WithArguments(2));

    /// <summary>A type has no value to assert, so static members of one type get the scope.</summary>
    [Fact]
    public Task AdvisesAScopeWhenValuesAreStaticMembersOfOneType() =>
        Verify.Fires(
            """
            public class EnvironmentTests
            {
                public void Machine()
                {
                    {|#0:new { Environment.MachineName, Environment.ProcessorCount }|}
                        .Should().BeEquivalentTo(new { MachineName = "ci", ProcessorCount = 4 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison)
                .WithLocation(0)
                .WithArguments(2));

    /// <summary>
    /// <c>this</c> is a value the test can assert, so members read through it get the one-object
    /// advice. The implicit form names no object and gets the scope.
    /// </summary>
    [Fact]
    public Task AdvisesOneBeEquivalentToOnThisOnlyWhenTheBundleNamesIt() =>
        Verify.Fires(
            """
            public class PageTests
            {
                public int Page { get; set; }

                public int Size { get; set; }

                public void Explicit()
                {
                    {|#0:new { this.Page, this.Size }|}.Should().BeEquivalentTo(new { Page = 2, Size = 3 });
                    {|#1:new { Page, Size }|}.Should().BeEquivalentTo(new { Page = 2, Size = 3 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparisonOfOneObject)
                .WithLocation(0)
                .WithArguments("this"),
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison)
                .WithLocation(1)
                .WithArguments(2));

    [Fact]
    public Task AdvisesOneBeEquivalentToForALocalCopiedFromOneObject() =>
        Verify.Fires(
            """
            public class PageTests
            {
                public void Page(PagedResult result)
                {
                    var actual = new { result.Page, result.PageSize };

                    {|#0:actual|}.Should().BeEquivalentTo(new { Page = 2, PageSize = 3 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparisonOfOneObject)
                .WithLocation(0)
                .WithArguments("result"));

    /// <summary>
    /// One value needs no bundle and no scope. The advice is to assert that value directly.
    /// </summary>
    [Fact]
    public Task AdvisesAssertingTheValueDirectlyWhenTheBundleHoldsOne() =>
        Verify.Fires(
            """
            public class CountTests
            {
                public void Count(int count)
                {
                    {|#0:new { Count = count }|}.Should().BeEquivalentTo(new { Count = 3 });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparisonOfOneValue)
                .WithLocation(0)
                .WithArguments("count"));

    /// <summary>A single value read from an object still gets the advice to assert that object.</summary>
    [Fact]
    public Task AdvisesOneBeEquivalentToWhenTheOneValueComesFromAnObject() =>
        Verify.Fires(
            """
            public class ItemTests
            {
                public void Name(Item item)
                {
                    {|#0:new { item.Name }|}.Should().BeEquivalentTo(new { Name = "Alice" });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparisonOfOneObject)
                .WithLocation(0)
                .WithArguments("item"));

    /// <summary>A value reached through a call is not a member of the object, so the advice is a scope.</summary>
    [Fact]
    public Task AdvisesAScopeWhenAValueIsReachedThroughACall() =>
        Verify.Fires(
            """
            using System.Linq;

            public class PageTests
            {
                public void Page(PagedResult result)
                {
                    {|#0:new { result.Page, First = result.Items.First().Name }|}
                        .Should().BeEquivalentTo(new { Page = 2, First = "Alice" });
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoAnonymousObjectComparison)
                .WithLocation(0)
                .WithArguments(2));

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

    /// <summary>The pattern the combine-assertions guidance recommends. The subject is a real object.</summary>
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

    /// <summary>
    /// A local holding one element of a projection has an anonymous type, but it was picked out
    /// of a real object rather than built as a bundle, so following the local finds no creation.
    /// </summary>
    [Fact]
    public Task SilentOnALocalHoldingAProjectedElement() =>
        Verify.Silent(
            """
            using System.Linq;

            public class ItemTests
            {
                public void First(PagedResult result)
                {
                    var first = result.Items.Select(i => new { i.Name, i.Age }).First();

                    first.Should().BeEquivalentTo(new { Name = "Alice", Age = 30 });
                }
            }
            """);

    /// <summary>An empty anonymous object bundles no values, so there is nothing to advise.</summary>
    [Fact]
    public Task SilentOnTwoEmptyAnonymousObjects() =>
        Verify.Silent(
            """
            public class EmptyTests
            {
                public void Empty()
                {
                    new { }.Should().BeEquivalentTo(new { });
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
