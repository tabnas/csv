/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// `make(options)` is `new Tabnas().use(jsonic).use(Csv, options)` behind one
// call. These tests hold it to that: every committed fixture parses the
// same through both routes, with the fixture's own options, and a handful
// of non-default options are checked directly. The Go half is
// go/make_test.go; Rust's `make_with` is covered by rs/tests/csv_test.rs.

import { describe, test } from 'node:test'
import assert from 'node:assert'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv, make } from '../dist/csv'

const fixturesDir = join(__dirname, '..', '..', 'test', 'fixtures')
const manifest = JSON.parse(
  readFileSync(join(fixturesDir, 'manifest.json'), 'utf8'),
)

// What a parse produced: the value, or the error's code. Comparing codes
// rather than messages is the fixture contract (test/AGENTS.md).
function outcome(parser: Tabnas, src: string): any {
  try {
    return { value: parser.parse(src) }
  } catch (e: any) {
    return { error: e.code ?? String(e) }
  }
}

function plugin(options?: any): Tabnas {
  return new Tabnas().use(jsonic).use(Csv, options)
}

describe('make', () => {
  test('returns a Tabnas instance with the csv plugin installed', () => {
    const parser = make()
    assert.ok(parser instanceof Tabnas)
    assert.deepEqual(parser.parse('a,b\n1,2\n3,4'), [
      { a: '1', b: '2' },
      { a: '3', b: '4' },
    ])
  })

  test('default options parse as the plugin route does', () => {
    for (const src of ['', '\n', 'a\n"b""c"', 'a,b\n1,2,3', 'a\n"b']) {
      assert.deepEqual(outcome(make(), src), outcome(plugin(), src), src)
      assert.deepEqual(outcome(make({}), src), outcome(plugin(), src), src)
    }
    assert.deepEqual(outcome(make(), 'a\n"b'), {
      error: 'unterminated_string',
    })
  })

  test('non-default options parse as the plugin route does', () => {
    const cases: [any, string, any][] = [
      [{ header: false, object: false }, 'a,b\n1,2', [['a', 'b'], ['1', '2']]],
      [{ field: { separation: ';' } }, 'a;b\n1;2', [{ a: '1', b: '2' }]],
      [{ record: { empty: true } }, 'a\n1\n\n2', [{ a: '1' }, { a: '' }, { a: '2' }]],
      [{ strict: false }, 'a,b\n{x:1},true', [{ a: { x: 1 }, b: true }]],
      [{ trim: true }, 'a\n 1 ', [{ a: '1' }]],
      [{ header: false, field: { names: ['x', 'y'] } }, '1,2', [{ x: '1', y: '2' }]],
      [{ string: { quote: "'" } }, "a\n'x''y'", [{ a: "x'y" }]],
    ]
    for (const [opts, src, expected] of cases) {
      const label = JSON.stringify(opts)
      assert.deepEqual(make(opts).parse(src), expected, label)
      assert.deepEqual(outcome(make(opts), src), outcome(plugin(opts), src), label)
    }
  })

  test('a rejection carries the same code through both routes', () => {
    const opts = { field: { exact: true } }
    assert.deepEqual(outcome(make(opts), 'a,b\n1,2,3'), {
      error: 'csv_extra_field',
    })
    assert.deepEqual(outcome(make(opts), 'a,b\n1'), {
      error: 'csv_missing_field',
    })
    for (const src of ['a,b\n1,2,3', 'a,b\n1']) {
      assert.deepEqual(outcome(make(opts), src), outcome(plugin(opts), src))
    }
  })

  test('the stream callback is honoured', () => {
    const seen: any[] = []
    const out = make({
      stream: (what, record) => seen.push([what, record]),
    }).parse('a\n1\n2')
    assert.deepEqual(out, [])
    assert.deepEqual(seen, [
      ['start', undefined],
      ['record', { a: '1' }],
      ['record', { a: '2' }],
      ['end', undefined],
    ])
  })

  test('each call is a fresh instance; options do not leak', () => {
    const arrays = make({ object: false })
    const objects = make()
    assert.notEqual(arrays, objects)
    assert.deepEqual(arrays.parse('a\n1'), [['1']])
    assert.deepEqual(objects.parse('a\n1'), [{ a: '1' }])
  })

  test('every fixture parses the same through make and the plugin', () => {
    let judged = 0
    for (const [key, entry] of Object.entries(manifest) as [string, any][]) {
      // jsonicOpt sets ENGINE options between the two `use` calls, which
      // make() does not take: those fixtures are the plugin route's alone.
      if (entry.jsonicOpt) continue
      const raw = readFileSync(
        join(fixturesDir, (entry.csvFile || key) + '.csv'),
        'utf8',
      )
      assert.deepEqual(
        outcome(make(entry.opt), raw),
        outcome(plugin(entry.opt), raw),
        entry.name,
      )
      judged++
    }
    assert.ok(20 < judged, 'judged only ' + judged + ' fixtures')
  })
})
