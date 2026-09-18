/**
 * lint-changed-dotnet.sh end to end: a staged C# warning on a changed line blocks the commit.
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
import test from 'node:test'
import assert from 'node:assert/strict'

const harness = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..'))
const hub = dirname(harness)
const script = join(harness, 'lint-changed-dotnet.sh')
const analyzerProps = join(hub, 'dotnet', 'artifacts', 'local', 'Tvrmsmith.Analyzers.Local.props')
const missing = [['dotnet', '--version'], ['go', 'version']].find(
  ([cmd, ...args]) => spawnSync(cmd, args, { stdio: 'ignore' }).status !== 0,
)?.[0]

function git(cwd, ...args) {
  return execFileSync('git', args, { cwd, encoding: 'utf8' })
}

/**
 * A repository holding one project, wired for .NET through the registry the script reads, with
 * an unwarned version of Foo.cs committed. Also materialises the analyzer props the script
 * insists on, which bootstrap would otherwise have written; the cleanup removes it only when
 * this fixture created it.
 */
function fixture() {
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'tvrmsmith-dotnet-'))
  const repo = join(root, 'repo')
  const registry = join(root, 'coding-standards.props')

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

  const borrowedAnalyzerProps = existsSync(analyzerProps)
  const artifacts = join(hub, 'dotnet', 'artifacts')
  const borrowedArtifacts = existsSync(artifacts)
  if (!borrowedAnalyzerProps) {
    mkdirSync(dirname(analyzerProps), { recursive: true })
    writeFileSync(analyzerProps, '<Project />\n')
  }

  return {
    root,
    repo,
    registry,
    cleanup: () => {
      rmSync(root, { recursive: true, force: true })
      if (!borrowedAnalyzerProps) rmSync(analyzerProps, { force: true })
      if (!borrowedArtifacts) rmSync(artifacts, { recursive: true, force: true })
    },
  }
}

/** Runs the script over the staged change. @returns {{ status: number, stdout: string, stderr: string }} */
function lint(f) {
  const result = spawnSync(script, ['--staged'], {
    cwd: f.repo,
    encoding: 'utf8',
    env: {
      ...process.env,
      TVRMSMITH_ANALYZER_PROPS: f.registry,
      TVRMSMITH_WAIVERS: join(f.root, 'waivers.jsonl'),
      XDG_CACHE_HOME: join(f.root, 'cache'),
    },
  })
  return { status: result.status, stdout: result.stdout, stderr: result.stderr }
}

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
