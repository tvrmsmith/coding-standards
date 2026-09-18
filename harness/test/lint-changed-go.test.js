/**
 * The registry lookup in lint-changed-go.sh, which is the one piece of the Go half with no
 * other consumer: the hook calls it, the hook is silent when it skips, and a skip that should
 * have been a run looks exactly like a repo with no findings.
 *
 * The linter itself is stubbed. What is under test is which repository path the script decides
 * to look up, not what golangci-lint says about the file, and stubbing it keeps the case at a
 * git fixture and a text file instead of a real Go toolchain.
 */
import { execFileSync } from 'node:child_process'
import { chmodSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import assert from 'node:assert/strict'

const harness = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..'))
const script = join(harness, 'lint-changed-go.sh')

function git(cwd, ...args) {
  return execFileSync('git', args, { cwd, encoding: 'utf8' })
}

/**
 * A repository with one committed .go file, a linked worktree, a stub linter and an empty
 * registry. Returns the paths the cases need, and a cleanup.
 */
function fixture() {
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'tvrmsmith-go-'))
  const repo = join(root, 'repo')
  const worktree = join(root, 'worktree')
  const registry = join(root, 'registry')
  const stub = join(root, 'stub-gcl')

  git(root, 'init', '--quiet', '--initial-branch=main', repo)
  git(repo, 'config', 'user.email', 'test@example.com')
  git(repo, 'config', 'user.name', 'Test')
  git(repo, 'config', 'commit.gpgsign', 'false')
  writeFileSync(join(repo, 'go.mod'), 'module example.test\n\ngo 1.26.0\n')
  writeFileSync(join(repo, 'main.go'), 'package main\n\nfunc main() {}\n')
  git(repo, 'add', '.')
  git(repo, 'commit', '--quiet', '-m', 'initial')
  git(repo, 'worktree', 'add', '--quiet', '-b', 'side', worktree)

  // Prints one finding in golangci-lint's text shape. Absolute, because the script runs the
  // linter with --path-mode abs and filters the output by absolute path, and it is run from
  // inside the module, so $PWD is the checkout under test.
  writeFileSync(stub, '#!/bin/sh\necho "$PWD/main.go:1:1: stub finding (stub)"\n')
  chmodSync(stub, 0o755)
  writeFileSync(registry, '')

  return { root, repo, worktree, registry, stub, cleanup: () => rmSync(root, { recursive: true, force: true }) }
}

/** @returns {{ status: number, output: string }} */
function lint(cwd, { registry, stub }, env = {}) {
  const result = execFileSync(script, ['--since', 'HEAD'], {
    cwd,
    encoding: 'utf8',
    env: { ...process.env, TVRMSMITH_GO_REPOS: registry, TVRMSMITH_GCL: stub, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  return result
}

test('an unregistered repository is skipped rather than linted', () => {
  const f = fixture()
  try {
    assert.doesNotMatch(lint(f.repo, f), /stub finding/)
  } finally {
    f.cleanup()
  }
})

test('a registered repository is linted', () => {
  const f = fixture()
  try {
    writeFileSync(f.registry, `${f.repo}\n`)
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')
    assert.match(lint(f.repo, f), /stub finding/)
  } finally {
    f.cleanup()
  }
})

test('a linked worktree is covered by the main checkout being registered', () => {
  const f = fixture()
  try {
    // Only the main checkout is listed, which is what `bootstrap go` writes even when it is
    // pointed at a worktree. The hook is shared between them, so keying on the worktree's own
    // path would make it fire and skip.
    writeFileSync(f.registry, `${f.repo}\n`)
    writeFileSync(join(f.worktree, 'main.go'), 'package main\n\nfunc main() { _ = 2 }\n')
    assert.match(lint(f.worktree, f), /stub finding/)
  } finally {
    f.cleanup()
  }
})

test('a worktree of an unregistered repository is still skipped', () => {
  const f = fixture()
  try {
    writeFileSync(join(f.worktree, 'main.go'), 'package main\n\nfunc main() { _ = 3 }\n')
    assert.doesNotMatch(lint(f.worktree, f), /stub finding/)
  } finally {
    f.cleanup()
  }
})

test('TVRMSMITH_REGISTRY_KEY names the adopted checkout the lookup cannot reach', () => {
  const f = fixture()
  try {
    // The no-mistakes shape. Its pipeline lints in a worktree of a bare repository it keeps
    // under ~/.no-mistakes, so the common git dir resolves to that bare repo rather than to the
    // checkout the user adopted, and the derived key is never in the registry. The clone here
    // stands in for it: its own path is unregistered and it shares no git dir with f.repo.
    const elsewhere = join(f.root, 'elsewhere')
    git(f.root, 'clone', '--quiet', f.repo, elsewhere)
    writeFileSync(f.registry, `${f.repo}\n`)
    writeFileSync(join(elsewhere, 'main.go'), 'package main\n\nfunc main() { _ = 4 }\n')

    assert.doesNotMatch(lint(elsewhere, f), /stub finding/)
    assert.match(lint(elsewhere, f, { TVRMSMITH_REGISTRY_KEY: f.repo }), /stub finding/)
  } finally {
    f.cleanup()
  }
})

test('TVRMSMITH_REGISTRY_KEY answers the adoption question, it does not bypass it', () => {
  const f = fixture()
  try {
    // An override naming a path nobody registered still skips. The variable moves which path is
    // looked up; it is not a way to lint a repository that was never adopted.
    writeFileSync(f.registry, `${f.repo}\n`)
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 5 }\n')
    assert.doesNotMatch(lint(f.repo, f, { TVRMSMITH_REGISTRY_KEY: join(f.root, 'never-adopted') }), /stub finding/)
  } finally {
    f.cleanup()
  }
})
