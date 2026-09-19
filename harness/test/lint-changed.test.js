/**
 * The lint-changed.sh entrypoint: which branches it runs, and what each one puts on stdout.
 *
 * The thing worth pinning is that one invocation covers every language the repo is wired for,
 * since a caller having to name the branches is what this script exists to remove. Each branch's
 * output is its linter's own, so the cases here check that it arrives intact rather than
 * re-testing the linter.
 *
 * TypeScript and Go are stubbed: what is under test is the dispatch, not what ESLint or
 * golangci-lint think of a file. C# is not stubbable the same way — its findings come from a
 * Roslyn SARIF log — so lint-changed-dotnet.test.js covers that branch against a real dotnet.
 */
import { execFileSync, spawnSync } from 'node:child_process'
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test, { describe } from 'node:test'
import assert from 'node:assert/strict'

const harness = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..'))

function git(cwd, ...args) {
  return execFileSync('git', args, { cwd, encoding: 'utf8' })
}

/** A repository with one commit, ready for `--since HEAD` to see a worktree edit. */
function repository(prefix) {
  const root = mkdtempSync(join(realpathSync(tmpdir()), prefix))
  const repo = join(root, 'repo')
  mkdirSync(repo)
  git(repo, 'init', '--quiet', '--initial-branch=main')
  git(repo, 'config', 'user.email', 'test@example.com')
  git(repo, 'config', 'user.name', 'Test')
  git(repo, 'config', 'commit.gpgsign', 'false')
  return { root, repo, cleanup: () => rmSync(root, { recursive: true, force: true }) }
}

function executable(path, body) {
  mkdirSync(dirname(path), { recursive: true })
  writeFileSync(path, body)
  chmodSync(path, 0o755)
}

function commitAll(repo) {
  git(repo, 'add', '-A')
  git(repo, 'commit', '--quiet', '-m', 'initial')
}

/**
 * The harness variables every case has to start from a known state: each is honoured whenever it
 * is set, so an ambient value on the developer's machine would decide adoption or which binary
 * runs for any case that does not override it, and a negative case would pass for the wrong
 * reason. `undefined` removes the variable from the child's environment.
 */
const neutralised = {
  TVRMSMITH_REGISTRY_KEY: undefined,
  TVRMSMITH_ESLINT_LAYER: undefined,
  TVRMSMITH_GCL: undefined,
  TVRMSMITH_GO_REPOS: undefined,
  TVRMSMITH_GOLANGCI_CONFIG: undefined,
  TVRMSMITH_ANALYZER_PROPS: undefined,
  TVRMSMITH_ANALYZER_LOCAL_PROPS: undefined,
}

/**
 * Both streams and the status. stderr is where every diagnostic goes, and exit 1 is the code the
 * script shares between a bad argument, an unresolvable ref and a branch that could not run, so a
 * case asserting the status alone would pass on a failure it never meant to provoke.
 *
 * @returns {{ status: number, stdout: string, stderr: string }}
 */
function capture(cwd, args, env) {
  const result = spawnSync(join(harness, 'lint-changed.sh'), args, {
    cwd,
    encoding: 'utf8',
    env: { ...process.env, ...neutralised, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  if (result.error) throw result.error
  return { status: result.status, stdout: result.stdout, stderr: result.stderr }
}

/** @returns {string} stdout only, so a case can assert on what the flag promises to emit. */
function run(cwd, args, env) {
  const { status, stdout, stderr } = capture(cwd, args, env)
  assert.equal(status, 0, `lint-changed.sh exited ${status}\n${stderr}`)
  return stdout
}

describe('the TypeScript branch', () => {
  /** A package whose ESLint is a stub printing one finding in the stylish shape. */
  function fixture() {
    const f = repository('tvrmsmith-lint-ts-')
    writeFileSync(join(f.repo, 'eslint.config.js'), 'export default []\n')
    writeFileSync(join(f.repo, 'main.ts'), 'export const a = 1\n')
    executable(
      join(f.repo, 'node_modules/.bin/eslint'),
      [
        '#!/usr/bin/env node',
        "console.log(`${process.cwd()}/main.ts`)",
        "console.log('  1:7  warning  Unexpected thing.  tvrmsmith/no-thing')",
      ].join('\n') + '\n',
    )
    commitAll(f.repo)
    writeFileSync(join(f.repo, 'main.ts'), 'export const a = 2\n')
    return f
  }

  test('the default output is the untouched stylish report under its package header', () => {
    const f = fixture()
    try {
      const out = run(f.repo, ['--only', 'ts', '--since', 'HEAD'])
      assert.equal(out, `=== . ===\n${f.repo}/main.ts\n  1:7  warning  Unexpected thing.  tvrmsmith/no-thing\n`)
    } finally {
      f.cleanup()
    }
  })

  test('a changed path holding a space reaches the linter as one argument', () => {
    const f = fixture()
    try {
      // Split on the space, ESLint is handed two paths that do not exist and fails, so the commit
      // is blocked over a file nobody wrote. The stub prints its argv so the case can see which
      // it got.
      executable(
        join(f.repo, 'node_modules/.bin/eslint'),
        '#!/usr/bin/env node\nfor (const a of process.argv.slice(2)) console.log(`arg: ${a}`)\n',
      )
      writeFileSync(join(f.repo, 'my file.ts'), 'export const b = 1\n')
      // Staged, because `git diff HEAD` reports a new file only once the index carries it.
      git(f.repo, 'add', 'my file.ts')
      const out = run(f.repo, ['--only', 'ts', '--since', 'HEAD'])
      assert.match(out, /^arg: my file\.ts$/m)
      assert.doesNotMatch(out, /^arg: my$/m)
    } finally {
      f.cleanup()
    }
  })
})

describe('the Go branch', () => {
  /** A module whose golangci-lint is a stub printing one finding in the text shape. */
  function fixture() {
    const f = repository('tvrmsmith-lint-go-')
    f.registry = join(f.root, 'registry')
    f.gcl = join(f.root, 'stub-gcl')
    writeFileSync(join(f.repo, 'go.mod'), 'module example.test\n\ngo 1.26.0\n')
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() {}\n')
    commitAll(f.repo)
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')
    // Absolute, because the script runs the linter with --path-mode abs and filters by absolute
    // path. It runs from inside the module, so $PWD is the checkout under test.
    executable(f.gcl, '#!/bin/sh\necho "$PWD/main.go:3:14: assignment to _ is pointless (tvrmsmithnoop)"\nexit 0\n')
    writeFileSync(f.registry, `${realpathSync(f.repo)}\n`)
    return f
  }

  const env = (f) => ({ TVRMSMITH_GO_REPOS: f.registry, TVRMSMITH_GCL: f.gcl })

  test('the default output keeps its heading, indentation and not-blocking note', () => {
    const f = fixture()
    try {
      const out = run(f.repo, ['--only', 'go', '--since', 'HEAD'], env(f))
      assert.match(out, /^\npersonal coding standards — 1 finding\(s\) in the changed Go files:\n/)
      assert.match(out, /^ {2}main\.go:3:14: assignment to _ is pointless \(tvrmsmithnoop\)$/m)
      assert.match(out, /reported, not blocking\./)
    } finally {
      f.cleanup()
    }
  })

  test('an unwired repository emits nothing and exits 0', () => {
    const f = fixture()
    try {
      writeFileSync(f.registry, '')
      assert.equal(run(f.repo, ['--only', 'go', '--since', 'HEAD'], env(f)), '')
    } finally {
      f.cleanup()
    }
  })
})

describe('one invocation, every language the repo is wired for', () => {
  /**
   * A repo that is both a TypeScript package and a Go module, with a stub for each linter. The
   * TypeScript stub's exit code is the fixture's to choose, because the aggregated status is the
   * one thing the branches do not decide for themselves.
   */
  function fixture({ tsExit = 0, gclExit = 0 } = {}) {
    const f = repository('tvrmsmith-dispatch-')
    f.registry = join(f.root, 'registry')
    f.gcl = join(f.root, 'stub-gcl')

    writeFileSync(join(f.repo, 'eslint.config.js'), 'export default []\n')
    writeFileSync(join(f.repo, 'go.mod'), 'module example.test\n\ngo 1.26.0\n')
    writeFileSync(join(f.repo, 'main.ts'), 'export const a = 1\n')
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() {}\n')
    executable(
      join(f.repo, 'node_modules/.bin/eslint'),
      [
        '#!/usr/bin/env node',
        "console.log('stylish ts finding')",
        `process.exit(${tsExit})`,
      ].join('\n') + '\n',
    )
    executable(
      f.gcl,
      gclExit === 0
        ? '#!/bin/sh\necho "$PWD/main.go:3:1: go finding (gorule)"\nexit 0\n'
        : `#!/bin/sh\necho "the config is unreadable" >&2\nexit ${gclExit}\n`,
    )
    commitAll(f.repo)
    writeFileSync(join(f.repo, 'main.ts'), 'export const a = 2\n')
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')
    writeFileSync(f.registry, `${realpathSync(f.repo)}\n`)
    return f
  }

  const env = (f) => ({ TVRMSMITH_GO_REPOS: f.registry, TVRMSMITH_GCL: f.gcl })

  test('no --only reports TypeScript and Go from a single call', () => {
    const f = fixture()
    try {
      const out = run(f.repo, ['--since', 'HEAD'], env(f))
      assert.match(out, /^stylish ts finding$/m)
      assert.match(out, /^ {2}main\.go:3:1: go finding \(gorule\)$/m)
    } finally {
      f.cleanup()
    }
  })

  test('a language the repo is not wired for contributes nothing and stops nothing', () => {
    const f = fixture()
    try {
      // The props file names no repository, so the C# branch skips. It must not take the other
      // two down with it, and it must contribute no stdout at all. Asserted as the whole of
      // stdout rather than as the absence of a string: a skipping branch has nothing it is
      // *supposed* to print, so only the exact report proves it stayed silent.
      writeFileSync(join(f.repo, 'Program.cs'), 'class Program {}\n')
      // Staged, or `git diff HEAD` never lists it, the branch is never dispatched, and the case
      // proves nothing about C# at all.
      git(f.repo, 'add', 'Program.cs')
      writeFileSync(join(f.root, 'empty.props'), '<Project />\n')
      const { status, stdout, stderr } = capture(f.repo, ['--since', 'HEAD'], {
        ...env(f),
        TVRMSMITH_ANALYZER_PROPS: join(f.root, 'empty.props'),
      })
      assert.equal(status, 0)
      // Named, so the case cannot pass through some other early return the branch takes.
      assert.match(stderr, /is not wired for \.NET/)
      assert.equal(
        stdout,
        '=== . ===\nstylish ts finding\n' +
          '\npersonal coding standards — 1 finding(s) in the changed Go files:\n' +
          '  main.go:3:1: go finding (gorule)\n' +
          '\n  reported, not blocking. Every personal Go rule is advisory, so the commit proceeds.\n',
      )
    } finally {
      f.cleanup()
    }
  })

  test('--only narrows to the one branch', () => {
    const f = fixture()
    try {
      const out = run(f.repo, ['--only', 'go', '--since', 'HEAD'], env(f))
      assert.match(out, /gorule/)
      assert.doesNotMatch(out, /stylish ts finding/)
    } finally {
      f.cleanup()
    }
  })

  test('a surviving finding exits 2, and the advisory branches still report', () => {
    const f = fixture({ tsExit: 1 })
    try {
      const { status, stdout } = capture(f.repo, ['--since', 'HEAD'], env(f))
      // ESLint's 1 is a lint error, which is this gate's 2: the gate says stop.
      assert.equal(status, 2)
      // Go runs after TypeScript and its findings are advisory. A failing earlier branch must not
      // swallow them, or a mixed commit reports only whichever language failed first.
      assert.match(stdout, /gorule/)
    } finally {
      f.cleanup()
    }
  })

  test('a broken branch exits 1 even alongside another branch reporting a finding', () => {
    // TypeScript reports a surviving finding (2) and the Go run then breaks (1). 1 has to win:
    // one branch never read the code it was given, so the gate did not answer, and a hook told 2
    // would report a fixable finding when half the change went unexamined.
    const f = fixture({ tsExit: 1, gclExit: 3 })
    try {
      const { status, stdout } = capture(f.repo, ['--since', 'HEAD'], env(f))
      assert.equal(status, 1)
      // The module and the linter's own words both survive. Piping the header and the output
      // into an undefined function discarded exactly this, leaving a blocked commit with no
      // stated cause.
      assert.match(stdout, /^=== \. — the lint run failed ===$/m)
      assert.match(stdout, /^the config is unreadable$/m)
    } finally {
      f.cleanup()
    }
  })

  test('an unknown --only is rejected rather than silently linting nothing', () => {
    const f = fixture()
    try {
      // 1, not 2: nothing was linted, so this is the gate breaking rather than a finding.
      const { status, stderr } = capture(f.repo, ['--only', 'rust', '--since', 'HEAD'], env(f))
      assert.equal(status, 1)
      assert.match(stderr, /--only takes one of: ts dotnet go/)
    } finally {
      f.cleanup()
    }
  })

  test('a multi-word --only is rejected rather than matching as a substring', () => {
    const f = fixture()
    try {
      const { status, stdout } = capture(f.repo, ['--only', 'ts go', '--since', 'HEAD'], env(f))
      assert.equal(status, 1)
      assert.equal(stdout, '')
    } finally {
      f.cleanup()
    }
  })

  test('a --since ref git cannot resolve fails rather than reporting a clean run', () => {
    const f = fixture()
    try {
      const { status, stdout } = capture(f.repo, ['--since', 'no-such-ref-xyz'], env(f))
      assert.equal(status, 1)
      assert.equal(stdout, '')
    } finally {
      f.cleanup()
    }
  })

  test('TVRMSMITH_REGISTRY_KEY naming no directory fails rather than skipping every branch', () => {
    const f = fixture()
    try {
      const { status, stderr } = capture(f.repo, ['--only', 'go', '--since', 'HEAD'], {
        ...env(f),
        TVRMSMITH_REGISTRY_KEY: join(f.root, 'typo'),
      })
      assert.equal(status, 1)
      assert.match(stderr, /which is not a directory/)
    } finally {
      f.cleanup()
    }
  })

  test('--files lints the named paths in every language they span', () => {
    const f = fixture()
    try {
      const out = run(f.repo, ['--files', 'main.ts', 'main.go'], env(f))
      assert.match(out, /^stylish ts finding$/m)
      assert.match(out, /^ {2}main\.go:3:1: go finding \(gorule\)$/m)
    } finally {
      f.cleanup()
    }
  })

  test('--files with no paths lints nothing and exits 0', () => {
    const f = fixture()
    try {
      // An empty list is nothing to lint, not a broken run. The entrypoint exits 1 when the
      // changed set cannot be computed, and bash 3.2 makes expanding an empty array an error, so
      // this is the case where the guard's own status could become that verdict.
      const { status, stdout } = capture(f.repo, ['--files'], env(f))
      assert.equal(status, 0)
      assert.equal(stdout, '')
    } finally {
      f.cleanup()
    }
  })
})

describe('the staged content, not the working copy', () => {
  test('TypeScript lints what the commit would contain and an error there exits 2', () => {
    const f = repository('tvrmsmith-staged-ts-')
    try {
      writeFileSync(join(f.repo, 'eslint.config.js'), 'export default []\n')
      writeFileSync(join(f.repo, 'main.ts'), 'export const a = 1\n')
      // Prints what it was fed on stdin, so the case can tell the staged copy from the disk copy,
      // and exits 1, ESLint's lint-error code.
      executable(
        join(f.repo, 'node_modules/.bin/eslint'),
        [
          '#!/usr/bin/env node',
          "let body = ''",
          "process.stdin.on('data', (c) => { body += c })",
          "process.stdin.on('end', () => { process.stdout.write(`linted: ${body}`); process.exit(1) })",
        ].join('\n') + '\n',
      )
      commitAll(f.repo)

      writeFileSync(join(f.repo, 'main.ts'), 'export const a = 2\n')
      git(f.repo, 'add', 'main.ts')
      // A third state on disk. The hook gates what the commit will contain, so linting this copy
      // would pass a commit on content nobody reviewed.
      writeFileSync(join(f.repo, 'main.ts'), 'export const a = 3\n')

      const { status, stdout } = capture(f.repo, ['--only', 'ts', '--staged'])
      assert.equal(status, 2)
      assert.match(stdout, /^=== main\.ts \(staged content\) ===$/m)
      assert.match(stdout, /^linted: export const a = 2$/m)
    } finally {
      f.cleanup()
    }
  })
})

describe('the Go branch, a package named after its module', () => {
  test('the package holding the changed file is linted, not the module root', () => {
    const f = repository('tvrmsmith-go-nested-')
    try {
      f.registry = join(f.root, 'registry')
      f.gcl = join(f.root, 'stub-gcl')
      mkdirSync(join(f.repo, 'gate/gate'), { recursive: true })
      writeFileSync(join(f.repo, 'gate/go.mod'), 'module gate\n\ngo 1.26.0\n')
      writeFileSync(join(f.repo, 'gate/gate/handler.go'), 'package gate\n\nfunc H() {}\n')
      // Reports only for the package it was actually asked to lint, which is what golangci-lint
      // does: name the module root and the finding in ./gate never appears.
      executable(
        f.gcl,
        [
          '#!/bin/sh',
          'for a in "$@"; do',
          '  [ "$a" = "./gate" ] && echo "$PWD/gate/handler.go:3:1: exported func H (gorule)"',
          'done',
          'exit 0',
        ].join('\n') + '\n',
      )
      writeFileSync(f.registry, `${realpathSync(f.repo)}\n`)
      commitAll(f.repo)
      writeFileSync(join(f.repo, 'gate/gate/handler.go'), 'package gate\n\nfunc H() { _ = 1 }\n')

      const out = run(f.repo, ['--only', 'go', '--since', 'HEAD'], {
        TVRMSMITH_GO_REPOS: f.registry,
        TVRMSMITH_GCL: f.gcl,
      })
      assert.match(out, /^ {2}gate\/gate\/handler\.go:3:1: exported func H \(gorule\)$/m)
    } finally {
      f.cleanup()
    }
  })
})

describe('the C# branch in a repository wired for .NET', () => {
  test('no dotnet on PATH fails rather than passing the staged C# unexamined', () => {
    const f = repository('tvrmsmith-dotnet-path-')
    try {
      const props = join(f.root, 'coding-standards.props')
      writeFileSync(join(f.repo, 'Program.cs'), 'class Program {}\n')
      commitAll(f.repo)
      writeFileSync(join(f.repo, 'Program.cs'), 'class Program { void M() { } }\n')
      git(f.repo, 'add', 'Program.cs')
      // The props file's path-scoped condition is the .NET registry, so this repo is adopted.
      writeFileSync(props, `<Project>\n  <!-- StartsWith('${realpathSync(f.repo)}/') -->\n</Project>\n`)

      // Every PATH entry that carries a dotnet is dropped, so the case is the same on a machine
      // with the SDK installed and one without. git and the rest of the toolchain stay.
      const path = (process.env.PATH ?? '')
        .split(':')
        .filter((entry) => entry && !existsSync(join(entry, 'dotnet')))
        .join(':')

      const { status, stderr } = capture(f.repo, ['--only', 'dotnet', '--staged'], {
        PATH: path,
        TVRMSMITH_ANALYZER_PROPS: props,
      })
      assert.equal(status, 1)
      assert.match(stderr, /no dotnet on PATH/)
    } finally {
      f.cleanup()
    }
  })
})
