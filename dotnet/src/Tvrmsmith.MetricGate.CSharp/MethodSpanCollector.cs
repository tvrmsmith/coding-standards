using System.Collections.Generic;
using System.Linq;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;

namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>
/// Walks a parsed file and collects one span per <see cref="MethodDeclarationSyntax"/>. Plain
/// methods only: constructors, properties, accessors, local functions, lambdas, operators, and
/// expression-bodied members other than a plain method never contribute a span. Issue 18 widens
/// this set; this collector does not anticipate it.
/// </summary>
internal sealed class MethodSpanCollector : CSharpSyntaxWalker
{
    private readonly string _file;
    private readonly List<MethodSpanResult> _spans = new();

    public MethodSpanCollector(string file)
    {
        _file = file;
    }

    public IReadOnlyList<MethodSpanResult> Spans => _spans;

    public override void VisitMethodDeclaration(MethodDeclarationSyntax node)
    {
        var lineSpan = node.GetLocation().GetLineSpan();
        var complexityWalker = new ComplexityWalker();
        complexityWalker.Visit(node);

        _spans.Add(new MethodSpanResult(
            _file,
            QualifiedName(node),
            Signature(node),
            lineSpan.StartLinePosition.Line + 1,
            lineSpan.EndLinePosition.Line + 1,
            complexityWalker.Complexity));

        base.VisitMethodDeclaration(node);
    }

    private static string QualifiedName(MethodDeclarationSyntax node)
    {
        var typeNames = node.Ancestors()
            .OfType<TypeDeclarationSyntax>()
            .Reverse()
            .Select(type => type.Identifier.Text);

        return string.Join(".", typeNames.Append(node.Identifier.Text));
    }

    /// <summary>Builds the spelling <see cref="MethodSpanResult"/> documents.</summary>
    private static string Signature(MethodDeclarationSyntax node)
    {
        var arity = node.TypeParameterList?.Parameters.Count ?? 0;
        var prefix = arity > 0 ? "`" + arity : string.Empty;
        var parameters = node.ParameterList.Parameters.Select(Parameter);

        return prefix + "(" + string.Join(", ", parameters) + ")";
    }

    private static string Parameter(ParameterSyntax parameter)
    {
        var modifiers = parameter.Modifiers.Select(modifier => modifier.Text + " ");
        var type = parameter.Type is null
            ? string.Empty
            : parameter.Type.NormalizeWhitespace().ToFullString();

        return string.Concat(modifiers) + type;
    }
}
