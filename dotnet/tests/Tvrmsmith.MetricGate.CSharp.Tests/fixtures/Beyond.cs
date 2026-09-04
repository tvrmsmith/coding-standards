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

    // Operator arity inside an extension block reads off the parameter list alone: the receiver is
    // not an operand, so one parameter is unary plus and two is subtraction. Compiling this against
    // the real compiler emits op_UnaryPlus and op_Subtraction, which is what the arity check already
    // produces. An operator is static, so it takes no receiver parameter in its signature either.
    extension(string)
    {
        public static string operator +(string b) => b;

        public static string operator -(string a, string b) => a + b;
    }
}
