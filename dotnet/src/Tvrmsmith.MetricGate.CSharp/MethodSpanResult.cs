namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>One measured method span. <c>Name</c> is declaring type names joined by <c>.</c>,
/// then the method name — no namespace, no parameter list. Lines are 1-based and cover the whole
/// declaration, signature through closing brace.</summary>
public sealed record MethodSpanResult(string File, string Name, int StartLine, int EndLine, int Complexity);
