/**
 * The TypeScript branch of lint-changed.sh: whether an ESLint finding on a changed line stops the
 * commit, and whether one on a line the change never touched is dropped.
 *
 * ESLint is stubbed. What is under test is the script's own wiring — the flags it hands ESLint,
 * the per-package report files it captures, and the one `lint-changed` run it folds them into —
 * not what ESLint thinks of a file. The stub's JSON is the shape real ESLint 9.39.5 emits, checked
 * against the binary before these cases were written, including the ignored-file message TS7 pins.
 *
 * `lint-changed` itself is not stubbed, because the scoping verdict is the thing being pinned.
 * That needs go on PATH, so the cases skip without it.
 */
import { execFileSync, spawnSync } from 'node:child_process'
import { chmodSync, mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import assert from 'node:assert/strict'

const harness = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..'))
const script = join(harness, 'lint-changed.sh')
const noGo = spawnSync('go', ['version'], { stdio: 'ignore' }).status !== 0
const skip = noGo && 'no go on PATH'

function git(cwd, ...args) {
  return execFileSync('git', args, { cwd, encoding: 'utf8' })
}

/**
 * Stands in for the package's own ESLint. It writes one result per file argument to stdout in
 * ESLint's `--format json` shape and exits the way ESLint does: 1 when it reported an error, 0
 * otherwise, and `STUB_EXIT` for a run that never got as far as linting.
 *
 * Two couplings are deliberate, because pinning what ts.sh passes is half of what these cases are
 * for. `filePath` is absolute under `$PWD`, which is the package ts.sh runs ESLint from and the
 * form real ESLint reports. And `STUB_IGNORED` reproduces the ignored-file warning verbatim unless
 * `--no-warn-ignored` is in argv, so a branch that stopped passing the flag fails TS7 rather than
 * quietly blocking every commit that touches an ignored file.
 */
const stubSource = `#!/usr/bin/env node
const argv = process.argv.slice(2)
const exit = Number(process.env.STUB_EXIT ?? 0)
if (exit !== 0) process.exit(exit)

const valued = new Set(['--config', '--format', '--stdin-filename', '--output-file'])
const files = []
for (let i = 0; i < argv.length; i++) {
  if (valued.has(argv[i])) { i++; continue }
  if (argv[i].startsWith('-')) continue
  files.push(argv[i])
}

const line = Number(process.env.STUB_LINE ?? 3)
const severity = Number(process.env.STUB_SEVERITY ?? 2)
const ignored = process.env.STUB_IGNORED
const noWarnIgnored = argv.includes('--no-warn-ignored')

const results = []
for (const file of files) {
  const filePath = \`\${process.cwd()}/\${file}\`
  if (file === ignored) {
    if (noWarnIgnored) continue
    results.push({
      filePath,
      messages: [{
        ruleId: null,
        fatal: false,
        severity: 1,
        message: 'File ignored because of a matching ignore pattern. Use "--no-ignore" to disable file ignore settings or use "--no-warn-ignored" to suppress this warning.',
        nodeType: null,
      }],
      suppressedMessages: [],
    })
    continue
  }
  results.push({
    filePath,
    messages: [{
      ruleId: 'no-unused-vars',
      severity,
      message: "'x' is assigned a value but never used.",
      line,
      column: 7,
      endLine: line,
      endColumn: 8,
    }],
    suppressedMessages: [],
  })
}

process.stdout.write(JSON.stringify(results))
process.exit(results.some((r) => r.messages.some((m) => m.severity === 2)) ? 1 : 0)
`

/**
 * A repository that is one ESLint package with a stub binary installed. src/a.js is three lines,
 * so a case changing line 3 has a line the stub's default position falls on and line 1 is a line
 * it never touched.
 */
function fixture() {
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'tvrmsmith-ts-'))
  const repo = join(root, 'repo')

  git(root, 'init', '--quiet', '--initial-branch=main', repo)
  git(repo, 'config', 'user.email', 'test@example.com')
  git(repo, 'config', 'user.name', 'Test')
  git(repo, 'config', 'commit.gpgsign', 'false')
  writeFileSync(join(repo, 'package.json'), '{"name":"example","type":"module"}\n')
  writeFileSync(join(repo, 'eslint.config.js'), 'export default []\n')
  mkdirSync(join(repo, 'src'))
  writeFileSync(join(repo, 'src/a.js'), 'export const a = 1\nexport const b = 2\nconst x = 3\n')
  installStub(repo)
  git(repo, 'add', '.')
  git(repo, 'commit', '--quiet', '-m', 'initial')

  return { root, repo, cleanup: () => rmSync(root, { recursive: true, force: true }) }
}

function installStub(dir) {
  const bin = join(dir, 'node_modules/.bin')
  mkdirSync(bin, { recursive: true })
  // .mjs, so node reads the stub as a module whatever the package.json above it says.
  const stub = join(dir, 'node_modules/.bin/eslint.mjs')
  writeFileSync(stub, stubSource)
  writeFileSync(join(bin, 'eslint'), `#!/bin/sh\nexec node "${stub}" "$@"\n`)
  chmodSync(join(bin, 'eslint'), 0o755)
}

/** Touch line 3, the line the stub's default position falls on. */
function touchLineThree(repo, file = 'src/a.js') {
  writeFileSync(join(repo, file), 'export const a = 1\nexport const b = 2\nconst x = 4\n')
}

/** @returns {{ status: number, stdout: string, stderr: string }} */
function lint(cwd, f, { args = ['--since', 'HEAD'], env = {} } = {}) {
  const result = spawnSync(script, ['--only', 'ts', ...args], {
    cwd,
    encoding: 'utf8',
    env: {
      ...process.env,
      TVRMSMITH_REGISTRY_KEY: undefined,
      // Both under the fixture's own root, so a case neither reads the developer's waiver log nor
      // writes a lint-changed binary into their real cache.
      TVRMSMITH_WAIVERS: join(f.root, 'waivers.jsonl'),
      XDG_CACHE_HOME: join(f.root, 'cache'),
      ...env,
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  return { status: result.status, stdout: result.stdout, stderr: result.stderr }
}

test('an ESLint error on a changed line blocks the commit', { skip }, () => {
  const f = fixture()
  try {
    touchLineThree(f.repo)

    const { status, stdout, stderr } = lint(f.repo, f)
    assert.equal(status, 2, `expected the blocking exit code\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    // Exactly the porcelain line and nothing else. stdout is what no-mistakes reads with one
    // regex across every language, so a package header or a stylish report leaking onto it is a
    // bug, not noise.
    assert.match(stdout, /^src\/a\.js:\d+:\d+: no-unused-vars: .+$/m)
    assert.equal(stdout.trimEnd().split('\n').length, 1, `expected one line, got:\n${stdout}`)
    // The waive command is not a finding, so it lives on stderr.
    assert.doesNotMatch(stdout, /waive/)
    assert.match(stderr, /waive --language ts/)
  } finally {
    f.cleanup()
  }
})

test('an ESLint WARNING on a changed line blocks the commit, which it did not before', { skip }, () => {
  const f = fixture()
  try {
    touchLineThree(f.repo)

    // The behaviour change of this slice. Severity 1 used to leave ESLint's exit at 0 and the
    // branch reported it and let the commit through; ADR 0010 blocks on severity 1 and 2 alike,
    // and the verdict is lint-changed's now rather than ESLint's exit code.
    const { status, stdout, stderr } = lint(f.repo, f, { env: { STUB_SEVERITY: '1' } })
    assert.equal(status, 2, `a warning must block\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    assert.match(stdout, /^src\/a\.js:\d+:\d+: no-unused-vars: .+$/m)
  } finally {
    f.cleanup()
  }
})

test('a finding on a line the change never touched lets the commit through', { skip }, () => {
  const f = fixture()
  try {
    touchLineThree(f.repo)

    // Line 1 is committed and untouched. Scoping is the whole reason this branch can block at
    // all: without it, one line written in a legacy file hands back its backlog.
    const { status, stdout, stderr } = lint(f.repo, f, { env: { STUB_LINE: '1' } })
    assert.equal(status, 0, `expected a clean pass\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    assert.equal(stdout, '')
  } finally {
    f.cleanup()
  }
})

test('an ESLint run that broke fails the gate rather than blocking on a finding', { skip }, () => {
  const f = fixture()
  try {
    touchLineThree(f.repo)

    // 2 is ESLint's own code for a fatal config error, and it writes no report at all, so it says
    // nothing about the code it never read. 1 and not 2: a hook told 2 would report a finding
    // nobody found.
    const { status, stdout, stderr } = lint(f.repo, f, { env: { STUB_EXIT: '2' } })
    assert.equal(status, 1, `expected the gate to break\nstdout:\n${stdout}\nstderr:\n${stderr}`)
  } finally {
    f.cleanup()
  }
})

/** One repository holding two ESLint packages, `a` and `b`, neither at the root. */
function twoPackageFixture() {
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'tvrmsmith-ts-'))
  const repo = join(root, 'repo')

  git(root, 'init', '--quiet', '--initial-branch=main', repo)
  git(repo, 'config', 'user.email', 'test@example.com')
  git(repo, 'config', 'user.name', 'Test')
  git(repo, 'config', 'commit.gpgsign', 'false')
  for (const name of ['a', 'b']) {
    mkdirSync(join(repo, name, 'src'), { recursive: true })
    writeFileSync(join(repo, name, 'package.json'), `{"name":"${name}","type":"module"}\n`)
    writeFileSync(join(repo, name, 'eslint.config.js'), 'export default []\n')
    writeFileSync(join(repo, name, 'src/a.js'), 'export const a = 1\nexport const b = 2\nconst x = 3\n')
    installStub(join(repo, name))
  }
  git(repo, 'add', '.')
  git(repo, 'commit', '--quiet', '-m', 'initial')
  for (const name of ['a', 'b']) touchLineThree(repo, `${name}/src/a.js`)

  return { root, repo, cleanup: () => rmSync(root, { recursive: true, force: true }) }
}

test('two packages report through one filter run', { skip }, () => {
  const f = twoPackageFixture()
  try {
    const { status, stdout, stderr } = lint(f.repo, f)
    assert.equal(status, 2, `expected the blocking exit code\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    // Both, from one lint-changed process over both reports. A process per package spends a waiver
    // on the first package's finding while the second still blocks the commit, so the permission
    // is burnt on a commit that never went through.
    assert.match(stdout, /^a\/src\/a\.js:\d+:\d+: no-unused-vars: /m)
    assert.match(stdout, /^b\/src\/a\.js:\d+:\d+: no-unused-vars: /m)
  } finally {
    f.cleanup()
  }
})

test('a staged file whose disk copy differs stops the run and names it', { skip }, () => {
  const f = fixture()
  try {
    touchLineThree(f.repo)
    git(f.repo, 'add', 'src/a.js')
    // A third state on disk. ESLint reads disk and the commit carries the index, so the whole
    // report would describe code that is not being committed. ADR 0010 makes that a hard stop
    // before any finding is judged, which is why this branch no longer feeds ESLint on stdin.
    writeFileSync(join(f.repo, 'src/a.js'), 'export const a = 1\nexport const b = 2\nconst x = 5\n')

    const { status, stdout, stderr } = lint(f.repo, f, { args: ['--staged'] })
    assert.equal(status, 1, `expected the gate to stop\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    assert.match(stderr, /src\/a\.js/)
  } finally {
    f.cleanup()
  }
})

test('a staged file deleted from the working tree stops the commit', { skip }, () => {
  const f = fixture()
  try {
    touchLineThree(f.repo)
    git(f.repo, 'add', 'src/a.js')
    rmSync(join(f.repo, 'src/a.js'))

    // ESLint reads disk and the commit carries the index, so no package reaches a report at all
    // and the branch has nothing to hand the filter. ADR 0010's staged-versus-disk hard stop is
    // asked anyway, across every staged path, or this commit would ship content nothing linted.
    const { status, stdout, stderr } = lint(f.repo, f, { args: ['--staged'] })
    assert.equal(status, 1, `expected the gate to stop\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    assert.match(stderr, /src\/a\.js/)
  } finally {
    f.cleanup()
  }
})

test('a package with no installed eslint fails rather than passing its files unlinted', { skip }, () => {
  const f = fixture()
  try {
    // Committed, so the only changed file the run sees is src/a.js.
    rmSync(join(f.repo, 'node_modules'), { recursive: true })
    git(f.repo, 'add', '-A')
    git(f.repo, 'commit', '--quiet', '-m', 'uninstall')
    touchLineThree(f.repo)

    // The package carries an ESLint config, so it is adopted and not_wired does not cover it.
    // Skipping it left every changed file in it unlinted and the commit passing clean, which is
    // the one verdict a gate must never produce. 1 and not 2: nothing was found, nothing ran.
    const { status, stdout, stderr } = lint(f.repo, f)
    assert.equal(status, 1, `expected the gate to refuse\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    assert.match(stderr, /no installed eslint/)
    assert.equal(stdout, '')
  } finally {
    f.cleanup()
  }
})

test('a file the package ignores does not block', { skip }, () => {
  const f = fixture()
  try {
    touchLineThree(f.repo)

    // Without --no-warn-ignored, ESLint 9 reports `{"ruleId":null,"severity":1,"message":"File
    // ignored because of a matching ignore pattern"}` with no line at exit 0. The parser reads a
    // null ruleId with no line as UNPARSED with IgnoresScope, which blocks every commit touching
    // an ignored file and is waivable only by a path-less master key. The flag kills it at source.
    const { status, stdout, stderr } = lint(f.repo, f, { env: { STUB_IGNORED: 'src/a.js' } })
    assert.equal(status, 0, `an ignored file must not block\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    assert.equal(stdout, '')
  } finally {
    f.cleanup()
  }
})
