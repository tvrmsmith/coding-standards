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
/// collected" for exactly this purpose; a lambda never pushes onto it. A local function with no
/// enclosing span at all, which is what a top-level-statements file or a field or property
/// initializer lambda produces, takes its bare local name with no container prefix.
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

        var name = QualifiedName(
            node,
            WithTypeParameters(node.Identifier.Text, node.TypeParameterList),
            node.ExplicitInterfaceSpecifier);
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

        var name = QualifiedName(node, "get_" + node.Identifier.Text, node.ExplicitInterfaceSpecifier);
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

        var name = QualifiedName(node, "get_Item", node.ExplicitInterfaceSpecifier);
        RecordSpan(node, name, Signature(node.ParameterList));
        Descend(name, () => base.VisitIndexerDeclaration(node));
    }

    public override void VisitAccessorDeclaration(AccessorDeclarationSyntax node)
    {
        if (node.Parent?.Parent is not MemberDeclarationSyntax member)
        {
            return;
        }

        var hasImplementation = node.Body is not null || node.ExpressionBody is not null;
        if (!hasImplementation && !IsConcreteAutoAccessor(member))
        {
            return;
        }

        var (name, signature) = AccessorIdentity(node, member);
        RecordSpan(node, name, signature);

        if (hasImplementation)
        {
            Descend(name, () => base.VisitAccessorDeclaration(node));
        }
    }

    public override void VisitLocalFunctionStatement(LocalFunctionStatementSyntax node)
    {
        var localName = WithTypeParameters(node.Identifier.Text, node.TypeParameterList);
        var name = _nameStack.Count == 0 ? localName : _nameStack.Peek() + "." + localName;
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
        var complexityWalker = new ComplexityWalker(node);
        complexityWalker.Visit(node);

        _spans.Add(new MethodSpanResult(
            _file,
            name,
            signature,
            lineSpan.StartLinePosition.Line + 1,
            lineSpan.EndLinePosition.Line + 1,
            complexityWalker.Complexity));
    }

    /// <summary>
    /// Whether a bodyless <c>get;</c>/<c>set;</c> is one the compiler synthesizes a body for, which
    /// is the single carve-out to the body-or-expression-body span rule. An <c>abstract</c>,
    /// <c>extern</c> or <c>partial</c> member declares an accessor the compiler does not fill in, so
    /// none of them earns a span; a partial property's two declarations would otherwise earn four.
    /// An interface member is the same case, except when it is <c>static</c>, since a static
    /// auto-property on an interface has been legal and compiler-synthesized since C# 11.
    /// </summary>
    private static bool IsConcreteAutoAccessor(MemberDeclarationSyntax member)
    {
        if (member.Modifiers.Any(SyntaxKind.AbstractKeyword)
            || member.Modifiers.Any(SyntaxKind.ExternKeyword)
            || member.Modifiers.Any(SyntaxKind.PartialKeyword))
        {
            return false;
        }

        if (member.Modifiers.Any(SyntaxKind.StaticKeyword))
        {
            return true;
        }

        return member.Ancestors().OfType<TypeDeclarationSyntax>().FirstOrDefault() is not InterfaceDeclarationSyntax;
    }

    private static (string Name, string Signature) AccessorIdentity(
        AccessorDeclarationSyntax node,
        MemberDeclarationSyntax member)
    {
        var prefix = AccessorPrefix(node.Kind());

        return member switch
        {
            IndexerDeclarationSyntax indexer =>
                (QualifiedName(indexer, prefix + "Item", indexer.ExplicitInterfaceSpecifier),
                    Signature(indexer.ParameterList)),
            PropertyDeclarationSyntax property =>
                (QualifiedName(property, prefix + property.Identifier.Text, property.ExplicitInterfaceSpecifier),
                    "()"),
            EventDeclarationSyntax evt =>
                (QualifiedName(evt, prefix + evt.Identifier.Text, evt.ExplicitInterfaceSpecifier), "()"),
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

    /// <summary>
    /// Metadata operator names, keyed by operator token. <c>+</c> and <c>-</c> name a different
    /// operator depending on their arity, which is what <paramref name="isUnary"/> decides. A token
    /// this table does not name (a future language version's operator) falls back to the token's
    /// <see cref="SyntaxKind"/> name with a trailing <c>Token</c> stripped, so an unrecognized
    /// operator degrades to an ugly but stable name rather than crashing the extractor.
    /// </summary>
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

    /// <summary>
    /// Declaring type names outermost first, then the member name. An explicit interface
    /// implementation takes the interface it implements between the two, the way metadata spells
    /// it, so <c>int IA.M()</c> on <c>C</c> reads <c>C.IA.M</c> rather than colliding with the
    /// <c>IB.M</c> declared beside it.
    /// </summary>
    private static string QualifiedName(
        SyntaxNode node,
        string memberName,
        ExplicitInterfaceSpecifierSyntax? explicitInterface = null)
    {
        var typeNames = node.Ancestors()
            .OfType<TypeDeclarationSyntax>()
            .Reverse()
            .Select(type => WithTypeParameters(type.Identifier.Text, type.TypeParameterList));

        var qualified = explicitInterface is null
            ? memberName
            : explicitInterface.Name.NormalizeWhitespace().ToFullString() + "." + memberName;

        return string.Join(".", typeNames.Append(qualified));
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

    /// <summary>Builds the signature spelling <see cref="MethodSpanResult"/> documents.</summary>
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
