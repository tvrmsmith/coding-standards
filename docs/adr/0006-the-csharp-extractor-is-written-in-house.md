# The C# complexity extractor is written in-house, because no existing tool emits a method's end line

The extractor for C# is a `dotnet tool` shipped with `metric-gate`. It reads source paths on stdin, one per line,
parses each file with Roslyn's syntax parser alone, walks the method-like declarations, counts decision points for
McCabe, and writes JSON on stdout carrying `{ file, name, startLine, endLine, complexity }` per span plus a
per-file parse status. No semantic model, no MSBuild, no project or solution load, so no restore and no build.

The gate locates it from a built-in table keyed by language, asks it for its extension list with `--capabilities`,
and hands it only changed files. Nothing on `PATH` and no repo config file participates in that lookup.

**Amended 2026-09-02.** Each row of the table also carries an extension list, and it decides exactly one thing:
whether the gate execs that binary at all. Locating and launching the extractor before knowing whether any changed
file could belong to it made a docs-only commit fail with `extractor_failed` wherever the extractor is not sitting
beside the gate, which contradicts [ADR 0003](0003-changed-method-is-a-span-holding-a-touched-line.md)'s rule that
an empty changed-method set exits 0 before resolving any input. `build.sh` ships only the gate binaries and nothing
packages the extractor with them, so that deployment is the likely one rather than the exotic one. The two rejected
alternatives were worse: treating an absent extractor as claiming nothing would silently unscore a real `.cs`
change, and leaving the behaviour while deleting the comment that promised otherwise would keep a gate that refuses
to pass a README edit. The static list never filters the paths handed in on stdin, so once the gate does exec,
`--capabilities` remains the sole authority on what the extractor handles and the closing paragraph below stands
unchanged. A table that over-claims costs a launch that finds nothing; a table that under-claims silently unscores
real source, because a path whose extension no row declares never reaches an extractor at all. That asymmetry is
why the list is an upper bound and why a new extension is added to the row before the extractor learns to parse it.

**Amended 2026-09-02.** Among the paths the static list selects, an extractor whose `--capabilities` set claims
**none** of them fails the run with exit 1 and the typed code `extractor_capabilities_mismatch`. The paragraph
above leaves `--capabilities` free to narrow the selection to nothing, and a gate that scores nothing emits the
same `status: pass`, `changed_methods: 0`, exit 0 as a docs-only commit, so a stale or wrongly built extractor
reads as a clean run. Narrowing to a subset stays silent, because that is the routing `--capabilities` exists to
do. Only the empty intersection is typed, because that is the one case where the gate can prove it measured no
part of a change it was handed.

**Amended 2026-09-02.** The extractor emits a `signature` beside `name` on every span, carrying the parameter
spelling, and the gate uses `(file, name, startLine, endLine, signature)` to decide whether two spans are two
methods or one method reported twice. `class C { int F(int x) => 1; int F(string x) => 2; }` is valid C# and Roslyn reports
two spans agreeing on file, name, start line and end line, so name alone cannot separate them and the gate failed
that input with `extractor_duplicate_span`. `signature` is confined to the extractor's JSON wire and never reaches
the document, where `name` remains the whole of a method's printed identity, so no golden and no consumer moves.
Appending the parameter list to `name` instead was rejected because it changes the `name` cell of every row in
every document to buy disambiguation the reader almost never needs, and ordering the duplicates by position within
their line range was rejected because it leaves the two rows printing the same name with nothing to tell them
apart. An extractor in another language owes the same field, spelled however that language spells a parameter
list, and the obligation is only that it be stable across runs and different for overloads.

**Amended 2026-09-03.** Three further decisions on the extractor seam, each authorized on its own evidence.

**Extension routing folds case.** A touched `Order.CS` matched no row and the run passed with
`changed_methods: 0`, which on macOS and Windows is a file a developer creates without noticing. Folding widens
what the gate offers the extractor, and the asymmetry above says that is the cheap direction, since
`--capabilities` remains the authority on what the binary actually handles.
[ADR 0004](0004-source-paths-are-repo-relative-and-resolved-deterministically.md)'s rejection of case folding does
not reach this: it governs resolving a coverage-report path onto a file on disk, where folding can merge two real
files, while routing only decides whether to launch a process.

**`extractor_capabilities_mismatch` has a second trigger.** Beyond the empty intersection the amendment above
records, an extractor answering `--capabilities` with a language other than the one the table routed to it fails
the same way. A binary sitting under the expected name but answering for another language is a misroute, and the
paragraph above scoped the code to one condition only because the second had not been written yet.

**Span numbers are validated.** A span reporting complexity below 1, a start line below 1, or an end line before
its start fails the run with `extractor_invalid_span`. Both violations otherwise read as a clean pass rather than
an error, an absent complexity unmarshalling to 0 and scoring 0, and an inverted range making the method vanish
from the table. The gate already types four extractor-contract violations on the principle that a broken extractor
must not read as a clean run, so leaving the numbers untyped was the inconsistency. The Roslyn extractor emits
neither; this is defence against a version-skewed or third-party one.

**Amended 2026-09-03.** The complexity contract **counts a switch expression arm and a pattern combinator**, each
at +1, alongside the constructs the walker already scored. A twenty-arm switch expression previously reported
complexity 1 and scored CRAP at or below 2 at any coverage, while the same logic as a `switch` statement scored 20,
so the metric was blind to a construct ordinary C# uses. The walker was also inconsistent with itself, since an
arm's `when` guard scored while the arm it guarded did not. This ADR's Consequences already says whether such an
arm counts is a line in this repo, and this is that line. It is not a widening of the **span** rule: which
declarations get a row is a separate question, answered next.

**Amended 2026-09-03.** The opening's "walks the method-like declarations" describes the finished extractor. The
tracer walks **`MethodDeclarationSyntax` alone**, so no constructor, accessor or local function gets a span, and
the widening lands with [issue 18](https://github.com/tvrmsmith/coding-standards/issues/18). Two consequences
follow while it is deferred, and both are deliberate.
[ADR 0005](0005-the-machine-document-is-the-only-output.md)'s worked document prints an `Order.get_Id` row this
extractor cannot yet produce, and with no local-function span the containing method absorbs a local function's
lines, which is the absorption [ADR 0001](0001-crap-gate-topology.md) calls out. Recorded so a reader meets a dated
deferral rather than reading either as a defect.

**Amended 2026-09-03.** [Issue 18](https://github.com/tvrmsmith/coding-standards/issues/18) landed, and
it retires both named consequences of the "Amended 2026-09-03" deferral paragraph above: the walker now
walks constructors, accessors, and local functions alongside methods, so [ADR 0005](0005-the-machine-document-is-the-only-output.md)'s
worked document's `Order.get_Id` row is producible, and a local function is its own span rather than
being absorbed into its container's line count. The decision-point rules, including the switch
expression arm and pattern combinator points the earlier amendment named and every rule this widened
walker adds, live in [`docs/csharp-decision-points.md`](../csharp-decision-points.md), not here. An ADR
is append-only, and a reference table that gets edited as constructs are added or reconsidered needs a
home that isn't. The span rule that document states, a declaration gets a row when it carries a body or
an expression body and none when it carries neither, is keyed on body-or-expression-body rather than on
declaration kind because a body is exactly what gives a declaration lines to attribute complexity and
coverage to; a declaration without one has nothing for a span to measure.

[ADR 0001](0001-crap-gate-topology.md) decided that complexity comes from a source-level AST walker.
[Issue 4](https://github.com/tvrmsmith/coding-standards/issues/4) named `ComplexityRipper` as the tool filling
that role for .NET today. This ADR replaces the tool, not the decision, and ADR 0001 needs no amendment because it
never named one.

## Considered options

**`ComplexityRipper`, as issue 4 named it.** Rejected on its actual CLI. `analyze --root <dir>` takes a directory
or a directory of repos, filters by regex over repos, and writes `stats.json` to a file. There is no file list, no
stdin, no per-file parse status, and no way to echo back a path it was handed, so it fails
[ADR 0003](0003-changed-method-is-a-span-holding-a-touched-line.md)'s changed-files-only extraction, issue 5's
parse-status obligation, and [ADR 0004](0004-source-paths-are-repo-relative-and-resolved-deterministically.md)'s
byte-identical path echo, all three at once.

**Some other existing tool.** A survey of the field found none that emits a method's end line.
`Dependably.CodeMetrics` (Apache-2.0, active) has the cleanest JSON schema of anything found and its
`MethodMetric` record carries `StartLine` with no `EndLine`, added, per its own commit, so a diagnostic can point
at a `file:line` locus. `Crap4DotNet` (MIT) already computes CRAP and reports a singular `lineNumber`. `lizard`
(Python) is the only candidate that takes a file list and keys its rows `function@startline@file`. `Roslynator`'s
CLI has no metrics command at all. `sonar-dotnet` is source-available rather than permissive and NDepend is
commercial. `Boresight.Analyzer.Tool` ships on NuGet with a repository URL that 404s.

The pattern is not an accident. Every one of those tools answers "where is this method declared", which one line
serves, while this gate answers "does this line belong to this method", which needs two. Roslyn hands the second
one over in a single call, `GetLocation().GetLineSpan()`.

**Fork `Dependably.CodeMetrics` to add `EndLine`.** The only real alternative, and it was close. Rejected because
it buys less than it looks: the inherited walker is the same walker this ADR writes, the fork or the merge wait is
permanent overhead for a one-field change, and it still takes no file list, so every run parses the whole repo.

**Infer each method's end from the next method's start.** Rejected on the measurement in issue 4.
`WithLocal` occupies lines 55 to 67 and holds a local function at 58 to 64, so sorting by start line gives
`WithLocal` an inferred end of 57, dropping its own coverage on 66 and 67 and leaving the smallest-containing-span
rule nothing to disambiguate. The same inference swallows whatever sits between methods, fields, `#region`
markers, doc comments, attributes, nested types, into the method above, so a touched field marks the preceding
method changed and its uncovered lines drag that method's coverage down.

## Consequences

The end line is load-bearing in two places, which is why a start-line-only tool cannot serve. `cov(m)` is the
fraction of instrumentable lines inside the span that were hit, so the span is the denominator. And ADR 0003
defines a changed method as a span holding a touched line, so without an end there is no containment test. Both
errors feed the same formula, and this gate blocks, so a wrong span fails the wrong method with a wrong number.

Owning the walker means owning the decision-point rules. Whether a switch expression arm, a `??=`, or a pattern
`when` clause counts is a line in this repo rather than a disagreement with an opaque tool. Issue 2 measured
exactly that disagreement between coverlet and `dotnet-coverage`, and it is why the IL route lost in ADR 0001.

Two artifacts ship instead of one: the Go gate, and a `dotnet tool` per .NET target. The target repo needs the
.NET SDK, which any C# repo has, and which was equally true of every rejected option.

The extractor wire is JSON, not TOON, though [ADR 0005](0005-the-machine-document-is-the-only-output.md) makes
TOON the gate's own output. No model reads this seam, so the token saving buys nothing, and requiring TOON here
would oblige every future extractor author to find an encoder for their language. Paths go in on stdin rather than
argv because a large rename can exceed `ARG_MAX`, and that failure is rare enough to escape testing.
`--capabilities` is asked of the located binary rather than recorded in the gate's table, because ADR 0003 made
the extension list the extractor's obligation and a table that disagrees with the binary is a silent misroute.
