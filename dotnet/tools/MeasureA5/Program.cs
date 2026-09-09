// Buckets every assertion statement in a target repo by whether the assertion can fail to
// run, which is guideline A5 (Assertions Must Actually Execute) in test-best-practices.
//
// Written because the A5 question in C# is "which of these shapes actually occurs, and often
// enough to earn an analyzer", and the honest answer is a count rather than an intuition. Same
// job measure-a2-residue.mjs does for the TypeScript side.
//
//   dotnet run --project dotnet/tools/MeasureA5 -- <repo-root> [--sample N]
//
// Parses with Roslyn rather than grepping, because every shape here is about the syntactic
// context an assertion sits in, and that is exactly what a regex cannot see.

using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;

var root = args.FirstOrDefault();
if (root is null || !Directory.Exists(root))
{
    Console.Error.WriteLine("usage: MeasureA5 <repo-root> [--sample N]");
    return 1;
}

var sampleSize = 0;
var sampleFlag = Array.IndexOf(args, "--sample");
if (sampleFlag >= 0 && sampleFlag + 1 < args.Length)
{
    int.TryParse(args[sampleFlag + 1], out sampleSize);
}

var files = Directory
    .EnumerateFiles(root, "*.cs", SearchOption.AllDirectories)
    .Where(path => !path.Contains($"{Path.DirectorySeparatorChar}bin{Path.DirectorySeparatorChar}")
                   && !path.Contains($"{Path.DirectorySeparatorChar}obj{Path.DirectorySeparatorChar}"))
    .ToArray();

var counts = new Dictionary<string, int>();
var filesWith = new Dictionary<string, HashSet<string>>();
var samples = new Dictionary<string, List<string>>();
var scanned = 0;
var withAssertions = 0;

foreach (var path in files)
{
    var text = File.ReadAllText(path);
    // Cheap pre-filter: parsing 13k files costs real time, and a file with no Should() call
    // cannot contain any shape this tool looks for.
    if (!text.Contains(".Should()")) continue;

    scanned++;
    var tree = CSharpSyntaxTree.ParseText(text, path: path);
    var unit = tree.GetCompilationUnitRoot();
    var found = false;

    foreach (var statement in unit.DescendantNodes().OfType<ExpressionStatementSyntax>())
    {
        if (statement.Expression is not InvocationExpressionSyntax invocation) continue;
        if (!IsAssertionChain(invocation)) continue;
        found = true;

        foreach (var bucket in Classify(statement, invocation))
        {
            Record(bucket, path, statement);
        }
    }

    if (found) withAssertions++;
}

Console.WriteLine($"files scanned (contain .Should()): {scanned} of {files.Length}");
Console.WriteLine($"files with at least one assertion statement: {withAssertions}");
Console.WriteLine();
Console.WriteLine($"{"bucket",-34} {"hits",8} {"files",8}");
foreach (var (bucket, count) in counts.OrderByDescending(pair => pair.Value))
{
    Console.WriteLine($"{bucket,-34} {count,8} {filesWith[bucket].Count,8}");
}

if (sampleSize > 0)
{
    foreach (var (bucket, lines) in samples)
    {
        Console.WriteLine();
        Console.WriteLine($"--- {bucket} ---");
        foreach (var line in lines.Take(sampleSize)) Console.WriteLine($"  {line}");
    }
}

return 0;

void Record(string bucket, string path, ExpressionStatementSyntax statement)
{
    counts[bucket] = counts.GetValueOrDefault(bucket) + 1;
    if (!filesWith.TryGetValue(bucket, out var set)) filesWith[bucket] = set = new HashSet<string>();
    set.Add(path);

    if (sampleSize <= 0) return;
    var list = samples.TryGetValue(bucket, out var existing) ? existing : samples[bucket] = new List<string>();
    if (list.Count >= sampleSize) return;
    var line = statement.GetLocation().GetLineSpan().StartLinePosition.Line + 1;
    var snippet = statement.ToString().Replace("\n", " ").Replace("\r", "");
    if (snippet.Length > 120) snippet = snippet[..120] + "...";
    list.Add($"{Path.GetRelativePath(root, path)}:{line}  {snippet}");
}

// True when the statement's invocation chain runs through a Should() call, which is what makes
// it an assertion rather than an ordinary call.
static bool IsAssertionChain(ExpressionSyntax expression)
{
    while (true)
    {
        switch (expression)
        {
            case InvocationExpressionSyntax { Expression: MemberAccessExpressionSyntax member } invocation:
                if (member.Name.Identifier.ValueText == "Should" && invocation.ArgumentList.Arguments.Count == 0)
                {
                    return true;
                }
                expression = member.Expression;
                continue;
            case MemberAccessExpressionSyntax member:
                expression = member.Expression;
                continue;
            case ConditionalAccessExpressionSyntax conditional:
                expression = conditional.Expression;
                continue;
            case ParenthesizedExpressionSyntax parenthesized:
                expression = parenthesized.Expression;
                continue;
            default:
                return false;
        }
    }
}

// One statement can land in more than one bucket: an un-awaited async assertion inside an if
// is both. Returning every match keeps the buckets independent, so each one answers "how often
// does this shape occur" rather than "how often does it occur and nothing else does".
static IEnumerable<string> Classify(ExpressionStatementSyntax statement, InvocationExpressionSyntax invocation)
{
    if (IsBareShould(invocation))
    {
        yield return "2-no-matcher";
    }

    if (LastCallName(invocation) is { } name && name.EndsWith("Async", StringComparison.Ordinal))
    {
        // The compiler's CS4014 only reaches an un-awaited call inside an async method, so the
        // two halves are counted apart: one is already enforced, the other is the gap.
        yield return EnclosingMethodIsAsync(statement)
            ? "1-dropped-async (CS4014 covers)"
            : "1-dropped-async (uncovered)";
    }

    var branches = EnclosingBranches(statement).ToHashSet();
    foreach (var construct in branches)
    {
        yield return $"3-branch: {construct}";
    }

    // The sub-bucket the decision turns on. A conditional assertion inside a loop is the
    // data-driven conformance shape: walk a table, assert the rows that qualify. A conditional
    // assertion with no loop around it is a test that took a branch and checked nothing.
    var loops = new[] { "foreach", "loop", "switch" };
    if (!branches.Overlaps(new[] { "if", "else" }) || branches.Overlaps(loops)) yield break;

    yield return "3-branch: if/else outside any loop";

    // Two idioms live inside that bucket and neither is an A5 violation, so they are split out
    // rather than argued away. Both branches asserting means nothing is skipped. An assertion
    // whose subject is the very thing the condition tested is a deliberate failure reporter:
    // reaching it means the test has already lost, and the assertion exists to print why.
    var ifStatement = statement.Ancestors().OfType<IfStatementSyntax>().FirstOrDefault();
    if (ifStatement is null) yield break;

    if (ifStatement.Else is { } elseClause && ContainsAssertion(elseClause) && ContainsAssertion(ifStatement.Statement))
    {
        yield return "3-if: both branches assert";
    }
    else if (SubjectOf(invocation) is { } subject && ifStatement.Condition.ToString().Contains(subject, StringComparison.Ordinal))
    {
        yield return "3-if: subject is the condition (failure reporter)";
    }
    else
    {
        yield return "3-if: residue";
    }
}

// `subject.Should();` and nothing after it. Should() with a trailing member access is a normal
// assertion, so the test is that the statement's own invocation is the Should call itself.
static bool IsBareShould(InvocationExpressionSyntax invocation) =>
    invocation.Expression is MemberAccessExpressionSyntax { Name.Identifier.ValueText: "Should" }
    && invocation.ArgumentList.Arguments.Count == 0;

static bool ContainsAssertion(SyntaxNode node) =>
    node.DescendantNodesAndSelf()
        .OfType<InvocationExpressionSyntax>()
        .Any(call => call.Expression is MemberAccessExpressionSyntax { Name.Identifier.ValueText: "Should" });

// The text left of `.Should()`, which is what the assertion is about.
static string? SubjectOf(ExpressionSyntax expression)
{
    while (true)
    {
        switch (expression)
        {
            case InvocationExpressionSyntax { Expression: MemberAccessExpressionSyntax member } invocation:
                if (member.Name.Identifier.ValueText == "Should" && invocation.ArgumentList.Arguments.Count == 0)
                {
                    return member.Expression.ToString();
                }
                expression = member.Expression;
                continue;
            case MemberAccessExpressionSyntax member:
                expression = member.Expression;
                continue;
            default:
                return null;
        }
    }
}

static string? LastCallName(InvocationExpressionSyntax invocation) =>
    invocation.Expression switch
    {
        MemberAccessExpressionSyntax member => member.Name.Identifier.ValueText,
        _ => null,
    };

static bool EnclosingMethodIsAsync(SyntaxNode node)
{
    foreach (var ancestor in node.Ancestors())
    {
        switch (ancestor)
        {
            case MethodDeclarationSyntax method:
                return method.Modifiers.Any(SyntaxKind.AsyncKeyword);
            case LocalFunctionStatementSyntax local:
                return local.Modifiers.Any(SyntaxKind.AsyncKeyword);
            case AnonymousFunctionExpressionSyntax anonymous:
                return anonymous.AsyncKeyword.IsKind(SyntaxKind.AsyncKeyword);
        }
    }

    return false;
}

// Walks out to the enclosing method, naming each construct that can skip the assertion. Stops
// at a lambda: an assertion inside a callback passed to an assertion helper runs under that
// helper's control, not the test's, so the enclosing if above it says nothing about it.
static IEnumerable<string> EnclosingBranches(SyntaxNode node)
{
    foreach (var ancestor in node.Ancestors())
    {
        switch (ancestor)
        {
            case MethodDeclarationSyntax:
            case LocalFunctionStatementSyntax:
            case AnonymousFunctionExpressionSyntax:
                yield break;
            case IfStatementSyntax:
                yield return "if";
                break;
            case ElseClauseSyntax:
                yield return "else";
                break;
            case CatchClauseSyntax:
                yield return "catch";
                break;
            case ForEachStatementSyntax:
                yield return "foreach";
                break;
            case ForStatementSyntax:
            case WhileStatementSyntax:
            case DoStatementSyntax:
                yield return "loop";
                break;
            case SwitchSectionSyntax:
                yield return "switch";
                break;
        }
    }
}
