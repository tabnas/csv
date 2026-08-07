#!/usr/bin/env node
/*
 * Convert the Go standard library `encoding/csv` reader test table into a
 * runtime-neutral JSON corpus that both the TypeScript and Go halves of
 * @tabnas/csv can run.
 *
 * INPUT   test/suites/go-encoding-csv/reader_test.go
 *         (downloaded at a pinned commit by scripts/fetch-csv-suites.sh)
 * OUTPUT  test/suites/go-encoding-csv/cases.json
 *
 * Neither file is committed - see scripts/fetch-csv-suites.sh and .gitignore.
 *
 * WHY THIS CORPUS. csv-spectrum (the most-cited third-party CSV corpus) is
 * valid-documents-only: it has no must-fail half at all. The W3C csvw suite
 * has 145 negative tests but *zero* of them are bare-CSV syntax violations -
 * every one is a JSON-LD metadata/schema violation (verified against
 * tests/manifest-validation.jsonld). The Go standard library's own
 * `encoding/csv` test table is the reference RFC 4180 implementation's
 * conformance table, is third-party, is versioned, and - crucially - carries
 * explicit expected *errors* (ErrQuote, ErrBareQuote, ErrFieldCount). It is
 * the closest thing to an authoritative must-fail CSV corpus that exists.
 *
 * HOW CASES MAP. Go's readTest.Output is [][]string - rows of raw fields with
 * no header interpretation - which is exactly what @tabnas/csv produces under
 * { header: false, object: false }. A case with any non-nil entry in
 * readTest.Errors is a must-fail case (Go's Reader is record-streaming and can
 * report an error on record N; @tabnas/csv parses a whole document, so any
 * record error means the document must be rejected).
 *
 * EXCLUSIONS. Two classes are dropped, and both are counted and written into
 * cases.json so the test can report them. Nothing is dropped for failing.
 *   - LazyQuotes: true. That flag selects a deliberately non-RFC-4180 lenient
 *     mode ("a bare quote may appear in an unquoted field"). @tabnas/csv has
 *     no such mode, so there is no behaviour to assert either way.
 *   - Cases with no Input at all (the errInvalidDelim group). They assert that
 *     Go's NewReader rejects a bad Comma/Comment *rune*; they are an API
 *     validation test, not a document.
 */

import fs from 'node:fs'
import path from 'node:path'

const [, , inFile, outFile] = process.argv
if (!inFile || !outFile) {
  console.error('usage: extract-go-csv-cases.mjs <reader_test.go> <cases.json>')
  process.exit(2)
}

const src = fs.readFileSync(inFile, 'utf8')

// ---------------------------------------------------------------- lexer ----

// Go source is scanned literal-by-literal. Only the subset of Go syntax that
// actually occurs inside the readTests table is supported; anything else is a
// hard error, so a change upstream cannot be silently mis-read.

const ESC = {
  a: '\x07', b: '\b', f: '\f', n: '\n', r: '\r', t: '\t', v: '\v',
  '\\': '\\', "'": "'", '"': '"',
}

class Cursor {
  constructor(s, i) { this.s = s; this.i = i }
  err(msg) {
    const line = this.s.slice(0, this.i).split('\n').length
    throw new Error(`${msg} at ${inFile}:${line}`)
  }
  ws() {
    for (;;) {
      const c = this.s[this.i]
      if (c === ' ' || c === '\t' || c === '\n' || c === '\r') { this.i++; continue }
      if (c === '/' && this.s[this.i + 1] === '/') {
        const nl = this.s.indexOf('\n', this.i)
        this.i = nl < 0 ? this.s.length : nl
        continue
      }
      if (c === '/' && this.s[this.i + 1] === '*') {
        const end = this.s.indexOf('*/', this.i)
        if (end < 0) this.err('unterminated block comment')
        this.i = end + 2
        continue
      }
      return
    }
  }
  peek() { this.ws(); return this.s[this.i] }
  eat(tok) {
    this.ws()
    if (!this.s.startsWith(tok, this.i)) this.err(`expected ${JSON.stringify(tok)}`)
    this.i += tok.length
  }
  tryEat(tok) {
    this.ws()
    if (this.s.startsWith(tok, this.i)) { this.i += tok.length; return true }
    return false
  }
  ident() {
    this.ws()
    const m = /^[A-Za-z_][A-Za-z0-9_.]*/.exec(this.s.slice(this.i))
    if (!m) this.err('expected identifier')
    this.i += m[0].length
    return m[0]
  }

  // Interpreted string literal: "..." with Go escapes.
  interpString() {
    this.eat('"')
    let out = ''
    for (;;) {
      const c = this.s[this.i]
      if (c === undefined) this.err('unterminated string')
      if (c === '"') { this.i++; return out }
      if (c !== '\\') { out += c; this.i++; continue }
      this.i++
      const e = this.s[this.i++]
      if (e in ESC) { out += ESC[e]; continue }
      if (e === 'x') {
        out += String.fromCharCode(parseInt(this.s.substr(this.i, 2), 16))
        this.i += 2
        continue
      }
      if (e === 'u') {
        out += String.fromCharCode(parseInt(this.s.substr(this.i, 4), 16))
        this.i += 4
        continue
      }
      if (e === 'U') {
        out += String.fromCodePoint(parseInt(this.s.substr(this.i, 8), 16))
        this.i += 8
        continue
      }
      if (e >= '0' && e <= '7') {
        out += String.fromCharCode(parseInt(this.s.substr(this.i - 1, 3), 8))
        this.i += 2
        continue
      }
      this.err(`unsupported string escape \\${e}`)
    }
  }

  // Raw string literal: `...`. No escapes; carriage returns are discarded
  // (Go spec: "carriage return characters ('\r') inside raw string literals
  // are discarded from the raw string value").
  rawString() {
    this.eat('`')
    const end = this.s.indexOf('`', this.i)
    if (end < 0) this.err('unterminated raw string')
    const out = this.s.slice(this.i, end).replace(/\r/g, '')
    this.i = end + 1
    return out
  }

  // Rune literal: 'x' with Go escapes. Returns the character.
  rune() {
    this.eat("'")
    let out
    if (this.s[this.i] === '\\') {
      this.i++
      const e = this.s[this.i++]
      if (e in ESC) out = ESC[e]
      else if (e === 'x') { out = String.fromCharCode(parseInt(this.s.substr(this.i, 2), 16)); this.i += 2 }
      else if (e === 'u') { out = String.fromCharCode(parseInt(this.s.substr(this.i, 4), 16)); this.i += 4 }
      else if (e === 'U') { out = String.fromCodePoint(parseInt(this.s.substr(this.i, 8), 16)); this.i += 8 }
      else this.err(`unsupported rune escape \\${e}`)
    } else {
      out = String.fromCodePoint(this.s.codePointAt(this.i))
      this.i += out.length
    }
    this.eat("'")
    return out
  }

  // A string-typed expression: literals, strings.Repeat(s, n), and `+`.
  stringExpr() {
    let out = this.stringTerm()
    while (this.tryEat('+')) out += this.stringTerm()
    return out
  }
  stringTerm() {
    const c = this.peek()
    if (c === '"') return this.interpString()
    if (c === '`') return this.rawString()
    const id = this.ident()
    if (id === 'strings.Repeat') {
      this.eat('(')
      const s = this.stringExpr()
      this.eat(',')
      const n = this.number()
      this.eat(')')
      return s.repeat(n)
    }
    this.err(`unsupported string expression ${id}`)
  }

  number() {
    this.ws()
    const m = /^-?\d+/.exec(this.s.slice(this.i))
    if (!m) this.err('expected number')
    this.i += m[0].length
    return parseInt(m[0], 10)
  }

  // [][]string{ {"a","b"}, {"c"} }
  rowsLiteral() {
    this.eat('[][]string')
    this.eat('{')
    const rows = []
    while (!this.tryEat('}')) {
      if (this.tryEat('{')) {
        const row = []
        while (!this.tryEat('}')) {
          row.push(this.stringExpr())
          this.tryEat(',')
        }
        rows.push(row)
      } else {
        this.err('expected row literal')
      }
      this.tryEat(',')
    }
    return rows
  }

  // []error{ nil, &ParseError{Err: ErrQuote} }
  errorsLiteral() {
    this.eat('[]error')
    this.eat('{')
    const errs = []
    while (!this.tryEat('}')) {
      if (this.tryEat('nil')) {
        errs.push(null)
      } else if (this.tryEat('&ParseError')) {
        this.eat('{')
        this.eat('Err')
        this.eat(':')
        errs.push(this.ident())
        this.eat('}')
      } else {
        errs.push(this.ident()) // e.g. errInvalidDelim
      }
      this.tryEat(',')
    }
    return errs
  }
}

// -------------------------------------------------------------- extract ----

const START = 'var readTests = []readTest{'
const startAt = src.indexOf(START)
if (startAt < 0) {
  throw new Error(`could not find "${START}" in ${inFile} - upstream layout changed`)
}

const cur = new Cursor(src, startAt + START.length)
const raw = []

// The table is written as `{...}, {...}}` - the outer brace was consumed above.
for (;;) {
  cur.eat('{')
  const rec = {}
  while (!cur.tryEat('}')) {
    const field = cur.ident()
    cur.eat(':')
    switch (field) {
      case 'Name': rec.Name = cur.stringExpr(); break
      case 'Input': rec.Input = cur.stringExpr(); break
      case 'Output': rec.Output = cur.rowsLiteral(); break
      case 'Errors': rec.Errors = cur.errorsLiteral(); break
      case 'Positions': cur.err('Positions field is not supported'); break
      case 'Comma': rec.Comma = cur.peek() === "'" ? cur.rune() : cur.ident(); break
      case 'Comment': rec.Comment = cur.peek() === "'" ? cur.rune() : cur.ident(); break
      case 'FieldsPerRecord': rec.FieldsPerRecord = cur.number(); break
      case 'UseFieldsPerRecord': rec.UseFieldsPerRecord = cur.ident() === 'true'; break
      case 'LazyQuotes': rec.LazyQuotes = cur.ident() === 'true'; break
      case 'TrimLeadingSpace': rec.TrimLeadingSpace = cur.ident() === 'true'; break
      case 'ReuseRecord': rec.ReuseRecord = cur.ident() === 'true'; break
      default: cur.err(`unknown readTest field ${field}`)
    }
    cur.tryEat(',')
  }
  raw.push(rec)
  if (cur.tryEat(',')) {
    if (cur.tryEat('}')) break // trailing `}}` closes the slice
    continue
  }
  cur.eat('}')
  break
}

// ---------------------------------------------------------------- shape ----

// Strip the position markers documented in reader_test.go: they mark field
// starts, record boundaries and error positions and are removed before parse.
const MARKERS = /[§¶∑]/g

const cases = []
const excluded = []

for (const r of raw) {
  const name = r.Name
  if (r.Input === undefined) {
    excluded.push({ name, reason: 'no-input (errInvalidDelim: Go NewReader rune validation, not a document)' })
    continue
  }
  if (r.LazyQuotes) {
    excluded.push({ name, reason: 'LazyQuotes: deliberately non-RFC-4180 lenient mode; @tabnas/csv has no equivalent' })
    continue
  }

  const input = r.Input.replace(MARKERS, '')
  const errs = r.Errors || []
  const mustFail = errs.some((e) => e !== null)

  // Map the Go Reader configuration onto @tabnas/csv options. Output is
  // [][]string, so header/object are always off.
  const opts = { header: false, object: false }
  const notes = []
  if (r.Comma !== undefined && r.Comma !== ',') {
    opts.field = Object.assign({}, opts.field, { separation: r.Comma })
    notes.push('Comma')
  }
  if (r.TrimLeadingSpace) { opts.trim = true; notes.push('TrimLeadingSpace->trim(both sides)') }
  if (r.UseFieldsPerRecord) {
    opts.field = Object.assign({}, opts.field, { exact: true })
    notes.push('FieldsPerRecord->field.exact')
  }
  // Go's Reader.Comment is a configurable rune. @tabnas/csv turns comment
  // support on with { comment: true } and takes the marker itself from the
  // engine's comment definition (the same route test/fixtures/manifest.json
  // uses for "papa-comment-with-non-default-character").
  let jsonicOpts = null
  if (r.Comment !== undefined) {
    opts.comment = true
    jsonicOpts = { comment: { def: { hash: { start: r.Comment } } } }
    notes.push('Comment=' + JSON.stringify(r.Comment))
  }

  cases.push({
    name,
    input,
    mustFail,
    // Go emits no record for an all-blank document; our [] is the same thing.
    expected: mustFail ? null : (r.Output || []),
    opts,
    jsonicOpts,
    // "default" == the RFC 4180 reader configuration, no Go-specific flags.
    profile: notes.length === 0 ? 'default' : 'configured',
    notes,
  })
}

const out = {
  source: 'golang/go src/encoding/csv/reader_test.go (readTests)',
  generatedBy: 'scripts/extract-go-csv-cases.mjs',
  rawCount: raw.length,
  cases,
  excluded,
}

fs.mkdirSync(path.dirname(outFile), { recursive: true })
fs.writeFileSync(outFile, JSON.stringify(out, null, 2) + '\n')

const dflt = cases.filter((c) => c.profile === 'default')
console.log(
  `extract-go-csv-cases: ${raw.length} readTests -> ${cases.length} cases ` +
  `(${cases.filter((c) => c.mustFail).length} must-fail, ` +
  `${dflt.length} default-profile), ${excluded.length} excluded`,
)

// A silently-shrinking corpus is the failure mode this whole exercise exists
// to stop. The counts below are pinned to the fetched commit; if upstream
// changes, this aborts rather than quietly measuring less.
const EXPECT = { raw: 68, cases: 55, mustFail: 12, excluded: 13 }
const got = {
  raw: raw.length,
  cases: cases.length,
  mustFail: cases.filter((c) => c.mustFail).length,
  excluded: excluded.length,
}
for (const k of Object.keys(EXPECT)) {
  if (EXPECT[k] !== got[k]) {
    throw new Error(
      `corpus size drift: expected ${k}=${EXPECT[k]}, got ${got[k]}. ` +
      `The pinned upstream commit changed, or this extractor is wrong. ` +
      `Do NOT relax this check to get green - investigate and re-pin.`,
    )
  }
}
