using System.Collections.Generic;

namespace Fixtures;

public class Overloads
{
    public int F(int x) => 1; public int F(string x) => 2;

    public int G(ref int a, out string b, params int[] rest) { b = ""; return a + rest.Length; }

    public int H(Dictionary<string,   int> map, int[]? xs, (int x, string y) pair) => map.Count;

    public int I<T>(T value) => 1;

    // The rest of the modifier list MethodSpanResult documents, plus the two things it says never
    // appear: a parameter attribute and a default value.
    public int J(in int a, ref readonly int b, scoped System.Span<int> c) => a;

    public int K([System.Diagnostics.CodeAnalysis.AllowNull] string s, int n = 5) => n;
}

public static class OverloadsExtensions
{
    public static int L(this Overloads self, in int a) => a;
}
