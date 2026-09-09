import { describe, it } from 'node:test'

import tsParser from '@typescript-eslint/parser'
import { RuleTester } from 'eslint'

import rule from '../rules/no-quantifier-assertion.js'

// ESLint's RuleTester picks up `describe`/`it` from the global scope when they are there,
// which is how each case gets its own line in `node --test` output. Under `node --test`
// they are module exports rather than globals, so hand them over.
globalThis.describe = describe
globalThis.it = it

const ruleTester = new RuleTester({
  languageOptions: {
    parser: tsParser,
    ecmaVersion: 2022,
    sourceType: 'module',
    parserOptions: { ecmaFeatures: { jsx: true } },
  },
})

const quantifier = (name, expected) => ({
  messageId: 'quantifier',
  data: { quantifier: name, expected: String(expected), actual: String(!expected) },
})

ruleTester.run('no-quantifier-assertion', rule, {
  valid: [
    // ---- the shapes this rule is asking for ----
    {
      name: 'filtering to the offending elements',
      code: `expect(users.filter((u) => !u.active)).toEqual([])`,
    },
    {
      name: 'membership against the collection',
      code: `expect(routes).toContainEqual({ path: 'roles' })`,
    },
    {
      name: 'the quantifier result named and asserted elsewhere',
      code: `const allActive = users.every(isActive); expect(allActive).toBe(true)`,
    },

    // ---- a quantifier, but not asserted as a boolean ----
    {
      name: 'every compared against a non-boolean',
      code: `expect(users.every(isActive)).toBe(undefined)`,
    },
    {
      name: 'a matcher that keeps the collection in the message',
      code: `expect(users.every(isActive)).toMatchSnapshot()`,
    },

    // ---- a boolean assertion, but not on a quantifier ----
    {
      name: 'a named predicate call',
      code: `expect(isActive(user)).toBe(true)`,
    },
    {
      name: 'a plain property',
      code: `expect(user.active).toBe(true)`,
    },
    {
      name: 'a method that is not a quantifier',
      code: `expect(users.includes(user)).toBe(true)`,
    },
    {
      name: 'every called with no callback',
      code: `expect(users.every()).toBe(true)`,
    },
    {
      name: 'a computed member that is only spelled every at runtime',
      code: `expect(users[method](isActive)).toBe(true)`,
    },

    // ---- modifiers, excluded on purpose ----
    {
      name: 'not',
      code: `expect(users.every(isActive)).not.toBe(true)`,
    },
    {
      name: 'resolves',
      code: `expect(loaded.every(isActive)).resolves.toBe(true)`,
    },
    {
      name: 'rejects',
      code: `expect(loaded.some(isActive)).rejects.toBe(true)`,
    },

    // ---- not an expect at all ----
    {
      name: 'a bare quantifier in production code',
      code: `if (users.every(isActive)) { activate() }`,
    },
    {
      name: 'a same-named helper that is not expect',
      code: `assert(users.every(isActive)).toBe(true)`,
    },
    {
      name: 'expect with vitest per-assertion message, whose rewrite would drop it',
      code: `expect(users.every(isActive), 'all active').toBe(true)`,
    },
  ],

  invalid: [
    {
      name: 'every asserted true',
      code: `expect(users.every(isActive)).toBe(true)`,
      errors: [quantifier('every', true)],
    },
    {
      name: 'every asserted false',
      code: `expect(users.every(isActive)).toBe(false)`,
      errors: [quantifier('every', false)],
    },
    {
      name: 'some asserted true',
      code: `expect(routes.some((r) => r.path === 'roles')).toBe(true)`,
      errors: [quantifier('some', true)],
    },
    {
      name: 'some asserted false',
      code: `expect(calls.some(([, reason]) => reason === 'login')).toBe(false)`,
      errors: [quantifier('some', false)],
    },
    {
      name: 'toEqual rather than toBe',
      code: `expect(users.every(isActive)).toEqual(true)`,
      errors: [quantifier('every', true)],
    },
    {
      name: 'toStrictEqual rather than toBe',
      code: `expect(users.every(isActive)).toStrictEqual(true)`,
      errors: [quantifier('every', true)],
    },
    {
      name: 'toBeTruthy',
      code: `expect(users.every(isActive)).toBeTruthy()`,
      errors: [quantifier('every', true)],
    },
    {
      name: 'toBeFalsy',
      code: `expect(users.some(isBanned)).toBeFalsy()`,
      errors: [quantifier('some', false)],
    },
    {
      name: 'a quantifier at the end of a longer chain',
      code: `expect(Object.values(sizes).every((n) => typeof n === 'number')).toBe(true)`,
      errors: [quantifier('every', true)],
    },
    {
      name: 'a cast receiver, which the rule looks straight through',
      code: `expect((cache.get(KEY) as Note[]).some((n) => n.pending)).toBe(true)`,
      errors: [quantifier('some', true)],
    },
  ],
})
