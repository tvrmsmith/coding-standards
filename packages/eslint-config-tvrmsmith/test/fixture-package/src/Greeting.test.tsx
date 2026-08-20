import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { Greeting } from './Greeting'

/**
 * Compliant test: `screen` queries, `userEvent` with `setup()`, jest-dom matchers,
 * awaited async helpers, no `!` or `?.` inside `expect(...)`. The preset must stay
 * silent on every line of this file.
 */
it('greets after the button is pressed', async () => {
  const user = userEvent.setup()
  render(<Greeting first="Ada" last="Lovelace" />)

  const button = screen.getByRole('button', { name: 'Ada Lovelace' })
  expect(button).toBeInTheDocument()

  await user.click(button)

  expect(await screen.findByRole('button', { name: 'Hello, Ada Lovelace' })).toHaveTextContent(
    'Hello, Ada Lovelace',
  )
})
