# How-to guide (TypeScript)

Short recipes for specific problems. Each is independent; pick the one
that matches the question you have right now. For a guided
introduction, start with the [tutorial](tutorial.md); for exact
signatures and defaults, see the [reference](reference.md).

Every recipe builds on the same three lines:

```typescript
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, /* options */)
```

## Return arrays instead of objects

Set `object: false` to receive each record as a `string[]`. With
`header: true` (the default), the header row is consumed for internal
field tracking and not emitted:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { object: false })

parse.parse('a,b,c\n1,2,3\n4,5,6') // => [['1','2','3'], ['4','5','6']]
```

To get every row out as an array, including the first, also set
`header: false`:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { header: false, object: false })

parse.parse('a,b,c\n1,2,3\n4,5,6') // => [['a','b','c'], ['1','2','3'], ['4','5','6']]
```

## Parse a file with no header row

If your CSV has no header at all, use `header: false`. Combined with
`object: false` you get plain arrays:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { header: false, object: false })

parse.parse('1,2,3\n4,5,6') // => [['1','2','3'], ['4','5','6']]
```

With the default `object: true` and no field names supplied, the plugin
invents keys (`field~0`, `field~1`, …):

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { header: false })

parse.parse('1,2,3') // => [{ 'field~0': '1', 'field~1': '2', 'field~2': '3' }]
```

If you still want object output but with names you supply, use
`field.names`:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, {
  header: false,
  field: { names: ['x', 'y', 'z'] },
})

parse.parse('1,2,3\n4,5,6') // => [{ x: '1', y: '2', z: '3' }, { x: '4', y: '5', z: '6' }]
```

## Use a different field delimiter

Tab-separated, pipe-separated, or anything else:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { field: { separation: '\t' } })

parse.parse('name\tage\nAlice\t30') // => [{ name: 'Alice', age: '30' }]
```

The separator can be more than one character:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { field: { separation: '~~' } })

parse.parse('a~~b\n1~~2') // => [{ a: '1', b: '2' }]
```

## Use a different record delimiter

By default a record ends at `\n`, `\r\n`, or `\r`. Override with
`record.separators`:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { record: { separators: '%' } })

parse.parse('a,b%1,2%3,4') // => [{ a: '1', b: '2' }, { a: '3', b: '4' }]
```

## Trim surrounding whitespace from fields

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { trim: true })

parse.parse('a,b\n  hello  ,  world  ') // => [{ a: 'hello', b: 'world' }]
```

Internal whitespace is preserved — `'  hello world  '` trims to
`'hello world'`.

## Skip comment lines

Enable `comment` to strip lines starting with `#`:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { comment: true })

parse.parse('a,b\n# this row is ignored\n1,2') // => [{ a: '1', b: '2' }]
```

A `#` *inside* a field is left alone unless it follows whitespace — see
[Concepts: Comments and whitespace](concepts.md#comments-and-whitespace).

## Preserve blank lines as empty records

Blank lines are skipped by default. To keep them:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { record: { empty: true } })

parse.parse('a\n1\n\n2') // => [{ a: '1' }, { a: '' }, { a: '2' }]
```

## Substitute a value for empty fields

Use `field.empty` for the placeholder. Any value works, including
`null` or a sentinel:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { field: { empty: null } })

parse.parse('a,b,c\n1,,3') // => [{ a: '1', b: null, c: '3' }]
```

## Reject rows with the wrong number of fields

`field.exact: true` errors when a record's field count doesn't match
the header's:

```typescript
const parse = new Tabnas().use(jsonic).use(Csv, { field: { exact: true } })

parse.parse('a,b\n1,2,3')
// throws; error code 'csv_extra_field'

parse.parse('a,b\n1')
// throws; error code 'csv_missing_field'
```

The thrown error's `code` property is `csv_extra_field` or
`csv_missing_field`. See [Reference: Errors](reference.md#errors).

## Allow JSON values inside fields

In strict mode, `[1,2]` and `{x:1}` are just text. Switch to non-strict
mode and jsonic re-engages — you get the JSON value back as a parsed
JavaScript value:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { strict: false })

parse.parse('a,b,c\ntrue,[1,2],{x:{y:"q"}}') // => [{ a: true, b: [1, 2], c: { x: { y: 'q' } } }]
```

Non-strict mode also enables `trim`, `comment`, `number`, and `value`
by default. See
[Concepts: Strict vs non-strict](concepts.md#strict-vs-non-strict-mode).

## Use a different quote character

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Csv } from '@tabnas/csv'

const parse = new Tabnas().use(jsonic).use(Csv, { string: { quote: "'" } })

parse.parse("a,b\n'hi, there','x'") // => [{ a: 'hi, there', b: 'x' }]
```

## Stream records to a callback

For large inputs, set `stream`. The callback fires for `start`,
`record`, `end`, and `error`. The parse function returns `[]` because
records are no longer collected:

```typescript
const records: any[] = []

const parse = new Tabnas().use(jsonic).use(Csv, {
  stream: (what, payload) => {
    if (what === 'record') records.push(payload)
  },
})

parse.parse('a,b\n1,2\n3,4')
// records: [{ a: '1', b: '2' }, { a: '3', b: '4' }]
```

Errors thrown inside the parser are forwarded to the callback as an
`'error'` event rather than re-thrown — branch on `what` accordingly.

## Reuse the same parser for many inputs

The result of `new Tabnas().use(jsonic).use(Csv, opts)` is a parser
instance that is fully reusable — there is no per-call cost beyond the
parse itself:

```typescript
const parse = new Tabnas().use(jsonic).use(Csv, { number: true })

const a = parse.parse('x,y\n1,2')
const b = parse.parse('p,q\n3,4')
```
