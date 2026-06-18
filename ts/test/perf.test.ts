/* Copyright (c) 2026 tabnas, MIT License */

import { describe, test } from 'node:test'
import assert from 'node:assert'

import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '../dist/csv'

// Build a fresh engine with the Csv plugin installed — the (expensive) work a
// caller does once and then reuses. Building the grammar dominates a parse, so
// doing this per parse is many times slower than reusing one instance.
function makeCsvParser(): Tabnas {
  return new Tabnas().use(jsonic).use(Csv)
}

describe('perf', () => {
  // Guards against the performance trap this package exposes: there is no
  // package-level convenience parse() to cache, so callers must build ONE
  // instance (new Tabnas().use(jsonic).use(Csv)) and reuse it. Rebuilding the
  // instance — and therefore the CSV grammar — on every parse is many times
  // slower. This test pins the representative "reuse one instance" usage and
  // asserts it is dramatically faster than rebuilding per call, so a future
  // change that accidentally rebuilds per parse (or a convenience entry point
  // that forgets to cache) is caught.
  //
  // The check is machine-INDEPENDENT: it compares rebuild-per-call against
  // instance reuse on the SAME machine in the SAME run, so a slow CI box cannot
  // make it flaky (both sides scale together). There is deliberately NO
  // wall-clock budget.
  test('reuses-instance', () => {
    const src = 'a,b,c\n1,2,3'
    const n = 120

    // Warm both paths so the comparison is steady-state.
    for (let i = 0; i < 20; i++) {
      makeCsvParser().parse(src)
    }
    const shared = makeCsvParser()
    for (let i = 0; i < 50; i++) {
      shared.parse(src)
    }

    // Anti-pattern: rebuild the instance (and grammar) on every parse.
    const t0 = process.hrtime.bigint()
    for (let i = 0; i < n; i++) {
      makeCsvParser().parse(src)
    }
    const rebuild = Number(process.hrtime.bigint() - t0)

    // Correct pattern: build once, reuse for every parse.
    const t1 = process.hrtime.bigint()
    for (let i = 0; i < n; i++) {
      shared.parse(src)
    }
    const reuse = Number(process.hrtime.bigint() - t1)

    const ratio = rebuild / reuse

    // Reusing one instance must be far cheaper than rebuilding per call.
    // Rebuilding is many times slower here (grammar construction dominates),
    // so requiring reuse to be at least 4x faster catches a regression that
    // rebuilds per parse, without depending on absolute wall-clock speed.
    assert.ok(
      rebuild >= 4 * reuse,
      `reusing one Csv instance is not meaningfully faster than rebuilding ` +
        `per parse: ${n} rebuild-per-call parses took ${(rebuild / 1e6).toFixed(1)}ms ` +
        `vs ${(reuse / 1e6).toFixed(1)}ms reusing one instance ` +
        `(ratio ${ratio.toFixed(1)}x, want >=4x). Building the CSV grammar ` +
        `should dominate — reuse one new Tabnas().use(jsonic).use(Csv) instance ` +
        `instead of rebuilding per parse.`,
    )

    console.log(
      `perf reuses-instance: rebuild-per-call=${(rebuild / 1e6).toFixed(1)}ms ` +
        `reuse=${(reuse / 1e6).toFixed(1)}ms ratio=${ratio.toFixed(2)}x`,
    )
  })
})
