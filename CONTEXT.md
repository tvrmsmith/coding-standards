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

Prefer "span" over "method identity" or "method key". The latter two invite a name-based reading,
which is the reading this project rejects.

## Extractor

The language-specific half of the measurement. Reads source, produces one complexity number per
method span. There is one extractor per language.

## Gate

The language-neutral half. Consumes spans and coverage, joins them, scopes to changed code,
compares against a threshold, and decides pass or fail. A gate is the only thing that knows the
threshold, and it is shared across every language.
