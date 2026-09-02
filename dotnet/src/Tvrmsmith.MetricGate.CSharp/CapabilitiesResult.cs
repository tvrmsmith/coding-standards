using System.Collections.Generic;

namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>The <c>--capabilities</c> response: the language this extractor handles, and the
/// file extensions the gate should route to it.</summary>
public sealed record CapabilitiesResult(string Language, IReadOnlyList<string> Extensions);
