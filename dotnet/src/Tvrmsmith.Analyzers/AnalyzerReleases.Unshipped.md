; Unshipped analyzer release
; https://github.com/dotnet/roslyn-analyzers/blob/main/src/Microsoft.CodeAnalysis.Analyzers/ReleaseTrackingAnalyzers.Help.md

### New Rules

Rule ID | Category | Severity | Notes
--------|----------|----------|-------
TVRM0001 | Tvrmsmith.Assertions | Warning | CombineAssertionsOnSameObjectAnalyzer, [Documentation](https://github.com/tvrmsmith/coding-standards/blob/main/plugins/coding-standards/skills/test-best-practices/references/dotnet-awesome-assertions.md#combining-assertions-beequivalentto-with-anonymous-objects)
TVRM0002 | Tvrmsmith.Assertions | Warning | NoSuppressionBeforeAssertionAnalyzer, [Documentation](https://github.com/tvrmsmith/coding-standards/blob/main/plugins/coding-standards/skills/test-best-practices/references/dotnet-awesome-assertions.md#null-safety-in-assertions--custom-rule)
TVRM0003 | Tvrmsmith.Assertions | Warning | NoAssertionEscapeCastAnalyzer, [Documentation](https://github.com/tvrmsmith/coding-standards/blob/main/plugins/coding-standards/skills/test-best-practices/references/dotnet-atlas.md#never-object-cast-to-escape-the-custom-type--custom-rule)
