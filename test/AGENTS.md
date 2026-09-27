# Agents Guide — shared test data

Everything under `test/` is runtime-neutral: both the TypeScript and the Go
suite read it, so a change here affects both implementations at once.

| Path | What it is |
|---|---|
| `fixtures/` | Our own `.csv` → `.json` fixtures, driven by `fixtures/manifest.json`. Committed. |
| `spec/*.tsv` | Our own cross-runtime parity fixtures, auto-discovered. Committed. |
| `suites/` | Third-party conformance corpora, vendored unchanged at pinned upstream commits, each under its own licence (`suites/README.md`). Committed, never edited. |

## `spec/*.tsv` — format

The format is `@tabnas/support`'s, not this repo's: one loader, in two
languages, shared by every tabnas package. Its
[reference](https://github.com/tabnas/support/blob/main/doc/reference.md)
is the authority; the short version is that a fixture is tab-separated,
one case per line, with a header row naming the columns. Blank lines are
skipped, and so are comment lines — a line starting with `#` that contains
no tab. (A data row always has at least one tab, so a `#`-leading CSV
source still works.)

| Column | Meaning |
|---|---|
| `input` | CSV source. Escapes `\n` `\r` `\t` `\\` are decoded. |
| `expected` | A JSON value (the parse result), or `ERROR` / `ERROR:<code>` for inputs that must fail. |
| `opts` | Optional JSON object of plugin options (empty means defaults). |

The **code** in an `ERROR:` cell is compared exactly — `csv_extra_field`
is the error's code, not a substring of its message. Two runtimes that
reject the same input for different reasons have not agreed on anything.

`expected` and `opts` are **not** escape-decoded — they are raw JSON, so
JSON's own escape rules apply (`"a\nb"` is a string containing a newline).
To put a literal backslash in `input`, write `\\`.

Results are compared after a JSON round-trip, so key order and the
`OrderedMap` / null-prototype-object representations do not affect the
comparison.

## Who runs what

- TypeScript: `ts/test/parity.test.ts` — `makeRunner(...).dir(...)`.
- Go: `go/parity_test.go` — `support.Runner{...}.Dir(t, dir)`.
- Rust: `rs/tests/parity_test.rs` — `tabnas_support::Runner::new_with_row(...).dir(dir)`.

All three are a dozen lines holding only what is specific to csv: how to
build the parser for a row's `opts`, and the JSON flattening. Everything
else — finding `test/spec`, reading the file, decoding escapes, the
`ERROR:` contract, the comparison, the `<file>:<line>` in a failure
message — comes from `@tabnas/support` / `github.com/tabnas/support/go` /
the `tabnas-support` crate, so the loaders cannot drift from each other
either.

All three discover files by directory listing: adding a `.tsv` here runs
it in every runtime without touching a runner. An empty fixture, and a
spec directory with no fixtures in it, both **fail** — a runner that
reports green having run nothing is indistinguishable from coverage that
was never there.

## Rules

- Prefer adding a fixture here over a one-off in-language assertion when a
  case is expressible as input → output. That is what keeps the two
  runtimes honest against each other.
- TypeScript is canonical. If the two runtimes disagree, the TS behaviour is
  the expected value — unless Go has exposed a genuine TS defect, in which
  case fix TS first and pin the corrected behaviour here.
- A new fixture must pass in EVERY runtime: run `go test ./...` (from `go/`),
  `npm test` (from `ts/`) and `cargo test` (from `rs/`) before considering
  it done.

## `suites/` — third-party corpora (vendored)

`spec/` and `fixtures/` are OUR fixtures. The third-party corpora in
`suites/` are other people's work, vendored unchanged at pinned upstream
commits so every runtime judges them offline, and pinned so the reported
numbers stay tied to a named revision. Each directory carries its upstream
licence and a `PINNED` file; `suites/README.md` credits the authors and
records the source, licence and copyright of each. Never edit these files:
a corpus is replaced whole, from a new upstream revision, with its pins.

- `suites/csv-spectrum/` — `max-mapper/csv-spectrum` @ `d30e80f`
  (BSD-2-Clause), 12 valid documents (the corpus has no must-fail half),
  pinned by document count and a SHA-256 content digest. The three
  `*_crlf.csv` documents are CRLF, as upstream checks them out;
  `.gitattributes` marks `suites/` `-text` so git keeps every byte.
- `suites/go-encoding-csv/` — `golang/go` `src/encoding/csv/reader_test.go`
  @ tag `go1.27.1` (BSD-3-Clause), SHA-256 pinned, converted to the
  committed `cases.json` by `scripts/extract-go-csv-cases.mjs`: 43 valid +
  12 must-fail, 13 excluded.

`scripts/verify-csv-suites.sh` checks all of it, with no network: the pins,
the counts, the digests, the `PINNED` files, and that `cases.json` is still
exactly what the extractor derives. `npm test` runs it from the `pretest`
hook.

Run by `ts/test/conformance.test.ts`, `go/conformance_test.go` and
`rs/tests/conformance_test.rs`. Both halves are exercised: valid documents
must produce the **expected value**, invalid documents must be **rejected**.

These tests **must never skip.** A missing corpus FAILS every runtime. A
conformance suite that quietly does not run reports green while measuring
nothing.

The scores are asserted, not merely reported — see the "Conformance" section of
the root [`AGENTS.md`](../AGENTS.md) for the current figures and the divergence
table. Never trim a corpus, loosen an assertion or add a skip to move a number.
