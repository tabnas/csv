# Reference (TypeScript)

The public API, every option with its default and effect, the parse
return value, the streaming protocol, error codes, and the grammar the
plugin accepts. For task recipes see the [how-to guide](guide.md); for
background see [concepts](concepts.md).

## Exports

```typescript
import { Csv, buildCsvStringMatcher } from '@tabnas/csv'
import type { CsvOptions } from '@tabnas/csv'
```

| Export | Kind | Purpose |
|---|---|---|
| `Csv` | `Plugin` | The Tabnas plugin. Pass to `Tabnas.use()`. |
| `buildCsvStringMatcher` | function | Factory for the RFC 4180 quote matcher (advanced). |
| `CsvOptions` | type | The full options shape. |

## `Csv` (plugin)

```typescript
const parse = new Tabnas().use(jsonic).use(Csv, options?)
```

The Tabnas plugin function. Register it on a Tabnas instance that
already has the jsonic grammar loaded (`new Tabnas().use(jsonic)`). Pass
an optional partial options object as the second argument to
`Tabnas.use()`; the plugin merges your options on top of `Csv.defaults`,
so you only specify what differs.

`Csv.defaults` is attached to the plugin and holds the complete default
option set (see the table below).

## `CsvOptions`

All keys are optional. Defaults are in the rightmost column.

| Key | Type | Default | Notes |
|---|---|---|---|
| `trim` | `boolean \| null` | `null` | Trim leading/trailing whitespace from a field value. `null` resolves to `false` in strict mode and `true` in non-strict. |
| `comment` | `boolean \| null` | `null` | Strip `#` line comments. `null` resolves to `false` in strict / `true` in non-strict. |
| `number` | `boolean \| null` | `null` | Parse numeric literals into `number`. `null` resolves to `false` strict / `true` non-strict. |
| `value` | `boolean \| null` | `null` | Parse the literals `true`, `false`, `null` into their JS values. `null` resolves to `false` in strict / `true` in non-strict. |
| `header` | `boolean` | `true` | Treat the first record as field names. |
| `object` | `boolean` | `true` | Emit each record as `Record<string, any>`; if `false`, emit `any[]`. |
| `stream` | `((what, payload?) => void) \| null` | `null` | Streaming callback. See [Streaming callback](#streaming-callback). |
| `strict` | `boolean` | `true` | Disable jsonic value syntax inside fields. |

`field` (nested):

| Key | Type | Default | Notes |
|---|---|---|---|
| `field.separation` | `string \| null` | `null` | Field delimiter. `null` keeps `,`. May be more than one character. |
| `field.nonameprefix` | `string` | `'field~'` | Prefix used when a record has more fields than the header has names: extra columns are emitted as `field~N`. |
| `field.empty` | `any` | `''` | Value substituted for an empty field. |
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

## Return value

Calling `parse.parse(src)` returns:

- `Record<string, any>[]` when `object: true` (default)
- `any[][]` when `object: false`
- `[]` when `stream` is set (records are emitted to the callback instead)
- `[]` for empty input

Leading and trailing blank lines are skipped by default; with
`record.empty: true` they become empty records.

## Streaming callback

```typescript
type StreamFn =
  (what: string, record?: Record<string, any> | Error) => void
```

`what` is one of:

| Event | Payload |
|---|---|
| `'start'` | `undefined` |
| `'record'` | The parsed record (object or array) |
| `'end'` | `undefined` |
| `'error'` | An `Error` thrown during parsing |

Errors thrown inside the parser are forwarded to the callback and not
re-thrown.

## Errors

Field-count violations under `field.exact` raise these error codes
(visible on the thrown Tabnas/Jsonic error's `code`):

| Code | Message |
|---|---|
| `csv_extra_field` | `unexpected extra field value: <src>` |
| `csv_missing_field` | `missing field` |

Other errors come from the engine itself, e.g. `unterminated_string`
for a quoted field with no closing quote, and `unexpected` for content
that cannot be matched by any grammar alternate (such as trailing junk
after a complete non-strict value: `parse.parse('a\n{x:1}y')` throws
with code `unexpected`).

## `buildCsvStringMatcher(opts)`

```typescript
import { buildCsvStringMatcher } from '@tabnas/csv'
```

Factory for the custom CSV double-quote string matcher. Exported for
advanced cases where you want to plug the CSV quote handling into a
non-default Tabnas instance manually. Most users never need this; the
`Csv` plugin installs it for you when appropriate (in strict mode, or
in non-strict mode when `string.csv: true`).

## Grammar and accepted syntax

The plugin parses CSV with the rules in
[`csv-grammar.jsonic`](../../csv-grammar.jsonic) (embedded into the
source at build time) plus the `list` / `elem` / `val` rules configured
in code. The shape is:

```
csv      = ( record  ( newline record )* )?
record   = list                    # one row
list     = elem ( separator elem )* # the row's fields
elem     = val | <empty>           # one field
val      = text | scalar | <empty> # a field's value
text     = run of #VAL and #SP tokens (whitespace significant)
newline  = one or more record separators between records
```

Concretely, the plugin accepts:

- **Records** separated by line endings. By default `\n`, `\r\n`, and
  `\r` all end a record; configurable via `record.separators`.
- **Fields** separated by `,` (configurable via `field.separation`,
  including multi-character separators).
- **Empty fields**, including leading (`,1`), trailing (`1,`), and
  consecutive (`1,,3`) separators. Each empty field becomes
  `field.empty`.
- **Quoted fields** (strict mode): a field beginning with `"` (at a
  field boundary) runs to the matching `"`. Inside, `""` is a literal
  quote, and commas / line breaks / other significant characters are
  text. The quote character is set by `string.quote`.
- **Blank lines**: skipped by default; kept as empty records with
  `record.empty: true`. Leading and trailing blank lines are always
  skipped.
- **Comment lines** (when `comment: true`): a line starting with `#` is
  dropped before record assembly.
- **Embedded jsonic values** (non-strict mode only): a field body may
  be any jsonic value — `[1,2]`, `{x:1}`, a quoted string with
  backslash escapes, a number, or a keyword.

The token legend used by the railroad diagram:

| Token | Meaning |
|---|---|
| `#LN` | newline (ends a record / row) |
| `#SP` | whitespace (significant: field text) |
| `#CA` | comma / field separator |
| `#ZZ` | end of input |
| `#VAL` | a field-value token (text, number, string, keyword) |
| `#OB` `#CB` `#OS` `#CS` `#CL` | embedded JSON `{` `}` `[` `]` `:` (non-strict only) |

A railroad/syntax diagram of the live grammar is in
[`grammar.svg`](grammar.svg) (ASCII version: [`grammar.txt`](grammar.txt)).
