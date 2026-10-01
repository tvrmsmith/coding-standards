/**
 * Builds a throwaway world of the machine-local sources `analyze` reads, from a compact
 * description, and returns the `paths` object `analyze` takes.
 *
 * Every relative path in the description (a repo's `workingPath`, a session's `cwd`) resolves
 * against the world's root, so a test can write `checkouts/widgets` and match it on both sides.
 *
 * @typedef {{ id: string, severity: string, file?: string, line?: number, description?: string }} Finding
 * @typedef {Finding[] | null} Round one step round's findings; null writes `"findings": null`
 * @typedef {{
 *   step?: string, round?: number, purpose?: string, workloadLines?: number,
 *   input?: number, output?: number, cacheRead?: number, cacheWrite?: number,
 * }} Invocation an `agent_invocations` row; an omitted token count is NULL
 * @typedef {{
 *   id: string, branch: string, status?: string, createdAt: number, version?: string,
 *   steps?: Record<string, Round[]>, invocations?: Invocation[],
 *   parkedMs?: number, completedAt?: Record<string, number>,
 * }} Run `steps` maps a step name to its rounds, round 1 first; the head is the branch's head commit.
 *   `completedAt` maps a step name to its `completed_at` epoch seconds, which defaults to `createdAt`;
 *   an omitted `parkedMs` is NULL
 * @typedef {{ files: Record<string, number>, inBare?: boolean }} Branch one commit off the default
 *   branch appending the given line count to each file; `inBare: false` leaves it out of the bare repo
 * @typedef {{
 *   id: string, upstream: string, defaultBranch?: string, workingPath: string,
 *   base?: Record<string, number>, branches?: Record<string, Branch>, checkout?: boolean,
 *   runs?: Run[],
 * }} Repo `checkout: true` also clones every branch into a non-bare repo at `workingPath`
 * @typedef {{ input?: number, output?: number, cacheRead?: number, cacheWrite?: number }} Usage
 * @typedef {{
 *   at?: string, cwd?: string, branch?: string, messageId?: string, usage?: Usage,
 *   skills?: string[], repeat?: number, raw?: string, stringMessage?: string,
 * }} Entry assistant line(s) of a transcript. `at` is an ISO timestamp, `cwd` and `branch` default to
 *   the session's, `usage` defaults to one input and one output token, and `repeat` writes the line
 *   that many times, as streaming does for one message. `raw` writes its text verbatim instead,
 *   and `stringMessage` writes a line whose `message` is a string
 * @typedef {{ id: string, name: string, entries: Entry[], meta?: string }} Subagent a subagent file under
 *   the session, with a `.meta.json` carrying `name`; `meta` writes its text verbatim as the meta file instead
 * @typedef {{
 *   id: string, cwd: string, branch: string, skills?: string[], entries?: Entry[],
 *   subagents?: Subagent[], prRepository?: string, dir?: string,
 * }} Session one transcript. Without `entries` it is a single assistant line that calls the Skill tool
 *   once per entry in `skills`. `prRepository` adds a `pr-link` line, and `dir` names the project
 *   directory instead of the one derived from `cwd`
 * @typedef {Record<string, unknown> | string} LogRow one line of a state-dir log; a string is written verbatim
 * @typedef {{
 *   repos?: Repo[], sessions?: Session[], db?: boolean, arms?: LogRow[], lint?: LogRow[], reviewAb?: LogRow[],
 * }} World `db: false` writes no state.sqlite; `arms` and `lint` write `arms.jsonl` and
 *   `lint-runs.jsonl` into the state dir, `reviewAb` writes `review-ab.jsonl` outside it, and an
 *   omitted list writes no file
 */
import { execFileSync } from 'node:child_process'
import { appendFileSync, mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, isAbsolute, join } from 'node:path'
import { DatabaseSync } from 'node:sqlite'

/** Column names and types as the real no-mistakes schema spells them, trimmed to what analyze reads. */
const SCHEMA = `
CREATE TABLE repos (
  id TEXT PRIMARY KEY, working_path TEXT NOT NULL UNIQUE, upstream_url TEXT NOT NULL,
  default_branch TEXT NOT NULL DEFAULT 'main', created_at INTEGER NOT NULL);
CREATE TABLE runs (
  id TEXT PRIMARY KEY, repo_id TEXT NOT NULL REFERENCES repos(id), branch TEXT NOT NULL,
  head_sha TEXT NOT NULL, base_sha TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, parked_ms INTEGER, no_mistakes_version TEXT);
CREATE TABLE step_results (
  id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES runs(id), step_name TEXT NOT NULL,
  step_order INTEGER NOT NULL, status TEXT NOT NULL DEFAULT 'pending', findings_json TEXT,
  started_at INTEGER, completed_at INTEGER);
CREATE TABLE step_rounds (
  id TEXT PRIMARY KEY, step_result_id TEXT NOT NULL REFERENCES step_results(id), round INTEGER NOT NULL,
  trigger_type TEXT NOT NULL, findings_json TEXT, duration_ms INTEGER NOT NULL, created_at INTEGER NOT NULL);
CREATE TABLE agent_invocations (
  id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES runs(id), step_name TEXT NOT NULL,
  round INTEGER NOT NULL, purpose TEXT NOT NULL, agent TEXT NOT NULL, session_mode TEXT NOT NULL,
  started_at INTEGER NOT NULL, completed_at INTEGER NOT NULL, duration_ms INTEGER NOT NULL,
  exit_status TEXT NOT NULL, input_tokens INTEGER, output_tokens INTEGER, cache_read_tokens INTEGER,
  cache_creation_tokens INTEGER, workload_lines INTEGER);
`

/**
 * Isolates fixture commits from the machine's git config: a CI runner has no identity, and a
 * workstation may demand commit signing.
 */
const GIT_ENV = {
  ...process.env,
  GIT_CONFIG_GLOBAL: '/dev/null',
  GIT_CONFIG_NOSYSTEM: '1',
  GIT_CONFIG_COUNT: '1',
  GIT_CONFIG_KEY_0: 'commit.gpgsign',
  GIT_CONFIG_VALUE_0: 'false',
  GIT_AUTHOR_NAME: 'Fixture',
  GIT_AUTHOR_EMAIL: 'fixture@example.com',
  GIT_AUTHOR_DATE: '2026-09-01T00:00:00Z',
  GIT_COMMITTER_NAME: 'Fixture',
  GIT_COMMITTER_EMAIL: 'fixture@example.com',
  GIT_COMMITTER_DATE: '2026-09-01T00:00:00Z',
}

/**
 * @param {import('node:test').TestContext} t removes the world after the test
 * @param {World} world
 */
export function buildWorld(t, { repos = [], sessions = [], db = true, arms, lint, reviewAb }) {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'guidance-ab-')))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const at = (/** @type {string} */ path) => (isAbsolute(path) ? path : join(root, path))
  const paths = {
    stateDir: join(root, 'state'),
    noMistakesHome: join(root, 'no-mistakes'),
    reviewAb: join(root, 'review-ab.jsonl'),
    projectsDir: join(root, 'projects'),
  }
  mkdirSync(join(paths.noMistakesHome, 'repos'), { recursive: true })

  const heads = new Map(repos.map((repo) => [repo.id, buildGit(root, paths.noMistakesHome, repo, at)]))
  if (db) writeDb(join(paths.noMistakesHome, 'state.sqlite'), repos, heads, at)
  for (const session of sessions) writeSession(paths.projectsDir, session, at)
  if (arms) writeLog(join(paths.stateDir, 'arms.jsonl'), arms)
  if (lint) writeLog(join(paths.stateDir, 'lint-runs.jsonl'), lint)
  if (reviewAb) writeLog(paths.reviewAb, reviewAb)
  return paths
}

/**
 * @param {string} path
 * @param {LogRow[]} rows
 */
function writeLog(path, rows) {
  mkdirSync(dirname(path), { recursive: true })
  writeFileSync(path, `${rows.map((row) => (typeof row === 'string' ? row : JSON.stringify(row))).join('\n')}\n`)
}

/**
 * Commits the repo's history in a scratch repo, then fetches it into the bare repo and the checkout.
 *
 * @param {string} root
 * @param {string} noMistakesHome
 * @param {Repo} repo
 * @param {(path: string) => string} at
 * @returns {{ base: string, branches: Map<string, string> }} commit shas
 */
function buildGit(root, noMistakesHome, repo, at) {
  const { defaultBranch = 'main', base = { 'README.md': 1 }, branches = {} } = repo
  const scratch = join(root, 'scratch', repo.id)
  git(root, 'init', '-q', '-b', defaultBranch, scratch)
  commit(scratch, base, 'base')
  const baseSha = git(scratch, 'rev-parse', 'HEAD')
  const shas = new Map()
  for (const [name, branch] of Object.entries(branches)) {
    git(scratch, 'checkout', '-q', '-b', name, defaultBranch)
    commit(scratch, branch.files, name)
    shas.set(name, git(scratch, 'rev-parse', 'HEAD'))
    git(scratch, 'checkout', '-q', defaultBranch)
  }

  const baseRef = `${defaultBranch}:refs/remotes/origin/${defaultBranch}`
  const bare = join(noMistakesHome, 'repos', `${repo.id}.git`)
  git(root, 'init', '-q', '--bare', '-b', defaultBranch, bare)
  const bareHeads = Object.entries(branches).filter(([, branch]) => branch.inBare !== false)
  git(bare, 'fetch', '-q', scratch, baseRef, ...bareHeads.map(([name]) => `${name}:refs/heads/${name}`))
  if (repo.checkout) {
    const checkout = at(repo.workingPath)
    git(root, 'init', '-q', '-b', defaultBranch, checkout)
    git(checkout, 'fetch', '-q', scratch, baseRef, ...Object.keys(branches).map((name) => `${name}:refs/heads/${name}`))
  }
  return { base: baseSha, branches: shas }
}

/**
 * @param {string} dir
 * @param {Record<string, number>} files line counts to append, creating a file that does not exist
 * @param {string} message
 */
function commit(dir, files, message) {
  for (const [file, lines] of Object.entries(files)) {
    const path = join(dir, file)
    mkdirSync(dirname(path), { recursive: true })
    appendFileSync(path, Array.from({ length: lines }, (_, i) => `${message} line ${i + 1}\n`).join(''))
  }
  git(dir, 'add', '-A')
  git(dir, 'commit', '-q', '-m', message)
}

/**
 * @param {string} cwd
 * @param {...string} args
 */
function git(cwd, ...args) {
  return execFileSync('git', args, { cwd, env: GIT_ENV, encoding: 'utf8' }).trim()
}

/**
 * @param {string} dbPath
 * @param {Repo[]} repos
 * @param {Map<string, { base: string, branches: Map<string, string> }>} heads
 * @param {(path: string) => string} at
 */
function writeDb(dbPath, repos, heads, at) {
  const db = new DatabaseSync(dbPath)
  db.exec(SCHEMA)
  const insertRepo = db.prepare('INSERT INTO repos VALUES (?, ?, ?, ?, 0)')
  const insertRun = db.prepare('INSERT INTO runs VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)')
  const insertStep = db.prepare('INSERT INTO step_results VALUES (?, ?, ?, ?, ?, ?, ?, ?)')
  const insertRound = db.prepare('INSERT INTO step_rounds VALUES (?, ?, ?, ?, ?, 0, ?)')
  const insertInvocation = db.prepare(
    `INSERT INTO agent_invocations VALUES (?, ?, ?, ?, ?, 'claude', 'fresh', ?, ?, 0, 'ok', ?, ?, ?, ?, ?)`,
  )
  for (const repo of repos) {
    const { defaultBranch = 'main', runs = [] } = repo
    const sha = /** @type {{ base: string, branches: Map<string, string> }} */ (heads.get(repo.id))
    insertRepo.run(repo.id, at(repo.workingPath), repo.upstream, defaultBranch)
    for (const run of runs) {
      const { status = 'completed', createdAt, version = null, steps = {}, invocations = [] } = run
      const head = sha.branches.get(run.branch)
      if (!head) throw new Error(`run ${run.id} names branch ${run.branch}, which repo ${repo.id} does not declare`)
      insertRun.run(run.id, repo.id, run.branch, head, sha.base, status, createdAt, createdAt, run.parkedMs ?? null, version)
      Object.entries(steps).forEach(([step, rounds], order) => {
        const stepId = `${run.id}-${step}`
        const last = rounds.length ? findingsJson(rounds[rounds.length - 1]) : null
        insertStep.run(stepId, run.id, step, order + 1, 'completed', last, createdAt, run.completedAt?.[step] ?? createdAt)
        rounds.forEach((findings, i) => {
          const trigger = i === 0 ? 'initial' : 'auto_fix'
          insertRound.run(`${stepId}-${i + 1}`, stepId, i + 1, trigger, findingsJson(findings), createdAt + i)
        })
      })
      invocations.forEach((inv, i) => {
        const { step = 'review', round = 1, purpose = 'review', workloadLines = null } = inv
        const { input = null, output = null, cacheRead = null, cacheWrite = null } = inv
        const id = `${run.id}-invocation-${i + 1}`
        insertInvocation.run(id, run.id, step, round, purpose, createdAt, createdAt, input, output, cacheRead, cacheWrite, workloadLines)
      })
    }
  }
  db.close()
}

/** @param {Round} findings */
function findingsJson(findings) {
  const full = findings?.map(({ id, severity, file = 'src/app.ts', line = 1, description = id }) => ({
    id,
    severity,
    file,
    line,
    description,
  }))
  return JSON.stringify({ findings: full ?? null, summary: 'fixture round' })
}

/** Before every fixture run's `createdAt`, so a default line counts as written before the first run. */
const DEFAULT_AT = '1970-01-01T00:00:00.000Z'

/**
 * Writes the transcript under the directory name Claude Code derives from the session's cwd.
 *
 * @param {string} projectsDir
 * @param {Session} session
 * @param {(path: string) => string} at
 */
function writeSession(projectsDir, session, at) {
  const { id, cwd, branch, skills = [], subagents = [], prRepository, dir = at(cwd).replace(/[^A-Za-z0-9]/g, '-') } = session
  const sessionDir = join(projectsDir, dir)
  mkdirSync(sessionDir, { recursive: true })
  const entries = session.entries ?? [{ messageId: `msg_${id}`, skills }]
  const lines = entries.flatMap((entry) => entryLines(session, entry, false, at))
  if (prRepository) {
    const prLink = { type: 'pr-link', sessionId: id, prNumber: 1, prUrl: `https://example.com/${prRepository}/pull/1`, prRepository, timestamp: DEFAULT_AT }
    lines.push(JSON.stringify(prLink))
  }
  writeFileSync(join(sessionDir, `${id}.jsonl`), `${lines.join('\n')}\n`)
  for (const subagent of subagents) {
    const agentDir = join(sessionDir, id, 'subagents')
    mkdirSync(agentDir, { recursive: true })
    const sidechain = subagent.entries.flatMap((entry) => entryLines(session, entry, true, at))
    writeFileSync(join(agentDir, `agent-${subagent.id}.jsonl`), `${sidechain.join('\n')}\n`)
    const meta = subagent.meta ?? JSON.stringify({ agentType: 'general-purpose', description: 'fixture', name: subagent.name, toolUseId: 'toolu_0', spawnDepth: 1 })
    writeFileSync(join(agentDir, `agent-${subagent.id}.meta.json`), meta)
  }
}

/**
 * @param {Session} session
 * @param {Entry} entry
 * @param {boolean} sidechain
 * @param {(path: string) => string} at
 * @returns {string[]}
 */
function entryLines(session, entry, sidechain, at) {
  if (entry.raw != null) return [entry.raw]
  const { skills = [], usage = {}, repeat = 1 } = entry
  const { input = 1, output = 1, cacheRead = 0, cacheWrite = 0 } = usage
  const content = skills.length
    ? skills.map((skill, i) => ({ type: 'tool_use', id: `toolu_${i + 1}`, name: 'Skill', input: { skill } }))
    : [{ type: 'text', text: 'Working on it.' }]
  const message = entry.stringMessage ?? {
    id: entry.messageId ?? `msg_${session.id}`,
    role: 'assistant',
    content,
    usage: { input_tokens: input, output_tokens: output, cache_read_input_tokens: cacheRead, cache_creation_input_tokens: cacheWrite },
  }
  const line = JSON.stringify({
    type: 'assistant',
    sessionId: session.id,
    ...(sidechain && { isSidechain: true }),
    cwd: at(entry.cwd ?? session.cwd),
    gitBranch: entry.branch ?? session.branch,
    timestamp: entry.at ?? DEFAULT_AT,
    message,
  })
  return Array.from({ length: repeat }, () => line)
}

/** The upstream of the arm world's one repo, whose branches hash to the arms `ARM_WORLD_ARMS` names. */
export const ARM_WORLD_UPSTREAM = 'git@github-personal:Acme/Widgets.git'

/**
 * Each branch of the arm world with its arm, which is the first hex digit of the sha1 of
 * `<upstream>#<branch>`: `0-7` is guidance, `8-f` review-only.
 */
const ARM_WORLD_ARMS = { 'feat-a': 'review-only', 'feat-b': 'guidance', 'feat-c': 'review-only', 'feat-e': 'guidance' }

/**
 * The description of a world with one arm-log row per branch at 1000, a first run at 2000 with a code
 * change, and a session that loads the skill at 1500. A branch's first review round carries as many
 * errors as its arm's entry in `errors`.
 *
 * @param {{
 *   errors?: Record<'guidance' | 'review-only', number>,
 *   run?: (branch: string) => Partial<Run>,
 *   withoutSession?: string[],
 *   lint?: LogRow[], reviewAb?: LogRow[],
 * }} [options] `run` overrides fields of a branch's run, and `withoutSession` names branches left with no transcript
 * @returns {World}
 */
export function armSummaryWorld({ errors = { guidance: 3, 'review-only': 1 }, run = () => ({}), withoutSession = [], lint, reviewAb } = {}) {
  const names = Object.keys(ARM_WORLD_ARMS)
  const findings = (/** @type {number} */ count) => Array.from({ length: count }, (_, i) => ({ id: `error-${i + 1}`, severity: 'error' }))
  return {
    repos: [
      {
        id: 'r1',
        upstream: ARM_WORLD_UPSTREAM,
        workingPath: 'checkouts/widgets',
        branches: Object.fromEntries(names.map((name) => [name, { files: { 'src/app.ts': 3 } }])),
        runs: names.map((name) => ({
          id: `run-${name}`,
          branch: name,
          createdAt: 2000,
          version: '1.2.3',
          steps: { review: [findings(errors[ARM_WORLD_ARMS[name]])] },
          ...run(name),
        })),
      },
    ],
    sessions: names
      .filter((name) => !withoutSession.includes(name))
      .map((name) => ({
        id: `s-${name}`,
        cwd: 'checkouts/widgets',
        branch: name,
        entries: [{ at: '1970-01-01T00:25:00.000Z', skills: ['coding-standards:coding-standards'] }],
      })),
    arms: names.map((name) => ({ ts: 1000, session_id: `s-${name}`, repo: ARM_WORLD_UPSTREAM, branch: name, arm: ARM_WORLD_ARMS[name] })),
    lint,
    reviewAb,
  }
}
