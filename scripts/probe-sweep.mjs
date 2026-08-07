#!/usr/bin/env node
/*
 * Phase-1 measuring probe. Not part of the test suite - run by hand:
 *
 *   node scripts/probe-sweep.mjs > /tmp/ts-sweep.json
 *   (cd go && go test -run TestProbeSweep -v)      # writes /tmp/go-sweep.json
 *   node scripts/probe-sweep.mjs --compare /tmp/ts-sweep.json /tmp/go-sweep.json
 *
 * It answers two Phase-1 questions with numbers rather than anecdotes:
 *
 *  1. LENIENCY LEAK. Does layering the jsonic base grammar under the plugin
 *     (`new Tabnas().use(jsonic).use(Csv)`, the documented stack) accept
 *     things `new Tabnas().use(Csv)` alone rejects? That is the json5 defect
 *     shape, where '{a:1' errors with the plugin alone but is ACCEPTED through
 *     the documented stack.
 *
 *  2. TS/GO DIVERGENCE. Which inputs do the two runtimes classify or value
 *     differently?
 *
 * The input set is an exhaustive enumeration over a CSV-significant alphabet,
 * so it is deterministic and reproducible - no randomness, no seed.
 */

import { createRequire } from 'node:module'
import fs from 'node:fs'
import path from 'node:path'

const require = createRequire(import.meta.url)
const here = path.dirname(new URL(import.meta.url).pathname)

// --- compare mode ----------------------------------------------------------

if (process.argv[2] === '--compare') {
  const ts = JSON.parse(fs.readFileSync(process.argv[3], 'utf8'))
  const go = JSON.parse(fs.readFileSync(process.argv[4], 'utf8'))
  const byInput = new Map(go.map((r) => [r.input, r]))

  // Go's json.Marshal sorts object keys; the TS side preserves insertion
  // order. Canonicalise both so key ORDER is not reported as a divergence.
  const sortKeys = (v) => {
    if (Array.isArray(v)) return v.map(sortKeys)
    if (v && typeof v === 'object') {
      const o = {}
      for (const k of Object.keys(v).sort()) o[k] = sortKeys(v[k])
      return o
    }
    return v
  }
  const canon = (s) => {
    if (!s.startsWith('OK:')) return s
    try {
      return 'OK:' + JSON.stringify(sortKeys(JSON.parse(s.slice(3))))
    } catch {
      return s
    }
  }

  let same = 0
  const diffs = []
  for (const t of ts) {
    const g = byInput.get(t.input)
    if (!g) continue
    if (canon(t.stack) === canon(g.stack)) same++
    else diffs.push({ input: t.input, ts: canon(t.stack), go: canon(g.stack) })
  }
  console.log(`TS/Go agreement (documented stack, strict): ${same}/${ts.length}`)
  console.log(`divergences: ${diffs.length}`)
  for (const d of diffs.slice(0, 40)) {
    console.log(`  ${JSON.stringify(d.input)}\n    ts: ${d.ts}\n    go: ${d.go}`)
  }
  process.exit(0)
}

// --- sweep mode ------------------------------------------------------------

const { Tabnas } = require(path.join(here, '..', 'ts', 'node_modules', '@tabnas', 'parser'))
const { jsonic } = require(path.join(here, '..', 'ts', 'node_modules', '@tabnas', 'jsonic'))
const { Csv } = require(path.join(here, '..', 'ts', 'dist', 'csv.js'))

// A CSV-significant alphabet: the RFC 4180 structural characters, the base
// engine's other default string delimiters (' and `), an ordinary letter, and
// whitespace.
const ALPHABET = ['a', ',', '"', '\n', "'", '`', ' ']
const LENGTH = 4 // 7^4 = 2401 inputs

function* enumerate(alphabet, len) {
  if (len === 0) { yield ''; return }
  for (const head of alphabet) {
    for (const rest of enumerate(alphabet, len - 1)) yield head + rest
  }
}

function classify(make, src) {
  try {
    return 'OK:' + JSON.stringify(make().parse(src))
  } catch (e) {
    return 'ERR:' + (e.code || String(e.message).split('\n')[0])
  }
}

const rows = []
let leniencyLeaks = 0
for (const input of enumerate(ALPHABET, LENGTH)) {
  const stack = classify(() => new Tabnas().use(jsonic).use(Csv), input)
  const alone = classify(() => new Tabnas().use(Csv), input)
  if (stack !== alone) leniencyLeaks++
  rows.push({ input, stack, alone })
}

console.error(`inputs: ${rows.length}`)
console.error(
  `strict mode, use(jsonic).use(Csv) vs use(Csv): ` +
  `${rows.length - leniencyLeaks}/${rows.length} agree, ${leniencyLeaks} differ`,
)
for (const r of rows.filter((r) => r.stack !== r.alone).slice(0, 20)) {
  console.error(`  LEAK ${JSON.stringify(r.input)}\n    stack: ${r.stack}\n    alone: ${r.alone}`)
}

// Non-strict mode is a separate question: there the plugin genuinely needs the
// jsonic base grammar, so a difference is by design, not a leak.
const NONSTRICT = { strict: false }
let nsStackOk = 0
let nsAloneOk = 0
for (const input of ['a\nb', 'a\n[1,2]', 'a\n{x:1}', 'a\n1']) {
  if (classify(() => new Tabnas().use(jsonic).use(Csv, NONSTRICT), input).startsWith('OK')) nsStackOk++
  if (classify(() => new Tabnas().use(Csv, NONSTRICT), input).startsWith('OK')) nsAloneOk++
}
console.error(`non-strict sanity: stack accepts ${nsStackOk}/4, plugin-alone accepts ${nsAloneOk}/4`)

process.stdout.write(JSON.stringify(rows))
