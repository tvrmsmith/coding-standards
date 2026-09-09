# `tvrmsmith/no-quantifier-assertion`

Assert against the collection rather than against the boolean an `every` / `some` call
reduces it to.

**Guideline:** *Assertions Should Communicate Meaning*, [`test-best-practices/SKILL.md`](../../../../plugins/coding-standards/skills/test-best-practices/SKILL.md#assertions)
(guideline **A2** in the enforcement mapping). Most of A2 is off the shelf:
`eslint-plugin-jest`'s `prefer-*` family for the matcher-choice half,
`eslint-plugin-jest-dom` for the DOM half. This rule covers the residue, the shape that
survived both and still has no home.

Severity in the preset: **warn**, matching A1. The fix is a restructure rather than a local
edit, and which restructure is right depends on what the test means.

## Rule details

`expect(users.every(isActive)).toBe(true)` collapses the collection to one bit before the
matcher ever runs. The matcher then has a boolean and nothing else, so the failure reads:

```
expected false to be true
```

Which user failed, and what it held, are gone. Re-running with a debugger or a `console.log`
is the only way to find out. Asserting against the collection keeps the values in the
matcher's hands, so the failure prints them.

👎 Examples of **incorrect** code:

```ts
expect(users.every((u) => u.active)).toBe(true)
expect(routes.some((r) => r.path === 'roles')).toBe(true)
expect(calls.some(([, reason]) => reason === 'login')).toBe(false)
```

👍 Examples of **correct** code:

```ts
// Every element satisfies a predicate: filter to the ones that do not and expect none.
// The failure prints the offending users.
expect(users.filter((u) => !u.active)).toEqual([])

// Membership: the matcher prints the collection alongside the missing member.
expect(routes).toContainEqual(expect.objectContaining({ path: 'roles' }))

// A mapped projection, when the predicate is really a property read.
expect(calls.map(([, reason]) => reason)).not.toContain('login')
```

## What it does not flag

Narrow on purpose, the same way the A1 rule beside it is. It reports only the direct
`expect(<quantifier call>).<matcher>(<boolean>)` shape.

| Shape | Why it is left alone |
|---|---|
| `expect(users.every(isActive)).not.toBe(true)` | A modifier in the chain. `.not`, `.resolves` and `.rejects` all change what the rewrite would have to be, and none of them occurred at the adoption target. |
| `const ok = users.every(isActive); expect(ok).toBe(true)` | The quantifier is not the assertion's subject. The named boolean carries its own meaning. |
| `expect(users.every(isActive)).toMatchSnapshot()` | Only boolean assertions qualify: `toBe` / `toEqual` / `toStrictEqual` against a boolean literal, plus `toBeTruthy` / `toBeFalsy`. |
| `expect(users.includes(user)).toBe(true)` | `jest/prefer-to-contain` owns this one. One shape, one home. |
| `expect(isActive(user)).toBe(true)` | A named predicate call reads as its own expectation, and the shape is far too common to gate on. Review-only. |
| `expect(users[method](isActive)).toBe(true)` | A computed member is not statically an `every`. |
| `expect(users.every(isActive), 'all active').toBe(true)` | vitest's per-assertion message already says what the boolean does not. |

The valid cases in `test/no-quantifier-assertion.test.js` are the specification of this
list.

## Options

None.

## No autofix

Deliberate, and not just caution. An `every` inverts into `filter(not predicate)` and an
empty-array assertion; a `some` is usually a `toContain` or `toContainEqual` against a value
the rule cannot derive from an arbitrary callback. Both rewrites need the predicate read for
meaning, which is the author's job. Prefer no fix over a wrong one.

There is no suggestion either, for the same reason as A1: suggestions have no surface
outside an editor, and this plugin does not reach the editor through `overrideConfig`.

## Runner compatibility

Runner-agnostic. All five matchers are spelled the same under vitest and jest, and the rule
matches the global `expect` call rather than any runner import, so a repo mixing both
runners gets the same behaviour everywhere.

## Provenance

The shape was chosen by measurement, not intuition. `measure-a2-residue.mjs` in
`eslint-config-tvrmsmith` buckets every boolean assertion in a target repo by what its
subject is, and this bucket was the largest one that is both syntactic and has a single
clear rewrite. Re-run it against a target before adding another rule here.
