# eslint-config-tvrmsmith

The curated preset behind the personal coding standards. Every rule here traces to a named
guideline, listed below with its id; nothing is enabled because a plugin's `recommended` preset
happened to include it.

It is off the shelf apart from one rule: `eslint-plugin-tvrmsmith` is a dependency, and the preset
registers and configures its single custom rule so a consumer needs no extra wiring. The rule itself
is authored there, not here.

## Install and compose

ESLint 9 flat config. Each entry point default-exports a plain array, so it composes by spreading —
**after** the consumer's own config, so the personal layer wins on the rules it names.

```js
// a React package
import tvrmsmith from 'eslint-config-tvrmsmith'
export default [...packageOwnConfig, ...tvrmsmith]

// a package with no React
import base from 'eslint-config-tvrmsmith/base'
export default [...packageOwnConfig, ...base]
```

| Entry point | Contents |
|---|---|
| `eslint-config-tvrmsmith` | `base` + `react` |
| `eslint-config-tvrmsmith/base` | A1, A4, and D11's empty-`waitFor` selector. No React, no DOM, no test runner. |
| `eslint-config-tvrmsmith/react` | Testing Library, jest-dom, React effects, `react-hooks`. |

The preset does not set a parser or `languageOptions`. The consumer's config supplies those; the
preset only adds plugins and rules.

## One wrapper, both paths

A consumer reaches these rules through the `eslint-layer.js` wrapper in `harness/` — the pre-commit
hook runs it, and the editor loads it as `eslint.options.overrideConfigFile`. Both paths carry every
rule, including all nine `react-you-might-not-need-an-effect` rules.

The obvious alternative for the editor, `eslint.options.overrideConfig`, reaches only a fraction of
them: it cannot register a plugin in a mixed ESLint 8/9 workspace, so it can only enable rules from
plugins a package already declares. `overrideConfigFile` sidesteps that entirely, because the file
it names registers the plugins itself.

## No test runner is assumed

A repo of any size tends to have several vitest majors and jest side by side, React Native included.
Nothing here depends on which: `eslint-plugin-testing-library` and
`eslint-plugin-jest-dom` key off `@testing-library/*` imports and `expect(...)` calls. No
`eslint-plugin-vitest` or `eslint-plugin-jest` rule is enabled, and none should be baked in — if one
is ever wanted, it belongs behind its own entry point.

Test files are matched by `**/*.{test,spec}.*` and `**/__tests__/**` (see `globs.js`), a convention
both runners share.

## Rules, and the guideline each one enforces

Guideline ids are the rows of the enforcement mapping. Severity convention: **error** for
mechanical shapes with one right answer, **warn** where the guideline admits a legitimate exception
or the fix is a judgment call.

### base — the assertion guidelines that need no React (test files only)

| Rule | Severity | Guideline |
|---|---|---|
| `@typescript-eslint/no-non-null-assertion` | error | A4 |
| `no-restricted-syntax` (`?.` inside `expect(...)`) | error | A4 |
| `no-restricted-syntax` (empty `waitFor` callback) | error | D11 |
| `tvrmsmith/combine-assertions-on-same-object` | warn | A1 |

All scoped to test files: these are assertion guidelines, and banning `!` across production source
would enforce something no guideline asks for. The A4 selector matches only inside the `expect(...)`
call's own arguments, so `expect(a).toBe(b?.c)` is left alone.

D11's empty-callback selector sits in `base` rather than `react` for a mechanical reason:
`no-restricted-syntax` is a core rule, so a second config object setting it would replace these
selectors wholesale (see the sharp edge below). `waitFor` is `@testing-library/dom` vocabulary shared
by every framework binding, so a non-React consumer loading `base` alone is not carrying a React rule.

A4's C# half is the Roslyn analyzer `no-suppression-before-assertion`. TypeScript needs no rule
authoring — the pair above covers it.

A1 is the one guideline with no off-the-shelf rule in either language, so its TypeScript half is the
single custom rule in [`eslint-plugin-tvrmsmith`](../eslint-plugin-tvrmsmith). warn: it proposes a
restructure, and the `toEqual` vs `toMatchObject` choice is the author's.

### react — Testing Library + jest-dom (test files only)

| Rule | Severity | Guideline |
|---|---|---|
| `testing-library/prefer-screen-queries` | error | D1 |
| `testing-library/render-result-naming-convention` | warn | D2 |
| `testing-library/no-manual-cleanup` | error | D3 |
| `testing-library/prefer-user-event` | error | D4 |
| `testing-library/prefer-user-event-setup` | error | D4 |
| `testing-library/no-test-id-queries` | warn | D6 |
| `testing-library/no-container` | error | D6 |
| `testing-library/no-node-access` | error | D6 |
| `testing-library/prefer-presence-queries` | error | D7 |
| `testing-library/prefer-find-by` | error | D7 |
| `testing-library/prefer-query-by-disappearance` | error | D7 |
| `testing-library/no-unnecessary-act` | error | D8 |
| `testing-library/await-async-queries` | error | D10 |
| `testing-library/await-async-utils` | error | D10 |
| `testing-library/await-async-events` | error | D10 |
| `testing-library/no-await-sync-queries` | error | D10 |
| `testing-library/no-await-sync-events` | error | D10 |
| `testing-library/no-wait-for-multiple-assertions` | error | D11 |
| `testing-library/no-wait-for-side-effects` | error | D11 |
| `testing-library/no-wait-for-snapshot` | error | D11 |
| D11's empty-callback half — `no-restricted-syntax`, in `base` | error | D11 |
| `jest-dom/prefer-*` — all 12 | error | A2 / D5 |

`render-result-naming-convention` and `no-test-id-queries` are warn because both describe a
preference with a real exception: testId is a legitimate last resort when nothing accessible
identifies the node.

### react — effects (all source files)

| Rule | Severity | Guideline |
|---|---|---|
| `react-you-might-not-need-an-effect/no-derived-state` | warn | F1, and the derives-from-a-prop half of F11 |
| `react-you-might-not-need-an-effect/no-chain-state-updates` | warn | F2 |
| `react-you-might-not-need-an-effect/no-event-handler` | warn | F3 |
| `react-you-might-not-need-an-effect/no-pass-live-state-to-parent` | warn | F4 |
| `react-you-might-not-need-an-effect/no-pass-data-to-parent` | warn | F5 |
| `react-you-might-not-need-an-effect/no-external-store-subscription` | warn | F6 |
| `react-you-might-not-need-an-effect/no-initialize-state` | warn | F7 |
| `react-you-might-not-need-an-effect/no-reset-all-state-on-prop-change` | warn | F10 |
| `react-you-might-not-need-an-effect/no-adjust-state-on-prop-change` | warn | F11, the rest of it |
| `react-hooks/exhaustive-deps` | warn | F8 |

All nine of the effect plugin's rules are enabled. All warn: each proposes a restructure rather than
a local edit, the effect plugin's own recommended preset ships them at warn, and these are exactly
the rules with zero editor reach — they surface in a batch at commit time, where an error severity
would block the commit on refactors that need thought.

`no-derived-state` and `no-adjust-state-on-prop-change` are complements, and between them F11 has no
review residue left: the first needs the new state value to derive from a prop, and the second fires
precisely when it does not — `setSelection(null)` on an `[items]` change, the store-an-id-and-`find`
shape the mapping recorded as undetectable.

`no-reset-all-state-on-prop-change` carries F10 in name only. v1.0.1 gates on *setter references in
the effect === state references in the whole component*, counting references rather than `useState`
declarations, so any other read or write of the state silences it — including React's own worked
example. Enabled for its message, which is the only one that names `key`; the shapes that matter are
caught by `no-adjust-state-on-prop-change`.

## Deliberately not enabled

- **A `no-restricted-imports` ban on `fireEvent`** — withdrawn. `testing-library/prefer-user-event`
  covers D4 off the shelf.
- **Anything from `eslint-plugin-vitest` / `eslint-plugin-jest`** — see above.

`testing-library/no-wait-for-empty-callback` is not available to enable: it was removed in the
plugin's **6.0.0** — 6.5.0 already ships without it — and the last version carrying it is 5.11.1.
It is replaced by a `no-restricted-syntax` selector in `base` that
reproduces both shapes 5.11.1 reported (an empty block body, and the literal `waitFor(noop)`) and
inherits its blind spot for `waitFor(() => undefined)`. The one behavioural difference is that the
rule resolved the callee back to a `@testing-library/*` import: a renamed import
(`import { waitFor as until }`) is missed by the selector, and an unrelated local `waitFor` would be
caught by it. A namespaced `rtl.waitFor(() => {})` was outside the original rule as well.

## Known sharp edge: `no-restricted-syntax`

`no-restricted-syntax` is a **core** rule, so a later flat-config object replaces an earlier one's
options wholesale rather than merging. A consumer that already configures it, as React Native
apps commonly do, must pass its own selectors through, or they are silently dropped:

```js
import { createBase } from 'eslint-config-tvrmsmith'
export default [...packageOwnConfig, ...createBase({ extraRestrictedSyntax: packageOwnSelectors })]
```

The default export cannot do this automatically; the wrapper or the consumer has to hand the existing
selectors over.

## Tests

```bash
pnpm test
```

`test/cases.js` holds at least one case per enabled rule — a violating sample the rule must fire on
and a compliant sample it must stay silent on — linted through the preset against `test/fixture-package/`,
a stand-in consumer that supplies the parser exactly as a real package would. The suite also asserts
the case list and the preset name the same rule set, so a rule cannot be added without a fixture.
A case for one of the `no-restricted-syntax` selectors names the selector and is matched on its
message, so it cannot pass on a different selector's report.
`test/fixture-package/src/` holds compliant component and test files that must produce zero reports.
