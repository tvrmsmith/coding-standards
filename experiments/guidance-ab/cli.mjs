#!/usr/bin/env node
/**
 * Prints the guidance A/B report: `node cli.mjs <arm|history> [options]`, run from anywhere.
 */
import { homedir } from 'node:os'
import { join } from 'node:path'
import { parseArgs } from 'node:util'
import { analyze } from './analyze.mjs'
import { render } from './render.mjs'

const USAGE = `usage: node cli.mjs <arm|history> [--json] [--state-dir D] [--no-mistakes-home D] [--review-ab F] [--projects D] [--seed N]`

const OPTIONS = {
  json: { type: 'boolean' },
  'state-dir': { type: 'string' },
  'no-mistakes-home': { type: 'string' },
  'review-ab': { type: 'string' },
  projects: { type: 'string' },
  seed: { type: 'string' },
}

/**
 * @param {string} message
 * @param {number} code
 * @returns {never}
 */
function exitWith(message, code) {
  console.error(message)
  process.exit(code)
}

let parsed
try {
  parsed = parseArgs({ options: OPTIONS, allowPositionals: true })
} catch (error) {
  exitWith(`${error.message}\n${USAGE}`, 2)
}
const { values, positionals } = parsed
const [mode] = positionals
if (positionals.length !== 1 || (mode !== 'arm' && mode !== 'history')) exitWith(USAGE, 2)
const seed = values.seed === undefined ? undefined : Number(values.seed)
if (seed !== undefined && !Number.isInteger(seed)) exitWith(`--seed must be an integer\n${USAGE}`, 2)

const home = homedir()
const paths = {
  stateDir: values['state-dir'] ?? join(process.env.XDG_STATE_HOME ?? join(home, '.local', 'state'), 'coding-standards'),
  noMistakesHome: values['no-mistakes-home'] ?? join(home, '.no-mistakes'),
  reviewAb: values['review-ab'] ?? join(home, '.claude', 'review-ab.jsonl'),
  projectsDir: values.projects ?? join(home, '.claude', 'projects'),
}

try {
  const report = analyze({ mode, paths, seed })
  process.stdout.write(values.json ? `${JSON.stringify(report, null, 2)}\n` : render(report))
} catch (error) {
  exitWith(error.message, 1)
}
