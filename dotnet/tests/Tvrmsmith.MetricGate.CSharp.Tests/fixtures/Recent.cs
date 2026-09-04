namespace Fixtures;

// C# 13 syntax the parser has to accept. An "allows ref struct" constraint and an "\e" escape are
// both parse errors under Microsoft.CodeAnalysis.CSharp 4.8, so a downgrade of that reference, or
// a parse pinned to an older language version, turns this file into a failed row with no spans
// rather than passing quietly.
public class Recent
{
    public static T Pick<T>(T a, bool first) where T : allows ref struct => first ? a : a;

    public string Reset() => "\e[0m";
}
