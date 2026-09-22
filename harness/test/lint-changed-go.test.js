/**
 * The Go branch of lint-changed.sh: which repository path it decides to look up, and whether a
 * golangci-lint finding on a changed line stops the commit.
 *
 * The linter is stubbed. What is under test is the script's own wiring — the registry lookup, the
 * flags it hands golangci, and the one `lint-changed` run it folds every module's report into —
 * not what golangci says about a file, and stubbing it keeps the cases at a git fixture and a
 * JSON file instead of a real Go linter.
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
 * Stands in for the personal golangci-lint binary. It writes one issue to the file named by
 * `--output.json.path`, at the line and column the case asks for, and exits `STUB_EXIT`.
 *
 * Reading the flags out of argv is deliberate coupling: pinning the spelling go.sh actually
 * passes is half of what these cases are for, and every flag checked here is one whose loss is
 * invisible to a case that reports a single issue.
 *
 * `--output.json.path FILE` in the space-separated form, because `--output.json.path=FILE` or the
 * report going to stdout leaves `out` empty and would let the branch report a clean run on a
 * report nobody wrote. `--show-stats=false`, because the stats summary real golangci prints
 * otherwise lands straight in the porcelain stream. `--max-issues-per-linter 0` and
 * `--max-same-issues 0`, because golangci's defaults of 50 and 3 truncate silently, which is the
 * silent drop the design exists to prevent. `--path-mode abs`, because a relative path resolves
 * against the wrong directory in a monorepo submodule.
 *
 * The filename is absolute under $PWD because go.sh runs the linter from inside the module, which
 * is also the directory --path-mode abs makes golangci's own paths absolute against.
 */
const stubSource = `#!/bin/sh
out=
path_mode=
show_stats=
per_linter=
same_issues=
while [ $# -gt 0 ]; do
  case "$1" in
    --output.json.path) out=$2; shift 2 ;;
    --path-mode) path_mode=$2; shift 2 ;;
    --show-stats=*) show_stats=\${1#--show-stats=}; shift ;;
    --max-issues-per-linter) per_linter=$2; shift 2 ;;
    --max-same-issues) same_issues=$2; shift 2 ;;
    *) shift ;;
  esac
done

[ "\${STUB_EXIT:-0}" -eq 0 ] || exit "$STUB_EXIT"

if [ -z "$out" ]; then
  echo "stub: no --output.json.path in argv" >&2
  exit 9
fi

require() {
  [ "$2" = "$3" ] && return 0
  echo "stub: wanted $1 $3 in argv, got '$2'" >&2
  exit 9
}
require --path-mode "$path_mode" abs
require --show-stats "$show_stats" false
require --max-issues-per-linter "$per_linter" 0
require --max-same-issues "$same_issues" 0

cat >"$out" <<JSON
{"Issues":[{"FromLinter":"stub","Text":"stub finding","Pos":{"Filename":"$PWD/main.go","Line":\${STUB_LINE:-3},"Column":\${STUB_COLUMN:-2}}}]}
JSON
`

/**
 * A repository with one committed .go file, a linked worktree, a stub linter and an empty
 * registry. main.go is three lines, so a case changing line 3 has a line the stub's default
 * position falls on and line 1 is a line it never touched.
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

  writeFileSync(stub, stubSource)
  chmodSync(stub, 0o755)
  writeFileSync(registry, '')

  return { root, repo, worktree, registry, stub, cleanup: () => rmSync(root, { recursive: true, force: true }) }
}

/**
 * `TVRMSMITH_REGISTRY_KEY` is honoured whenever it is set, so an ambient value on the developer's
 * machine would decide which path every case below looks up, and the cases asserting a skip would
 * pass for the wrong reason. `undefined` removes it from the child's environment.
 */
const neutralised = { TVRMSMITH_REGISTRY_KEY: undefined, TVRMSMITH_GOLANGCI_CONFIG: undefined }

/** @returns {{ status: number, stdout: string, stderr: string }} */
function lint(cwd, { root, registry, stub }, env = {}, args = ['--since', 'HEAD']) {
  const result = spawnSync(script, ['--only', 'go', ...args], {
    cwd,
    encoding: 'utf8',
    env: {
      ...process.env,
      ...neutralised,
      TVRMSMITH_GO_REPOS: registry,
      TVRMSMITH_GCL: stub,
      // Both under the fixture's own root, so a case neither reads the developer's waiver log nor
      // writes a lint-changed binary into their real cache.
      TVRMSMITH_WAIVERS: join(root, 'waivers.jsonl'),
      XDG_CACHE_HOME: join(root, 'cache'),
      ...env,
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  return { status: result.status, stdout: result.stdout, stderr: result.stderr }
}

/**
 * A checkout that resolves to no registered repository on its own: its own git dir, so
 * `--git-common-dir` names it rather than anything in the registry. Stands in for the detached
 * worktree of the daemon's bare gate repo that a no-mistakes run lints in.
 */
function detachedCheckout(f) {
  const elsewhere = join(f.root, 'gate-worktree')
  git(f.root, 'init', '--quiet', '--initial-branch=main', elsewhere)
  git(elsewhere, 'config', 'user.email', 'test@example.com')
  git(elsewhere, 'config', 'user.name', 'Test')
  git(elsewhere, 'config', 'commit.gpgsign', 'false')
  writeFileSync(join(elsewhere, 'go.mod'), 'module example.test\n\ngo 1.26.0\n')
  writeFileSync(join(elsewhere, 'main.go'), 'package main\n\nfunc main() {}\n')
  git(elsewhere, 'add', '.')
  git(elsewhere, 'commit', '--quiet', '-m', 'initial')
  writeFileSync(join(elsewhere, 'main.go'), 'package main\n\nfunc main() { _ = 4 }\n')
  return elsewhere
}

test('a finding on a changed line blocks the commit', { skip }, () => {
  const f = fixture()
  try {
    writeFileSync(f.registry, `${f.repo}\n`)
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')

    const { status, stdout, stderr } = lint(f.repo, f)
    assert.equal(status, 2, `expected the blocking exit code\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    // Exactly the porcelain line and nothing else. stdout is what no-mistakes reads with one
    // regex across every language, so a human summary leaking onto it is a bug, not noise.
    assert.equal(stdout, 'main.go:3:2: stub: stub finding\n')
  } finally {
    f.cleanup()
  }
})

test('a finding on a line the change never touched lets the commit through', { skip }, () => {
  const f = fixture()
  try {
    writeFileSync(f.registry, `${f.repo}\n`)
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')

    // Line 1 is `package main`, committed and untouched. Scoping is the whole reason this branch
    // can block at all: without it, one line written in a legacy file hands back its backlog.
    const { status, stdout, stderr } = lint(f.repo, f, { STUB_LINE: '1', STUB_COLUMN: '1' })
    assert.equal(status, 0, `expected a clean pass\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    assert.doesNotMatch(stdout, /stub finding/)
  } finally {
    f.cleanup()
  }
})

test('a golangci run that broke fails the gate rather than blocking on a finding', { skip }, () => {
  const f = fixture()
  try {
    writeFileSync(f.registry, `${f.repo}\n`)
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')

    // 1 and not 2. A golangci run that broke wrote no report, so it says nothing at all about the
    // code it never read, and a hook told 2 would report a finding nobody found.
    const { status, stdout, stderr } = lint(f.repo, f, { STUB_EXIT: '3' })
    assert.equal(status, 1, `expected the gate to break\nstdout:\n${stdout}\nstderr:\n${stderr}`)
  } finally {
    f.cleanup()
  }
})

/**
 * One repository holding two modules, `a` and `b`, neither at the root. golangci-lint runs once
 * per module, so this is the shape that produces two reports from one commit.
 */
function twoModuleFixture() {
  const f = fixture()
  for (const name of ['a', 'b']) {
    mkdirSync(join(f.repo, name))
    writeFileSync(join(f.repo, name, 'go.mod'), `module example.test/${name}\n\ngo 1.26.0\n`)
    writeFileSync(join(f.repo, name, 'main.go'), 'package main\n\nfunc main() {}\n')
  }
  git(f.repo, 'add', '.')
  git(f.repo, 'commit', '--quiet', '-m', 'two modules')
  for (const name of ['a', 'b']) {
    writeFileSync(join(f.repo, name, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')
  }
  writeFileSync(f.registry, `${f.repo}\n`)
  return f
}

test('two modules report through one filter run', { skip }, () => {
  const f = twoModuleFixture()
  try {
    const { status, stdout, stderr } = lint(f.repo, f)
    assert.equal(status, 2, `expected the blocking exit code\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    // Both, from one lint-changed process over both reports. A process per module spends a waiver
    // on the first module's finding while the second still blocks the commit, so the permission is
    // burnt on a commit that never went through.
    assert.match(stdout, /^a\/main\.go:3:2: stub: stub finding$/m)
    assert.match(stdout, /^b\/main\.go:3:2: stub: stub finding$/m)
  } finally {
    f.cleanup()
  }
})

/**
 * Asserts a clean pass that reported nothing. Both halves matter: without the changed file the
 * changed set is empty and the branch reports nothing whatever the registry said, and without the
 * status an exit 1 with empty stdout would read as a skip.
 */
function assertSkipped(result) {
  const { status, stdout, stderr } = result
  assert.equal(status, 0, `expected a clean skip\nstdout:\n${stdout}\nstderr:\n${stderr}`)
  assert.doesNotMatch(stdout, /stub finding/)
}

test('a staged file deleted from the working tree stops the commit', { skip }, () => {
  const f = fixture()
  try {
    writeFileSync(f.registry, `${f.repo}\n`)
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')
    git(f.repo, 'add', 'main.go')
    rmSync(join(f.repo, 'main.go'))

    // golangci reads disk and the commit carries the index, so no module reaches a report at all
    // and the branch has nothing to hand the filter. ADR 0010's staged-versus-disk hard stop is
    // asked anyway, across every staged path, or this commit would ship content nothing linted.
    const { status, stdout, stderr } = lint(f.repo, f, {}, ['--staged'])
    assert.equal(status, 1, `expected the gate to stop\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    assert.match(stderr, /main\.go/)
  } finally {
    f.cleanup()
  }
})

test('an unregistered repository is skipped rather than linted', { skip }, () => {
  const f = fixture()
  try {
    // The change the sibling case below is linted for. Without it there is nothing for the
    // registry lookup to gate, and the case would pass with the lookup deleted.
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')
    assertSkipped(lint(f.repo, f))
  } finally {
    f.cleanup()
  }
})

test('a registered repository is linted', { skip }, () => {
  const f = fixture()
  try {
    writeFileSync(f.registry, `${f.repo}\n`)
    writeFileSync(join(f.repo, 'main.go'), 'package main\n\nfunc main() { _ = 1 }\n')
    assert.match(lint(f.repo, f).stdout, /stub finding/)
  } finally {
    f.cleanup()
  }
})

test('a linked worktree is covered by the main checkout being registered', { skip }, () => {
  const f = fixture()
  try {
    // Only the main checkout is listed, which is what `bootstrap go` writes even when it is
    // pointed at a worktree. The hook is shared between them, so keying on the worktree's own
    // path would make it fire and skip.
    writeFileSync(f.registry, `${f.repo}\n`)
    writeFileSync(join(f.worktree, 'main.go'), 'package main\n\nfunc main() { _ = 2 }\n')
    assert.match(lint(f.worktree, f).stdout, /stub finding/)
  } finally {
    f.cleanup()
  }
})

test('a worktree of an unregistered repository is still skipped', { skip }, () => {
  const f = fixture()
  try {
    writeFileSync(join(f.worktree, 'main.go'), 'package main\n\nfunc main() { _ = 3 }\n')
    assertSkipped(lint(f.worktree, f))
  } finally {
    f.cleanup()
  }
})

test('TVRMSMITH_REGISTRY_KEY names the checkout a detached worktree cannot resolve to', { skip }, () => {
  const f = fixture()
  try {
    writeFileSync(f.registry, `${f.repo}\n`)
    const gate = detachedCheckout(f)
    // Resolving for itself, this checkout is in no registry, and the skip reads as clean.
    assertSkipped(lint(gate, f))
    assert.match(lint(gate, f, { TVRMSMITH_REGISTRY_KEY: f.repo }).stdout, /stub finding/)
  } finally {
    f.cleanup()
  }
})

test('an override naming an unregistered path still skips', { skip }, () => {
  const f = fixture()
  try {
    // The override moves which path is looked up. It does not bypass the lookup. Without this
    // pinned, the variable is a door into linting any repo and nothing would notice the drift.
    writeFileSync(f.registry, `${f.repo}\n`)
    const gate = detachedCheckout(f)
    assertSkipped(lint(gate, f, { TVRMSMITH_REGISTRY_KEY: gate }))
  } finally {
    f.cleanup()
  }
})
