import assert from 'node:assert/strict'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import test, { describe } from 'node:test'

import tsParser from '@typescript-eslint/parser'
import { ESLint } from 'eslint'

import base from '../base.js'
import preset from '../index.js'
import react from '../react.js'
import { cases, preambles } from './cases.js'

const fixturePackage = join(dirname(fileURLToPath(import.meta.url)), 'fixture-package')

/**
 * Stands in for the consumer package's own config. The preset is spread *after* it,
 * exactly as a real package layers it.
 */
const packageOwnConfig = {
  name: 'fixture-package/own',
  files: ['**/*.{ts,tsx}'],
  languageOptions: {
    parser: tsParser,
    parserOptions: { ecmaFeatures: { jsx: true }, sourceType: 'module' },
  },
}

const eslint = new ESLint({
  cwd: fixturePackage,
  overrideConfigFile: true,
  overrideConfig: [packageOwnConfig, ...preset],
})

const filePathFor = (scope) =>
  join(fixturePackage, scope === 'test' ? 'src/sample.test.tsx' : 'src/Sample.tsx')

async function reportsOn(code, scope) {
  const [result] = await eslint.lintText(preambles[scope] + code, {
    filePath: filePathFor(scope),
  })
  return result.messages.map((m) => ({ rule: m.ruleId ?? `fatal: ${m.message}`, text: m.message }))
}

async function rulesFiredOn(code, scope) {
  return (await reportsOn(code, scope)).map((r) => r.rule)
}

/**
 * A case matches a report by rule id, and — for the several guidelines sharing
 * `no-restricted-syntax` — by the selector's own message too.
 */
const matcherFor = ({ rule, selector }) =>
  selector
    ? (report) => report.rule === rule && report.text === selector.message
    : (report) => report.rule === rule

const describeCase = ({ rule, selector }) => (selector ? `${rule} (${selector.selector})` : rule)

/** Every rule the preset enables, flattened from the slices themselves. */
const enabledRules = new Set(preset.flatMap((config) => Object.keys(config.rules ?? {})))

describe('every enabled rule has a fixture case', () => {
  test('the case list and the preset name the same rules', () => {
    const covered = new Set(cases.map((c) => c.rule))
    assert.deepEqual(
      [...enabledRules].filter((r) => !covered.has(r)),
      [],
      'enabled rules with no fixture case',
    )
    assert.deepEqual(
      [...covered].filter((r) => !enabledRules.has(r)),
      [],
      'fixture cases for rules the preset does not enable',
    )
  })
})

describe('each rule fires on a violation and stays quiet on compliant code', () => {
  for (const testCase of cases) {
    const { scope, violating, compliant } = testCase
    const matches = matcherFor(testCase)
    const label = describeCase(testCase)

    test(`${label} fires`, async () => {
      const reports = await reportsOn(violating, scope)
      assert.ok(
        reports.some(matches),
        `expected ${label}, got: ${reports.map((r) => r.rule).join(', ') || 'nothing'}`,
      )
    })

    test(`${label} stays quiet`, async () => {
      const reports = await reportsOn(compliant, scope)
      assert.ok(
        !reports.some(matches),
        `${label} fired on the compliant sample; all reports: ${reports.map((r) => r.rule).join(', ')}`,
      )
    })
  }
})

describe('slice scoping', () => {
  test('the React/RTL testing rules do not apply to production source', async () => {
    const fired = await rulesFiredOn(
      `export const f = () => { fireEvent.click(el) }`.replace('fireEvent', 'globalThis.fireEvent'),
      'source',
    )
    assert.ok(!fired.some((r) => r?.startsWith('testing-library/')))
  })

  test('base is React-free', () => {
    const plugins = base.flatMap((config) => Object.keys(config.plugins ?? {})).sort()
    assert.deepEqual(plugins, ['@typescript-eslint', 'tvrmsmith'])
  })

  test('react carries only the React and RTL plugins', () => {
    const plugins = react.flatMap((config) => Object.keys(config.plugins ?? {})).sort()
    assert.deepEqual(plugins, [
      'jest-dom',
      'react-hooks',
      'react-you-might-not-need-an-effect',
      'testing-library',
    ])
  })

  test('no rule from a test-runner plugin is enabled', () => {
    const runnerScoped = [...enabledRules].filter(
      (r) => r.startsWith('vitest/') || r.startsWith('jest/'),
    )
    assert.deepEqual(runnerScoped, [], 'the preset must not assume vitest or jest')
  })
})

describe('the fixture package lints clean', () => {
  test('no report on any compliant fixture file', async () => {
    const results = await eslint.lintFiles([join(fixturePackage, 'src')])
    const reports = results.flatMap((r) =>
      r.messages.map((m) => `${r.filePath}: ${m.ruleId ?? m.message}`),
    )
    assert.deepEqual(reports, [])
  })
})
