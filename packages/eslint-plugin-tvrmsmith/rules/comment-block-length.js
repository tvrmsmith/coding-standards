/**
 * Guideline **"Comments"** (coding-standards/SKILL.md) — "a paragraph justifying a workaround
 * means the code is wrong". Its C# half is the `CommentBlockLengthAnalyzer` Roslyn analyzer.
 *
 * The skill states the judgement is *whether the comment is justifying something*, not how long
 * it is, and no lint rule can read intent. Length is the one mechanical proxy available, and it
 * is a weak one: a long comment is a prompt to re-read the code, not a verdict on it. A proxy
 * that weak warns and never fails, and the message names the question rather than the fix.
 *
 * Doc comments (`/** ... *\/`) are exempt. The same skill section requires documenting "public
 * API contracts" and "non-obvious logic, invariants, units, side effects", so a rule that
 * punished a thorough JSDoc block would contradict the standard it enforces. Measured against
 * this repo, every comment block over ten lines is a doc comment and no non-doc block exceeds
 * ten, so the exemption is what makes the threshold land as a regression guard rather than a
 * refactor mandate.
 *
 * A doc comment also *ends* the run it sits next to rather than absorbing it. Otherwise a
 * two-line summary glued above thirty lines of prose would exempt the prose, which is the shape
 * the guard exists to catch. Roslyn splits them this way of its own accord, so the C# half got
 * there first and this side follows it.
 */

/** Whether nothing but whitespace precedes the comment on its own opening line. */
function startsItsLine(sourceCode, comment) {
  const line = sourceCode.lines[comment.loc.start.line - 1]
  return line.slice(0, comment.loc.start.column).trim() === ''
}

/**
 * Whether the comment is a documentation comment — `/** ... *\/` in JS and TS.
 *
 * Read off the raw text rather than the node type: a block comment's `value` is everything
 * between the delimiters, so a JSDoc opener leaves a leading `*` behind.
 */
function isDocComment(comment) {
  return comment.type === 'Block' && comment.value.startsWith('*')
}

/**
 * The comment blocks in a file, in source order.
 *
 * A block is a maximal run of own-line, non-documentation comments on consecutive lines. A blank
 * line ends a block, because the author put a gap there and two paragraphs separated by one are
 * two comments. A trailing comment (`x++ // why`) neither opens nor extends a block: it annotates
 * the code on its line, and gluing a run of them together would measure the code, not the
 * comment. A doc comment ends a block too, and never appears in one.
 *
 * @returns {{ comments: import('estree').Comment[], lines: number }[]}
 */
function readCommentBlocks(sourceCode) {
  const blocks = []
  let current = null

  for (const comment of sourceCode.getAllComments()) {
    if (isDocComment(comment) || !startsItsLine(sourceCode, comment)) {
      current = null
      continue
    }

    const previous = current?.comments[current.comments.length - 1]
    if (previous && comment.loc.start.line === previous.loc.end.line + 1) {
      current.comments.push(comment)
    } else {
      current = { comments: [comment] }
      blocks.push(current)
    }
  }

  return blocks.map((block) => {
    const first = block.comments[0]
    const last = block.comments[block.comments.length - 1]
    return { ...block, lines: last.loc.end.line - first.loc.start.line + 1 }
  })
}

/** @type {import('eslint').Rule.RuleModule} */
const rule = {
  meta: {
    type: 'suggestion',
    docs: {
      description: 'Limit how many lines one run of non-documentation comments may span',
      recommended: true,
      url: 'https://github.com/tvrmsmith/coding-standards/blob/main/packages/eslint-plugin-tvrmsmith/docs/rules/comment-block-length.md',
    },
    // No fixer, and there cannot be one. Every honest response — shortening the prose, moving
    // it to a doc comment, extracting the code it describes into a named function, or fixing
    // the code it justifies — is a judgement the author has to make.
    schema: [
      {
        type: 'object',
        /** `max` is the budget: a block spanning more lines than this is reported. */
        properties: {
          max: { type: 'integer', minimum: 1 },
        },
        additionalProperties: false,
      },
    ],
    defaultOptions: [{ max: 10 }],
    messages: {
      tooLong:
        '{{lines}}-line comment block, over the {{max}}-line budget. Is it justifying the code below it? Then fix the code. Is it documenting a contract or an invariant? Then make it a doc comment, which is exempt.',
    },
  },

  create(context) {
    const { max = 10 } = context.options[0] ?? {}
    const { sourceCode } = context

    return {
      'Program:exit': () => {
        for (const block of readCommentBlocks(sourceCode)) {
          if (block.lines <= max) continue

          const first = block.comments[0]
          const last = block.comments[block.comments.length - 1]

          context.report({
            loc: { start: first.loc.start, end: last.loc.end },
            messageId: 'tooLong',
            data: { lines: String(block.lines), max: String(max) },
          })
        }
      },
    }
  },
}

export default rule
