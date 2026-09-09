using System.Threading.Tasks;
using Xunit;
using Verify = Tvrmsmith.Analyzers.Tests.AssertionAnalyzerTest<
    Tvrmsmith.Analyzers.NoDroppedAsyncAssertionAnalyzer>;

namespace Tvrmsmith.Analyzers.Tests;

/// <summary>TVRM0005 — <c>no-dropped-async-assertion</c>.</summary>
public class NoDroppedAsyncAssertionAnalyzerTests
{
    [Fact]
    public Task FiresOnADroppedThrowAsyncInASynchronousTestBody() =>
        Verify.Fires(
            """
            public class ThrowTests
            {
                public void Rejects(Func<Task> act)
                {
                    {|#0:act.Should().ThrowAsync<InvalidOperationException>();|}
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoDroppedAsyncAssertion)
                .WithLocation(0)
                .WithArguments("act.Should().ThrowAsync<InvalidOperationException>()"));

    [Fact]
    public Task FiresOnADroppedNotThrowAsync() =>
        Verify.Fires(
            """
            public class ThrowTests
            {
                public void Succeeds(Func<Task> act)
                {
                    {|#0:act.Should().NotThrowAsync();|}
                }
            }
            """,
            Expect.Diagnostic(Descriptors.NoDroppedAsyncAssertion)
                .WithLocation(0)
                .WithArguments("act.Should().NotThrowAsync()"));

    [Fact]
    public Task SilentOnceTheAssertionIsAwaited() =>
        Verify.Silent(
            """
            public class ThrowTests
            {
                public async Task Rejects(Func<Task> act)
                {
                    await act.Should().ThrowAsync<InvalidOperationException>();
                }
            }
            """);

    /// <summary>
    /// Inside an <c>async</c> method the compiler's CS4014 already reports this, and two
    /// diagnostics on one line is noise, so the rule leaves that case to the compiler. This
    /// harness reports compiler errors only, so CS4014 does not appear in the expectation; what
    /// the test pins is the deferral, that this rule adds nothing here.
    /// </summary>
    [Fact]
    public Task SilentInsideAnAsyncMethodWhereCs4014Reports() =>
        Verify.Silent(
            """
            public class ThrowTests
            {
                public async Task Rejects(Func<Task> act)
                {
                    act.Should().ThrowAsync<InvalidOperationException>();
                    await Task.Yield();
                }
            }
            """);

    /// <summary>A synchronous matcher returns the assertions object, not a Task.</summary>
    [Fact]
    public Task SilentOnASynchronousAssertion() =>
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
}
