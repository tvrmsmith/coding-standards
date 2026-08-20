import tseslint from '@typescript-eslint/eslint-plugin'
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
 * Base slice — no React, no DOM, no test runner assumed.
 *
 * Carries the two assertion guidelines that need no React: **A1, "Combine assertions on
 * the same object"** and **A4, "Don't suppress null/missing value failures"**
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
  ]
}

export default createBase()
