/* Copyright (c) 2021-2026 Richard Rodger, MIT License */

import * as assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import * as path from 'node:path'
import { test } from 'node:test'

const { translate } = require('../dist/csv')

const root = path.resolve(__dirname, '..', '..')

test('translation parts expose the manifest and builtin render entry', () => {
  const parts = translate()
  assert.ok(parts)
  assert.equal(parts.manifest, readFileSync(path.join(root, 'tabnas.plugin.json'), 'utf8'))
  assert.equal(parts.lift, undefined)
  assert.equal(parts.render?.entry, 'csv')
  assert.equal(parts.render?.source, undefined)
})

// An embed takes a plain tree into a format's own schema. CSV's events
// carry a plain tree, so its manifest names none and the package carries
// none; a manifest that named one would be held to its file here.
test('translation parts carry the embed the manifest names, and none where it names none', () => {
  const parts = translate()
  const spec = JSON.parse(readFileSync(path.join(root, 'tabnas.plugin.json'), 'utf8')).translate
  if (null == spec.embed) {
    assert.equal(parts.embed, undefined)
  } else {
    assert.equal(parts.embed?.entry, 'csv-embed')
    assert.equal(parts.embed?.source, readFileSync(path.join(root, spec.embed), 'utf8'))
  }
})

// A transducer makes the format's JSON events of its parser's rule passes.
// CSV's is alchemy/transduce.alc, which the manifest names and the
// package embeds under the entry csv-transduce: the copy is held to the
// file.
test('translation parts carry the transducer the manifest names', () => {
  const parts = translate()
  const spec = JSON.parse(readFileSync(path.join(root, 'tabnas.plugin.json'), 'utf8')).translate
  assert.equal(spec.transduce, 'alchemy/transduce.alc')
  const source = readFileSync(path.join(root, spec.transduce), 'utf8')
  assert.ok(source.includes('def csv-transduce [input]'))
  assert.equal(parts.transduce?.entry, 'csv-transduce')
  assert.equal(parts.transduce?.source, source)
})
