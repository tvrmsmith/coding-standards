using System;
using System.Collections.Generic;
using System.Linq;
using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.CSharp.Syntax;

namespace Tvrmsmith.MetricGate.CSharp;

/// <summary>
/// Walks a parsed file and collects one span per declaration that carries a body or an expression
/// body: methods, constructors, static constructors, finalizers, operators, conversion operators,
/// property/indexer/event accessors, expression-bodied properties and indexers, and local
/// functions. An auto-property accessor (<c>get;</c>/<c>set;</c> on a concrete property) gets a
/// span too, spanning its own line, since it is the accessor that is trivial, not absent. A
/// declaration with neither gets none, so an interface member with no default implementation, an
/// abstract or extern member, a field-like event, a partial method with no implementation and a
/// primary constructor never contribute one. A lambda or anonymous method gets no span of its own;
/// its
/// decision points are scored by whichever span holds it, per <see cref="ComplexityWalker"/>.
///
/// A local function's name is its containing span's name, then <c>.</c>, then the local name. A
/// local function declared inside a lambda takes the name of the span holding the lambda, and
/// nested local functions append again. <c>_nameStack</c> tracks "the span currently being
/// collected" for exactly this purpose; a lambda never pushes onto it.
/// </summary>
internal sealed class MethodSpanCollector : CSharpSyntaxWalker
{
    private readonly string _file;
    private readonly List<MethodSpanResult> _spans = new();
    private readonly Stack<string> _nameStack = new();

    public MethodSpanCollector(string file)
    {
        _file = file;
    }

    public IReadOnlyList<MethodSpanResult> Spans => _spans;

    public override void VisitMethodDeclaration(MethodDeclarationSyntax node)
    {
        if (node.Body is null && node.ExpressionBody is null)
        {
            return;
        }

        var name = QualifiedName(node, WithTypeParameters(node.Identifier.Text, node.TypeParameterList));
        RecordSpan(node, name, Signature(node.ParameterList, node.TypeParameterList));
        Descend(name, () => base.VisitMethodDeclaration(node));
    }

    public override void VisitConstructorDeclaration(ConstructorDeclarationSyntax node)
    {
        if (node.Body is null && node.ExpressionBody is null)
        {
            return;
        }

        var isStatic = node.Modifiers.Any(SyntaxKind.StaticKeyword);
        var name = QualifiedName(node, isStatic ? ".cctor" : ".ctor");
        RecordSpan(node, name, Signature(node.ParameterList));
        Descend(name, () => base.VisitConstructorDeclaration(node));
    }

    public override void VisitDestructorDeclaration(DestructorDeclarationSyntax node)
    {
        if (node.Body is null && node.ExpressionBody is null)
        {
            return;
        }

        var name = QualifiedName(node, "Finalize");
        RecordSpan(node, name, "()");
        Descend(name, () => base.VisitDestructorDeclaration(node));
    }

    public override void VisitOperatorDeclaration(OperatorDeclarationSyntax node)
    {
        if (node.Body is null && node.ExpressionBody is null)
        {
            return;
        }

        var name = QualifiedName(node, OperatorName(node));
        RecordSpan(node, name, Signature(node.ParameterList));
        Descend(name, () => base.VisitOperatorDeclaration(node));
    }

    public override void VisitConversionOperatorDeclaration(ConversionOperatorDeclarationSyntax node)
    {
        if (node.Body is null && node.ExpressionBody is null)
        {
            return;
        }

        var name = QualifiedName(node, ConversionOperatorName(node));
        RecordSpan(node, name, Signature(node.ParameterList));
        Descend(name, () => base.VisitConversionOperatorDeclaration(node));
    }

    public override void VisitPropertyDeclaration(PropertyDeclarationSyntax node)
    {
        if (node.ExpressionBody is null)
        {
            base.VisitPropertyDeclaration(node);
            return;
        }

        var name = QualifiedName(node, "get_" + node.Identifier.Text);
        RecordSpan(node, name, "()");
        Descend(name, () => base.VisitPropertyDeclaration(node));
    }

    public override void VisitIndexerDeclaration(IndexerDeclarationSyntax node)
    {
        if (node.ExpressionBody is null)
        {
            base.VisitIndexerDeclaration(node);
            return;
        }

        var name = QualifiedName(node, "get_Item");
        RecordSpan(node, name, Signature(node.ParameterList));
        Descend(name, () => base.VisitIndexerDeclaration(node));
    }

    public override void VisitAccessorDeclaration(AccessorDeclarationSyntax node)
    {
        var hasImplementation = node.Body is not null || node.ExpressionBody is not null;
        if (!hasImplementation && !IsConcreteAutoAccessor(node))
        {
            return;
        }

        var (name, signature) = AccessorIdentity(node);
        RecordSpan(node, name, signature);

        if (hasImplementation)
        {
            Descend(name, () => base.VisitAccessorDeclaration(node));
        }
    }

    public override void VisitLocalFunctionStatement(LocalFunctionStatementSyntax node)
    {
        var containerName = _nameStack.Peek();
        var name = containerName + "." + node.Identifier.Text;
        RecordSpan(node, name, Signature(node.ParameterList, node.TypeParameterList));
        Descend(name, () => base.VisitLocalFunctionStatement(node));
    }

    private void Descend(string name, Action visitChildren)
    {
        _nameStack.Push(name);
        visitChildren();
        _nameStack.Pop();
    }

    private void RecordSpan(SyntaxNode node, string name, string signature)
    {
        var lineSpan = node.GetLocation().GetLineSpan();
        var complexityWalker = new ComplexityWalker();
        complexityWalker.Visit(node);

        _spans.Add(new MethodSpanResult(
            _file,
            name,
            signature,
            lineSpan.StartLinePosition.Line + 1,
            lineSpan.EndLinePosition.Line + 1,
            complexityWalker.Complexity));
    }

    private static bool IsConcreteAutoAccessor(AccessorDeclarationSyntax node)
    {
        if (node.Parent?.Parent is not MemberDeclarationSyntax member)
        {
            return false;
        }

        if (member.Modifiers.Any(SyntaxKind.AbstractKeyword) || member.Modifiers.Any(SyntaxKind.ExternKeyword))
        {
            return false;
        }

        return member.Ancestors().OfType<TypeDeclarationSyntax>().FirstOrDefault() is not InterfaceDeclarationSyntax;
    }

    private static (string Name, string Signature) AccessorIdentity(AccessorDeclarationSyntax node)
    {
        var prefix = AccessorPrefix(node.Kind());
        var member = (MemberDeclarationSyntax)node.Parent!.Parent!;

        return member switch
        {
            IndexerDeclarationSyntax indexer =>
                (QualifiedName(indexer, prefix + "Item"), Signature(indexer.ParameterList)),
            PropertyDeclarationSyntax property =>
                (QualifiedName(property, prefix + property.Identifier.Text), "()"),
            EventDeclarationSyntax evt =>
                (QualifiedName(evt, prefix + evt.Identifier.Text), "()"),
            _ => throw new InvalidOperationException($"Unexpected accessor container: {member.GetType()}"),
        };
    }

    private static string AccessorPrefix(SyntaxKind kind) => kind switch
    {
        SyntaxKind.GetAccessorDeclaration => "get_",
        SyntaxKind.SetAccessorDeclaration => "set_",
        SyntaxKind.InitAccessorDeclaration => "init_",
        SyntaxKind.AddAccessorDeclaration => "add_",
        SyntaxKind.RemoveAccessorDeclaration => "remove_",
        _ => throw new InvalidOperationException($"Unexpected accessor kind: {kind}"),
    };

    /// <summary>
    /// Metadata operator names, keyed by operator token. A token this table does not name (a
    /// future language version's operator) falls back to <c>op_</c> plus the token's
    /// <see cref="SyntaxKind"/> name with a trailing <c>Token</c> stripped, so an unrecognized
    /// operator degrades to an ugly but stable name rather than crashing the extractor.
    /// </summary>
    private static string OperatorName(OperatorDeclarationSyntax node)
    {
        var checkedPrefix = node.CheckedKeyword.IsKind(SyntaxKind.CheckedKeyword) ? "Checked" : string.Empty;
        var isUnary = node.ParameterList.Parameters.Count == 1;

        return "op_" + checkedPrefix + OperatorBaseName(node.OperatorToken.Kind(), isUnary);
    }

    private static string ConversionOperatorName(ConversionOperatorDeclarationSyntax node)
    {
        var checkedPrefix = node.CheckedKeyword.IsKind(SyntaxKind.CheckedKeyword) ? "Checked" : string.Empty;
        var kind = node.ImplicitOrExplicitKeyword.IsKind(SyntaxKind.ImplicitKeyword) ? "Implicit" : "Explicit";

        return "op_" + checkedPrefix + kind;
    }

    private static string OperatorBaseName(SyntaxKind tokenKind, bool isUnary) => tokenKind switch
    {
        SyntaxKind.PlusToken => isUnary ? "UnaryPlus" : "Addition",
        SyntaxKind.MinusToken => isUnary ? "UnaryNegation" : "Subtraction",
        SyntaxKind.AsteriskToken => "Multiply",
        SyntaxKind.SlashToken => "Division",
        SyntaxKind.PercentToken => "Modulus",
        SyntaxKind.AmpersandToken => "BitwiseAnd",
        SyntaxKind.BarToken => "BitwiseOr",
        SyntaxKind.CaretToken => "ExclusiveOr",
        SyntaxKind.LessThanLessThanToken => "LeftShift",
        SyntaxKind.GreaterThanGreaterThanToken => "RightShift",
        SyntaxKind.GreaterThanGreaterThanGreaterThanToken => "UnsignedRightShift",
        SyntaxKind.EqualsEqualsToken => "Equality",
        SyntaxKind.ExclamationEqualsToken => "Inequality",
        SyntaxKind.LessThanToken => "LessThan",
        SyntaxKind.GreaterThanToken => "GreaterThan",
        SyntaxKind.LessThanEqualsToken => "LessThanOrEqual",
        SyntaxKind.GreaterThanEqualsToken => "GreaterThanOrEqual",
        SyntaxKind.ExclamationToken => "LogicalNot",
        SyntaxKind.TildeToken => "OnesComplement",
        SyntaxKind.PlusPlusToken => "Increment",
        SyntaxKind.MinusMinusToken => "Decrement",
        SyntaxKind.TrueKeyword => "True",
        SyntaxKind.FalseKeyword => "False",
        _ => FallbackOperatorName(tokenKind),
    };

    private static string FallbackOperatorName(SyntaxKind tokenKind)
    {
        var kindName = tokenKind.ToString();
        return kindName.EndsWith("Token", StringComparison.Ordinal)
            ? kindName[..^"Token".Length]
            : kindName;
    }

    private static string QualifiedName(SyntaxNode node, string memberName)
    {
        var typeNames = node.Ancestors()
            .OfType<TypeDeclarationSyntax>()
            .Reverse()
            .Select(type => WithTypeParameters(type.Identifier.Text, type.TypeParameterList));

        return string.Join(".", typeNames.Append(memberName));
    }

    private static string WithTypeParameters(string identifier, TypeParameterListSyntax? typeParameterList)
    {
        if (typeParameterList is null || typeParameterList.Parameters.Count == 0)
        {
            return identifier;
        }

        var names = typeParameterList.Parameters.Select(p => p.Identifier.Text);
        return identifier + "<" + string.Join(", ", names) + ">";
    }

    /// <summary>Builds the spelling <see cref="MethodSpanResult"/> documents.</summary>
    private static string Signature(BaseParameterListSyntax parameterList, TypeParameterListSyntax? typeParameterList = null)
    {
        var arity = typeParameterList?.Parameters.Count ?? 0;
        var prefix = arity > 0 ? "`" + arity : string.Empty;
        var parameters = parameterList.Parameters.Select(Parameter);

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
