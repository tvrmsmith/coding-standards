/**
 * errorlog.props, driven through its only consumer: a real `dotnet build`.
 *
 * The file is the contract between lint-changed.sh's C# branch and the compiler — it names the SARIF
 * report the changed-line filter then reads. Both halves of that name were wrong when written
 * on the command line instead, and neither failure is visible without building: MSBuild left
 * `$(TargetFramework)` literal, so a multi-targeted project's frameworks all wrote to one file,
 * and it swallowed the `,version=2.1` suffix, so Roslyn wrote SARIF 1.0.0, which lintfind
 * rejects as malformed JSON and which failed every C# commit.
 *
 * Skipped where there is no dotnet, since there is nothing to ask without it.
 */
import { execFileSync, spawnSync } from 'node:child_process'
import { mkdtempSync, mkdirSync, readFileSync, readdirSync, realpathSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, test } from 'node:test'
import assert from 'node:assert/strict'

const harness = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..'))
const props = join(harness, 'errorlog.props')
const noDotnet = spawnSync('dotnet', ['--version'], { stdio: 'ignore' }).status !== 0

/** Builds a project with the given frameworks and returns the reports by file name. */
function build(frameworks) {
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'tvrmsmith-errorlog-'))
  try {
    const element = frameworks.length > 1 ? 'TargetFrameworks' : 'TargetFramework'
    mkdirSync(join(root, 'src'))
    writeFileSync(
      join(root, 'src', 'App.csproj'),
      `<Project Sdk="Microsoft.NET.Sdk">\n  <PropertyGroup><${element}>${frameworks.join(';')}</${element}></PropertyGroup>\n</Project>\n`,
    )
    // CS0219 on the only line, so every framework's report carries a result to read.
    writeFileSync(join(root, 'src', 'Foo.cs'), 'public class Foo { public void M() { int x = 1; } }\n')

    execFileSync('dotnet', ['build', 'src/App.csproj', '--no-incremental', `-p:TvrmsmithSarifPrefix=${join(root, 'out')}`, '-v:q', '--nologo'], {
      cwd: root,
      encoding: 'utf8',
      env: { ...process.env, CustomAfterMicrosoftCommonTargets: props },
      stdio: ['ignore', 'pipe', 'pipe'],
    })

    const reports = {}
    for (const name of readdirSync(root).filter((n) => n.endsWith('.sarif'))) {
      reports[name] = JSON.parse(readFileSync(join(root, name), 'utf8'))
    }
    return reports
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
}

describe('errorlog.props', () => {
  test('each target framework gets its own report', { skip: noDotnet && 'no dotnet on PATH' }, () => {
    const reports = build(['net8.0', 'netstandard2.0'])
    assert.deepEqual(Object.keys(reports).sort(), ['out.net8.0.sarif', 'out.netstandard2.0.sarif'])
  })

  test('the report is SARIF 2.1, the only version the filter reads', { skip: noDotnet && 'no dotnet on PATH' }, () => {
    const reports = build(['net8.0'])
    const report = reports['out.net8.0.sarif']
    assert.equal(report.version, '2.1.0')
    // The 2.1 message shape, which is what 1.0.0 fails on: a string there is the malformed-JSON
    // error the filter reported against every real build.
    const result = report.runs[0].results.find((r) => r.ruleId === 'CS0219')
    assert.equal(typeof result.message.text, 'string')
  })
})
