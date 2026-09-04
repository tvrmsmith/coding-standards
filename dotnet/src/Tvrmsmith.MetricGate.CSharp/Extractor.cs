using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;

namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>
/// Turns a list of source paths into a parse-status row per file plus every method span found,
/// using Roslyn's syntax parser alone. A file is <c>failed</c> when it cannot be read or when its
/// parsed tree carries an error diagnostic; Roslyn's parser is error-tolerant and always returns a
/// tree, so an error diagnostic is the definition of "did not parse". A file whose collection
/// throws is <c>failed</c> too, so one unrecognised file costs its own row and not the batch.
/// </summary>
public static class Extractor
{
    /// <summary>
    /// <see cref="LanguageVersion.Preview"/> is the highest setting the referenced parser exposes,
    /// so this opens every feature that parser implements rather than leaving the default, which is
    /// the newest stable version it knows. It cannot reach past the reference: the
    /// <c>Microsoft.CodeAnalysis.CSharp</c> version in the project file is a hard ceiling on the C#
    /// this tool can read, and a construct newer than that ceiling comes back <c>failed</c> for its
    /// file even though the consumer's own compiler accepts it. That is a failed row rather than a
    /// wrong score, which is the safe direction to be wrong in.
    /// <c>docs/csharp-decision-points.md</c> records where the ceiling currently sits.
    /// </summary>
    private static readonly CSharpParseOptions ParseOptions =
        new CSharpParseOptions(LanguageVersion.Preview);

    public static ExtractionResult Extract(IEnumerable<string> paths)
    {
        var files = new List<FileStatusResult>();
        var spans = new List<MethodSpanResult>();

        foreach (var path in paths)
        {
            string source;
            try
            {
                source = File.ReadAllText(path);
            }
            // ArgumentException and NotSupportedException are the two a malformed
            // path raises rather than a missing one, and they are as much a file the
            // extractor could not read as a permission error is. Letting either
            // escape crashes the process, so the gate gets no JSON at all in place of
            // the one failed row this loop is here to build.
            catch (Exception e) when (e is IOException or UnauthorizedAccessException
                or ArgumentException or NotSupportedException)
            {
                Console.Error.WriteLine($"metric-gate-csharp: {path}: {e.Message}");
                files.Add(new FileStatusResult(path, "failed"));
                continue;
            }

            MethodSpanCollector collector;
            try
            {
                var tree = CSharpSyntaxTree.ParseText(source, ParseOptions);
                var errors = tree.GetDiagnostics()
                    .Where(d => d.Severity == DiagnosticSeverity.Error)
                    .ToList();
                if (errors.Count > 0)
                {
                    // A construct past the language ceiling and a genuinely broken source file both
                    // land here, and only the diagnostic tells the two apart, so it is what the
                    // operator needs to see. First error only: a single unrecognised construct
                    // cascades into dozens, and the first one names it.
                    Console.Error.WriteLine($"metric-gate-csharp: {path}: {errors[0]}");
                    files.Add(new FileStatusResult(path, "failed"));
                    continue;
                }

                collector = new MethodSpanCollector(path);
                collector.Visit(tree.GetRoot());
            }
            // The collector walks a shape of C# it does not recognise by throwing, which is the
            // right answer for the file it is on and the wrong one for every other file in the
            // batch. Letting it escape crashes the process, so the gate gets no JSON at all in
            // place of the one failed row this loop is here to build. The filter keeps that from
            // swallowing a fault that is about the host rather than about this file: a broken
            // Roslyn load or an exhausted heap would otherwise be recorded as "every .cs file in
            // the batch is bad" and still exit 0, so it stays loud and stops the run.
            catch (Exception e) when (e is not (OutOfMemoryException or TypeLoadException
                or BadImageFormatException or FileLoadException or MissingMemberException))
            {
                Console.Error.WriteLine($"metric-gate-csharp: {path}: {e}");
                files.Add(new FileStatusResult(path, "failed"));
                continue;
            }

            files.Add(new FileStatusResult(path, "parsed"));
            spans.AddRange(collector.Spans.OrderBy(s => s.StartLine));
        }

        return new ExtractionResult(files, spans);
    }
}
