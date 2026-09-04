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

/**
 * `expect`, `it`, `describe`, `beforeEach` and `test` are deliberately **not** declared
 * here, unlike every other name below.
 *
 * `eslint-plugin-jest` treats a locally-declared `expect` as "not an assertion" and stays
 * silent on it, so a `declare const expect: any` would switch off every A2 and A5 rule in
 * the base slice while leaving the rest of the suite green. Left undeclared, `expect`
 * resolves to an unresolved global — the shape real test files have under either runner,
 * and the one the plugin recognises. This preamble is a fixture, so an implicit global
 * costs nothing; the same line in a real file would be the runner's injected global.
 */
const TEST_PREAMBLE = `
import { render, screen, act, waitFor, waitForElementToBeRemoved, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
declare const afterEach: (fn: () => unknown) => void
declare const App: () => JSX.Element
declare const getUser: () => { name: string, age: number }
declare const other: { name?: string }
declare const el: HTMLInputElement
declare const noop: () => void
declare const assertSaved: () => void
declare const list: string[]
declare const total: number
declare const save: (n: number) => void
declare const loadUser: () => Promise<{ name: string }>
declare const throwing: () => void
declare const obj: { method: () => void }
declare const spy: {
  mock: { calls: unknown[][] }
  mockImplementation: (fn: () => unknown) => void
  mockResolvedValue: (v: unknown) => void
}
declare const jest: { fn: () => () => void, spyOn: (o: object, k: string) => void }
declare const cy: { debug: () => void, get: (s: string) => void }
`

const SOURCE_PREAMBLE = `
import { useState, useEffect, useSyncExternalStore } from 'react'
declare const store: { subscribe: (fn: () => void) => () => void, getSnapshot: () => number }
declare const useQuery: (key: string) => number
declare const navigator: {
  onLine: boolean
  geolocation: { getCurrentPosition: (fn: (p: unknown) => void) => void }
}
declare const window: {
  addEventListener: (e: string, fn: () => void) => void
  removeEventListener: (e: string, fn: () => void) => void
  open: (url: string, target?: string, features?: string) => unknown
}
declare const console: { log: (...args: unknown[]) => void }
declare const compute: (n: number) => number
declare const url: string
declare const cmd: string
declare const key: Buffer
declare const iv: Buffer
declare const app: { use: (m: unknown) => void }
interface Item { id: number }
`

/**
 * The typed half of the suite gets its own preambles, and the reason is not tidiness.
 *
 * Under `projectService` every name has to resolve for real. The two preambles above lean on
 * `declare const` stand-ins and on imports that are not installed here, and an unresolved
 * import is `any` — which would make the `no-unsafe-*` family report on the preamble of every
 * typed case, on the compliant sample as loudly as the violating one. So typed cases start
 * from names the checker actually knows.
 */
const TYPED_SOURCE_PREAMBLE = `
declare const flag: boolean
declare const n: number
declare const s: string
declare const list: string[]
declare const nums: number[]
declare const maybe: string | undefined
declare const box: { a: number, b?: string }
declare const looselyTyped: any
declare function work(): Promise<number>
declare function sideEffect(value: number): void
declare function onEvent(cb: () => void): void
`

const TYPED_TEST_PREAMBLE = `
declare const total: number
declare const err: Error
declare function loadUser(): Promise<{ name: string }>
declare class Service { method(): void }
declare const service: Service
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

  // ---- base: A2, assertions should communicate meaning (non-DOM half) ----
  {
    rule: 'jest/prefer-to-be',
    scope: 'test',
    violating: `it('a', () => { expect(getUser().age).toEqual(36) })`,
    compliant: `it('a', () => { expect(getUser().age).toBe(36) })`,
  },
  {
    rule: 'jest/prefer-to-contain',
    scope: 'test',
    violating: `it('a', () => { expect(list.includes('a')).toBe(true) })`,
    compliant: `it('a', () => { expect(list).toContain('a') })`,
  },
  {
    rule: 'jest/prefer-to-have-length',
    scope: 'test',
    violating: `it('a', () => { expect(list.length).toBe(3) })`,
    compliant: `it('a', () => { expect(list).toHaveLength(3) })`,
  },
  {
    rule: 'jest/prefer-comparison-matcher',
    scope: 'test',
    violating: `it('a', () => { expect(total > 5).toBe(true) })`,
    compliant: `it('a', () => { expect(total).toBeGreaterThan(5) })`,
  },
  {
    rule: 'jest/prefer-equality-matcher',
    scope: 'test',
    violating: `it('a', () => { expect(total === 5).toBe(true) })`,
    compliant: `it('a', () => { expect(total).toBe(5) })`,
  },
  {
    rule: 'jest/prefer-strict-equal',
    scope: 'test',
    violating: `it('a', () => { expect(getUser()).toEqual({ name: 'Ada', age: 36 }) })`,
    compliant: `it('a', () => { expect(getUser()).toStrictEqual({ name: 'Ada', age: 36 }) })`,
  },
  {
    rule: 'jest/prefer-called-with',
    scope: 'test',
    violating: `it('a', () => { expect(save).toHaveBeenCalled() })`,
    compliant: `it('a', () => { expect(save).toHaveBeenCalledWith(1) })`,
  },

  // ---- base: A5, assertions must actually execute ----
  {
    rule: 'jest/valid-expect',
    scope: 'test',
    // An async matcher whose promise is dropped: the test finishes before it resolves.
    violating: `it('a', async () => { expect(loadUser()).resolves.toMatchObject({ name: 'Ada' }) })`,
    compliant: `it('a', async () => { await expect(loadUser()).resolves.toMatchObject({ name: 'Ada' }) })`,
  },
  {
    rule: 'jest/no-conditional-expect',
    scope: 'test',
    violating: `it('a', () => {
  if (total > 0) {
    expect(getUser().name).toBe('Ada')
  }
})`,
    // The branch is decided before the assertion, so the assertion always runs.
    compliant: `it('a', () => {
  const expected = total > 0 ? 'Ada' : 'Grace'
  expect(getUser().name).toBe(expected)
})`,
  },

  {
    rule: 'jest/require-to-throw-message',
    scope: 'test',
    violating: `it('a', () => { expect(() => throwing()).toThrow() })`,
    compliant: `it('a', () => { expect(() => throwing()).toThrow('boom') })`,
  },
  {
    rule: 'jest/no-alias-methods',
    scope: 'test',
    violating: `it('a', () => { expect(save).toBeCalledWith(1) })`,
    compliant: `it('a', () => { expect(save).toHaveBeenCalledWith(1) })`,
  },
  {
    rule: 'jest/prefer-to-have-been-called',
    scope: 'test',
    violating: `it('a', () => { expect(save).toHaveBeenCalledTimes(0) })`,
    // A real count is not this rule's business — only the zero.
    compliant: `it('a', () => { expect(save).toHaveBeenCalledTimes(2) })`,
  },
  {
    rule: 'jest/prefer-to-have-been-called-times',
    scope: 'test',
    violating: `it('a', () => { expect(spy.mock.calls).toHaveLength(1) })`,
    compliant: `it('a', () => { expect(spy).toHaveBeenCalledTimes(1) })`,
  },

  // ---- base: A5, the two chain/placement shapes `valid-expect` does not cover ----
  {
    rule: 'jest/valid-expect-in-promise',
    scope: 'test',
    violating: `it('a', () => {
  loadUser().then((user) => { expect(user.name).toBe('Ada') })
})`,
    compliant: `it('a', async () => {
  await loadUser().then((user) => { expect(user.name).toBe('Ada') })
})`,
  },
  {
    rule: 'jest/no-standalone-expect',
    scope: 'test',
    violating: `describe('g', () => { expect(total).toBe(1) })`,
    compliant: `describe('g', () => { it('a', () => { expect(total).toBe(1) }) })`,
  },
  {
    rule: 'jest/expect-expect',
    scope: 'test',
    violating: `it('a', () => { save(1) })`,
    compliant: `it('a', () => { save(1); expect(total).toBe(1) })`,
  },

  // ---- base: test integrity — tests that do not run ----
  {
    rule: 'jest/no-focused-tests',
    scope: 'test',
    violating: `it.only('a', () => { expect(total).toBe(1) })`,
    compliant: `it('a', () => { expect(total).toBe(1) })`,
  },
  {
    rule: 'jest/no-disabled-tests',
    scope: 'test',
    violating: `it.skip('a', () => { expect(total).toBe(1) })`,
    compliant: `it('a', () => { expect(total).toBe(1) })`,
  },
  {
    rule: 'jest/no-commented-out-tests',
    scope: 'test',
    violating: `// it('a', () => { expect(total).toBe(1) })
it('b', () => { expect(total).toBe(2) })`,
    // A comment that is not a test is none of the rule's business.
    compliant: `// the total is seeded by the fixture
it('b', () => { expect(total).toBe(2) })`,
  },

  // ---- base: test integrity — titles and hooks ----
  {
    rule: 'jest/no-identical-title',
    scope: 'test',
    violating: `describe('g', () => {
  it('a', () => { expect(total).toBe(1) })
  it('a', () => { expect(total).toBe(2) })
})`,
    compliant: `describe('g', () => {
  it('a', () => { expect(total).toBe(1) })
  it('b', () => { expect(total).toBe(2) })
})`,
  },
  {
    rule: 'jest/valid-title',
    scope: 'test',
    violating: `it('', () => { expect(total).toBe(1) })`,
    compliant: `it('reports the total', () => { expect(total).toBe(1) })`,
  },
  {
    rule: 'jest/no-duplicate-hooks',
    scope: 'test',
    violating: `describe('g', () => {
  beforeEach(() => { save(1) })
  beforeEach(() => { save(2) })
  it('a', () => { expect(total).toBe(1) })
})`,
    compliant: `describe('g', () => {
  beforeEach(() => { save(1) })
  it('a', () => { expect(total).toBe(1) })
})`,
  },

  // ---- base: test integrity — mocks ----
  {
    rule: 'jest/prefer-spy-on',
    scope: 'test',
    violating: `it('a', () => { obj.method = jest.fn() })`,
    compliant: `it('a', () => { jest.spyOn(obj, 'method') })`,
  },
  {
    rule: 'jest/prefer-mock-promise-shorthand',
    scope: 'test',
    violating: `it('a', () => { spy.mockImplementation(() => Promise.resolve(1)) })`,
    compliant: `it('a', () => { spy.mockResolvedValue(1) })`,
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

  // ---- typescript-eslint: bugs the compiler does not report ----
  {
    rule: '@typescript-eslint/no-shadow',
    scope: 'source',
    violating: `const value = 1
export function f() { const value = 2; return value }`,
    compliant: `const value = 1
export function f() { const inner = 2; return inner + value }`,
  },
  {
    rule: '@typescript-eslint/no-use-before-define',
    scope: 'source',
    violating: `export const a = b
const b = 1`,
    compliant: `const b = 1
export const a = b`,
  },
  {
    rule: 'no-loop-func',
    scope: 'source',
    violating: `export function f() {
  const fns: (() => number)[] = []
  for (var i = 0; i < 3; i++) { fns.push(() => i) }
  return fns
}`,
    compliant: `export function f() {
  const fns: (() => number)[] = []
  for (let i = 0; i < 3; i++) { fns.push(() => i) }
  return fns
}`,
  },
  {
    rule: 'no-loss-of-precision',
    scope: 'source',
    violating: `export const n = 9007199254740993`,
    compliant: `export const n = 9007199254740992`,
  },
  {
    rule: '@typescript-eslint/default-param-last',
    scope: 'source',
    violating: `export function f(a = 1, b: number) { return a + b }`,
    compliant: `export function f(b: number, a = 1) { return a + b }`,
  },
  {
    rule: '@typescript-eslint/no-dupe-class-members',
    scope: 'source',
    violating: `export class C { m() { return 1 } m() { return 2 } }`,
    compliant: `export class C { m() { return 1 } n() { return 2 } }`,
  },
  {
    rule: '@typescript-eslint/no-invalid-this',
    scope: 'source',
    violating: `export function f() { return this }`,
    compliant: `export class C { m() { return this } }`,
  },
  {
    rule: '@typescript-eslint/no-unused-private-class-members',
    scope: 'source',
    violating: `export class C { #x = 1 }`,
    compliant: `export class C { #x = 1; get x() { return this.#x } }`,
  },
  {
    rule: '@typescript-eslint/no-unnecessary-parameter-property-assignment',
    scope: 'source',
    violating: `export class C {
  constructor(private readonly x: number) { this.x = x }
}`,
    compliant: `export class C {
  constructor(private readonly x: number) {}
}`,
  },
  {
    rule: '@typescript-eslint/no-non-null-asserted-nullish-coalescing',
    scope: 'source',
    // Member access, not a bare identifier: the rule keys off the property read.
    violating: `export const f = (o: { a?: string }) => o.a! ?? 'x'`,
    compliant: `export const f = (o: { a?: string }) => o.a ?? 'x'`,
  },
  {
    rule: '@typescript-eslint/no-confusing-non-null-assertion',
    scope: 'source',
    violating: `export const f = (a?: number, b?: number) => a! == b`,
    compliant: `export const f = (a?: number, b?: number) => a === b`,
  },
  {
    rule: '@typescript-eslint/no-invalid-void-type',
    scope: 'source',
    violating: `export function f(x: void) { return x }`,
    compliant: `export function f(): void {}`,
  },
  {
    rule: '@typescript-eslint/prefer-literal-enum-member',
    scope: 'source',
    violating: `const base = 1
export enum E { A = base }`,
    compliant: `export enum E { A = 1 }`,
  },
  {
    rule: '@typescript-eslint/no-useless-constructor',
    scope: 'source',
    violating: `export class C { constructor() {} }`,
    compliant: `export class C { constructor(readonly x: number) {} }`,
  },
  {
    rule: '@typescript-eslint/ban-ts-comment',
    scope: 'source',
    violating: `// @ts-ignore
export const n = 1`,
    // `@ts-expect-error` with a description is the sanctioned form: it fails once the
    // underlying error is gone, so the suppression cannot outlive the bug.
    compliant: `// @ts-expect-error the upstream types are wrong here
export const n = 1`,
  },
  {
    rule: '@typescript-eslint/consistent-type-imports',
    scope: 'source',
    violating: `import { Thing } from './thing'
export const f = (t: Thing) => t`,
    compliant: `import type { Thing } from './thing'
export const f = (t: Thing) => t`,
  },
  {
    rule: '@typescript-eslint/no-import-type-side-effects',
    scope: 'source',
    violating: `import { type Thing } from './thing'
export const f = (t: Thing) => t`,
    compliant: `import type { Thing } from './thing'
export const f = (t: Thing) => t`,
  },
  {
    rule: '@typescript-eslint/no-dynamic-delete',
    scope: 'source',
    violating: `export const f = (o: Record<string, number>, k: string) => { delete o[k] }`,
    compliant: `export const f = (o: Map<string, number>, k: string) => { o.delete(k) }`,
  },
  {
    rule: '@typescript-eslint/no-extraneous-class',
    scope: 'source',
    violating: `export class Utils { static twice(n: number) { return n * 2 } }`,
    compliant: `export function twice(n: number) { return n * 2 }`,
  },
  {
    rule: '@typescript-eslint/unified-signatures',
    scope: 'source',
    violating: `export declare function f(a: number): void
export declare function f(a: number, b: number): void`,
    compliant: `export declare function f(a: number, b?: number): void`,
  },
  {
    rule: '@typescript-eslint/consistent-type-definitions',
    scope: 'source',
    violating: `export type P = { a: number }`,
    compliant: `export interface P { a: number }`,
  },
  {
    rule: '@typescript-eslint/no-inferrable-types',
    scope: 'source',
    violating: `export const n: number = 1`,
    compliant: `export const n = 1`,
  },
  {
    rule: '@typescript-eslint/prefer-for-of',
    scope: 'source',
    violating: `export function f(items: number[]) {
  let sum = 0
  for (let i = 0; i < items.length; i++) { sum += items[i] }
  return sum
}`,
    compliant: `export function f(items: number[]) {
  let sum = 0
  for (const item of items) { sum += item }
  return sum
}`,
  },

  // ---- regexp: what the pattern actually does ----
  {
    rule: 'regexp/no-super-linear-backtracking',
    scope: 'source',
    violating: `export const re = /^(?:a+)+$/`,
    compliant: `export const re = /^a+$/`,
  },
  {
    rule: 'regexp/no-contradiction-with-assertion',
    scope: 'source',
    violating: `export const re = /^\\d*(?<=\\d)/`,
    compliant: `export const re = /^\\d+(?<=\\d)/`,
  },
  {
    rule: 'regexp/no-misleading-capturing-group',
    scope: 'source',
    violating: `export const re = /(\\d*)\\w+/`,
    compliant: `export const re = /(\\d*)[a-z]+/`,
  },
  {
    rule: 'regexp/no-misleading-unicode-character',
    scope: 'source',
    violating: `export const re = /[👍]/`,
    compliant: `export const re = /👍/u`,
  },
  {
    rule: 'regexp/no-optional-assertion',
    scope: 'source',
    violating: `export const re = /a(?:(?=b))?b/`,
    compliant: `export const re = /a(?=b)b/`,
  },
  {
    rule: 'regexp/no-potentially-useless-backreference',
    scope: 'source',
    violating: `export const re = /(?:(a)|b)\\1/`,
    compliant: `export const re = /(a)\\1/`,
  },
  {
    rule: 'regexp/no-useless-assertions',
    scope: 'source',
    violating: `export const re = /\\ba(?=\\w)b/`,
    compliant: `export const re = /\\bab/`,
  },
  {
    rule: 'regexp/no-useless-backreference',
    scope: 'source',
    violating: `export const re = /(a)|\\1b/`,
    compliant: `export const re = /(a)\\1b/`,
  },
  {
    rule: 'regexp/optimal-lookaround-quantifier',
    scope: 'source',
    violating: `export const re = /(?<=a{2,3})b/`,
    compliant: `export const re = /(?<=a{2})b/`,
  },
  {
    rule: 'regexp/no-lazy-ends',
    scope: 'source',
    // The lazy quantifier only reads as pointless where the pattern is matched, not merely built.
    violating: `export const f = (s: string) => /ab*?/.test(s)`,
    compliant: `export const f = (s: string) => /ab*/.test(s)`,
  },
  {
    rule: 'regexp/no-empty-alternative',
    scope: 'source',
    violating: `export const re = /a(?:b|c|)/`,
    compliant: `export const re = /a(?:b|c)?/`,
  },
  {
    rule: 'regexp/no-empty-capturing-group',
    scope: 'source',
    violating: `export const re = /a(){0}/`,
    compliant: `export const re = /a(b)/`,
  },
  {
    rule: 'regexp/no-empty-character-class',
    scope: 'source',
    violating: `export const re = /a[]b/`,
    compliant: `export const re = /a[bc]d/`,
  },
  {
    rule: 'regexp/no-empty-group',
    scope: 'source',
    violating: `export const re = /a(?:)b/`,
    compliant: `export const re = /ab/`,
  },
  {
    rule: 'regexp/no-empty-lookarounds-assertion',
    scope: 'source',
    violating: `export const re = /a(?=)b/`,
    compliant: `export const re = /a(?=b)b/`,
  },
  {
    rule: 'regexp/no-zero-quantifier',
    scope: 'source',
    violating: `export const re = /a(?:b){0}c/`,
    compliant: `export const re = /a(?:b)c/`,
  },
  {
    rule: 'regexp/no-escape-backspace',
    scope: 'source',
    violating: `export const re = /[\\b]/`,
    compliant: `export const re = /\\u0008/`,
  },
  {
    rule: 'regexp/no-invisible-character',
    scope: 'source',
    violating: `export const re = /a\u00a0b/`,
    compliant: `export const re = /a\\u00a0b/`,
  },
  {
    rule: 'regexp/no-obscure-range',
    scope: 'source',
    violating: `export const re = /[A-\\x5a]/`,
    compliant: `export const re = /[A-Z]/`,
  },
  {
    rule: 'regexp/no-non-standard-flag',
    scope: 'source',
    violating: `export const re = new RegExp('a', 'l')`,
    compliant: `export const re = new RegExp('a', 'g')`,
  },
  {
    rule: 'regexp/no-legacy-features',
    scope: 'source',
    violating: `export const last = RegExp.$1`,
    compliant: `export const last = /(\\w)/.exec('a')?.[1]`,
  },
  {
    rule: 'regexp/strict',
    scope: 'source',
    violating: `export const re = /\\p{ASCII}/`,
    compliant: `export const re = /\\p{ASCII}/u`,
  },
  {
    rule: 'regexp/no-invalid-regexp',
    scope: 'source',
    violating: `export const re = new RegExp('(')`,
    compliant: `export const re = new RegExp('\\\\(')`,
  },
  {
    rule: 'regexp/no-missing-g-flag',
    scope: 'source',
    // The receiver is a literal on purpose. `strictTypes` (on by default) makes the rule
    // require proof the receiver is a string, and an untyped `s: string` parameter is not it.
    violating: `export const f = 'abc'.replaceAll(/a/, 'b')`,
    compliant: `export const f = 'abc'.replaceAll(/a/g, 'b')`,
  },
  {
    rule: 'regexp/no-useless-dollar-replacements',
    scope: 'source',
    violating: `export const f = 'abc'.replace(/(a)/, '$2')`,
    compliant: `export const f = 'abc'.replace(/(a)/, '$1')`,
  },
  {
    rule: 'regexp/confusing-quantifier',
    scope: 'source',
    violating: `export const re = /(?:a?b?)+/`,
    compliant: `export const re = /(?:a?b?)*/`,
  },
  {
    rule: 'regexp/no-dupe-characters-character-class',
    scope: 'source',
    violating: `export const re = /[aa]/`,
    compliant: `export const re = /[ab]/`,
  },
  {
    rule: 'regexp/no-dupe-disjunctions',
    scope: 'source',
    violating: `export const re = /a|a/`,
    compliant: `export const re = /a|b/`,
  },
  {
    rule: 'regexp/no-unused-capturing-group',
    scope: 'source',
    violating: `export const f = (s: string) => /(\\d+)-\\d+/.test(s)`,
    compliant: `export const f = (s: string) => /(?:\\d+)-\\d+/.test(s)`,
  },

  // ---- sonarjs: bugs ----
  {
    rule: 'sonarjs/no-identical-expressions',
    scope: 'source',
    // A bare `a === a` is exempt: Sonar treats it as a deliberate NaN check.
    violating: `export const f = (a: number) => a + 1 === a + 1`,
    compliant: `export const f = (a: number, b: number) => a + 1 === b + 1`,
  },
  {
    rule: 'sonarjs/no-identical-conditions',
    scope: 'source',
    violating: `export function f(a: number) {
  if (a > 1) { return 1 }
  else if (a > 1) { return 2 }
  return 3
}`,
    compliant: `export function f(a: number) {
  if (a > 1) { return 1 }
  else if (a > 2) { return 2 }
  return 3
}`,
  },
  {
    rule: 'sonarjs/no-all-duplicated-branches',
    scope: 'source',
    // Both branches have to be explicit; an implicit fall-through is a different shape.
    violating: `export function f(a: number) {
  if (a > 1) { return 1 } else { return 1 }
}`,
    compliant: `export function f(a: number) {
  if (a > 1) { return 1 } else { return 2 }
}`,
  },
  {
    rule: 'sonarjs/no-element-overwrite',
    scope: 'source',
    violating: `export function f() {
  const arr: number[] = []
  arr[0] = 1
  arr[0] = 2
  return arr
}`,
    compliant: `export function f() {
  const arr: number[] = []
  arr[0] = 1
  arr[1] = 2
  return arr
}`,
  },
  {
    rule: 'sonarjs/no-useless-increment',
    scope: 'source',
    violating: `export function f() {
  let i = 0
  i = i++
  return i
}`,
    compliant: `export function f() {
  let i = 0
  i++
  return i
}`,
  },
  {
    rule: 'sonarjs/non-existent-operator',
    scope: 'source',
    violating: `export function f() {
  let a = 1
  a =- 1
  return a
}`,
    compliant: `export function f() {
  let a = 1
  a -= 1
  return a
}`,
  },
  {
    rule: 'sonarjs/for-loop-increment-sign',
    scope: 'source',
    violating: `export function f() {
  let n = 0
  for (let i = 0; i < 3; i--) { n += i }
  return n
}`,
    compliant: `export function f() {
  let n = 0
  for (let i = 0; i < 3; i++) { n += i }
  return n
}`,
  },
  {
    rule: 'sonarjs/no-floating-point-equality',
    scope: 'source',
    violating: `export function f() {
  let x = 0.1
  for (let i = 0; i < 3; i += 0.1) { x = i }
  return x === 0.3
}`,
    compliant: `export function f() {
  let x = 0.1
  for (let i = 0; i < 3; i += 0.1) { x = i }
  return Math.abs(x - 0.3) < Number.EPSILON
}`,
  },
  {
    rule: 'sonarjs/no-unthrown-error',
    scope: 'source',
    violating: `export function f(a: number) {
  if (a < 0) { new Error('negative') }
  return a
}`,
    compliant: `export function f(a: number) {
  if (a < 0) { throw new Error('negative') }
  return a
}`,
  },
  {
    rule: 'sonarjs/constructor-for-side-effects',
    scope: 'source',
    violating: `class Registered { constructor() { compute(1) } }
export function f() { new Registered() }`,
    compliant: `class Registered { constructor() { compute(1) } }
export function f() { return new Registered() }`,
  },
  {
    rule: 'sonarjs/no-use-of-empty-return-value',
    scope: 'source',
    // The value has to be *used*, and the emptiness inferred from the body rather than
    // the annotation: Sonar decides this without types.
    violating: `function v() { return }
export const x = v() + 1`,
    compliant: `function v() { return 1 }
export const x = v() + 1`,
  },
  {
    rule: 'sonarjs/generator-without-yield',
    scope: 'source',
    violating: `export function* g() { return 1 }`,
    compliant: `export function* g() { yield 1 }`,
  },
  {
    rule: 'sonarjs/no-extra-arguments',
    scope: 'source',
    violating: `function f(a: number) { return a }
export const x = f(1, 2)`,
    compliant: `function f(a: number, b: number) { return a + b }
export const x = f(1, 2)`,
  },
  {
    rule: 'sonarjs/no-literal-call',
    scope: 'source',
    // The cast form is invisible to the rule: it reads the callee node, and a TSAsExpression
    // is not a literal.
    violating: `export const x = (42)()`,
    compliant: `export const x = compute(42)`,
  },
  {
    rule: 'sonarjs/updated-const-var',
    scope: 'source',
    violating: `export function f() {
  const a = 1
  a = 2
  return a
}`,
    compliant: `export function f() {
  let a = 1
  a = 2
  return a
}`,
  },
  {
    rule: 'sonarjs/no-delete-var',
    scope: 'source',
    violating: `export function f(o: Record<string, number>) {
  let a = 1
  delete a
  return o
}`,
    compliant: `export function f(o: Record<string, number>) {
  delete o.a
  return o
}`,
  },
  {
    rule: 'sonarjs/no-globals-shadowing',
    scope: 'source',
    violating: `export function f() {
  let NaN = 1
  return NaN
}`,
    compliant: `export function f() {
  let n = 1
  return n
}`,
  },
  {
    rule: 'sonarjs/no-built-in-override',
    scope: 'source',
    // Rebinding the global itself. Patching one of its members is a different (unflagged) shape.
    violating: `export function f() { Object = {} as never }`,
    compliant: `export function f() { return Object.keys({}) }`,
  },
  {
    rule: 'sonarjs/no-function-declaration-in-block',
    scope: 'source',
    violating: `export function f(a: number) {
  if (a > 0) {
    function g() { return 1 }
    return g()
  }
  return 0
}`,
    compliant: `export function f(a: number) {
  const g = () => 1
  if (a > 0) { return g() }
  return 0
}`,
  },
  {
    rule: 'sonarjs/comma-or-logical-or-case',
    scope: 'source',
    violating: `export function f(a: number) {
  switch (a) {
    case 1, 2: return 'x'
    default: return 'y'
  }
}`,
    compliant: `export function f(a: number) {
  switch (a) {
    case 1:
    case 2: return 'x'
    default: return 'y'
  }
}`,
  },
  {
    rule: 'sonarjs/for-in',
    scope: 'source',
    violating: `export function f(o: Record<string, number>) {
  let n = 0
  for (const k in o) { n += o[k] }
  return n
}`,
    compliant: `export function f(o: Record<string, number>) {
  let n = 0
  for (const k in o) {
    if (Object.prototype.hasOwnProperty.call(o, k)) { n += o[k] }
  }
  return n
}`,
  },
  {
    rule: 'sonarjs/no-parameter-reassignment',
    scope: 'source',
    violating: `export function f(a: number) {
  a = 1
  return a
}`,
    compliant: `export function f(a: number) {
  const b = a + 1
  return b
}`,
  },
  {
    rule: 'sonarjs/no-empty-collection',
    scope: 'source',
    violating: `export function f() {
  const arr: number[] = []
  return arr[0]
}`,
    compliant: `export function f() {
  const arr: number[] = [1]
  return arr[0]
}`,
  },
  {
    rule: 'sonarjs/stateful-regex',
    scope: 'source',
    // Two calls on *different* inputs: that is what makes the carried lastIndex a bug.
    violating: `const re = /a/g
export const f = (a: string, b: string) => re.test(a) && re.test(b)`,
    compliant: `const re = /a/
export const f = (a: string, b: string) => re.test(a) && re.test(b)`,
  },

  // ---- sonarjs: security ----
  {
    rule: 'sonarjs/no-hardcoded-passwords',
    scope: 'source',
    violating: `export const connect = () => compute(1)
const password = 'hunter2please'`,
    compliant: `export const connect = () => compute(1)
const password = process.env.DB_PASSWORD`,
  },
  {
    rule: 'sonarjs/no-hardcoded-secrets',
    scope: 'source',
    // Sonar wants a secret-shaped *name* bound to a long enough literal, and it only looks
    // at bindings something reads.
    violating: `const apiKey = 'abcdefghijklmnopqrstuvwxyz0123456789ABCD'
export const use = () => apiKey`,
    compliant: `const apiKey = process.env.API_KEY
export const use = () => apiKey`,
  },
  {
    rule: 'sonarjs/hardcoded-secret-signatures',
    scope: 'source',
    // Despite the name, this rule matches call *signatures* known to take a secret, not
    // token formats. A bare `ghp_...` literal on its own never fires.
    violating: `import { createHmac } from 'node:crypto'
export const h = createHmac('sha256', 'literal-signing-key')`,
    compliant: `import { createHmac } from 'node:crypto'
export const h = createHmac('sha256', process.env.SIGNING_KEY ?? '')`,
  },
  {
    rule: 'sonarjs/hashing',
    scope: 'source',
    violating: `import { createHash } from 'node:crypto'
export const h = createHash('md5')`,
    compliant: `import { createHash } from 'node:crypto'
export const h = createHash('sha512')`,
  },
  {
    rule: 'sonarjs/no-weak-cipher',
    scope: 'source',
    violating: `import { createCipheriv } from 'node:crypto'
export const c = createCipheriv('DES', key, iv)`,
    compliant: `import { createCipheriv } from 'node:crypto'
export const c = createCipheriv('aes-256-gcm', key, iv)`,
  },
  {
    rule: 'sonarjs/no-weak-keys',
    scope: 'source',
    // The callee has to be a member expression; the rule never matches a destructured import.
    violating: `import * as crypto from 'node:crypto'
export const k = crypto.generateKeyPairSync('rsa', { modulusLength: 1024 })`,
    compliant: `import * as crypto from 'node:crypto'
export const k = crypto.generateKeyPairSync('rsa', { modulusLength: 4096 })`,
  },
  {
    rule: 'sonarjs/encryption-secure-mode',
    scope: 'source',
    violating: `import { createCipheriv } from 'node:crypto'
export const c = createCipheriv('aes-256-ecb', key, iv)`,
    compliant: `import { createCipheriv } from 'node:crypto'
export const c = createCipheriv('aes-256-gcm', key, iv)`,
  },
  {
    rule: 'sonarjs/insecure-jwt-token',
    scope: 'source',
    violating: `import jwt from 'jsonwebtoken'
export const d = jwt.verify(url, 'k', { algorithms: ['none'] })`,
    compliant: `import jwt from 'jsonwebtoken'
export const d = jwt.verify(url, 'k', { algorithms: ['HS256'] })`,
  },
  {
    rule: 'sonarjs/pseudo-random',
    scope: 'source',
    violating: `export const r = Math.random()`,
    compliant: `import { randomInt } from 'node:crypto'
export const r = randomInt(100)`,
  },
  {
    rule: 'sonarjs/weak-ssl',
    scope: 'source',
    violating: `import { request } from 'node:https'
export const r = request({ secureProtocol: 'TLSv1_method' })`,
    compliant: `import { request } from 'node:https'
export const r = request({ secureProtocol: 'TLSv1_2_method' })`,
  },
  {
    rule: 'sonarjs/unverified-certificate',
    scope: 'source',
    violating: `import { request } from 'node:https'
export const r = request({ rejectUnauthorized: false })`,
    compliant: `import { request } from 'node:https'
export const r = request({ rejectUnauthorized: true })`,
  },
  {
    rule: 'sonarjs/unverified-hostname',
    scope: 'source',
    // Sonar reads the callback body: a block whose every return is absent or `true` is a
    // check that accepts any hostname. An expression-bodied arrow is not inspected.
    violating: `import { request } from 'node:https'
export const r = request({ checkServerIdentity: () => { return true } })`,
    compliant: `import { request } from 'node:https'
export const r = request({})`,
  },
  {
    rule: 'sonarjs/no-clear-text-protocols',
    scope: 'source',
    // A bare `http://` literal does not fire; the rule reads client configuration.
    violating: `import nodemailer from 'nodemailer'
export const t = nodemailer.createTransport({ secure: false })`,
    compliant: `import nodemailer from 'nodemailer'
export const t = nodemailer.createTransport({ secure: true })`,
  },
  {
    rule: 'sonarjs/code-eval',
    scope: 'source',
    violating: `export const f = (s: string) => eval(s)`,
    compliant: `export const f = (s: string) => JSON.parse(s)`,
  },
  {
    rule: 'sonarjs/os-command',
    scope: 'source',
    violating: `import { exec } from 'node:child_process'
export const run = () => exec(cmd)`,
    compliant: `import { execFile } from 'node:child_process'
export const run = () => execFile('/usr/bin/git', ['status'])`,
  },
  {
    rule: 'sonarjs/no-os-command-from-path',
    scope: 'source',
    violating: `import { spawn } from 'node:child_process'
export const run = () => spawn('git', ['status'])`,
    compliant: `import { spawn } from 'node:child_process'
export const run = () => spawn('/usr/bin/git', ['status'])`,
  },
  {
    rule: 'sonarjs/xml-parser-xxe',
    scope: 'source',
    violating: `import libxmljs from 'libxmljs'
export const doc = libxmljs.parseXmlString(url, { noent: true })`,
    compliant: `import libxmljs from 'libxmljs'
export const doc = libxmljs.parseXmlString(url, { noent: false })`,
  },
  {
    rule: 'sonarjs/file-permissions',
    scope: 'source',
    violating: `const fs = require('fs')
export const f = () => fs.chmodSync('/var/data', 0o777)`,
    compliant: `const fs = require('fs')
export const f = () => fs.chmodSync('/var/data', 0o640)`,
  },
  {
    rule: 'sonarjs/publicly-writable-directories',
    scope: 'source',
    violating: `export const scratch = '/tmp/export.csv'`,
    compliant: `import { mkdtempSync } from 'node:fs'
export const scratch = mkdtempSync('export-')`,
  },
  {
    rule: 'sonarjs/confidential-information-logging',
    scope: 'source',
    violating: `import signale from 'signale'
export const log = new signale.Signale({ secrets: [] })`,
    compliant: `import signale from 'signale'
export const log = new signale.Signale({ secrets: ['password'] })`,
  },
  {
    rule: 'sonarjs/production-debug',
    scope: 'source',
    violating: `import errorhandler from 'errorhandler'
export const wire = () => app.use(errorhandler())`,
    compliant: `import errorhandler from 'errorhandler'
export const wire = () => {
  if (process.env.NODE_ENV === 'development') { app.use(errorhandler()) }
}`,
  },
  {
    rule: 'sonarjs/link-with-target-blank',
    scope: 'source',
    // The URL has to be a literal the rule can see start with http, not a variable.
    violating: `export const open = () => window.open('https://example.com', '_blank')`,
    compliant: `export const open = () => window.open('https://example.com', '_blank', 'noopener,noreferrer')`,
  },
  {
    rule: 'sonarjs/no-intrusive-permissions',
    scope: 'source',
    violating: `export const locate = (cb: (p: unknown) => void) => navigator.geolocation.getCurrentPosition(cb)`,
    compliant: `export const locate = (cb: (p: unknown) => void) => cb(undefined)`,
  },

  // ---- sonarjs: test integrity ----
  {
    // Both this rule and the next gate on a recognised runner: they read the import list and
    // package.json, and return no visitors otherwise. Hence the explicit import.
    rule: 'sonarjs/synchronous-suite-callback',
    scope: 'test',
    violating: `import { describe, it } from '@jest/globals'
describe('g', async () => { it('a', () => { expect(total).toBe(1) }) })`,
    compliant: `import { describe, it } from '@jest/globals'
describe('g', () => { it('a', () => { expect(total).toBe(1) }) })`,
  },
  {
    rule: 'sonarjs/no-mixed-completion-style',
    scope: 'test',
    violating: `import { it } from '@jest/globals'
it('a', async (done: () => void) => { expect(total).toBe(1); done() })`,
    compliant: `import { it } from '@jest/globals'
it('a', async () => { expect(total).toBe(1) })`,
  },
  {
    rule: 'sonarjs/no-debug-commands-in-ui-tests',
    scope: 'test',
    violating: `it('a', () => { cy.debug() })`,
    compliant: `it('a', () => { cy.get('.submit') })`,
  },

  // ---- react: JSX correctness ----
  {
    rule: 'react/jsx-key',
    scope: 'source',
    violating: `export const C = ({ items }: { items: Item[] }) => <ul>{items.map((i) => <li>{i.id}</li>)}</ul>`,
    compliant: `export const C = ({ items }: { items: Item[] }) => <ul>{items.map((i) => <li key={i.id}>{i.id}</li>)}</ul>`,
  },
  {
    rule: 'react/jsx-no-duplicate-props',
    scope: 'source',
    violating: `export const C = () => <input value="a" value="b" />`,
    compliant: `export const C = () => <input value="a" name="b" />`,
  },
  {
    rule: 'react/jsx-props-no-spread-multi',
    scope: 'source',
    violating: `export const C = ({ p }: { p: object }) => <input {...p} {...p} />`,
    compliant: `export const C = ({ p }: { p: object }) => <input {...p} />`,
  },
  {
    rule: 'react/jsx-no-comment-textnodes',
    scope: 'source',
    violating: `export const C = () => <div>// the label</div>`,
    compliant: `export const C = () => <div>{/* the label */}</div>`,
  },
  {
    rule: 'react/jsx-no-leaked-render',
    scope: 'source',
    violating: `export const C = ({ items }: { items: Item[] }) => <div>{items.length && <span>some</span>}</div>`,
    compliant: `export const C = ({ items }: { items: Item[] }) => <div>{items.length > 0 ? <span>some</span> : null}</div>`,
  },
  {
    rule: 'react/no-this-in-sfc',
    scope: 'source',
    violating: `export function C() { return <p>{this.props.a}</p> }`,
    compliant: `export function C({ a }: { a: string }) { return <p>{a}</p> }`,
  },
  {
    rule: 'react/void-dom-elements-no-children',
    scope: 'source',
    violating: `export const C = () => <br>text</br>`,
    compliant: `export const C = () => <span>text</span>`,
  },
  {
    rule: 'react/no-danger-with-children',
    scope: 'source',
    violating: `export const C = ({ html }: { html: string }) => <div dangerouslySetInnerHTML={{ __html: html }}>child</div>`,
    compliant: `export const C = ({ html }: { html: string }) => <div dangerouslySetInnerHTML={{ __html: html }} />`,
  },
  {
    rule: 'react/no-children-prop',
    scope: 'source',
    violating: `export const C = () => <div children="text" />`,
    compliant: `export const C = () => <div>text</div>`,
  },
  {
    rule: 'react/style-prop-object',
    scope: 'source',
    violating: `export const C = () => <div style="color: red" />`,
    compliant: `export const C = () => <div style={{ color: 'red' }} />`,
  },
  {
    rule: 'react/checked-requires-onchange-or-readonly',
    scope: 'source',
    violating: `export const C = () => <input type="checkbox" checked />`,
    compliant: `export const C = () => <input type="checkbox" checked readOnly />`,
  },
  {
    rule: 'react/no-unknown-property',
    scope: 'source',
    violating: `export const C = () => <div class="row" />`,
    compliant: `export const C = () => <div className="row" />`,
  },
  {
    rule: 'react/no-unescaped-entities',
    scope: 'source',
    violating: `export const C = () => <div>the patient's chart</div>`,
    compliant: `export const C = () => <div>the patient&apos;s chart</div>`,
  },
  {
    rule: 'react/no-unstable-nested-components',
    scope: 'source',
    violating: `export function Parent() {
  function Row() { return <li /> }
  return <ul><Row /></ul>
}`,
    compliant: `function Row() { return <li /> }
export function Parent() {
  return <ul><Row /></ul>
}`,
  },

  // ---- react: security ----
  {
    rule: 'react/jsx-no-target-blank',
    scope: 'source',
    violating: `export const C = () => <a href={url} target="_blank">go</a>`,
    compliant: `export const C = () => <a href={url} target="_blank" rel="noreferrer">go</a>`,
  },
  {
    rule: 'react/jsx-no-script-url',
    scope: 'source',
    violating: `export const C = () => <a href="javascript:void(0)">go</a>`,
    compliant: `export const C = () => <button type="button">go</button>`,
  },

  // ---- react: deprecated and removed APIs ----
  {
    rule: 'react/no-deprecated',
    scope: 'source',
    violating: `import { render } from 'react-dom'
export const mount = (el: HTMLElement) => render(<p />, el)`,
    compliant: `import { createRoot } from 'react-dom/client'
export const mount = (el: HTMLElement) => createRoot(el).render(<p />)`,
  },
  {
    rule: 'react/no-unsafe',
    scope: 'source',
    violating: `import { Component } from 'react'
export class C extends Component {
  UNSAFE_componentWillMount() {}
  render() { return <p /> }
}`,
    compliant: `import { Component } from 'react'
export class C extends Component {
  componentDidMount() {}
  render() { return <p /> }
}`,
  },
  {
    rule: 'react/no-find-dom-node',
    scope: 'source',
    violating: `import { Component } from 'react'
import { findDOMNode } from 'react-dom'
export class C extends Component {
  componentDidMount() { findDOMNode(this) }
  render() { return <p /> }
}`,
    compliant: `import { Component, createRef } from 'react'
export class C extends Component {
  node = createRef<HTMLParagraphElement>()
  render() { return <p ref={this.node} /> }
}`,
  },
  {
    rule: 'react/no-string-refs',
    scope: 'source',
    violating: `import { Component } from 'react'
export class C extends Component {
  render() { return <p ref="node" /> }
}`,
    compliant: `import { Component, createRef } from 'react'
export class C extends Component {
  node = createRef<HTMLParagraphElement>()
  render() { return <p ref={this.node} /> }
}`,
  },
  {
    rule: 'react/no-is-mounted',
    scope: 'source',
    violating: `import { Component } from 'react'
export class C extends Component {
  componentDidMount() { if (this.isMounted()) { compute(1) } }
  render() { return <p /> }
}`,
    compliant: `import { Component } from 'react'
export class C extends Component {
  componentDidMount() { compute(1) }
  render() { return <p /> }
}`,
  },
  {
    rule: 'react/no-render-return-value',
    scope: 'source',
    // The rule matches the namespace call, `ReactDOM.render`, not a destructured `render`.
    violating: `import ReactDOM from 'react-dom'
export const mount = (el: HTMLElement) => { const inst = ReactDOM.render(<p />, el); return inst }`,
    compliant: `import { createRoot } from 'react-dom/client'
export const mount = (el: HTMLElement) => { createRoot(el).render(<p />) }`,
  },
  {
    rule: 'react/no-direct-mutation-state',
    scope: 'source',
    violating: `import { Component } from 'react'
export class C extends Component<object, { n: number }> {
  componentDidMount() { this.state.n = 1 }
  render() { return <p /> }
}`,
    compliant: `import { Component } from 'react'
export class C extends Component<object, { n: number }> {
  componentDidMount() { this.setState({ n: 1 }) }
  render() { return <p /> }
}`,
  },
  {
    rule: 'react/require-render-return',
    scope: 'source',
    violating: `import { Component } from 'react'
export class C extends Component {
  render() {}
}`,
    compliant: `import { Component } from 'react'
export class C extends Component {
  render() { return <p /> }
}`,
  },

  // ---- react: judgement calls ----
  {
    rule: 'react/no-array-index-key',
    scope: 'source',
    violating: `export const C = ({ items }: { items: Item[] }) => <ul>{items.map((item, i) => <li key={i}>{item.id}</li>)}</ul>`,
    compliant: `export const C = ({ items }: { items: Item[] }) => <ul>{items.map((item) => <li key={item.id}>{item.id}</li>)}</ul>`,
  },
  {
    rule: 'react/jsx-no-constructed-context-values',
    scope: 'source',
    violating: `import { createContext } from 'react'
const Ctx = createContext({ n: 0 })
export const C = ({ n }: { n: number }) => <Ctx.Provider value={{ n }} />`,
    compliant: `import { createContext, useMemo } from 'react'
const Ctx = createContext({ n: 0 })
export const C = ({ n }: { n: number }) => {
  const value = useMemo(() => ({ n }), [n])
  return <Ctx.Provider value={value} />
}`,
  },
  {
    rule: 'react/no-object-type-as-default-prop',
    scope: 'source',
    violating: `export const C = ({ items = [] }: { items?: Item[] }) => <p>{items.length}</p>`,
    compliant: `const EMPTY: Item[] = []
export const C = ({ items = EMPTY }: { items?: Item[] }) => <p>{items.length}</p>`,
  },
  {
    rule: 'react/jsx-no-useless-fragment',
    scope: 'source',
    violating: `export const C = () => <><p /></>`,
    compliant: `export const C = () => <p />`,
  },
  {
    rule: 'react/button-has-type',
    scope: 'source',
    violating: `export const C = () => <button>go</button>`,
    compliant: `export const C = () => <button type="button">go</button>`,
  },
  {
    rule: 'react/no-danger',
    scope: 'source',
    violating: `export const C = ({ html }: { html: string }) => <div dangerouslySetInnerHTML={{ __html: html }} />`,
    compliant: `export const C = ({ html }: { html: string }) => <div>{html}</div>`,
  },
  {
    rule: 'react/iframe-missing-sandbox',
    scope: 'source',
    violating: `export const C = () => <iframe title="report" src={url} />`,
    compliant: `export const C = () => <iframe title="report" src={url} sandbox="allow-scripts" />`,
  },

  // ---- react: Sonar's two render-loop rules ----
  {
    rule: 'sonarjs/no-hook-setter-in-body',
    scope: 'source',
    violating: `export function C() {
  const [n, setN] = useState(0)
  setN(1)
  return <p>{n}</p>
}`,
    compliant: `export function C() {
  const [n, setN] = useState(0)
  return <button type="button" onClick={() => setN(1)}>{n}</button>
}`,
  },
  {
    rule: 'sonarjs/no-useless-react-setstate',
    scope: 'source',
    violating: `export function C() {
  const [n, setN] = useState(0)
  return <button type="button" onClick={() => setN(n)}>{n}</button>
}`,
    compliant: `export function C() {
  const [n, setN] = useState(0)
  return <button type="button" onClick={() => setN(n + 1)}>{n}</button>
}`,
  },

  // ---- a11y: text alternatives ----
  {
    rule: 'jsx-a11y/alt-text',
    scope: 'source',
    violating: `export const C = () => <img src="/cat.png" />`,
    compliant: `export const C = () => <img src="/cat.png" alt="A tabby cat asleep on a keyboard" />`,
  },
  {
    rule: 'jsx-a11y/img-redundant-alt',
    scope: 'source',
    violating: `export const C = () => <img src="/cat.png" alt="Photo of a cat" />`,
    compliant: `export const C = () => <img src="/cat.png" alt="A tabby cat" />`,
  },
  {
    rule: 'jsx-a11y/iframe-has-title',
    scope: 'source',
    violating: `export const C = () => <iframe src="/report" />`,
    compliant: `export const C = () => <iframe src="/report" title="Quarterly report" />`,
  },
  {
    rule: 'sonarjs/object-alt-content',
    scope: 'source',
    violating: `export const C = () => <object data="/chart.svg" />`,
    compliant: `export const C = () => <object data="/chart.svg">Admissions by month</object>`,
  },
  {
    rule: 'jsx-a11y/anchor-has-content',
    scope: 'source',
    violating: `export const C = () => <a href="/home" />`,
    compliant: `export const C = () => <a href="/home">Home</a>`,
  },
  {
    rule: 'jsx-a11y/heading-has-content',
    scope: 'source',
    violating: `export const C = () => <h1 />`,
    compliant: `export const C = () => <h1>Admissions</h1>`,
  },

  // ---- a11y: document language ----
  {
    rule: 'jsx-a11y/html-has-lang',
    scope: 'source',
    violating: `export const C = () => <html />`,
    compliant: `export const C = () => <html lang="en" />`,
  },
  {
    rule: 'jsx-a11y/lang',
    scope: 'source',
    violating: `export const C = () => <html lang="engrish" />`,
    compliant: `export const C = () => <html lang="en-GB" />`,
  },

  // ---- a11y: ARIA that is simply wrong ----
  {
    rule: 'jsx-a11y/aria-props',
    scope: 'source',
    violating: `export const C = () => <div aria-labeledby="title" />`,
    compliant: `export const C = () => <div aria-labelledby="title" />`,
  },
  {
    rule: 'jsx-a11y/aria-proptypes',
    scope: 'source',
    violating: `export const C = () => <div aria-hidden="yes" />`,
    compliant: `export const C = () => <div aria-hidden={true} />`,
  },
  {
    rule: 'jsx-a11y/aria-role',
    scope: 'source',
    violating: `export const C = () => <div role="datepicker" />`,
    compliant: `export const C = () => <div role="grid" />`,
  },
  {
    rule: 'jsx-a11y/aria-unsupported-elements',
    scope: 'source',
    violating: `export const C = () => <meta charSet="utf-8" aria-hidden={true} />`,
    compliant: `export const C = () => <meta charSet="utf-8" />`,
  },
  {
    rule: 'jsx-a11y/role-has-required-aria-props',
    scope: 'source',
    violating: `export const C = () => <div role="checkbox" />`,
    compliant: `export const C = () => <div role="checkbox" aria-checked={false} />`,
  },
  {
    rule: 'jsx-a11y/role-supports-aria-props',
    scope: 'source',
    violating: `export const C = () => <div role="heading" aria-required={true} />`,
    compliant: `export const C = () => <div role="heading" aria-level={2} />`,
  },
  {
    rule: 'jsx-a11y/no-redundant-roles',
    scope: 'source',
    violating: `export const C = () => <button type="button" role="button">Go</button>`,
    compliant: `export const C = () => <button type="button">Go</button>`,
  },
  {
    rule: 'jsx-a11y/aria-activedescendant-has-tabindex',
    scope: 'source',
    violating: `export const C = () => <div aria-activedescendant="opt-1" />`,
    compliant: `export const C = () => <div aria-activedescendant="opt-1" tabIndex={0} />`,
  },
  {
    rule: 'jsx-a11y/no-interactive-element-to-noninteractive-role',
    scope: 'source',
    violating: `export const C = () => <button type="button" role="presentation">Go</button>`,
    compliant: `export const C = () => <button type="button">Go</button>`,
  },
  {
    rule: 'jsx-a11y/no-noninteractive-element-to-interactive-role',
    scope: 'source',
    violating: `export const C = () => <li role="button">Go</li>`,
    compliant: `export const C = () => <ul role="menu"><li role="menuitem">Go</li></ul>`,
  },
  {
    rule: 'jsx-a11y/no-aria-hidden-on-focusable',
    scope: 'source',
    violating: `export const C = () => <button type="button" aria-hidden={true}>Go</button>`,
    compliant: `export const C = () => <div aria-hidden={true}>Decorative</div>`,
  },

  // ---- a11y: keyboard reachability ----
  {
    rule: 'jsx-a11y/tabindex-no-positive',
    scope: 'source',
    violating: `export const C = () => <button type="button" tabIndex={1}>Go</button>`,
    compliant: `export const C = () => <button type="button" tabIndex={0}>Go</button>`,
  },
  {
    rule: 'jsx-a11y/no-noninteractive-tabindex',
    scope: 'source',
    violating: `export const C = () => <div tabIndex={0}>Text</div>`,
    compliant: `export const C = () => <button type="button" tabIndex={0}>Go</button>`,
  },
  {
    rule: 'jsx-a11y/interactive-supports-focus',
    scope: 'source',
    violating: `export const C = () => <div role="button" onClick={() => {}} onKeyDown={() => {}}>Go</div>`,
    compliant: `export const C = () => (
  <div role="button" tabIndex={0} onClick={() => {}} onKeyDown={() => {}}>Go</div>
)`,
  },
  {
    rule: 'jsx-a11y/no-access-key',
    scope: 'source',
    violating: `export const C = () => <button type="button" accessKey="h">Go</button>`,
    compliant: `export const C = () => <button type="button">Go</button>`,
  },
  {
    rule: 'jsx-a11y/no-autofocus',
    scope: 'source',
    violating: `export const C = () => <input autoFocus />`,
    compliant: `export const C = () => <input />`,
  },
  {
    rule: 'jsx-a11y/mouse-events-have-key-events',
    scope: 'source',
    violating: `export const C = () => <span onMouseOver={() => {}}>Total</span>`,
    compliant: `export const C = () => <span onMouseOver={() => {}} onFocus={() => {}}>Total</span>`,
  },

  // ---- a11y: form controls and tables ----
  {
    rule: 'jsx-a11y/label-has-associated-control',
    scope: 'source',
    violating: `export const C = () => <label>Name</label>`,
    compliant: `export const C = () => <label>Name<input name="name" /></label>`,
  },
  {
    rule: 'jsx-a11y/autocomplete-valid',
    scope: 'source',
    violating: `export const C = () => <input autoComplete="given-nam" />`,
    compliant: `export const C = () => <input autoComplete="given-name" />`,
  },
  {
    rule: 'jsx-a11y/scope',
    scope: 'source',
    violating: `export const C = () => <div scope="col">Name</div>`,
    compliant: `export const C = () => <th scope="col">Name</th>`,
  },
  {
    rule: 'sonarjs/table-header',
    scope: 'source',
    violating: `export const C = () => (
  <table>
    <tr><td>Ada</td><td>36</td></tr>
  </table>
)`,
    compliant: `export const C = () => (
  <table>
    <tr><th scope="col">Name</th><th scope="col">Age</th></tr>
    <tr><td>Ada</td><td>36</td></tr>
  </table>
)`,
  },
  {
    rule: 'sonarjs/no-table-as-layout',
    scope: 'source',
    violating: `export const C = () => (
  <table role="presentation">
    <tr><td>Sidebar</td><td>Content</td></tr>
  </table>
)`,
    compliant: `export const C = () => (
  <table>
    <tr><th scope="col">Name</th></tr>
    <tr><td>Ada</td></tr>
  </table>
)`,
  },
  {
    rule: 'sonarjs/table-header-reference',
    scope: 'source',
    violating: `export const C = () => (
  <table>
    <tr><th id="name">Name</th><th id="age">Age</th></tr>
    <tr><td headers="age">Ada</td><td headers="age">36</td></tr>
  </table>
)`,
    compliant: `export const C = () => (
  <table>
    <tr><th id="name">Name</th><th id="age">Age</th></tr>
    <tr><td headers="name">Ada</td><td headers="age">36</td></tr>
  </table>
)`,
  },
  {
    rule: 'jsx-a11y/no-distracting-elements',
    scope: 'source',
    violating: `export const C = () => <marquee>Now hiring</marquee>`,
    compliant: `export const C = () => <div>Now hiring</div>`,
  },

  // ---- a11y: judgement calls ----
  {
    rule: 'jsx-a11y/click-events-have-key-events',
    scope: 'source',
    violating: `export const C = () => <div role="button" tabIndex={0} onClick={() => {}}>Go</div>`,
    compliant: `export const C = () => (
  <div role="button" tabIndex={0} onClick={() => {}} onKeyDown={() => {}}>Go</div>
)`,
  },
  {
    rule: 'jsx-a11y/no-static-element-interactions',
    scope: 'source',
    violating: `export const C = () => <div onClick={() => {}} onKeyDown={() => {}}>Go</div>`,
    compliant: `export const C = () => (
  <div role="button" tabIndex={0} onClick={() => {}} onKeyDown={() => {}}>Go</div>
)`,
  },
  {
    rule: 'jsx-a11y/no-noninteractive-element-interactions',
    scope: 'source',
    violating: `export const C = () => <li onClick={() => {}} onKeyDown={() => {}}>Go</li>`,
    compliant: `export const C = () => (
  <li><button type="button" onClick={() => {}}>Go</button></li>
)`,
  },
  {
    rule: 'jsx-a11y/prefer-tag-over-role',
    scope: 'source',
    violating: `export const C = () => <div role="button">Go</div>`,
    compliant: `export const C = () => <button type="button">Go</button>`,
  },
  {
    rule: 'jsx-a11y/anchor-is-valid',
    scope: 'source',
    violating: `export const C = () => <a href="#">Go</a>`,
    compliant: `export const C = () => <a href="/home">Go</a>`,
  },
  {
    rule: 'jsx-a11y/anchor-ambiguous-text',
    scope: 'source',
    violating: `export const C = () => <a href="/report">click here</a>`,
    compliant: `export const C = () => <a href="/report">View the quarterly report</a>`,
  },
  {
    rule: 'jsx-a11y/media-has-caption',
    scope: 'source',
    violating: `export const C = () => <video src="/intro.mp4" />`,
    compliant: `export const C = () => (
  <video src="/intro.mp4"><track kind="captions" src="/intro.vtt" /></video>
)`,
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

  // ================================================================================
  // The typed layer. Everything below runs through `projectService`; the runner picks
  // the instance from the rule id, so nothing here needs a flag.
  // ================================================================================

  // ---- typed: promises ----
  {
    rule: '@typescript-eslint/no-floating-promises',
    scope: 'source',
    violating: `export function f() { work() }`,
    compliant: `export function f() { void work() }`,
  },
  {
    rule: '@typescript-eslint/no-misused-promises',
    scope: 'source',
    violating: `export function f() { onEvent(async () => { await work() }) }`,
    compliant: `export function f() { onEvent(() => { void work() }) }`,
  },
  {
    rule: '@typescript-eslint/await-thenable',
    scope: 'source',
    violating: `export async function f() { await n }`,
    compliant: `export async function f() { await work() }`,
  },
  {
    rule: '@typescript-eslint/require-await',
    scope: 'source',
    violating: `export async function f() { return n }`,
    compliant: `export function f() { return n }`,
  },
  {
    rule: '@typescript-eslint/return-await',
    scope: 'source',
    violating: `export async function f() { try { return work() } catch { return 0 } }`,
    compliant: `export async function f() { try { return await work() } catch { return 0 } }`,
  },
  {
    rule: '@typescript-eslint/no-implied-eval',
    scope: 'source',
    violating: `export function f() { setTimeout('sideEffect(1)', 0) }`,
    compliant: `export function f() { setTimeout(() => { sideEffect(1) }, 0) }`,
  },
  {
    rule: '@typescript-eslint/prefer-promise-reject-errors',
    scope: 'source',
    violating: `export const f = () => Promise.reject('boom')`,
    compliant: `export const f = () => Promise.reject(new Error('boom'))`,
  },
  {
    rule: '@typescript-eslint/only-throw-error',
    scope: 'source',
    violating: `export function f() { throw 'boom' }`,
    compliant: `export function f() { throw new Error('boom') }`,
  },
  {
    rule: '@typescript-eslint/use-unknown-in-catch-callback-variable',
    scope: 'source',
    violating: `export const f = () => { void work().catch((e: Error) => { sideEffect(e.message.length) }) }`,
    compliant: `export const f = () => { void work().catch((e: unknown) => { sideEffect(String(e).length) }) }`,
  },
  {
    rule: '@typescript-eslint/strict-void-return',
    scope: 'source',
    violating: `export function f() { onEvent(() => n) }`,
    compliant: `export function f() { onEvent(() => { sideEffect(n) }) }`,
  },

  // ---- typed: wrong at runtime, invisible without types ----
  {
    rule: '@typescript-eslint/no-array-delete',
    scope: 'source',
    violating: `export function f() { delete nums[0] }`,
    compliant: `export function f() { nums.splice(0, 1) }`,
  },
  {
    rule: '@typescript-eslint/no-for-in-array',
    scope: 'source',
    violating: `export function f() { for (const i in nums) { sideEffect(Number(i)) } }`,
    compliant: `export function f() { for (const v of nums) { sideEffect(v) } }`,
  },
  {
    rule: '@typescript-eslint/no-base-to-string',
    scope: 'source',
    violating: `export const x = String(box)`,
    compliant: `export const x = JSON.stringify(box)`,
  },
  {
    rule: '@typescript-eslint/no-misused-spread',
    scope: 'source',
    violating: `export const f = (m: Map<string, number>) => ({ ...m })`,
    compliant: `export const f = (m: Map<string, number>) => Object.fromEntries(m)`,
  },
  {
    rule: '@typescript-eslint/no-mixed-enums',
    scope: 'source',
    violating: `export enum E { A = 1, B = 'b' }`,
    compliant: `export enum E { A = 1, B = 2 }`,
  },
  {
    rule: '@typescript-eslint/no-unsafe-enum-comparison',
    scope: 'source',
    violating: `enum E { A }
export const f = (e: E) => e === 0`,
    compliant: `enum E { A }
export const f = (e: E) => e === E.A`,
  },
  {
    rule: '@typescript-eslint/no-unsafe-unary-minus',
    scope: 'source',
    violating: `export const x = -s`,
    compliant: `export const x = -n`,
  },
  {
    rule: '@typescript-eslint/restrict-plus-operands',
    scope: 'source',
    // An object operand, not `number + string`: the rule allows that pair by default, and
    // `sonarjs/no-incorrect-string-concat` is the one that argues about it.
    violating: `export const x = box + n`,
    compliant: `export const x = box.a + n`,
  },
  {
    rule: '@typescript-eslint/require-array-sort-compare',
    scope: 'source',
    violating: `export const f = () => [...nums].sort()`,
    compliant: `export const f = () => [...nums].sort((a, b) => a - b)`,
  },
  {
    rule: '@typescript-eslint/related-getter-setter-pairs',
    scope: 'source',
    violating: `export class C {
  get v(): number { return 1 }
  set v(next: string) { sideEffect(next.length) }
}`,
    compliant: `export class C {
  get v(): number { return 1 }
  set v(next: number) { sideEffect(next) }
}`,
  },
  {
    rule: '@typescript-eslint/switch-exhaustiveness-check',
    scope: 'source',
    violating: `type Kind = 'a' | 'b'
export function f(k: Kind) { switch (k) { case 'a': return 1 } return 0 }`,
    compliant: `type Kind = 'a' | 'b'
export function f(k: Kind) { switch (k) { case 'a': return 1; case 'b': return 2 } }`,
  },
  {
    rule: '@typescript-eslint/unbound-method',
    scope: 'source',
    violating: `class C { m() { sideEffect(1) } }
declare const c: C
export const g = c.m`,
    compliant: `class C { m() { sideEffect(1) } }
declare const c: C
export const g = () => { c.m() }`,
  },
  {
    rule: '@typescript-eslint/restrict-template-expressions',
    scope: 'source',
    violating: 'export const x = `${box}`',
    compliant: 'export const x = `id ${s}`',
  },
  {
    rule: '@typescript-eslint/no-useless-default-assignment',
    scope: 'source',
    violating: `export function f() { const { a = 1 } = box; return a }`,
    compliant: `export function f() { const { b = 'x' } = box; return b }`,
  },

  // ---- typed: `any` leaking through the type system ----
  {
    rule: '@typescript-eslint/no-unsafe-argument',
    scope: 'source',
    violating: `export const f = () => { sideEffect(looselyTyped) }`,
    compliant: `export const f = () => { sideEffect(n) }`,
  },
  {
    rule: '@typescript-eslint/no-unsafe-assignment',
    scope: 'source',
    violating: `export const x: number = looselyTyped`,
    compliant: `export const x: number = n`,
  },
  {
    rule: '@typescript-eslint/no-unsafe-call',
    scope: 'source',
    violating: `export const f = () => { looselyTyped() }`,
    compliant: `export const f = () => { sideEffect(1) }`,
  },
  {
    rule: '@typescript-eslint/no-unsafe-member-access',
    scope: 'source',
    violating: `export const x: unknown = looselyTyped.a`,
    compliant: `export const x: unknown = box.a`,
  },
  {
    rule: '@typescript-eslint/no-unsafe-return',
    scope: 'source',
    violating: `export function f(): number { return looselyTyped }`,
    compliant: `export function f(): number { return Number(looselyTyped) }`,
  },
  {
    rule: '@typescript-eslint/no-unsafe-type-assertion',
    scope: 'source',
    violating: `export const x = box as { a: number, b: string }`,
    compliant: `export const x: { a: number } = box`,
  },

  // ---- typed: dead code and redundancy the checker can prove ----
  {
    rule: '@typescript-eslint/no-unnecessary-condition',
    scope: 'source',
    violating: `export const f = () => { if (box) { sideEffect(1) } }`,
    compliant: `export const f = () => { if (box.b) { sideEffect(1) } }`,
  },
  {
    rule: '@typescript-eslint/no-unnecessary-type-assertion',
    scope: 'source',
    violating: `export const x = n as number`,
    compliant: `const raw: unknown = box
export const x = raw as { a: number }`,
  },
  {
    rule: '@typescript-eslint/no-unnecessary-type-conversion',
    scope: 'source',
    violating: `export const x = String(s)`,
    compliant: `export const x = String(n)`,
  },
  {
    rule: '@typescript-eslint/no-unnecessary-boolean-literal-compare',
    scope: 'source',
    violating: `export const x = flag === true`,
    compliant: `export const x = flag`,
  },
  {
    rule: '@typescript-eslint/no-unnecessary-template-expression',
    scope: 'source',
    violating: 'export const x = `${s}`',
    compliant: 'export const x = `id ${s}`',
  },
  {
    rule: '@typescript-eslint/no-unnecessary-type-arguments',
    scope: 'source',
    violating: `declare function g<T = number>(v: T): T
export const x = g<number>(1)`,
    compliant: `declare function g<T = number>(v: T): T
export const x = g<string>('a')`,
  },
  {
    rule: '@typescript-eslint/no-unnecessary-type-parameters',
    scope: 'source',
    violating: `export function f<T>(value: T): void { sideEffect(1) }`,
    compliant: `export function f<T>(value: T): T { return value }`,
  },
  {
    rule: '@typescript-eslint/no-unnecessary-qualifier',
    scope: 'source',
    violating: `export namespace N {
  export type T = number
  export const x: N.T = 1
}`,
    compliant: `export namespace N {
  export type T = number
  export const x: T = 1
}`,
  },
  {
    rule: '@typescript-eslint/no-duplicate-type-constituents',
    scope: 'source',
    violating: `export type T = string | string`,
    compliant: `export type T = string | number`,
  },
  {
    rule: '@typescript-eslint/no-redundant-type-constituents',
    scope: 'source',
    violating: `export type T = number | any`,
    compliant: `export type T = number | string`,
  },
  {
    rule: '@typescript-eslint/no-deprecated',
    scope: 'source',
    violating: `/** @deprecated use fresh */ declare function old(): void
export const f = () => { old() }`,
    compliant: `declare function fresh(): void
export const f = () => { fresh() }`,
  },

  // ---- typed: shapes with one right answer, now provable ----
  {
    rule: '@typescript-eslint/dot-notation',
    scope: 'source',
    violating: `export const x = box['a']`,
    compliant: `export const x = box.a`,
  },
  {
    rule: '@typescript-eslint/prefer-includes',
    scope: 'source',
    violating: `export const x = list.indexOf('a') !== -1`,
    compliant: `export const x = list.includes('a')`,
  },
  {
    rule: '@typescript-eslint/prefer-string-starts-ends-with',
    scope: 'source',
    violating: `export const x = s.indexOf('a') === 0`,
    compliant: `export const x = s.startsWith('a')`,
  },
  {
    rule: '@typescript-eslint/prefer-find',
    scope: 'source',
    violating: `export const x = list.filter((v) => v === 'a')[0]`,
    compliant: `export const x = list.find((v) => v === 'a')`,
  },
  {
    rule: '@typescript-eslint/prefer-regexp-exec',
    scope: 'source',
    violating: `export const x = s.match(/a/)`,
    compliant: `export const x = /a/.exec(s)`,
  },
  {
    rule: '@typescript-eslint/prefer-reduce-type-parameter',
    scope: 'source',
    violating: `export const x = nums.reduce((acc, v) => acc.concat(v), [] as number[])`,
    compliant: `export const x = nums.reduce<number[]>((acc, v) => acc.concat(v), [])`,
  },
  {
    rule: '@typescript-eslint/prefer-return-this-type',
    scope: 'source',
    violating: `export class C { self(): C { return this } }`,
    compliant: `export class C { self(): this { return this } }`,
  },
  {
    rule: '@typescript-eslint/prefer-nullish-coalescing',
    scope: 'source',
    violating: `export const x = maybe || 'fallback'`,
    compliant: `export const x = maybe ?? 'fallback'`,
  },
  {
    rule: '@typescript-eslint/prefer-optional-chain',
    scope: 'source',
    violating: `export const x = box.b && box.b.length`,
    compliant: `export const x = box.b?.length`,
  },
  {
    rule: '@typescript-eslint/prefer-readonly',
    scope: 'source',
    violating: `export class C {
  private value = 1
  get v() { return this.value }
}`,
    compliant: `export class C {
  private readonly value = 1
  get v() { return this.value }
}`,
  },
  {
    rule: '@typescript-eslint/consistent-type-exports',
    scope: 'source',
    violating: `type Local = number
export { Local }`,
    compliant: `type Local = number
export type { Local }`,
  },
  {
    rule: '@typescript-eslint/consistent-return',
    scope: 'source',
    violating: `export function f(x: number) { if (x > 0) { return 1 } return }`,
    compliant: `export function f(x: number) { if (x > 0) { return 1 } return 0 }`,
  },
  {
    rule: '@typescript-eslint/no-confusing-void-expression',
    scope: 'source',
    violating: `export const x = sideEffect(1)`,
    compliant: `export const x = () => { sideEffect(1) }`,
  },
  {
    rule: '@typescript-eslint/no-meaningless-void-operator',
    scope: 'source',
    violating: `export const f = () => { void sideEffect(1) }`,
    compliant: `export const f = () => { void work() }`,
  },

  // ---- typed: sonarjs, type confusion ----
  {
    rule: 'sonarjs/null-dereference',
    scope: 'source',
    // The type has to be exactly `null` or `undefined`: the rule reads the type flags directly,
    // and a union carries the Union flag rather than Undefined.
    violating: `declare const missing: undefined
export const x = missing.valueOf()`,
    compliant: `declare const missing: undefined
export const x = missing?.valueOf()`,
  },
  {
    rule: 'sonarjs/different-types-comparison',
    scope: 'source',
    violating: `export const f = (a: string, b: number) => a === b`,
    compliant: `export const f = (a: string, b: string) => a === b`,
  },
  {
    rule: 'sonarjs/argument-type',
    scope: 'source',
    violating: `export const x = [1, 2].join(1)`,
    compliant: `export const x = [1, 2].join(',')`,
  },
  {
    rule: 'sonarjs/in-operator-type-error',
    scope: 'source',
    violating: `export const x = 'a' in s`,
    compliant: `export const x = 'a' in box`,
  },
  {
    rule: 'sonarjs/no-in-misuse',
    scope: 'source',
    violating: `export const x = '0' in nums`,
    compliant: `export const x = nums.includes(0)`,
  },
  {
    rule: 'sonarjs/operation-returning-nan',
    scope: 'source',
    violating: `export const x = box * 2`,
    compliant: `export const x = box.a * 2`,
  },
  {
    rule: 'sonarjs/values-not-convertible-to-numbers',
    scope: 'source',
    violating: `export const x = box > 1`,
    compliant: `export const x = box.a > 1`,
  },
  {
    rule: 'sonarjs/no-incorrect-string-concat',
    scope: 'source',
    violating: `export const x = s + n`,
    compliant: `export const x = s + String(n)`,
  },
  {
    rule: 'sonarjs/new-operator-misuse',
    scope: 'source',
    // A plain `function` declaration counts as instantiable to this rule, so the callee has to
    // be something with no construct signature at all.
    violating: `declare const NotAConstructor: string
export const x = new NotAConstructor()`,
    compliant: `class C {}
export const x = new C()`,
  },
  {
    rule: 'sonarjs/no-collection-size-mischeck',
    scope: 'source',
    violating: `export const x = nums.length >= 0`,
    compliant: `export const x = nums.length > 0`,
  },
  {
    rule: 'sonarjs/index-of-compare-to-positive-number',
    scope: 'source',
    // Arrays only. The rule checks the receiver is an Array, so a string `indexOf` slips past.
    violating: `export const x = list.indexOf('a') > 0`,
    compliant: `export const x = list.indexOf('a') >= 0`,
  },

  // ---- typed: sonarjs, results thrown away ----
  {
    rule: 'sonarjs/no-ignored-return',
    scope: 'source',
    violating: `export const f = () => { s.trim() }`,
    compliant: `export const f = () => s.trim()`,
  },
  {
    rule: 'sonarjs/no-misleading-array-reverse',
    scope: 'source',
    violating: `export const f = () => { const reversed = nums.reverse(); return reversed }`,
    compliant: `export const f = () => [...nums].reverse()`,
  },
  {
    rule: 'sonarjs/reduce-initial-value',
    scope: 'source',
    violating: `export const x = nums.reduce((a, b) => a + b)`,
    compliant: `export const x = nums.reduce((a, b) => a + b, 0)`,
  },
  {
    rule: 'sonarjs/array-callback-without-return',
    scope: 'source',
    violating: `export const x = nums.map((v) => { sideEffect(v) })`,
    compliant: `export const x = nums.map((v) => v + 1)`,
  },

  // ---- typed: sonarjs, async ----
  {
    rule: 'sonarjs/no-try-promise',
    scope: 'source',
    violating: `export function f() { try { work() } catch { sideEffect(0) } }`,
    compliant: `export async function f() { try { await work() } catch { sideEffect(0) } }`,
  },
  {
    rule: 'sonarjs/no-async-constructor',
    scope: 'source',
    violating: `export class C { constructor() { void work().then(() => { sideEffect(1) }) } }`,
    compliant: `export class C { constructor() { sideEffect(1) } }`,
  },

  // ---- typed: sonarjs, security ----
  {
    rule: 'sonarjs/post-message',
    scope: 'source',
    violating: `export const f = (w: Window) => { w.postMessage('x', '*') }`,
    compliant: `export const f = (w: Window) => { w.postMessage('x', 'https://example.com') }`,
  },
  {
    rule: 'sonarjs/disabled-resource-integrity',
    scope: 'source',
    // Script elements built imperatively, not JSX, and only when the src carries a version in
    // the path. That is the shape the rule looks for.
    violating: `export const load = () => {
  const el = document.createElement('script')
  el.src = 'https://cdn.example.com/jquery@3.7.1/jquery.min.js'
  document.head.append(el)
}`,
    compliant: `export const load = () => {
  const el = document.createElement('script')
  el.src = 'https://cdn.example.com/jquery@3.7.1/jquery.min.js'
  el.integrity = 'sha384-abc'
  el.crossOrigin = 'anonymous'
  document.head.append(el)
}`,
  },
  {
    rule: 'sonarjs/disabled-auto-escaping',
    scope: 'source',
    violating: `import Handlebars from 'handlebars'
export const t = Handlebars.compile('{{x}}', { noEscape: true })`,
    compliant: `import Handlebars from 'handlebars'
export const t = Handlebars.compile('{{x}}', { noEscape: false })`,
  },
  {
    rule: 'sonarjs/dompurify-unsafe-config',
    scope: 'source',
    violating: `import DOMPurify from 'dompurify'
export const x = DOMPurify.sanitize(s, { ADD_TAGS: ['script'] })`,
    compliant: `import DOMPurify from 'dompurify'
export const x = DOMPurify.sanitize(s)`,
  },
  {
    rule: 'sonarjs/sql-queries',
    scope: 'source',
    violating: `import mysql from 'mysql'
export const f = () => {
  const c = mysql.createConnection({})
  c.query('SELECT * FROM patients WHERE id = ' + s)
}`,
    compliant: `import mysql from 'mysql'
export const f = () => {
  const c = mysql.createConnection({})
  c.query('SELECT * FROM patients WHERE id = ?', [s])
}`,
  },

  // ---- typed: sonarjs, judgement calls ----
  {
    rule: 'sonarjs/arguments-order',
    scope: 'source',
    violating: `declare function g(first: string, second: string): void
export const f = (first: string, second: string) => { g(second, first) }`,
    compliant: `declare function g(first: string, second: string): void
export const f = (first: string, second: string) => { g(first, second) }`,
  },
  {
    rule: 'sonarjs/bitwise-operators',
    scope: 'source',
    // Non-numeric operands only: the rule is looking for `&&`/`||` typed with one ampersand.
    violating: `export const f = (a: boolean, b: boolean) => { if (a | b) { sideEffect(1) } }`,
    compliant: `export const f = (a: boolean, b: boolean) => { if (a || b) { sideEffect(1) } }`,
  },
  {
    rule: 'sonarjs/strings-comparison',
    scope: 'source',
    violating: `export const f = (a: string, b: string) => a < b`,
    compliant: `export const f = (a: string, b: string) => a.localeCompare(b) < 0`,
  },
  {
    rule: 'sonarjs/no-undefined-argument',
    scope: 'source',
    violating: `declare function g(a?: number): void
export const f = () => { g(undefined) }`,
    compliant: `declare function g(a?: number): void
export const f = () => { g() }`,
  },
  {
    rule: 'sonarjs/no-associative-arrays',
    scope: 'source',
    violating: `export const f = () => { const arr: string[] = []; arr['key'] = 'v'; return arr }`,
    compliant: `export const f = () => { const map: Record<string, string> = {}; map.key = 'v'; return map }`,
  },
  {
    rule: 'sonarjs/class-prototype',
    scope: 'source',
    violating: `class C {}
C.prototype.m = function () { return 1 }
export { C }`,
    compliant: `class C { m() { return 1 } }
export { C }`,
  },
  {
    rule: 'sonarjs/no-require-or-define',
    scope: 'source',
    violating: `const path = require('node:path')
export const x = path`,
    compliant: `import * as path from 'node:path'
export const x = path`,
  },
  {
    rule: 'sonarjs/unused-import',
    scope: 'source',
    violating: `import { useState } from 'react'
export const x = 1`,
    compliant: `export const x = 1`,
  },
  {
    rule: 'sonarjs/function-return-type',
    scope: 'source',
    violating: `export function f(x: number) { if (x > 0) { return 'a' } return 1 }`,
    compliant: `export function f(x: number) { if (x > 0) { return 'a' } return 'b' }`,
  },
  {
    rule: 'sonarjs/no-return-type-any',
    scope: 'source',
    violating: `export function f(): any { return 1 }`,
    compliant: `export function f(): number { return 1 }`,
  },
  {
    rule: 'sonarjs/no-useless-intersection',
    scope: 'source',
    violating: `export type T = number & any`,
    compliant: `export type T = { a: number } & { b: number }`,
  },
  {
    rule: 'sonarjs/no-redundant-optional',
    scope: 'source',
    violating: `export interface I { a?: number | undefined }`,
    compliant: `export interface I { a?: number }`,
  },
  {
    rule: 'sonarjs/prefer-immediate-return',
    scope: 'source',
    violating: `export function f() { const r = n + 1; return r }`,
    compliant: `export function f() { return n + 1 }`,
  },
  {
    rule: 'sonarjs/no-small-switch',
    scope: 'source',
    violating: `export function f(k: string) { switch (k) { case 'a': return 1; default: return 0 } }`,
    compliant: `export function f(k: string) { return k === 'a' ? 1 : 0 }`,
  },
  {
    rule: 'sonarjs/no-selector-parameter',
    scope: 'source',
    violating: `export function f(compact: boolean) { if (compact) { sideEffect(1) } else { sideEffect(2) } }`,
    compliant: `export function f(width: number) { sideEffect(width) }`,
  },

  // ---- typed: react ----
  {
    rule: 'sonarjs/prefer-read-only-props',
    scope: 'source',
    violating: `export function C(props: { name: string }) { return <p>{props.name}</p> }`,
    compliant: `export function C(props: Readonly<{ name: string }>) { return <p>{props.name}</p> }`,
  },

  // ---- typed: test files ----
  {
    rule: 'jest/valid-expect-with-promise',
    scope: 'test',
    violating: `it('a', () => { expect(loadUser()).toEqual({ name: 'Ada' }) })`,
    compliant: `it('a', async () => { await expect(loadUser()).resolves.toEqual({ name: 'Ada' }) })`,
  },
  {
    rule: 'jest/no-error-equal',
    scope: 'test',
    violating: `it('a', () => { expect(err).toEqual(new Error('boom')) })`,
    compliant: `it('a', () => { expect(err.message).toBe('boom') })`,
  },
  {
    rule: 'jest/no-unnecessary-assertion',
    scope: 'test',
    violating: `it('a', () => { expect(total).toBeDefined() })`,
    compliant: `it('a', () => { expect(total).toBe(1) })`,
  },
  {
    rule: 'jest/unbound-method',
    scope: 'test',
    violating: `it('a', () => { const m = service.method; m() })`,
    compliant: `it('a', () => { const m = () => { service.method() }; m() })`,
  },
  {
    rule: 'sonarjs/no-incompatible-assertion-types',
    scope: 'test',
    // The explicit import is what tells the rule which assertion library it is reading; the
    // fixture package does not depend on a runner, so the dependency fallback finds nothing.
    violating: `import { it, expect } from '@jest/globals'
it('a', () => { expect(total).toBe('1') })`,
    compliant: `import { it, expect } from '@jest/globals'
it('a', () => { expect(total).toBe(1) })`,
  },
]

export const preambles = {
  test: TEST_PREAMBLE,
  source: SOURCE_PREAMBLE,
  typedTest: TYPED_TEST_PREAMBLE,
  typedSource: TYPED_SOURCE_PREAMBLE,
}
