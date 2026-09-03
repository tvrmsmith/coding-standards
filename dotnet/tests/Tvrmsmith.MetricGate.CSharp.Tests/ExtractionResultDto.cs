using System.Collections.Generic;

namespace Tvrmsmith.MetricGate.CSharp.Tests;

/// <summary>
/// The tool's stdout JSON, deserialized purely for assertions — a plain data shape the tests read
/// the process's stdout into, independent of whatever DTOs the tool itself uses internally.
/// </summary>
internal sealed record FileStatusDto(string File, string Status);

internal sealed record MethodSpanDto(
    string File,
    string Name,
    string Signature,
    int StartLine,
    int EndLine,
    int Complexity);

internal sealed record ExtractionResultDto(List<FileStatusDto> Files, List<MethodSpanDto> Spans);
