---
name: coding-standards
description: Coding standards to apply when writing, modifying, or reviewing code
---

# Coding Standards

Guidance while writing or reviewing.

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

## Code smells
Grounded in Fowler, _Refactoring_ ch.3. A smell is always a judgement call in context, never a hard
violation, and a documented repo standard that endorses one of these shapes wins over the list.

Smell → fix:
- **Mysterious Name** — a name that doesn't reveal what it does or holds → rename it. No honest
  name comes → the design is murky, fix that first.
- **Duplicated Code** — the same logic shape in more than one place → extract it, call it from both.
- **Feature Envy** — a method reaching into another object's data more than its own → move the
  method onto the data it envies.
- **Data Clumps** — the same few fields or params keep travelling together, a type wanting to be
  born → bundle them into one type, pass that.
- **Primitive Obsession** — a primitive standing in for a domain concept → give the concept its own
  small type, parsed at the boundary.
- **Repeated Switches** — the same `switch` or `if`-cascade on the same type recurs → polymorphism,
  or one map both sites share.
- **Shotgun Surgery** — one logical change forces scattered edits across many files → gather what
  changes together into one module.
- **Divergent Change** — one file edited for several unrelated reasons → split so each module
  changes for one reason.
- **Speculative Generality** — abstraction or hooks added for needs nothing has → delete it, inline
  back until a real need shows.
- **Message Chains** — long `a.b().c().d()` navigation the caller shouldn't depend on → hide the
  walk behind one method on the first object.
- **Middle Man** — a class or function that mostly just delegates onward → cut it, call the real
  target direct.
- **Refused Bequest** — a subclass that ignores most of what it inherits → drop the inheritance,
  use composition.

## Make illegal states unrepresentable
The compiler enforces the invariant instead of a convention.

- **Illegal states representable** — a bad instance can be constructed. Boolean pairs encoding
  three real states, optional fields required in some mode, nullable soup → replace flag combos
  with a sum type, validate in a smart constructor so an invalid value never exists.
- **Leaky encapsulation** — exposed mutable fields or collections let callers break an invariant
  the type is supposed to hold → make the field private, route mutation through methods that
  preserve validity.

## Errors
Zero tolerance for silent failure. A silent failure is an error swallowed, hidden, or turned into a
misleading success, and it is worse than a crash because it hides the bug in production instead of
reporting it. Errors go loud, contextual, and propagated.

Anti-pattern → fix:
- **Swallowed exception** — an empty catch, a catch that only debug-logs and continues, or a catch
  returning a fake-success value → rethrow, or propagate a Result.
- **Fallback that masks failure** — returning empty, default, or cached data when the real
  operation failed, with no failure signal → surface the failure, or let it propagate.
- **Vague message** — "something went wrong", no context, no identifiers, no original error → say
  what failed, with enough detail to act on.
- **Lost context** — rethrowing without the cause, or catching a broad type that hides unrelated
  errors → attach the cause, narrow the catch.
- **Unchecked result** — an ignored return code or Result value, an unawaited promise or task →
  check it, or await it.
- **Overly broad catch** — `catch (Exception)` around a wide block → catch the specific type,
  around the smallest block that can throw it.

Use the project's own logger and error-id patterns rather than generic ones.

## Comments
Code documents itself. Do not write comments: a comment narrating the code duplicates it and rots
beside it, and a wrong comment is worse than none, because the reader trusts it.

- **Say it in the code.** Carry the meaning in a name, a type, a constant or an extracted
  function. `i++ // increment i` is deleted, and `// convert to seconds` becomes `toSeconds()`.
- **A comment justifying a workaround means the code is wrong.** Fix the code rather than
  explaining it.
- **What another agent needs to know goes in a test.** Name the test for the behavior and assert
  it. It documents the same fact as a comment would, and it fails when the fact stops being true.
- **Doc comments on declarations are the exception.** JSDoc, TSDoc, XML doc and Go doc comments
  document public API contracts, units, invariants and side effects. Put them on the declaration
  they describe; one above a statement documents nothing.
- **A suppression carries its reason on itself.** `-- reason` after an `eslint-disable`,
  `// reason` after `//nolint:linter`, or on the `#pragma warning disable` line.
- **Generated files belong to their generator.** Do not edit their comments, or their code.

## React
Touching `.jsx`/`.tsx`, or JSX or a `use*` hook in `.js`/`.ts` → read [`react.md`](./react.md) and
apply it to every component and hook you write or review.
