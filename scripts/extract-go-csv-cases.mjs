#!/usr/bin/env node
/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// Extract the `readTests` corpus from Go's `src/encoding/csv/reader_test.go`
// into `test/suites/go-encoding-csv/cases.json`, the runtime-neutral form
// that both `ts/test/conformance.test.ts` and `go/conformance_test.go` read.
//
// The upstream file is a Go source file, so it is parsed as one: a small
// tokenizer plus a recursive-descent reader for composite literals. A regex
// pass over 450 lines of nested literals with raw strings, rune literals and
// `&ParseError{...}` values would be guesswork; this is not.
//
// Usage: node scripts/extract-go-csv-cases.mjs <reader_test.go> <out.json>
//
// The Go reader's knobs are mapped onto @tabnas/csv options as follows, and
// every mapping is recorded in the case's `notes` so the conformance report
// can say WHY a case is configured the way it is:
//
//   Comma              -> field.separation
//   Comment            -> comment:true + engine comment.def.hash.start
//   TrimLeadingSpace   -> trim:true          (see NOTE below)
//   UseFieldsPerRecord -> field.exact:true   (see NOTE below)
//   LazyQuotes         -> case EXCLUDED (deliberately non-RFC-4180 mode)
//   ReuseRecord        -> ignored (a Go allocation detail, not a document)
//
// NOTE: neither `trim` nor `field.exact` is an exact analogue of the Go knob
// it stands in for — Go trims leading space only, and Go's FieldsPerRecord
// counts fields without needing a header. The mismatch is deliberate: it is
// better to run the case and record the divergence than to drop it.

import { readFileSync, writeFileSync } from 'node:fs'

// --------------------------------------------------------------------------
// Tokenizer
// --------------------------------------------------------------------------

const PUNCT = new Set(['{', '}', '[', ']', '(', ')', ',', ':', '&', '*'])

function tokenize(src) {
  const toks = []
  let i = 0
  const n = src.length

  while (i < n) {
    const c = src[i]

    // Whitespace.
    if (' \t\r\n'.includes(c)) {
      i++
      continue
    }

    // Line comment.
    if ('/' === c && '/' === src[i + 1]) {
      while (i < n && '\n' !== src[i]) i++
      continue
    }

    // Block comment.
    if ('/' === c && '*' === src[i + 1]) {
      i += 2
      while (i < n && !('*' === src[i] && '/' === src[i + 1])) i++
      i += 2
      continue
    }

    // Interpreted string.
    if ('"' === c) {
      const [val, next] = readInterpretedString(src, i)
      toks.push({ t: 'str', v: val })
      i = next
      continue
    }

    // Raw string. Go drops carriage returns inside raw strings.
    if ('`' === c) {
      const end = src.indexOf('`', i + 1)
      if (-1 === end) throw new Error('unterminated raw string at ' + i)
      toks.push({ t: 'str', v: src.slice(i + 1, end).replace(/\r/g, '') })
      i = end + 1
      continue
    }

    // Rune literal.
    if ("'" === c) {
      const [val, next] = readRune(src, i)
      toks.push({ t: 'rune', v: val })
      i = next
      continue
    }

    // Number.
    if (/[0-9]/.test(c) || ('-' === c && /[0-9]/.test(src[i + 1]))) {
      let j = i + 1
      while (j < n && /[0-9xXa-fA-F_.]/.test(src[j])) j++
      toks.push({ t: 'num', v: Number(src.slice(i, j).replace(/_/g, '')) })
      i = j
      continue
    }

    // Identifier (dotted qualifiers kept as one token: io.ErrUnexpectedEOF).
    if (/[A-Za-z_]/.test(c)) {
      let j = i + 1
      while (j < n && /[A-Za-z0-9_.]/.test(src[j])) j++
      toks.push({ t: 'ident', v: src.slice(i, j) })
      i = j
      continue
    }

    if (PUNCT.has(c)) {
      toks.push({ t: 'punct', v: c })
      i++
      continue
    }

    // Anything else (operators inside expressions we do not evaluate).
    toks.push({ t: 'punct', v: c })
    i++
  }

  return toks
}

function readInterpretedString(src, i) {
  let out = ''
  i++ // opening quote
  while (i < src.length) {
    const c = src[i]
    if ('"' === c) return [out, i + 1]
    if ('\\' !== c) {
      out += c
      i++
      continue
    }
    const e = src[i + 1]
    i += 2
    switch (e) {
      case 'n': out += '\n'; break
      case 'r': out += '\r'; break
      case 't': out += '\t'; break
      case 'a': out += '\x07'; break
      case 'b': out += '\b'; break
      case 'f': out += '\f'; break
      case 'v': out += '\v'; break
      case '\\': out += '\\'; break
      case "'": out += "'"; break
      case '"': out += '"'; break
      case 'x':
        out += String.fromCharCode(parseInt(src.slice(i, i + 2), 16))
        i += 2
        break
      case 'u':
        out += String.fromCodePoint(parseInt(src.slice(i, i + 4), 16))
        i += 4
        break
      case 'U':
        out += String.fromCodePoint(parseInt(src.slice(i, i + 8), 16))
        i += 8
        break
      default:
        if (/[0-7]/.test(e)) {
          out += String.fromCharCode(parseInt(src.slice(i - 1, i + 2), 8))
          i += 2
        } else {
          throw new Error('unhandled string escape \\' + e)
        }
    }
  }
  throw new Error('unterminated interpreted string')
}

function readRune(src, i) {
  i++ // opening quote
  if ('\\' === src[i]) {
    const [val, next] = readInterpretedString('"' + src.slice(i) + '"', 0)
    // readInterpretedString stops at the first unescaped quote, which for a
    // rune body is the closing `'` we replaced. Recover the length instead.
    void val
    void next
    // Simple path: handle the escapes Go rune literals actually use here.
    const e = src[i + 1]
    const map = { n: '\n', r: '\r', t: '\t', '\\': '\\', "'": "'", '"': '"' }
    if (map[e]) return [map[e], i + 3]
    throw new Error('unhandled rune escape \\' + e)
  }
  const cp = String.fromCodePoint(src.codePointAt(i))
  return [cp, i + cp.length + 1]
}

// --------------------------------------------------------------------------
// Composite-literal reader
// --------------------------------------------------------------------------

class Reader {
  constructor(toks) {
    this.toks = toks
    this.i = 0
  }
  peek(k = 0) {
    return this.toks[this.i + k]
  }
  next() {
    return this.toks[this.i++]
  }
  expect(v) {
    const t = this.next()
    if (!t || t.v !== v) {
      throw new Error(`expected ${v} got ${t && t.v} at token ${this.i}`)
    }
    return t
  }

  // Consume a type expression sitting in front of a `{`: [], [2], [][]string,
  // readTest, ParseError, ... Stops with `{` as the next token.
  skipType() {
    for (;;) {
      const t = this.peek()
      if (!t) return
      if ('punct' === t.t && '[' === t.v) {
        this.next()
        let depth = 1
        while (depth > 0) {
          const u = this.next()
          if ('punct' === u.t && '[' === u.v) depth++
          if ('punct' === u.t && ']' === u.v) depth--
        }
        continue
      }
      if ('ident' === t.t) {
        this.next()
        continue
      }
      if ('punct' === t.t && '*' === t.v) {
        this.next()
        continue
      }
      return
    }
  }

  // A value may be a `+` chain of string-producing primaries, e.g.
  // `strings.Repeat("#ignore\n", 10000) + "§" + ...` (HugeLines).
  value() {
    let v = this.primary()
    while (this.peek() && 'punct' === this.peek().t && '+' === this.peek().v) {
      this.next()
      const rhs = this.primary()
      if ('string' !== typeof v || 'string' !== typeof rhs) {
        throw new Error('cannot evaluate non-string `+` in corpus literal')
      }
      v += rhs
    }
    return v
  }

  primary() {
    const t = this.peek()
    if (!t) throw new Error('unexpected end of tokens')

    if ('punct' === t.t && '&' === t.v) {
      this.next()
      return this.value()
    }
    if ('punct' === t.t && '{' === t.v) return this.braceGroup()
    if ('punct' === t.t && ('[' === t.v || '*' === t.v)) {
      this.skipType()
      return this.braceGroup()
    }
    if ('str' === t.t || 'rune' === t.t) {
      this.next()
      return t.v
    }
    if ('num' === t.t) {
      this.next()
      return t.v
    }
    if ('ident' === t.t) {
      // A named composite literal (ParseError{...}), a call, or a bare
      // identifier.
      const nt = this.peek(1)
      if (nt && 'punct' === nt.t && '{' === nt.v) {
        this.next()
        return this.braceGroup()
      }
      if (nt && 'punct' === nt.t && '(' === nt.v) {
        this.next()
        return this.call(t.v)
      }
      this.next()
      if ('nil' === t.v) return null
      if ('true' === t.v) return true
      if ('false' === t.v) return false
      return { $ident: t.v }
    }
    throw new Error('unhandled token ' + JSON.stringify(t))
  }

  // Evaluate the handful of stdlib calls the corpus uses to build inputs.
  call(fn) {
    this.expect('(')
    const args = []
    while (!('punct' === this.peek().t && ')' === this.peek().v)) {
      args.push(this.value())
      if ('punct' === this.peek().t && ',' === this.peek().v) this.next()
    }
    this.expect(')')
    if ('strings.Repeat' === fn) return args[0].repeat(args[1])
    throw new Error('unhandled call in corpus literal: ' + fn)
  }

  // `{ ... }` is either a keyed struct literal or an element list. Which one
  // is decided by the first entry: `Ident:` means keyed.
  braceGroup() {
    this.expect('{')
    const first = this.peek()
    const second = this.peek(1)
    const keyed =
      first &&
      'ident' === first.t &&
      second &&
      'punct' === second.t &&
      ':' === second.v

    if (keyed) {
      const obj = {}
      while (!('punct' === this.peek().t && '}' === this.peek().v)) {
        const key = this.next().v
        this.expect(':')
        obj[key] = this.value()
        if ('punct' === this.peek().t && ',' === this.peek().v) this.next()
      }
      this.expect('}')
      return obj
    }

    const arr = []
    while (!('punct' === this.peek().t && '}' === this.peek().v)) {
      arr.push(this.value())
      if ('punct' === this.peek().t && ',' === this.peek().v) this.next()
    }
    this.expect('}')
    return arr
  }
}

// --------------------------------------------------------------------------
// Extraction
// --------------------------------------------------------------------------

// Position markers upstream injects into Input to assert FieldPos. They are
// stripped before parsing, exactly as Go's own test harness does.
const MARKERS = /[§¶∑]/g

function extract(goSrc) {
  const at = goSrc.indexOf('var readTests = ')
  if (-1 === at) throw new Error('readTests not found')
  const toks = tokenize(goSrc.slice(at + 'var readTests = '.length))
  const rd = new Reader(toks)
  rd.skipType()
  return rd.braceGroup()
}

function mapCase(t) {
  const name = t.Name
  const notes = []
  const opts = { header: false, object: false }
  let jsonicOpts = null

  // Cases with no Input at all assert Go's NewReader rune validation, not a
  // document. Nothing to feed a parser.
  const hasInput = 'string' === typeof t.Input && '' !== t.Input.replace(MARKERS, '')

  if (t.LazyQuotes) {
    return {
      excluded: {
        name,
        reason:
          'LazyQuotes: deliberately non-RFC-4180 lenient mode; ' +
          '@tabnas/csv has no equivalent',
      },
    }
  }

  if (!hasInput && (t.Errors || undefined === t.Output)) {
    return {
      excluded: {
        name,
        reason:
          'no-input (errInvalidDelim: Go NewReader rune validation, not a document)',
      },
    }
  }

  if (t.Comma) {
    opts.field = Object.assign({}, opts.field, { separation: t.Comma })
    notes.push('Comma')
  }
  if (t.TrimLeadingSpace) {
    opts.trim = true
    notes.push('TrimLeadingSpace->trim(both sides)')
  }
  if (t.UseFieldsPerRecord) {
    opts.field = Object.assign({}, opts.field, { exact: true })
    notes.push('FieldsPerRecord->field.exact')
  }
  if (t.Comment) {
    opts.comment = true
    jsonicOpts = { comment: { def: { hash: { start: t.Comment } } } }
    notes.push('Comment="' + t.Comment + '"')
  }

  const mustFail = Array.isArray(t.Errors) && t.Errors.some((e) => null !== e)

  return {
    case: {
      name,
      input: (t.Input || '').replace(MARKERS, ''),
      mustFail,
      expected: mustFail ? null : t.Output || [],
      opts,
      jsonicOpts,
      profile:
        0 === notes.length && null === jsonicOpts ? 'default' : 'configured',
      notes,
    },
  }
}

function main() {
  const [, , inPath, outPath] = process.argv
  if (!inPath || !outPath) {
    console.error(
      'usage: extract-go-csv-cases.mjs <reader_test.go> <cases.json>',
    )
    process.exit(2)
  }

  const raw = extract(readFileSync(inPath, 'utf8'))
  const cases = []
  const excluded = []
  for (const t of raw) {
    const m = mapCase(t)
    if (m.excluded) excluded.push(m.excluded)
    else cases.push(m.case)
  }

  const out = {
    source: 'golang/go src/encoding/csv/reader_test.go (readTests)',
    generatedBy: 'scripts/extract-go-csv-cases.mjs',
    rawCount: raw.length,
    cases,
    excluded,
  }

  writeFileSync(outPath, JSON.stringify(out, null, 2) + '\n')
  console.error(
    `extracted ${cases.length} cases (${excluded.length} excluded) ` +
      `from ${raw.length} readTests -> ${outPath}`,
  )
}

main()
