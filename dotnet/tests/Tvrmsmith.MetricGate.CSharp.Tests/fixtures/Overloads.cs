using System.Collections.Generic;

namespace Fixtures;

public class Overloads
{
    public int F(int x) => 1; public int F(string x) => 2;

    public int G(ref int a, out string b, params int[] rest) { b = ""; return a + rest.Length; }

    public int H(Dictionary<string,   int> map, int[]? xs, (int x, string y) pair) => map.Count;

    public int I<T>(T value) => 1;
}
