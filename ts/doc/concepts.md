# Concepts (TypeScript)

Background on how the CSV plugin works, and why it is built the way it
is. This is understanding-oriented reading — for steps see the
[tutorial](tutorial.md) and [how-to guide](guide.md), and for exact
signatures see the [reference](reference.md).

## Why a Tabnas/Jsonic plugin

Tabnas is a configurable parsing engine whose grammar can be extended
at runtime; jsonic is the relaxed-JSON grammar that runs on it. CSV is
not jsonic syntax, but it lexes and parses naturally with the same
machinery once a few rules are added. The plugin route means CSV
parsing reuses the engine's tokenizer, error-reporting, comment
handling, and streaming hooks rather than re-implementing them.

It also opens the door to *non-strict* mode, where a CSV cell can
contain a jsonic value (an object, an array, a string with backslash
escapes). That capability is unique to this plugin.

## How it sits on the engine

The plugin does not replace the parser; it reconfigures one. When you
write `new Tabnas().use(jsonic).use(Csv)`:

1. `jsonic` installs the base relaxed-JSON grammar — the `val`, `map`,
   `list`, `pair`, and `elem` rules — and the standard lexer matchers.
2. `Csv` then layers CSV behavior on top: it adds the `csv`, `newline`,
   `record`, and `text` rules from the embedded grammar, reconfigures
   `list` / `elem` / `val` for fields, removes the `#LN` (line-end)
   token from the ignore set so row breaks become significant, and
   (in strict mode) removes `#SP` (whitespace) too so in-field spaces
   survive.

Per-instance options (token overrides, the ignore set) are applied
*after* the grammar is installed, because installing the grammar runs an
internal options pass that would otherwise clobber those overrides.

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
  per-record field array; `elem` consumes one field (handling empties
  around separators); `val` resolves a field's value.
- `text` — accumulates a run of value and whitespace tokens into one
  field string, applying `trim` if enabled.
- `newline` — collapses one or more record separators between records.

The `csv`, `newline`, `record`, and `text` rules live in the shared
`csv-grammar.jsonic` file. The `list`, `elem`, and `val` rules are
configured *in code* (via `tn.rule(...)`) because non-strict mode must
preserve jsonic's default alternatives for those rules — see
[Relationship to the grammar file](#relationship-to-the-grammar-file).

## Strict vs non-strict mode

**Strict mode (default).** The plugin disables jsonic's value parsing.
Every field is the raw text of the cell, and CSV's RFC 4180-style
double-quote escaping is applied. Numbers, booleans, and null literals
stay as strings unless you explicitly enable `number` or `value`. This
is what you want for "normal" CSV ingested from the outside world.

**Non-strict mode** (`strict: false`). Field bodies are parsed *as
jsonic*. So `[1,2]` becomes the array `[1, 2]`, `{x:1}` becomes
`{ x: 1 }`, and quoted strings honour jsonic's escape rules rather than
CSV's. To make this convenient, non-strict mode also flips `trim`,
`comment`, `number`, and `value` on by default. The trade-off is that
pure-CSV quirks (unescaped quotes, certain malformed cells) may no
longer be tolerated.

In practice: use strict for ingesting CSV from the outside world; use
non-strict when the file is your own and you want richer in-cell types
without inventing a new format.

## Quoted fields (RFC 4180)

In strict mode the plugin installs a custom string matcher that follows
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

This is a different escaping convention from jsonic's backslash escapes,
which is exactly why strict mode swaps in the dedicated matcher. In
non-strict mode, jsonic's own string matcher handles quotes (with
backslash escapes), unless you force the CSV matcher with
`string.csv: true`.

## Comments and whitespace

`comment: true` enables jsonic's standard `#` line-comment lexer. Two
subtleties:

1. A line that is *only* a comment is dropped before record processing.
   With `record.empty: true`, dropped comment lines do not become empty
   records (only genuine blank lines do).
2. A `#` *inside* a field is treated as text until preceded by
   whitespace. So `1,#x` keeps `#x` (the lexer sees `#` as the start of
   a value token), while `1, #x` strips `#x`.

`trim: true` removes leading and trailing whitespace from each field's
*value*, not from the whole line. Internal runs of whitespace are
preserved.

## Object key ordering and the `field~N` prefix

When `object: true`:

- Header names are used as keys, in their declared order.
- If a record has *more* fields than the header has names, the excess
  columns become keys `field~N` (where `N` is the column index). The
  prefix is configurable via `field.nonameprefix`.
- If a record has *fewer* fields, missing keys take `field.empty`.

Set `field.exact: true` to make field-count mismatches errors instead
(`csv_extra_field` / `csv_missing_field`).

## Streaming model

When `stream` is set:

- The parser still consumes the whole input string in one call (this is
  a string parser, not a chunk parser).
- Records are emitted to the callback as they are completed, rather
  than collected into an array.
- The top-level call returns `[]`.

This is useful when you want to process millions of records without
holding them all in memory at once. To consume an arbitrarily large
file, read it as a string (or as chunks joined into a string), and let
`stream` drain the records into your downstream sink. The callback also
receives `'error'` events instead of the parser throwing — wrap
accordingly.

## Accepted vs rejected edge cases

A few cases are worth calling out because they trip up CSV users:

- **Leading / trailing blank lines** are skipped by default, even with
  CRLF endings: `'\r\n\r\na,b\r\nA,B\r\n\r\n'` parses to a single
  record `[{ a: 'A', b: 'B' }]`.
- **Empty fields** are real fields: `'a\n1,'` yields
  `[{ a: '1', 'field~1': '' }]` — the trailing comma created a second,
  empty field.
- **Unbalanced quotes** are rejected: a quoted field with no closing
  quote raises `unterminated_string`.
- **Trailing junk after a non-strict value** is rejected: in non-strict
  mode `parse.parse('a\n{x:1}y')` throws `unexpected` — the `{x:1}`
  parses, but the trailing `y` matches no alternate.

## Relationship to the grammar file

This plugin and the [Go implementation](../../go/doc/concepts.md) share
`csv-grammar.jsonic` (embedded into both source files at build time).
The grammar file declares the `csv`, `newline`, `record`, and `text`
rules together with static options (`rule.start`, `lex.emptyResult`,
error codes, hint templates).

The `list`, `elem`, and `val` rules are configured in code rather than
in the grammar file because *non-strict* mode must preserve jsonic's
default alternatives for those rules in order to support embedded JSON.
Putting them in code keeps the strict and non-strict variants on the
same path.

If you want to study the grammar, read `csv-grammar.jsonic` — it is a
single page of declarative rules.
