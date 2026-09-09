# eslint-plugin-tvrmsmith

The custom half of the tvrmsmith lint layer: the guidelines, and the parts of a guideline,
with **no off-the-shelf rule**. Everything that does have one lives in
[`eslint-config-tvrmsmith`](../eslint-config-tvrmsmith).

Flat config only.

## Rules

| Rule | Guideline | Fixable |
|---|---|---|
| [`combine-assertions-on-same-object`](docs/rules/combine-assertions-on-same-object.md) | A1 — *Combine Assertions on the Same Object* (`test-best-practices`) | no, on purpose |
| [`no-quantifier-assertion`](docs/rules/no-quantifier-assertion.md) | A2 — *Assertions Should Communicate Meaning* (`test-best-practices`) | no, on purpose |

A1 has no off-the-shelf rule at all. A2 mostly does, and `no-quantifier-assertion` covers
what those rules leave behind: `expect(xs.every(p)).toBe(true)`, where the collection is
gone before the matcher runs. Which residue shape earned a rule was decided by measuring a
target repo, not by intuition. See `measure-a2-residue.mjs` in the config package, and the
rule doc's provenance section.

The enforcement mapping names three custom rules, but the other two are C#-only:

- `no-suppression-before-assertion` — TypeScript needs no rule authoring;
  `@typescript-eslint/no-non-null-assertion` plus a `no-restricted-syntax` selector for
  `?.` inside `expect(...)` covers it, both in `eslint-config-tvrmsmith/base`.
- `no-assertion-escape-cast` — bans `((object)x).Should()`. No TypeScript shape.

Both live in `Tvrmsmith.Analyzers`.

## Usage

Through the preset, which registers the plugin and sets the severity:

```js
import tvrmsmith from 'eslint-config-tvrmsmith'
export default [...packageOwnConfig, ...tvrmsmith]
```

Or directly:

```js
import tvrmsmith from 'eslint-plugin-tvrmsmith'

export default [
  {
    files: ['**/*.{test,spec}.{js,jsx,ts,tsx}'],
    plugins: { tvrmsmith },
    rules: {
      'tvrmsmith/combine-assertions-on-same-object': 'warn',
      'tvrmsmith/no-quantifier-assertion': 'warn',
    },
  },
]
```

## Test

```bash
pnpm test
```

`RuleTester`, one case per shape. The **valid** cases carry the weight: they are the
specification of where the rule deliberately stays quiet, and a false-positive-heavy rule
gets the whole preset disabled.
