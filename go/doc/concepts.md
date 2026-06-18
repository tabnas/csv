# Concepts (Go)

Background on how the CSV plugin works, and why it is built the way it
is — plus where the Go port differs from the canonical TypeScript
implementation. This is understanding-oriented reading; for steps see
the [tutorial](tutorial.md) and [how-to guide](guide.md), and for exact
signatures see the [reference](reference.md).

## Why a Jsonic plugin

Jsonic is a configurable parser whose grammar can be extended at
runtime. CSV is not jsonic syntax, but it lexes and parses naturally
with the same machinery once a few rules are added. The plugin route
reuses jsonic's tokenizer, error-reporting, comment handling, and
streaming hooks rather than re-implementing them.

It also opens the door to *non-strict* mode, where a CSV cell can
contain a jsonic value (an object, an array, a string with backslash
escapes). That capability is unique to this plugin.

## How it sits on the engine

The plugin does not replace the parser; it reconfigures one. When you
write `j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults)`:

1. The base relaxed-JSON grammar is already present — the `val`, `map`,
   `list`, `pair`, and `elem` rules and the standard lexer matchers.
2. `Csv` then layers CSV behavior on top: it adds the `csv`, `newline`,
   `record`, and `text` rules from the embedded grammar, reconfigures
   `list` / `elem` / `val` for fields, removes the `#LN` (line-end)
   token from the ignore set so row breaks become significant, and
   (in strict mode) removes `#SP` (whitespace) too so in-field spaces
   survive.

Per-instance options (token overrides, the ignore set) are applied
*after* the grammar is installed via `j.Grammar(...)`, because
installing the grammar runs an internal `SetOptions` pass that would
otherwise clobber those overrides.

## The grammar model

A parse runs in two stages. The **lexer** turns the source string into
tokens; the **parser** consumes tokens according to named rules. Each
rule has an *open* and a *close* phase, each phase a list of
*alternates* matching a short token pattern (at most two tokens of
lookahead). The CSV grammar is a small ladder of rules:

- `csv` — the start rule. Skips leading blank lines, then alternates
  between `record` and `newline`.
- `record` — one row. Pushes `list` to collect the fields, and closes
  at a line ending or end of input.
- `list` / `elem` / `val` — the fields of a row. `list` allocates the
  per-record field slice; `elem` consumes one field (handling empties
  around separators); `val` resolves a field's value.
- `text` — accumulates a run of value and whitespace tokens into one
  field string, applying `trim` if enabled.
- `newline` — collapses one or more record separators between records.

The `csv`, `newline`, `record`, and `text` rules live in the shared
`csv-grammar.jsonic` file. The `list`, `elem`, and `val` rules are
configured *in code* (via `j.Rule(...)`) because non-strict mode must
preserve jsonic's default alternatives for those rules — see
[Relationship to the grammar file](#relationship-to-the-grammar-file).

## Strict vs non-strict mode

**Strict mode (default).** The plugin disables jsonic's value parsing.
Every field is the raw text of the cell, and CSV's RFC 4180-style
double-quote escaping is applied. Numbers, booleans, and null literals
stay as strings unless you explicitly enable `number` or `value`. This
is what you want for "normal" CSV.

**Non-strict mode** (`"strict": false`). Field bodies are parsed *as
jsonic*. Scalars (`true`, `false`, `null`, numbers) decode to native Go
types, and structural jsonic values inside a cell work too — `[1,2]`
becomes `[]any{1, 2}`, `{x:1}` becomes `map[string]any{"x": 1}`. Quoted
strings honour jsonic's escape rules (e.g. `"a\"b"`) rather than CSV's
`""`-doubling. To make this convenient, non-strict mode also flips
`trim`, `comment`, `number`, and `value` on by default. The trade-off
is that pure-CSV quirks (unescaped quotes, some malformed cells) may no
longer be tolerated.

In practice: use strict for ingesting CSV from the outside world; use
non-strict when the file is your own and you want richer in-cell types
without inventing a new format.

## Quoted fields (RFC 4180)

In strict mode the plugin installs a custom string lexer that follows
RFC 4180:

- A quoted field starts with `"` *at the beginning of a field* (i.e.
  directly after a delimiter, line break, or start of input) and
  continues until a matching `"`.
- A literal `"` inside the field is written `""`.
- The quoted body may contain commas, line breaks, and any other
  character that would otherwise be significant.

So `"a""b"` lexes to the value `a"b`, and a multi-line quoted field is
one record's worth of one field. The quote character can be changed via
`string.quote`.

## Object output

When `object: true` (the default), each record is a plain
`map[string]any`. Type-assert and read it directly, or pass it to
`json.Marshal`. Note that Go's `json.Marshal` sorts map keys
alphabetically — if you need to preserve column order in JSON output,
use `object: false` and emit your own JSON from the arrays.

When a record has more fields than the header has names, extra columns
are emitted under keys `field~0`, `field~1`, … — the prefix is
configurable via `field.nonameprefix`. Missing fields take
`field.empty`. Set `field.exact: true` to make either case an error.

## Comments and whitespace

`comment: true` enables jsonic's standard `#` line-comment lexer. Two
subtleties:

1. A line that is *only* a comment is dropped before record processing.
   With `record.empty: true`, dropped comment lines do not become empty
   records (only genuine blank lines do).
2. A `#` *inside* a field is treated as text until preceded by
   whitespace. So `1,#x` keeps `#x`, while `1, #x` strips `#x`.

`trim: true` removes leading and trailing whitespace from each field's
*value*, not from the whole line. Internal runs of whitespace are
preserved.

## Streaming model

When `stream` is set:

- The parser still consumes the whole input string in one call (this is
  a string parser, not an `io.Reader`).
- Records are emitted to the callback as they are produced rather than
  collected.
- The top-level call returns `[]any{}`.

This is useful when processing millions of records without holding them
all in memory. To consume an arbitrarily large file, read it as a
string (or as chunks joined into a string), and let `stream` drain the
records into your downstream sink. The callback also receives `"error"`
events instead of `Parse` returning the error — wrap accordingly.

## Relationship to the grammar file

This Go module and the [TypeScript implementation](../../ts/doc/concepts.md)
share `csv-grammar.jsonic` (embedded into both source files at build
time). The grammar file declares the `csv`, `newline`, `record`, and
`text` rules together with static options (`rule.start`,
`lex.emptyResult`, error codes, hint templates).

The `list`, `elem`, and `val` rules are configured in code rather than
in the grammar file because *non-strict* mode must preserve jsonic's
default alternatives for those rules to support embedded JSON values.
Putting them in code keeps the strict and non-strict variants on the
same path.

If you want to study the grammar, read `csv-grammar.jsonic` — it is a
single page of declarative rules.

## Differences from the TypeScript version

The TypeScript implementation is canonical; the Go module is a faithful
port driven by the same shared `csv-grammar.jsonic` and the same
`test/fixtures/` conformance suite. Successful parses produce equivalent
results across both runtimes. The differences are in API shape, value
types, and one known error-code gap.

### API shape

| Aspect | TypeScript | Go |
|---|---|---|
| Registration | `new Tabnas().use(jsonic).use(Csv, opts?)` | `j := tabnasjsonic.Make(); j.UseDefaults(tabnascsv.Csv, tabnascsv.Defaults, opts...)` |
| Plugin signature | `(tn, options) => void` | `func(j *tabnasjsonic.Jsonic, options map[string]any) error` |
| Options | partial `CsvOptions` object | `map[string]any` merged over `tabnascsv.Defaults` |
| Defaults | attached as `Csv.defaults` | exported as `tabnascsv.Defaults`, passed explicitly |
| Parse call | `parse.parse(src)` returns the value | `j.Parse(src)` returns `(any, error)` |
| Errors | thrown as exceptions | returned as `error` (never panics) |
| Stream callback | `(what: string, payload?) => void` | `func(what string, payload any)` |

In Go the plugin guards against re-invocation (`UseDefaults`/`Use`
re-run plugins on option changes) by decorating the instance with a
`csv-init` flag, so the second invocation is a no-op.

### Value types

Go returns `any`, but the concrete types are predictable:

| Value | Go type |
|---|---|
| A record (object output) | `map[string]any` |
| A record (slice output) | `[]any` |
| Top-level result | `[]any` |
| Strings / raw text | `string` |
| Numbers (under `number` / non-strict) | `float64` |
| Booleans (under `value` / non-strict) | `bool` |
| Null (under `value` / non-strict) | `nil` |
| Empty field placeholder | whatever `field.empty` is (default `""`) |

### Known accepted difference: `field.exact` error code

TypeScript raises `csv_extra_field` / `csv_missing_field` (via
`ctx.t0.bad(code)`) when a row's field count violates `field.exact`.
`jsonic/go`'s parser does not yet propagate a bad token's custom error
code — it surfaces every parse error under the generic `unexpected`
code — so the Go error currently reports `unexpected`. The port already
tags the offending token with the specific code, so parity follows
automatically once `jsonic/go` honours it.

Until then, treat a non-nil error from a `field.exact` parser as a
field-count violation. The Go fixture/unit tests assert that the parse
*fails*, not the specific code (`go/csv_test.go` `TestFieldExact`). See
`AGENTS.md` for the full note.
