namespace Tvrmsmith.Analyzers;

/// <summary>
/// The diagnostic IDs owned by this package, kept in one place so the analyzers, the
/// release-tracking files and the curated severity config cannot drift apart.
/// </summary>
public static class DiagnosticIds
{
    /// <summary>
    /// <c>combine-assertions-on-same-object</c> — separate assertions against one object should
    /// be a single expressive assertion. The only v1 guideline with no off-the-shelf coverage
    /// in either C# or TypeScript.
    /// </summary>
    public const string CombineAssertionsOnSameObject = "TVRM0001";

    /// <summary>
    /// <c>no-suppression-before-assertion</c> — no <c>!</c> or <c>?.</c> immediately before
    /// <c>.Should()</c>; the suppression hides the failure the assertion exists to catch.
    /// </summary>
    public const string NoSuppressionBeforeAssertion = "TVRM0002";

    /// <summary>
    /// <c>no-assertion-escape-cast</c> — no casting to <c>object</c> to escape a custom
    /// assertions type, e.g. <c>((object)x).Should()</c>.
    /// </summary>
    public const string NoAssertionEscapeCast = "TVRM0003";

    /// <summary>
    /// <c>no-assertion-without-matcher</c> — <c>x.Should();</c> as a whole statement. It builds
    /// an assertions object, checks nothing, and leaves the suite green.
    /// </summary>
    public const string NoAssertionWithoutMatcher = "TVRM0004";

    /// <summary>
    /// <c>no-dropped-async-assertion</c> — an assertion whose Task nobody awaits, in a context
    /// where the compiler's CS4014 cannot see it.
    /// </summary>
    public const string NoDroppedAsyncAssertion = "TVRM0005";

    /// <summary>
    /// <c>comment-block-length</c> — a run of non-documentation comments longer than the budget.
    /// </summary>
    public const string CommentBlockLength = "TVRM0006";
}
