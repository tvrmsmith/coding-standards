/**
 * Guideline **A2, "Assertions should communicate meaning"** (test-best-practices/SKILL.md).
 * The off-the-shelf half of A2 is `eslint-plugin-jest`'s `prefer-*` family plus
 * `eslint-plugin-jest-dom`; this rule covers the one high-frequency shape measured at the
 * adoption target that none of them see.
 *
 * `expect(users.every(isActive)).toBe(true)` collapses the whole collection to one bit
 * before the matcher ever runs, so the failure reads `expected false to be true`. Which
 * element failed, and what it held, are gone. Asserting against the collection keeps them:
 * the matcher prints the offending values because it still has them.
 *
 * Narrow on purpose, matching the A1 rule beside it. Only the direct
 * `expect(<quantifier>).<matcher>(<boolean>)` shape reports; anything with `.not`,
 * `.resolves` or `.rejects` in the chain is left alone. That costs almost nothing: of the
 * 71 files carrying this shape at the adoption target, the modifier forms did not occur.
 */

/** `Array#every` and `Array#some`, the two calls that reduce a collection to one bit. */
const QUANTIFIERS = new Set(['every', 'some'])

/**
 * Matchers that assert a boolean. `toBe` / `toEqual` / `toStrictEqual` need a boolean
 * literal argument to qualify; `toBeTruthy` / `toBeFalsy` are boolean by definition.
 * All five exist under both vitest and jest, which one repo often has side by side.
 */
const EQUALITY_MATCHERS = new Set(['toBe', 'toEqual', 'toStrictEqual'])
const TRUTHINESS_MATCHERS = new Set(['toBeTruthy', 'toBeFalsy'])

/**
 * The quantifier name in `expect(<receiver>.every(...))`, or `null` when the subject is
 * anything else.
 *
 * A computed member (`xs[key](p)`) is not statically an `every`, and a quantifier called
 * with no callback is not the shape this rule is about, so both are left alone.
 *
 * @param {import('estree').Node} subject
 * @returns {string | null}
 */
function readQuantifier(subject) {
  if (subject.type !== 'CallExpression') return null
  if (subject.arguments.length === 0) return null

  const { callee } = subject
  if (callee.type !== 'MemberExpression' || callee.computed || callee.optional) return null
  if (callee.property.type !== 'Identifier') return null

  return QUANTIFIERS.has(callee.property.name) ? callee.property.name : null
}

/**
 * The boolean the matcher asserts, or `null` when it asserts something else.
 *
 * @param {string} matcher
 * @param {import('estree').Node[]} args
 * @returns {boolean | null}
 */
function readAssertedBoolean(matcher, args) {
  if (TRUTHINESS_MATCHERS.has(matcher)) return matcher === 'toBeTruthy'
  if (!EQUALITY_MATCHERS.has(matcher)) return null
  if (args.length !== 1) return null

  const [argument] = args
  return argument.type === 'Literal' && typeof argument.value === 'boolean' ? argument.value : null
}

/** @type {import('eslint').Rule.RuleModule} */
export default {
  meta: {
    type: 'suggestion',
    docs: {
      description:
        'Assert against the collection rather than against the boolean an every/some call reduces it to',
      recommended: true,
      url: 'https://github.com/tvrmsmith/coding-standards/blob/main/packages/eslint-plugin-tvrmsmith/docs/rules/no-quantifier-assertion.md',
    },
    // No fixer. The rewrite depends on what the test means: an `every` that should have
    // been a `filter(...).toEqual([])`, a `some` that should have been `toContainEqual`,
    // or a predicate worth inverting. Prefer no fix over a wrong one.
    schema: [],
    messages: {
      quantifier:
        "expect(...{{quantifier}}(...)) reduces the collection to one boolean, so the failure reads 'expected {{actual}} to be {{expected}}' and names neither the element that failed nor its value. Assert against the collection instead: expect(items.filter(predicate)).toEqual([...]) reports the offending elements, and toContain / toContainEqual covers membership.",
    },
  },

  create(context) {
    return {
      CallExpression(node) {
        const matcherAccess = node.callee
        if (matcherAccess.type !== 'MemberExpression' || matcherAccess.computed) return
        if (matcherAccess.property.type !== 'Identifier') return

        // Nothing may sit between `expect(...)` and the matcher. That one check excludes
        // `.not`, `.resolves` and `.rejects` without naming any of them.
        const expectCall = matcherAccess.object
        if (expectCall.type !== 'CallExpression') return
        if (expectCall.callee.type !== 'Identifier' || expectCall.callee.name !== 'expect') return
        if (expectCall.arguments.length !== 1) return

        const quantifier = readQuantifier(expectCall.arguments[0])
        if (quantifier === null) return

        const expected = readAssertedBoolean(matcherAccess.property.name, node.arguments)
        if (expected === null) return

        context.report({
          node,
          messageId: 'quantifier',
          data: { quantifier, expected: String(expected), actual: String(!expected) },
        })
      },
    }
  },
}
