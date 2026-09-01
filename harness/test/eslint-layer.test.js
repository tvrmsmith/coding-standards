/**
 * The typed layer is opt-in per package, and the wrapper decides for itself which packages get
 * it. Getting that wrong is not a subtle wrong: a type-aware rule with no program throws and
 * takes the whole lint run down, so the detector and its override are gated here.
 *
 * Each case runs in a child process, because the wrapper reads `process.cwd()` and the
 * environment once, at module load.
 */
import { execFileSync } from 'node:child_process'
import { mkdtempSync, realpathSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test, { describe } from 'node:test'
import assert from 'node:assert/strict'

const harness = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..'))
const layer = join(harness, 'eslint-layer.js')

const PROBE = `
const config = (await import(process.env.LAYER)).default
console.log(config.some((entry) => entry?.name === 'tvrmsmith/typed/typescript'))
`

/**
 * @param {string} eslintConfig source of the package's own `eslint.config.js`
 * @param {Record<string, string>} env extra environment for the child
 * @returns {boolean} whether the wrapper included the typed layer
 */
function typedLayerOn(eslintConfig, env = {}) {
  const pkg = mkdtempSync(join(realpathSync(tmpdir()), 'tvrmsmith-layer-'))
  writeFileSync(join(pkg, 'eslint.config.js'), eslintConfig)
  const stdout = execFileSync(process.execPath, ['--input-type=module', '-e', PROBE], {
    cwd: pkg,
    encoding: 'utf8',
    env: { ...process.env, LAYER: layer, ...env },
  })
  return stdout.trim() === 'true'
}

const withProjectService = `export default [{ languageOptions: { parserOptions: { projectService: true } } }]`
const withProject = `export default [{ languageOptions: { parserOptions: { project: './tsconfig.json' } } }]`
const withoutTypes = `export default [{ rules: {} }]`

describe('the typed layer gate', () => {
  test('is on where the package configures projectService', () => {
    assert.equal(typedLayerOn(withProjectService), true)
  })

  test('is on for the older per-tsconfig `project` spelling', () => {
    assert.equal(typedLayerOn(withProject), true)
  })

  test('is off where the package lints without type information', () => {
    assert.equal(typedLayerOn(withoutTypes), false)
  })

  test('TVRMSMITH_TYPED_LINT=0 skips the layer in a package that could take it', () => {
    assert.equal(typedLayerOn(withProjectService, { TVRMSMITH_TYPED_LINT: '0' }), false)
  })

  test('TVRMSMITH_TYPED_LINT=1 demands the layer in a package the detector rejects', () => {
    assert.equal(typedLayerOn(withoutTypes, { TVRMSMITH_TYPED_LINT: '1' }), true)
  })

  test('an empty TVRMSMITH_TYPED_LINT reads as unset, not as off', () => {
    // An exported-but-blank variable is a normal shell accident, and reading it as `0` would
    // silently drop the layer everywhere.
    assert.equal(typedLayerOn(withProjectService, { TVRMSMITH_TYPED_LINT: '' }), true)
  })
})
