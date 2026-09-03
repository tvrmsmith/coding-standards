namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>One measured method span. <c>Name</c> is declaring type names joined by <c>.</c>,
/// then the method name — no namespace, no parameter list. Lines are 1-based and cover the whole
/// declaration, signature through closing brace.
///
/// <para><c>Signature</c> is what tells two overloads declared on the same line apart, since they
/// share <c>Name</c>, <c>StartLine</c> and <c>EndLine</c>. The gate never prints it; it uses it
/// only to decide whether two spans are two methods or the same method reported twice. An
/// extractor for another language must spell it the same way:</para>
///
/// <list type="bullet">
/// <item>a backtick and the type-parameter count when the method declares any, so
/// <c>M&lt;T&gt;(T x)</c> is <c>`1(T)</c>, and nothing when it declares none;</item>
/// <item>then the parameter types in declaration order, comma-space separated, inside
/// parentheses;</item>
/// <item>each parameter is its modifiers in declaration order (<c>ref</c>, <c>out</c>, <c>in</c>,
/// <c>params</c>, <c>readonly</c>, <c>scoped</c>, <c>this</c>) each followed by a space, then the
/// type as written;</item>
/// <item>the type is written exactly as the source spells it, whitespace normalised to one space
/// after each comma and none elsewhere, so <c>Dictionary&lt;string, int&gt;</c>,
/// <c>int[]</c>, <c>int?</c> and <c>(int x, string y)</c> are stable however the source
/// spaced them;</item>
/// <item>parameter names, attributes and default values never appear.</item>
/// </list>
///
/// <para>It is syntactic, never resolved, so <c>List&lt;int&gt;</c> and an alias for it read as
/// different signatures. That is enough for the one question the gate asks of it, because two
/// overloads of one method are spelled by one author in one file.</para></summary>
public sealed record MethodSpanResult(
    string File,
    string Name,
    string Signature,
    int StartLine,
    int EndLine,
    int Complexity);
