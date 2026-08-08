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
// go/conformance_test.go runs the same two corpora with the same divergence
// table, so TS and Go cannot drift without one of them going red.
//
// On DIVERGENCES: go/encoding/csv is a strict RFC 4180 reader. @tabnas/csv is
// deliberately a lenient, PapaParse-compatible reader with RFC 4180 quoting
// (see AGENTS.md "Conformance"). Where the two differ ON PURPOSE, the case is
// NOT skipped — the @tabnas/csv result is pinned as a positive assertion, AND
// the test re-checks that the case really still disagrees with the corpus. So
// a divergence cannot rot into a silent exemption: if the plugin ever starts
// conforming, the divergence entry must be deleted or this suite goes red.

import { describe, test } from 'node:test'
import assert from 'node:assert'
import { existsSync, readdirSync, readFileSync } from 'node:fs'
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
  return {
    csvDir: requireDir(join(dir, 'csvs')),
    jsonDir: requireDir(join(dir, 'json')),
  }
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
      const expected = JSON.parse(
        readFileSync(join(jsonDir, name + '.json'), 'utf8'),
      )

      let actual: any
      try {
        actual = parser.parse(src)
      } catch (e: any) {
        failures.push(`${name}: threw ${e.code || e.message}`)
        continue
      }

      if (!jsonEq(actual, expected)) {
        failures.push(`${name}: expected ${show(expected)} got ${show(actual)}`)
      }
    }

    assert.equal(
      judged,
      names.length - 1,
      'every corpus document must be judged',
    )
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
  // Instead of an exclusion, the upstream defect is PINNED here: if
  // csv-spectrum ever fixes either half, this test goes red and the case moves
  // back into the judged set above.
  test('csv-spectrum upstream defect: location_coordinates contradicts itself', () => {
    const { csvDir, jsonDir } = spectrumDirs()
    const src = readFileSync(
      join(csvDir, SPECTRUM_UPSTREAM_DEFECT + '.csv'),
      'utf8',
    )
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
    const actual: any = new Tabnas().use(jsonic).use(Csv).parse(src)
    assert.equal(Array.isArray(actual), true)
    assert.equal(actual.length, 1)
    assert.equal(actual[0]['Contact Phone Number'], '2095257564')
    assert.deepEqual(Object.keys(actual[0]), [
      'Contact Phone Number',
      'Location Coordinates',
      'Cities',
      'Counties',
    ])
  })
})

// --------------------------------------------------------------------------
// SUITE 2 — golang/go src/encoding/csv @ 3901409b5d0fb7c85a3e6730a59943cc93b2835c
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

// Deliberate, documented departures from go/encoding/csv. Each entry pins what
// @tabnas/csv actually produces, so the behaviour is asserted rather than
// waived. Keep in step with AGENTS.md "Conformance" and go/conformance_test.go.
const DIVERGENCES: Record<string, { why: string; result: any | 'ERROR' }> = {
  // (1) A bare CR is a record separator (PapaParse-compatible, and what
  // `record.separators: null` documents: "\n / \r\n / \r"). go/encoding/csv
  // treats a CR not followed by LF as ordinary field data. Pinned by the
  // committed fixture test/fixtures/papa-two-rows-just-r.csv.
  BareCR: { why: 'bare-CR-is-a-record-separator', result: [['a', 'b'], ['c', 'd']] },
  FieldCR: { why: 'bare-CR-is-a-record-separator', result: [['field'], ['field']] },
  FieldCRCR: { why: 'bare-CR-is-a-record-separator', result: [['field'], ['field']] },
  FieldCRCRLF: { why: 'bare-CR-is-a-record-separator', result: [['field'], ['field']] },
  FieldCRCRLFCR: { why: 'bare-CR-is-a-record-separator', result: [['field'], ['field']] },
  FieldCRCRLFCRCR: { why: 'bare-CR-is-a-record-separator', result: [['field'], ['field']] },
  MultiFieldCRCRLFCRCR: {
    why: 'bare-CR-is-a-record-separator',
    result: [['field1', 'field2'], ['field1', 'field2'], ['', '']],
  },
  QuotedTrailingCRCR: { why: 'bare-CR-is-a-record-separator', result: [['field']] },

  // (2) A CRLF inside a quoted field is field data and survives verbatim.
  // go/encoding/csv normalises it to a bare LF, which is a Go convenience,
  // not an RFC 4180 requirement (RFC 4180 §2.6: CRLF inside quotes is data).
  CRLFInQuotedField: {
    why: 'quoted-CRLF-is-preserved-verbatim',
    result: [['A', 'Hello\r\nHi', 'B']],
  },

  // (3) Stray quotes inside an unquoted field are ordinary text, not an
  // error (PapaParse-compatible lenience). go/encoding/csv rejects them with
  // ErrBareQuote. Pinned by test/fixtures/papa-unquoted-field-with-quotes-*.
  BadDoubleQuotes: { why: 'stray-quotes-are-literal-text', result: [['a""b', 'c']] },
  BadBareQuote: { why: 'stray-quotes-are-literal-text', result: [['a "word"', 'b']] },
  BadTrailingQuote: { why: 'stray-quotes-are-literal-text', result: [['a word', 'b"']] },

  // (4) `trim` trims surrounding whitespace but does NOT then re-read the
  // remainder as a quoted field, so ` "a"` is the three-character text `"a"`.
  // Go's TrimLeadingSpace trims first and unquotes after. Pinned by
  // test/fixtures/papa-quoted-field-with-whitespace-around-quotes.csv.
  TrimQuote: {
    why: 'whitespace-then-quote-is-literal-text',
    result: [['"a"', ' b', 'c']],
  },

  // (5) `field.exact` is header-relative by documentation ("error when a
  // record's field count differs from the header's" — ts/doc/reference.md).
  // These cases run with `header: false` and no `field.names`, so there is no
  // expected count and the option is correctly inert. Go's FieldsPerRecord
  // needs no header; @tabnas/csv has no equivalent uniform-count mode.
  BadFieldCount: {
    why: 'field.exact-is-header-relative',
    result: [['a', 'b', 'c'], ['d', 'e']],
  },
  BadFieldCountMultiple: {
    why: 'field.exact-is-header-relative',
    result: [['a', 'b', 'c'], ['d', 'e'], ['f']],
  },
  BadFieldCount1: { why: 'field.exact-is-header-relative', result: [['a', 'b', 'c']] },
}

function loadGoCorpus(): { cases: GoCase[]; excluded: { name: string }[] } {
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

function run(c: GoCase): { threw: boolean; value?: any } {
  try {
    return { threw: false, value: makeParser(c).parse(c.input) }
  } catch (e: any) {
    return { threw: true, value: e.code || e.message }
  }
}

describe('conformance: go/encoding/csv', () => {
  const corpus = loadGoCorpus()
  const valid = corpus.cases.filter((c) => !c.mustFail)
  const invalid = corpus.cases.filter((c) => c.mustFail)

  test('valid documents parse to the expected value', () => {
    const failures: string[] = []

    for (const c of valid) {
      const d = DIVERGENCES[c.name]
      const got = run(c)

      if (d) {
        // A divergence is an assertion, not a waiver.
        const want = 'ERROR' === d.result ? 'ERROR' : d.result
        const actual = got.threw ? 'ERROR' : got.value
        if (!jsonEq(actual, want)) {
          failures.push(
            `${c.name} [divergence ${d.why}]: pinned ${show(want)} but got ` +
              `${show(actual)} — update DIVERGENCES or fix the regression`,
          )
        } else if (!got.threw && jsonEq(got.value, c.expected)) {
          failures.push(
            `${c.name}: listed as a divergence but now MATCHES the corpus — ` +
              'delete the DIVERGENCES entry',
          )
        }
        continue
      }

      if (got.threw) {
        failures.push(`${c.name} [${c.profile}]: threw ${got.value}`)
        continue
      }
      if (!jsonEq(got.value, c.expected)) {
        failures.push(
          `${c.name} [${c.profile}]: input ${show(c.input)} expected ` +
            `${show(c.expected)} got ${show(got.value)}`,
        )
      }
    }

    report('go/encoding/csv valid', valid.length, failures)
  })

  test('invalid documents are rejected', () => {
    const failures: string[] = []

    for (const c of invalid) {
      const d = DIVERGENCES[c.name]
      const got = run(c)

      if (d) {
        const actual = got.threw ? 'ERROR' : got.value
        if (!jsonEq(actual, d.result)) {
          failures.push(
            `${c.name} [divergence ${d.why}]: pinned ${show(d.result)} but ` +
              `got ${show(actual)} — update DIVERGENCES or fix the regression`,
          )
        } else if (got.threw) {
          failures.push(
            `${c.name}: listed as a divergence but is now REJECTED like the ` +
              'corpus requires — delete the DIVERGENCES entry',
          )
        }
        continue
      }

      if (!got.threw) {
        failures.push(
          `${c.name} [${c.profile}]: input ${show(c.input)} was ACCEPTED as ` +
            `${show(got.value)} but RFC 4180 / encoding/csv rejects it`,
        )
      }
    }

    report('go/encoding/csv invalid-rejected', invalid.length, failures)
  })

  // The headline number, asserted so it cannot drift unnoticed.
  test('the conformance score is exactly the documented one', () => {
    const names = Object.keys(DIVERGENCES)
    const known = new Set(corpus.cases.map((c) => c.name))
    for (const n of names) {
      assert.ok(known.has(n), `DIVERGENCES names a case not in the corpus: ${n}`)
    }
    assert.equal(
      corpus.cases.length - names.length,
      39,
      'documented in AGENTS.md as 39/55 go/encoding/csv cases conforming ' +
        `(got ${corpus.cases.length - names.length}/${corpus.cases.length}) — ` +
        'update AGENTS.md and README.md together with this number',
    )
    assert.equal(names.length, 16, 'AGENTS.md documents 16 divergences')
  })

  // The excluded set is asserted, not merely documented, so a future change to
  // the extractor cannot quietly grow it.
  test('the excluded set is exactly the documented one', () => {
    const names = corpus.excluded.map((e) => e.name).sort()
    assert.deepEqual(
      names,
      [
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
      ].sort(),
    )
  })
})
