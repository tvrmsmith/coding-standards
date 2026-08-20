# `tvrmsmith/combine-assertions-on-same-object`

Combine consecutive assertions on properties of the same object into one structural
assertion.

**Guideline:** *Combine Assertions on the Same Object* — [`test-best-practices/SKILL.md`](../../../../plugins/coding-standards/skills/test-best-practices/SKILL.md#assertions)
(guideline **A1** in the enforcement mapping). It is the one guideline in
the whole inventory with no off-the-shelf rule in either language. The C# half is the
Roslyn analyzer of the same name in `Tvrmsmith.Analyzers`.

Severity in the preset: **warn**. The restructure is mechanical but the choice between
`toEqual` and `toMatchObject` is the author's, and this rule reaches existing test suites
in a batch at commit time.

## Rule details

When several assertions in a row check properties of one object, the first failure ends
the test — the remaining mismatches never report. Characterising the failure then takes
one round trip per property. A single structural assertion reports them all at once.

👎 Examples of **incorrect** code:

```ts
expect(result.page).toBe(2)
expect(result.pageSize).toBe(3)
expect(result.total).toBe(10)
```

👍 Examples of **correct** code:

```ts
// The whole shape.
expect(result).toEqual({ page: 2, pageSize: 3, total: 10, rows })

// A subset, when the object carries more than the test cares about.
expect(result).toMatchObject({ page: 2, pageSize: 3, total: 10 })
```

## What it does not flag

The rule is narrow on purpose: a false-positive-heavy rule gets the whole preset disabled.
It reports only where the combined form asserts *exactly* what the separate ones did.

| Shape | Why it is left alone |
|---|---|
| `expect(a.x).toBe(1); doSomething(); expect(a.y).toBe(2)` | Not back-to-back. The statement between them may be the mutation that makes the two reads different values. |
| `expect(a.x).not.toBe(1)` | A negated object literal is a weaker assertion, not the same one. |
| `await expect(a.x).resolves.toBe(1)` | Combining changes the await semantics. |
| `expect(a.items).toHaveLength(3)` | Only `toBe`, `toEqual` and `toStrictEqual` have an object-literal equivalent. A run containing anything else is broken at that statement. |
| `expect(items[0].id).toBe(1); expect(items[1].id).toBe(2)` | The combined form is an array, not an object literal. Collections stay review-only in v1. |
| `expect(getUser().name).toBe('Ada')` | Two calls need not return the same object. |
| `expect(a.x, 'message').toBe(1)` | vitest's per-assertion message would be discarded. |
| `expect(a.x).toBe(1); expect(a.x).toBe(2)` | One key cannot hold two values. |
| `expect(a.user).toEqual(u); expect(a.user.name).toBe('Ada')` | A key cannot be both a value and an object. |

The near-miss cases in `test/combine-assertions-on-same-object.test.js` are the
specification of this list.

## Options

None.

## No autofix

Deliberate. `toBe` is reference equality and `toEqual` is structural, so rewriting
`expect(r.a).toBe(obj)` as `expect(r).toEqual({ a: obj })` silently weakens it; and
choosing between `toEqual` (exhaustive) and `toMatchObject` (subset) depends on what the
test means to pin down. Prefer no fix over a wrong one.

There is no suggestion either. Suggestions have no surface outside an editor, and this rule
reaches the editor only through a config file the extension is pointed at, never through
`overrideConfig`, which cannot register a plugin in a mixed ESLint 8/9 workspace.

## Runner compatibility

Runner-agnostic. `toBe`, `toEqual` and `toStrictEqual` are spelled the same under vitest
and jest, and the rule matches the global `expect` call rather than any runner import, so a
repo mixing both runners gets the same behaviour everywhere.
