# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root.
- **`docs/adr/README.md`**: the one-line live rule of every ADR, plus which are superseded and the
  amend-versus-supersede conventions. Start here to find the ADRs that touch your area, then read
  the `## Current rule` block of each. Read a full ADR file only for the reasoning behind its rule.

If either is missing, **proceed silently**. Don't flag its absence; don't suggest creating it
upfront. The `/domain-modeling` skill (reached via `/grill-with-docs` and
`/improve-codebase-architecture`) creates them lazily when terms or decisions actually get resolved.

## File structure

This is a single-context repo:

```
/
├── CONTEXT.md
├── docs/adr/
│   ├── README.md
│   ├── 0001-....md
│   └── 0002-....md
└── packages/, dotnet/, gate/, plugins/, harness/
```

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, that's a signal: either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0008 (the machine document is the only output), but worth reopening because…_
