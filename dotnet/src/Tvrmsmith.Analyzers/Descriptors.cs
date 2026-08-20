using Microsoft.CodeAnalysis;

namespace Tvrmsmith.Analyzers;

/// <summary>
/// The diagnostic descriptors for the three custom rules from the enforcement mapping.
/// </summary>
/// <remarks>
/// All three default to <see cref="DiagnosticSeverity.Warning"/> and none of them is ever an
/// error. Adoption is machine-local against code other people wrote and are not being asked to
/// change; an error would break their builds on their machines.
/// </remarks>
internal static class Descriptors
{
    private const string Category = "Tvrmsmith.Assertions";

    private const string SkillReferences =
        "https://github.com/tvrmsmith/coding-standards/blob/main/plugins/coding-standards/skills/test-best-practices/references/";

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
        helpLinkUri: SkillReferences + "dotnet-awesome-assertions.md#null-safety-in-assertions--custom-rule");

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
        helpLinkUri: SkillReferences + "dotnet-atlas.md#never-object-cast-to-escape-the-custom-type--custom-rule");
}
