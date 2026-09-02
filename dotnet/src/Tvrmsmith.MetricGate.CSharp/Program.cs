using System;
using System.Collections.Generic;
using System.IO;
using System.Text.Json;

namespace Tvrmsmith.MetricGate.CSharp;

public static class Program
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
    };

    public static int Main(string[] args)
    {
        if (Array.IndexOf(args, "--capabilities") >= 0)
        {
            var capabilities = new CapabilitiesResult("csharp", new[] { ".cs" });
            Console.WriteLine(JsonSerializer.Serialize(capabilities, JsonOptions));
            return 0;
        }

        var paths = ReadPaths(Console.In);
        var result = Extractor.Extract(paths);
        Console.WriteLine(JsonSerializer.Serialize(result, JsonOptions));
        return 0;
    }

    private static List<string> ReadPaths(TextReader input)
    {
        var paths = new List<string>();
        string? line;
        while ((line = input.ReadLine()) is not null)
        {
            if (line.Length > 0)
            {
                paths.Add(line);
            }
        }

        return paths;
    }
}
