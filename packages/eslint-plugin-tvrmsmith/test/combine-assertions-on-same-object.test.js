import { describe, it } from 'node:test'

import tsParser from '@typescript-eslint/parser'
import { RuleTester } from 'eslint'

import rule from '../rules/combine-assertions-on-same-object.js'

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

/** The rule reports once per run, spanning first statement to last. */
const combine = (count, root, properties) => ({
  messageId: 'combine',
  data: { count: String(count), root, properties },
})

ruleTester.run('combine-assertions-on-same-object', rule, {
  valid: [
    // ---- nothing to combine ----
    {
      name: 'a single assertion',
      code: `it('a', () => { expect(result.page).toBe(2) })`,
    },
    {
      name: 'the object itself, already the combined form',
      code: `it('a', () => { expect(result).toEqual({ page: 2, total: 10 }) })`,
    },
    {
      name: 'assertions on different objects',
      code: `it('a', () => {
        expect(response.status).toBe(200)
        expect(request.method).toBe('GET')
      })`,
    },
    {
      name: 'no expect at all',
      code: `const a = result.page; const b = result.total`,
    },
    {
      name: 'expect.assertions before a lone assertion',
      code: `it('a', () => {
        expect.assertions(1)
        expect(result.page).toBe(2)
      })`,
    },

    // ---- not back-to-back ----
    {
      name: 'a statement between them, which may be the mutation that changes the object',
      code: `it('a', () => {
        expect(cart.total).toBe(10)
        cart.add(item)
        expect(cart.count).toBe(2)
      })`,
    },
    {
      name: 'assertions in sibling blocks',
      code: `it('a', () => {
        if (flag) { expect(result.page).toBe(2) } else { expect(result.total).toBe(10) }
      })`,
    },
    {
      name: 'assertions in separate tests',
      code: `it('a', () => { expect(result.page).toBe(2) })
             it('b', () => { expect(result.total).toBe(10) })`,
    },

    // ---- modifiers the combined form cannot express ----
    {
      name: 'negation — a negated object literal is a weaker assertion, not the same one',
      code: `it('a', () => {
        expect(result.page).not.toBe(2)
        expect(result.total).not.toBe(10)
      })`,
    },
    {
      name: 'one negated, one not',
      code: `it('a', () => {
        expect(result.page).toBe(2)
        expect(result.total).not.toBe(10)
      })`,
    },
    {
      name: 'resolves — combining would change the await semantics',
      code: `it('a', async () => {
        await expect(result.page).resolves.toBe(2)
        await expect(result.total).resolves.toBe(10)
      })`,
    },
    {
      name: 'rejects',
      code: `it('a', async () => {
        await expect(result.save).rejects.toEqual(err)
        await expect(result.load).rejects.toEqual(err)
      })`,
    },

    // ---- matchers with no object-literal equivalent ----
    {
      name: 'toHaveLength and toContain',
      code: `it('a', () => {
        expect(result.items).toHaveLength(3)
        expect(result.names).toContain('Ada')
      })`,
    },
    {
      name: 'ordering and truthiness matchers',
      code: `it('a', () => {
        expect(result.total).toBeGreaterThan(0)
        expect(result.hasMore).toBeTruthy()
      })`,
    },
    {
      name: 'jest-dom matchers on the same element',
      code: `it('a', () => {
        expect(form.submit).toBeVisible()
        expect(form.reset).toBeDisabled()
      })`,
    },
    {
      name: 'one combinable matcher next to one that is not — the run never reaches two',
      code: `it('a', () => {
        expect(result.page).toBe(2)
        expect(result.items).toHaveLength(3)
        expect(result.total).toBe(10)
      })`,
    },

    // ---- subjects that are not a static property path ----
    {
      name: 'indexed elements — the combined form is an array, not an object literal',
      code: `it('a', () => {
        expect(items[0].id).toBe(1)
        expect(items[1].id).toBe(2)
      })`,
    },
    {
      name: 'call results — two reads need not be the same object',
      code: `it('a', () => {
        expect(getUser().name).toBe('Ada')
        expect(getUser().age).toBe(36)
      })`,
    },
    {
      name: 'optional chaining, which guideline A4 bans outright',
      code: `it('a', () => {
        expect(result?.page).toBe(2)
        expect(result?.total).toBe(10)
      })`,
    },
    {
      name: 'the whole object next to one of its properties',
      code: `it('a', () => {
        expect(result).toEqual(expected)
        expect(result.page).toBe(2)
      })`,
    },
    {
      name: "vitest's per-assertion message, which combining would discard",
      code: `it('a', () => {
        expect(result.page, 'first page').toBe(2)
        expect(result.total, 'all rows').toBe(10)
      })`,
    },

    // ---- paths the combined object literal cannot hold ----
    {
      name: 'the same property twice — one key cannot hold two values',
      code: `it('a', () => {
        expect(result.page).toBe(2)
        expect(result.page).toEqual(pageTwo)
      })`,
    },
    {
      name: 'overlapping paths — a key cannot be both a value and an object',
      code: `it('a', () => {
        expect(result.user).toEqual(ada)
        expect(result.user.name).toBe('Ada')
      })`,
    },
  ],

  invalid: [
    {
      name: 'the guideline example — three properties, first failure hides the rest',
      code: `it('a', () => {
        expect(result.page).toBe(2)
        expect(result.pageSize).toBe(3)
        expect(result.total).toBe(10)
      })`,
      errors: [combine(3, 'result', 'page, pageSize, total')],
    },
    {
      name: 'two is already enough',
      code: `it('a', () => {
        expect(response.status).toBe(200)
        expect(response.ok).toBe(true)
      })`,
      errors: [combine(2, 'response', 'status, ok')],
    },
    {
      name: 'mixed equality matchers still combine',
      code: `it('a', () => {
        expect(result.page).toBe(2)
        expect(result.rows).toEqual(rows)
        expect(result.meta).toStrictEqual(meta)
      })`,
      errors: [combine(3, 'result', 'page, rows, meta')],
    },
    {
      name: 'nested paths, which nest in the combined literal too',
      code: `it('a', () => {
        expect(result.user.name).toBe('Ada')
        expect(result.user.age).toBe(36)
      })`,
      errors: [combine(2, 'result', 'user.name, user.age')],
    },
    {
      name: 'a comment between them does not break the run',
      code: `it('a', () => {
        expect(result.page).toBe(2)
        // the second page holds the last three rows
        expect(result.total).toBe(10)
      })`,
      errors: [combine(2, 'result', 'page, total')],
    },
    {
      name: 'a run ends where the object changes, and the next run is judged on its own',
      code: `it('a', () => {
        expect(result.page).toBe(2)
        expect(result.total).toBe(10)
        expect(request.method).toBe('GET')
        expect(request.url).toBe('/rows')
      })`,
      errors: [combine(2, 'result', 'page, total'), combine(2, 'request', 'method, url')],
    },
    {
      name: 'two separate runs on the same object, split by a mutation',
      code: `it('a', () => {
        expect(cart.total).toBe(10)
        expect(cart.count).toBe(2)
        cart.add(item)
        expect(cart.total).toBe(15)
        expect(cart.count).toBe(3)
      })`,
      errors: [combine(2, 'cart', 'total, count'), combine(2, 'cart', 'total, count')],
    },
    {
      name: 'at module scope, outside any test callback',
      code: `expect(config.host).toBe('localhost')
             expect(config.port).toBe(5432)`,
      errors: [combine(2, 'config', 'host, port')],
    },
    {
      name: 'the report spans the whole run',
      code: `it('a', () => {
        expect(result.page).toBe(2)
        expect(result.total).toBe(10)
      })`,
      errors: [{ messageId: 'combine', line: 2, endLine: 3 }],
    },
  ],
})
