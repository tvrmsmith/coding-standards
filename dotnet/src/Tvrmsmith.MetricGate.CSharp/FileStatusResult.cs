namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>Per-file parse outcome. <c>Status</c> is <c>"parsed"</c> or <c>"failed"</c>; a failed
/// file contributes no spans.</summary>
public sealed record FileStatusResult(string File, string Status);
