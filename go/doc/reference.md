# Reference (Go)

The public API, every option with its default and effect, the parse
return value, the streaming protocol, error codes, and the grammar the
plugin accepts. For task recipes see the [how-to guide](guide.md); for
background see [concepts](concepts.md).

## `Csv` (plugin function)

```go
func Csv(j *tabnasjsonic.Jsonic, options map[string]any) error
```

The Jsonic plugin that installs the CSV grammar and options. Register
with `j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, overrides...)`. The function
is idempotent: re-invoking it on the same instance is a no-op.

## `Defaults` (option map)

```go
var Defaults map[string]any
```

The default option set. Pass it to `UseDefaults` so user-supplied
overrides are merged on top of the defaults.

## `Version` (string)

```go
const Version = "..."
```

The module version, kept in sync with the `go/v*` Git tag.

## Option keys

Top-level keys (set on the options map passed to `UseDefaults`):

| Key | Type | Default | Notes |
|---|---|---|---|
| `trim` | `bool` or `nil` | `nil` | Trim leading/trailing whitespace from a field value. `nil` resolves to `false` strict / `true` non-strict. |
| `comment` | `bool` or `nil` | `nil` | Strip `#` line comments. `nil` resolves to `false` strict / `true` non-strict. |
| `number` | `bool` or `nil` | `nil` | Parse numeric literals into `float64`. `nil` resolves to `false` strict / `true` non-strict. |
| `value` | `bool` or `nil` | `nil` | Parse `true` / `false` / `null` literals. `nil` resolves to `false` strict / `true` non-strict. |
| `header` | `bool` | `true` | First row is field names. |
| `object` | `bool` | `true` | Object output (`true`) vs slice output (`false`). |
| `strict` | `bool` | `true` | Disable jsonic value syntax inside fields. |
| `stream` | `func(string, any)` or `nil` | `nil` | Streaming callback (see below). |

Nested `field` group:

| Key | Type | Default | Notes |
|---|---|---|---|
| `field.separation` | `string` or `nil` | `nil` | Delimiter; `nil` keeps `,`. May be more than one character. |
| `field.nonameprefix` | `string` | `"field~"` | Prefix used when a record has more fields than names. |
| `field.empty` | `any` | `""` | Value substituted for an empty field. |
| `field.names` | `[]string` or `nil` | `nil` | Explicit field names. |
| `field.exact` | `bool` | `false` | Error on field-count mismatch. |

Nested `record` group:

| Key | Type | Default | Notes |
|---|---|---|---|
| `record.separators` | `string` or `nil` | `nil` | Custom record-separator chars. `nil` keeps `\n`/`\r\n`/`\r`. |
| `record.empty` | `bool` | `false` | Preserve blank lines as empty records. |

Nested `string` group:

| Key | Type | Default | Notes |
|---|---|---|---|
| `string.quote` | `string` | `"` | Quote character for the CSV string lexer. |
| `string.csv` | `bool` or `nil` | `nil` | Force the CSV string lexer; `nil` is auto (on in strict, off in non-strict). |

## Return value

`j.Parse(src)` returns `(any, error)`. On success the value is a `[]any`
whose elements are:

- `map[string]any` keyed by field name when `object: true` (default)
- `[]any` (a slice of fields) when `object: false`

The result is an empty `[]any{}` for empty input, and an empty `[]any{}`
(with records sent to the callback) when `stream` is set. Numbers
decoded under `number`/non-strict mode arrive as `float64`.

## Streaming callback

```go
func(what string, payload any)
```

`what` is one of:

| Event | Payload |
|---|---|
| `"start"` | `nil` |
| `"record"` | The parsed record (object or slice) |
| `"end"` | `nil` |
| `"error"` | An `error` raised during parsing |

## Errors

`field.exact` violations halt the parse with a non-nil error. The
TypeScript build (canonical) reports these under the dedicated codes
`csv_extra_field` and `csv_missing_field`.

> **Note (Go):** the underlying `jsonic/go` parser does not yet
> propagate a custom bad-token error code, so the error returned from
> `Parse` currently carries the generic `unexpected` code rather than
> `csv_extra_field` / `csv_missing_field`. The plugin already tags the
> offending token with the specific code, so parity follows
> automatically once `jsonic/go` honours it. Treat a non-nil error from
> a `field.exact` parser as a field-count violation. See `AGENTS.md`
> and [concepts](concepts.md#differences-from-the-typescript-version)
> for details.

Other errors come from the parser itself, e.g. `unterminated_string`
for an unclosed quoted field, and `unexpected` for content that matches
no grammar alternate (such as trailing junk after a complete non-strict
value: `j.Parse("a\n{x:1}y")` returns a non-nil error).

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

A railroad/syntax diagram of the live grammar is in
[`ts/doc/grammar.svg`](../../ts/doc/grammar.svg) (ASCII version:
[`ts/doc/grammar.txt`](../../ts/doc/grammar.txt)). The grammar is shared
between the Go and TypeScript implementations.
