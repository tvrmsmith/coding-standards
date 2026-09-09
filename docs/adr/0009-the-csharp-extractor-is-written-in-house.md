# The C# complexity extractor is written in-house, because no existing tool emits a method's end line

**Status:** accepted 2026-09-09. Consolidates
[ADR 0006](0006-the-csharp-extractor-is-written-in-house.md) and its eight amendments. The decision
is unchanged; 0006 holds the reasoning that got here.

## Current rule

The extractor for C# is a `dotnet tool` shipped with `metric-gate`. It reads source paths on stdin,
one per line, parses each file with Roslyn's syntax parser alone, walks the declarations that carry
a body or an expression body, counts decision points for McCabe, and writes JSON on stdout carrying
`{ file, name, signature, startLine, endLine, complexity }` per span plus a per-file parse status.
No semantic model, no MSBuild, no project or solution load, so no restore and no build.

The gate locates it from a built-in table keyed by language and hands it only changed files. Nothing
on `PATH` and no repo config file participates in that lookup.

Each table row carries an extension list, and it decides exactly one thing: whether the gate execs
that binary at all. The list is an upper bound, folded for case. Once the gate does exec,
`--capabilities` is the sole authority on what the extractor handles, and the static list never
filters the paths handed in on stdin.

A declaration gets a span when it carries a body or an expression body, and none when it carries
neither, because a body is exactly what gives a declaration lines to attribute complexity and
coverage to. Methods, constructors, accessors and local functions all qualify. The decision-point
rules live in [`docs/csharp-decision-points.md`](../csharp-decision-points.md), not here, because a
reference table that gets edited as constructs are added needs a home that is not append-only.

## What a broken extractor is not allowed to look like

A gate that scores nothing emits the same `status: pass`, `changed_methods: 0`, exit 0 as a
docs-only commit, so every way an extractor can fail silently is typed instead.

- **`extractor_capabilities_mismatch`**, on two triggers. The `--capabilities` set claims none of
  the paths the static list selected, or the extractor answers for a language other than the one
  the table routed to it. Narrowing to a non-empty subset stays silent, because that is the routing
  `--capabilities` exists to do. The empty intersection is the one case where the gate can prove it
  measured no part of a change it was handed.
- **`extractor_invalid_span`**, on complexity below 1, a start line below 1, or an end line before
  its start. An absent complexity unmarshals to 0 and scores 0; an inverted range makes the method
  vanish from the table. Both otherwise read as a clean pass. The Roslyn extractor emits neither;
  this is defence against a version-skewed or third-party one.
- **`extractor_duplicate_span`**, when two spans share an identity the gate cannot separate.
- **`extractor_path_mismatch`**, when a path comes back other than byte-identical.

Identity is `(file, name, startLine, endLine, signature)`. `signature` exists because
`class C { int F(int x) => 1; int F(string x) => 2; }` is valid C# and Roslyn reports two spans
agreeing on the other four fields. It carries whatever breaks the collisions real code produces: the
parameter spelling, a backtick arity prefix, a local function's start column, a conversion's target
type, an extension block's receiver. Its exact format is recorded in exactly one place, the C#
extractor's `MethodSpanResult` doc comment, and prose elsewhere points at that owner rather than
restating it. An extractor in another language owes the same field, and the obligation is the whole
of what it owes: stable across runs, and different for any two declarations the gate would otherwise
read as one span reported twice.

`signature` is confined to the extractor's JSON wire and never reaches the document, where `name`
remains the whole of a method's printed identity.

## Considered options

**`ComplexityRipper`, as [issue 4](https://github.com/tvrmsmith/coding-standards/issues/4) named
it.** Rejected on its actual CLI. `analyze --root <dir>` takes a directory, filters by regex over
repos, and writes `stats.json` to a file. No file list, no stdin, no per-file parse status, no way
to echo back a path it was handed, so it fails changed-files-only extraction, the parse-status
obligation, and the byte-identical path echo, all three at once.

**Some other existing tool.** A survey found none that emits a method's end line.
`Dependably.CodeMetrics` has the cleanest JSON schema of anything found and its `MethodMetric`
record carries `StartLine` with no `EndLine`. `Crap4DotNet` reports a singular `lineNumber`.
`Roslynator`'s CLI has no metrics command. `sonar-dotnet` is source-available rather than permissive
and NDepend is commercial. The pattern is not an accident: those tools answer "where is this method
declared", which one line serves, while this gate answers "does this line belong to this method",
which needs two. Roslyn hands the second one over in a single call,
`GetLocation().GetLineSpan()`.

**Fork `Dependably.CodeMetrics` to add `EndLine`.** The closest alternative. Rejected because the
inherited walker is the same walker this ADR writes, the fork or the merge wait is permanent
overhead for a one-field change, and it still takes no file list, so every run parses the whole repo.

**Infer each method's end from the next method's start.** Rejected on measurement. A method
occupying lines 55 to 67 and holding a local function at 58 to 64 gets an inferred end of 57,
dropping its own coverage on 66 and 67. The same inference swallows fields, `#region` markers, doc
comments and attributes into the method above, so a touched field marks the preceding method changed
and drags its coverage down.

**Treat an absent extractor as claiming nothing.** Rejected because it silently unscores a real
`.cs` change. The extension list on the table row exists so a docs-only commit does not fail with
`extractor_failed` on a deployment where the extractor is not sitting beside the gate. A table that
over-claims costs a launch that finds nothing; one that under-claims silently unscores real source.
That asymmetry is why a new extension is added to the row before the extractor learns to parse it.

## Consequences

The end line is load-bearing in two places. `cov(m)` is the fraction of instrumentable lines inside
the span that were hit, so the span is the denominator, and
[ADR 0007](0007-changed-method-is-a-span-holding-a-touched-line.md) defines a changed method as a
span holding a touched line, so without an end there is no containment test. Both errors feed the
same formula, and this gate blocks, so a wrong span fails the wrong method with a wrong number.

Owning the walker means owning the decision-point rules. Whether a switch expression arm, a `??=`,
or a pattern `when` clause counts is a line in this repo rather than a disagreement with an opaque
tool. Issue 2 measured exactly that disagreement between coverlet and `dotnet-coverage`, and it is
why the IL route lost in [ADR 0001](0001-crap-gate-topology.md).

Two artifacts ship instead of one: the Go gate, and a `dotnet tool` per .NET target. The target repo
needs the .NET SDK, which any C# repo has.

The extractor wire is JSON, not TOON, though
[ADR 0008](0008-the-machine-document-is-the-only-output.md) makes TOON the gate's own output. No
model reads this seam, so the token saving buys nothing, and requiring TOON would oblige every
future extractor author to find an encoder for their language. Paths go in on stdin rather than argv
because a large rename can exceed `ARG_MAX`.

ADR 0001 decided that complexity comes from a source-level AST walker. This ADR replaces the tool,
not the decision, and ADR 0001 needs no amendment because it never named one.
