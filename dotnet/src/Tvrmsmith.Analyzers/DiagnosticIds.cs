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
}
