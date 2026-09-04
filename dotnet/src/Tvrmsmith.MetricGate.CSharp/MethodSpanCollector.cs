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
/// its decision points are scored by whichever span holds it, per <see cref="ComplexityWalker"/>.
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
        if (!CarriesLines(node))
        {
            return;
        }

        var name = QualifiedName(
            node,
            WithTypeParameters(node.Identifier.Text, node.TypeParameterList),
            node.ExplicitInterfaceSpecifier);
        RecordSpan(node, name, Signature(node, node.ParameterList, node.TypeParameterList));
        Descend(name, () => base.VisitMethodDeclaration(node));
    }

    public override void VisitConstructorDeclaration(ConstructorDeclarationSyntax node)
    {
        if (!CarriesLines(node))
        {
            return;
        }

        var isStatic = node.Modifiers.Any(SyntaxKind.StaticKeyword);
        var name = QualifiedName(node, isStatic ? ".cctor" : ".ctor");
        RecordSpan(node, name, Signature(node, node.ParameterList));
        Descend(name, () => base.VisitConstructorDeclaration(node));
    }

    public override void VisitDestructorDeclaration(DestructorDeclarationSyntax node)
    {
        if (!CarriesLines(node))
        {
            return;
        }

        var name = QualifiedName(node, "Finalize");
        RecordSpan(node, name, "()");
        Descend(name, () => base.VisitDestructorDeclaration(node));
    }

    public override void VisitOperatorDeclaration(OperatorDeclarationSyntax node)
    {
        if (!CarriesLines(node))
        {
            return;
        }

        var name = QualifiedName(node, OperatorName(node));
        RecordSpan(node, name, Signature(node, node.ParameterList));
        Descend(name, () => base.VisitOperatorDeclaration(node));
    }

    public override void VisitConversionOperatorDeclaration(ConversionOperatorDeclarationSyntax node)
    {
        if (!CarriesLines(node))
        {
            return;
        }

        var name = QualifiedName(node, ConversionOperatorName(node));
        RecordSpan(node, name, Signature(node, node.ParameterList) + ":" + TypeName(node.Type));
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
        RecordSpan(node, name, Signature(node, parameterList: null));
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
        RecordSpan(node, name, Signature(node, node.ParameterList));
        Descend(name, () => base.VisitIndexerDeclaration(node));
    }

    public override void VisitAccessorDeclaration(AccessorDeclarationSyntax node)
    {
        if (node.Parent?.Parent is not BasePropertyDeclarationSyntax member)
        {
            return;
        }

        var hasImplementation = CarriesLines(node);
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
        if (!CarriesLines(node))
        {
            return;
        }

        var localName = WithTypeParameters(node.Identifier.Text, node.TypeParameterList);
        var name = _nameStack.Count == 0 ? localName : _nameStack.Peek() + "." + localName;
        var signature = Signature(node, node.ParameterList, node.TypeParameterList) + StartColumnSuffix(node);
        RecordSpan(node, name, signature);
        Descend(name, () => base.VisitLocalFunctionStatement(node));
    }

    /// <summary>
    /// The span rule, asked once per declaration kind that can carry lines of its own: the
    /// compiler gives a declaration lines to measure when it carries a body or an expression body.
    /// A property or indexer has no body of its own, so its visitor asks only whether it carries an
    /// expression body and otherwise leaves the question to each of its accessors, which is what
    /// the overload below answers. The single carve-out is <see cref="IsConcreteAutoAccessor"/>,
    /// which gives a bodyless <c>get;</c> a span because the compiler synthesizes one for it.
    /// </summary>
    private static bool CarriesLines(BaseMethodDeclarationSyntax node) =>
        node.Body is not null || node.ExpressionBody is not null;

    /// <inheritdoc cref="CarriesLines(BaseMethodDeclarationSyntax)"/>
    private static bool CarriesLines(LocalFunctionStatementSyntax node) =>
        node.Body is not null || node.ExpressionBody is not null;

    /// <inheritdoc cref="CarriesLines(BaseMethodDeclarationSyntax)"/>
    private static bool CarriesLines(AccessorDeclarationSyntax node) =>
        node.Body is not null || node.ExpressionBody is not null;

    private void Descend(string name, Action visitChildren)
    {
        _nameStack.Push(name);
        visitChildren();
        _nameStack.Pop();
    }

    private void RecordSpan(SyntaxNode node, string name, string signature)
    {
        var lineSpan = node.GetLocation().GetLineSpan();

        _spans.Add(new MethodSpanResult(
            _file,
            name,
            signature,
            lineSpan.StartLinePosition.Line + 1,
            lineSpan.EndLinePosition.Line + 1,
            ComplexityWalker.Score(node)));
    }

    /// <summary>
    /// Whether a bodyless <c>get;</c>/<c>set;</c> is one the compiler synthesizes a body for, which
    /// is the single carve-out to the body-or-expression-body span rule. An <c>abstract</c> or
    /// <c>extern</c> member declares an accessor the compiler does not fill in, so neither earns a
    /// span. An interface member is the same case, except when it is <c>static</c>, since a static
    /// auto-property on an interface has been legal and compiler-synthesized since C# 11. A partial
    /// property is declared twice and only its implementing half is measured, so
    /// <see cref="IsPartialImplementation"/> keeps one member from earning two pairs of rows.
    /// </summary>
    private static bool IsConcreteAutoAccessor(BasePropertyDeclarationSyntax member)
    {
        if (member.Modifiers.Any(SyntaxKind.AbstractKeyword)
            || member.Modifiers.Any(SyntaxKind.ExternKeyword))
        {
            return false;
        }

        if (member.Modifiers.Any(SyntaxKind.PartialKeyword) && !IsPartialImplementation(member))
        {
            return false;
        }

        if (member.Modifiers.Any(SyntaxKind.StaticKeyword))
        {
            return true;
        }

        return member.Ancestors().OfType<TypeDeclarationSyntax>().FirstOrDefault() is not InterfaceDeclarationSyntax;
    }

    /// <summary>
    /// Which half of a partial property a declaration is, read off the declaration itself rather
    /// than off the order the two halves appear in. The language says the defining half is the one
    /// whose accessors are all semicolons and which carries no initializer, so any accessor body or
    /// an initializer marks the implementing half. An implementing half written as a plain
    /// auto-property carries an initializer, since that is the only spelling C# accepts for one.
    /// </summary>
    private static bool IsPartialImplementation(BasePropertyDeclarationSyntax member)
    {
        var anyAccessorHasBody = member.AccessorList?.Accessors
            .Any(accessor => accessor.Body is not null || accessor.ExpressionBody is not null) ?? false;

        return anyAccessorHasBody
            || (member as PropertyDeclarationSyntax)?.Initializer is not null;
    }

    private static (string Name, string Signature) AccessorIdentity(
        AccessorDeclarationSyntax node,
        BasePropertyDeclarationSyntax member)
    {
        var prefix = AccessorPrefix(node.Kind());

        var (suffix, ownParameters) = member switch
        {
            IndexerDeclarationSyntax indexer => ("Item", indexer.ParameterList),
            PropertyDeclarationSyntax property => (property.Identifier.Text, null),
            EventDeclarationSyntax evt => (evt.Identifier.Text, (BaseParameterListSyntax?)null),
            _ => throw new InvalidOperationException($"Unexpected accessor container: {member.GetType()}"),
        };

        var signature = Signature(member, ownParameters);

        return (QualifiedName(member, prefix + suffix, member.ExplicitInterfaceSpecifier), signature);
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

        return "op_" + checkedPrefix
            + OperatorBaseName(node.OperatorToken.Kind(), node.ParameterList.Parameters.Count);
    }

    private static string ConversionOperatorName(ConversionOperatorDeclarationSyntax node)
    {
        var checkedPrefix = node.CheckedKeyword.IsKind(SyntaxKind.CheckedKeyword) ? "Checked" : string.Empty;
        var kind = node.ImplicitOrExplicitKeyword.IsKind(SyntaxKind.ImplicitKeyword) ? "Implicit" : "Explicit";

        return "op_" + checkedPrefix + kind;
    }

    /// <summary>
    /// Metadata operator names, keyed by operator token. Three tokens name a different operator
    /// depending on how many parameters the declaration takes, which is what
    /// <paramref name="parameterCount"/> decides: <c>+</c> and <c>-</c> are unary at one parameter
    /// and binary at two, and <c>++</c>/<c>--</c> are the C# 14 instance compound form at zero
    /// parameters and the classic static form at one. The C# 14 compound assignment operators,
    /// <c>+=</c> through <c>&gt;&gt;&gt;=</c>, are instance-only and take an <c>Assignment</c>
    /// suffix. A token this table does not name (a future language version's operator) falls back
    /// to the token's <see cref="SyntaxKind"/> name with a trailing <c>Token</c> stripped, so an
    /// unrecognized operator degrades to an ugly but stable name rather than crashing the
    /// extractor.
    /// </summary>
    private static string OperatorBaseName(SyntaxKind tokenKind, int parameterCount) => tokenKind switch
    {
        SyntaxKind.PlusToken => parameterCount == 1 ? "UnaryPlus" : "Addition",
        SyntaxKind.MinusToken => parameterCount == 1 ? "UnaryNegation" : "Subtraction",
        SyntaxKind.PlusPlusToken => parameterCount == 0 ? "IncrementAssignment" : "Increment",
        SyntaxKind.MinusMinusToken => parameterCount == 0 ? "DecrementAssignment" : "Decrement",
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
        SyntaxKind.TrueKeyword => "True",
        SyntaxKind.FalseKeyword => "False",
        SyntaxKind.PlusEqualsToken => "AdditionAssignment",
        SyntaxKind.MinusEqualsToken => "SubtractionAssignment",
        SyntaxKind.AsteriskEqualsToken => "MultiplicationAssignment",
        SyntaxKind.SlashEqualsToken => "DivisionAssignment",
        SyntaxKind.PercentEqualsToken => "ModulusAssignment",
        SyntaxKind.AmpersandEqualsToken => "BitwiseAndAssignment",
        SyntaxKind.BarEqualsToken => "BitwiseOrAssignment",
        SyntaxKind.CaretEqualsToken => "ExclusiveOrAssignment",
        SyntaxKind.LessThanLessThanEqualsToken => "LeftShiftAssignment",
        SyntaxKind.GreaterThanGreaterThanEqualsToken => "RightShiftAssignment",
        SyntaxKind.GreaterThanGreaterThanGreaterThanEqualsToken => "UnsignedRightShiftAssignment",
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
    /// <remarks>
    /// A C# 14 <c>extension(T receiver)</c> block is a <see cref="TypeDeclarationSyntax"/> with an
    /// empty identifier, so it is dropped rather than contributing a segment. Keeping it would
    /// spell <c>Beyond..get_IsLong</c>, whose doubled dot reads as the <c>..ctor</c> convention.
    /// Two extension blocks in one type can then declare the same member name, and writing both on
    /// one line makes their line ranges agree too; <see cref="ExtensionReceiver"/> is what keeps
    /// the two spans distinct.
    /// </remarks>
    private static string QualifiedName(
        SyntaxNode node,
        string memberName,
        ExplicitInterfaceSpecifierSyntax? explicitInterface = null)
    {
        var typeNames = node.Ancestors()
            .OfType<TypeDeclarationSyntax>()
            .Reverse()
            .Where(type => !string.IsNullOrEmpty(type.Identifier.Text))
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
    private static string Signature(
        SyntaxNode declaration,
        BaseParameterListSyntax? parameterList,
        TypeParameterListSyntax? typeParameterList = null)
    {
        var arity = typeParameterList?.Parameters.Count ?? 0;
        var prefix = arity > 0 ? "`" + arity : string.Empty;
        var own = parameterList?.Parameters.Select(Parameter) ?? Enumerable.Empty<string>();
        var parameters = ExtensionReceiver(declaration).Concat(own);

        return prefix + "(" + string.Join(", ", parameters) + ")";
    }

    /// <summary>
    /// The receiver parameter of the C# 14 <c>extension</c> block a member is declared directly in,
    /// or nothing when it is declared anywhere else. The compiler emits an instance extension member
    /// as a static method whose first parameter is the receiver, so prepending it is what metadata
    /// already spells, and it is also the coordinate that tells two extension blocks apart. An
    /// extension block contributes no name segment, so <c>M.Big</c> declared in
    /// <c>extension(string t)</c> and <c>M.Big</c> declared in <c>extension(int n)</c> agree on name
    /// and, if written on one line, on line range too; the receiver is the only thing left that
    /// differs. A <c>static</c> member of an extension block takes no receiver, and the compiler
    /// rejects two of them that would otherwise collide, so excluding it loses no identity.
    /// </summary>
    private static IEnumerable<string> ExtensionReceiver(SyntaxNode declaration)
    {
        if (declaration.Parent is not ExtensionBlockDeclarationSyntax block
            || declaration is not MemberDeclarationSyntax member
            || member.Modifiers.Any(SyntaxKind.StaticKeyword))
        {
            return Enumerable.Empty<string>();
        }

        return block.ParameterList?.Parameters.Select(Parameter) ?? Enumerable.Empty<string>();
    }

    private static string Parameter(ParameterSyntax parameter)
    {
        var modifiers = parameter.Modifiers.Select(modifier => modifier.Text + " ");
        var type = parameter.Type is null ? string.Empty : TypeName(parameter.Type);

        return string.Concat(modifiers) + type;
    }

    private static string TypeName(TypeSyntax type) => type.NormalizeWhitespace().ToFullString();

    /// <summary>
    /// The start column, 1-based, which is what tells two local functions apart when nothing else
    /// does. Sibling scopes can declare the same local name with the same parameters on one line,
    /// as in <c>{ int L() =&gt; 1; } { int L() =&gt; 2; }</c>, and every other part of a span's
    /// identity then matches. Column is the coordinate the identity is missing, and it moves only
    /// when the declaration's own line is rewritten, unlike a count of how many spans came before.
    /// </summary>
    private static string StartColumnSuffix(SyntaxNode node)
    {
        var column = node.GetLocation().GetLineSpan().StartLinePosition.Character + 1;

        return "@" + column;
    }
}
