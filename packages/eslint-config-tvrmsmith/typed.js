import tseslint from '@typescript-eslint/eslint-plugin'
import jest from 'eslint-plugin-jest'
import sonarjs from 'eslint-plugin-sonarjs'

import { sourceFiles, testFiles } from './globs.js'

/**
 * The type-aware layer. **Not** part of the default export, and that is the whole point of
 * the file existing separately.
 *
 * Every rule here calls into the TypeScript type checker, so it needs the linted package to
 * supply `parserOptions.projectService` (or the older `project`). Where that is missing,
 * `typescript-eslint` throws with a long "you have used a rule which requires type
 * information" message and the run dies — not a rule that quietly reports nothing, a hard
 * failure. So the layer stays off until a package is ready, and readiness means exactly one
 * thing: the package already configures type information for its own linting.
 *
 * Two ways in.
 *
 *   // a package writing its own flat config
 *   import base from 'eslint-config-tvrmsmith/base'
 *   import { typedBase } from 'eslint-config-tvrmsmith/typed'
 *   export default [...packageOwnConfig, ...base, ...typedBase]
 *
 * Or, through the machine-local wrapper, nothing at all: `harness/eslint-layer.js` reads the
 * package's own config and adds this layer when it finds type information there. See the
 * wrapper for the override.
 *
 * Cost. Typed linting builds a TypeScript program, so the first file in a package is slow in
 * a way untyped linting never is. That is the reason the pre-commit path scopes to changed
 * files, and the reason this is opt-in per package rather than repo-wide.
 */

/**
 * Rules whose fix reaches outside the file being linted are `warn`, even where the rule is
 * mechanically certain. The `no-unsafe-*` family is the clearest case: the violation is here,
 * but the `any` usually comes from an untyped dependency or a boundary somewhere upstream, and
 * blocking a commit on "go type that other module first" is how a layer gets switched off.
 */
const typescript = {
  name: 'tvrmsmith/typed/typescript',
  files: sourceFiles,
  plugins: { '@typescript-eslint': tseslint },
  rules: {
    // -- Promises. The reason typed linting is worth turning on at all. -------------------
    // A promise nobody awaits swallows its rejection and the failure surfaces somewhere else.
    '@typescript-eslint/no-floating-promises': 'error',
    // `onClick={async () => …}` and `array.filter(async x => …)`: the caller ignores the promise.
    '@typescript-eslint/no-misused-promises': 'error',
    '@typescript-eslint/await-thenable': 'error',
    // An `async` function with no `await` returns a promise for no reason and hides the fact
    // that nothing in it is actually asynchronous.
    '@typescript-eslint/require-await': 'error',
    // Bare `return promise` inside `try` escapes the `catch`.
    '@typescript-eslint/return-await': 'error',
    '@typescript-eslint/no-implied-eval': 'error',
    '@typescript-eslint/prefer-promise-reject-errors': 'error',
    '@typescript-eslint/only-throw-error': 'error',
    // A rejection reason is `any` at runtime whatever the callback's annotation claims.
    '@typescript-eslint/use-unknown-in-catch-callback-variable': 'error',
    // `[1, 2].forEach(async …)` and friends: the value-returning function is discarded.
    '@typescript-eslint/strict-void-return': 'error',

    // -- Wrong at runtime, invisible without types. ----------------------------------------
    // `delete arr[0]` leaves a hole and does not shorten the array.
    '@typescript-eslint/no-array-delete': 'error',
    // `for (const i in arr)` iterates keys as strings, including inherited ones.
    '@typescript-eslint/no-for-in-array': 'error',
    // The famous `[object Object]`.
    '@typescript-eslint/no-base-to-string': 'error',
    // Spreading a Map, a Set or a promise does not do what it looks like it does.
    '@typescript-eslint/no-misused-spread': 'error',
    // A mixed enum compares and serialises inconsistently.
    '@typescript-eslint/no-mixed-enums': 'error',
    // Comparing an enum against a raw literal silently stops matching when the enum moves.
    '@typescript-eslint/no-unsafe-enum-comparison': 'error',
    '@typescript-eslint/no-unsafe-unary-minus': 'error',
    // `'a' + 1` is a string; `[] + {}` is nonsense. Both compile.
    '@typescript-eslint/restrict-plus-operands': 'error',
    // `sort()` without a comparator sorts numbers lexicographically: 1, 10, 2.
    '@typescript-eslint/require-array-sort-compare': ['error', { ignoreStringArrays: true }],
    // A getter and setter pair whose types disagree round-trips wrong.
    '@typescript-eslint/related-getter-setter-pairs': 'error',
    // A missing union member in a switch is a bug the day the union grows.
    '@typescript-eslint/switch-exhaustiveness-check': [
      'error',
      { considerDefaultExhaustiveForUnions: true },
    ],
    // `arr.map(this.format)` loses `this`.
    '@typescript-eslint/unbound-method': 'error',
    // `${obj}` where obj is not a string. `allowNumber` because interpolating a number is
    // both universal and correct; the rule's default of banning it is the noisy part.
    '@typescript-eslint/restrict-template-expressions': ['error', { allowNumber: true }],
    // A default that can never apply because the parameter is not optional.
    '@typescript-eslint/no-useless-default-assignment': 'error',

    // -- `any` leaking through the type system. Warn: the fix is usually upstream. ---------
    '@typescript-eslint/no-unsafe-argument': 'warn',
    '@typescript-eslint/no-unsafe-assignment': 'warn',
    '@typescript-eslint/no-unsafe-call': 'warn',
    '@typescript-eslint/no-unsafe-member-access': 'warn',
    '@typescript-eslint/no-unsafe-return': 'warn',
    // An `as` that narrows is a claim the checker cannot verify. Warn, because at a parse
    // boundary it is sometimes the honest spelling.
    '@typescript-eslint/no-unsafe-type-assertion': 'warn',

    // -- Dead code and redundancy the checker can prove. -----------------------------------
    // A condition the type says can never change: either the check is dead or the type lies.
    '@typescript-eslint/no-unnecessary-condition': 'warn',
    '@typescript-eslint/no-unnecessary-type-assertion': 'error',
    '@typescript-eslint/no-unnecessary-type-conversion': 'error',
    '@typescript-eslint/no-unnecessary-boolean-literal-compare': 'warn',
    '@typescript-eslint/no-unnecessary-template-expression': 'warn',
    '@typescript-eslint/no-unnecessary-type-arguments': 'warn',
    '@typescript-eslint/no-unnecessary-type-parameters': 'warn',
    '@typescript-eslint/no-unnecessary-qualifier': 'warn',
    '@typescript-eslint/no-duplicate-type-constituents': 'warn',
    // `string | 'literal'` and `any | T`: the second constituent does nothing, or erases
    // the first.
    '@typescript-eslint/no-redundant-type-constituents': 'warn',
    // Using code already marked `@deprecated`, which is otherwise only visible on hover.
    '@typescript-eslint/no-deprecated': 'warn',

    // -- Shapes with one right answer, now provable. ---------------------------------------
    '@typescript-eslint/dot-notation': 'error',
    '@typescript-eslint/prefer-includes': 'warn',
    '@typescript-eslint/prefer-string-starts-ends-with': 'warn',
    '@typescript-eslint/prefer-find': 'warn',
    '@typescript-eslint/prefer-regexp-exec': 'warn',
    '@typescript-eslint/prefer-reduce-type-parameter': 'warn',
    '@typescript-eslint/prefer-return-this-type': 'warn',
    '@typescript-eslint/prefer-nullish-coalescing': 'warn',
    '@typescript-eslint/prefer-optional-chain': 'warn',
    '@typescript-eslint/prefer-readonly': 'warn',
    // The type-aware half of the pair; the untyped `consistent-type-imports` is in base.
    '@typescript-eslint/consistent-type-exports': 'warn',
    // A function that returns a value on one path and falls off the end on another.
    '@typescript-eslint/consistent-return': 'warn',
    // `void`-typed expressions used as values: almost always a forgotten `await` or return.
    // `ignoreArrowShorthand` stays on because `onClick={() => setOpen(true)}` is how every React
    // codebase writes a handler, and the shorthand return is never the bug the rule is after.
    '@typescript-eslint/no-confusing-void-expression': ['warn', { ignoreArrowShorthand: true }],
    '@typescript-eslint/no-meaningless-void-operator': 'warn',
  },
}

/**
 * SonarJS's type-aware rules, minus everything another plugin here already owns. Sonar
 * duplicates its own untyped rules, `eslint-plugin-regexp`'s whole regex family, and a dozen
 * `typescript-eslint` rules; enabling both copies double-reports and the messages disagree
 * about the fix.
 *
 * Dropped as duplicates: `deprecation`, `no-array-delete`, `no-alphabetical-sort`,
 * `prefer-regexp-exec`, `no-inconsistent-returns`, `void-use`, `assertions-in-tests`,
 * `no-for-in-iterable` (the array case is `@typescript-eslint/no-for-in-array`),
 * `non-number-in-arithmetic-expression` (subsumed by the two NaN rules below), the
 * eighteen regex rules, and `jsx-no-leaked-render` (the untyped `react` copy is in the React
 * slice). `useless-string-operation` and `web-sql-database` are deprecated upstream.
 */
const sonar = {
  name: 'tvrmsmith/typed/sonarjs',
  files: sourceFiles,
  plugins: { sonarjs },
  rules: {
    // -- Type confusion the checker can see and the reader cannot. --------------------------
    'sonarjs/null-dereference': 'error',
    'sonarjs/different-types-comparison': 'error',
    'sonarjs/argument-type': 'error',
    'sonarjs/in-operator-type-error': 'error',
    'sonarjs/no-in-misuse': 'error',
    'sonarjs/operation-returning-nan': 'error',
    'sonarjs/values-not-convertible-to-numbers': 'error',
    'sonarjs/no-incorrect-string-concat': 'error',
    'sonarjs/new-operator-misuse': 'error',
    // `arr.length >= 0` and `set.size < 0`: conditions that cannot do what they say.
    'sonarjs/no-collection-size-mischeck': 'error',
    'sonarjs/index-of-compare-to-positive-number': 'error',

    // -- Results thrown away. ---------------------------------------------------------------
    // `str.trim()` on its own line: strings are immutable and nothing was assigned.
    'sonarjs/no-ignored-return': 'error',
    // `arr.reverse()` reads like a copy and mutates in place.
    'sonarjs/no-misleading-array-reverse': 'error',
    'sonarjs/reduce-initial-value': 'error',
    'sonarjs/array-callback-without-return': 'error',

    // -- Async. -----------------------------------------------------------------------------
    // A `try` around an unawaited promise catches nothing.
    'sonarjs/no-try-promise': 'error',
    // A constructor cannot await, so the object escapes before its async work finishes.
    'sonarjs/no-async-constructor': 'error',

    // -- Security. The untyped security family is in base; these need types. ----------------
    'sonarjs/sql-queries': 'error',
    'sonarjs/post-message': 'error',
    'sonarjs/disabled-resource-integrity': 'error',
    'sonarjs/disabled-auto-escaping': 'error',
    'sonarjs/dompurify-unsafe-config': 'error',

    // -- Judgement calls. --------------------------------------------------------------------
    // Positional arguments whose names match the parameters in a different order. A heuristic,
    // and a good one, but it reads names.
    'sonarjs/arguments-order': 'warn',
    'sonarjs/bitwise-operators': 'warn',
    'sonarjs/strings-comparison': 'warn',
    'sonarjs/no-undefined-argument': 'warn',
    'sonarjs/no-associative-arrays': 'warn',
    'sonarjs/class-prototype': 'warn',
    'sonarjs/no-require-or-define': 'warn',
    'sonarjs/unused-import': 'warn',
    'sonarjs/function-return-type': 'warn',
    'sonarjs/no-return-type-any': 'warn',
    'sonarjs/no-useless-intersection': 'warn',
    'sonarjs/no-redundant-optional': 'warn',
    'sonarjs/prefer-immediate-return': 'warn',
    'sonarjs/no-small-switch': 'warn',
    // A boolean parameter that picks between two behaviours: `render(true)` at the call site
    // says nothing.
    'sonarjs/no-selector-parameter': 'warn',
  },
}

/**
 * Typed test rules. `eslint-plugin-jest` carries four, and they close the A2/A5 gaps the
 * untyped rules cannot reach — every one of them needs to know whether a value is a promise
 * or an Error.
 */
const tests = {
  name: 'tvrmsmith/typed/tests',
  files: testFiles,
  plugins: { jest, sonarjs },
  rules: {
    // A5. `expect(promise).toBe(…)` without `resolves` asserts on the promise object, and
    // `await expect(value).resolves` on a non-promise never runs the matcher.
    'jest/valid-expect-with-promise': 'error',
    // A2. `toEqual` on an Error compares only the message, so a `TypeError` passes as a
    // `RangeError` with the same text.
    'jest/no-error-equal': 'error',
    // A5. An assertion the types prove can never fail.
    'jest/no-unnecessary-assertion': 'warn',
    // The jest-aware copy of `@typescript-eslint/unbound-method`: it knows `jest.fn()` and
    // `expect(obj.method)` are fine. The base rule is switched off here so they do not both
    // report the same line.
    '@typescript-eslint/unbound-method': 'off',
    'jest/unbound-method': 'error',
    // `expect(a).toBe(b)` where the two types cannot overlap: the assertion is unfalsifiable.
    'sonarjs/no-incompatible-assertion-types': 'error',
  },
}

/**
 * The one type-aware React rule worth having. Sonar's `jsx-no-leaked-render` is the other,
 * and the untyped `react/jsx-no-leaked-render` in the React slice already covers it.
 */
const react = {
  name: 'tvrmsmith/typed/react',
  files: sourceFiles,
  plugins: { sonarjs },
  rules: {
    // Mutating a prop is a React bug the type system will happily allow otherwise.
    'sonarjs/prefer-read-only-props': 'warn',
  },
}

/** The typed layer for a package with no React. Pairs with `eslint-config-tvrmsmith/base`. */
export const typedBase = [typescript, sonar, tests]

/** The React half, kept separate so `typedBase` stays React-free the way `base` is. */
export const typedReact = [react]

export default [...typedBase, ...typedReact]
