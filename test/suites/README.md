# Third-party conformance corpora

The conformance suites in all three runtimes judge `@tabnas/csv` against
two corpora written by other people. Both are vendored here, unchanged,
at the upstream revisions below, and remain under their authors'
licences. Thank you to everyone who wrote and maintains them.

`scripts/verify-csv-suites.sh` checks every file against its pin. `npm
test` runs it first. It uses no network: to move a pin, replace the
files from the new upstream revision and update the pin, the document
count and the digest in that script together. Never relax a check to
get green.

## csv-spectrum

| | |
|---|---|
| Upstream | <https://github.com/max-mapper/csv-spectrum> |
| Revision | `d30e80f8b99d2eecb3778f1d7b9ed1cb425502ec` (v2.0.0) |
| Licence | BSD-2-Clause, declared in its `package.json` ([`LICENSE`](csv-spectrum/LICENSE)) |
| Copyright | Max Ogden and csv-spectrum contributors |
| Vendored | `csvs/` and `json/` (12 documents, each with its expected value), `readme.md`, `package.json` |

A set of CSV files meant as an acid test for CSV parsers, each paired
with the JSON a parser should produce. Its readme notes that some of the
files come from the examples in [csvkit](https://github.com/wireservice/csvkit)
(MIT, Christopher Groskopf and contributors), so thanks go to the csvkit
authors too.

The three `*_crlf.csv` files keep CRLF line endings, as upstream's
`.gitattributes` checks them out. `.gitattributes` here marks this
directory `-text`, so git stores and checks out every byte as it is.

## go-encoding-csv

| | |
|---|---|
| Upstream | <https://github.com/golang/go>, `src/encoding/csv/reader_test.go` |
| Revision | `862c888e612ac346c7c4d99c9392bdfd265f33b0` (go1.27.1) |
| Licence | BSD-3-Clause ([`LICENSE`](go-encoding-csv/LICENSE), Go's own, from the same revision) |
| Copyright | The Go Authors |
| Vendored | `reader_test.go` |

The reader tests from Go's standard library: a strict RFC 4180 reader,
so its cases include inputs that must be rejected as well as parsed.
`cases.json` is derived from `reader_test.go` by
`scripts/extract-go-csv-cases.mjs`, and is committed so the Go and Rust
suites need no Node. The verify script regenerates it and fails if the
committed copy differs.

Each directory's `PINNED` file records its revision in machine-readable
form. The verify script checks it.
