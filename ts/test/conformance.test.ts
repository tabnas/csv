/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// Third-party conformance corpora, run against the DOCUMENTED stack
// (`new Tabnas().use(jsonic).use(Csv)`), exercising BOTH halves:
//
//   valid   -> must parse AND produce the corpus's expected VALUE
//   invalid -> must be REJECTED with an error
//
// The corpora are NOT committed. `scripts/fetch-csv-suites.sh` fetches them at
// pinned upstream commits into `test/suites/`, which is gitignored. `npm test`
// runs that script via the `pretest` hook, and if the corpus is still missing
// this file FAILS LOUDLY rather than skipping. A conformance test that quietly
// does not run is worse than no test at all.
//
// go/conformance_test.go runs the same two corpora, so TS and Go cannot drift
// without one of them going red.

import { describe, test } from 'node:test'
import assert from 'node:assert'
import { readFileSync, readdirSync, existsSync } from 'node:fs'
import { join } from 'node:path'

import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '../dist/csv'

// At runtime this file is loaded from `dist-test/`, so hop up one level.
const repoRoot = join(__dirname, '..', '..')
const suitesDir = join(repoRoot, 'test', 'suites')

const MISSING =
  'Conformance corpus missing under ' +
  suitesDir +
  '. Run scripts/fetch-csv-suites.sh (npm test does this for you via the ' +
  'pretest hook). This test must never skip.'

function requireDir(dir: string): string {
  if (!existsSync(dir)) throw new Error(MISSING + '\n  expected: ' + dir)
  return dir
}

function jsonEq(a: any, b: any): boolean {
  return JSON.stringify(a) === JSON.stringify(b)
}

// Report every failing case by name in one assertion, so the dial reads a
// number rather than stopping at the first red.
function report(label: string, total: number, failures: string[]) {
  const passed = total - failures.length
  assert.equal(
    failures.length,
    0,
    `${label}: ${passed}/${total} passed. FAILING (${failures.length}):\n` +
      failures.map((f) => '  - ' + f).join('\n'),
  )
}

function show(v: any): string {
  const s = JSON.stringify(v)
  return s === undefined ? String(v) : s.length > 160 ? s.slice(0, 160) + '…' : s
}

// --------------------------------------------------------------------------
// SUITE 1 — max-mapper/csv-spectrum @ d30e80f8b99d2eecb3778f1d7b9ed1cb425502ec
// Valid documents only; the corpus has no must-fail half.
// --------------------------------------------------------------------------

// One csv-spectrum case is internally inconsistent UPSTREAM, and is handled
// below by a hard assertion on the inconsistency itself rather than by an
// exclusion. See the `csv-spectrum upstream defect` test.
const SPECTRUM_UPSTREAM_DEFECT = 'location_coordinates'

function spectrumDirs() {
  const dir = requireDir(join(suitesDir, 'csv-spectrum'))
  return { csvDir: requireDir(join(dir, 'csvs')), jsonDir: requireDir(join(dir, 'json')) }
}

describe('conformance: csv-spectrum', () => {
  test('valid documents parse to the expected value', () => {
    const { csvDir, jsonDir } = spectrumDirs()

    const names = readdirSync(csvDir)
      .filter((f) => f.endsWith('.csv'))
      .map((f) => f.replace(/\.csv$/, ''))
      .sort()

    assert.ok(
      names.length > 0,
      MISSING + '\n  found no .csv documents in ' + csvDir,
    )

    const parser = new Tabnas().use(jsonic).use(Csv)
    const failures: string[] = []
    let judged = 0

    for (const name of names) {
      if (name === SPECTRUM_UPSTREAM_DEFECT) continue // judged by the next test
      judged++
      const src = readFileSync(join(csvDir, name + '.csv'), 'utf8')
      const expected = JSON.parse(readFileSync(join(jsonDir, name + '.json'), 'utf8'))
      let actual: any
      try {
        actual = parser.parse(src)
      } catch (e: any) {
        failures.push(`${name}: threw ${e.code || e.message}`)
        continue
      }
      if (!jsonEq(actual, expected)) {
        failures.push(
          `${name}: expected ${show(expected)} got ${show(actual)}`,
        )
      }
    }

    assert.equal(judged, names.length - 1, 'every corpus document must be judged')
    report('csv-spectrum valid', judged, failures)
  })

  // csv-spectrum's `location_coordinates` expectation contradicts its own
  // input in two independent ways, so it cannot be used to judge any parser:
  //
  //   1. json/location_coordinates.json is a bare OBJECT. All 11 other
  //      expectations are ARRAYS of record objects, and the .csv is a header
  //      row plus one data row, so an array of one record is the only
  //      self-consistent reading.
  //   2. The expected "Contact Phone Number" is "1234567890"; the .csv says
  //      "2095257564". The JSON was scrubbed of a real phone number and the
  //      CSV was not.
  //
  // The previous harness hid this with `if (5 === i) continue` labelled
  // "Broken test, reenable when fixed" (ts/test/csv.test.ts:350), which named
  // neither the case nor the reason and silently pointed at whatever case
  // happened to be sixth. Instead of an exclusion, the upstream defect is
  // PINNED here: if csv-spectrum ever fixes either half, this test goes red
  // and the case moves back into the judged set above.
  test('csv-spectrum upstream defect: location_coordinates contradicts itself', () => {
    const { csvDir, jsonDir } = spectrumDirs()
    const src = readFileSync(join(csvDir, SPECTRUM_UPSTREAM_DEFECT + '.csv'), 'utf8')
    const expected = JSON.parse(
      readFileSync(join(jsonDir, SPECTRUM_UPSTREAM_DEFECT + '.json'), 'utf8'),
    )

    assert.equal(
      Array.isArray(expected),
      false,
      'upstream csv-spectrum fixed the shape of location_coordinates.json — ' +
        'delete SPECTRUM_UPSTREAM_DEFECT and judge this case normally',
    )
    assert.equal(
      expected['Contact Phone Number'],
      '1234567890',
      'upstream csv-spectrum changed the scrubbed phone number — re-check',
    )
    assert.match(
      src,
      /2095257564/,
      'upstream csv-spectrum changed location_coordinates.csv — re-check',
    )

    // What @tabnas/csv actually does with it, pinned so a regression shows up:
    // the correct reading, one record, faithful to the .csv.
    const actual = new Tabnas().use(jsonic).use(Csv).parse(src)
    assert.equal(Array.isArray(actual), true)
    assert.equal((actual as any).length, 1)
    assert.equal((actual as any)[0]['Contact Phone Number'], '2095257564')
    assert.deepEqual(
      Object.keys((actual as any)[0]),
      ['Contact Phone Number', 'Location Coordinates', 'Cities', 'Counties'],
    )
  })
})

// --------------------------------------------------------------------------
// SUITE 2 — golang/go src/encoding/csv/reader_test.go @ go1.24.0
// The reference RFC 4180 implementation's own conformance table. Supplies the
// must-fail half that csv-spectrum lacks.
// --------------------------------------------------------------------------

type GoCase = {
  name: string
  input: string
  mustFail: boolean
  expected: any
  opts: any
  jsonicOpts: any
  profile: string
  notes: string[]
}

function loadGoCorpus(): { cases: GoCase[]; excluded: any[] } {
  const file = join(suitesDir, 'go-encoding-csv', 'cases.json')
  if (!existsSync(file)) throw new Error(MISSING + '\n  expected: ' + file)
  const corpus = JSON.parse(readFileSync(file, 'utf8'))
  assert.ok(
    Array.isArray(corpus.cases) && corpus.cases.length > 0,
    MISSING + '\n  corpus at ' + file + ' has no cases',
  )
  return corpus
}

function makeParser(c: GoCase) {
  let j = new Tabnas().use(jsonic)
  if (c.jsonicOpts) j.options(c.jsonicOpts)
  return j.use(Csv, c.opts)
}

describe('conformance: go/encoding/csv', () => {
  const corpus = loadGoCorpus()
  const valid = corpus.cases.filter((c) => !c.mustFail)
  const invalid = corpus.cases.filter((c) => c.mustFail)

  test('valid documents parse to the expected value', () => {
    const failures: string[] = []
    for (const c of valid) {
      let actual: any
      try {
        actual = makeParser(c).parse(c.input)
      } catch (e: any) {
        failures.push(`${c.name} [${c.profile}]: threw ${e.code || e.message}`)
        continue
      }
      if (!jsonEq(actual, c.expected)) {
        failures.push(
          `${c.name} [${c.profile}]: input ${show(c.input)} expected ` +
            `${show(c.expected)} got ${show(actual)}`,
        )
      }
    }
    report('go/encoding/csv valid', valid.length, failures)
  })

  test('invalid documents are rejected', () => {
    const failures: string[] = []
    for (const c of invalid) {
      let actual: any
      try {
        actual = makeParser(c).parse(c.input)
      } catch (e: any) {
        continue // rejected, as required
      }
      failures.push(
        `${c.name} [${c.profile}]: input ${show(c.input)} was ACCEPTED as ` +
          `${show(actual)} but RFC 4180 / encoding/csv rejects it`,
      )
    }
    report('go/encoding/csv invalid-rejected', invalid.length, failures)
  })

  // The excluded set is asserted, not merely documented, so a future change to
  // the extractor cannot quietly grow it.
  test('the excluded set is exactly the documented one', () => {
    const names = corpus.excluded.map((e: any) => e.name).sort()
    assert.deepEqual(names, [
      // LazyQuotes: a deliberately non-RFC-4180 lenient mode with no
      // @tabnas/csv equivalent, so there is no behaviour to assert.
      'BareDoubleQuotes',
      'BareQuotes',
      'LazyOddQuotes',
      'LazyQuoteWithTrailingCRLF',
      'LazyQuotes',
      // No Input at all: these assert that Go's NewReader rejects a bad
      // Comma/Comment *rune*. An API validation test, not a document.
      'BadComma1',
      'BadComma2',
      'BadComma3',
      'BadComma4',
      'BadCommaComment',
      'BadComment1',
      'BadComment2',
      'BadComment3',
    ].sort())
  })
})
