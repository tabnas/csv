# @jsonic/csv — Go

A [Jsonic](https://jsonic.senecajs.org) syntax plugin that parses
CSV text into Go values. Headers, quoted fields, custom
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
go get github.com/jsonicjs/csv/go
```

The module path is `github.com/jsonicjs/csv/go` and is normally
imported with the alias `csv`. It depends on
`github.com/jsonicjs/jsonic/go` for the underlying parser.

```go
import (
    csv "github.com/jsonicjs/csv/go"
    jsonic "github.com/jsonicjs/jsonic/go"
)
```

A configured parser is one Jsonic instance with the CSV plugin
registered:

```go
j := jsonic.Make()
j.UseDefaults(csv.Csv, csv.Defaults)

result, err := j.Parse("a,b\n1,2")
```

`UseDefaults` merges any extra `map[string]any` arguments on top
of `csv.Defaults`, so you only specify what differs from default.

---

## Tutorial — your first parse

This tutorial walks through parsing CSV end-to-end: header rows,
quoted fields, type coercion, and a streaming pipeline. By the
end you will have written the four shapes of program you are
likely to need.

### 1. A two-line CSV

```go
package main

import (
    "fmt"

    csv "github.com/jsonicjs/csv/go"
    jsonic "github.com/jsonicjs/jsonic/go"
)

func main() {
    j := jsonic.Make()
    j.UseDefaults(csv.Csv, csv.Defaults)

    result, err := j.Parse("name,age\nAlice,30\nBob,25")
    if err != nil {
        panic(err)
    }

    for _, r := range result.([]any) {
        row := r.(map[string]any)
        fmt.Printf("%s is %s\n", row["name"], row["age"])
    }
    // Alice is 30
    // Bob is 25
}
```

The first row was treated as a header (the default), and each
subsequent row became a `map[string]any` keyed by those names.
`30` and `25` are *strings* — strict mode is on by default, and
strict mode keeps every field as the raw text it appeared as.

### 2. Turn the strings into numbers

Strict mode treats every cell as raw text. Enable `number` (and
optionally `value` for the literals `true`, `false`, `null`):

```go
j := jsonic.Make()
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "number": true,
    "value":  true,
})

result, _ := j.Parse("name,age,active\nAlice,30,true\nBob,25,false")
// [
//   {name: Alice, age: 30, active: true},
//   {name: Bob,   age: 25, active: false},
// ]
//
// where 30, 25 are float64 and true, false are bool.
```

These options are independent — turn on whichever ones the data
calls for.

### 3. Quoted fields with commas and newlines

CSV's quote rules are the bit you have to deal with eventually.
Wrap a field in `"` to include commas, newlines, or quotes; double
a quote (`""`) to escape it.

```go
j := jsonic.Make()
j.UseDefaults(csv.Csv, csv.Defaults)

src := `name,bio
Alice,"Likes ""cats"" and dogs"
Bob,"line 1
line 2"`

result, _ := j.Parse(src)
// [
//   {name: Alice, bio: Likes "cats" and dogs},
//   {name: Bob,   bio: "line 1\nline 2"},
// ]
```

The plugin ships its own quoted-string lexer to follow RFC 4180
(see *Explanation: Quoted fields*).

### 4. Stream records to a callback

If the input is too big to hold in memory, register a `stream`
callback. The plugin emits one event per record and the top-level
result is `[]any{}`.

```go
j := jsonic.Make()
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "stream": func(what string, payload any) {
        if what == "record" {
            fmt.Println("row:", payload)
        }
    },
})

j.Parse("a,b\n1,2\n3,4")
// row: map[a:1 b:2]
// row: map[a:3 b:4]
```

You now have the four shapes — basic parse, type coercion, quoted
data, streaming. Everything else in this document is variations
on these.

---

## How-to guides

Short recipes for specific problems. Each is independent; pick
the one that matches the question you have right now.

All examples build on a Jsonic instance:

```go
j := jsonic.Make()
j.UseDefaults(csv.Csv, csv.Defaults, /* overrides */)
result, err := j.Parse(input)
```

### Return arrays instead of maps

Set `object: false` to receive each record as a plain `[]any`.
With `header: true` (the default) the header row is consumed for
internal field tracking and not emitted:

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{"object": false})

result, _ := j.Parse("a,b,c\n1,2,3\n4,5,6")
// [[1 2 3] [4 5 6]]
```

To get every row including the first, also set `header: false`.

### Use a different field delimiter

Set `field.separation`. Tab, pipe, or any other string — including
multi-character strings such as `"~~"`:

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "field": map[string]any{"separation": "\t"},
})

result, _ := j.Parse("name\tage\nAlice\t30")
// [{name: Alice, age: 30}]
```

### Use a different record delimiter

By default a record ends at `\n`, `\r\n`, or `\r`. Override with
`record.separators`:

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "record": map[string]any{"separators": "%"},
})

result, _ := j.Parse("a,b%1,2%3,4")
// [{a: 1, b: 2}, {a: 3, b: 4}]
```

### Parse without a header row

If your CSV has no header, set `header: false`. With the default
`object: true` and no field names, the plugin invents keys
(`field~0`, `field~1`, …):

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{"header": false})

result, _ := j.Parse("1,2,3")
// [{field~0: 1, field~1: 2, field~2: 3}]
```

### Provide explicit field names

When the file has no header but you still want named fields, set
`header: false` and pass `field.names`:

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "header": false,
    "field":  map[string]any{"names": []string{"x", "y", "z"}},
})

result, _ := j.Parse("1,2,3")
// [{x: 1, y: 2, z: 3}]
```

`field.names` is ignored when `object: false` — every row comes
out as a plain `[]any` in that case.

### Trim surrounding whitespace from fields

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{"trim": true})

result, _ := j.Parse("a,b\n  hello  ,  world  ")
// [{a: hello, b: world}]
```

Internal whitespace is preserved.

### Skip comment lines

Enable `comment` to strip lines starting with `#`:

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{"comment": true})

result, _ := j.Parse("a,b\n# this row is ignored\n1,2")
// [{a: 1, b: 2}]
```

A `#` *inside* a field is left alone unless preceded by whitespace.

### Preserve blank lines as empty records

Blank lines are skipped by default. To keep them:

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "record": map[string]any{"empty": true},
})

result, _ := j.Parse("a\n1\n\n2")
// [{a: 1}, {a: }, {a: 2}]
```

### Substitute a value for empty fields

Use `field.empty` to set the placeholder for missing cells. Any
value works — string, `nil`, bool, number:

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "field": map[string]any{"empty": nil},
})

result, _ := j.Parse("a,b,c\n1,,3")
// [{a: 1, b: <nil>, c: 3}]
```

### Reject rows with the wrong number of fields

`field.exact: true` errors when a record's field count differs
from the header's:

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "field": map[string]any{"exact": true},
})

_, err := j.Parse("a,b\n1,2,3")
// err is non-nil; the message is "unexpected extra field value: 3"
```

### Allow Jsonic values inside fields

In strict mode, `[1,2]` and `{x:1}` are just text. Switch to
non-strict and Jsonic re-engages — you get the parsed value back
as a Go value:

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{"strict": false})

result, _ := j.Parse("a,b,c\ntrue,[1,2],{x:1}")
// [{a: true, b: [1 2], c: {x: 1}}]
```

Non-strict mode also enables `trim`, `comment`, and `number` by
default. Numbers come back as `float64`.

### Use a different quote character

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "object": false,
    "string": map[string]any{"quote": "'"},
})

result, _ := j.Parse("a,b\n'hi, there','x'")
// [[hi, there x]]
```

### Stream and forward errors

The streaming callback receives `start`, `record`, `end`, and
`error` events. Errors are sent to the callback rather than
returned from `Parse`:

```go
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{
    "object": false,
    "stream": func(what string, payload any) {
        switch what {
        case "record":
            // payload is one row
        case "error":
            // payload is an error value
        case "start", "end":
            // bracket events
        }
    },
})
```

### Reuse the same parser for many inputs

The configured Jsonic instance is reusable — there is no per-call
setup cost beyond the parse itself:

```go
j := jsonic.Make()
j.UseDefaults(csv.Csv, csv.Defaults, map[string]any{"number": true})

a, _ := j.Parse("x,y\n1,2")
b, _ := j.Parse("p,q\n3,4")
```

---

## Reference

### `Csv` (plugin function)

```go
func Csv(j *jsonic.Jsonic, options map[string]any) error
```

The Jsonic plugin that installs CSV grammar and options. Register
with `j.UseDefaults(csv.Csv, csv.Defaults, overrides...)`. The
function is idempotent: re-invoking it on the same instance is a
no-op.

### `Defaults` (option map)

```go
var Defaults map[string]any
```

The default option set. Pass it to `UseDefaults` so user-supplied
overrides are merged on top of the defaults.

### `Version` (string)

```go
const Version = "..."
```

The module version, kept in sync with the `go/v*` Git tag.

### Option keys

Top-level keys (set on the options map passed to `UseDefaults`):

| Key       | Type                    | Default | Notes                                             |
|-----------|-------------------------|---------|---------------------------------------------------|
| `trim`    | `bool` or `nil`         | `nil`   | `nil` resolves to `false` strict / `true` non-strict |
| `comment` | `bool` or `nil`         | `nil`   | `nil` resolves to `false` strict / `true` non-strict |
| `number`  | `bool` or `nil`         | `nil`   | `nil` resolves to `false` strict / `true` non-strict |
| `value`   | `bool` or `nil`         | `nil`   | Parse `true` / `false` / `null` literals          |
| `header`  | `bool`                  | `true`  | First row is field names                          |
| `object`  | `bool`                  | `true`  | Object output (`true`) vs slice output (`false`)  |
| `strict`  | `bool`                  | `true`  | Disable Jsonic syntax inside fields               |
| `stream`  | `func(string, any)` or `nil` | `nil` | Streaming callback (see below)                |

Nested `field` group:

| Key                  | Type        | Default    | Notes                                  |
|----------------------|-------------|------------|----------------------------------------|
| `field.separation`   | `string` or `nil` | `nil`  | Delimiter; `nil` keeps `,`             |
| `field.nonameprefix` | `string`    | `"field~"` | Prefix used when a record has more fields than names |
| `field.empty`        | `any`       | `""`       | Value substituted for empty fields     |
| `field.names`        | `[]string` or `nil` | `nil` | Explicit field names                  |
| `field.exact`        | `bool`      | `false`    | Error on field-count mismatch          |

Nested `record` group:

| Key                 | Type      | Default | Notes                                  |
|---------------------|-----------|---------|----------------------------------------|
| `record.separators` | `string` or `nil` | `nil` | Custom record-separator chars       |
| `record.empty`      | `bool`    | `false` | Preserve empty lines as records        |

Nested `string` group:

| Key            | Type        | Default | Notes                                 |
|----------------|-------------|---------|---------------------------------------|
| `string.quote` | `string`    | `"`     | Quote character for the CSV string lexer |
| `string.csv`   | `bool` or `nil` | `nil` | Force CSV string lexer; `nil` is auto |

### Return value

`j.Parse(src)` returns `(any, error)`. On success the value is a
`[]any` whose elements are:

- `map[string]any` keyed by field name when `object: true` (default)
- `[]any` (a slice of fields) when `object: false`

The result is an empty `[]any{}` for empty input, and an empty
`[]any{}` (with records sent to the callback) when `stream` is
set.

### Streaming callback

```go
func(what string, payload any)
```

`what` is one of:

| Event     | Payload                                  |
|-----------|------------------------------------------|
| `"start"` | `nil`                                    |
| `"record"`| The parsed record (object or slice)      |
| `"end"`   | `nil`                                    |
| `"error"` | An `error` raised during parsing         |

### Errors

Field-count violations under `field.exact` raise these error
codes (the parser returns an `error` whose code matches one of
these):

| Code                | Message                                  |
|---------------------|------------------------------------------|
| `csv_extra_field`   | `unexpected extra field value: <src>`    |
| `csv_missing_field` | `missing field`                          |

Other errors come from Jsonic itself (e.g. unterminated quoted
strings).

---

## Explanation

### Why a Jsonic plugin

Jsonic is a configurable parser whose grammar can be extended at
runtime. CSV is not Jsonic syntax, but it lexes and parses
naturally with the same machinery once a few rules are added.
The plugin route reuses Jsonic's tokenizer, error-reporting,
comment handling, and streaming hooks rather than re-implementing
them.

It also opens the door to *non-strict* mode, where a CSV cell can
contain a Jsonic value (an object, an array, a string with
backslash escapes). That capability is unique to this plugin.

### Strict vs non-strict mode

**Strict mode (default).** The plugin disables Jsonic's value
parsing. Every field is the raw text of the cell, and CSV's RFC
4180-style double-quote escaping is applied. Numbers, booleans,
and null literals stay as strings unless you explicitly enable
`number` or `value`. This is what you want for "normal" CSV.

**Non-strict mode** (`"strict": false`). Field bodies are parsed
*as Jsonic*. Scalars (`true`, `false`, `null`, numbers) decode
to native Go types, and structural Jsonic values inside a cell
work too — `[1,2]` becomes `[]any{1, 2}`, `{x:1}` becomes
`map[string]any{"x": 1}`. Quoted strings honour Jsonic's escape
rules (e.g. `"a\"b"`) rather than CSV's `""`-doubling. To make
this convenient, non-strict mode also flips `trim`, `comment`,
and `number` on by default. The trade-off is that pure-CSV
quirks (unescaped quotes, some malformed cells) may no longer
be tolerated.

In practice: use strict for ingesting CSV from the outside world.
Use non-strict when the file is your own and you want richer
in-cell types without inventing a new format.

### Quoted fields (RFC 4180)

In strict mode the plugin installs a custom string lexer that
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

### Object output

When `object: true` (the default), each record is a plain
`map[string]any`. Type-assert and read it directly, or pass it to
`json.Marshal`. Note that Go's `json.Marshal` sorts map keys
alphabetically — if you need to preserve column order in JSON
output, use `object: false` and emit your own JSON from the
arrays.

When a record has more fields than the header has names, extra
columns are emitted under keys `field~0`, `field~1`, … — the
prefix is configurable via `field.nonameprefix`. Missing fields
take `field.empty`. Set `field.exact: true` to make either case
an error.

### Comments and whitespace

`comment: true` enables Jsonic's standard `#` line-comment lexer.
Two subtleties:

1. A line that is *only* a comment is dropped before record
   processing. With `record.empty: true`, dropped lines do not
   become empty records.
2. A `#` *inside* a field is treated as text until preceded by
   whitespace. So `1,#x` keeps `#x`, while `1, #x` strips `#x`.

`trim: true` removes leading and trailing whitespace from each
field's *value*, not from the whole line. Internal runs of
whitespace are preserved.

### Streaming model

When `stream` is set:

- The parser still consumes the whole input string in one call
  (this is a string parser, not an `io.Reader`).
- Records are emitted to the callback as they are produced
  rather than collected.
- The top-level call returns `[]any{}`.

This is useful when processing millions of records without
holding them all in memory. To consume an arbitrarily large
file, read it as a string (or as chunks joined into a string),
and let `stream` drain the records into your downstream sink.

The callback also receives `"error"` events instead of `Parse`
returning the error — wrap accordingly.

### Relationship to the grammar file

This Go module and the [TypeScript implementation](csv-ts.md)
share `csv-grammar.jsonic` (embedded into both source files at
build time). The grammar file declares the `csv`, `newline`,
`record`, and `text` rules together with static options
(`rule.start`, `lex.emptyResult`, error codes, hint templates).

The `list`, `elem`, and `val` rules are configured in code
rather than in the grammar file because *non-strict* mode must
preserve Jsonic's default alternatives for those rules to support
embedded JSON values. Putting them in code keeps the strict and
non-strict variants on the same path.

If you want to study the grammar, read `csv-grammar.jsonic` —
it is a single page of declarative rules.
