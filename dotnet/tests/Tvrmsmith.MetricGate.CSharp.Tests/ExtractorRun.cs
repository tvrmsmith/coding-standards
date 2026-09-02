using System.Linq;
using System.Text.Json;

namespace Tvrmsmith.MetricGate.CSharp.Tests;

/// <summary>Runs the tool with paths on stdin and parses its stdout, the seam every extraction
/// test drives through.</summary>
internal static class ExtractorRun
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    public static (int ExitCode, ExtractionResultDto Result) Run(params string[] paths)
    {
        var stdin = string.Concat(paths.Select(p => p + "\n"));
        var cliResult = MetricGateCsharpProcess.Run(stdin);
        var result = JsonSerializer.Deserialize<ExtractionResultDto>(cliResult.StdOut, JsonOptions)!;
        return (cliResult.ExitCode, result);
    }
}
