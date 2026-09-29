using Microsoft.CodeAnalysis;

namespace Tvrmsmith.Analyzers;

/// <summary>
/// The diagnostic descriptors for the custom rules from the enforcement mapping.
/// </summary>
/// <remarks>
/// All of them default to <see cref="DiagnosticSeverity.Warning"/> and none is ever an
/// error. Adoption is machine-local against code other people wrote and are not being asked to
/// change; an error would break their builds on their machines.
/// <see cref="CommentBlockLength"/> stays a warning for a second reason too: length is a weak
/// proxy for what the comments guideline actually judges, so a report asks for a re-read rather
/// than declaring a defect.
/// </remarks>
internal static class Descriptors
{
    private const string Category = "Tvrmsmith.Assertions";

    private const string CommentCategory = "Tvrmsmith.Comments";

    private const string SkillReferences =
        "https://github.com/tvrmsmith/coding-standards/blob/main/plugins/coding-standards/skills/test-best-practices/references/";

    private const string CommentBlockLengthDocs =
        "https://github.com/tvrmsmith/coding-standards/blob/main/packages/eslint-plugin-tvrmsmith/docs/rules/comment-block-length.md";


    /// <summary>TVRM0001 — <c>combine-assertions-on-same-object</c>.</summary>
    public static readonly DiagnosticDescriptor CombineAssertionsOnSameObject = new(
        id: DiagnosticIds.CombineAssertionsOnSameObject,
        title: "Combine assertions on the same object",
        messageFormat: "Combine these {0} assertions on '{1}' into a single BeEquivalentTo with an anonymous object",
        category: Category,
        defaultSeverity: DiagnosticSeverity.Warning,
        isEnabledByDefault: true,
        description:
            "Back-to-back assertions against one object report only the first failure and read as a "
            + "list of properties rather than an expectation. A single BeEquivalentTo against an "
            + "anonymous object checks them together and names them all when it fails.",
        helpLinkUri: SkillReferences
            + "dotnet-awesome-assertions.md#combining-assertions-beequivalentto-with-anonymous-objects");

    /// <summary>TVRM0002 — <c>no-suppression-before-assertion</c>.</summary>
    public static readonly DiagnosticDescriptor NoSuppressionBeforeAssertion = new(
        id: DiagnosticIds.NoSuppressionBeforeAssertion,
        title: "Do not suppress null before an assertion",
        messageFormat: "'{0}' suppresses null on the value under test before .Should(); assert on the value itself instead",
        category: Category,
        defaultSeverity: DiagnosticSeverity.Warning,
        isEnabledByDefault: true,
        description:
            "'!' turns the null the assertion exists to catch into a NullReferenceException with no "
            + "expectation in the message, and '?.' skips the assertion chain entirely, passing the "
            + "test. Only the receiver chain feeding .Should() is flagged: a suppression in a setup "
            + "precondition, such as Client.BaseAddress! inside an expected value, is legitimate.",
        helpLinkUri: SkillReferences + "dotnet-awesome-assertions.md#null-safety-in-assertions-custom-rule");

    /// <summary>TVRM0003 — <c>no-assertion-escape-cast</c>.</summary>
    public static readonly DiagnosticDescriptor NoAssertionEscapeCast = new(
        id: DiagnosticIds.NoAssertionEscapeCast,
        title: "Do not cast to object to escape a custom assertions type",
        messageFormat: "Casting '{0}' to object only reaches the general assertions overload; assert on the inner value instead",
        category: Category,
        defaultSeverity: DiagnosticSeverity.Warning,
        isEnabledByDefault: true,
        description:
            "When a framework ships its own Should() extension returning a bespoke assertions type "
            + "that lacks BeEquivalentTo, casting to object to reach ObjectAssertions throws away the "
            + "type the framework deliberately gave you and produces failure messages about an "
            + "object. Assert on the inner DTO, and check transport-level concerns separately.",
        helpLinkUri: SkillReferences + "dotnet-atlas.md#never-object-cast-to-escape-the-custom-type-custom-rule");

    /// <summary>TVRM0004 — <c>no-assertion-without-matcher</c>.</summary>
    public static readonly DiagnosticDescriptor NoAssertionWithoutMatcher = new(
        id: DiagnosticIds.NoAssertionWithoutMatcher,
        title: "Assertion has no matcher",
        messageFormat: "'{0}.Should()' checks nothing without a matcher after it",
        category: Category,
        defaultSeverity: DiagnosticSeverity.Warning,
        isEnabledByDefault: true,
        description:
            "Should() on its own builds an assertions object and discards it. Nothing is compared, "
            + "the test passes, and the suite reports coverage of a case it never checked. An "
            + "assertion that never runs is worse than a missing one, because a missing one is "
            + "visible.",
        helpLinkUri: SkillReferences + "dotnet-awesome-assertions.md#assertions-must-actually-execute-custom-rule");

    /// <summary>TVRM0005 — <c>no-dropped-async-assertion</c>.</summary>
    public static readonly DiagnosticDescriptor NoDroppedAsyncAssertion = new(
        id: DiagnosticIds.NoDroppedAsyncAssertion,
        title: "Async assertion is never awaited",
        messageFormat: "'{0}' returns a Task nobody awaits, so the assertion may not run before the test ends",
        category: Category,
        defaultSeverity: DiagnosticSeverity.Warning,
        isEnabledByDefault: true,
        description:
            "An async matcher returns a Task that carries the result. Dropping it ends the test "
            + "before the assertion resolves, and a failure surfaces as an unobserved exception or "
            + "not at all. The compiler's CS4014 catches this inside an async method; this rule "
            + "covers the synchronous test body it cannot see.",
        helpLinkUri: SkillReferences + "dotnet-awesome-assertions.md#assertions-must-actually-execute-custom-rule");

    /// <summary>TVRM0007 <c>no-anonymous-object-comparison</c>.</summary>
    public static readonly DiagnosticDescriptor NoAnonymousObjectComparison = new(
        id: DiagnosticIds.NoAnonymousObjectComparison,
        title: "Do not compare two anonymous objects",
        messageFormat: "Assert these {0} values with one Should() each inside an AssertionScope, not as one anonymous object",
        category: Category,
        defaultSeverity: DiagnosticSeverity.Warning,
        isEnabledByDefault: true,
        description:
            "Bundling unrelated values into an anonymous object so one call can assert them all "
            + "costs the failure message. It reports a structural diff of a throwaway type rather "
            + "than which fact broke, and the expected side needs casts to match member types. One "
            + "Should() per value inside an AssertionScope still reports every failure at once. A "
            + "real object compared with an anonymous expectation is not this shape.",
        helpLinkUri: SkillReferences + "dotnet-awesome-assertions.md#never-bundle-values-into-an-anonymous-subject-custom-rule");

    /// <summary>
    /// TVRM0007 <c>no-anonymous-object-comparison</c>, when every value in the bundle is a member of
    /// one object, read one level deep. Same rule, different fix, so it shares the id and carries
    /// its own advice.
    /// </summary>
    public static readonly DiagnosticDescriptor NoAnonymousObjectComparisonOfOneObject = new(
        id: DiagnosticIds.NoAnonymousObjectComparison,
        title: "Do not compare two anonymous objects",
        messageFormat: "Every value here is read from '{0}', so assert '{0}' itself with one BeEquivalentTo against an anonymous expectation written with the member names of '{0}'",
        category: Category,
        defaultSeverity: DiagnosticSeverity.Warning,
        isEnabledByDefault: true,
        description:
            "Copying members of one object into an anonymous object before comparing adds a "
            + "throwaway type and nothing else. The object is already the subject, so compare it in a "
            + "single BeEquivalentTo with an anonymous expectation that uses the object's own member "
            + "names. BeEquivalentTo checks only the members the expectation names.",
        helpLinkUri: SkillReferences + "dotnet-awesome-assertions.md#never-bundle-values-into-an-anonymous-subject-custom-rule");

    /// <summary>
    /// TVRM0007 <c>no-anonymous-object-comparison</c>, when the bundle holds one value that is not
    /// a member of an object read one level deep. Same rule, different fix, so it shares the id and
    /// carries its own advice.
    /// </summary>
    public static readonly DiagnosticDescriptor NoAnonymousObjectComparisonOfOneValue = new(
        id: DiagnosticIds.NoAnonymousObjectComparison,
        title: "Do not compare two anonymous objects",
        messageFormat: "Assert '{0}' directly with one Should(), not wrapped in an anonymous object",
        category: Category,
        defaultSeverity: DiagnosticSeverity.Warning,
        isEnabledByDefault: true,
        description:
            "Wrapping one value in an anonymous object before comparing adds a throwaway type and "
            + "nothing else. One Should() on the value itself reports the failure directly, and one "
            + "assertion needs no AssertionScope.",
        helpLinkUri: SkillReferences + "dotnet-awesome-assertions.md#never-bundle-values-into-an-anonymous-subject-custom-rule");

    /// <summary>TVRM0006 — <c>comment-block-length</c>.</summary>
    public static readonly DiagnosticDescriptor CommentBlockLength = new(
        id: DiagnosticIds.CommentBlockLength,
        title: "Comment block is over the length budget",
        messageFormat:
            "{0}-line comment block, over the {1}-line budget. Is it justifying the code below it? "
            + "Then fix the code. Is it documenting a contract or an invariant? Then make it a doc "
            // Multi-sentence, so RS1032 requires the trailing period the single-sentence
            // messages above must not have.
            + "comment, which is exempt.",
        category: CommentCategory,
        defaultSeverity: DiagnosticSeverity.Warning,
        isEnabledByDefault: true,
        description:
            "The comments guideline says a paragraph justifying a workaround means the code is "
            + "wrong. Intent is not readable by a compiler, so length stands in as the one "
            + "mechanical proxy, and it is a weak one: read the report as a prompt to re-read the "
            + "code, not a verdict on it. Documentation comments are exempt at any length, because "
            + "the same guideline requires documenting public API contracts, invariants, units and "
            + "side effects.",
        helpLinkUri: CommentBlockLengthDocs);
}
