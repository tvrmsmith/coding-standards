using System;
using System.Diagnostics;
using System.IO;
using System.Threading.Tasks;

namespace Tvrmsmith.MetricGate.CSharp.Tests;

/// <summary>Runs the built tool as a child process — the only seam under test.</summary>
internal static class MetricGateCsharpProcess
{
    private static readonly string DllPath =
        Path.Combine(AppContext.BaseDirectory, "Tvrmsmith.MetricGate.CSharp.dll");

    public static CliResult Run(string? stdin, params string[] args)
    {
        var startInfo = new ProcessStartInfo("dotnet")
        {
            WorkingDirectory = TestPaths.ProjectDirectory(),
            RedirectStandardInput = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            UseShellExecute = false,
        };
        startInfo.ArgumentList.Add(DllPath);
        foreach (var arg in args)
        {
            startInfo.ArgumentList.Add(arg);
        }

        using var process = Process.Start(startInfo)
            ?? throw new InvalidOperationException("Failed to start metric-gate-csharp process.");

        if (stdin is not null)
        {
            process.StandardInput.Write(stdin);
        }

        process.StandardInput.Close();

        // Both pipes are drained concurrently. Reading stdout to EOF first
        // would hang the moment the child wrote more to stderr than its pipe
        // buffer holds, because it would block writing while nobody read.
        var stdOutTask = process.StandardOutput.ReadToEndAsync();
        var stdErrTask = process.StandardError.ReadToEndAsync();
        Task.WaitAll(stdOutTask, stdErrTask);
        process.WaitForExit();

        var stdOut = stdOutTask.Result;
        var stdErr = stdErrTask.Result;

        return new CliResult(process.ExitCode, stdOut, stdErr);
    }
}

internal sealed record CliResult(int ExitCode, string StdOut, string StdErr);
