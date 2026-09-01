#!/usr/bin/env node
/**
 * Generates the built-in CA severity block, and the two lists that have to agree with it,
 * from one source: the SDK's own rule metadata plus the exclusion table below.
 *
 * Why generated rather than hand-maintained. The same set of ids has to appear in three
 * places, and the three fail differently when they drift:
 *
 *   1. config/Tvrmsmith.Analyzers.globalconfig   — the severities themselves.
 *   2. local/Tvrmsmith.Analyzers.Local.props     — WarningsNotAsErrors. An id missing here
 *      becomes a build *error* wherever the adoption target sets TreatWarningsAsErrors, which is
 *      the one outcome the injection promises never to cause.
 *   3. local/tvrmsmith-scope-changed.sh          — the ids= list. An id missing here escapes
 *      changed-files scoping and reports the whole standing backlog on every build.
 *
 * Only the first announces itself. Generating all three from one list is what keeps the two
 * silent failures from happening.
 *
 * The rule set is read from the SDK, not pinned here, so a dotnet upgrade that adds CA rules
 * picks them up instead of quietly leaving them off. Run this after an SDK bump.
 *
 *   node dotnet/generate-ca-severities.mjs          # write
 *   node dotnet/generate-ca-severities.mjs --check  # exit 1 if the files are stale
 */

import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const dotnetRoot = dirname(fileURLToPath(import.meta.url))
const config = join(dotnetRoot, 'src/Tvrmsmith.Analyzers/config/Tvrmsmith.Analyzers.globalconfig')
const props = join(dotnetRoot, 'src/Tvrmsmith.Analyzers/local/Tvrmsmith.Analyzers.Local.props')
const scope = join(dotnetRoot, 'src/Tvrmsmith.Analyzers/local/tvrmsmith-scope-changed.sh')

/** Ids this package owns, plus the four that exist only because we injected at all. */
const ours = ['TVRM0001', 'TVRM0002', 'TVRM0003', 'FAA0001', 'FAA0002', 'FAA0003', 'FAA0004']
const injectionArtifacts = ['AD0001', 'CS8032', 'CS8034', 'CS9057']

/**
 * Every off-by-default CA rule is enabled except these. Four reasons, and the distinction
 * between them is the whole argument: "fires a lot" is not on the list, because the pre-commit
 * hook scopes reporting to changed files, so a large standing backlog costs nothing.
 *
 * Measured against the adoption target (Meridian.IMR, net10.0, 608 projects) — see
 * dotfiles-6oy for the numbers behind each claim.
 */
const excluded = {
  // -- The advice is wrong for this code, not merely unwelcome. ------------------------------
  CA2007: 'ConfigureAwait: 62 Microsoft.NET.Sdk.Web projects, and ASP.NET Core has no SynchronizationContext, so the fix is a no-op',
  CA1812: 'uninstantiated internal class: the DI container instantiates them, so every hit is a false positive',
  CA2227: 'read-only collection properties break JSON deserialization targets',
  CA1002: 'do not expose List<T>: DTOs need it for deserialization',
  CA1819: 'properties returning arrays: same, fights the DTO shape',
  CA1062: 'validate public arguments: duplicates the compiler null analysis where nullable is on',
  CA5391: 'antiforgery on MVC controllers: 3 .cshtml files against 119 [ApiController], so no form surface to protect',
  CA1303: 'literals as localized parameters: nothing here is localized',
  CA1724: 'type name matches namespace: unavoidable and harmless in a monorepo this shape',
  CA1515: 'make public types internal: aimed at apps, wrong for cross-project consumption',

  // -- Obsolete advice. ----------------------------------------------------------------------
  CA1014: 'CLSCompliant attribute: dead concern for modern C#',
  CA1017: 'ComVisible attribute: same',
  CA2235: 'non-serializable fields: legacy ISerializable',
  CA2237: 'mark ISerializable types: legacy ISerializable',

  // -- Right in principle, wrong too often in practice. ---------------------------------------
  CA2000: 'dispose before losing scope: misreads ownership transfer throughout DI and builder code',

  // -- Style whose recommendation is sometimes actively wrong. --------------------------------
  // Style rules are otherwise kept: an opinion that never misfires is cheap guidance.
  CA1024: 'use properties where appropriate: a GetX() method often signals expense on purpose',
  CA1027: 'mark enums with Flags: guesses intent from values happening to be powers of two',
  CA1028: 'enum storage Int32: wrong whenever byte was chosen for a wire format',
  CA1030: 'use events where appropriate: a naming heuristic on Fire*/Raise*',
  CA1814: 'jagged over multidimensional arrays: situational',
  CA2225: 'operator named alternates: CLS/VB interop, effectively dead',
}

/** Ask the compiler which CA rules exist and which ship disabled. */
function probeSdk() {
  const dir = mkdtempSync(join(tmpdir(), 'ca-probe-'))
  try {
    writeFileSync(
      join(dir, 'p.csproj'),
      `<Project Sdk="Microsoft.NET.Sdk">\n  <PropertyGroup>\n    <TargetFramework>net10.0</TargetFramework>\n    <ErrorLog>$(MSBuildThisFileDirectory)rules.sarif,version=2.1</ErrorLog>\n  </PropertyGroup>\n</Project>\n`,
    )
    writeFileSync(join(dir, 'c.cs'), 'public class C { public void M() { } }\n')
    execFileSync('dotnet', ['build', '-v', 'q', '--nologo'], { cwd: dir, stdio: 'pipe' })

    const sarif = JSON.parse(readFileSync(join(dir, 'rules.sarif'), 'utf8'))
    const rules = sarif.runs.flatMap((run) => [
      ...(run.tool.driver.rules ?? []),
      ...(run.tool.extensions ?? []).flatMap((e) => e.rules ?? []),
    ])

    const off = new Map()
    for (const rule of rules) {
      if (!rule.id.startsWith('CA')) continue
      const { enabled = true, level = 'warning' } = rule.defaultConfiguration ?? {}
      if (enabled && level !== 'none') continue
      const text = (rule.shortDescription ?? rule.fullDescription ?? {}).text ?? ''
      off.set(rule.id, text.replace(/\s+/g, ' ').trim())
    }
    return off
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
}

const offByDefault = probeSdk()
const enabled = [...offByDefault.keys()].filter((id) => !(id in excluded)).sort()
const skipped = Object.keys(excluded).filter((id) => !offByDefault.has(id))

const START = '# --- BEGIN generated: built-in CA severities (node dotnet/generate-ca-severities.mjs) ---'
const END = '# --- END generated: built-in CA severities ---'

function severityBlock() {
  const byReason = []
  for (const id of enabled) byReason.push(`# ${id} ${offByDefault.get(id)}\ndotnet_diagnostic.${id}.severity = warning`)

  return [
    START,
    '#',
    `# The ${enabled.length} CA rules that ship *disabled* in the .NET SDK, minus ${Object.keys(excluded).length}`,
    '# exclusions. The other CA rules are on by default and are deliberately left alone: raising',
    '# them here would also let the changed-files scoping mute them, which would reduce what the',
    '# target repo already reports.',
    '#',
    '# No <Analyzer Include> backs these. The SDK already loads the analyzers; a globalconfig',
    '# configures severity by id and does not care which assembly emits the diagnostic. Setting',
    '# an explicit severity is also what *enables* a rule that ships disabled, which is why this',
    '# reaches the same rules as <AnalysisMode>All</AnalysisMode> without injecting that property.',
    '#',
    '# Warning, never error, for the same reason as every block above.',
    '#',
    '# Excluded, and why:',
    ...Object.entries(excluded).map(([id, why]) => `#   ${id}  ${why}`),
    '',
    ...byReason,
    '',
    END,
  ].join('\n')
}

function replaceBetween(text, start, end, body) {
  const from = text.indexOf(start)
  if (from === -1) return `${text.trimEnd()}\n\n${body}\n`
  const to = text.indexOf(end, from)
  if (to === -1) throw new Error(`found ${start} with no matching ${end}`)
  return text.slice(0, from) + body + text.slice(to + end.length)
}

const allIds = [...ours, ...enabled]

const edits = [
  [config, replaceBetween(readFileSync(config, 'utf8'), START, END, severityBlock())],
  [
    props,
    readFileSync(props, 'utf8').replace(
      /(<WarningsNotAsErrors>)[^<]*(<\/WarningsNotAsErrors>)/,
      `$1$(WarningsNotAsErrors);${[...allIds, ...injectionArtifacts].join(';')}$2`,
    ),
  ],
  [
    scope,
    readFileSync(scope, 'utf8').replace(
      /^ids=.*$/m,
      `ids='${allIds.join(' ')}'`,
    ),
  ],
]

if (process.argv.includes('--check')) {
  const stale = edits.filter(([path, next]) => readFileSync(path, 'utf8') !== next)
  for (const [path] of stale) console.error(`stale: ${path}`)
  console.error(stale.length ? 'run: node dotnet/generate-ca-severities.mjs' : 'up to date')
  process.exit(stale.length ? 1 : 0)
}

for (const [path, next] of edits) writeFileSync(path, next)

console.log(`off by default in this SDK: ${offByDefault.size}`)
console.log(`enabled: ${enabled.length}   excluded: ${Object.keys(excluded).length}`)
console.log(`ids carried through all three files: ${allIds.length}`)
if (skipped.length) {
  console.log(`\nexclusions that no longer match an off-by-default rule (prune them): ${skipped.join(', ')}`)
}
