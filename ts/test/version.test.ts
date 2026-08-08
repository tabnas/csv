/* Copyright (c) 2026 Richard Rodger, MIT License */

// The exported VERSION must equal package.json "version".
//
// This is the CI check for version drift. It exists because the constant HAS
// drifted: @tabnas/json exported Version = '1.0.0' for several releases while
// the package shipped 0.4.x, because nothing rewrote it and AGENTS.md wrongly
// claimed `make publish-go` kept it in sync. A release that bumps
// package.json and forgets the constant now fails here.

import { describe, test } from 'node:test'
import assert from 'node:assert'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

// Read package.json directly rather than importing it: an unreadable or
// malformed package.json must FAIL this test loudly, never make it vacuous.
// A version check that silently does not run is the failure mode this test
// exists to prevent.
function readPkg(): { name: string; version: string } {
  const pkgPath = join(__dirname, '..', 'package.json')
  let raw: string
  try {
    raw = readFileSync(pkgPath, 'utf8')
  } catch (e: any) {
    assert.fail(
      `cannot read ${pkgPath}, so VERSION cannot be checked: ${e.message}`,
    )
  }
  let pkg: any
  try {
    pkg = JSON.parse(raw)
  } catch (e: any) {
    assert.fail(`${pkgPath} is not readable JSON: ${e.message}`)
  }
  assert.equal(
    typeof pkg.version,
    'string',
    `${pkgPath} has no version field`,
  )
  return pkg
}

// Resolve through the package root (package.json "main"), not the built file
// directly: consumers see VERSION only if it is reachable from there.
const api = require('..')

describe('version', () => {
  test('VERSION matches package.json', () => {
    const pkg = readPkg()
    assert.equal(
      api.VERSION,
      pkg.version,
      `VERSION drift: ${pkg.name} exports ${api.VERSION} but package.json is ` +
        `${pkg.version}. Both are rewritten by admin/publish.sh at release; ` +
        `if you bumped one by hand, bump the other.`,
    )
  })

  test('VERSION is exported and looks like a semver', () => {
    assert.equal(
      typeof api.VERSION,
      'string',
      'VERSION must be exported as a string',
    )
    assert.match(api.VERSION, /^\d+\.\d+\.\d+/, 'VERSION must be a semver')
  })
})
