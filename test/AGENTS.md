# Agents Guide — shared spec fixtures

`spec/*.tsv` holds the cross-runtime conformance fixtures. Both runtimes
auto-discover and run **every** file in this directory, so a change here
affects TypeScript and Go together — edit with that in mind.

## Format

Tab-separated, one case per line, with a header row naming the columns.
Blank lines are skipped, and so are comment lines — a line starting with
`#` that contains no tab. (A data row always has at least one tab, so a
`#`-leading source such as a C preprocessor directive still works.)

| Column | Meaning |
|---|---|
| `input` | CSV source. Escapes `\n` `\r` `\t` `\\` are decoded. |
| `expected` | A JSON value (the parse result), or `ERROR` / `ERROR:<substring>` for inputs that must fail. |
| `opts` | Optional JSON object of plugin options (empty means defaults). |

`expected` and `opts` are **not** escape-decoded — they are raw JSON, so
JSON's own escape rules apply (`"a\nb"` is a string containing a newline).
To put a literal backslash in `input`, write `\\`.

Results are compared after a JSON round-trip, so key order and the
`OrderedMap` / null-prototype-object representations do not affect the
comparison.

## Who runs what

- TypeScript: `ts/test/parity.test.ts` — reads `../../test/spec` at runtime
  from `dist-test/`, one `describe` per file.
- Go: `go/parity_test.go` — `TestSpec` globs `../test/spec/*.tsv`.

Both discover files by directory listing: adding a `.tsv` here runs it in
both runtimes without touching either runner.

## Rules

- Prefer adding a fixture here over a one-off in-language assertion when a
  case is expressible as input → output. That is what keeps the two
  runtimes honest against each other.
- TypeScript is canonical. If the two runtimes disagree, the TS behaviour is
  the expected value — unless Go has exposed a genuine TS defect, in which
  case fix TS first and pin the corrected behaviour here.
- A new fixture must pass in BOTH runtimes: run `go test ./...` (from `go/`)
  and `npm test` (from `ts/`) before considering it done.

## Third-party conformance corpora (`test/suites/`, gitignored)

`test/spec/*.tsv` and `test/fixtures/` are OUR fixtures. The third-party
corpora live in `test/suites/` and are **never committed** — they are fetched
at pinned upstream commit SHAs by `scripts/fetch-csv-suites.sh`:

- `test/suites/csv-spectrum/` — `max-mapper/csv-spectrum` @ `d30e80f`, 12
  valid documents (the corpus has no must-fail half).
- `test/suites/go-encoding-csv/` — `golang/go` `src/encoding/csv/reader_test.go`
  @ tag `go1.24.0`, converted to `cases.json` by
  `scripts/extract-go-csv-cases.mjs`: 43 valid + 12 must-fail.

Run by `ts/test/conformance.test.ts` and `go/conformance_test.go`. Both halves
are exercised: valid documents must produce the **expected value**, invalid
documents must be **rejected**.

These tests **must never skip**. `npm test` fetches via the `pretest` hook and
`go/conformance_test.go`'s `ensureCorpus` runs the fetch script itself; if the
corpus is still absent both fail loudly. A conformance test that quietly does
not run is worse than no test at all.

The suites are currently **RED on purpose** — see the "Conformance baseline"
section of the root `AGENTS.md`. Do not trim a corpus, loosen an assertion or
add a skip to change the number.
