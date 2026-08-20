---
name: coding-standards
description: Coding standards to apply when writing, modifying, or reviewing code — domain/DTO design (logic lives with its data) and React.
---

# Coding Standards

**Enforcement markers.** Guidelines tagged **[review-only]** cannot be caught by a linter —
they are judgment calls, and the only enforcement is a human or an agent reading the code.
Everything untagged is (or will be) enforced mechanically by `eslint-config-tvrmsmith`,
`eslint-plugin-tvrmsmith`, or `Tvrmsmith.Analyzers`; don't spend review attention on it.

## Logic lives with its data — **[review-only]**
Information Expert / no Feature Envy. Coalesce, keep-existing, and parse rules
belong as methods on the command/DTO — not field-by-field in handlers. Handler
deriving one value from 3+ fields of an object → move it onto that object.
Parse strings into value objects at the boundary (parse, don't validate).
Domain invariants stay in the domain; DTOs own only merge/coalesce semantics.

Three distinct judgments live here, all review-only:
- **Feature envy** — a heuristic rule (count member accesses on a foreign object) has too
  high a false-positive rate to ship; deliberately deferred out of v1.
- **Parse, don't validate** — whether a primitive is "primitive obsession" is contextual.
- **Domain invariants vs DTO semantics** — architectural boundary intent, not detectable.

## Comments — **[review-only]**
If you need a paragraph-long comment to justify why the workaround is OK, the code is wrong. Fix the code.

A comment-length threshold would be pure noise — the judgment is about whether the comment is
*justifying* something, not about how long it is.

## React
Writing/reviewing React → read [`react.md`](./react.md).
