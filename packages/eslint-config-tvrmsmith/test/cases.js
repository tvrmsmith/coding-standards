import { noEmptyWaitForCallback, noOptionalChainInExpect } from '../base.js'

/**
 * At least one case per enabled rule: a violating sample the rule must fire on, and a
 * compliant sample it must stay silent on.
 *
 * `scope` picks which fixture file the snippet is linted as — `test` for the test-file
 * globs, `source` for production source — which is also how the slices' `files` scoping
 * gets exercised.
 *
 * `no-restricted-syntax` carries several guidelines at once, so its cases name the
 * `selector` they exercise and are asserted on the reported message; matching on the
 * rule id alone would let one selector's case pass on another selector's report.
 */

const TEST_PREAMBLE = `
import { render, screen, act, waitFor, waitForElementToBeRemoved, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
declare const it: (name: string, fn: () => unknown) => void
declare const afterEach: (fn: () => unknown) => void
declare const expect: any
declare const App: () => JSX.Element
declare const getUser: () => { name: string, age: number }
declare const other: { name?: string }
declare const el: HTMLInputElement
declare const noop: () => void
declare const assertSaved: () => void
`

const SOURCE_PREAMBLE = `
import { useState, useEffect, useSyncExternalStore } from 'react'
declare const store: { subscribe: (fn: () => void) => () => void, getSnapshot: () => number }
declare const useQuery: (key: string) => number
declare const navigator: { onLine: boolean }
declare const window: { addEventListener: (e: string, fn: () => void) => void, removeEventListener: (e: string, fn: () => void) => void }
declare const compute: (n: number) => number
interface Item { id: number }
`

/**
 * @type {{
 *   rule: string,
 *   selector?: { selector: string, message: string },
 *   scope: 'test' | 'source',
 *   violating: string,
 *   compliant: string,
 * }[]}
 */
export const cases = [
  // ---- base: A4, don't suppress null/missing value failures ----
  {
    rule: '@typescript-eslint/no-non-null-assertion',
    scope: 'test',
    violating: `it('a', () => { const user = getUser(); expect(user!.name).toBe('Ada') })`,
    compliant: `it('a', () => { const user = getUser(); expect(user.name).toBe('Ada') })`,
  },

  // ---- base: A1, combine assertions on the same object (the one custom rule) ----
  {
    rule: 'tvrmsmith/combine-assertions-on-same-object',
    scope: 'test',
    violating: `it('a', () => {
  const user = getUser()
  expect(user.name).toBe('Ada')
  expect(user.age).toBe(36)
})`,
    compliant: `it('a', () => {
  const user = getUser()
  expect(user).toMatchObject({ name: 'Ada', age: 36 })
})`,
  },
  {
    rule: 'no-restricted-syntax',
    selector: noOptionalChainInExpect,
    scope: 'test',
    violating: `it('a', () => { const user = getUser(); expect(user?.name).toBe('Ada') })`,
    // A `?.` in the *expected* value is outside the expect(...) call's arguments and
    // must not be flagged.
    compliant: `it('a', () => { const user = getUser(); expect(user.name).toBe(other?.name) })`,
  },

  // ---- D11: the empty-callback half, the selector standing in for the rule
  // `testing-library/no-wait-for-empty-callback` removed in the plugin's 6.0.0 ----
  {
    rule: 'no-restricted-syntax',
    selector: noEmptyWaitForCallback,
    scope: 'test',
    violating: `it('a', async () => { await waitFor(() => {}) })`,
    compliant: `it('a', async () => { await waitFor(() => { expect(el).toBeEnabled() }) })`,
  },
  {
    rule: 'no-restricted-syntax',
    selector: noEmptyWaitForCallback,
    scope: 'test',
    violating: `it('a', async () => { await waitForElementToBeRemoved(function () {}) })`,
    // An empty callback passed to something that is not a wait util is none of the
    // selector's business.
    compliant: `it('a', () => { el.addEventListener('x', () => {}) })`,
  },
  {
    rule: 'no-restricted-syntax',
    selector: noEmptyWaitForCallback,
    scope: 'test',
    violating: `it('a', async () => { await waitFor(noop) })`,
    // The removed rule matched the identifier `noop` by name only; any other
    // callback reference is outside both it and this selector.
    compliant: `it('a', async () => { await waitFor(assertSaved) })`,
  },

  // ---- D1 ----
  {
    rule: 'testing-library/prefer-screen-queries',
    scope: 'test',
    violating: `it('a', () => { const { getByRole } = render(<App />); getByRole('button') })`,
    compliant: `it('a', () => { render(<App />); screen.getByRole('button') })`,
  },

  // ---- D2 ----
  {
    rule: 'testing-library/render-result-naming-convention',
    scope: 'test',
    violating: `it('a', () => { const wrapper = render(<App />); return wrapper })`,
    compliant: `it('a', () => { const view = render(<App />); return view })`,
  },

  // ---- D3 ----
  {
    rule: 'testing-library/no-manual-cleanup',
    scope: 'test',
    violating: `import { cleanup } from '@testing-library/react'
afterEach(() => { cleanup() })`,
    compliant: `it('a', () => { render(<App />) })`,
  },

  // ---- D4 ----
  {
    rule: 'testing-library/prefer-user-event',
    scope: 'test',
    violating: `it('a', () => { fireEvent.click(el) })`,
    compliant: `it('a', async () => { const user = userEvent.setup(); await user.click(el) })`,
  },
  {
    rule: 'testing-library/prefer-user-event-setup',
    scope: 'test',
    violating: `it('a', async () => { await userEvent.click(el) })`,
    compliant: `it('a', async () => { const user = userEvent.setup(); await user.click(el) })`,
  },

  // ---- D6 ----
  {
    rule: 'testing-library/no-test-id-queries',
    scope: 'test',
    violating: `it('a', () => { screen.getByTestId('submit') })`,
    compliant: `it('a', () => { screen.getByRole('button', { name: 'Submit' }) })`,
  },
  {
    rule: 'testing-library/no-container',
    scope: 'test',
    violating: `it('a', () => { const { container } = render(<App />); container.querySelector('.submit') })`,
    compliant: `it('a', () => { render(<App />); screen.getByRole('button') })`,
  },
  {
    rule: 'testing-library/no-node-access',
    scope: 'test',
    violating: `it('a', () => { screen.getByRole('button').parentElement })`,
    compliant: `it('a', () => { screen.getByRole('group') })`,
  },

  // ---- D7 ----
  {
    rule: 'testing-library/prefer-presence-queries',
    scope: 'test',
    violating: `it('a', () => { expect(screen.queryByRole('button')).toBeInTheDocument() })`,
    compliant: `it('a', () => { expect(screen.getByRole('button')).toBeInTheDocument() })`,
  },
  {
    rule: 'testing-library/prefer-find-by',
    scope: 'test',
    violating: `it('a', async () => { await waitFor(() => screen.getByRole('button')) })`,
    compliant: `it('a', async () => { await screen.findByRole('button') })`,
  },
  {
    rule: 'testing-library/prefer-query-by-disappearance',
    scope: 'test',
    violating: `it('a', async () => { await waitForElementToBeRemoved(() => screen.getByRole('button')) })`,
    compliant: `it('a', async () => { await waitForElementToBeRemoved(() => screen.queryByRole('button')) })`,
  },

  // ---- D8 ----
  {
    rule: 'testing-library/no-unnecessary-act',
    scope: 'test',
    violating: `it('a', () => { act(() => { render(<App />) }) })`,
    compliant: `it('a', () => { render(<App />) })`,
  },

  // ---- D10 ----
  {
    rule: 'testing-library/await-async-queries',
    scope: 'test',
    violating: `it('a', () => { screen.findByRole('button') })`,
    compliant: `it('a', async () => { await screen.findByRole('button') })`,
  },
  {
    rule: 'testing-library/await-async-utils',
    scope: 'test',
    violating: `it('a', () => { waitFor(() => { expect(el).toBeInTheDocument() }) })`,
    compliant: `it('a', async () => { await waitFor(() => { expect(el).toBeInTheDocument() }) })`,
  },
  {
    rule: 'testing-library/await-async-events',
    scope: 'test',
    violating: `it('a', () => { const user = userEvent.setup(); user.click(el) })`,
    compliant: `it('a', async () => { const user = userEvent.setup(); await user.click(el) })`,
  },
  {
    rule: 'testing-library/no-await-sync-queries',
    scope: 'test',
    violating: `it('a', async () => { await screen.getByRole('button') })`,
    compliant: `it('a', () => { screen.getByRole('button') })`,
  },
  {
    rule: 'testing-library/no-await-sync-events',
    scope: 'test',
    violating: `it('a', async () => { await fireEvent.click(el) })`,
    compliant: `it('a', () => { fireEvent.click(el) })`,
  },

  // ---- D11 ----
  {
    rule: 'testing-library/no-wait-for-multiple-assertions',
    scope: 'test',
    violating: `it('a', async () => {
  await waitFor(() => {
    expect(el).toBeInTheDocument()
    expect(el).toBeEnabled()
  })
})`,
    compliant: `it('a', async () => {
  await waitFor(() => {
    expect(el).toBeInTheDocument()
  })
})`,
  },
  {
    rule: 'testing-library/no-wait-for-side-effects',
    scope: 'test',
    violating: `it('a', async () => {
  await waitFor(() => {
    fireEvent.click(el)
    expect(el).toBeEnabled()
  })
})`,
    compliant: `it('a', async () => {
  await waitFor(() => {
    expect(el).toBeEnabled()
  })
})`,
  },
  {
    rule: 'testing-library/no-wait-for-snapshot',
    scope: 'test',
    violating: `it('a', async () => { await waitFor(() => { expect(el).toMatchSnapshot() }) })`,
    compliant: `it('a', async () => { await waitFor(() => { expect(el).toBeEnabled() }) })`,
  },

  // ---- A2 / D5: jest-dom ----
  {
    rule: 'jest-dom/prefer-checked',
    scope: 'test',
    violating: `it('a', () => { expect(screen.getByRole('checkbox')).toHaveAttribute('checked') })`,
    compliant: `it('a', () => { expect(screen.getByRole('checkbox')).toBeChecked() })`,
  },
  {
    rule: 'jest-dom/prefer-empty',
    scope: 'test',
    violating: `it('a', () => { expect(el.innerHTML).toBe('') })`,
    compliant: `it('a', () => { expect(el).toBeEmptyDOMElement() })`,
  },
  {
    rule: 'jest-dom/prefer-enabled-disabled',
    scope: 'test',
    violating: `it('a', () => { expect(screen.getByRole('button')).toHaveAttribute('disabled') })`,
    compliant: `it('a', () => { expect(screen.getByRole('button')).toBeDisabled() })`,
  },
  {
    rule: 'jest-dom/prefer-focus',
    scope: 'test',
    violating: `it('a', () => { expect(document.activeElement).toBe(el) })`,
    compliant: `it('a', () => { expect(el).toHaveFocus() })`,
  },
  {
    rule: 'jest-dom/prefer-in-document',
    scope: 'test',
    violating: `it('a', () => { expect(screen.queryByRole('button')).not.toBeNull() })`,
    compliant: `it('a', () => { expect(screen.getByRole('button')).toBeInTheDocument() })`,
  },
  {
    rule: 'jest-dom/prefer-pressed',
    scope: 'test',
    violating: `it('a', () => { expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'true') })`,
    compliant: `it('a', () => { expect(screen.getByRole('button')).toBePressed() })`,
  },
  {
    rule: 'jest-dom/prefer-required',
    scope: 'test',
    violating: `it('a', () => { expect(screen.getByRole('textbox')).toHaveAttribute('required') })`,
    compliant: `it('a', () => { expect(screen.getByRole('textbox')).toBeRequired() })`,
  },
  {
    rule: 'jest-dom/prefer-to-have-attribute',
    scope: 'test',
    violating: `it('a', () => { expect(el.getAttribute('type')).toBe('text') })`,
    compliant: `it('a', () => { expect(el).toHaveAttribute('type', 'text') })`,
  },
  {
    rule: 'jest-dom/prefer-to-have-class',
    scope: 'test',
    violating: `it('a', () => { expect(el.classList.contains('primary')).toBe(true) })`,
    compliant: `it('a', () => { expect(el).toHaveClass('primary') })`,
  },
  {
    rule: 'jest-dom/prefer-to-have-style',
    scope: 'test',
    violating: `it('a', () => { expect(el.style.color).toBe('red') })`,
    compliant: `it('a', () => { expect(el).toHaveStyle({ color: 'red' }) })`,
  },
  {
    rule: 'jest-dom/prefer-to-have-text-content',
    scope: 'test',
    violating: `it('a', () => { expect(el.textContent).toBe('Submit') })`,
    compliant: `it('a', () => { expect(el).toHaveTextContent('Submit') })`,
  },
  {
    rule: 'jest-dom/prefer-to-have-value',
    scope: 'test',
    violating: `it('a', () => { expect(screen.getByRole('textbox').value).toBe('Ada') })`,
    compliant: `it('a', () => { expect(screen.getByRole('textbox')).toHaveValue('Ada') })`,
  },

  // ---- F1-F7, F10, F11: you might not need an Effect ----
  {
    rule: 'react-you-might-not-need-an-effect/no-derived-state',
    scope: 'source',
    violating: `export function C({ first, last }: { first: string, last: string }) {
  const [fullName, setFullName] = useState('')
  useEffect(() => { setFullName(first + ' ' + last) }, [first, last])
  return <p>{fullName}</p>
}`,
    compliant: `export function C({ first, last }: { first: string, last: string }) {
  const fullName = first + ' ' + last
  return <p>{fullName}</p>
}`,
  },
  {
    rule: 'react-you-might-not-need-an-effect/no-chain-state-updates',
    scope: 'source',
    violating: `export function Game() {
  const [round, setRound] = useState(1)
  const [isGameOver, setIsGameOver] = useState(false)
  useEffect(() => {
    if (round > 10) { setIsGameOver(true) }
  }, [round])
  return <button onClick={() => setRound(round + 1)}>{isGameOver ? 'over' : round}</button>
}`,
    compliant: `export function Game() {
  const [round, setRound] = useState(1)
  const [isGameOver, setIsGameOver] = useState(false)
  const next = () => {
    setRound(round + 1)
    setIsGameOver(round + 1 > 10)
  }
  return <button onClick={next}>{isGameOver ? 'over' : round}</button>
}`,
  },
  {
    rule: 'react-you-might-not-need-an-effect/no-event-handler',
    scope: 'source',
    violating: `export function C({ onSubmit }: { onSubmit: () => void }) {
  const [submitted, setSubmitted] = useState(false)
  useEffect(() => { if (submitted) { onSubmit() } }, [submitted, onSubmit])
  return <button onClick={() => setSubmitted(true)}>go</button>
}`,
    compliant: `export function C({ onSubmit }: { onSubmit: () => void }) {
  return <button onClick={() => onSubmit()}>go</button>
}`,
  },
  {
    rule: 'react-you-might-not-need-an-effect/no-pass-live-state-to-parent',
    scope: 'source',
    violating: `export function C({ onChange }: { onChange: (v: string) => void }) {
  const [value, setValue] = useState('')
  useEffect(() => { onChange(value) }, [value, onChange])
  return <input value={value} onChange={(e) => setValue(e.target.value)} />
}`,
    compliant: `export function C({ value, onChange }: { value: string, onChange: (v: string) => void }) {
  return <input value={value} onChange={(e) => onChange(e.target.value)} />
}`,
  },
  {
    rule: 'react-you-might-not-need-an-effect/no-pass-data-to-parent',
    scope: 'source',
    violating: `export function Child({ onDataFetched }: { onDataFetched: (v: number) => void }) {
  const data = useQuery('/data')
  useEffect(() => { onDataFetched(data) }, [data, onDataFetched])
  return <p>{data}</p>
}`,
    // The parent owns the fetch and passes the result down.
    compliant: `export function Child({ data }: { data: number }) {
  return <p>{data}</p>
}`,
  },
  {
    rule: 'react-you-might-not-need-an-effect/no-external-store-subscription',
    scope: 'source',
    violating: `export function useOnlineStatus() {
  const [isOnline, setIsOnline] = useState(true)
  useEffect(() => {
    function updateState() { setIsOnline(navigator.onLine) }
    updateState()
    window.addEventListener('online', updateState)
    return () => { window.removeEventListener('online', updateState) }
  }, [])
  return isOnline
}`,
    compliant: `export function useOnlineStatus() {
  return useSyncExternalStore(store.subscribe, store.getSnapshot)
}`,
  },
  {
    rule: 'react-you-might-not-need-an-effect/no-initialize-state',
    scope: 'source',
    violating: `export function C() {
  const [data, setData] = useState(0)
  useEffect(() => { setData(compute(1)) }, [])
  return <p>{data}</p>
}`,
    compliant: `export function C() {
  const [data] = useState(() => compute(1))
  return <p>{data}</p>
}`,
  },

  {
    // F10 — every `useState` in the component goes back to its initial value, which is
    // exactly what React's `key` does for free.
    //
    // The violating sample is deliberately degenerate — state that is never read. v1.0.1
    // gates on `setter refs in the effect === state refs in the whole component`, and it
    // counts *references*, not `useState` declarations, so any other read or write of the
    // state pushes the count past the setters and the rule goes quiet. React's own
    // worked example (`ProfilePage` rendering the comment box) therefore does not fire.
    // Nothing is lost in practice: those shapes are caught by
    // `no-adjust-state-on-prop-change` below, which is why F10 is enforced at all.
    rule: 'react-you-might-not-need-an-effect/no-reset-all-state-on-prop-change',
    scope: 'source',
    violating: `export function Profile({ userId }: { userId: string }) {
  const [comment, setComment] = useState('')
  useEffect(() => { setComment('') }, [userId])
  return null
}`,
    // The parent passes the identity as `key`, so React remounts and resets the state.
    compliant: `export function Profile() {
  const [comment, setComment] = useState('')
  return <input value={comment} onChange={(e) => setComment(e.target.value)} />
}

export function ProfilePage({ userId }: { userId: string }) {
  return <Profile key={userId} />
}`,
  },
  {
    // F11 — only *some* state is adjusted, and the new value does not derive from a prop,
    // so neither no-derived-state nor no-reset-all-state-on-prop-change covers it. This is
    // the residue the mapping recorded as review-only; the ninth-rule audit closed it.
    rule: 'react-you-might-not-need-an-effect/no-adjust-state-on-prop-change',
    scope: 'source',
    violating: `export function List({ items }: { items: Item[] }) {
  const [isReverse, setIsReverse] = useState(false)
  const [selection, setSelection] = useState<Item | null>(null)
  useEffect(() => { setSelection(null) }, [items])
  return <button onClick={() => setIsReverse(!isReverse)}>{selection ? selection.id : 'none'}</button>
}`,
    // Store the id and find during render — the selection falls out of the current items.
    compliant: `export function List({ items }: { items: Item[] }) {
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const selection = items.find((item) => item.id === selectedId) ?? null
  return <button onClick={() => setSelectedId(items[0].id)}>{selection ? selection.id : 'none'}</button>
}`,
  },

  // ---- F8 ----
  {
    rule: 'react-hooks/exhaustive-deps',
    scope: 'source',
    violating: `export function C({ id }: { id: number }) {
  const [value, setValue] = useState(0)
  useEffect(() => { setValue(compute(id)) }, [])
  return <p>{value}</p>
}`,
    compliant: `export function C({ id }: { id: number }) {
  const value = compute(id)
  return <p>{value}</p>
}`,
  },
]

export const preambles = { test: TEST_PREAMBLE, source: SOURCE_PREAMBLE }
