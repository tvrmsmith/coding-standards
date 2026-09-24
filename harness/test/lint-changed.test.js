/**
 * The lint-changed.sh entrypoint: which branches it runs, and what each one puts on stdout.
 *
 * The thing worth pinning is that one invocation covers every language the repo is wired for,
 * since a caller having to name the branches is what this script exists to remove. Every branch
 * now emits `lint-changed`'s own porcelain rather than its linter's report, one
 * `path:line:column: RULE: message` line per surviving finding, repo-relative, and nothing else.
 * So the cases here check that a branch's finding reaches that one stream, rather than re-testing
 * the linter.
 *
 * TypeScript and Go are stubbed, in their linters' own report shapes: ESLint's `--format json` on
 * stdout, golangci-lint's JSON into the file named by `--output.json.path`. What is under test is
 * the dispatch, not what either tool thinks of a file. C# is not stubbable the same way — its
 * findings come from a Roslyn SARIF log — so lint-changed-dotnet.test.js covers that branch against
 * a real dotnet.
 *
 * `lint-changed` itself is not stubbed, because the verdict over every branch's reports is its own.
 * That needs go on PATH, so every case that reaches the filter skips without it.
 */
import { execFileSync, spawnSync } from 'node:child_process'
import {
  chmodSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from 'node:fs'
import { tmpdir } from 'node:os'
import { delimiter, dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test, { after, describe } from 'node:test'
import assert from 'node:assert/strict'

const harness = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..'))
const noGo = spawnSync('go', ['version'], { stdio: 'ignore' }).status !== 0
const skip = noGo && 'no go on PATH'

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

/**
 * A single-directory PATH resolving every command the real one did, except the named binary.
 * Dropping each PATH entry that holds a dotnet is not equivalent: where the SDK sits in /usr/bin,
 * as it does on the CI runner, that drops git and the rest of the toolchain too and the script
 * dies at 127 long before the check under test. Shadowing one name leaves everything else
 * reachable, so the case runs the same with the SDK installed and without it.
 *
 * @returns {string} the directory, ready to hand to a child as PATH
 */
function pathWithout(dir, command) {
  mkdirSync(dir, { recursive: true })
  const taken = new Set([command])
  for (const entry of (process.env.PATH ?? '').split(delimiter)) {
    if (!entry) continue
    let names = []
    try {
      names = readdirSync(entry)
    } catch {
      continue // A PATH entry that does not exist or cannot be read contributes nothing.
    }
    for (const name of names) {
      if (taken.has(name)) continue // First entry wins, the way PATH lookup itself resolves.
      taken.add(name)
      symlinkSync(join(entry, name), join(dir, name))
    }
  }
  return dir
}

function commitAll(repo) {
  git(repo, 'add', '-A')
  git(repo, 'commit', '--quiet', '-m', 'initial')
}

/**
 * One waiver log and one build cache for the whole file, so no case reads the developer's waiver
 * log or writes a lint-changed binary into their real cache. Shared across the cases rather than
 * made per fixture because the Go build cache underneath is what keeps every case after the first
 * from rebuilding the filter.
 */
const sandbox = mkdtempSync(join(realpathSync(tmpdir()), 'tvrmsmith-lint-sandbox-'))
after(() => rmSync(sandbox, { recursive: true, force: true }))

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
  TVRMSMITH_WAIVERS: join(sandbox, 'waivers.jsonl'),
  XDG_CACHE_HOME: join(sandbox, 'cache'),
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

/**
 * Stands in for a package's own ESLint: one severity-2 message per file argument, written to
 * stdout in ESLint's `--format json` shape, exiting 1 the way ESLint does once it has reported an
 * error. `filePath` is absolute under `$PWD`, the package directory ts.sh runs ESLint from, which
 * is the form real ESLint reports.
 *
 * The message sits on line 1 because every fixture below commits a one-line source file and then
 * rewrites that line, so line 1 is the changed line the finding has to survive on.
 */
const eslintStub = `#!/usr/bin/env node
const argv = process.argv.slice(2)
const valued = new Set(['--config', '--format'])
const files = []
for (let i = 0; i < argv.length; i++) {
  if (valued.has(argv[i])) { i++; continue }
  if (argv[i].startsWith('-')) continue
  files.push(argv[i])
}

const results = files.map((file) => ({
  filePath: \`\${process.cwd()}/\${file}\`,
  messages: [{
    ruleId: 'no-unused-vars',
    severity: 2,
    message: "'a' is assigned a value but never used.",
    line: 1,
    column: 7,
    endLine: 1,
    endColumn: 8,
  }],
  suppressedMessages: [],
}))

process.stdout.write(JSON.stringify(results))
process.exit(1)
`

/** The porcelain line the stub above produces for a file, once the branch has filtered it. */
const eslintPorcelain = (file) => `${file}:1:7: no-unused-vars: 'a' is assigned a value but never used.\n`

describe('the TypeScript branch', () => {
  /** A package whose ESLint is the stub above, with its one source line changed. */
  function fixture() {
    const f = repository('tvrmsmith-lint-ts-')
    writeFileSync(join(f.repo, 'eslint.config.js'), 'export default []\n')
    writeFileSync(join(f.repo, 'main.ts'), 'export const a = 1\n')
    executable(join(f.repo, 'node_modules/.bin/eslint'), eslintStub)
    commitAll(f.repo)
    writeFileSync(join(f.repo, 'main.ts'), 'export const a = 2\n')
    return f
  }

  test('a finding on a changed line reaches stdout as one porcelain line', { skip }, () => {
    const f = fixture()
    try {
      const { status, stdout, stderr } = capture(f.repo, ['--only', 'ts', '--since', 'HEAD'])
      assert.equal(status, 2, `expected the blocking exit code\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      // The whole of stdout. no-mistakes reads this stream with one regex across every language,
      // so a package header or ESLint's own report leaking onto it is a bug, not noise.
      assert.equal(stdout, eslintPorcelain('main.ts'))
    } finally {
      f.cleanup()
    }
  })

  test('a changed path holding a space reaches the linter as one argument', { skip }, () => {
    const f = fixture()
    try {
      // Split on the space, ESLint is handed two paths that do not exist and fails, so the commit
      // is blocked over a file nobody wrote. The stub reports its argv on stderr, which ts.sh lets
      // through untouched, because its stdout is captured into the report file and so cannot carry
      // it. The empty report it writes there is what keeps this case's verdict clean.
      executable(
        join(f.repo, 'node_modules/.bin/eslint'),
        [
          '#!/usr/bin/env node',
          'for (const a of process.argv.slice(2)) console.error(`arg: ${a}`)',
          "process.stdout.write('[]')",
        ].join('\n') + '\n',
      )
      writeFileSync(join(f.repo, 'my file.ts'), 'export const b = 1\n')
      // Staged, because `git diff HEAD` reports a new file only once the index carries it.
      git(f.repo, 'add', 'my file.ts')
      const { status, stdout, stderr } = capture(f.repo, ['--only', 'ts', '--since', 'HEAD'])
      assert.equal(status, 0, `expected a clean pass\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stderr, /^arg: my file\.ts$/m)
      assert.doesNotMatch(stderr, /^arg: my$/m)
    } finally {
      f.cleanup()
    }
  })

  test('a --files path holding a comma reaches the filter as one path', { skip }, () => {
    // Joined into one comma-separated --files value, a,b.ts reached lint-changed as a and b.ts,
    // neither of which exists, and the run broke with exit 1 instead of reporting the finding.
    const f = fixture()
    try {
      writeFileSync(join(f.repo, 'a,b.ts'), 'export const b = 1\n')
      const { status, stdout, stderr } = capture(f.repo, ['--only', 'ts', '--files', 'a,b.ts'])
      assert.equal(status, 2, `expected the blocking exit code\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.equal(stdout, eslintPorcelain('a,b.ts'))
    } finally {
      f.cleanup()
    }
  })
})

/**
 * Stands in for the personal golangci-lint binary. It writes golangci's own JSON to the file named
 * by `--output.json.path`, holding one issue at `file`. Given `wanted`, it holds that issue only
 * when `wanted` is among its arguments, which is golangci's own contract: it reports for the
 * package it was asked about and no other.
 *
 * Reading the flag out of argv in the space-separated form is deliberate coupling, since pinning
 * the spelling go.sh passes is part of what these cases are for. `--output.json.path=FILE`, or the
 * report going to stdout, leaves `out` empty and the stub fails loudly rather than letting the
 * branch report a clean run on a report nobody wrote.
 *
 * `file` is absolute because go.sh runs the linter with --path-mode abs, and baked in from the
 * fixture rather than derived from `$PWD`, which is the module directory and so differs per shape.
 */
function golangciStub({ file, linter, text, line, column, wanted }) {
  const issue =
    `{"FromLinter":"${linter}","Text":"${text}",` +
    `"Pos":{"Filename":"${file}","Line":${line},"Column":${column}}}`
  const record = wanted === undefined ? 'issues=$issue' : `[ "$1" = "${wanted}" ] && issues=$issue`
  return `#!/bin/sh
issue='${issue}'
out=
issues=
while [ $# -gt 0 ]; do
  case "$1" in
    --output.json.path) out=$2; shift 2 ;;
    *) ${record}; shift ;;
  esac
done

if [ -z "$out" ]; then
  echo "stub: no --output.json.path in argv" >&2
  exit 9
fi

printf '{"Issues":[%s]}\\n' "$issues" >"$out"
`
}

describe('the Go branch', () => {
  /** A module whose golangci-lint is the stub above, with its line 3 changed. */
  function fixture() {
    const f = repository('tvrmsmith-lint-go-')
    f.registry = join(f.root, 'registry')
    f.gcl = join(f.root, 'stub-gcl')
    writeFileSync(join(f.repo, 'go.mod'), 'module example.test\n\ngo 1.26.0\n')
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() {}\n')
    commitAll(f.repo)
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')
    executable(
      f.gcl,
      golangciStub({
        file: `${realpathSync(f.repo)}/main.go`,
        linter: 'tvrmsmithnoop',
        text: 'assignment to _ is pointless',
        line: 3,
        column: 14,
      }),
    )
    writeFileSync(f.registry, `${realpathSync(f.repo)}\n`)
    return f
  }

  const env = (f) => ({ TVRMSMITH_GO_REPOS: f.registry, TVRMSMITH_GCL: f.gcl })

  test('a finding on a changed line reaches stdout as one porcelain line', { skip }, () => {
    const f = fixture()
    try {
      const { status, stdout, stderr } = capture(f.repo, ['--only', 'go', '--since', 'HEAD'], env(f))
      assert.equal(status, 2, `expected the blocking exit code\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      // The whole of stdout. no-mistakes reads this stream with one regex across every language,
      // so golangci's own summary or a heading leaking onto it is a bug, not noise.
      assert.equal(stdout, 'main.go:3:14: tvrmsmithnoop: assignment to _ is pointless\n')
    } finally {
      f.cleanup()
    }
  })

  test('an unwired repository emits nothing and exits 0', { skip }, () => {
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
   * A repo that is both a TypeScript package and a Go module, with a stub for each linter, each
   * reporting one finding on the line the fixture then changes. The golangci stub's exit code is
   * the fixture's to choose, because how a broken branch folds into the aggregate status is the
   * one thing the branches do not decide for themselves.
   */
  function fixture({ gclExit = 0 } = {}) {
    const f = repository('tvrmsmith-dispatch-')
    f.registry = join(f.root, 'registry')
    f.gcl = join(f.root, 'stub-gcl')

    writeFileSync(join(f.repo, 'eslint.config.js'), 'export default []\n')
    writeFileSync(join(f.repo, 'go.mod'), 'module example.test\n\ngo 1.26.0\n')
    writeFileSync(join(f.repo, 'main.ts'), 'export const a = 1\n')
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() {}\n')
    executable(join(f.repo, 'node_modules/.bin/eslint'), eslintStub)
    executable(
      f.gcl,
      gclExit === 0
        ? golangciStub({
            file: `${realpathSync(f.repo)}/main.go`,
            linter: 'gorule',
            text: 'go finding',
            line: 3,
            column: 1,
          })
        : `#!/bin/sh\necho "the config is unreadable" >&2\nexit ${gclExit}\n`,
    )
    commitAll(f.repo)
    writeFileSync(join(f.repo, 'main.ts'), 'export const a = 2\n')
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')
    writeFileSync(f.registry, `${realpathSync(f.repo)}\n`)
    return f
  }

  /** stdout for a run where both branches report, in the order lint-changed.sh runs them. */
  const bothPorcelain = eslintPorcelain('main.ts') + 'main.go:3:1: gorule: go finding\n'

  const env = (f) => ({ TVRMSMITH_GO_REPOS: f.registry, TVRMSMITH_GCL: f.gcl })

  test('no --only reports TypeScript and Go from a single call', { skip }, () => {
    const f = fixture()
    try {
      const { status, stdout, stderr } = capture(f.repo, ['--since', 'HEAD'], env(f))
      assert.equal(status, 2, `expected the blocking exit code\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stdout, /^main\.ts:1:7: no-unused-vars: /m)
      assert.match(stdout, /^main\.go:3:1: gorule: go finding$/m)
    } finally {
      f.cleanup()
    }
  })

  test('a language the repo is not wired for contributes nothing and stops nothing', { skip }, () => {
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
      // 2, because both the other branches' findings survive. The C# branch skipping is not
      // allowed to turn that into a clean run any more than into a broken one.
      assert.equal(status, 2, `stdout:\n${stdout}\nstderr:\n${stderr}`)
      // Named, so the case cannot pass through some other early return the branch takes.
      assert.match(stderr, /is not wired for \.NET/)
      assert.equal(stdout, bothPorcelain)
    } finally {
      f.cleanup()
    }
  })

  test('--only narrows to the one branch', { skip }, () => {
    const f = fixture()
    try {
      const { status, stdout, stderr } = capture(f.repo, ['--only', 'go', '--since', 'HEAD'], env(f))
      assert.equal(status, 2, `stdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stdout, /gorule/)
      assert.doesNotMatch(stdout, /no-unused-vars/)
    } finally {
      f.cleanup()
    }
  })

  test('a branch reporting a finding does not swallow a later branch', { skip }, () => {
    const f = fixture()
    try {
      const { status, stdout, stderr } = capture(f.repo, ['--since', 'HEAD'], env(f))
      assert.equal(status, 2, `stdout:\n${stdout}\nstderr:\n${stderr}`)
      // TypeScript reports first and Go runs after it, and both findings have to reach the same
      // stdout, or a mixed commit hands back only whichever language blocked first and the next
      // run surfaces the rest. Asserted as the whole of stdout, in dispatch order.
      assert.equal(stdout, bothPorcelain)
      // One filter reads both reports, so each waive hint takes its language from its own finding.
      assert.match(stderr, /waive --language ts --path main\.ts --rule no-unused-vars /)
      assert.match(stderr, /waive --language go --path main\.go --rule gorule /)
    } finally {
      f.cleanup()
    }
  })

  test('a broken branch exits 1 even alongside another branch reporting a finding', { skip }, () => {
    // TypeScript reports a surviving finding (2) and the Go run then breaks (1). 1 has to win:
    // one branch never read the code it was given, so the gate did not answer, and a hook told 2
    // would report a fixable finding when half the change went unexamined.
    const f = fixture({ gclExit: 3 })
    try {
      const { status, stdout, stderr } = capture(f.repo, ['--since', 'HEAD'], env(f))
      assert.equal(status, 1, `stdout:\n${stdout}\nstderr:\n${stderr}`)
      // The module and the linter's own words both survive, on stderr. Piping the header and the
      // output into an undefined function discarded exactly this, leaving a blocked commit with
      // no stated cause. stdout stays the porcelain stream alone, so the banner cannot be read as
      // a finding by the one regex no-mistakes runs over it.
      assert.match(stderr, /^=== \. — the lint run failed ===$/m)
      assert.match(stderr, /^the config is unreadable$/m)
      assert.equal(stdout, "main.ts:1:7: no-unused-vars: 'a' is assigned a value but never used.\n")
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

  test('a --since ref git cannot resolve fails rather than reporting a clean run', { skip }, () => {
    const f = fixture()
    try {
      const { status, stdout, stderr } = capture(f.repo, ['--since', 'no-such-ref-xyz'], env(f))
      assert.equal(status, 1)
      assert.equal(stdout, '')
      assert.match(stderr, /no-such-ref-xyz/)
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

  test('--files lints the named paths in every language they span', { skip }, () => {
    const f = fixture()
    try {
      const { status, stdout, stderr } = capture(f.repo, ['--files', 'main.ts', 'main.go'], env(f))
      assert.equal(status, 2, `stdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stdout, /^main\.ts:1:7: no-unused-vars: /m)
      assert.match(stdout, /^main\.go:3:1: gorule: go finding$/m)
    } finally {
      f.cleanup()
    }
  })

  describe("a waiver is one commit's worth of permission", () => {
    /**
     * A waiver log of the fixture's own, rather than the file-wide one, because a waiver these
     * cases leave unspent would still match the same finding in every later fixture.
     */
    function waivers(f, ...findings) {
      const log = join(f.root, 'waivers.jsonl')
      const lines = findings.map(({ language, path, rule }) =>
        JSON.stringify({ kind: 'waiver', id: `${language}-waiver`, language, path, rule, reason: 'test' }),
      )
      writeFileSync(log, lines.map((line) => `${line}\n`).join(''))
      return log
    }

    /**
     * Like `waivers`, but every waiver is already spent against a tree that is never the
     * fixture's own: the shape a no-mistakes run sees in a detached worktree whose index tree
     * differs from the tree the commit spent the waiver against.
     */
    function waiversSpentElsewhere(f, ...findings) {
      const log = join(f.root, 'waivers.jsonl')
      const ids = findings.map(({ language }) => `${language}-waiver`)
      const waiverLines = findings.map(({ language, path, rule }, i) =>
        JSON.stringify({ kind: 'waiver', id: ids[i], language, path, rule, reason: 'test' }),
      )
      const spendLines = ids.map((id) =>
        JSON.stringify({ kind: 'spend', id, tree: '0'.repeat(40), spent: '2026-01-01T00:00:00Z' }),
      )
      writeFileSync(log, [...waiverLines, ...spendLines].map((line) => `${line}\n`).join(''))
      return log
    }

    /** @returns {object[]} every record in the log, in the order they were written */
    const records = (log) =>
      readFileSync(log, 'utf8')
        .split('\n')
        .filter(Boolean)
        .map((line) => JSON.parse(line))

    /** The pre-commit hook's run, the only one that spends: every change staged, disk matching. */
    function staged(f) {
      git(f.repo, 'add', 'main.ts', 'main.go')
      return f
    }

    const tsFinding = { language: 'ts', path: 'main.ts', rule: 'no-unused-vars' }
    const goFinding = { language: 'go', path: 'main.go', rule: 'gorule' }

    test('a waiver per finding lets the whole commit through and is spent', { skip }, () => {
      const f = staged(fixture())
      try {
        const log = waivers(f, tsFinding, goFinding)
        const { status, stdout, stderr } = capture(f.repo, ['--staged'], {
          ...env(f),
          TVRMSMITH_WAIVERS: log,
        })
        assert.equal(status, 0, `stdout:\n${stdout}\nstderr:\n${stderr}`)
        assert.equal(stdout, '')
        assert.deepEqual(
          records(log).map(({ kind, id }) => ({ kind, id })),
          [
            { kind: 'waiver', id: 'ts-waiver' },
            { kind: 'waiver', id: 'go-waiver' },
            { kind: 'spend', id: 'ts-waiver' },
            { kind: 'spend', id: 'go-waiver' },
          ],
        )
      } finally {
        f.cleanup()
      }
    })

    test('a waiver is not spent while another language still blocks', { skip }, () => {
      // One filter process per language spent the TypeScript waiver on TypeScript's clean share,
      // then Go blocked the commit, so the waiver was gone and the commit it paid for never made.
      const f = staged(fixture())
      try {
        const log = waivers(f, tsFinding)
        const { status, stdout, stderr } = capture(f.repo, ['--staged'], {
          ...env(f),
          TVRMSMITH_WAIVERS: log,
        })
        assert.equal(status, 2, `stdout:\n${stdout}\nstderr:\n${stderr}`)
        assert.equal(stdout, 'main.go:3:1: gorule: go finding\n')
        assert.equal(records(log).length, 1)
      } finally {
        f.cleanup()
      }
    })

    test('a waiver is not spent while another branch is broken', { skip }, () => {
      // The filter comes back clean, since the one finding it read is waived, but the Go run broke
      // and git will not make the commit.
      const f = staged(fixture({ gclExit: 3 }))
      try {
        const log = waivers(f, tsFinding)
        const { status, stdout, stderr } = capture(f.repo, ['--staged'], {
          ...env(f),
          TVRMSMITH_WAIVERS: log,
        })
        assert.equal(status, 1, `stdout:\n${stdout}\nstderr:\n${stderr}`)
        assert.equal(stdout, '')
        assert.equal(records(log).length, 1)
      } finally {
        f.cleanup()
      }
    })

    for (const [name, args] of [
      ['a clean --since run', ['--since', 'HEAD']],
      ['a clean --files run', ['--files', 'main.ts', 'main.go']],
    ]) {
      test(`${name} passes on its waivers and spends none`, { skip }, () => {
        // Neither run is a commit, so a waiver it matches lets the finding through and stays for
        // the commit that follows.
        const f = fixture()
        try {
          const log = waivers(f, tsFinding, goFinding)
          const { status, stdout, stderr } = capture(f.repo, args, { ...env(f), TVRMSMITH_WAIVERS: log })
          assert.equal(status, 0, `stdout:\n${stdout}\nstderr:\n${stderr}`)
          assert.equal(stdout, '')
          assert.match(stderr, /waiver ts-waiver matched/)
          assert.match(stderr, /waiver go-waiver matched/)
          assert.deepEqual(
            records(log).map(({ kind }) => kind),
            ['waiver', 'waiver'],
          )
        } finally {
          f.cleanup()
        }
      })
    }

    test('a clean --only --staged run spends none', { skip }, () => {
      // --only lints one language's share of the commit, so its clean result says nothing about
      // the rest, and no-mistakes runs one such entry per language.
      const f = staged(fixture())
      try {
        const log = waivers(f, goFinding)
        const { status, stdout, stderr } = capture(f.repo, ['--only', 'go', '--staged'], {
          ...env(f),
          TVRMSMITH_WAIVERS: log,
        })
        assert.equal(status, 0, `stdout:\n${stdout}\nstderr:\n${stderr}`)
        assert.equal(stdout, '')
        assert.match(stderr, /waiver go-waiver matched/)
        assert.equal(records(log).length, 1)
      } finally {
        f.cleanup()
      }
    })

    test('a clean run whose spend fails exits 1', { skip: skip || (process.getuid?.() === 0 && 'root ignores a read-only log') }, () => {
      // Every finding is waived, so the total is clean until spend runs. The filter only reads the
      // log, and spend's append is the first write, so a read-only log fails the spend alone and
      // the commit must not go through with its waivers unspent.
      const f = staged(fixture())
      try {
        const log = waivers(f, tsFinding, goFinding)
        chmodSync(log, 0o444)
        const { status, stdout, stderr } = capture(f.repo, ['--staged'], {
          ...env(f),
          TVRMSMITH_WAIVERS: log,
        })
        assert.equal(status, 1, `stdout:\n${stdout}\nstderr:\n${stderr}`)
        assert.equal(stdout, '')
        assert.equal(records(log).length, 2)
      } finally {
        f.cleanup()
      }
    })

    test('a clean --since run accepts a waiver spent against another tree', { skip }, () => {
      // A no-mistakes run lints a detached worktree whose index tree is never the tree the
      // commit spent the waiver against, so a non-spending run has to accept it anyway.
      const f = fixture()
      try {
        const log = waiversSpentElsewhere(f, tsFinding, goFinding)
        const { status, stdout, stderr } = capture(f.repo, ['--since', 'HEAD'], {
          ...env(f),
          TVRMSMITH_WAIVERS: log,
        })
        assert.equal(status, 0, `stdout:\n${stdout}\nstderr:\n${stderr}`)
        assert.equal(stdout, '')
        assert.match(stderr, /waiver ts-waiver matched/)
        assert.match(stderr, /waiver go-waiver matched/)
        assert.deepEqual(
          records(log).map(({ kind }) => kind),
          ['waiver', 'waiver', 'spend', 'spend'],
        )
      } finally {
        f.cleanup()
      }
    })

    test('a clean --files run accepts a waiver spent against another tree', { skip }, () => {
      const f = fixture()
      try {
        const log = waiversSpentElsewhere(f, tsFinding, goFinding)
        const { status, stdout, stderr } = capture(f.repo, ['--files', 'main.ts', 'main.go'], {
          ...env(f),
          TVRMSMITH_WAIVERS: log,
        })
        assert.equal(status, 0, `stdout:\n${stdout}\nstderr:\n${stderr}`)
        assert.equal(stdout, '')
        assert.deepEqual(
          records(log).map(({ kind }) => kind),
          ['waiver', 'waiver', 'spend', 'spend'],
        )
      } finally {
        f.cleanup()
      }
    })

    test('a clean --only --staged run accepts a waiver spent against another tree', { skip }, () => {
      // --only sees one language's share of the commit, never the whole of it, so it does not
      // spend either and gets the same latitude as --since and --files.
      const f = staged(fixture())
      try {
        const log = waiversSpentElsewhere(f, goFinding)
        const { status, stdout, stderr } = capture(f.repo, ['--only', 'go', '--staged'], {
          ...env(f),
          TVRMSMITH_WAIVERS: log,
        })
        assert.equal(status, 0, `stdout:\n${stdout}\nstderr:\n${stderr}`)
        assert.equal(stdout, '')
        assert.match(stderr, /waiver go-waiver matched/)
        assert.equal(records(log).length, 2)
      } finally {
        f.cleanup()
      }
    })

    test('a full --staged run still blocks on a waiver spent against another tree', { skip }, () => {
      // The pre-commit hook is the one run that spends, so it keeps today's rule: unspent, or
      // spent against this same tree. A waiver spent elsewhere does not cover it.
      const f = staged(fixture())
      try {
        const log = waiversSpentElsewhere(f, tsFinding, goFinding)
        const { status, stdout, stderr } = capture(f.repo, ['--staged'], {
          ...env(f),
          TVRMSMITH_WAIVERS: log,
        })
        assert.equal(status, 2, `stdout:\n${stdout}\nstderr:\n${stderr}`)
        assert.match(stdout, /^main\.ts:1:7: no-unused-vars: /m)
        assert.match(stdout, /^main\.go:3:1: gorule: go finding$/m)
        assert.equal(records(log).length, 4)
      } finally {
        f.cleanup()
      }
    })
  })

  test('--files naming a path absent from disk still lints the rest', { skip }, () => {
    // lint-changed refuses a --files path that is not a regular file, so the dispatcher must
    // leave the absent one out of the filter's scope rather than break the whole run over it.
    const f = fixture()
    try {
      const { status, stdout, stderr } = capture(f.repo, ['--files', 'main.ts', 'main.go', 'gone.go'], env(f))
      assert.equal(status, 2, `stdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stdout, /^main\.ts:1:7: no-unused-vars: /m)
      assert.match(stdout, /^main\.go:3:1: gorule: go finding$/m)
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

describe('the Go branch names the package holding the changed file', () => {
  /**
   * golangci-lint runs per package, so the whole result rides on which package path it is handed.
   * Each shape below is a module location crossed with a file depth, and each is a way the path
   * arithmetic can collapse to the wrong package — and a wrong package is silent, since the report
   * then holds nothing and the file reads as clean unlinted.
   *
   * The stub is golangci-lint's own contract: it reports only for the package it was asked about.
   */
  const shapes = [
    { name: 'a module at the repo root, file one directory down', module: '.', dir: 'gate', want: './gate' },
    { name: 'a module at the repo root, file two directories down', module: '.', dir: 'gate/gate', want: './gate/gate' },
    { name: 'a module below the root, file in the module root', module: 'gate', dir: 'gate', want: './.' },
    { name: 'a module below the root, package repeating the module name', module: 'gate', dir: 'gate/gate', want: './gate' },
  ]

  for (const shape of shapes) {
    test(shape.name, { skip }, () => {
      const f = repository('tvrmsmith-go-shape-')
      try {
        const registry = join(f.root, 'registry')
        const gcl = join(f.root, 'stub-gcl')
        const file = `${shape.dir}/handler.go`
        mkdirSync(join(f.repo, shape.dir), { recursive: true })
        writeFileSync(join(f.repo, shape.module, 'go.mod'), 'module gate\n\ngo 1.26.0\n')
        writeFileSync(join(f.repo, file), 'package gate\n\nfunc H() {}\n')
        executable(
          gcl,
          golangciStub({
            file: `${realpathSync(f.repo)}/${file}`,
            linter: 'gorule',
            text: 'exported func H',
            line: 3,
            column: 1,
            wanted: shape.want,
          }),
        )
        writeFileSync(registry, `${realpathSync(f.repo)}\n`)
        commitAll(f.repo)
        writeFileSync(join(f.repo, file), 'package gate\n\nfunc H() { _ = 1 }\n')

        const { status, stdout, stderr } = capture(f.repo, ['--only', 'go', '--since', 'HEAD'], {
          TVRMSMITH_GO_REPOS: registry,
          TVRMSMITH_GCL: gcl,
        })
        // A wrong package path is silent rather than loud. The stub writes an empty report, the
        // filter has nothing to keep, and the run reads as a clean 0 over a file nobody linted, so
        // the status carries as much of the property as the line does.
        assert.equal(status, 2, `stdout:\n${stdout}\nstderr:\n${stderr}`)
        assert.equal(stdout, `${file}:3:1: gorule: exported func H\n`)
      } finally {
        f.cleanup()
      }
    })
  }
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

      const { status, stderr } = capture(f.repo, ['--only', 'dotnet', '--staged'], {
        PATH: pathWithout(join(f.root, 'bin'), 'dotnet'),
        TVRMSMITH_ANALYZER_PROPS: props,
      })
      assert.equal(status, 1)
      assert.match(stderr, /no dotnet on PATH/)
    } finally {
      f.cleanup()
    }
  })
})

describe('the changed-line filter every branch shares', () => {
  // The guard lives in common.sh now that all three branches build the same binary. No go means
  // no filter, and an unrun filter proves nothing about the code it never read, so the branch
  // fails the commit rather than reporting a clean run on unexamined TypeScript.
  test('no go on PATH fails rather than passing the changed TypeScript unexamined', () => {
    const f = repository('tvrmsmith-nogo-')
    try {
      writeFileSync(join(f.repo, 'eslint.config.js'), 'export default []\n')
      writeFileSync(join(f.repo, 'main.ts'), 'export const a = 1\n')
      executable(join(f.repo, 'node_modules/.bin/eslint'), eslintStub)
      commitAll(f.repo)
      writeFileSync(join(f.repo, 'main.ts'), 'export const a = 2\n')

      const { status, stdout, stderr } = capture(f.repo, ['--only', 'ts', '--since', 'HEAD'], {
        PATH: pathWithout(join(f.root, 'bin'), 'go'),
      })
      assert.equal(status, 1, `expected the gate to fail\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stderr, /no go on PATH/)
    } finally {
      f.cleanup()
    }
  })
})

describe('--staged during an uncommitted merge', () => {
  /** An ESLint stand-in that reports its argv on stderr, so a case can see which files reached it. */
  function mergeRepository(prefix) {
    const f = repository(prefix)
    writeFileSync(join(f.repo, 'eslint.config.js'), 'export default []\n')
    writeFileSync(join(f.repo, '.gitignore'), 'node_modules\n')
    // The empty report keeps the verdict clean.
    executable(
      join(f.repo, 'node_modules/.bin/eslint'),
      [
        '#!/usr/bin/env node',
        'for (const a of process.argv.slice(2)) console.error(`arg: ${a}`)',
        "process.stdout.write('[]')",
      ].join('\n') + '\n',
    )
    return f
  }

  /** Starts the merge of `incoming` and proves it stopped on a conflict with MERGE_HEAD in place. */
  function mergeIncoming(repo) {
    const merge = spawnSync('git', ['merge', '--quiet', '--no-commit', '--no-ff', 'incoming'], {
      cwd: repo,
      encoding: 'utf8',
    })
    assert.equal(merge.status, 1, `expected a conflict\nstdout:\n${merge.stdout}\nstderr:\n${merge.stderr}`)
    git(repo, 'rev-parse', '--quiet', '--verify', 'MERGE_HEAD')
  }

  // Against HEAD, a merge commit's index holds everything the incoming side brought, which on a
  // long-lived branch meant thousands of files and dozens of .NET builds per commit. Against
  // MERGE_HEAD it holds everything the branch already committed, whose lines all match HEAD, so the
  // filter drops every finding in them. Only a file differing from both can carry one.
  test('lints only the files the merge writes, not either side\'s own changes', { skip }, () => {
    const f = mergeRepository('tvrmsmith-merge-')
    try {
      writeFileSync(join(f.repo, 'shared.ts'), 'export const shared = 1\n')
      commitAll(f.repo)
      git(f.repo, 'checkout', '--quiet', '-b', 'incoming')
      writeFileSync(join(f.repo, 'incoming.ts'), 'export const theirs = 1\n')
      writeFileSync(join(f.repo, 'shared.ts'), 'export const shared = 2\n')
      commitAll(f.repo)
      git(f.repo, 'checkout', '--quiet', 'main')
      writeFileSync(join(f.repo, 'mine.ts'), 'export const mine = 1\n')
      writeFileSync(join(f.repo, 'shared.ts'), 'export const shared = 3\n')
      commitAll(f.repo)
      mergeIncoming(f.repo)
      // The resolution is the content neither side had.
      writeFileSync(join(f.repo, 'shared.ts'), 'export const shared = 4\n')
      git(f.repo, 'add', 'shared.ts')

      const { status, stdout, stderr } = capture(f.repo, ['--staged'])
      assert.equal(status, 0, `expected a clean pass\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stderr, /^arg: shared\.ts$/m)
      assert.doesNotMatch(stderr, /^arg: incoming\.ts$/m)
      assert.doesNotMatch(stderr, /^arg: mine\.ts$/m)
    } finally {
      f.cleanup()
    }
  })

  // Against MERGE_HEAD a path the branch renamed reads as a rename, which the ACM filter drops
  // unless rename detection is off.
  test('lints a conflict resolved in a file the branch renamed', { skip }, () => {
    const f = mergeRepository('tvrmsmith-merge-rename-')
    const body = (value) =>
      ['export const a = 1', 'export const b = 2', 'export const c = 3', `export const d = ${value}`, ''].join('\n')
    try {
      writeFileSync(join(f.repo, 'before.ts'), body(1))
      commitAll(f.repo)
      git(f.repo, 'checkout', '--quiet', '-b', 'incoming')
      writeFileSync(join(f.repo, 'before.ts'), body(2))
      commitAll(f.repo)
      git(f.repo, 'checkout', '--quiet', 'main')
      git(f.repo, 'mv', 'before.ts', 'after.ts')
      writeFileSync(join(f.repo, 'after.ts'), body(3))
      commitAll(f.repo)
      mergeIncoming(f.repo)
      writeFileSync(join(f.repo, 'after.ts'), body(4))
      git(f.repo, 'add', 'after.ts')

      const { status, stdout, stderr } = capture(f.repo, ['--staged'])
      assert.equal(status, 0, `expected a clean pass\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stderr, /^arg: after\.ts$/m)
    } finally {
      f.cleanup()
    }
  })
})
