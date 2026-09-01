# eslint-config-tvrmsmith

The curated preset behind the personal coding standards. Nothing is enabled because a plugin's
`recommended` preset happened to include it.

Most rules trace to a named guideline, listed below with its id. The mapping runs in **one
direction only**: every guideline needs an enforcement home, but a rule does not have to trace
back to a guideline. An off-the-shelf rule that is already installed and already correct earns
its place without a paragraph being written for it first — those are marked `—` in the tables.

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
| `eslint-config-tvrmsmith/base` | A1, A4, D11's empty-`waitFor` selector, and the TypeScript, regex and Sonar layers. No React, no DOM, no test runner. |
| `eslint-config-tvrmsmith/react` | Testing Library, jest-dom, React effects, `react-hooks`, and the React correctness layer. |

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
Nothing here depends on which. `eslint-plugin-testing-library` and `eslint-plugin-jest-dom` key off
`@testing-library/*` imports and `expect(...)` calls.

`eslint-plugin-jest` is registered too, and its name is the misleading part: **it is not a bet on
jest.** The A2/A5 rules enabled from it decide syntactically, through the plugin's `parseJestFnCall`,
which resolves `expect` three ways:

| How `expect` resolves | Treated as an assertion? |
|---|---|
| a local declaration in the file | no, the rules stay silent |
| an import | only if the source matches `settings.jest.globalPackage` |
| an unresolved global | always, whichever runner injected it |

So a vitest file gets these rules by either of the last two paths, and the setting decides one thing
only: which explicit import is recognised. `base` sets it to `vitest`. Three consequences worth
knowing before changing it:

- **It takes a single string.** An array is accepted without error and matches nothing, switching
  detection off for every import form at once. Verified against the plugin, not inferred.
- **`vitest` and `@jest/globals` are mutually exclusive.** `vitest` is chosen because it is the
  import that exists in practice: across the adoption target's 2058 test files, 387 import `expect`
  from `vitest` and zero from `@jest/globals`. Jest packages there use the injected globals, which
  the setting does not affect.
- **`jest/no-deprecated-functions` must stay off.** It is the one rule that resolves the jest
  package, and it *throws* where jest is not installed. A test asserts it is not enabled.

`@vitest/eslint-plugin` is deliberately absent. It is a fork of these same rules with a smaller set
in this family, so registering it alongside would double-report every violation and add no coverage.

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
| `jest/prefer-to-be` | error | A2 |
| `jest/prefer-to-contain` | error | A2 |
| `jest/prefer-to-have-length` | error | A2 |
| `jest/prefer-comparison-matcher` | error | A2 |
| `jest/prefer-equality-matcher` | error | A2 |
| `jest/prefer-strict-equal` | warn | A2 |
| `jest/prefer-called-with` | warn | A2 |
| `jest/require-to-throw-message` | error | A2 |
| `jest/no-alias-methods` | error | A2 |
| `jest/prefer-to-have-been-called` | error | A2 |
| `jest/prefer-to-have-been-called-times` | error | A2 |
| `jest/valid-expect` | error | A5 |
| `jest/no-conditional-expect` | error | A5 |
| `jest/valid-expect-in-promise` | error | A5 |
| `jest/no-standalone-expect` | error | A5 |
| `jest/expect-expect` | warn | A5 |

`expect-expect` is the one A5 rule at warn, and not because the guideline is soft. The rule counts
only calls named in `assertFunctionNames` (default `expect`), so a suite asserting through another
vocabulary reads as assertionless. The measured case is WebdriverIO e2e specs, where
`browser.waitUntil(...)` *is* the assertion and the rule cannot see it. The guideline is right and
the detector is incomplete, which is the same reason `prefer-strict-equal` warns.

`require-to-throw-message` is the sharpest of these: a bare `toThrow()` passes for *any* error,
including a `TypeError` thrown by a typo in the test itself.

### base — test integrity (test files only, no guideline)

Lint-only. A test that silently does not run, a title that cannot identify a failure, and mocks
that leak into later tests.

| Rule | Severity | Guideline |
|---|---|---|
| `jest/no-focused-tests` | error | — |
| `jest/no-disabled-tests` | error | — |
| `jest/no-commented-out-tests` | error | — |
| `jest/no-identical-title` | error | — |
| `jest/valid-title` | error | — |
| `jest/no-duplicate-hooks` | error | — |
| `jest/prefer-spy-on` | error | — |
| `jest/prefer-mock-promise-shorthand` | error | — |

`no-disabled-tests` and `no-commented-out-tests` are error rather than warn on purpose:
`eslint-disable-next-line` is the better escape hatch, because it puts the reason for the skip in
the file next to the skip instead of in a log nobody reads. `no-commented-out-tests` fuzzy-matches
comment text, so it can misjudge what it found; the same hatch applies.

`prefer-spy-on` is isolation rather than style. `Date.now = jest.fn()` destroys the original and
nothing restores it, so the damage leaks into every later test in the file.

This slice registers `eslint-plugin-jest` a second time. Flat config permits that because both
slices spread the same imported plugin object; it rejects two *different* objects under one
namespace.

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

## The off-the-shelf layers (no guideline)

Several slices enable rules that answer to no guideline. Each is here on the one-directional mapping:
the plugin was already installed, and the rule is already right. `base.js` and `react.js` name every
rule with a comment on the ones that need one, and they are the enumeration. Repeating the list here
would only give it a second place to go stale.

One constraint spans them all. **The default export enables no rule that requires type
information**, because a typed rule in a package with no `projectService` throws and takes the run
down. The type-aware rules live in their own entry point instead; see [the typed
layer](#the-typed-layer) below. A test walks each set and fails on a rule in the wrong one, so the
split cannot erode by accident.

| Slice | Files | What it covers |
|---|---|---|
| `tvrmsmith/base/typescript` | source | `typescript-eslint` rules that need no types: shadowing, class and enum shapes, `@ts-ignore`, plus a few style rules whose recommendation is never wrong |
| `tvrmsmith/base/regexp` | source | `eslint-plugin-regexp` correctness only: catastrophic backtracking, assertions that can never match, dead groups and quantifiers |
| `tvrmsmith/base/sonarjs-bugs` | source | Sonar rules whose `meta.type` is `problem`: identical branches, overwritten elements, unthrown errors, shadowed globals |
| `tvrmsmith/base/sonarjs-security` | source | Sonar's security family: hardcoded secrets, weak crypto, command and XXE injection, permissive file modes |
| `tvrmsmith/base/sonarjs-tests` | test | Suite shapes that make a test not run or not report |
| `tvrmsmith/react/jsx` | source | `eslint-plugin-react` correctness, security and deprecated-API rules, plus Sonar's two render-loop rules |

A few things worth knowing before editing them:

- **The `typescript-eslint` extension rules switch off the core rule they replace.** Both would
  otherwise report the same violation twice. The preset does this itself rather than asking the
  consumer, which is why the slice contains `off` entries.
- **`react` version detection cannot work here.** It resolves `react/package.json` from the
  *config's* location, and this preset is installed machine-locally outside the package it lints, so
  detection finds nothing and every version-gated rule quietly downgrades. The slice pins
  `settings.react.version` instead.
- **Several Sonar test rules gate on the runner.** They read the import list and `package.json`, and
  return no visitors unless jest, mocha, jasmine or cypress is present. A vitest package leaning on
  injected globals gets nothing from them. They stay on because silence costs nothing and the jest
  packages at the target do get them.
- **The overlaps are resolved in favour of the untyped rule.** `react/jsx-no-leaked-render` over
  Sonar's typed copy, `jest/no-focused-tests` over `sonarjs/no-exclusive-tests`, `jest/valid-expect`
  over `sonarjs/async-test-assertions`, and `eslint-plugin-regexp` over Sonar's regex family.
- **Volume is not a reason to exclude a rule.** The pre-commit hook scopes reporting to changed
  files, so a large standing backlog costs nothing. The exclusions below are all about the rule
  being wrong, not loud.

## The typed layer

`typed.js` holds every rule that reads the TypeScript checker: the `typescript-eslint` promise,
`any`-leakage, dead-code and one-right-answer families, Sonar's type-confusion and ignored-result
rules, and the handful of jest rules that need a type to decide. `typescript-eslint` treats these as
the half of the plugin worth having, and the reason they were held back was never that they are
wrong. It was that a typed rule in a package with no program throws
`you have used a rule which requires type information` and fails the whole run.

So the layer is a separate entry point, and a package opts in by having already configured type
information for its own linting:

```js
import tvrmsmith from 'eslint-config-tvrmsmith'
import typed from 'eslint-config-tvrmsmith/typed'

export default [...packageOwnConfig, ...tvrmsmith, ...typed]
```

`typedBase` and `typedReact` are the React-free and React-only halves, matching `base` and `react`.

**The machine-local wrapper composes this for you.** `harness/eslint-layer.js` reads the package's
own resolved config and adds the typed layer only where it finds
`parserOptions.projectService`, `parserOptions.project`, or the v6/v7 spelling
`EXPERIMENTAL_useProjectService`. Nothing to add to the repo, nothing to keep in sync, and no marker
file that outlives the config it describes. `TVRMSMITH_TYPED_LINT=0` forces the layer off in a
package that would otherwise get it; `TVRMSMITH_TYPED_LINT=1` forces it on. Set
`TVRMSMITH_ESLINT_DEBUG=1` to have the wrapper print which way it went and why.

Two things about the severities here:

- **The `no-unsafe-*` family warns rather than errors.** The `any` is almost always arriving from an
  untyped dependency or a `JSON.parse`, so the fix is upstream of the line being flagged, and an
  error would block a commit on someone else's type declarations.
- **The overlaps stay resolved in favour of the untyped copy.** Where a plugin ships both, the
  typed layer drops its version rather than double-reporting. `typed.js` names each one it dropped.

## Deliberately not enabled

- **A `no-restricted-imports` ban on `fireEvent`** — withdrawn. `testing-library/prefer-user-event`
  covers D4 off the shelf.
- **Anything from `@vitest/eslint-plugin`** — it forked the jest rules already enabled here, so
  registering both double-reports every violation. See above.
- **`jest/no-conditional-in-test`** — a superset of `jest/no-conditional-expect`, which is enabled
  and catches the sharp case. Conditionals in test *setup* are frequently legitimate, and the rule
  reports 497 times across 167 files at the adoption target, which would bury the signal.
- **`jest/prefer-each`** — a `for` loop around `it()` works; `.each` mainly buys nicer failure
  output. Shipping a rule at warn because it is only half believed is how a preset accumulates
  noise nobody acts on.
- **`sonarjs/no-reference-error`** — it flags any unresolved global, and the preset sets no
  `languageOptions.globals`, so it fired on `Buffer` and on the injected test globals. TypeScript
  and core `no-undef` already own this.
- **`sonarjs/no-same-argument-assert`** — it calls `isImported` for *chai* and returns no visitors
  otherwise, so in a jest/vitest repo it can never report.
- **The `regexp` `prefer-*` and `sort-*` families** — style opinions on regex syntax, where the
  original spelling is often the clearer one.
- **`react`'s PropTypes family, the classic-runtime rules, and the formatting families** — PropTypes
  is dead in a TypeScript codebase, the JSX transform makes `react-in-jsx-scope` wrong, and Prettier
  owns formatting.
- **Sonar's Express and helmet security rules** — they apply: three packages at the adoption target
  run Express. Each needs a framework-shaped fixture, and they form one coherent batch, so they are
  filed separately rather than half-done here.

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
