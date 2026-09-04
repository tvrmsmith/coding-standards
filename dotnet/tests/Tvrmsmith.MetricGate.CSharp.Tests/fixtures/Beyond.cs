namespace Fixtures;

// C# 14 extension members, the newest construct the documented language ceiling covers. This needs
// Microsoft.CodeAnalysis.CSharp 5.x; on 4.x it comes back failed with no spans. The extension block
// contributes no name segment, so the accessor reads "Beyond.get_IsLong" rather than doubling the
// dot into the "..ctor" spelling. See the ceiling section of docs/csharp-decision-points.md.
public static class Beyond
{
    extension(string text)
    {
        public bool IsLong => text.Length > 10;
    }
}
