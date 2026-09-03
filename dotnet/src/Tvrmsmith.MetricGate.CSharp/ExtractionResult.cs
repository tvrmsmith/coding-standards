using System.Collections.Generic;

namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>The tool's stdout shape for a run: one status row per input path, plus every method
/// span found across the parsed files.</summary>
public sealed record ExtractionResult(IReadOnlyList<FileStatusResult> Files, IReadOnlyList<MethodSpanResult> Spans);
