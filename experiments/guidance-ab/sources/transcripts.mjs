/**
 * Reads Claude Code transcripts into segments: the run of lines in one session file that share a
 * `cwd` and `gitBranch`, since a session can move between branches.
 *
 * @typedef {{ input: number, cacheWrite: number, cacheRead: number, output: number }} Usage
 * @typedef {{
 *   sessionId: string, cwd: string, gitBranch: string, prRepository: string | null,
 *   firstAt: number, loadTimes: number[], usage: Usage,
 * }} Segment `prRepository` is the `owner/name`, lowercased, of the session's pr-link line; `firstAt`
 *   and `loadTimes` are epoch seconds, `loadTimes` one per coding-standards load; `usage` sums each message once
 *
 * @typedef {{ present: boolean, rows: number, skipped: number }} Source what one input yielded. For a JSONL
 *   log `rows` counts the lines parsed and `skipped` the lines that did not parse. For the projects dir
 *   `rows` counts the transcript files read and `skipped` the relevant lines and subagent meta files
 *   that did not parse
 * @typedef {{
 *   segments: Map<string, Segment>, seen: Set<string>, prRepository: string | null, source: Source,
 * }} SessionRead `seen` holds the message ids already counted across the session's files
 */
import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { basename, join } from 'node:path'

/** Review subagents load the standards to judge the change, not to write it. */
const REVIEW_PREFIX = 'rev-'

/** Marks the project directories of no-mistakes gate sessions, which are never the author's work. */
const GATE_DIRECTORY = '-no-mistakes-worktrees-'

/** Lines of any other type are never parsed, which keeps a multi-megabyte transcript cheap to read. */
const RELEVANT_LINE = /"assistant"|"pr-link"/

const STANDARDS_SKILLS = new Set(['coding-standards:coding-standards', 'coding-standards'])

/**
 * @param {string} projectsDir
 * @returns {{ segments: Segment[], source: Source }} no segments when the directory does not exist
 */
export function readSegments(projectsDir) {
  /** @type {Source} */
  const source = { present: existsSync(projectsDir), rows: 0, skipped: 0 }
  /** @type {Segment[]} */
  const segments = []
  if (!source.present) return { segments, source }
  for (const project of readdirSync(projectsDir, { withFileTypes: true })) {
    if (!project.isDirectory() || project.name.includes(GATE_DIRECTORY)) continue
    const dir = join(projectsDir, project.name)
    for (const file of readdirSync(dir)) {
      if (file.endsWith('.jsonl')) segments.push(...readSession(dir, basename(file, '.jsonl'), source))
    }
  }
  return { segments, source }
}

/**
 * Reads the session's own file, then its subagent files, counting each message once across all of them.
 * A subagent whose meta file does not parse is skipped, since its loads cannot be told apart from a
 * review subagent's.
 *
 * @param {string} dir the project directory
 * @param {string} sessionId
 * @param {Source} source tallies the files and lines read
 */
function readSession(dir, sessionId, source) {
  /** @type {SessionRead} */
  const session = { segments: new Map(), seen: new Set(), prRepository: null, source }
  readFile(join(dir, `${sessionId}.jsonl`), session, true)
  const subagentDir = join(dir, sessionId, 'subagents')
  if (existsSync(subagentDir)) {
    for (const file of readdirSync(subagentDir).sort()) {
      if (!file.endsWith('.jsonl')) continue
      const meta = join(subagentDir, file.replace(/\.jsonl$/, '.meta.json'))
      const parsed = existsSync(meta) ? parseLine(readFileSync(meta, 'utf8')) : {}
      if (parsed == null) {
        source.skipped += 1
        continue
      }
      readFile(join(subagentDir, file), session, !String(parsed.name).startsWith(REVIEW_PREFIX))
    }
  }
  const segments = [...session.segments.values()]
  for (const segment of segments) segment.prRepository = session.prRepository
  return segments
}

/**
 * @param {string} path
 * @param {SessionRead} session
 * @param {boolean} countsLoads whether a coding-standards load in this file counts as the author's
 */
function readFile(path, session, countsLoads) {
  session.source.rows += 1
  for (const text of readFileSync(path, 'utf8').split('\n')) {
    if (!RELEVANT_LINE.test(text)) continue
    const line = parseLine(text)
    if (!line) {
      session.source.skipped += 1
      continue
    }
    if (line.type === 'pr-link' && typeof line.prRepository === 'string') {
      session.prRepository = line.prRepository.toLowerCase()
    }
    if (typeof line.cwd !== 'string' || typeof line.gitBranch !== 'string') continue
    const key = `${line.cwd}\0${line.gitBranch}`
    const segment = session.segments.get(key) ?? {
      sessionId: String(line.sessionId),
      cwd: line.cwd,
      gitBranch: line.gitBranch,
      prRepository: null,
      firstAt: Infinity,
      loadTimes: [],
      usage: { input: 0, cacheWrite: 0, cacheRead: 0, output: 0 },
    }
    segment.firstAt = Math.min(segment.firstAt, Date.parse(line.timestamp) / 1000)
    if (countsLoads && loadsStandards(line)) segment.loadTimes.push(Date.parse(line.timestamp) / 1000)
    addUsage(segment.usage, line, session.seen)
    session.segments.set(key, segment)
  }
}

/**
 * @param {string} text
 * @returns {any} null when the text is not valid JSON
 */
function parseLine(text) {
  try {
    return JSON.parse(text)
  } catch {
    return null
  }
}

/** @param {any} line */
function loadsStandards(line) {
  if (line.type !== 'assistant' || !Array.isArray(line.message?.content)) return false
  return line.message.content.some(
    (/** @type {any} */ part) =>
      part.type === 'tool_use' && part.name === 'Skill' && STANDARDS_SKILLS.has(part.input?.skill),
  )
}

/**
 * Adds the line's message usage unless an earlier line already carried that message, since streaming
 * repeats one message's usage on every content block.
 *
 * @param {Usage} total
 * @param {any} line
 * @param {Set<string>} seen message ids already counted
 */
function addUsage(total, line, seen) {
  const message = line.message
  if (line.type !== 'assistant' || !message?.usage || seen.has(message.id)) return
  seen.add(message.id)
  total.input += message.usage.input_tokens ?? 0
  total.cacheWrite += message.usage.cache_creation_input_tokens ?? 0
  total.cacheRead += message.usage.cache_read_input_tokens ?? 0
  total.output += message.usage.output_tokens ?? 0
}
