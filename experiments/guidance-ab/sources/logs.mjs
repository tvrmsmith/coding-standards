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
 * Lint metrics for one branch from its lint-changed lines. lint-changed writes one line per language
 * per run, every line of a run sharing `ts` and `head`, and no line when nothing lintable ran. Gate
 * lines are ignored. `lintPreCommit` sums the findings over the lines of the earliest run, and
 * `blockedCommits` counts the runs that blocked. A branch with no run scores 0 on both when its work
 * began at or after the log's first line, and null when it began earlier or there is no log.
 *
 * @param {LintRow[]} rows the branch's lines
 * @param {{ began: number, logStart: number }} span `began` is when the branch's work began and
 *   `logStart` the smallest `ts` in the log, gate lines included, or Infinity with no log
 * @returns {{ lintPreCommit: number | null, blockedCommits: number | null }}
 */
export function lintMetrics(rows, { began, logStart }) {
  /** @type {Map<string, LintRow[]>} */
  const runs = new Map()
  for (const row of rows.filter((r) => !r.gate).sort((a, b) => a.ts - b.ts)) {
    const key = `${row.ts}\0${row.head}`
    runs.set(key, [...(runs.get(key) ?? []), row])
  }
  if (!runs.size) {
    const none = began >= logStart ? 0 : null
    return { lintPreCommit: none, blockedCommits: none }
  }
  const [earliest] = runs.values()
  return {
    lintPreCommit: earliest.flatMap(({ findings }) => Object.values(findings)).reduce((total, count) => total + count, 0),
    blockedCommits: [...runs.values()].filter((lines) => lines.some((row) => row.blocked)).length,
  }
}
