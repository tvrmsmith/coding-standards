import tseslint from '@typescript-eslint/eslint-plugin'
import jest from 'eslint-plugin-jest'
import regexp from 'eslint-plugin-regexp'
import sonarjs from 'eslint-plugin-sonarjs'
import tvrmsmith from 'eslint-plugin-tvrmsmith'

import { sourceFiles, testFiles } from './globs.js'

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
    {
      /**
       * TypeScript language rules, production source and tests alike.
       *
       * `typescript-eslint` was already a dependency of every preset the adoption target
       * loads, with one of its rules enabled. This turns on the ones that need no type
       * information. The typed half of the plugin stays off until the target has a
       * `projectService` config — see `emr-mexnz` there.
       *
       * Seven of these extend a core rule, which has to be switched off or both report the
       * same line. Turning it off here rather than asking the consumer to is deliberate:
       * this slice is spread last, so it wins, and a consumer that had the core rule on
       * gets the TypeScript-aware version instead of a duplicate.
       */
      name: 'tvrmsmith/base/typescript',
      files: sourceFiles,
      plugins: { '@typescript-eslint': tseslint },
      rules: {
        'default-param-last': 'off',
        'no-dupe-class-members': 'off',
        'no-invalid-this': 'off',
        'no-shadow': 'off',
        'no-unused-private-class-members': 'off',
        'no-use-before-define': 'off',
        'no-useless-constructor': 'off',

        // Bugs. A shadowed name reads as the outer one and is not; a closure created in a
        // loop captures the loop variable; a value read before its `let` reaches the
        // temporal dead zone at runtime.
        '@typescript-eslint/no-shadow': 'error',
        '@typescript-eslint/no-use-before-define': 'error',
        'no-loop-func': 'error',
        // The `@typescript-eslint` copies of these two are deprecated: the core rules
        // understand TypeScript on their own now.
        'no-loss-of-precision': 'error',

        // `f(a = 1, b)` makes the default unreachable — every caller has to pass it.
        '@typescript-eslint/default-param-last': 'error',
        // A second member with the same name silently replaces the first.
        '@typescript-eslint/no-dupe-class-members': 'error',
        // `this` outside a class or a method is `undefined` under modules.
        //
        // The option is spelled out rather than left to the default. The rule wraps ESLint's
        // core `no-invalid-this`, which destructures `context.options[0]` unguarded. ESLint 9
        // injects schema defaults into `context.options`, ESLint 8 does not, so under an
        // ESLint 8 repo the array is empty and the rule throws on load. `true` is the schema
        // default, so this changes nothing on 9 and unbreaks 8.
        '@typescript-eslint/no-invalid-this': ['error', { capIsConstructor: true }],
        // A `#private` member nothing reads is either dead or a typo at the read site.
        '@typescript-eslint/no-unused-private-class-members': 'error',
        // `constructor(private readonly x: T) { this.x = x }` assigns the parameter
        // property to itself — the shorthand already did it.
        '@typescript-eslint/no-unnecessary-parameter-property-assignment': 'error',
        // `a ?? b` where `a` is `a!`: the `!` already asserted the nullish case away, so
        // the fallback is unreachable. One of the two is wrong.
        '@typescript-eslint/no-non-null-asserted-nullish-coalescing': 'error',
        // `a! == b` parses as `(a!) == b`, which reads like `!==` and is not.
        '@typescript-eslint/no-confusing-non-null-assertion': 'error',
        // `void` outside a return position means "any value, ignored", which is `unknown`
        // or `undefined` depending on which the author meant.
        '@typescript-eslint/no-invalid-void-type': 'error',
        // An enum member computed from a non-literal is evaluated at runtime, so the enum
        // stops being a set of constants.
        '@typescript-eslint/prefer-literal-enum-member': 'error',
        // A constructor that only calls `super` with the same arguments does nothing.
        '@typescript-eslint/no-useless-constructor': 'error',
        // `@ts-ignore` suppresses an error and stays silent once the error is gone.
        // `@ts-expect-error` fails when it is no longer needed, so the suppression is
        // deleted with the bug. (This replaces `prefer-ts-expect-error`, deprecated into
        // this rule.)
        '@typescript-eslint/ban-ts-comment': 'error',
        // `import { T } from './t'` where `T` is only a type keeps the module in the
        // emitted bundle. `import type` does not, and it makes a circular type-only
        // reference legal.
        '@typescript-eslint/consistent-type-imports': 'warn',
        // `import type {} from './side-effects'` is elided, so the side effect never runs.
        '@typescript-eslint/no-import-type-side-effects': 'error',

        // Judgement calls, so warn.
        //
        // `delete obj[key]` on a non-index-signature type deoptimises the object and is
        // usually a `Map` wanting to be written.
        '@typescript-eslint/no-dynamic-delete': 'warn',
        // A class with only static members is a namespace spelled as a class.
        '@typescript-eslint/no-extraneous-class': 'warn',
        // Two overloads that differ only in an optional or rest parameter are one
        // signature written twice.
        '@typescript-eslint/unified-signatures': 'warn',
        // `interface` merges and produces better error messages; `type` is for unions,
        // mapped types and everything an interface cannot express.
        '@typescript-eslint/consistent-type-definitions': 'warn',
        // `const n: number = 1` restates what inference already knows.
        '@typescript-eslint/no-inferrable-types': 'warn',
        // A `for` loop whose index is only used to index the array is `for…of`.
        '@typescript-eslint/prefer-for-of': 'warn',
      },
    },
    {
      /**
       * Regular expressions. `eslint-plugin-regexp` parses the pattern rather than matching
       * its text, so it reports what the regex *does*: a group that can never capture, an
       * assertion that always passes, a quantifier pair that backtracks exponentially.
       *
       * The correctness rules only. The plugin's `prefer-*` and `sort-*` families rewrite a
       * working pattern into an equivalent one, and a regex is the last place to spend
       * review attention on spelling.
       */
      name: 'tvrmsmith/base/regexp',
      files: sourceFiles,
      plugins: { regexp },
      rules: {
        // ReDoS. A pattern with polynomial or exponential backtracking hangs the process on
        // an input an attacker picks.
        'regexp/no-super-linear-backtracking': 'error',

        // The pattern does not do what it looks like it does.
        'regexp/no-contradiction-with-assertion': 'error',
        'regexp/no-misleading-capturing-group': 'error',
        'regexp/no-misleading-unicode-character': 'error',
        'regexp/no-optional-assertion': 'error',
        'regexp/no-potentially-useless-backreference': 'error',
        'regexp/no-useless-assertions': 'error',
        'regexp/no-useless-backreference': 'error',
        'regexp/optimal-lookaround-quantifier': 'error',
        'regexp/no-lazy-ends': 'error',

        // Parts that match nothing, or nothing useful.
        'regexp/no-empty-alternative': 'error',
        'regexp/no-empty-capturing-group': 'error',
        'regexp/no-empty-character-class': 'error',
        'regexp/no-empty-group': 'error',
        'regexp/no-empty-lookarounds-assertion': 'error',
        'regexp/no-zero-quantifier': 'error',

        // Escapes and flags that mean something other than they appear to.
        'regexp/no-escape-backspace': 'error',
        'regexp/no-invisible-character': 'error',
        'regexp/no-obscure-range': 'error',
        'regexp/no-non-standard-flag': 'error',
        'regexp/no-legacy-features': 'error',
        'regexp/strict': 'error',

        // Wrong at the call site rather than in the pattern.
        'regexp/no-invalid-regexp': 'error',
        'regexp/no-missing-g-flag': 'error',
        'regexp/no-useless-dollar-replacements': 'error',

        // warn: each is a strong smell that occasionally reads correctly.
        'regexp/confusing-quantifier': 'warn',
        'regexp/no-dupe-characters-character-class': 'warn',
        'regexp/no-dupe-disjunctions': 'warn',
        'regexp/no-unused-capturing-group': 'warn',
      },
    },
    {
      /**
       * SonarJS bug detectors, production source and tests alike.
       *
       * Sonar's rule set is the largest of the four packages here and the least overlapping:
       * these are dataflow and control-flow findings core ESLint has no equivalent for.
       * Restricted to `meta.type === 'problem'` — Sonar also ships naming conventions, line
       * counts and complexity thresholds, which are somebody else's opinion, not a bug.
       *
       * Sonar's type-aware rules are all off here for the same reason the typed
       * `typescript-eslint` rules are.
       */
      name: 'tvrmsmith/base/sonarjs-bugs',
      files: sourceFiles,
      plugins: { sonarjs },
      rules: {
        // The code says something that cannot be what was meant.
        'sonarjs/no-identical-expressions': 'error',
        'sonarjs/no-identical-conditions': 'error',
        'sonarjs/no-all-duplicated-branches': 'error',
        'sonarjs/no-element-overwrite': 'error',
        'sonarjs/no-useless-increment': 'error',
        'sonarjs/non-existent-operator': 'error',
        'sonarjs/for-loop-increment-sign': 'error',
        'sonarjs/no-floating-point-equality': 'error',

        // The value goes nowhere.
        'sonarjs/no-unthrown-error': 'error',
        'sonarjs/constructor-for-side-effects': 'error',
        'sonarjs/no-use-of-empty-return-value': 'error',
        'sonarjs/generator-without-yield': 'error',

        // The call or reference is wrong.
        'sonarjs/no-extra-arguments': 'error',
        'sonarjs/no-literal-call': 'error',
        'sonarjs/updated-const-var': 'error',
        'sonarjs/no-delete-var': 'error',

        // Names that are not what they appear to be.
        'sonarjs/no-globals-shadowing': 'error',
        'sonarjs/no-built-in-override': 'error',
        'sonarjs/no-function-declaration-in-block': 'error',

        // Control flow.
        'sonarjs/comma-or-logical-or-case': 'error',

        // warn: right most of the time, and each has a shape where the author meant it.
        //
        // A `for…in` that acts on inherited keys.
        'sonarjs/for-in': 'warn',
        // A parameter or caught exception overwritten before its initial value is read.
        'sonarjs/no-parameter-reassignment': 'warn',
        // A collection that is only ever read, never filled.
        'sonarjs/no-empty-collection': 'warn',
        // A `/g` regex held in a variable carries `lastIndex` between calls, so the second
        // `test` on the same string returns `false`.
        'sonarjs/stateful-regex': 'warn',
      },
    },
    {
      /**
       * SonarJS security rules that decide from the code alone, without a framework.
       *
       * Sonar's other security family reads Express and helmet configuration — response
       * headers, cookie flags, CORS, CSRF. Three packages at the adoption target run
       * Express, so those apply too; they are filed separately because each needs a
       * framework-shaped fixture and they are one coherent batch.
       */
      name: 'tvrmsmith/base/sonarjs-security',
      files: sourceFiles,
      plugins: { sonarjs },
      rules: {
        // Secrets in the source. All three, because they detect different shapes: a
        // password-named assignment, a high-entropy literal, and the known formats of
        // issued credentials.
        'sonarjs/no-hardcoded-passwords': 'error',
        'sonarjs/no-hardcoded-secrets': 'error',
        'sonarjs/hardcoded-secret-signatures': 'error',

        // Cryptography that does not do the job it is being asked to do.
        'sonarjs/hashing': 'error',
        'sonarjs/no-weak-cipher': 'error',
        'sonarjs/no-weak-keys': 'error',
        'sonarjs/encryption-secure-mode': 'error',
        'sonarjs/insecure-jwt-token': 'error',
        'sonarjs/pseudo-random': 'error',

        // Transport that is not protected, or protected and then unchecked.
        'sonarjs/weak-ssl': 'error',
        'sonarjs/unverified-certificate': 'error',
        'sonarjs/unverified-hostname': 'error',
        'sonarjs/no-clear-text-protocols': 'error',

        // Executing something the process should not.
        'sonarjs/code-eval': 'error',
        'sonarjs/os-command': 'error',
        'sonarjs/no-os-command-from-path': 'error',
        'sonarjs/xml-parser-xxe': 'error',

        // The filesystem and the world.
        'sonarjs/file-permissions': 'error',
        'sonarjs/publicly-writable-directories': 'error',

        // Leaking.
        'sonarjs/confidential-information-logging': 'error',
        'sonarjs/production-debug': 'error',
        'sonarjs/link-with-target-blank': 'error',

        // warn: a real prompt the user sees, and sometimes the feature.
        'sonarjs/no-intrusive-permissions': 'warn',
      },
    },
    {
      /**
       * SonarJS test-integrity rules, alongside the `jest` ones above. No overlap with them:
       * `sonarjs/no-exclusive-tests` duplicates `jest/no-focused-tests` and is off, and
       * `sonarjs/async-test-assertions` duplicates `jest/valid-expect`.
       *
       * `sonarjs/no-same-argument-assert` is off for a different reason: it calls
       * `isImported(context)` for *chai* and returns no visitors otherwise, so in a
       * jest/vitest repo it can never report.
       *
       * The two suite-shape rules below gate the same way, on jest, mocha, jasmine or
       * cypress being imported or a package.json dependency. A vitest package that leans on
       * injected globals therefore gets nothing from them. They stay on because they cost
       * nothing where they are silent, and the jest packages at the target do get them.
       */
      name: 'tvrmsmith/base/sonarjs-tests',
      files: testFiles,
      plugins: { sonarjs },
      rules: {
        // An `async` suite callback registers its tests after the runner has collected
        // them, so they never run.
        'sonarjs/synchronous-suite-callback': 'error',
        // `async (done) => …` uses both completion styles; the runner honours one.
        'sonarjs/no-mixed-completion-style': 'error',
        // A committed `cy.debug()` or `page.pause()` stalls the run.
        'sonarjs/no-debug-commands-in-ui-tests': 'error',
      },
    },
  ]
}

export default createBase()
