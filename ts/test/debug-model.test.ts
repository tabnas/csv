/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// Composition test: the csv grammar plugin layered with the official
// @tabnas/debug plugin. @tabnas/debug is a devDependency, but this resolves
// it dynamically and SKIPS when it is absent so the suite stays runnable
// outside the package; set TABNAS_DEBUG_PATH to point at a sibling
// checkout's built plugin.

import { describe, test } from 'node:test'
import assert from 'node:assert'

import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '../dist/csv'

function loadDebug(): any {
  const candidates = [process.env.TABNAS_DEBUG_PATH, '@tabnas/debug'].filter(
    Boolean,
  ) as string[]
  for (const c of candidates) {
    try {
      return require(c).Debug
    } catch {
      /* try next */
    }
  }
  return null
}

const Debug = loadDebug()
const skip = Debug
  ? false
  : '@tabnas/debug not available (set TABNAS_DEBUG_PATH)'

describe('debug-model', () => {
  test('debug.model() returns the structured csv grammar', { skip }, () => {
    const tn = new Tabnas().use(jsonic).use(Csv)
    tn.use(Debug, { print: false, trace: false })

    const m = tn.debug.model()

    // The structured rule set: csv-specific rules (csv/record/text/newline)
    // plus the shared jsonic rules (val/map/list/pair/elem).
    assert.deepStrictEqual(m.rules.map((r: any) => r.name).sort(), [
      'csv',
      'elem',
      'list',
      'map',
      'newline',
      'pair',
      'record',
      'text',
      'val',
    ])

    // The entry rule for csv parsing.
    assert.equal(m.config.start, 'csv')

    // The csv plugin is registered in the plugin chain.
    assert.ok(
      m.plugins.some((p: any) => p.name === 'Csv'),
      'plugins should list Csv',
    )

    // Structural facts specific to this grammar:
    // the top-level csv rule opens by pushing newline and record rules,
    const csv = m.rules.find((r: any) => r.name === 'csv')
    const csvPushes = (csv.open || []).map((a: any) => a.push).filter(Boolean)
    assert.ok(csvPushes.includes('newline'), 'csv should push newline')
    assert.ok(csvPushes.includes('record'), 'csv should push record')

    // and each record is parsed as a list of fields.
    const record = m.rules.find((r: any) => r.name === 'record')
    const recordPushes = (record.open || [])
      .map((a: any) => a.push)
      .filter(Boolean)
    assert.ok(recordPushes.includes('list'), 'record should push list')

    // The grammar portion is JSON-serialisable and round-trips.
    assert.deepStrictEqual(JSON.parse(JSON.stringify(m.rules)), m.rules)
  })
})
