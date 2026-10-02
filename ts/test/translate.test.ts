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
