using System.Linq;
using System.Reflection;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp.Testing;
using Microsoft.CodeAnalysis.Diagnostics;
using Microsoft.CodeAnalysis.Testing;

namespace Tvrmsmith.Analyzers.Tests;

/// <summary>
/// Builds an expectation from the analyzer's own descriptor, so the assertion covers the
/// formatted message and not just the diagnostic ID.
/// </summary>
internal static class Expect
{
    public static DiagnosticResult Diagnostic(DiagnosticDescriptor descriptor) => new(descriptor);
}

/// <summary>
/// Runs one analyzer over a snippet compiled against the real AwesomeAssertions, the package a
/// consuming repo is expected to pin.
/// </summary>
/// <remarks>
/// Binding against the real package rather than a stub is the point. All three analyzers gate on
/// <c>Should()</c> resolving to an extension method returning an <c>*Assertions</c> type, so a
/// hand-rolled stub could make a test pass against a shape that never occurs.
/// </remarks>
internal static class AssertionAnalyzerTest<TAnalyzer>
    where TAnalyzer : DiagnosticAnalyzer, new()
{
    /// <summary>Types the snippets assert against, appended to every test compilation.</summary>
    /// <remarks>
    /// <c>ApiResponse&lt;T&gt;</c> and its bespoke <c>Should()</c> reproduce the Atlas shape the
    /// escape cast exists to get around: a custom assertions type with no <c>BeEquivalentTo</c>.
    /// </remarks>
    private const string Fixtures = """

        namespace Fixtures
        {
            public sealed class PagedResult
            {
                public int Page { get; set; }
                public int PageSize { get; set; }
                public int TotalResults { get; set; }
                public System.Collections.Generic.IList<Item> Items { get; set; } = new System.Collections.Generic.List<Item>();
            }

            public sealed class Item
            {
                public string Name { get; set; } = "";
                public int Age { get; set; }
            }

            public sealed class Response
            {
                public int StatusCode { get; set; }
                public Headers Headers { get; set; } = new Headers();
            }

            public sealed class Headers
            {
                public System.Uri? Location { get; set; }
            }

            public sealed class Client
            {
                public System.Uri? BaseAddress { get; set; }
            }

            public sealed class ApiResponse<T>
            {
                public int StatusCode { get; set; }
                public T Result { get; set; } = default!;
            }

            public sealed class ApiResponseAssertions<T>
            {
                public ApiResponseAssertions(ApiResponse<T> subject) => Subject = subject;

                public ApiResponse<T> Subject { get; }

                public ApiResponseAssertions<T> HaveStatusCode(int expected) => this;
            }

            public static class ApiResponseExtensions
            {
                public static ApiResponseAssertions<T> Should<T>(this ApiResponse<T> subject) => new(subject);
            }
        }
        """;

    /// <summary>Asserts the analyzer stays silent on a compliant or near-miss snippet.</summary>
    public static Task Silent(string source) => Fires(source);

    /// <summary>
    /// Asserts the analyzer produces exactly the diagnostics the markup in
    /// <paramref name="source"/> and <paramref name="expected"/> describe.
    /// </summary>
    public static Task Fires(string source, params DiagnosticResult[] expected)
    {
        var test = new Test
        {
            TestCode = Preamble + source + Fixtures,
        };

        test.ExpectedDiagnostics.AddRange(expected);
        return test.RunAsync(CancellationToken.None);
    }

    private const string Preamble = """
        #nullable enable
        using System;
        using System.Collections.Generic;
        using AwesomeAssertions;
        using AwesomeAssertions.Execution;
        using Fixtures;

        """;

    /// <summary>
    /// The pinned AwesomeAssertions version, read from assembly metadata the build writes out of
    /// <c>$(AwesomeAssertionsVersion)</c>. Restating the number here would let the version the
    /// snippets bind against drift away from the one the rest of the build resolves.
    /// </summary>
    private static readonly string AssertionsVersion = Assembly.GetExecutingAssembly()
        .GetCustomAttributes<AssemblyMetadataAttribute>()
        .Single(attribute => attribute.Key == "AwesomeAssertionsVersion")
        .Value!;

    private sealed class Test : CSharpAnalyzerTest<TAnalyzer, DefaultVerifier>
    {
        public Test() =>
            ReferenceAssemblies = ReferenceAssemblies.Net.Net80
                .AddPackages([new PackageIdentity("AwesomeAssertions", AssertionsVersion)]);
    }
}
