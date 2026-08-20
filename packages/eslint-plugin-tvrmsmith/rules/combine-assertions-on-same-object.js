/**
 * Guideline **A1, "Combine assertions on the same object"** (test-best-practices/SKILL.md)
 * — the one guideline in the whole inventory with no off-the-shelf coverage in either
 * language. Its C# half is the Roslyn analyzer of the same name.
 *
 * Back-to-back assertions on the same object are wasteful and fragile: when the first
 * fails the rest never run, so one report hides the others and the failure takes several
 * round trips to characterise. One structural assertion reports every mismatch at once.
 *
 * The rule is deliberately narrow. A false-positive-heavy rule gets the whole preset
 * disabled, so every shape where the combined form would not be equivalent — a negation,
 * a non-equality matcher, an indexed element, a repeated or overlapping property path —
 * is left alone rather than guessed at. The near-miss cases in the test suite are the
 * specification of that narrowness.
 */

/**
 * Matchers whose combined form is a plain object literal. `toBe`, `toEqual` and
 * `toStrictEqual` exist under both vitest and jest, which matters: one repo often has both,
 * so nothing here may key off a single runner.
 *
 * Everything else is excluded on purpose. `toHaveLength`, `toContain`, `toBeGreaterThan`
 * and friends have no object-literal equivalent — folding them into `toEqual` would
 * assert something different from what the author wrote.
 */
const COMBINABLE_MATCHERS = new Set(['toBe', 'toEqual', 'toStrictEqual'])

/**
 * The dotted property path a subject reads, rooted at a plain identifier.
 *
 * Returns `null` for anything that is not a static, non-empty property path — a bare
 * identifier (`expect(result)`, already the combined form), a call result
 * (`expect(get().a)`, where the two reads need not be the same object), an indexed
 * element (`expect(items[0].id)`, where the combined form is an array rather than an
 * object literal), or an optional chain (banned outright by guideline A4).
 *
 * @param {import('estree').Node} node
 * @returns {{ root: string, path: string[] } | null}
 */
function readPropertyPath(node) {
  const path = []
  let current = node

  while (current.type === 'MemberExpression') {
    if (current.computed || current.optional || current.property.type !== 'Identifier') return null
    path.unshift(current.property.name)
    current = current.object
  }

  if (current.type !== 'Identifier' || path.length === 0) return null
  return { root: current.name, path }
}

/**
 * The subject of `expect(<subject>).<combinable matcher>(...)`, or `null` for any other
 * statement.
 *
 * The shape is matched exactly: nothing may sit between the `expect(...)` call and the
 * matcher. That single check excludes `.not` (whose combined form would be a negated
 * object, which is a weaker assertion, not the same one) and `.resolves` / `.rejects`
 * (whose combined form has different await semantics) without naming either.
 *
 * A second argument to `expect` is vitest's per-assertion message; combining would
 * discard it, so those are left alone too.
 *
 * @param {import('estree').Node} statement
 * @returns {{ root: string, path: string[] } | null}
 */
function readAssertionSubject(statement) {
  if (statement.type !== 'ExpressionStatement') return null

  const matcherCall = statement.expression
  if (matcherCall.type !== 'CallExpression') return null

  const matcher = matcherCall.callee
  if (matcher.type !== 'MemberExpression' || matcher.computed) return null
  if (matcher.property.type !== 'Identifier') return null
  if (!COMBINABLE_MATCHERS.has(matcher.property.name)) return null

  const expectCall = matcher.object
  if (expectCall.type !== 'CallExpression') return null
  if (expectCall.callee.type !== 'Identifier' || expectCall.callee.name !== 'expect') return null
  if (expectCall.arguments.length !== 1) return null

  return readPropertyPath(expectCall.arguments[0])
}

/** Whether every path in the run is a distinct key in the combined object literal. */
function pathsCombineCleanly(subjects) {
  const dotted = subjects.map(({ path }) => path.join('.'))

  return dotted.every((path, index) =>
    dotted.every(
      (other, otherIndex) =>
        index === otherIndex ||
        // A repeat (`r.a` twice) would need the same key twice; an overlap (`r.user` and
        // `r.user.name`) would need a key to be both a value and an object. Either way the
        // author is asserting something the combined form cannot express.
        !(other === path || other.startsWith(`${path}.`)),
    ),
  )
}

/** @type {import('eslint').Rule.RuleModule} */
export default {
  meta: {
    type: 'suggestion',
    docs: {
      description:
        'Combine consecutive assertions on properties of the same object into one structural assertion',
      recommended: true,
      url: 'https://github.com/tvrmsmith/coding-standards/blob/main/packages/eslint-plugin-tvrmsmith/docs/rules/combine-assertions-on-same-object.md',
    },
    // No fixer and no suggestion, on purpose. `toBe` is reference equality and `toEqual`
    // is structural, so rewriting `expect(r.a).toBe(obj)` as `expect(r).toEqual({ a: obj })`
    // silently weakens it; and choosing between `toEqual` (exhaustive) and `toMatchObject`
    // (subset) is the author's call, not the rule's. Prefer no fix over a wrong one.
    schema: [],
    messages: {
      combine:
        "{{count}} consecutive assertions on properties of '{{root}}' ({{properties}}). If the first fails the rest never run, so the other mismatches stay hidden. Assert once against '{{root}}' with toEqual({ ... }) for the whole shape, or toMatchObject({ ... }) for a subset.",
    },
  },

  create(context) {
    /** @param {import('estree').Node[]} statements */
    function checkStatementList(statements) {
      let run = []

      const flush = () => {
        if (run.length >= 2 && pathsCombineCleanly(run)) {
          const [first] = run
          const last = run[run.length - 1]

          context.report({
            loc: { start: first.statement.loc.start, end: last.statement.loc.end },
            messageId: 'combine',
            data: {
              count: String(run.length),
              root: first.root,
              properties: run.map(({ path }) => path.join('.')).join(', '),
            },
          })
        }
        run = []
      }

      for (const statement of statements) {
        const subject = readAssertionSubject(statement)

        // A run is broken by anything that is not an assertion on the same root — a
        // different object, a non-combinable matcher, or an intervening statement, which
        // may well be the mutation that makes the two reads different values.
        if (!subject || (run.length > 0 && subject.root !== run[0].root)) {
          flush()
        }
        if (subject) run.push({ ...subject, statement })
      }

      flush()
    }

    return {
      Program: (node) => checkStatementList(node.body),
      BlockStatement: (node) => checkStatementList(node.body),
      StaticBlock: (node) => checkStatementList(node.body),
      SwitchCase: (node) => checkStatementList(node.consequent),
    }
  },
}
