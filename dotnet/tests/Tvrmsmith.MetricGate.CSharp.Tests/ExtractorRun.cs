using System.Linq;
using System.Text.Json;
using AwesomeAssertions;

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
        var result = JsonSerializer.Deserialize<ExtractionResultDto>(cliResult.StdOut, JsonOptions);
        // What the tool printed is the thing under test, so a stdout that does
        // not deserialize has to be reported here with the text that caused
        // it, not as a NullReferenceException in whichever assertion runs next.
        result.Should().NotBeNull(
            "the tool must print an extraction result on stdout, but it printed {0} (stderr: {1})",
            cliResult.StdOut,
            cliResult.StdErr);
        return (cliResult.ExitCode, result!);
    }
}
