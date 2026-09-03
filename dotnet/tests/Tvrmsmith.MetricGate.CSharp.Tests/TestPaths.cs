using System.IO;
using System.Runtime.CompilerServices;

namespace Tvrmsmith.MetricGate.CSharp.Tests;

/// <summary>
/// Resolves the test project's own directory at compile time, so fixture paths and the child
/// process's working directory don't depend on the current test runner's bin layout.
/// </summary>
internal static class TestPaths
{
    public static string ProjectDirectory([CallerFilePath] string sourceFilePath = "") =>
        Path.GetDirectoryName(sourceFilePath)!;
}
