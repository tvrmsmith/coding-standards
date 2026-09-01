import jestDom from 'eslint-plugin-jest-dom'
import react from 'eslint-plugin-react'
import reactHooks from 'eslint-plugin-react-hooks'
import noEffect from 'eslint-plugin-react-you-might-not-need-an-effect'
import sonarjs from 'eslint-plugin-sonarjs'
import testingLibrary from 'eslint-plugin-testing-library'

import { sourceFiles, testFiles } from './globs.js'

/**
 * React/RTL slice. A non-React package should not load this — spread `base` alone there.
 *
 * Two config objects, because the guidelines they carry live in different files:
 *
 * - **tests** — `references/react-rtl.md` (D1–D8, D10, D11) and the TypeScript column of
 *   A2, "assertions communicate meaning" (D5, the jest-dom matchers).
 * - **source** — `coding-standards/react.md`, "You might not need an Effect" (F1–F8, F10, F11).
 *
 * Runner-agnostic: `eslint-plugin-testing-library` and `eslint-plugin-jest-dom` key off
 * `@testing-library/*` imports and `expect(...)` calls, not off vitest or jest.
 *
 * Severity convention throughout: **error** for mechanical shapes with one right answer,
 * **warn** where the guideline admits a legitimate exception or the fix is a judgment call.
 */

/** D1–D8, D10, D11 + A2/D5 — Testing Library and jest-dom, test files only. */
const reactTests = {
  name: 'tvrmsmith/react/testing-library',
  files: testFiles,
  plugins: {
    'testing-library': testingLibrary,
    'jest-dom': jestDom,
  },
  rules: {
    // D1 — query through `screen`, not the render result.
    'testing-library/prefer-screen-queries': 'error',

    // D2 — `wrapper` is enzyme vocabulary and hides what was rendered.
    // warn: a naming convention, no correctness consequence.
    'testing-library/render-result-naming-convention': 'warn',

    // D3 — RTL auto-cleans; a manual `cleanup()` means the test framework is
    // misunderstood or the auto-cleanup was disabled behind the test's back.
    'testing-library/no-manual-cleanup': 'error',

    // D4 — `userEvent` fires the full event sequence a real user produces;
    // `fireEvent` fires one synthetic event and passes tests that would fail in a
    // browser. (This pair replaces the `no-restricted-imports` ban on `fireEvent`
    // the earlier plan called for — off the shelf covers it.)
    'testing-library/prefer-user-event': 'error',
    'testing-library/prefer-user-event-setup': 'error',

    // D6 — testId is the last-resort query, below role/label/placeholder/text.
    // warn: it *is* a legitimate last resort when nothing accessible identifies the
    // node. The ordering among the accessible queries is not lintable at all and
    // stays review-only.
    'testing-library/no-test-id-queries': 'warn',
    // D6 — reaching through `container` or raw DOM nodes bypasses the query priority
    // entirely rather than sitting at the bottom of it. error.
    'testing-library/no-container': 'error',
    'testing-library/no-node-access': 'error',

    // D7 — `get*` asserts presence, `find*` awaits it, `query*` is for absence only.
    // Using `query*` for presence turns a missing element into `expect(null)`; using
    // `get*` inside `waitForElementToBeRemoved` throws before the wait even starts.
    'testing-library/prefer-presence-queries': 'error',
    'testing-library/prefer-find-by': 'error',
    'testing-library/prefer-query-by-disappearance': 'error',

    // D8 — RTL already wraps renders and events in `act()`; wrapping again hides
    // missing awaits.
    'testing-library/no-unnecessary-act': 'error',

    // D10 — a forgotten `await` makes the assertion run against the pre-update tree,
    // and the rejection surfaces in an unrelated test. Both directions: awaiting a
    // sync query is the same misunderstanding pointed the other way.
    'testing-library/await-async-queries': 'error',
    'testing-library/await-async-utils': 'error',
    'testing-library/await-async-events': 'error',
    'testing-library/no-await-sync-queries': 'error',
    'testing-library/no-await-sync-events': 'error',

    // D11 — `waitFor` retries its callback: multiple assertions mean the first
    // failure masks the rest, side effects run repeatedly, and a snapshot inside a
    // retry loop is nondeterministic.
    // The empty-callback case is carried by the `noEmptyWaitForCallback` selector in
    // the base slice — `no-wait-for-empty-callback` was removed in
    // eslint-plugin-testing-library 6.0.0, and `no-restricted-syntax` is a core rule
    // that can only be configured in one place.
    'testing-library/no-wait-for-multiple-assertions': 'error',
    'testing-library/no-wait-for-side-effects': 'error',
    'testing-library/no-wait-for-snapshot': 'error',

    // A2 / D5 — jest-dom matchers, all 12. `expect(el).toBeInTheDocument()` fails with
    // "element not found"; the hand-rolled `expect(el).not.toBeNull()` fails with
    // "expected null not to be null". Every one is autofixable and mechanical: error.
    'jest-dom/prefer-checked': 'error',
    'jest-dom/prefer-empty': 'error',
    'jest-dom/prefer-enabled-disabled': 'error',
    'jest-dom/prefer-focus': 'error',
    'jest-dom/prefer-in-document': 'error',
    'jest-dom/prefer-pressed': 'error',
    'jest-dom/prefer-required': 'error',
    'jest-dom/prefer-to-have-attribute': 'error',
    'jest-dom/prefer-to-have-class': 'error',
    'jest-dom/prefer-to-have-style': 'error',
    'jest-dom/prefer-to-have-text-content': 'error',
    'jest-dom/prefer-to-have-value': 'error',
  },
}

/** F1–F8, F10, F11 — "You might not need an Effect", production source and tests alike. */
const reactSource = {
  name: 'tvrmsmith/react/effects',
  files: sourceFiles,
  plugins: {
    'react-you-might-not-need-an-effect': noEffect,
    'react-hooks': reactHooks,
  },
  rules: {
    // F1 — state derived from props/state: calculate during render.
    'react-you-might-not-need-an-effect/no-derived-state': 'warn',
    // F2 — chains of computations, each Effect triggering the next.
    'react-you-might-not-need-an-effect/no-chain-state-updates': 'warn',
    // F3 — interaction-driven work belongs in the handler, not an Effect.
    'react-you-might-not-need-an-effect/no-event-handler': 'warn',
    // F4/F5 — notifying or feeding the parent from an Effect.
    'react-you-might-not-need-an-effect/no-pass-live-state-to-parent': 'warn',
    'react-you-might-not-need-an-effect/no-pass-data-to-parent': 'warn',
    // F6 — external stores belong in `useSyncExternalStore`.
    'react-you-might-not-need-an-effect/no-external-store-subscription': 'warn',
    // F7 — app initialization belongs at module scope / the entry point.
    'react-you-might-not-need-an-effect/no-initialize-state': 'warn',
    // F10 — resetting *all* of a component's state when a prop changes is what React's
    // `key` does. Enabled for its message, which is the only one that names `key`, but it
    // carries F10 in name only: v1.0.1 gates on `setter refs in the effect === state refs
    // in the whole component` and counts references rather than `useState` declarations,
    // so any other read or write of the state silences it — including React's own worked
    // example. The shapes that matter are caught by `no-adjust-state-on-prop-change`.
    'react-you-might-not-need-an-effect/no-reset-all-state-on-prop-change': 'warn',
    // F11 — adjusting *some* state when a prop changes. This is the complement of
    // `no-derived-state`: that rule needs the new state value to derive from a prop,
    // and this one fires precisely when it does not — `setSelection(null)` on an
    // `[items]` change, the store-an-id-and-`find` shape. Together they leave F11 with
    // no review residue.
    'react-you-might-not-need-an-effect/no-adjust-state-on-prop-change': 'warn',
    //
    // warn for all nine: each proposes a restructure rather than a local edit, and
    // the plugin's own recommended preset ships them at warn. They are also the rules
    // least likely to be installed in a consuming package, so in practice they surface at
    // commit time, in a batch — an error severity there would block commits on refactors that
    // need thought.

    // F8 — an empty or trimmed dep array fakes "run once" and silently goes stale.
    // warn: upstream's own severity, and the rule's suggested deps are occasionally
    // wrong in ways that need a human.
    'react-hooks/exhaustive-deps': 'warn',
  },
}

/**
 * The rest of React: JSX correctness, the render-time bugs, and the two DOM escape hatches
 * that are also XSS holes.
 *
 * `eslint-plugin-react` predates hooks and TypeScript, and about half of it is aimed at
 * neither. Off here, and why:
 *
 * - **PropTypes** (`prop-types`, `require-default-props`, the `forbid-*` and `sort-*`
 *   families). TypeScript is the prop contract; PropTypes is not installed.
 * - **The classic React runtime** (`react-in-jsx-scope`, `jsx-uses-react`). The automatic
 *   runtime has been the default since React 17.
 * - **`jsx-uses-vars`**. It only teaches core `no-unused-vars` to see JSX, which
 *   `typescript-eslint`'s own rule already does.
 * - **Class components** (`sort-comp`, `state-in-constructor`, `no-access-state-in-setstate`
 *   and the lifecycle rules). Kept only where the code being flagged is *deprecated* rather
 *   than merely class-shaped, since that is a migration signal rather than a style.
 * - **Formatting** (the `jsx-indent`, `jsx-spacing` and `jsx-wrap-multilines` families).
 *   Prettier owns this.
 */
const reactJsx = {
  name: 'tvrmsmith/react/jsx',
  files: sourceFiles,
  plugins: { react, sonarjs },
  settings: {
    /**
     * Pinned rather than `'detect'`. Detection resolves `react/package.json` from the
     * *config's* location, and this preset is installed machine-locally outside the package
     * it lints, so detection finds nothing and every version-gated rule silently downgrades.
     * Only `no-deprecated` and `no-unsafe` read this, and both want the current major.
     */
    react: { version: '19.0' },
  },
  rules: {
    // A list rendered without `key` remounts rows on every reorder, losing their state.
    'react/jsx-key': 'error',
    // The second one wins and the first is silently dropped. Same for a repeated spread.
    'react/jsx-no-duplicate-props': 'error',
    'react/jsx-props-no-spread-multi': 'error',
    // `{/* comment */}` without the braces renders the comment as visible text.
    'react/jsx-no-comment-textnodes': 'error',
    // `<div>{count && <List />}</div>` renders a literal `0` when the count is zero.
    // (Sonar has a rule for this too, but its version needs type information.)
    'react/jsx-no-leaked-render': 'error',
    // `this` inside a function component is `undefined`.
    'react/no-this-in-sfc': 'error',
    // `<img>` and `<br>` cannot have children; React drops them.
    'react/void-dom-elements-no-children': 'error',
    // `children` alongside `dangerouslySetInnerHTML` is discarded.
    'react/no-danger-with-children': 'error',
    // `children` passed as a prop rather than between the tags is ignored by some
    // components and confusing in the rest.
    'react/no-children-prop': 'error',
    // `style="color: red"` throws; React wants an object.
    'react/style-prop-object': 'error',
    // `checked` with no `onChange` and no `readOnly` makes an input that cannot be typed in.
    'react/checked-requires-onchange-or-readonly': 'error',
    // `class` instead of `className`, `for` instead of `htmlFor`. React ignores them.
    'react/no-unknown-property': 'error',
    // A bare `'` or `>` in markup is a mis-paste more often than it is intended text.
    'react/no-unescaped-entities': 'error',
    // A component defined inside another component is a new type on every render, so React
    // unmounts and remounts the subtree and every bit of its state is lost.
    'react/no-unstable-nested-components': 'error',

    // Security.
    //
    // `target="_blank"` without `rel="noreferrer"` hands the opened page a handle to this one.
    'react/jsx-no-target-blank': 'error',
    // `href="javascript:…"` executes on click.
    'react/jsx-no-script-url': 'error',

    // Deprecated and removed APIs. These are migration signals rather than style: each names
    // something that no longer works, or is about to stop.
    'react/no-deprecated': 'error',
    'react/no-unsafe': 'error',
    'react/no-find-dom-node': 'error',
    'react/no-string-refs': 'error',
    'react/no-is-mounted': 'error',
    'react/no-render-return-value': 'error',
    'react/no-direct-mutation-state': 'error',
    'react/require-render-return': 'error',

    // warn: each has a legitimate use, and the fix is a judgement call.
    //
    // An index key is correct only for a list that never reorders, filters or splices.
    'react/no-array-index-key': 'warn',
    // A context value built inline is a new object every render, so every consumer rerenders.
    'react/jsx-no-constructed-context-values': 'warn',
    // `= {}` as a default is a new object every render, which restarts any effect
    // depending on it.
    'react/no-object-type-as-default-prop': 'warn',
    // A fragment wrapping one child does nothing.
    'react/jsx-no-useless-fragment': 'warn',
    // A `<button>` inside a form defaults to `type="submit"`.
    'react/button-has-type': 'warn',
    // `dangerouslySetInnerHTML` is sometimes the answer, and always worth a second look.
    'react/no-danger': 'warn',
    // An `<iframe>` with no `sandbox` runs the embedded page with full privileges.
    'react/iframe-missing-sandbox': 'warn',

    // Sonar's two React rules, neither of which the plugins above cover. Both describe a
    // render loop rather than a style.
    //
    // A setter called during render, not in an effect or a handler, loops forever.
    'sonarjs/no-hook-setter-in-body': 'error',
    // `setX(x)` with the current value: React bails out, so the line does nothing.
    'sonarjs/no-useless-react-setstate': 'error',
  },
}

export default [reactTests, reactSource, reactJsx]
