import { useState } from 'react'

type GreetingProps = {
  first: string
  last: string
}

/** Compliant source: the full name is calculated during render, not synced by an Effect. */
export function Greeting({ first, last }: GreetingProps) {
  const [greeted, setGreeted] = useState(false)
  const fullName = `${first} ${last}`

  return (
    <button type="button" onClick={() => setGreeted(true)}>
      {greeted ? `Hello, ${fullName}` : fullName}
    </button>
  )
}
