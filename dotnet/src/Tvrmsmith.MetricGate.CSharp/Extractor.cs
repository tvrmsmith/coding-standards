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
/// tree, so an error diagnostic is the definition of "did not parse".
/// </summary>
public static class Extractor
{
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
                files.Add(new FileStatusResult(path, "failed"));
                continue;
            }

            var tree = CSharpSyntaxTree.ParseText(source);
            var hasParseError = tree.GetDiagnostics().Any(d => d.Severity == DiagnosticSeverity.Error);
            if (hasParseError)
            {
                files.Add(new FileStatusResult(path, "failed"));
                continue;
            }

            files.Add(new FileStatusResult(path, "parsed"));

            var collector = new MethodSpanCollector(path);
            collector.Visit(tree.GetRoot());
            spans.AddRange(collector.Spans.OrderBy(s => s.StartLine));
        }

        return new ExtractionResult(files, spans);
    }
}
