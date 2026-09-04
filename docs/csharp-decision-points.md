# C# decision points

The C# extractor computes cyclomatic complexity as base 1 plus one point per decision point it finds
walking a span's syntax. [ADR 0006](adr/0006-the-csharp-extractor-is-written-in-house.md) picked a
definition close to Roslyn's own code-metrics walker, `CodeAnalysisMetricData`, on purpose, not
byte-identical to it. That ADR's Consequences section says owning the walker means owning this list, so
this document is where the list lives, and the second section below names every point where the two
disagree.

The threshold a CRAP score is compared against is set against this extractor's own numbers, never
against Roslyn's, so a delta from Roslyn costs nothing on its own. It only matters once it is written
down, which is what this document is for.

## Decision points

Each of these adds one point.

| Construct | Notes |
| --- | --- |
| `if` | |
| `while` | |
| `do` | |
| `for` | |
| `foreach` | including the deconstructing form, `foreach (var (a, b) in pairs)` |
| a `case` label | constant or pattern |
| a switch expression arm | one point per arm, regardless of its pattern |
| `catch` | one point per catch clause |
| a `when` filter clause | both a `catch` filter and a `case` or arm guard |
| a pattern combinator | `and`, `or` |
| `&&` | |
| `\|\|` | |
| `?:` | |
| `??` | |

None of these add a point, each for its own reason.

| Construct | Reason |
| --- | --- |
| `default:` | not a pattern, and it is the fall-through the other labels already account for |
| a `not` pattern | adds no branch |
| `??=` | a compound assignment, and the walker scores no compound assignment operator |
| `goto` | an unconditional jump, not a fork |
| a `return` other than the last | an exit, not a fork; the walker counts syntactic forks, not control-flow edges |
| `yield return` | an emission point, not a fork |
| `await` | a suspension point, not a fork |

The walker carries two asymmetries on purpose.

A switch expression arm scores uniformly, so an arm whose pattern is the discard `_` counts like any
other arm. `default:` does not, because the case-label rule treats it as the label that catches what
the enumerated labels already didn't, not as a pattern in its own right. The two rules are answering
different questions: the arm rule scores the arm itself, the case-label rule scores the pattern a label
carries.

A `when` guard scores on top of the label or arm it decorates. The guard is a second, independent fork
in the same source line: the label or arm can match while the guard still sends control to the next
case, so a guarded case is two forks sharing one line, not one.

## Deltas from Roslyn

Read from `dotnet/roslyn`'s `MetricsHelper.ComputeCoupledTypesAndComplexityExcludingMemberDecls` and
its local function `hasConditionalLogic`
([`src/RoslynAnalyzers/Utilities/Compiler/CodeMetrics/MetricsHelper.cs`](https://github.com/dotnet/roslyn/blob/main/src/RoslynAnalyzers/Utilities/Compiler/CodeMetrics/MetricsHelper.cs)),
which is the method `CodeAnalysisMetricData` calls to compute cyclomatic complexity. Roslyn walks the
compiler's `IOperation` tree rather than syntax, and adds a point for `OperationKind.CaseClause`,
`Coalesce`, `Conditional`, `ConditionalAccess`, `Loop`, and a `BinaryOperator` whose kind is
`ConditionalAnd`, `ConditionalOr`, or a boolean-typed `And`/`Or`. `Conditional` covers both an `if`
statement and a `?:` expression, since Roslyn's operation tree represents both the same way, and `Loop`
covers `while`, `do`, `for`, and `foreach` alike.

| Construct | Roslyn | This extractor | Reason |
| --- | --- | --- | --- |
| `default:` | counts. `IDefaultCaseClauseOperation` is a `CaseClauseOperation`, and `hasConditionalLogic` returns true for every `OperationKind.CaseClause` regardless of its `CaseKind` | does not count | `default:` is the fall-through the enumerated labels already account for; scoring it would count the same branch twice |
| `catch` | does not count. There is no `OperationKind.CatchClause` case in `hasConditionalLogic` | counts, one point per clause | each `catch` clause is a distinct handler the exception's runtime type selects between, and that selection is a real fork Roslyn's list happens to skip |
| a `when` filter clause | no dedicated point. Only a decision-bearing operator written inside the filter expression, such as `&&`, scores on its own terms | a dedicated point, on top of the label or arm it guards | the guard reasoning above: the guard is an independent fork sharing a line with its label |
| a switch expression arm | not counted. `hasConditionalLogic` has no case for a switch expression arm or for any pattern operation kind | +1 per arm, since [ADR 0006's 2026-09-03 amendment](adr/0006-the-csharp-extractor-is-written-in-house.md) | a twenty-arm switch expression scored complexity 1 under the walker before that amendment, while the same logic written as a `switch` statement scored 20; the amendment closed that blind spot |
| a pattern combinator (`and`, `or`) | not counted. A pattern combinator lowers to a pattern operation, never to `OperationKind.BinaryOperator`, so it never reaches the operator-kind check `hasConditionalLogic` runs | +1 per combinator, same amendment | a recursive pattern's `and`/`or` does the same job `&&`/`||` does in a boolean expression, which the extractor already scores |
| `?.` (conditional access) | counts every `ConditionalAccess` operation | not scored | a real fork this extractor does not yet score, deferred rather than excluded. Issue 18 pinned the scored set rather than widening it, and adding `?.` moves the score of most methods in a real codebase, so it belongs to whichever change also moves the threshold |
| a decision point in a field or property initializer | counts, folded into every constructor of the declaring type, since the compiler emits initializer code into each one | scored nowhere. An initializer is not a declaration with a body, so it gets no span, and no span contains it | a real gap, deferred rather than excluded. Folding an initializer into each constructor needs the extractor to synthesize a body it can see no syntax for, and a type with no declared constructor has nowhere to fold into at all, so the fix belongs to whichever change is willing to invent spans |
| boolean `&` / `\|` | counts identically to `&&`/`\|\|` when the operand type is `bool`, per the same `BinaryOperator` check | not scored | neither operator short-circuits, so both sides always evaluate and there is no fork to count. Roslyn scores them because it keys on the operator's kind rather than on whether control can skip an operand |

## Spans

Owning the walker means owning which declarations get a row, not only which constructs score inside one.

A declaration gets a span when it carries a body or an expression body. It gets none when it carries
neither. So an interface member with no default implementation, an `abstract` or `extern` member, a
field-like event, a bodyless partial method or partial property, an `extern` local function, and a
`record` or `class` primary constructor all produce no row, while an auto-property accessor does,
because the compiler synthesizes a body for it.

That carve-out is keyed on synthesis, not on the `get;` spelling. A `static` auto-property on an
interface has been legal since C# 11 and is synthesized like any other, so the interface exclusion does
not reach it.

A partial property is declared twice, once as a promise and once as an implementation, and only the
implementing half is measured, so one member yields exactly one pair of accessor rows rather than two.
Which half a declaration is gets read off the declaration itself, never off the order the two halves
appear in, following the language's own rule: the defining half is the one whose accessors are all
semicolons and which carries no initializer, so any accessor body or an initializer marks the
implementing half. An implementing half written as a plain auto-property carries an initializer, since
that is the only spelling C# accepts for one.

A lambda gets no span of its own. Its decision points count inside whichever span holds it, and when
no span holds it, in a field or property initializer, they are counted nowhere; the Roslyn deltas table
above records that gap. A local function is its own span wherever it is written, including inside an
initializer lambda, and two separate mechanisms keep that from double counting.

Complexity is excluded in the walker, which stops at a nested local function so its decision points
score against itself alone. Line ranges are not excluded: an emitted local-function span sits fully
inside its container's span, and the gate resolves the overlap downstream with the
smallest-containing-span rule from [ADR 0001](adr/0001-crap-gate-topology.md). A consumer reading this
document must not assume the emitted ranges are disjoint.

Top-level statements are the second deferral, alongside the initializer one the Roslyn deltas table
records. The compiler wraps a file's global statements in a synthesized `<Main>$`, which has no
declaration syntax for the collector to key on, so however many branches those statements carry they
produce no span and score nowhere. The gate then files every changed line among them as `outsideSpans`
in the join, which never gates, so branchy top-level code passes unmeasured. This is deferred rather
than excluded, for the reason the initializer row gives: issue 18 pinned the scored set rather than
widening it, and synthesizing a span for a construct with no declaration syntax moves real numbers, so
it belongs with whichever change also moves the threshold. A local function declared in such a file is
unaffected, since it does have a declaration and does get a span, under its bare name.

A span's printed name follows the CLR's own naming for the member kind:

| Declaration | Printed name |
| --- | --- |
| Method | `Order.Total` |
| Generic method | `Order.Map<TKey, TValue>` |
| Constructor | `Order..ctor` |
| Static constructor | `Order..cctor` |
| Finalizer | `Order.Finalize` |
| Property getter | `Order.get_Id` |
| Property setter | `Order.set_Id` |
| Init-only accessor | `Order.init_Id` |
| Indexer getter | `Order.get_Item` |
| Event add accessor | `Order.add_Changed` |
| Event remove accessor | `Order.remove_Changed` |
| Operator overload | `Order.op_Addition` |
| Conversion operator | `Order.op_Implicit` (the target type lives in the signature, not the name) |
| Local function | `Order.Total.Running` (container name, dot, local name) |
| Generic local function | `Order.Total.Map<T>` (type parameters are part of a name here too) |
| Local function with no container | `Helper` (top-level statements, or an initializer lambda) |
| Explicit interface implementation | `Order.IComparable.CompareTo`, and `Order.IShifted.get_Described` for an accessor |

Every conversion operator on a type shares the name `op_Implicit` or `op_Explicit`, so the name alone
cannot tell two of them apart. The signature carries the target type after a colon to close that,
`(Widened):long` for `explicit operator long(Widened v)`, which is the only place a return type appears
in a signature. `MethodSpanResult`'s doc comment spells the rest of the signature format.

A local function has the same problem for a different reason. Two sibling scopes in one method can
declare the same local name with the same parameters, and if they are written on one line, as in
`{ int L() => 1; } { int L() => 2; }`, then name, signature and line range all match and the gate
rejects the pair as one span reported twice. So a local function's signature appends `@` and its
1-based start column, `(int)@9`. Column is the coordinate the identity was missing, and it moves only
when the declaration's own line is rewritten, unlike a count of how many local functions came before,
which an unrelated edit earlier in the method would churn.

A generic name's printed form carries a comma, and this document's own tables carry it unquoted because
the delimiter is a pipe. That is the reason [ADR 0005](adr/0005-the-machine-document-is-the-only-output.md)
chose the pipe over TOON's comma default: a comma delimiter would force `Order.Map<TKey, TValue>` to be
quoted, while pipe never needs to quote it.

## The language ceiling

**The ceiling is C# 13**, set by the `Microsoft.CodeAnalysis.CSharp` 4.13.0 reference in
`dotnet/src/Tvrmsmith.MetricGate.CSharp/Tvrmsmith.MetricGate.CSharp.csproj`. The extractor asks that
parser for `LanguageVersion.Preview`, which is the highest setting it exposes, so every feature the
referenced parser implements is open. The setting cannot reach past the reference, though. The package
version is the real limit, and raising it is the only way to raise the ceiling.

A construct newer than the ceiling makes its file a `failed` row, even though the consumer's own
compiler accepts it, and a `failed` row stops the run naming that file. So the ceiling is a real
constraint on which repositories the gate can score, not just a note. C# 14 extension members,
`public static class E { extension(string s) { ... } }`, are the first construct past it. Getting them
needs `Microsoft.CodeAnalysis.CSharp` 4.14.0 or later, and that bump also forces two decisions this
extractor has not made: how a member declared inside an `extension` block is qualified, and what to do
about the arity check in `OperatorName`, which reads a binary operator inside an extension block as
unary because the receiver is not in the parameter list.

`fixtures/Recent.cs` and `fixtures/Beyond.cs` pin the ceiling from both sides, so a silent downgrade of
the reference reddens the suite and a bump reddens it too, which is the signal to update this section.
