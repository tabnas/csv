/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// A parse from a reader (`tn.parseReader`, admin ADR-30 and ADR-33).
//
// `make()` declares `lex.window`, so that a reader is read a window at a
// time. The quoted field, the one token this grammar cuts itself, reads the
// source only through the window (`lex.fill` where the field runs past its
// end), and jsonic's base is the engine's own matchers. These tests hold it
// to that: `make()` declares the window and the plugin alone does not;
// every row of the shared fixtures, every committed fixture and every
// document of the two vendored suites parses the same from a reader, in
// pieces of 1, 3 and 64 bytes, under windows of a few UTF-16 units and the
// engine's own, as from its text; quoted fields longer than the window
// parse the same; and so does a long document, read under a window of 256
// units that moves on thousands of times, with an error at its end too.

import { describe, test } from 'node:test'
import assert from 'node:assert'
import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { inspect } from 'node:util'

import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { findSpecDir, loadSpec } from '@tabnas/support'

import { Csv, make } from '../dist/csv'

const SPEC = findSpecDir(__dirname)
const FIXTURES = join(__dirname, '..', '..', 'test', 'fixtures')
const SUITES = join(__dirname, '..', '..', 'test', 'suites')

type Reader = { read(buf: Uint8Array): number }
type Seen = { value?: unknown; error?: string }

// A reader that hands its bytes over at most `size` at a time.
function pieces(text: string, size: number): Reader {
  const bytes = Buffer.from(text, 'utf8')
  let at = 0
  return {
    read(buf: Uint8Array): number {
      const n = Math.min(size, buf.length, bytes.length - at)
      buf.set(bytes.subarray(at, at + n))
      at += n
      return n
    },
  }
}

// `tn` with `lex.window` set, as `make()` sets it.
function windowed(tn: Tabnas): Tabnas {
  tn.options({ lex: { window: true } } as any)
  return tn
}

// What a parse shows: its value, or its error's code, position, row,
// column and token text. An error's excerpt is left out: a parse from a
// reader shows a long line only as far back as the window reaches
// (tabnas/parser DIVERGENCE.md), the one difference allowed.
function seen(parse: () => unknown): Seen {
  try {
    return { value: parse() }
  } catch (e: any) {
    if (null == e || 'function' !== typeof e.toJSON) {
      return { error: 'throw ' + (e && e.name) + ' ' + (e && e.message) }
    }
    const d = e.toJSON()
    return {
      error:
        d.code + ' at ' + d.pos + ' ' + d.row + ':' + d.col + ' ' +
        JSON.stringify(d.token.src),
    }
  }
}

const show = (o: Seen): string =>
  (undefined !== o.error ? 'error ' + o.error : 'value ' + inspect(o.value, { depth: 4 }))
    .slice(0, 300)

function same(a: Seen, b: Seen): boolean {
  if (a.error !== b.error) return false
  try {
    assert.deepStrictEqual(a.value, b.value)
    return true
  } catch (e) {
    return false
  }
}

// The pieces a short document is handed over in, and the windows (margin
// and block) it is read under besides the engine's own: a few units, so
// that every matcher, the quoted field's among them, meets the window's
// edge. A document over 4 KiB is read in pieces of 4096 and of a little
// over the window, under the engine's window.
const PIECES = [1, 3, 64]
const WINDOWS = [[1, 1], [3, 2], [64, 64]]

// `text` parsed by `tn` whole, and from a reader in every pieces size, as
// above: how many reader parses were compared. A difference is pushed
// onto `failures`.
function sameFromReader(tn: Tabnas, at: string, text: string, failures: string[]): number {
  const whole = seen(() => tn.parse(text))
  const anyTn = tn as any
  const large = 4096 < text.length
  let compared = 0
  for (const size of large ? [4096, (64 << 10) + 3] : PIECES) {
    const reads: [string, () => unknown][] = [['', () => anyTn.parseReader(pieces(text, size))]]
    for (const [margin, block] of large ? [] : WINDOWS) {
      reads.push([
        ', margin ' + margin + ', block ' + block,
        () => anyTn.parseReaderWithSizes(pieces(text, size), { margin, block, chunk: size }),
      ])
    }
    for (const [how, parse] of reads) {
      const read = seen(parse)
      compared++
      if (!same(read, whole)) {
        failures.push(
          at + ': ' + JSON.stringify(text.slice(0, 80)) + ' from a reader in pieces of ' +
          size + how + '\n  whole:  ' + show(whole) + '\n  reader: ' + show(read),
        )
      }
    }
  }
  return compared
}

describe('window', () => {
  test('make-declares-the-window', () => {
    assert.equal((make() as any).internal().config.lex.window, true)
    // The plugin alone declares nothing: a host may install it beneath
    // matchers of its own.
    const plugin = new Tabnas().use(jsonic).use(Csv) as any
    assert.ok(!plugin.internal().config.lex.window)
  })

  test('every-spec-row-parses-the-same-from-a-reader', () => {
    const failures: string[] = []
    let compared = 0
    for (const file of readdirSync(SPEC).filter((f) => f.endsWith('.tsv')).sort()) {
      for (const row of loadSpec(join(SPEC, file)).rows) {
        const opts = row.named('opts')
        compared += sameFromReader(
          make('' === opts.trim() ? {} : JSON.parse(opts)),
          file + ':' + row.line,
          row.unescNamed('input'),
          failures,
        )
      }
    }
    console.log('window spec: ' + compared + ' reader parses (100%)')
    assert.ok(1000 < compared, '' + compared)
    assert.deepEqual(failures, [])
  })

  test('every-fixture-parses-the-same-from-a-reader', () => {
    // Each under the parser csv.test.ts builds for it.
    const manifest = JSON.parse(readFileSync(join(FIXTURES, 'manifest.json'), 'utf8'))
    const failures: string[] = []
    let compared = 0
    for (const [key, entry] of Object.entries(manifest) as [string, any][]) {
      let parser = new Tabnas().use(jsonic)
      if (entry.opt && entry.jsonicOpt) parser.options(entry.jsonicOpt)
      parser = parser.use(Csv, entry.opt ?? {})
      const text = readFileSync(join(FIXTURES, (entry.csvFile || key) + '.csv'), 'utf8')
      compared += sameFromReader(windowed(parser), key, text, failures)
    }
    console.log('window fixtures: ' + compared + ' reader parses (100%)')
    assert.ok(1000 < compared, '' + compared)
    assert.deepEqual(failures, [])
  })

  test('every-suite-document-parses-the-same-from-a-reader', () => {
    const failures: string[] = []
    let compared = 0
    const csvDir = join(SUITES, 'csv-spectrum', 'csvs')
    for (const file of readdirSync(csvDir).filter((f) => f.endsWith('.csv')).sort()) {
      const text = readFileSync(join(csvDir, file), 'utf8')
      compared += sameFromReader(make(), 'csv-spectrum ' + file, text, failures)
    }
    // Each go/encoding/csv case under the parser conformance.test.ts
    // builds for it: its engine options, then the plugin with its own.
    const corpus = JSON.parse(
      readFileSync(join(SUITES, 'go-encoding-csv', 'cases.json'), 'utf8'),
    )
    for (const c of corpus.cases) {
      const parser = new Tabnas().use(jsonic)
      if (c.jsonicOpts) parser.options(c.jsonicOpts)
      compared += sameFromReader(
        windowed(parser.use(Csv, c.opts)),
        'go/encoding/csv ' + c.name,
        c.input,
        failures,
      )
    }
    console.log('window suites: ' + compared + ' reader parses (100%)')
    assert.ok(500 < compared, '' + compared)
    assert.deepEqual(failures, [])
  })

  test('quoted-fields-parse-the-same-under-small-windows', () => {
    // Fields whose doubled quotes, closing quotes, line breaks and control
    // characters meet the window's edge at every place.
    const documents = [
      'a,b\n"x","y"',
      'a\n"b""c"',
      'a\n""""',
      'a\n""',
      'a,b\n"one, two","three\nfour"\n"5",6',
      'a\n"ñ""é"""',
      'a\n"long field with ""quotes"" and, commas, and\nlines"',
      'a\n"x"',
      'a\n"x"""',
      'a\n"b',
      'a\n"b"""',
      'a,b\n"","",\n"""",x',
      'a\n"tab\there"\n"cr\r\nlf"',
      'a\n"x"y',
    ]
    const failures: string[] = []
    let compared = 0
    for (const options of [{}, { header: false, object: false }]) {
      for (const document of documents) {
        compared += sameFromReader(make(options), 'quoted', document, failures)
      }
    }
    assert.equal(compared, 2 * documents.length * 12)
    assert.deepEqual(failures, [])
  })

  test('fields-longer-than-the-window-parse-the-same-from-a-reader', () => {
    // The engine's window reaches 64 KiB past the cursor before each
    // token: a field longer than that meets its edge under the engine's
    // own sizes. Each document begins with its field, and the shift moves
    // what the matcher looks for across the edge a byte at a time.
    const pad = (n: number) => 'a'.repeat(n)
    const failures: string[] = []
    const tn = make({ header: false, object: false })
    for (let shift = -4; shift <= 4; shift++) {
      const edge = (64 << 10) + shift
      for (const doc of [
        '"' + pad(edge - 1) + '",b\n',
        '"' + pad(edge - 2) + '""b",c\n',
        '"' + pad(edge - 1) + '"""\n',
        '"' + pad(edge - 1) + '\nb",c\n',
        '"' + pad(edge),
        pad(edge + 5) + ',b\n',
      ]) {
        sameFromReader(tn, 'shift ' + shift, doc, failures)
      }
    }
    assert.deepEqual(failures, [])
  })

  test('a-long-document-parses-the-same-from-a-reader', () => {
    const rows: string[] = ['id,name,note']
    for (let i = 0; i < 8000; i++) {
      rows.push(i + ',"ñ' + i + ' ""q""","line\nbreak, ' + i + '"')
    }
    const text = rows.join('\n') + '\n'
    assert.ok(256 << 10 < text.length, '' + text.length)
    const broken = text + 'z,"open'
    const tn = make() as any
    ;[text, broken].forEach((doc, n) => {
      const whole = seen(() => tn.parse(doc))
      assert.equal(undefined === whole.error, 0 === n, show(whole))
      for (const sizes of [undefined, { margin: 256, block: 256, chunk: 4096 }]) {
        const read = seen(() =>
          undefined === sizes
            ? tn.parseReader(pieces(doc, 4096))
            : tn.parseReaderWithSizes(pieces(doc, 4096), sizes),
        )
        assert.ok(same(read, whole), show(read))
      }
      console.log('window long document: ' + (n + 1) + ' of 2 (' + (n + 1) * 50 + '%)')
    })
  })
})
