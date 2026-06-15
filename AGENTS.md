# Agent guide — `@tabnas/csv`

Guidance for AI agents (and humans) working in this repository.
Keep this file accurate when you change build steps, layout, or
the canonical/port contract below.

## What this is

A CSV syntax plugin for the [Jsonic](https://jsonic.senecajs.org)
parser. It turns CSV text into objects or arrays, with headers,
RFC 4180 quoting, custom delimiters, streaming, and a strict /
non-strict mode. There are two implementations that must behave
identically.

## Layout

| Path | Description |
|---|---|
| `ts/` | TypeScript / JavaScript implementation. **Canonical.** |
| `go/` | Go port. Must match the TS behaviour. |
| `test/fixtures/` | Shared conformance fixtures, run by both runtimes. |
| `test/fixtures/manifest.json` | Drives the fixture suite (per-case options, expected errors). |

Key source files:

- `ts/src/csv.ts` — canonical plugin source.
- `ts/csv-grammar.jsonic` — the grammar, source of truth for both runtimes.
- `ts/embed-grammar.js` — embeds the grammar into both source files.
- `go/csv.go` — the Go port.
- `ts/test/csv.test.ts`, `go/csv_test.go` — per-runtime unit tests.

## The canonical contract

**TypeScript is canonical. Go is a port of it.** When you change
behaviour:

1. Change `ts/src/csv.ts` first.
2. Port the same change to `go/csv.go`.
3. Add/extend the shared fixture(s) in `test/fixtures/` +
   `manifest.json` so both runtimes assert the new behaviour.
4. Mirror any new unit cases across `ts/test/csv.test.ts` and
   `go/csv_test.go` — the two unit suites should cover the same
   ground.
5. Run both test suites (below) and confirm green.

Do not let the Go behaviour drift from TS. If the Go runtime
genuinely cannot match TS because of a `jsonic/go` limitation,
document the gap here and in the relevant `doc/*.md` Errors
section rather than silently diverging (see "Known limitations").

## The grammar is embedded — never hand-edit the embedded block

`ts/csv-grammar.jsonic` is embedded verbatim into **both**
`ts/src/csv.ts` and `go/csv.go`, between these markers:

```
// --- BEGIN EMBEDDED csv-grammar.jsonic ---
...
// --- END EMBEDDED csv-grammar.jsonic ---
```

Edit `ts/csv-grammar.jsonic`, then run the embed step. Never edit
the text between the markers by hand — it will be overwritten.

```bash
cd ts && node embed-grammar.js   # writes into ts/src/csv.ts AND go/csv.go
```

`npm run build` runs the embed step first, so a normal TS build
keeps both files in sync.

The `csv`, `newline`, `record`, and `text` rules live in the
grammar file. The `list`, `elem`, and `val` rules are configured
in code instead, because non-strict mode must preserve Jsonic's
default alternatives for those rules to keep embedded JSON values
(`[1,2]`, `{x:1}`) working.

## Build & test

The plugin builds against **jsonic `main`**.

TypeScript:

```bash
cd ts
npm install            # dev deps; jsonic is a peerDependency (>= 2)
npm run build          # embeds grammar, then tsc
npm test               # node --test over dist-test
```

To build against jsonic `main` explicitly (rather than a published
release), install it from the GitHub `main` tarball, e.g.:

```bash
curl -sSL -o /tmp/jsonic-main.tar.gz \
  https://github.com/tabnas/jsonic/archive/refs/heads/main.tar.gz
# unpack, `npm pack`, then `npm install --no-save <the-tgz>` in ts/
```

Go:

```bash
cd go
go test ./...          # requires module network access for jsonic/go
```

`go/go.mod` requires `github.com/tabnas/jsonic/go` at the version
that corresponds to jsonic `main` (currently `v0.1.22`; `main`
HEAD is tagged at that version). Keep `go.sum` tidy with
`go mod tidy`.

## Dependencies

- **jsonic** (the parser this plugins into). TS: peer dependency
  `jsonic >= 2`. Go: `github.com/tabnas/jsonic/go`. Track
  `main`.
- The plugin reuses jsonic's lexer, parser, comment handling, and
  streaming hooks rather than re-implementing them.

## Debugging with the jsonic Debug plugin

Use jsonic's built-in debug tooling — do not scatter `print`
statements.

Go:

```go
import jsonic "github.com/tabnas/jsonic/go"

j := jsonic.Make()
j.Use(jsonic.Debug, map[string]any{"trace": true}) // [lex]/[rule] trace
j.UseDefaults(Csv, Defaults)
_, _ = j.Parse("a,b\n1,2")
fmt.Println(jsonic.Describe(j))                      // grammar dump
```

TypeScript:

```typescript
import { Jsonic } from 'jsonic'
import { Debug } from 'jsonic/debug'

const j = Jsonic.make().use(Debug, { trace: true }).use(Csv)
j('a,b\n1,2')
```

The trace prints each lexed token and each rule transition, which
is the fastest way to see why a grammar alternate did or did not
match.

## Architecture notes

- **Strict mode (default).** Jsonic value parsing is disabled;
  every field is raw text, and the plugin's custom RFC 4180 string
  lexer handles `""`-escaped quotes. JSON structural tokens
  (`#OB`, `#CB`, `#OS`, `#CS`, `#CL`) are switched off.
- **Non-strict mode** (`strict: false`). Field bodies parse as
  Jsonic, so a cell can hold `[1,2]`, `{x:1}`, or a backslash-
  escaped string. Non-strict also defaults `trim`, `comment`,
  `number`, and `value` to on.
- `#LN` (line end) is removed from the IGNORE set so row breaks
  are significant; in strict mode `#SP` is also removed so
  in-field whitespace survives.
- Per-instance jsonic options are applied **after** `Grammar()` so
  the `TokenSet`/`Fixed` overrides survive Grammar's internal
  re-`SetOptions`.

## Known limitations

- **`field.exact` error code (Go).** TS raises `csv_extra_field` /
  `csv_missing_field` (via `ctx.t0.bad(code)`). `jsonic/go`'s
  parser does not yet propagate a bad token's custom error code —
  it reports every `ctx.ParseErr` under the generic `unexpected`
  code — so the Go error currently surfaces as `unexpected`. The
  port already tags the token with the specific code (`ctx.T0.Bad`)
  so parity is automatic once `jsonic/go` honours it. Until then,
  treat a non-nil error from a `field.exact` parser as a
  field-count violation. The Go fixture/unit tests assert that the
  parse fails, not the specific code.

## Conventions

- Match the surrounding style in each language; keep the TS and Go
  structures recognisably parallel.
- Preserve fixture line endings — `test/fixtures/*.csv` and
  `*.json` are marked `binary` in `.gitattributes` because some
  cases depend on exact `\r\n` vs `\n`.
- Keep the two `doc/*.md` files (`ts/doc/csv-ts.md`,
  `go/doc/csv-go.md`) in step: tutorial → how-to → reference →
  explanation, with the same capabilities described for each
  runtime.
