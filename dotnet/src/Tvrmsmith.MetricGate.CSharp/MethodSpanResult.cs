namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>One measured method span. <c>Name</c> is declaring type names joined by <c>.</c>, then
/// the member name as metadata spells it, so a constructor is <c>Order..ctor</c>, a getter is
/// <c>Order.get_Id</c>, an operator is <c>Order.op_Addition</c> and a local function is its
/// container's name, a dot, then the local name. Type parameters are part of a name, so a generic
/// method reads <c>Order.Map&lt;TKey, TValue&gt;</c> and a generic type qualifies its members as
/// <c>Cache&lt;TKey, TValue&gt;.Get</c>. No namespace and no parameter list. The full table, and
/// which declarations get a span at all, live in <c>docs/csharp-decision-points.md</c>.
///
/// <para>Lines are 1-based and cover the declaration's full syntax, first token to last. For a
/// method with a block that is the signature through the closing brace, for an expression-bodied
/// member it ends at the semicolon, and for an auto-property accessor it is the single line
/// <c>get;</c> sits on.</para>
///
/// <para><c>Signature</c> is what tells two overloads declared on the same line apart, since they
/// share <c>Name</c>, <c>StartLine</c> and <c>EndLine</c>. The gate never prints it; it uses it
/// only to decide whether two spans are two methods or the same method reported twice. An
/// extractor for another language must spell it the same way:</para>
///
/// <list type="bullet">
/// <item>a backtick and the type-parameter count when the declaration declares any, so
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
/// <para>A constructor, finalizer, operator or local function spells its own parameter list the
/// same way a method does. A local function then appends <c>@</c> and its 1-based start column,
/// because two sibling scopes can declare the same local name with the same parameters on one
/// line and nothing else in the identity would differ. A conversion operator spells its parameter
/// list that way and then
/// appends a colon and its target type, so <c>explicit operator long(Widened v)</c> is
/// <c>(Widened):long</c>. Two conversions on one type differ only in that target type, and
/// <c>op_Explicit</c> gives them the same <c>Name</c>, so without it two conversions written on one
/// line would be indistinguishable. A property or event accessor takes <c>()</c>, since
/// the declaration itself carries no parameter list and the implicit <c>value</c> parameter is
/// never spelled. An indexer accessor takes the indexer's own parameter list instead, so
/// <c>get_Item</c> on <c>this[int index]</c> is <c>(int)</c>.</para>
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
