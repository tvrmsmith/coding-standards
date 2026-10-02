/**
 * Reads the no-mistakes state db into branches, each carrying its runs oldest first.
 *
 * @typedef {{ id: string, severity: string, file: string, line: number, description: string }} Finding
 * @typedef {{ round: number, findings: Finding[] }} Round a round with `"findings": null` has none
 * @typedef {{ id: string, workingPath: string, upstreamUrl: string, defaultBranch: string }} Repo
 * @typedef {{
 *   step: string, round: number, purpose: string, workloadLines: number | null,
 *   input: number | null, output: number | null, cacheRead: number | null, cacheWrite: number | null,
 * }} Invocation one agent call; a column the db left NULL is null
 * @typedef {{
 *   id: string, status: string, createdAt: number, headSha: string, noMistakesVersion: string | null,
 *   parkedMs: number | null, completedAt: number | null,
 *   steps: Map<string, Round[]>, invocations: Invocation[],
 * }} Run `steps` maps a step name to its rounds in round order; `completedAt` is the latest
 *   `step_results.completed_at`, null when no step recorded one
 * @typedef {{ repo: Repo, branch: string, runs: Run[] }} Branch
 */
import { existsSync } from 'node:fs'
import { join } from 'node:path'
import { DatabaseSync } from 'node:sqlite'

/**
 * @param {string} noMistakesHome
 * @returns {Branch[]}
 * @throws {Error} naming the db path when it does not exist
 */
export function readBranches(noMistakesHome) {
  const dbPath = join(noMistakesHome, 'state.sqlite')
  if (!existsSync(dbPath)) throw new Error(`no-mistakes state db not found: ${dbPath}`)
  const db = new DatabaseSync(dbPath, { readOnly: true })
  try {
    return collect(db)
  } finally {
    db.close()
  }
}

/**
 * Every repo no-mistakes knows, including one with no runs.
 *
 * @param {string} noMistakesHome
 * @returns {Repo[]}
 * @throws {Error} naming the db path when it does not exist
 */
export function readRepos(noMistakesHome) {
  const dbPath = join(noMistakesHome, 'state.sqlite')
  if (!existsSync(dbPath)) throw new Error(`no-mistakes state db not found: ${dbPath}`)
  const db = new DatabaseSync(dbPath, { readOnly: true })
  try {
    return [...collectRepos(db).values()]
  } finally {
    db.close()
  }
}

/**
 * @param {DatabaseSync} db
 * @returns {Map<string, Repo>}
 */
function collectRepos(db) {
  /** @type {Map<string, Repo>} */
  const repos = new Map()
  for (const row of db.prepare('SELECT id, working_path, upstream_url, default_branch FROM repos').all()) {
    repos.set(String(row.id), {
      id: String(row.id),
      workingPath: String(row.working_path),
      upstreamUrl: String(row.upstream_url),
      defaultBranch: String(row.default_branch),
    })
  }
  return repos
}

/** @param {DatabaseSync} db */
function collect(db) {
  const repos = collectRepos(db)

  /** @type {Map<string, Branch>} */
  const branches = new Map()
  /** @type {Map<string, Run>} */
  const runs = new Map()
  const runRows = db
    .prepare(
      `SELECT id, repo_id, branch, head_sha, status, created_at, parked_ms, no_mistakes_version
       FROM runs ORDER BY created_at, id`,
    )
    .all()
  for (const row of runRows) {
    const key = `${row.repo_id}\0${row.branch}`
    if (!branches.has(key)) {
      const repo = repos.get(String(row.repo_id))
      if (!repo) throw new Error(`run ${row.id} names repo ${row.repo_id}, which the repos table lacks`)
      branches.set(key, { repo, branch: String(row.branch), runs: [] })
    }
    /** @type {Run} */
    const run = {
      id: String(row.id),
      status: String(row.status),
      createdAt: Number(row.created_at),
      headSha: String(row.head_sha),
      noMistakesVersion: row.no_mistakes_version == null ? null : String(row.no_mistakes_version),
      parkedMs: nullableNumber(row.parked_ms),
      completedAt: null,
      steps: new Map(),
      invocations: [],
    }
    runs.set(run.id, run)
    branches.get(key)?.runs.push(run)
  }

  const completedRows = db
    .prepare('SELECT run_id, MAX(completed_at) AS completed_at FROM step_results GROUP BY run_id')
    .all()
  for (const row of completedRows) {
    const run = runs.get(String(row.run_id))
    if (!run) throw new Error(`step result names run ${row.run_id}, which the runs table lacks`)
    run.completedAt = nullableNumber(row.completed_at)
  }

  const roundRows = db
    .prepare(
      `SELECT s.run_id, s.step_name, r.round, r.findings_json
       FROM step_rounds r JOIN step_results s ON s.id = r.step_result_id
       ORDER BY r.round`,
    )
    .all()
  for (const row of roundRows) {
    const run = runs.get(String(row.run_id))
    if (!run) throw new Error(`step round names run ${row.run_id}, which the runs table lacks`)
    const step = String(row.step_name)
    const rounds = run.steps.get(step) ?? []
    rounds.push({ round: Number(row.round), findings: parseFindings(row.findings_json, run.id, step) })
    run.steps.set(step, rounds)
  }

  const invocationRows = db
    .prepare(
      `SELECT run_id, step_name, round, purpose, workload_lines,
              input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens
       FROM agent_invocations ORDER BY started_at, id`,
    )
    .all()
  for (const row of invocationRows) {
    const run = runs.get(String(row.run_id))
    if (!run) throw new Error(`agent invocation names run ${row.run_id}, which the runs table lacks`)
    run.invocations.push({
      step: String(row.step_name),
      round: Number(row.round),
      purpose: String(row.purpose),
      workloadLines: nullableNumber(row.workload_lines),
      input: nullableNumber(row.input_tokens),
      output: nullableNumber(row.output_tokens),
      cacheRead: nullableNumber(row.cache_read_tokens),
      cacheWrite: nullableNumber(row.cache_creation_tokens),
    })
  }
  return [...branches.values()]
}

/** @param {unknown} value */
function nullableNumber(value) {
  return value == null ? null : Number(value)
}

/**
 * @param {unknown} json
 * @param {string} runId
 * @param {string} step
 * @returns {Finding[]}
 */
function parseFindings(json, runId, step) {
  if (json == null) return []
  try {
    return JSON.parse(String(json)).findings ?? []
  } catch (cause) {
    throw new Error(`run ${runId} step ${step} has unparseable findings_json`, { cause })
  }
}
