using System;
using System.Diagnostics;
using System.IO;

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

        var stdOut = process.StandardOutput.ReadToEnd();
        var stdErr = process.StandardError.ReadToEnd();
        process.WaitForExit();

        return new CliResult(process.ExitCode, stdOut, stdErr);
    }
}

internal sealed record CliResult(int ExitCode, string StdOut, string StdErr);
