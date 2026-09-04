using AwesomeAssertions;
using Xunit;

namespace Tvrmsmith.MetricGate.CSharp.Tests;

public class CapabilitiesTests
{
    [Fact]
    public void ReportsLanguageAndExtensionsAndExitsZero()
    {
        var result = MetricGateCsharpProcess.Run(stdin: null, "--capabilities");

        result.ExitCode.Should().Be(0);
        result.StdOut.Trim().Should().Be("""{"language":"csharp","extensions":[".cs"]}""");
    }
}
