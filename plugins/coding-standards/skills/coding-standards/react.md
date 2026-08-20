# React effects — you might not need an Effect
Grounded in react.dev — [You Might Not Need an Effect](https://react.dev/learn/you-might-not-need-an-effect).

Default stance: an Effect is probably wrong. Effects are an escape hatch for
**synchronizing with an external system** (network, DOM, non-React widget). No
external system involved → no Effect. Use one only for code that must run
*because the component was displayed*. Caused by an interaction → event handler.
Producible from props/state → calculate it. Empty/omitted deps faking "run once"
= smell (`react-hooks/exhaustive-deps`).

**Enforcement markers.** Each anti-pattern names the ESLint rule that catches it — all from
`eslint-plugin-react-you-might-not-need-an-effect` v1.0.1 (9 rules, all enabled) unless stated
otherwise.
Items tagged **[review-only]** have no rule and are caught only by reading the code.

Anti-patterns → fix (react.dev section names):
- Updating state from props/state → calculate during render, don't mirror to state.
  (`no-derived-state`)
- Caching expensive calculations → `useMemo(fn, deps)` (only if measurably
  expensive per `console.time`; rarely is). **[review-only]** — "measurably expensive" is a
  measurement judgment, not a lintable shape.
- Resetting all state on a prop change → pass a different `key` to remount.
  (`no-reset-all-state-on-prop-change` names the `key` fix, but only fires when the state
  is referenced nowhere but the reset; in practice the shape is caught by
  `no-adjust-state-on-prop-change`)
- Adjusting some state on a prop change → calculate during render (store an id,
  `find` the object); no Effect. (`no-derived-state` when the new value derives from a
  prop, `no-adjust-state-on-prop-change` when it does not — together they cover it,
  including the store-an-id-and-`find` shape)
- Sharing logic between event handlers → extract a shared function. (`no-event-handler`)
- Sending a POST → interaction-driven POST goes in the handler; only
  display-driven POST (e.g. analytics `visit_form`) stays in an Effect.
  **[review-only]** — the interaction/display distinction is semantic.
- Chains of computations → calculate in render + do all next-state updates in the
  one handler that started it. (`no-chain-state-updates`)
- Initializing the application → module scope or entry point (`App.js`), not a
  component Effect. (`no-initialize-state`)
- Notifying parent about state changes → update both in the same event, or lift
  state up (controlled). "Whenever you try to keep two different state variables
  synchronized, try lifting state up instead." (`no-pass-live-state-to-parent`)
- Passing data to the parent → parent fetches, passes down. (`no-pass-data-to-parent`)
- Subscribing to an external store → `useSyncExternalStore`, not manual
  `addEventListener` + state in an Effect. (`no-external-store-subscription`)

Legit Effects — syncing with something outside React. **[review-only]** throughout:
recognising a legitimate Effect is the whole judgment, and no rule attempts it.
- Fetching data — **with a cleanup/ignore flag** for race conditions; prefer a
  framework's fetching, a cache lib, or a custom Hook over raw `useEffect`.
- Imperative DOM not expressible in render (`el.play()`, `dialog.showModal()`).
- Subscribing to browser/3rd-party APIs, legacy widgets.
- Connecting to an external server/store (chat connect/disconnect in cleanup).
- Timers/intervals tied to the component being on screen.
- Display-driven analytics logging (not tied to a click).
