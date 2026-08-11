# Agents Guide — shared test data

Everything under `test/` is runtime-neutral: both the TypeScript and the Go
suite read it, so a change here affects both implementations at once.

| Path | What it is |
|---|---|
| `fixtures/` | Our own `.csv` → `.json` fixtures, driven by `fixtures/manifest.json`. Committed. |
| `spec/*.tsv` | Our own cross-runtime parity fixtures, auto-discovered. Committed. |
| `suites/` | Third-party conformance corpora, fetched at pinned upstream commits. **Never committed** (gitignored). |

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

Both are a dozen lines holding only what is specific to csv: how to build
the parser for a row's `opts`, and the JSON flattening. Everything else —
finding `test/spec`, reading the file, decoding escapes, the `ERROR:`
contract, the comparison, the `<file>:<line>` in a failure message —
comes from `@tabnas/support` / `github.com/tabnas/support/go`, so the two
loaders cannot drift from each other either.

Both discover files by directory listing: adding a `.tsv` here runs it in
both runtimes without touching either runner. An empty fixture, and a
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
- A new fixture must pass in BOTH runtimes: run `go test ./...` (from `go/`)
  and `npm test` (from `ts/`) before considering it done.

## `suites/` — third-party corpora (gitignored)

`spec/` and `fixtures/` are OUR fixtures. The third-party corpora live in
`suites/` and are never committed — they carry their own licences, and
pinning them by upstream commit keeps the reported numbers tied to a named
revision. `scripts/fetch-csv-suites.sh` fetches them:

- `suites/csv-spectrum/` — `max-mapper/csv-spectrum` @ `d30e80f`, 12 valid
  documents (the corpus has no must-fail half). Verified after fetch against
  a pinned document count and a pinned SHA-256 content digest.
- `suites/go-encoding-csv/` — `golang/go` `src/encoding/csv/reader_test.go`
  @ tag `go1.24.0`, SHA-256 pinned, converted to `cases.json` by
  `scripts/extract-go-csv-cases.mjs`: 43 valid + 12 must-fail, 13 excluded.

Run by `ts/test/conformance.test.ts` and `go/conformance_test.go`. Both halves
are exercised: valid documents must produce the **expected value**, invalid
documents must be **rejected**.

These tests **must never skip.** `npm test` fetches via the `pretest` hook and
`go/conformance_test.go` shells out to the same script on the miss path; if the
corpus is still absent afterwards, both runtimes FAIL. A conformance suite that
quietly does not run reports green while measuring nothing.

The scores are asserted, not merely reported — see the "Conformance" section of
the root [`AGENTS.md`](../AGENTS.md) for the current figures and the divergence
table. Never trim a corpus, loosen an assertion or add a skip to move a number.
