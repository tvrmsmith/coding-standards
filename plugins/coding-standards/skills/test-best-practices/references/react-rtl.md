# React Testing Library (RTL) Best Practices

Canonical source: Kent C. Dodds — [Common Mistakes with React Testing Library](https://kentcdodds.com/blog/common-mistakes-with-react-testing-library).

Technology-specific patterns implementing the principles from the parent skill.
Examples are TypeScript + React and are runner-agnostic — they work under vitest and Jest alike.
`@testing-library/jest-dom` matchers apply either way; under vitest import them in setup with
`import '@testing-library/jest-dom/vitest'`.

## Use `screen` for Queries

Don't destructure queries from `render`. `screen` exposes every query, needs no
maintenance as you add queries, and gets editor autocomplete. `screen.debug()` is
always available.

```tsx
// BAD — destructured queries must be kept in sync
const { getByRole } = render(<Login />);
getByRole('button', { name: /submit/i });

// GOOD — screen has every query
render(<Login />);
screen.getByRole('button', { name: /submit/i });
```

## Don't Name the Render Result `wrapper`

`testing-library/render-result-naming-convention`.

`wrapper` is enzyme cruft — the return value wraps nothing. Destructure what you
need, or call it `view`.

```tsx
// BAD
const wrapper = render(<Login />);
wrapper.rerender(<Login disabled />);

// GOOD — destructure, or name it `view`
const { rerender } = render(<Login />);
rerender(<Login disabled />);
```

## Don't Manually Call `cleanup`

`testing-library/no-manual-cleanup`.

Cleanup runs automatically (vitest, Jest, etc. via `afterEach`). Importing and
calling `cleanup` yourself is redundant.

```tsx
// BAD
import { render, cleanup } from '@testing-library/react';
afterEach(cleanup);

// GOOD — nothing needed
import { render } from '@testing-library/react';
```

## Prefer `userEvent` over `fireEvent`

`testing-library/prefer-user-event` + `testing-library/prefer-user-event-setup`.

`userEvent` simulates real interactions (a `type` fires keydown/keypress/keyup per
character, focus, etc.) — `fireEvent` dispatches one raw event. Call
`userEvent.setup()` first and `await` every interaction.

```tsx
// BAD — single synthetic event, no real-world fidelity
fireEvent.change(screen.getByLabelText(/name/i), { target: { value: 'Ada' } });
fireEvent.click(screen.getByRole('button', { name: /save/i }));

// GOOD — setup() once, await each interaction
const user = userEvent.setup();
await user.type(screen.getByLabelText(/name/i), 'Ada');
await user.click(screen.getByRole('button', { name: /save/i }));
```

## Use jest-dom Matchers

Asserting on raw DOM properties yields cryptic failures (`expected false to be
true`). jest-dom matchers describe intent and print the element on failure.

```tsx
// BAD — opaque failure messages
expect(screen.getByRole('button').disabled).toBe(true);
expect(screen.queryByText(/done/i)).not.toBe(null);

// GOOD — expressive matchers, clear failures
expect(screen.getByRole('button')).toBeDisabled();
expect(screen.getByText(/done/i)).toBeInTheDocument();
expect(screen.getByRole('status')).toHaveTextContent(/saved/i);
```

Common matchers: `toBeInTheDocument`, `toBeDisabled`, `toBeEnabled`,
`toHaveTextContent`, `toHaveValue`, `toHaveAttribute`, `toBeChecked`, `toHaveFocus`.

## Query Priority: Accessible Queries First

Query the DOM the way users (and assistive tech) do. Prefer, in order:
`getByRole` → `getByLabelText` → `getByPlaceholderText` → `getByText` →
`getByDisplayValue`. Use `getByTestId` / `container.querySelector` only as a last
resort. `*ByRole`'s `name` option matches the accessible name (even when split
across elements), and its failures log every available role.

```tsx
// BAD — testid / container coupling to structure
render(<Login />);
const { container } = render(<Login />);
container.querySelector('.submit-btn');
screen.getByTestId('submit-btn');

// GOOD — role + accessible name, plus real text
screen.getByRole('button', { name: /submit/i });
screen.getByLabelText(/email/i);
screen.getByText(/welcome back/i); // also verifies i18n/translations applied
```

## `get*` vs `query*` vs `find*`

`testing-library/prefer-presence-queries`, `testing-library/prefer-find-by`,
`testing-library/prefer-query-by-disappearance`.

- `get*` — element is present now. Throws a helpful error (with DOM) if missing.
- `find*` — element appears asynchronously. Returns a promise; always `await`.
- `query*` — **only** for asserting absence. Returns `null` instead of throwing.

Never use `query*().toBeNull()` for presence, and don't bother with
`get*().toBeInTheDocument()` as a no-op — `get*` already throws on absence (though
an explicit assertion can document intent).

```tsx
// BAD — query* for a present element gives a weak "null" failure
expect(screen.queryByRole('button')).toBeInTheDocument();

// BAD — waitFor where find* is simpler
await waitFor(() => screen.getByText(/loaded/i));

// GOOD — get* for present, find* for async, query* for absence
screen.getByRole('button');
await screen.findByText(/loaded/i);
expect(screen.queryByText(/error/i)).not.toBeInTheDocument();
```

## Don't Wrap in Unnecessary `act()`

`testing-library/no-unnecessary-act`.

`render` and `fireEvent`/`userEvent` are already wrapped in `act`. Manual `act()`
around them is noise. A "not wrapped in act(...)" warning signals a state update
you didn't await — fix the root cause (await the async update via `findBy*` or
`waitFor`), don't silence it with `act`.

```tsx
// BAD — manual act wrapping + silenced warning
act(() => {
  render(<Profile />);
});

// GOOD — await the async update that triggers the warning
render(<Profile />);
expect(await screen.findByText(/jane doe/i)).toBeInTheDocument();
```

## Don't Test Implementation Details

Assert on rendered output and behavior, not component state, props, or instance
methods. Tests coupled to internals break on refactors that don't change behavior
and miss bugs that do.

```tsx
// BAD — reaching into internals
expect(instance.state.isOpen).toBe(true);
expect(component.props.onToggle).toHaveBeenCalled();

// GOOD — assert what the user sees after the interaction
const user = userEvent.setup();
await user.click(screen.getByRole('button', { name: /open/i }));
expect(screen.getByRole('dialog')).toBeInTheDocument();
```

## Always `await` Async Helpers

`findBy*`, `waitFor`, and every `userEvent` interaction return promises. Forgetting
`await` causes false passes, act warnings, and leaked work into later tests.

```tsx
// BAD — missing awaits
user.click(screen.getByRole('button'));
screen.findByText(/saved/i);
waitFor(() => expect(mockSave).toHaveBeenCalled());

// GOOD
await user.click(screen.getByRole('button'));
await screen.findByText(/saved/i);
await waitFor(() => expect(mockSave).toHaveBeenCalled());
```

`waitFor` notes: wait for a single specific assertion inside the callback — never
pass an empty callback, never run multiple assertions or side-effects in it (the
callback runs an unpredictable number of times).

## Write Fewer, Longer Tests

Group related interactions into a single longer test that walks a real user flow,
rather than splitting one flow into many tiny `it` blocks with repeated setup. See
[Write fewer, longer tests](https://kentcdodds.com/blog/write-fewer-longer-tests).

Guiding heuristic — the [Testing Trophy](https://kentcdodds.com/blog/the-testing-trophy-and-testing-classifications):
**"Write tests. Not too many. Mostly integration."** Favor integration tests that
render real component trees over isolated, mock-heavy unit tests.

## Quick Reference

| Scenario | Pattern |
|----------|---------|
| Query elements | `screen.getByRole(...)` — not destructured results |
| Render result name | destructure, or `view` — never `wrapper` |
| Cleanup | automatic — don't call `cleanup` |
| User interaction | `const user = userEvent.setup()`; `await user.click(...)` |
| Assertions | jest-dom matchers (`toBeDisabled`, `toBeInTheDocument`, ...) |
| Query selection | role → label → text → ... → testId (last resort) |
| Element present now | `getBy*` |
| Element appears async | `await findBy*` |
| Element absent | `queryBy*().not.toBeInTheDocument()` |
| act warning | await the async update — don't wrap in `act()` |
| What to assert | rendered output / behavior, not state/props/instances |
| Async helpers | always `await` `findBy*`, `waitFor`, `userEvent` |
| Strategy | fewer/longer integration tests (Testing Trophy) |
