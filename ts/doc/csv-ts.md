# @jsonic/csv — TypeScript

A [Jsonic](https://jsonic.senecajs.org) syntax plugin that parses
CSV text into JavaScript values. Headers, quoted fields, custom
delimiters, streaming, and a strict / non-strict mode are all
supported.

This document follows the [Diataxis](https://diataxis.fr) framework:
a guided **tutorial** for first-time use, focused **how-to guides**
for common problems, complete **reference** material, and
**explanation** of the design.

---

## Contents

- [Install](#install)
- [Tutorial — your first parse](#tutorial--your-first-parse)
- [How-to guides](#how-to-guides)
- [Reference](#reference)
- [Explanation](#explanation)

---

## Install

```bash
npm install @jsonic/csv jsonic
```

`jsonic` (>= 2) is a peer dependency. The plugin re-uses Jsonic's
lexer and parser, so you always create a Jsonic instance and
register the plugin on it.

```typescript
import { Jsonic } from 'jsonic'
import { Csv } from '@jsonic/csv'

const parse = Jsonic.make().use(Csv)
```

`parse` is now a function that accepts a CSV string and returns
the parsed result. Reuse it for as many inputs as you need — each
call is independent.

---

## Tutorial — your first parse

This tutorial walks through parsing CSV end-to-end: header rows,
quoted fields, type coercion, and finally a streaming pipeline.
By the end you will have written the four shapes of program you
are likely to need in real code.

### 1. A two-line CSV

Make a Jsonic instance, register the plugin, and call it with a
string:

```typescript
import { Jsonic } from 'jsonic'
import { Csv } from '@jsonic/csv'

const parse = Jsonic.make().use(Csv)

const out = parse('name,age\nAlice,30\nBob,25')
// [
//   { name: 'Alice', age: '30' },
//   { name: 'Bob', age: '25' },
// ]
```

The first row was treated as a header (this is the default), and
each subsequent row became an object keyed by those names. Note
that `30` and `25` are *strings* — strict mode is on, and strict
mode keeps every field as the raw text it appeared as.

### 2. Turn the numbers into numbers

Strict mode treats CSV as data, not Jsonic source. To opt in to
type coercion, enable `number` (and optionally `value` for the
literals `true`, `false`, `null`):

```typescript
const parse = Jsonic.make().use(Csv, { number: true, value: true })

parse('name,age,active\nAlice,30,true\nBob,25,false')
// [
//   { name: 'Alice', age: 30, active: true },
//   { name: 'Bob',   age: 25, active: false },
// ]
```

These options are independent — turn on whichever ones the data
calls for.

### 3. Quoted fields with commas and newlines

CSV's quote rules are the bit everybody has to deal with eventually.
Wrap a field in `"` to include commas, newlines, or quotes in the
value; double a quote (`""`) to escape it.

```typescript
const parse = Jsonic.make().use(Csv)

parse('name,bio\nAlice,"Likes ""cats"" and dogs"\nBob,"line 1\nline 2"')
// [
//   { name: 'Alice', bio: 'Likes "cats" and dogs' },
//   { name: 'Bob',   bio: 'line 1\nline 2' },
// ]
```

The plugin ships its own quoted-string lexer to follow RFC 4180
quoting precisely (see *Explanation: Quoted fields*).

### 4. Stream a large file

If the input is too big to hold in memory, supply a `stream`
callback. The plugin emits one event per record and the top-level
result is `[]`:

```typescript
import { createReadStream } from 'node:fs'

const parse = Jsonic.make().use(Csv, {
  number: true,
  stream: (event, payload) => {
    if (event === 'record') {
      // payload is one parsed record
      console.log(payload)
    } else if (event === 'error') {
      console.error(payload)
    }
  },
})

// Parse the whole string in one shot — records flow through the
// callback as they are produced.
parse('a,b\n1,2\n3,4')
```

You now have the four shapes — basic parse, type coercion, quoted
data, streaming. Everything else in this document is variations
on these.

---

## How-to guides

Short recipes for specific problems. Each is independent; pick
the one that matches the question you have right now.

### Return arrays instead of objects

Set `object: false` to receive each record as a `string[]`. With
`header: true` (the default), the header row is consumed for
internal field tracking and not emitted:

```typescript
const parse = Jsonic.make().use(Csv, { object: false })

parse('a,b,c\n1,2,3\n4,5,6')
// [['1','2','3'], ['4','5','6']]
```

To get every row out as an array, including the first, also set
`header: false`:

```typescript
const parse = Jsonic.make().use(Csv, { header: false, object: false })

parse('a,b,c\n1,2,3\n4,5,6')
// [['a','b','c'], ['1','2','3'], ['4','5','6']]
```

### Parse a file with no header row

If your CSV has no header at all, use `header: false`. Combined
with `object: false` you get plain arrays:

```typescript
const parse = Jsonic.make().use(Csv, { header: false, object: false })

parse('1,2,3\n4,5,6')
// [['1','2','3'], ['4','5','6']]
```

With the default `object: true` and no field names supplied, the
plugin invents keys (`field~0`, `field~1`, …):

```typescript
const parse = Jsonic.make().use(Csv, { header: false })

parse('1,2,3')
// [{ 'field~0': '1', 'field~1': '2', 'field~2': '3' }]
```

If you still want object output but with names you supply, use
`field.names`:

```typescript
const parse = Jsonic.make().use(Csv, {
  header: false,
  field: { names: ['x', 'y', 'z'] },
})

parse('1,2,3\n4,5,6')
// [{ x: '1', y: '2', z: '3' }, { x: '4', y: '5', z: '6' }]
```

### Use a different field delimiter

Tab-separated, pipe-separated, or anything else:

```typescript
const parse = Jsonic.make().use(Csv, { field: { separation: '\t' } })

parse('name\tage\nAlice\t30')
// [{ name: 'Alice', age: '30' }]
```

The separator can be more than one character:

```typescript
const parse = Jsonic.make().use(Csv, { field: { separation: '~~' } })

parse('a~~b\n1~~2')
// [{ a: '1', b: '2' }]
```

### Use a different record delimiter

By default a record ends at `\n`, `\r\n`, or `\r`. Override with
`record.separators`:

```typescript
const parse = Jsonic.make().use(Csv, { record: { separators: '%' } })

parse('a,b%1,2%3,4')
// [{ a: '1', b: '2' }, { a: '3', b: '4' }]
```

### Trim surrounding whitespace from fields

```typescript
const parse = Jsonic.make().use(Csv, { trim: true })

parse('a,b\n  hello  ,  world  ')
// [{ a: 'hello', b: 'world' }]
```

Internal whitespace is preserved — `'  hello world  '` trims to
`'hello world'`.

### Skip comment lines

Enable `comment` to strip lines starting with `#`:

```typescript
const parse = Jsonic.make().use(Csv, { comment: true })

parse('a,b\n# this row is ignored\n1,2')
// [{ a: '1', b: '2' }]
```

A `#` *inside* a field is left alone unless it follows whitespace
on its own — see *Explanation: Comments and whitespace*.

### Preserve blank lines as empty records

Blank lines are skipped by default. To keep them:

```typescript
const parse = Jsonic.make().use(Csv, { record: { empty: true } })

parse('a\n1\n\n2')
// [{ a: '1' }, { a: '' }, { a: '2' }]
```

### Substitute a value for empty fields

Use `field.empty` for the placeholder. Any value works, including
`null` or a sentinel:

```typescript
const parse = Jsonic.make().use(Csv, { field: { empty: null } })

parse('a,b,c\n1,,3')
// [{ a: '1', b: null, c: '3' }]
```

### Reject rows with the wrong number of fields

`field.exact: true` errors when a record's field count doesn't
match the header's:

```typescript
const parse = Jsonic.make().use(Csv, { field: { exact: true } })

parse('a,b\n1,2,3')
// throws: 'unexpected extra field value: 3'

parse('a,b\n1')
// throws: 'missing field'
```

### Allow JSON values inside fields

In strict mode, `[1,2]` and `{x:1}` are just text. Switch to
non-strict mode and Jsonic re-engages — you get the JSON value
back as a parsed JavaScript value:

```typescript
const parse = Jsonic.make().use(Csv, { strict: false })

parse('a,b,c\ntrue,[1,2],{x:{y:"q"}}')
// [{ a: true, b: [1, 2], c: { x: { y: 'q' } } }]
```

Non-strict mode also enables `trim`, `comment`, and `number` by
default. See *Explanation: Strict vs non-strict*.

### Stream records to a callback

For large inputs, set `stream`. The callback fires for `start`,
`record`, `end`, and `error`. The parse function returns `[]`
because records are no longer collected:

```typescript
const records: any[] = []

const parse = Jsonic.make().use(Csv, {
  stream: (what, payload) => {
    if (what === 'record') records.push(payload)
  },
})

parse('a,b\n1,2\n3,4')
// records: [{ a: '1', b: '2' }, { a: '3', b: '4' }]
```

### Use a different quote character

```typescript
const parse = Jsonic.make().use(Csv, { string: { quote: "'" } })

parse("a,b\n'hi, there','x'")
// [{ a: 'hi, there', b: 'x' }]
```

### Reuse the same parser for many inputs

The result of `Jsonic.make().use(Csv, opts)` is a parser function
that is fully reusable — there is no per-call cost beyond the
parse itself:

```typescript
const parse = Jsonic.make().use(Csv, { number: true })

const a = parse('x,y\n1,2')
const b = parse('p,q\n3,4')
```

---

## Reference

### `Csv: Plugin`

```typescript
import { Csv } from '@jsonic/csv'
const parse = Jsonic.make().use(Csv, options?)
```

The Jsonic plugin function. Pass it (and an optional options
object) to `Jsonic.use()`. The plugin merges your options on top
of `Csv.defaults`, so you only specify what differs.

### `CsvOptions`

All keys are optional.

| Key | Type | Default | Notes |
|---|---|---|---|
| `trim` | `boolean \| null` | `null` | Trim leading/trailing whitespace from fields. `null` resolves to `false` in strict mode and `true` in non-strict. |
| `comment` | `boolean \| null` | `null` | Strip `#` comments. `null` resolves to `false` in strict / `true` in non-strict. |
| `number` | `boolean \| null` | `null` | Parse numeric literals into `number`. `null` resolves to `false` strict / `true` non-strict. |
| `value` | `boolean \| null` | `null` | Parse the literals `true`, `false`, `null` into their JS values. `null` resolves to `false` in both modes. |
| `header` | `boolean` | `true` | Treat the first record as field names. |
| `object` | `boolean` | `true` | Emit each record as `Record<string, any>`; if `false`, emit `string[]`. |
| `stream` | `(what, payload?) => void \| null` | `null` | Streaming callback. See [Streaming callback](#streaming-callback). |
| `strict` | `boolean` | `true` | Disable Jsonic syntax inside fields. |

`field` (nested):

| Key | Type | Default | Notes |
|---|---|---|---|
| `field.separation` | `string \| null` | `null` | Field delimiter. `null` keeps `,`. May be more than one character. |
| `field.nonameprefix` | `string` | `'field~'` | Prefix used when a record has more fields than the header has names: extra columns are emitted as `field~N`. |
| `field.empty` | `any` | `''` | Value substituted for empty fields. |
| `field.names` | `string[] \| undefined` | `undefined` | Explicit field names. Used for object output when `header: false`. |
| `field.exact` | `boolean` | `false` | If `true`, error when a record's field count differs from the header's. |

`record` (nested):

| Key | Type | Default | Notes |
|---|---|---|---|
| `record.separators` | `string \| null` | `null` | Custom record-separator characters. `null` keeps `\n`/`\r\n`/`\r`. |
| `record.empty` | `boolean` | `false` | Preserve blank lines as empty records. |

`string` (nested):

| Key | Type | Default | Notes |
|---|---|---|---|
| `string.quote` | `string` | `'"'` | Quote character for the CSV string lexer. |
| `string.csv` | `boolean \| null` | `null` | Force the CSV string lexer on (`true`) or off (`false`). `null` is auto: on in strict, off in non-strict. |

The exported type signature:

```typescript
type CsvOptions = {
  trim: boolean | null
  comment: boolean | null
  number: boolean | null
  value: boolean | null
  header: boolean
  object: boolean
  stream: null | ((what: string, record?: Record<string, any> | Error) => void)
  strict: boolean
  field: {
    separation: null | string
    nonameprefix: string
    empty: any
    names: undefined | string[]
    exact: boolean
  }
  record: {
    separators: null | string
    empty: boolean
  }
  string: {
    quote: string
    csv: null | boolean
  }
}
```

### Return value

Calling the configured parser returns:

- `Record<string, any>[]` when `object: true` (default)
- `any[][]` when `object: false`
- `[]` when `stream` is set (records are emitted to the callback)
- `[]` for empty input

### Streaming callback

```typescript
type StreamFn =
  (what: string, record?: Record<string, any> | Error) => void
```

`what` is one of:

| Event     | Payload                                  |
|-----------|------------------------------------------|
| `'start'` | `undefined`                              |
| `'record'`| The parsed record (object or array)      |
| `'end'`   | `undefined`                              |
| `'error'` | An `Error` thrown during parsing         |

Errors thrown inside the parser are forwarded to the callback and
not re-thrown.

### Errors

Field-count violations under `field.exact` raise these error
codes (visible on the thrown Jsonic error's `code`):

| Code                | Message                                  |
|---------------------|------------------------------------------|
| `csv_extra_field`   | `unexpected extra field value: <src>`    |
| `csv_missing_field` | `missing field`                          |

Other errors come from Jsonic itself (e.g. `unterminated_string`).

### `buildCsvStringMatcher(opts)`

```typescript
import { buildCsvStringMatcher } from '@jsonic/csv'
```

Factory for the custom CSV double-quote string matcher. Exported
for advanced cases where you want to plug the CSV quote handling
into a non-default Jsonic instance manually. Most users never
need this.

---

## Explanation

### Why a Jsonic plugin

Jsonic is a configurable parser whose grammar can be extended at
runtime. CSV is not Jsonic syntax, but it lexes and parses
naturally with the same machinery once a few rules are added. The
plugin route means CSV parsing reuses Jsonic's tokenizer,
error-reporting, comment handling, and streaming hooks rather than
re-implementing them.

It also opens the door to *non-strict* mode, where a CSV cell can
contain a Jsonic value (an object, an array, a string with
backslash escapes). That capability is unique to this plugin.

### Strict vs non-strict mode

**Strict mode (default).** The plugin disables Jsonic's value
parsing. Every field is the raw text of the cell, and CSV's RFC
4180-style double-quote escaping is applied. Numbers, booleans,
and null literals stay as strings unless you explicitly enable
`number` or `value`. This is what you want for "normal" CSV.

**Non-strict mode** (`strict: false`). Field bodies are parsed
*as Jsonic*. So `[1,2]` becomes the array `[1, 2]`, `{x:1}`
becomes `{ x: 1 }`, and quoted strings honour Jsonic's escape
rules rather than CSV's. To make this convenient, non-strict mode
also flips `trim`, `comment`, and `number` on by default. The
trade-off is that pure-CSV quirks (unescaped quotes, certain
malformed cells) may no longer be tolerated.

In practice: use strict for ingesting CSV from the outside world.
Use non-strict when the file is your own and you want richer
in-cell types without inventing a new format.

### Quoted fields (RFC 4180)

In strict mode the plugin installs a custom string matcher that
follows RFC 4180:

- A quoted field starts with `"` *at the beginning of a field*
  (i.e. directly after a delimiter, line break, or start of input)
  and continues until a matching `"`.
- A literal `"` inside the field is written `""`.
- The quoted body may contain commas, line breaks, and any other
  character that would otherwise be significant.

So `"a""b"` lexes to the value `a"b`, and a multi-line quoted
field is one record's worth of one field.

The quote character can be changed via `string.quote`.

### Comments and whitespace

`comment: true` enables Jsonic's standard `#` line-comment lexer.
Two subtleties:

1. A line that is *only* a comment is dropped before record
   processing. With `record.empty: true`, dropped lines do not
   become empty records.
2. A `#` *inside* a field is treated as text until preceded by
   whitespace. So `1,#x` keeps `#x` (the lexer sees `#` as the
   start of a value token), while `1, #x` strips `#x`.

`trim: true` removes leading and trailing whitespace from each
field's *value*, not from the whole line. Internal runs of
whitespace are preserved.

### Streaming model

When `stream` is set:

- The parser still consumes the whole input string in one call
  (this is a string parser, not a chunk parser).
- Records are emitted to the callback as they are completed,
  rather than collected into an array.
- The top-level call returns `[]`.

This is useful when you want to process millions of records
without holding them all in memory at once. To consume an
arbitrarily large file, read it as a string (or as chunks
joined into a string), and let `stream` drain the records into
your downstream sink.

The callback also receives `'error'` events instead of the parser
throwing — wrap accordingly.

### Object key ordering and the `field~N` prefix

When `object: true`:

- Header names are used as keys, in their declared order.
- If a record has *more* fields than the header has names, the
  excess columns become keys `field~N` (where `N` is the column
  index). The prefix is configurable via `field.nonameprefix`.
- If a record has *fewer* fields, missing keys take
  `field.empty`.

Set `field.exact: true` to make field-count mismatches errors
instead.

### Relationship to the grammar file

This plugin and the [Go implementation](csv-go.md) share
`csv-grammar.jsonic` (embedded into both source files at build
time). The grammar file declares the `csv`, `newline`, `record`,
and `text` rules together with static options (`rule.start`,
`lex.emptyResult`, error codes, hint templates).

The `list`, `elem`, and `val` rules are configured in code rather
than in the grammar file because *non-strict* mode must preserve
Jsonic's default alternatives for those rules in order to support
embedded JSON. Putting them in code keeps the strict and
non-strict variants on the same path.

If you want to study the grammar, read `csv-grammar.jsonic` —
it is a single page of declarative rules.
