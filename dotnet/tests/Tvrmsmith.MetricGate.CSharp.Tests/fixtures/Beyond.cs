namespace Fixtures;

// C# 14 extension members, the first construct past the documented language ceiling. A consumer's
// own compiler accepts this file; the referenced parser does not, so it comes back failed with no
// spans. When the Microsoft.CodeAnalysis.CSharp reference moves to 4.14.0 or later this file starts
// parsing, which is the signal to move the ceiling section of docs/csharp-decision-points.md.
public static class Beyond
{
    extension(string text)
    {
        public bool IsLong => text.Length > 10;
    }
}
