---
name: test-best-practices
description: Use when writing, reviewing, or refactoring tests in any language/framework. Covers assertion style, test structure, naming, isolation, and common anti-patterns. Triggers on: test, unit test, integration test, acceptance test, assertion, should, expect, assert, test style, test review, test refactor, TDD, test coverage.
---

# Test Best Practices

Technology-agnostic principles for writing clear, maintainable, reliable tests. For language/framework-specific patterns and examples, check `references/`.

## Progressive Discovery

After reading these principles, read the relevant technology-specific reference for concrete syntax and patterns:

```
references/
  dotnet-awesome-assertions.md   — .NET (AwesomeAssertions / FluentAssertions + NUnit)
  dotnet-atlas.md                — escaping a framework's custom assertions type
  react-rtl.md                   — React (Testing Library + jest-dom + user-event)
```

Read the relevant reference file at `<skill-directory>/references/<file>` before writing tests.

---

## Assertions

### Combine Assertions on the Same Object

Back-to-back assertions on the same object are wasteful and fragile. When the first fails, subsequent assertions never run — hiding additional problems.

Use a single assertion that verifies multiple properties at once. Most frameworks provide structural/equivalence comparison that checks a subset of properties in one expression.

**Bad** — sequential assertions, first failure hides the rest:
```
assert result.page == 2
assert result.page_size == 3
assert result.total == 10
```

**Good** — one assertion, all properties checked together:
```
assert result matches { page: 2, page_size: 3, total: 10 }
```

This applies to collections too. Don't assert count then index into elements separately — use a single equivalence check against expected items.

### Assertions Should Communicate Meaning

The assertion method/matcher should describe what's being tested. A reader should understand the expectation from the assertion alone, without reading setup code.

**Bad** — generic, says nothing about intent:
```
assert result != null
assert result.name == "expected"
```

**Good** — assertion method names convey the expectation:
```
assert result is_equivalent_to { name: "expected" }
assert items contain_single_item matching { id: 42 }
assert response.status == OK
```

Choose specific matchers over generic equality when they exist. `contain_single` is more expressive than `length == 1`. `be_empty` is clearer than `length == 0`.

### Use Lazy/Scoping for Unavoidable Multi-Assertions

Whether to reach for a scope is a judgment call, and the "when NOT to scope" list below is
inherently contextual.

Sometimes combining into one assertion isn't possible — different comparison strategies, mixed type checks and property checks, or assertions on genuinely different values that share logical context.

Wrap assertions in a **scope** (or use lazy evaluation) so all assertions execute and all failures surface in a single test run.

Without scoping, only the first failure reports — slow feedback loops. Scoping shows everything at once.

**When to scope:**
- Assertions that can't be combined but verify related aspects of the same operation
- Mixed assertion types (type check + property check + status check)
- Verifying side effects alongside return values

**When NOT to scope:**
- Single assertion — no need
- Assertions that naturally combine into structural equivalence — combine instead
- Guard assertions that should fail fast (e.g., response succeeded before checking body)

### Don't Suppress Null/Missing Value Failures

Never use language features that silently skip assertions when a value is null or missing. The test should fail loudly if an intermediate value is unexpectedly absent.

- Don't use optional chaining (`?.`, `&.`) before assertion calls — null silently passes
- Don't use null-forgiving/force-unwrap (`!`, `!!`) to bypass type safety — construct the expected value and compare directly
- If a value might legitimately be null, assert that explicitly

This is about the *value under test* — the receiver chain feeding the assertion — not about
every `!` in the file. Setup preconditions (e.g. `Client.BaseAddress!`) are fine.

### Assertions Must Actually Execute

An assertion that never runs is worse than no assertion: the suite is green and nothing was
checked. A missing assertion is at least visible.

Three shapes produce it:

- **A dropped async matcher.** The matcher returns a promise nobody awaits, so the test ends
  before the assertion resolves. Await the assertion, not just the value.
- **No matcher at all.** `expect(value)` on its own is a statement that computes nothing.
- **An assertion inside a branch.** An assertion under `if` or in a `catch` reports nothing when
  the branch is not taken, so a test that never enters the branch passes having checked nothing.
  Decide the expected value before the assertion and assert unconditionally. To assert that
  something threw, assert on the throwing call itself rather than asserting inside `catch`.

---

## Assertion Quick Reference

| Scenario | Approach |
|----------|----------|
| Multiple properties on one object | Single structural/equivalence assertion |
| Collection contents | Equivalence against expected items |
| Collection count only | Direct count assertion |
| Single item + type verification | Filter to single → type check → equivalence |
| Can't combine assertions | Wrap in assertion scope / use lazy evaluation |
| Nullable intermediate value | Construct expected, compare directly — no null suppression |
