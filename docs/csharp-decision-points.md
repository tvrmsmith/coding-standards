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
| `foreach` | |
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
| boolean `&` / `\|` | counts identically to `&&`/`\|\|` when the operand type is `bool`, per the same `BinaryOperator` check | not scored | neither operator short-circuits, so both sides always evaluate and there is no fork to count. Roslyn scores them because it keys on the operator's kind rather than on whether control can skip an operand |

## Spans

Owning the walker means owning which declarations get a row, not only which constructs score inside one.

A declaration gets a span when it carries a body or an expression body. It gets none when it carries
neither. So an interface member with no default implementation, an `abstract` or `extern` member, a
field-like event, a bodyless partial method, and a `record` or `class` primary constructor all produce
no row, while an auto-property accessor does, because the compiler synthesizes a body for it.

A lambda's body counts inside the span of the method, accessor, or other declaration holding it. A
local function is its own span: its lines are not counted twice against the container. That is the
absorption [ADR 0001](adr/0001-crap-gate-topology.md) calls out for coverage; the same rule now applies
to complexity.

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
| Conversion operator | `Order.op_Implicit` |
| Local function | `Order.Total.Running` (container name, dot, local name) |

A generic name's printed form carries a comma, and this document's own tables carry it unquoted because
the delimiter is a pipe. That is the reason [ADR 0005](adr/0005-the-machine-document-is-the-only-output.md)
chose the pipe over TOON's comma default: a comma delimiter would force `Order.Map<TKey, TValue>` to be
quoted, while pipe never needs to quote it.
