#!/usr/bin/env node
/**
 * Count the A2 shapes the off-the-shelf jest rules leave uncovered.
 *
 * ```sh
 * node measure-a2-residue.mjs ~/dev/target-monorepo
 * ```
 *
 * A2 asks that an assertion say what it expected. `expect(pred(x)).toBe(true)` fails with
 * `expected false to be true` and names neither `x` nor the property that broke, so the
 * concept is in scope even though no shipped rule sees it. The question this script answers
 * is which of those shapes occur often enough, in a form syntactic enough, to earn a custom
 * rule. Run `measure.mjs <target> jest/` alongside it for what the preset already reports.
 *
 * The buckets are deliberately over-inclusive: a shape landing in one of them is a candidate,
 * not a defect. Reading the samples is the point.
 */
import { readdirSync } from 'node:fs'
import { join, resolve } from 'node:path'

import tsParser from '@typescript-eslint/parser'
import { ESLint } from 'eslint'

import { testFiles } from './globs.js'

const target = process.argv[2]
if (!target) {
  console.error('usage: node measure-a2-residue.mjs <target-repo>')
  process.exit(2)
}
const root = resolve(target)

/** Matcher modifiers that sit between `expect(...)` and the matcher itself. */
const MODIFIERS = new Set(['not', 'resolves', 'rejects'])

/**
 * Unwrap `expect(subject).not.resolves.matcher(args)` from the matcher call outwards.
 * Returns `null` for anything that is not an `expect` chain.
 */
function readExpectCall(node) {
  if (node.callee.type !== 'MemberExpression' || node.callee.computed) return null
  const matcher = node.callee.property.name
  let object = node.callee.object
  while (
    object.type === 'MemberExpression' &&
    !object.computed &&
    MODIFIERS.has(object.property.name)
  ) {
    object = object.object
  }
  if (object.type !== 'CallExpression') return null
  if (object.callee.type !== 'Identifier' || object.callee.name !== 'expect') return null
  if (object.arguments.length !== 1) return null
  return { subject: object.arguments[0], matcher, args: node.arguments }
}

/** `true` when the matcher asserts a boolean literal, whichever way it is spelled. */
function isBooleanAssertion({ matcher, args }) {
  if (matcher === 'toBeTruthy' || matcher === 'toBeFalsy') return true
  if (matcher !== 'toBe' && matcher !== 'toEqual' && matcher !== 'toStrictEqual') return false
  return args.length === 1 && args[0].type === 'Literal' && typeof args[0].value === 'boolean'
}

/** The `.every(...)` / `.some(...)` at the top of a subject, if that is what it is. */
function quantifierCall(subject) {
  if (subject.type !== 'CallExpression') return null
  const { callee } = subject
  if (callee.type !== 'MemberExpression' || callee.computed) return null
  const name = callee.property.name
  return name === 'every' || name === 'some' ? name : null
}

/** Buckets, in the order a subject is tested against them. First match wins. */
const BUCKETS = [
  {
    id: 'quantifier-predicate',
    doc: 'expect(xs.every(p)).toBe(true). The failing element never reaches the message',
    test: (subject, call) => isBooleanAssertion(call) && quantifierCall(subject) !== null,
  },
  {
    id: 'typeof-subject',
    doc: "expect(typeof x).toBe('string'). Reports the tag, never the value behind it",
    test: (subject) => subject.type === 'UnaryExpression' && subject.operator === 'typeof',
  },
  {
    id: 'predicate-call',
    doc: 'expect(pred(x)).toBe(true). The argument that failed the predicate is lost',
    test: (subject, call) => isBooleanAssertion(call) && subject.type === 'CallExpression',
  },
  {
    id: 'negated-subject',
    doc: 'expect(!x).toBe(true). Double negative, and still no value in the message',
    test: (subject, call) =>
      isBooleanAssertion(call) && subject.type === 'UnaryExpression' && subject.operator === '!',
  },
  {
    id: 'plain-truthiness',
    doc: 'expect(obj.flag).toBe(true). The baseline. Named subject, so A2 is already met',
    test: (subject, call) =>
      isBooleanAssertion(call) &&
      (subject.type === 'Identifier' || subject.type === 'MemberExpression'),
  },
]

/** First match wins, so every assertion lands in exactly one bucket. */
function classify(subject, call) {
  return BUCKETS.find((bucket) => bucket.test(subject, call)) ?? null
}

const samples = new Map(BUCKETS.map((b) => [b.id, []]))

const plugin = {
  rules: Object.fromEntries(
    BUCKETS.map((bucket) => [
      bucket.id,
      {
        create(context) {
          return {
            CallExpression(node) {
              const call = readExpectCall(node)
              if (!call) return
              if (classify(call.subject, call) !== bucket) return
              const seen = samples.get(bucket.id)
              if (seen.length < 5) seen.push(context.sourceCode.getText(node).replace(/\s+/g, ' ').slice(0, 100))
              context.report({ node, message: bucket.id })
            },
          }
        },
      },
    ]),
  ),
}

const TEST_FILE = /\.(test|spec)\.(ts|tsx|js|jsx|mts|cts|mjs|cjs)$/
function* walk(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === 'node_modules' || entry.name.startsWith('.')) continue
    const path = join(dir, entry.name)
    if (entry.isDirectory()) yield* walk(path)
    else if (TEST_FILE.test(entry.name)) yield path
  }
}

const files = [...walk(root)]
if (files.length === 0) {
  console.error(`no test files under ${root}`)
  process.exit(1)
}

const eslint = new ESLint({
  cwd: root,
  ignore: false,
  overrideConfigFile: true,
  overrideConfig: [
    {
      files: ['**/*.{ts,tsx,js,jsx,mts,cts,mjs,cjs}'],
      languageOptions: {
        parser: tsParser,
        parserOptions: { ecmaFeatures: { jsx: true }, sourceType: 'module' },
      },
    },
    {
      files: testFiles,
      plugins: { residue: plugin },
      rules: Object.fromEntries(BUCKETS.map((b) => [`residue/${b.id}`, 'error'])),
    },
  ],
})

const counts = new Map()
const filesHit = new Map()
for (const result of await eslint.lintFiles(files)) {
  for (const message of result.messages) {
    const id = message.ruleId ?? `fatal: ${message.message}`
    counts.set(id, (counts.get(id) ?? 0) + 1)
    filesHit.set(id, (filesHit.get(id) ?? new Set()).add(result.filePath))
  }
}

console.log(`${files.length} test files under ${root}\n`)
for (const bucket of BUCKETS) {
  const id = `residue/${bucket.id}`
  console.log(`${String(counts.get(id) ?? 0).padStart(6)} hits  ${String(filesHit.get(id)?.size ?? 0).padStart(4)} files  ${bucket.id}`)
  console.log(`               ${bucket.doc}`)
  for (const sample of samples.get(bucket.id)) console.log(`               | ${sample}`)
  console.log()
}
for (const [id, n] of counts) {
  if (!id.startsWith('residue/')) console.log(`${String(n).padStart(6)} ${id}`)
}
