import tseslint from '@typescript-eslint/eslint-plugin'
import jest from 'eslint-plugin-jest'
import tvrmsmith from 'eslint-plugin-tvrmsmith'

import { testFiles } from './globs.js'

/**
 * A4 — the `?.` half. `expect(user?.name).toBe('x')` passes `undefined` into the
 * matcher instead of failing on the missing object. The selector matches only inside
 * the `expect(...)` call's own arguments, so `expect(a).toBe(b?.c)` — a `?.` in the
 * *expected* value rather than the value under test — is left alone.
 */
export const noOptionalChainInExpect = {
  selector: 'CallExpression[callee.name="expect"] ChainExpression',
  message:
    'Optional chaining inside expect(...) suppresses the null/undefined failure the assertion exists to catch. Assert on the value directly.',
}

/**
 * D11 — the empty-callback half. `await waitFor(() => {})` waits for the next tick and
 * asserts nothing, so the test passes whatever the UI did. Replaces
 * `testing-library/no-wait-for-empty-callback`, which was removed in
 * `eslint-plugin-testing-library` **6.0.0** (not v7, as an earlier note here said).
 *
 * Reproduces both shapes the removed rule reported — a callback whose body is an empty
 * block, and the literal `waitFor(noop)` spelling — and inherits its blind spot for a
 * concise noop body (`waitFor(() => undefined)`), which it also let through. The one
 * behavioural difference: the rule resolved the callee back to a `@testing-library/*`
 * import, so a renamed import (`import { waitFor as until }`) is missed here and an
 * unrelated local `waitFor` would be caught. Measured against v5.11.1's implementation,
 * a namespaced `rtl.waitFor(() => {})` was outside the rule too. The residue is
 * review-only.
 *
 * Lives in the base slice rather than the React one because `no-restricted-syntax` is a
 * core rule: a second config object setting it would replace this one's selectors
 * wholesale. `waitFor` is `@testing-library/dom` vocabulary shared by every framework
 * binding, so a non-React consumer loading base alone is not carrying a React rule.
 */
export const noEmptyWaitForCallback = {
  selector:
    ':matches(CallExpression[callee.name="waitFor"], CallExpression[callee.name="waitForElementToBeRemoved"]) > :matches(:matches(ArrowFunctionExpression, FunctionExpression)[body.body.length=0], Identifier[name="noop"])',
  message:
    'An empty waitFor callback waits for a tick and asserts nothing, so the test passes whatever the UI did. Put the single assertion you are waiting for inside it.',
}

/**
 * The one runner plugin the preset registers, and the setting that makes it cover both
 * runners. See `assertionIntentSettings` for why `eslint-plugin-jest` is not a bet on jest.
 */
export const assertionIntentSettings = {
  /**
   * `eslint-plugin-jest` resolves `expect` three ways, and only the middle one names a
   * package: a local declaration means "not an assertion" and the rules stay silent; an
   * *import* is matched against this setting; an unresolved global is always treated as an
   * assertion, whichever runner injected it.
   *
   * So the globals-style majority is covered whatever this says, and this setting decides
   * one thing only: which explicit `import { expect } from '…'` is recognised. It takes a
   * **single string**. An array is accepted without error and silently matches nothing,
   * disabling detection for every import form at once — verified, not inferred.
   *
   * `'vitest'` rather than the default `'@jest/globals'` because that is the import that
   * exists in practice: across the adoption target's 2058 test files, 387 import `expect`
   * from `vitest` and **zero** import it from `@jest/globals`. Jest packages there write
   * assertions against the injected globals, which this setting does not affect. A repo
   * that does import from `@jest/globals` needs the default back, and cannot have both.
   */
  jest: { globalPackage: 'vitest' },
}

/**
 * Base slice — no React, no DOM, and no assumption about *which* runner is installed.
 *
 * Carries the assertion guidelines that need no React: **A1, "Combine assertions on
 * the same object"**, **A2, "Assertions should communicate meaning"**, **A4, "Don't
 * suppress null/missing value failures"** and **A5, "Assertions must actually execute"**
 * (test-best-practices/SKILL.md). Everything here is scoped to test files — these are
 * assertion guidelines, and banning `!` across production source would enforce something
 * no guideline asks for.
 *
 * A4's C# half is a Roslyn analyzer (`no-suppression-before-assertion`); TypeScript
 * needs no rule authoring — an off-the-shelf rule plus one selector covers it. A1 is the
 * one guideline with no off-the-shelf rule in *either* language, so its TypeScript half
 * is the single custom rule in `eslint-plugin-tvrmsmith`.
 *
 * `no-restricted-syntax` is a **core** rule, so a later config object replaces an
 * earlier one's options wholesale rather than merging them. A consumer that already
 * configures `no-restricted-syntax` (React Native apps commonly do) must pass
 * its own selectors through `extraRestrictedSyntax`, or they are silently dropped.
 *
 * @param {{ extraRestrictedSyntax?: unknown[] }} [options]
 * @returns {import('eslint').Linter.Config[]}
 */
export function createBase({ extraRestrictedSyntax = [] } = {}) {
  return [
    {
      name: 'tvrmsmith/base/no-suppression-before-assertion',
      files: testFiles,
      plugins: { '@typescript-eslint': tseslint },
      rules: {
        // A4 — `expect(user!.name)` turns a null value under test into a type-level
        // shrug. error: mechanical, no judgment, and it is exactly the failure the
        // test exists to catch.
        '@typescript-eslint/no-non-null-assertion': 'error',

        // A4's `?.` half and D11's empty-callback half. Both are selectors on the one
        // core rule, so they share a single entry — see `noEmptyWaitForCallback`.
        'no-restricted-syntax': [
          'error',
          ...extraRestrictedSyntax,
          noOptionalChainInExpect,
          noEmptyWaitForCallback,
        ],
      },
    },
    {
      name: 'tvrmsmith/base/combine-assertions-on-same-object',
      files: testFiles,
      plugins: { tvrmsmith },
      rules: {
        // A1 — back-to-back assertions on one object hide every failure after the first.
        // warn: the rule proposes a restructure rather than a local edit, the choice
        // between `toEqual` and `toMatchObject` is the author's, and it lands on existing
        // suites in a batch at commit time (the plugin has zero editor reach — a plugin
        // cannot be registered through `overrideConfig` in a mixed ESLint 8/9 workspace).
        'tvrmsmith/combine-assertions-on-same-object': 'warn',
      },
    },
    {
      name: 'tvrmsmith/base/assertion-intent',
      files: testFiles,
      plugins: { jest },
      settings: assertionIntentSettings,
      rules: {
        // A2 — the matcher should name the expectation. These four rewrite one call into
        // a more specific one with the same meaning, so error: mechanical, autofixable,
        // one right answer.
        //
        // `toEqual(1)` / `toBe(null)` on a primitive says "compare these"; `toBe` says it
        // exactly.
        'jest/prefer-to-be': 'error',
        // `expect(list.includes(x)).toBe(true)` reports `false !== true` on failure.
        // `toContain` reports the list and the missing member.
        'jest/prefer-to-contain': 'error',
        // `expect(list.length).toBe(3)` reports `2 !== 3`. `toHaveLength` reports the list.
        'jest/prefer-to-have-length': 'error',
        // `expect(n > 5).toBe(true)` reports `false !== true` and names neither operand.
        'jest/prefer-comparison-matcher': 'error',
        // `expect(a === b).toBe(true)` — same failure, same fix, but suggestion-only
        // upstream because the negated spelling picks between `toBe` and `not.toBe`.
        'jest/prefer-equality-matcher': 'error',

        // A2, but warn: both propose a strictly stronger assertion, and whether the
        // strengthening is wanted is the author's call.
        //
        // `toStrictEqual` also compares undefined keys and class identity, which is
        // sometimes the point and sometimes an unrelated failure.
        'jest/prefer-strict-equal': 'warn',
        // Asserting only that a spy was called is occasionally all the test means.
        'jest/prefer-called-with': 'warn',

        // A2, continued. `toThrow()` with no argument passes for *any* error, including a
        // TypeError thrown by a typo in the test itself, so the assertion looks like it
        // verifies behaviour and verifies almost nothing.
        'jest/require-to-throw-message': 'error',
        // `toBeCalled` and friends are aliases; the canonical spellings were removed from
        // jest in v30, so this is forward compatibility as well as consistency.
        'jest/no-alias-methods': 'error',
        // `toHaveBeenCalledTimes(0)` states "not called" as arithmetic. The fix is
        // `.not.toHaveBeenCalled()`, and real counts are left alone.
        'jest/prefer-to-have-been-called': 'error',
        // Reaching into `.mock.calls` and measuring its length describes an array's size
        // instead of naming the expectation.
        'jest/prefer-to-have-been-called-times': 'error',

        // A5 — an assertion that never runs, or never asserts, is worse than none: the
        // suite is green and nothing was checked.
        //
        // A missing matcher, or an async matcher whose promise is never awaited.
        'jest/valid-expect': 'error',
        // An assertion inside `if`/`catch` reports nothing when the branch is not taken.
        'jest/no-conditional-expect': 'error',
        // Assertions in a `.then()` on a promise nobody returns or awaits. Distinct from
        // `valid-expect` above, which covers the matcher rather than the chain.
        'jest/valid-expect-in-promise': 'error',
        // An `expect` in a `describe` body runs at collection time, so no test owns the
        // failure. Assertions inside helper functions are exempt.
        'jest/no-standalone-expect': 'error',

        // A5, but warn: the guideline is right and the *detector* is incomplete. The rule
        // only counts calls named in `assertFunctionNames` (default `expect`), so a suite
        // that asserts through another vocabulary reads as assertionless. WebdriverIO e2e
        // specs are the measured case: `browser.waitUntil(...)` is the assertion and the
        // rule cannot see it. Same reason `prefer-strict-equal` warns.
        'jest/expect-expect': 'warn',
      },
    },
    {
      /**
       * Test integrity: a test that silently does not run, a title that cannot identify a
       * failure, and mocks that leak across tests.
       *
       * These map to no guideline. The mapping's one-home invariant runs in one direction
       * only — every guideline needs an enforcement home, but a rule need not trace back to
       * a guideline. An off-the-shelf rule that is already installed and already right
       * costs nothing to enable and does not need a paragraph written for it first.
       *
       * Registering `jest` here as well as above is safe: flat config rejects two
       * *different* plugin objects under one namespace, and this is the same import.
       */
      name: 'tvrmsmith/base/test-integrity',
      files: testFiles,
      plugins: { jest },
      settings: assertionIntentSettings,
      rules: {
        // Tests that do not run. A committed `.only` skips every other test in the file
        // and the suite still reports green.
        'jest/no-focused-tests': 'error',
        // `.skip`, `xit`, and a test declared with no body. error rather than warn because
        // `eslint-disable-next-line` is the better escape hatch: it puts the reason for the
        // skip in the file, next to the skip, instead of in a log nobody reads.
        'jest/no-disabled-tests': 'error',
        // Fuzzy-matches `it(`/`describe(` in comments, so it can misjudge what it found.
        // Same escape hatch applies.
        'jest/no-commented-out-tests': 'error',

        // Titles. Two tests with one name make a red suite ambiguous; an empty, non-string,
        // block-name-prefixed or space-padded title fails to identify anything.
        'jest/no-identical-title': 'error',
        'jest/valid-title': 'error',
        // Two `beforeEach` in one `describe` is nearly always an accident.
        'jest/no-duplicate-hooks': 'error',

        // Mocks. `Date.now = jest.fn()` destroys the original and nothing restores it, so
        // the damage leaks into every later test in the file. Isolation, not style, and
        // autofixable.
        'jest/prefer-spy-on': 'error',
        // `mockImplementation(() => Promise.resolve(x))` is `mockResolvedValue(x)`.
        'jest/prefer-mock-promise-shorthand': 'error',
      },
    },
  ]
}

export default createBase()
