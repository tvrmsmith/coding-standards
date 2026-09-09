# Context

Glossary for the coding-standards repo. Terms only, no implementation detail.

## Mode

An **enforcement mechanism**, distinguished by when it fires and what kind of judgement it applies.
Guidelines are assigned to modes, but a mode exists independently of the mapping: a mode with no
guideline behind it is legitimate, in the same way an off-the-shelf lint rule earns its place
without a guideline being written for it first.

Do not define a mode as "a home for a guideline". That reading makes a guideline-free mode
incoherent, and the repo already has guideline-free enforcement.

The modes:

| Mode | Fires | Judgement |
| --- | --- | --- |
| skill-as-guidance | before the code is written | human/agent, from prose |
| lint | after | mechanical, deterministic shape match, from source alone |
| skill-as-review | after | human/agent, from prose |
| metric-gate | after | mechanical, measured quantity against a threshold, requires executing the tests |

## Guideline

A single rule of the personal standards, carrying an id (`A1`, `D11`, `F10`). Every guideline is
assigned to exactly one mode. The mapping runs one way: a guideline needs a mode, a mode's rules do
not need a guideline.

## metric-gate

The mode that compares a **measured quantity** against a threshold. Two properties separate it from
lint, and it takes both together:

- lint decides from source alone and is free; a metric-gate must execute the tests.
- lint matches a deterministic shape; a metric-gate compares a number against a threshold.

Together these explain its two awkward properties: it is slow, and it can fail because someone
deleted a test rather than because someone changed the code.

## CRAP score

Change Risk Anti-Patterns score, per **method**: `comp(m)² × (1 − cov(m))³ + comp(m)`, where `comp`
is cyclomatic complexity and `cov` is that method's coverage fraction, 0 to 1.

At 0% coverage it reduces to `comp² + comp`; at 100% coverage it reduces to `comp`. So a threshold
is best read as the complexity tolerated in a wholly untested method: threshold 30 permits
complexity 5 untested, 18 at half coverage, 30 fully covered.

The measured unit is a method, while the rest of the harness scopes by file.

## Method span

The identity of a method for measurement purposes: its file path plus its first and last source
line. Both the complexity number and the coverage number are attributed to a span, never to a
method name. Where spans nest, a source line belongs to the smallest span containing it.

Two methods can share one span, since `int F(int x) => 1; int F(string x) => 2;` on one line is
valid C#. Attribution still consults nothing but the span, so both overloads score against the same
lines, and the extractor's signature spelling separates them only as identities. A signature is more
than a parameter list where a parameter list cannot separate two declarations on one line; the C#
extractor's `MethodSpanResult` doc comment owns that format.

Prefer "span" over "method identity" or "method key". The latter two invite a name-based reading,
which is the reading this project rejects.

## Source path

The gate's one path currency: a repo-relative, slash-separated path from the git top level. Every
path the gate handles is either already a source path or is resolved to one, and a path that cannot
be resolved fails the run rather than being matched approximately.

That rule is about the paths the gate scores. The rest of this section is about a report's own paths,
which answer at two granularities: a single path is ignored when it will not resolve, and it is the
whole report that fails.

The gate resolves the changed set and looks coverage up against it, rather than canonicalizing every
path a report mentions. One report path the gate cannot place inside the repo, or places inside it
but the diff never touched, is not the gate's business. A whole report that places no class inside
the repo root, or whose source root was erased before it was written, fails the run, and so does a
single class the gate places at two different paths inside the root, because that report contradicts
itself rather than falling short.

Prefer "source path" over "file path" or "normalized path". The point of the term is that there is
exactly one form, not that some normalizing happened.

## Touched line

A line the diff reports on its **new side**. Deleting lines touches the line at the point they were
removed from, so a deletion is a touch and not an absence. Whitespace-only differences produce no
touched lines.

## Changed method

A method span in the working tree holding at least one touched line. This is the unit the gate
measures; a method with no touched line is never measured, however badly it scores.

One touched line changes the whole method, because the metric is a per-method number. Where spans
nest, only the smallest containing span is changed, matching the rule the join already uses.

## Extractor

The language-specific half of the measurement. Reads source, produces one complexity number per
method span. There is one extractor per language. It declares which file extensions it handles and
reports, per file, whether it parsed that file, so "not my language" and "nothing to measure here"
are distinguishable from "I failed".

## Gate

The language-neutral half. Consumes spans and coverage, joins them, scopes to changed code,
compares against a threshold, and decides pass or fail. A gate is the only thing that knows the
threshold, and it is shared across every language. It hosts more than one metric, so do not name it
after any of them.

## Metric

A single measured quantity the gate computes per method and compares against a threshold. CRAP is
the first. A metric declares the inputs it needs, and the gate demands an input only when a selected
metric declared it.

## Coverage report

An input the gate consumes and never produces. Someone else runs the tests. A report carries the
time it was written, so it can be judged **stale** against the source it claims to describe, and it
is the producer's timestamp that counts, not the file's.

## Method state

What the join could establish about one method span.

| State | Meaning | Effect on the score |
| --- | --- | --- |
| measured | coverage was attributed to the span | scored normally |
| structural n/a | the span holds no instrumentable lines | treated as fully covered |
| unknown | the span could not be attributed at all | excluded, carries a typed reason |

`structural n/a` is what makes trivial members exclude themselves. `unknown` means the join broke,
never that the code is untested, because a coverage report lists methods nobody called.
