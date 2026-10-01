/**
 * Joins the review A/B log to a branch's no-mistakes runs: which arm each review aspect ran under, what
 * it found, and what it cost.
 *
 * @typedef {{ critical: number, important: number, suggestion: number, tokens: Record<'in' | 'cw' | 'cr' | 'out', number | null> }} ResultRow
 * @typedef {{ arm: string, pinned: boolean, critical: number, important: number, suggestion: number, cost: number | null }} AspectReport
 * @typedef {{
 *   run: string, ts: number, repo: string, arms: Record<string, [string, string | null]>,
 *   results: Map<string, ResultRow & { arm: string }>,
 * }} Assign an assign row with the results kept for it, keyed by aspect
 */
import { readJsonl } from './logs.mjs'

/** Pinning is not retroactive, so a pinned aspect keeps a few rows from before it was pinned. */
const PINNED_SHARE = 0.95

/**
 * @param {string} path
 * @returns {{ assigns: Assign[], pinned: Set<string>, source: import('./transcripts.mjs').Source }}
 *   `pinned` holds the aspects whose most common arm string covers at least `PINNED_SHARE` of their kept results; results with no assign are dropped
 */
export function readReviewAb(path) {
  const { rows, source } = readJsonl(path)
  /** @type {Map<string, Assign>} */
  const byRun = new Map()
  for (const row of rows.filter((r) => r.kind === 'assign')) {
    byRun.set(row.run, { run: row.run, ts: row.ts, repo: row.repo, arms: row.arms, results: new Map() })
  }
  const results = rows.filter((r) => r.kind === 'result').sort((a, b) => a.ts - b.ts)
  /** @type {Map<string, Map<string, number>>} kept result count per arm string, by aspect */
  const armsByAspect = new Map()
  for (const row of results) {
    const assign = byRun.get(row.run)
    if (!assign || assign.results.has(row.aspect)) continue
    const arm = armString(assign.arms[row.aspect]) ?? row.arm
    assign.results.set(row.aspect, { ...row, arm })
    const counts = armsByAspect.get(row.aspect) ?? new Map()
    counts.set(arm, (counts.get(arm) ?? 0) + 1)
    armsByAspect.set(row.aspect, counts)
  }
  const pinned = new Set(
    [...armsByAspect]
      .filter(([, counts]) => Math.max(...counts.values()) >= PINNED_SHARE * [...counts.values()].reduce((a, b) => a + b, 0))
      .map(([aspect]) => aspect),
  )
  return { assigns: [...byRun.values()], pinned, source }
}

/**
 * The aspects of the branch's first review assign, with severities from that assign alone and cost
 * summed over every assign on the branch.
 *
 * @param {{ assigns: Assign[], pinned: Set<string> }} log
 * @param {import('./no-mistakes.mjs').Run[]} runs the branch's runs
 * @param {import('./no-mistakes.mjs').Run | null} firstRun
 * @returns {Record<string, AspectReport | null>} an aspect the first assign names but has no result for is null
 */
export function branchAspects(log, runs, firstRun) {
  const ids = new Set(runs.map((run) => run.id))
  const own = log.assigns.filter((assign) => ids.has(assign.repo))
  const first = own.filter((assign) => assign.repo === firstRun?.id).sort((a, b) => a.ts - b.ts)[0]
  if (!first) return {}
  const aspects = new Set([...Object.keys(first.arms), ...first.results.keys()])
  return Object.fromEntries(
    [...aspects].map((aspect) => {
      const result = first.results.get(aspect)
      if (!result) return [aspect, null]
      const { arm, critical, important, suggestion } = result
      const kept = own.flatMap((assign) => assign.results.get(aspect) ?? [])
      return [aspect, { arm, pinned: log.pinned.has(aspect), critical, important, suggestion, cost: cost(kept) }]
    }),
  )
}

/**
 * @param {[string, string | null] | undefined} pair
 * @returns {string | null} null when the assign does not name the aspect
 */
function armString(pair) {
  if (!pair) return null
  const [agent, model] = pair
  return model == null || model === '-' ? agent : `${agent}/${model}`
}

/**
 * Cost in input-token equivalents, `think` excluded because it is a subset of `out`.
 *
 * @param {ResultRow[]} results
 * @returns {number | null} null when every token field is null
 */
function cost(results) {
  const fields = results.flatMap(({ tokens }) => [tokens.in, tokens.cw, tokens.cr, tokens.out])
  if (fields.every((n) => n == null)) return null
  return results.reduce((total, { tokens }) => total + (tokens.in ?? 0) + (tokens.cw ?? 0) * 1.25 + (tokens.cr ?? 0) * 0.1 + (tokens.out ?? 0) * 5, 0)
}

/** A `path:line` or `path:start-end` reference, which a finding brackets without naming a section. */
const LOCATION = /^[^\s\]]+:\d+(-\d+)?$/

/** A leading bracket, after an optional severity word and its dash or colon. */
const LEADING_TAG = /^\s*(?:(?:critical|important|suggestion|error|warning|info)\s*[-–—:]\s*)?\[([^\]]*)\]/i

/**
 * How many findings each review section tagged, by the bracket that opens the description. A location
 * or a bracket containing `(` is code, not a tag, and a finding without a tag adds nothing.
 *
 * @param {import('./no-mistakes.mjs').Finding[]} findings
 * @returns {Record<string, number>}
 */
export function sectionCounts(findings) {
  /** @type {Record<string, number>} */
  const counts = {}
  for (const { description } of findings) {
    const tag = LEADING_TAG.exec(description)?.[1].trim()
    if (tag && !LOCATION.test(tag) && !tag.includes('(')) counts[tag] = (counts[tag] ?? 0) + 1
  }
  return counts
}
