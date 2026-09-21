# Agents Guide: rs/

The Rust port of the canonical TypeScript in [`../ts`](../ts). Read
[`../AGENTS.md`](../AGENTS.md) first: it holds the cross-runtime rules,
the conformance claim and the divergence table, and this file only covers
what is specific to this crate.

## Layout

| Path | |
|---|---|
| `src/lib.rs` | the whole port: the embedded grammar, `CsvOptions`, every closure the grammar names, the RFC 4180 string matcher, the `list` / `elem` / `val` rule modifications, `csv`, `plugin`, `plugin_with`, `make`, `make_with`, `parse` |
| `tests/parity_test.rs` | every `../test/spec/*.tsv` fixture through `tabnas_support::Runner::new_with_row`, a fresh parser per row from its `opts` column |
| `tests/csv_test.rs` | the in-language port of `go/csv_test.go` and `ts/test/csv.test.ts`: the `../test/fixtures` manifest corpus (with its engine-level `jsonicOpt`), the API surface, streaming, typed options, `field.exact` messages, prototype-pollution keys, `derive`, threads |
| `tests/conformance_test.rs` | the two third-party corpora, with the divergence table; runs `../scripts/fetch-csv-suites.sh` itself and FAILS when a corpus is absent |
| `tests/csv_export_test.rs` | the exported string matcher on a plain jsonic instance (`go/csv_export_test.go`) |
| `tests/perf_test.rs` | instance reuse beats rebuild-per-parse by 4x (`go/perf_test.go`, `ts/test/perf.test.ts`) |
| `tests/embed_test.rs` | the embedded grammar equals `../csv-grammar.jsonic` |
| `tests/version_test.rs` | Cargo.toml == `VERSION` == ts/package.json |
| `tests/common/mod.rs` | shared helpers: the per-row parser, the `jsonicOpt` applier, JSON flattening, failure conversion |
| `README.md` | the crate front page, prose-gated; its `rust` fences are doctests of this crate (see below) |

Crate `tabnas-csv`, library `tabnas_csv`. The engine (`tabnas`), the
jsonic base (`tabnas-jsonic`, which brings `tabnas-json`) and the fixture
runner (`tabnas-support`, dev only) are **path dependencies on sibling
checkouts** (`../../parser/rs`, `../../jsonic/rs`, `../../json/rs`,
`../../support/rs`). None is published, so there is no registry version
to fall back on.

```bash
cargo build --all-targets
cargo test --all-targets && cargo test --doc
cargo clippy --all-targets --all-features -- -D warnings
cargo fmt
```

`make test-rs` from the repository root is the fast loop; `ci/rust/run.sh`
is the full gate and adds `fmt --check`, the lockfile check and the MSRV
pin.

## How the plugin is built

`csv(parser, &CsvOptions)` mirrors the TypeScript `Csv` function top to
bottom, in this order, and the order is load-bearing:

1. **The closures are registered** under the names the grammar uses
   (`@csv-bo`, `@record-bc`, `@text-follows`, `@text-space-push`, ...)
   and the string matcher factory under `@csv-string`. The engine looks
   them up when a document is installed, so they go first.
2. **The per-instance options document** is applied: `rule.exclude`,
   the fixed-token overrides (the JSON structural tokens off in strict
   mode, `#CA` rebound to `field.separation`), the IGNORE token set
   (`#LN` dropped; `#SP` too in strict mode), the `number` / `value` /
   `comment` lexer switches, `line.single` / `line.chars`, and the
   `lex.match.stringcsv` entry. This engine REPLACES a token set
   outright, as Go's does, so the survivors are listed.
3. **The embedded grammar** is parsed by `tabnas_jsonic::parse` (the
   shared default instance, so a rebuild costs no second jsonic), every
   whole number is turned into an integer (jsonic yields `f64`; the
   engine's loader reads `b` and `n` with `as_u64`), and the document is
   installed with every alternate tagged `csv`.
4. **`list`, `elem` and `val` are modified in code** through
   `define_rule`, as Go does with `j.Rule`, not through a serialized
   document. Strict mode `clear()`s each rule and rebuilds it; that must
   drop jsonic's lifecycle references (`@list-bo/append`,
   `@val-bc/replace`, ...) as the TypeScript `rs.clear()` does, and a
   serialized re-declaration would auto-wire them straight back in.
   Non-strict mode prepends the CSV alternates (and appends the
   `elem` push) so jsonic's alternates keep reading embedded values.

The plugin is guarded by the presence of the `csv` rule, not by a
decoration: `derive` copies decorations onto a child that starts with NO
rules and re-runs the plugins, so a decoration guard would leave the
child without the grammar.

## The shared node cell

A pushed or replaced rule SHARES its parent's `Rc<RefCell<Value>>`, so
`*rule.node.borrow_mut() = v` overwrites the parent's node too. An
assignment, `r.node = v` in TypeScript, installs a fresh cell here
(`set_node`). That is what `@csv-bo` (the document array), `list`
before-open (the field array) and `val` before-open (the reset to no
value) do. The cell is borrowed directly only to mutate a container the
rule genuinely shares: `elem` pushing a field onto the record, `record`
before-close pushing a record onto the document.

Two deliberate direct writes stand in for the canonical grammar's
assignments through another rule:

- `@text-bc` writes `rule.parent_node` (the enclosing `val`'s cell):
  `r.parent.node = ...`.
- The chained text actions write `prev_rule.node` on the second and
  later text rules: `r.node = v.node = ...` where `v` is `r.prev`.

## The string matcher

`csv_string_matcher` is a `lex_match_factory_ref`: the factory sees the
resolved options (`line.chars` decides which control characters are
field text) and returns an imperative matcher. Order `1e5` puts it ahead
of every engine band, exactly where TypeScript and Go register it. It
matches only at the quote, and the engine only offers it a slot where
`#ST` is wanted. A control character below 32 that is not a line
character is `unprintable`; an open quote at end of source is
`unterminated_string`, detected by loop exhaustion so an odd run of
quotes cannot pass as terminated.

The jsonic string matcher stays ON in strict mode, as in TypeScript. Go
switches it off there (`String.Lex=false`); do not copy that, or `'x'`
in a strict document stops being the string `x`.

## The conformance corpora

`tests/conformance_test.rs` runs `../scripts/fetch-csv-suites.sh` once
per test binary, on every run (the script is idempotent and verifies the
pinned digests whether or not it fetched), and then judges
`../test/suites`. A missing corpus is a FAILURE, never a skip.

The script fetches csv-spectrum from `codeload.github.com`, which some
sandboxes refuse while allowing `github.com` clones. The script does not
need to do the fetch itself: lay out `test/suites/csv-spectrum/{csvs,json}`
from a checkout of `max-mapper/csv-spectrum` at the pinned commit and
re-run it; it verifies the document count and the content digest and
carries on to the Go source, which comes from `raw.githubusercontent.com`.
The corpus directory is gitignored.

## What a fixture cannot hold

- A `Stream` callback: `tests/csv_test.rs` pins the event order and that
  streamed records are not stored.
- The `field.exact` messages: the `{row}` / `{len}` / `{fsrc}` details
  are supplied with the bad token and the engine's injector fills them,
  so the TypeScript message test is ported (Go cannot port it; see the
  root guide's "Known limitations").
- `field.empty` as a non-string: any JSON value, through the bag or the
  typed struct.
- The one divergence in [`../DIVERGENCE.md`](../DIVERGENCE.md): an object
  header cell (`a,{x:1}` in non-strict mode). The canonical throws a raw
  JavaScript `TypeError`, which is neither a value a row can compare nor
  an `ERROR:<code>` a row can name, so `tests/csv_test.rs` pins the
  refusal instead. An ARRAY header cell is not a divergence: `key_text`
  joins it as `Array.prototype.toString` does, and
  `../test/spec/unstrict.tsv` runs those rows in all three runtimes.

## The docs are gated

`README.md` is in the published set: no em dashes in prose, no first
person singular, no links to any `AGENTS.md`, no project history. This
file is internal and may be blunt.

## The README is doctested

`src/lib.rs` includes `README.md` as rustdoc under `#[cfg(doctest)]`, so
every `rust` fence in it runs on `cargo test --doc` (they show up as
`readme_examples (line N)`). rustdoc runs each fence as written, so a
fence must be a complete program: wrap it in
`fn main() -> Result<(), Box<dyn std::error::Error>> { ... Ok(()) }`
rather than using `?` at the top level, and never use hidden `# ` lines,
which render as garbage on GitHub. The `toml` and `bash` fences are not
run.
