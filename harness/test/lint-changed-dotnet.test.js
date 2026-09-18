/**
 * The C# branch end to end: a staged C# warning on a changed line blocks the commit.
 *
 * errorlog-props.test.js drives errorlog.props by setting CustomAfterMicrosoftCommonTargets and
 * TvrmsmithSarifPrefix by hand, so it says nothing about whether the script passes those. That
 * wiring is the whole .NET half: point it at the wrong file, or spell either property
 * differently, and the build writes no report the script can find, so the gate exits 1 or 0 and
 * no finding is ever named.
 *
 * A plain compiler warning (CS0219) is enough. Decision 1 in the intent is that the warning set
 * is everything the tool reports, so the analyzer package is not needed to prove the path.
 *
 * Skipped where there is no dotnet or no go, since the script itself skips or fails on those.
 */
import { execFileSync, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, test } from 'node:test'
import assert from 'node:assert/strict'

const harness = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..'))
const hub = dirname(harness)
const script = join(harness, 'lint-changed.sh')
const analyzerProps = join(hub, 'dotnet', 'artifacts', 'local', 'Tvrmsmith.Analyzers.Local.props')
const missing = [['dotnet', '--version'], ['go', 'version']].find(
  ([cmd, ...args]) => spawnSync(cmd, args, { stdio: 'ignore' }).status !== 0,
)?.[0]

function git(cwd, ...args) {
  return execFileSync('git', args, { cwd, encoding: 'utf8' })
}

/**
 * A repository holding one project, wired for .NET through the registry the script reads, with
 * an unwarned version of Foo.cs committed. Everything it writes lives under its own temp root,
 * the analyzer props included: a stub left behind in the checkout by an interrupted run would
 * satisfy the script's own check on every later commit there and build with no analyzers loaded,
 * which is the silent pass this slice exists to close.
 */
function fixture() {
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'tvrmsmith-dotnet-'))
  const repo = join(root, 'repo')
  const registry = join(root, 'coding-standards.props')
  const localProps = join(root, 'Tvrmsmith.Analyzers.Local.props')

  git(root, 'init', '--quiet', '--initial-branch=main', repo)
  git(repo, 'config', 'user.email', 'test@example.com')
  git(repo, 'config', 'user.name', 'Test')
  git(repo, 'config', 'commit.gpgsign', 'false')
  mkdirSync(join(repo, 'src'))
  writeFileSync(
    join(repo, 'src', 'App.csproj'),
    '<Project Sdk="Microsoft.NET.Sdk">\n  <PropertyGroup><TargetFramework>net8.0</TargetFramework></PropertyGroup>\n</Project>\n',
  )
  writeFileSync(join(repo, 'src', 'Foo.cs'), 'public class Foo { public void M() { } }\n')
  git(repo, 'add', '.')
  git(repo, 'commit', '--quiet', '-m', 'initial')

  // The script's registry is the path-scoped Import in the props file, matched by its condition.
  writeFileSync(registry, `<Project>\n  <!-- StartsWith('${realpathSync(repo)}/') -->\n</Project>\n`)

  // The real props are read where bootstrap has written them, so a machine that has them runs
  // against the analyzers the hook really loads. Where it has not, an empty import is enough:
  // CS0219 is the compiler's own warning and needs no analyzer package.
  if (!existsSync(analyzerProps)) writeFileSync(localProps, '<Project />\n')

  return {
    root,
    repo,
    registry,
    localProps: existsSync(analyzerProps) ? analyzerProps : localProps,
    cleanup: () => rmSync(root, { recursive: true, force: true }),
  }
}

/** Runs the script over the staged change. @returns {{ status: number, stdout: string, stderr: string }} */
function lint(f) {
  const result = spawnSync(script, ['--only', 'dotnet', '--staged'], {
    cwd: f.repo,
    encoding: 'utf8',
    env: {
      ...process.env,
      TVRMSMITH_ANALYZER_PROPS: f.registry,
      TVRMSMITH_ANALYZER_LOCAL_PROPS: f.localProps,
      TVRMSMITH_WAIVERS: join(f.root, 'waivers.jsonl'),
      XDG_CACHE_HOME: join(f.root, 'cache'),
    },
  })
  return { status: result.status, stdout: result.stdout, stderr: result.stderr }
}

describe('lint-changed.sh --only dotnet', () => {
  test('a warning on a staged line blocks the commit', { skip: missing && `no ${missing} on PATH` }, () => {
    const f = fixture()
    try {
      writeFileSync(join(f.repo, 'src', 'Foo.cs'), 'public class Foo { public void M() { int x = 1; } }\n')
      git(f.repo, 'add', 'src/Foo.cs')

      const { status, stdout, stderr } = lint(f)
      assert.equal(status, 2, `expected the blocking exit code\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stdout, /CS0219/)
      assert.match(stdout, /src\/Foo\.cs/)
      // Decision 4: the printed waive command is the only route past a false positive.
      assert.match(stdout, /waive/)
    } finally {
      f.cleanup()
    }
  })

  // The commit carries Foo.cs, MSBuild has nothing on disk to compile, and the build set is empty.
  // Exiting there passed the commit with nothing examined, which is the staged-versus-disk hard
  // stop of intent decision 3 going unasked.
  test('a staged file deleted from the working tree stops the commit', { skip: missing && `no ${missing} on PATH` }, () => {
    const f = fixture()
    try {
      writeFileSync(join(f.repo, 'src', 'Foo.cs'), 'public class Foo { public void M() { int x = 1; } }\n')
      git(f.repo, 'add', 'src/Foo.cs')
      rmSync(join(f.repo, 'src', 'Foo.cs'))

      const { status, stdout, stderr } = lint(f)
      assert.notEqual(status, 0, `expected the commit to be refused\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stderr, /src\/Foo\.cs/)
    } finally {
      f.cleanup()
    }
  })

  // The staged content and the working-tree content of the same file disagree. MSBuild compiles
  // disk and the commit carries the index, so intent decision 3 refuses the commit rather than
  // reporting on code it does not contain. The file is on disk here, so the build set is not
  // empty and the check is lint-changed's own, not the absent-from-disk branch above.
  test('a staged file whose working tree copy differs stops the commit', { skip: missing && `no ${missing} on PATH` }, () => {
    const f = fixture()
    try {
      writeFileSync(join(f.repo, 'src', 'Foo.cs'), 'public class Foo { public void M() { int x = 1; } }\n')
      git(f.repo, 'add', 'src/Foo.cs')
      writeFileSync(join(f.repo, 'src', 'Foo.cs'), 'public class Foo { public void M() { int y = 2; } }\n')

      const { status, stdout, stderr } = lint(f)
      assert.equal(status, 1, `expected the divergence hard stop\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stdout + stderr, /re-stage/)
      assert.match(stdout + stderr, /src\/Foo\.cs/)
    } finally {
      f.cleanup()
    }
  })

  // Reaching here means the registry already said the repo is wired for .NET, so absent props are a
  // broken install. Building without the analyzers reports clean on a compilation nothing inspected.
  test('missing analyzer props refuse the commit', { skip: missing && `no ${missing} on PATH` }, () => {
    const f = fixture()
    try {
      writeFileSync(join(f.repo, 'src', 'Foo.cs'), 'public class Foo { public void M() { int x = 1; } }\n')
      git(f.repo, 'add', 'src/Foo.cs')
      f.localProps = join(f.root, 'never-bootstrapped.props')

      const { status, stdout, stderr } = lint(f)
      assert.equal(status, 1, `expected the broken-install exit\nstdout:\n${stdout}\nstderr:\n${stderr}`)
      assert.match(stderr, /bootstrap dotnet/)
    } finally {
      f.cleanup()
    }
  })

  test('an unwarned staged line lets the commit through', { skip: missing && `no ${missing} on PATH` }, () => {
    const f = fixture()
    try {
      writeFileSync(
        join(f.repo, 'src', 'Foo.cs'),
        'namespace Fixture;\n\npublic static class Foo\n{\n    public static int M() => 1;\n}\n',
      )
      git(f.repo, 'add', 'src/Foo.cs')

      const { status, stdout, stderr } = lint(f)
      assert.equal(status, 0, `expected a clean pass\nstdout:\n${stdout}\nstderr:\n${stderr}`)
    } finally {
      f.cleanup()
    }
  })
})
