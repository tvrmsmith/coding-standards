/**
 * Reads the JSONL logs in the state directory: the arm log and the lint log.
 *
 * @typedef {import('./transcripts.mjs').Source} Source
 * @typedef {{
 *   ts: number, repo: string, branch: string, head: string, lang: 'ts' | 'go' | 'dotnet',
 *   mode: 'staged' | 'since' | 'files', gate: boolean, blocked: boolean, findings: Record<string, number>,
 * }} LintRow `findings` maps a rule id to its count, empty for a clean run
 */
import { existsSync, readFileSync } from 'node:fs'

/**
 * @param {string} path
 * @returns {{ rows: any[], source: Source }} no rows when the file does not exist; a line that is not
 *   valid JSON is skipped and counted
 */
export function readJsonl(path) {
  /** @type {Source} */
  const source = { present: existsSync(path), rows: 0, skipped: 0 }
  /** @type {any[]} */
  const rows = []
  if (!source.present) return { rows, source }
  for (const text of readFileSync(path, 'utf8').split('\n')) {
    if (!text.trim()) continue
    try {
      rows.push(JSON.parse(text))
    } catch {
      source.skipped += 1
    }
  }
  source.rows = rows.length
  return { rows, source }
}

/**
 * Lint metrics for one branch from its lint-changed runs, or nulls when it has no non-gate run.
 * `preCommit` sums, over each language, the findings of that language's earliest run; `blocked` counts
 * the runs that blocked a commit. Gate runs are ignored.
 *
 * @param {LintRow[]} rows the branch's runs
 * @returns {{ lintPreCommit: number | null, blockedCommits: number | null }}
 */
export function lintMetrics(rows) {
  const runs = rows.filter((row) => !row.gate).sort((a, b) => a.ts - b.ts)
  if (!runs.length) return { lintPreCommit: null, blockedCommits: null }
  /** @type {Map<string, number>} */
  const earliest = new Map()
  for (const { lang, findings } of runs) {
    if (!earliest.has(lang)) earliest.set(lang, Object.values(findings).reduce((total, count) => total + count, 0))
  }
  return {
    lintPreCommit: [...earliest.values()].reduce((total, count) => total + count, 0),
    blockedCommits: runs.filter((row) => row.blocked).length,
  }
}
