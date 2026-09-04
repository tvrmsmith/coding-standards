# coding-standards

Personal coding standards, in one place: the guidance skills, the lint rules that mechanically
enforce them, **and** the metric gate that scores changed methods against a threshold.

## Four enforcement modes

A mode is an enforcement mechanism, distinguished by *when* it fires and what kind of judgement it
applies. Every guideline is assigned to exactly one mode, but a mode stands on its own: one with no
guideline behind it is legitimate, in the same way an off-the-shelf lint rule earns its place
without a guideline being written for it first.

| Mode | When | What it is |
|---|---|---|
| **skill-as-guidance** | before the code is written | The skills below, loaded by the agent while writing code. |
| **lint** | after, mechanical | ESLint + Roslyn rules. Deterministic shapes only — an off-the-shelf rule where one exists, a custom rule where none does. |
| **skill-as-review** | after, judgement | The same skills, read while reviewing. |
| **metric-gate** | after, mechanical, needs the tests run | The `gate/` binary. Measures a quantity per changed method and compares it against a threshold, so it costs a test run and can fail because a test was deleted. |

There is no separate review skill and no review tooling — the guidance and review modes are the
same text read at different times.

`CONTEXT.md` defines the terms these modes are built from, and `docs/adr/` records the decisions
behind the metric gate.

**The skills carry guidelines, never enforcement.** No rule ids, no plugin names, no `[review-only]`
tags. An agent that follows the guidance needs none of it, and an agent that ignores the guidance
gets told by the linter. Keeping the mapping out of the skill also keeps it in one place instead of
two that drift.

Most guidelines need judgement to spot, so the skills are their only enforcement. The rest have a
deterministic shape a linter can catch. Each of those has an id (`A1`, `D11`, `F10`) carried by the
enforcement mapping and repeated on the rule that enforces it — see the rule tables in
`packages/eslint-config-tvrmsmith/README.md` and `dotnet/README.md`.

**The mapping runs one way.** Every guideline needs an enforcement home; a rule does not have to
trace back to a guideline. An off-the-shelf rule that is already installed and already correct
earns its place without a guideline being written for it first. Most of what the preset enables is
exactly that: the `test-integrity` rules, the TypeScript, regex, Sonar and React correctness layers,
and the built-in `CA` rules on the C# side. Requiring a round trip would
mean either writing guideline prose nobody asked for or leaving a good rule off, and both are
worse than an unmapped rule.

## Layout

```
.claude-plugin/marketplace.json      # the marketplace
plugins/coding-standards/            # the plugin
  .claude-plugin/plugin.json
  skills/
    coding-standards/                # domain & DTO design, code smells, type design,
                                     # errors, comments, React effects
      SKILL.md
      react.md
    test-best-practices/             # assertion style, test structure, isolation
      SKILL.md
      references/
        dotnet-awesome-assertions.md
        dotnet-atlas.md
        react-rtl.md
packages/                            # npm
  eslint-config-tvrmsmith/           # the curated off-the-shelf preset
  eslint-plugin-tvrmsmith/           # the one custom ESLint rule
dotnet/                              # NuGet
  src/Tvrmsmith.Analyzers/           # the three custom analyzers + curated severities
  src/Tvrmsmith.MetricGate.CSharp/   # the C# extractor: method spans + cyclomatic complexity
  tests/                             # analyzer and extractor tests, a stand-in consumer, the severity proof
gate/                                # the metric gate: a Go binary, one TOON document on stdout
  cmd/metric-gate/
  internal/                          # diff scope, coverage, join, CRAP, TOON encoder
  test/                              # black-box tests against the built binary, with goldens
harness/                             # machine-local adoption harness: editor layer + pre-commit
```

The plugin loader ignores `packages/`, `dotnet/` and `gate/` — it reads only `.claude-plugin/` and
`skills/`.

## The three custom rules (v1)

Everything else is off the shelf. These three have no off-the-shelf equivalent:

1. `combine-assertions-on-same-object` — Roslyn (`TVRM0001`) **and** ESLint. The ESLint half is
   [written](packages/eslint-plugin-tvrmsmith/docs/rules/combine-assertions-on-same-object.md)
   and wired into the preset.
2. `no-suppression-before-assertion` — Roslyn only (`TVRM0002`). TypeScript is covered by
   `@typescript-eslint/no-non-null-assertion` plus a `no-restricted-syntax` selector.
3. `no-assertion-escape-cast` — Roslyn only (`TVRM0003`). Bans `((object)x).Should()`.

The off-the-shelf layer around them is already curated: `packages/eslint-config-tvrmsmith` for
TypeScript, and for C# both FluentAssertions.Analyzers `FAA0001`–`FAA0004` and most of the built-in
`CAxxxx` rules the SDK ships disabled (see `dotnet/README.md` — the FluentAssertions pairing is
version-sensitive and mixing it with AwesomeAssertions fails *silently*, and the `CA` set is
generated from the SDK's own rule metadata rather than a pinned list).

TypeScript also registers `eslint-plugin-jest`, for the A2 and A5 assertion rules and for a slice
of lint-only test-integrity rules. Despite the name it is not a bet on jest: those rules resolve
`expect` syntactically, so they cover vitest packages too.
`packages/eslint-config-tvrmsmith/README.md` explains the one setting this depends on.

Beyond the guideline rules, the preset enables the untyped half of `typescript-eslint`,
`eslint-plugin-regexp`, `eslint-plugin-sonarjs` and `eslint-plugin-react`. Every one of those
plugins was already an indirect dependency of the packages being linted, with almost nothing turned
on.

The type-aware rules from those same plugins are opt-in, in a separate `eslint-config-tvrmsmith/typed`
entry point, because a typed rule in a package with no `projectService` throws and fails the run.
The machine-local wrapper adds the layer per package, wherever the package already lints with type
information, and `TVRMSMITH_TYPED_LINT` forces the answer either way.

## Install

Local development — the marketplace is read live, so skill edits take effect immediately with no
reinstall:

```bash
claude plugin marketplace add ~/dev/personal/coding-standards
claude plugin install coding-standards@coding-standards
```

Published:

```bash
claude plugin marketplace add tvrmsmith/coding-standards
```

## Distribution

Deliberately solo and machine-local: nothing is committed to the repos being linted, no CI gate,
no teammate impact. The packages wire up by local path while the rules churn, and publish to
GitHub Packages (npm + NuGet) under `tvrmsmith` once they settle — not to the public registries.
