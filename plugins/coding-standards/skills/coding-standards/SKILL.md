---
name: coding-standards
description: Coding standards to apply when writing, modifying, or reviewing code — domain/DTO design (logic lives with its data) and React.
---

# Coding Standards

## Logic lives with its data
Information Expert / no Feature Envy. Coalesce, keep-existing, and parse rules
belong as methods on the command/DTO — not field-by-field in handlers. Handler
deriving one value from 3+ fields of an object → move it onto that object.
Parse strings into value objects at the boundary (parse, don't validate).
Domain invariants stay in the domain; DTOs own only merge/coalesce semantics.

Three distinct judgments live here:
- **Feature envy** — counting member accesses on a foreign object is a heuristic, so weigh
  what the code means rather than the count.
- **Parse, don't validate** — whether a primitive is "primitive obsession" is contextual.
- **Domain invariants vs DTO semantics** — architectural boundary intent.

## Comments
If you need a paragraph-long comment to justify why the workaround is OK, the code is wrong. Fix the code.

The judgment is about whether the comment is *justifying* something, not about how long it is.

## React
Writing/reviewing React → read [`react.md`](./react.md).
