/**
 * Reads what a branch head changed against its default branch, from whichever repo holds both.
 */
import { execFileSync } from 'node:child_process'
import { existsSync } from 'node:fs'

/**
 * @param {string[]} repos git dirs to try in order, bare or not; a path that does not exist is skipped
 * @param {string} head
 * @param {string} baseRef
 * @returns {string[] | null} paths changed since the merge base, or null when no repo has both commits
 */
export function changedFiles(repos, head, baseRef) {
  for (const repo of repos.filter((path) => existsSync(path))) {
    const base = gitOrNull(repo, 'merge-base', head, baseRef)
    if (base == null) continue
    const diff = gitOrNull(repo, 'diff', '--name-only', base, head)
    if (diff != null) return diff.split('\n').filter(Boolean)
  }
  return null
}

/**
 * Runs git, answering null when git exits non-zero, which here means the repo lacks a commit or ref.
 *
 * @param {string} repo
 * @param {...string} args
 */
function gitOrNull(repo, ...args) {
  try {
    return execFileSync('git', ['-C', repo, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
  } catch (error) {
    if (typeof (/** @type {{ status?: unknown }} */ (error).status) === 'number') return null
    throw error
  }
}
