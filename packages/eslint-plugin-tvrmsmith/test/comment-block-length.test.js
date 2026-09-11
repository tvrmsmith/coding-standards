import { describe, it } from 'node:test'

import tsParser from '@typescript-eslint/parser'
import { RuleTester } from 'eslint'

import rule from '../rules/comment-block-length.js'

// See the note in combine-assertions-on-same-object.test.js: RuleTester takes `describe`/`it`
// from the global scope, and under `node --test` they are module exports rather than globals.
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

/** `n` own-line `//` comments on consecutive lines, each distinguishable in a failure message. */
const lineComments = (n) =>
  Array.from({ length: n }, (_, i) => `// paragraph line ${i + 1}`).join('\n')

/** One `/* *\/` block comment spanning `n` lines. */
const blockComment = (n) =>
  ['/*', ...Array.from({ length: n - 2 }, (_, i) => ` * body ${i + 1}`), ' */'].join('\n')

/** One doc comment spanning `n` lines. */
const docComment = (n) =>
  ['/**', ...Array.from({ length: n - 2 }, (_, i) => ` * body ${i + 1}`), ' */'].join('\n')

const tooLong = (lines, max) => ({ messageId: 'tooLong', data: { lines: String(lines), max: String(max) } })

ruleTester.run('comment-block-length', rule, {
  valid: [
    // ---- inside the budget ----
    {
      name: 'a block exactly at the default budget',
      code: `${lineComments(10)}\nconst a = 1`,
    },
    {
      name: 'a short block comment',
      code: `${blockComment(4)}\nconst a = 1`,
    },
    {
      name: 'no comments at all',
      code: 'const a = 1',
    },

    // ---- doc comments are exempt ----
    {
      name: 'a long doc comment, which the skill asks for on public contracts',
      code: `${docComment(40)}\nexport function f() {}`,
    },
    {
      name: 'a short run ended by the doc comment below it',
      code: `${lineComments(6)}\n${docComment(6)}\nexport function f() {}`,
    },

    // ---- a blank line ends a block ----
    {
      name: 'two short paragraphs separated by a blank line',
      code: `${lineComments(8)}\n\n${lineComments(8)}\nconst a = 1`,
    },

    // ---- trailing comments never form a block ----
    {
      name: 'a run of trailing comments on consecutive lines of code',
      code: Array.from({ length: 15 }, (_, i) => `const v${i} = ${i} // why ${i}`).join('\n'),
    },
    {
      name: 'a trailing comment does not extend the own-line block above it',
      code: `${lineComments(9)}\nconst a = 1 // and one more`,
    },

    // ---- the budget is configurable ----
    {
      name: 'inside a raised budget',
      code: `${lineComments(14)}\nconst a = 1`,
      options: [{ max: 20 }],
    },
  ],

  invalid: [
    {
      name: 'a run of line comments one over the budget',
      code: `${lineComments(11)}\nconst a = 1`,
      errors: [tooLong(11, 10)],
    },
    {
      name: 'a single block comment over the budget',
      code: `${blockComment(14)}\nconst a = 1`,
      errors: [tooLong(14, 10)],
    },
    {
      name: 'a run mixing a non-doc block comment and line comments',
      code: `${blockComment(6)}\n${lineComments(6)}\nconst a = 1`,
      errors: [tooLong(12, 10)],
    },
    {
      name: 'a much longer block is still one report',
      code: `${lineComments(30)}\nconst a = 1`,
      errors: [tooLong(30, 10)],
    },
    {
      name: 'over a lowered budget',
      code: `${lineComments(5)}\nconst a = 1`,
      options: [{ max: 3 }],
      errors: [tooLong(5, 3)],
    },
    {
      name: 'each of two long blocks is reported separately',
      code: `${lineComments(11)}\n\n${lineComments(12)}\nconst a = 1`,
      errors: [tooLong(11, 10), tooLong(12, 10)],
    },
    {
      name: 'a comment block inside a function body',
      code: `function f() {\n${lineComments(11)}\nreturn 1\n}`,
      errors: [tooLong(11, 10)],
    },
    // A doc comment ends the run rather than absorbing it, so a short summary cannot be used
    // to exempt a long stretch of prose glued underneath it.
    {
      name: 'the plain run below a doc comment is measured on its own',
      code: `${docComment(12)}\n${lineComments(12)}\nconst a = 1`,
      errors: [tooLong(12, 10)],
    },
    {
      name: 'a two-line summary does not exempt the prose below it',
      code: `${docComment(2)}\n${lineComments(14)}\nconst a = 1`,
      errors: [tooLong(14, 10)],
    },
  ],
})
