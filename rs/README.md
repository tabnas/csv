# tabnas-csv (Rust)

The CSV grammar plugin for the
[`tabnas`](https://github.com/tabnas/parser) parsing engine, crate
`tabnas_csv`.

CSV text becomes an array of objects, one per row and keyed by the header
row, or an array of arrays. Quoting follows RFC 4180 (`""` escapes an
embedded quote, a quoted field may hold the separator or a line break),
the field and record separators are configurable, records can be streamed
as they are parsed, and a non-strict mode lets a field body hold embedded
jsonic (`[1,2]`, `{x:1}`). The plugin is not standalone: it layers on the
relaxed-JSON grammar of
[`tabnas-jsonic`](https://github.com/tabnas/jsonic), and reuses the
engine's lexer, comment handling, and lifecycle hooks.

This is the Rust port of the canonical TypeScript implementation in
[`../ts`](../ts); the TypeScript version is authoritative and this crate
tracks it. The Go port is in [`../go`](../go). All three embed the one
grammar, [`../csv-grammar.jsonic`](../csv-grammar.jsonic).

## Use

```rust
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let value = tabnas_csv::parse("name,age\nAlice,30\nBob,25")?;
    assert_eq!(
        value.to_json().to_string(),
        r#"[{"name":"Alice","age":"30"},{"name":"Bob","age":"25"}]"#
    );
    Ok(())
}
```

`parse` builds one parser on first use and reuses it. Building the parser
(installing the jsonic base, then the CSV grammar over it) costs far more
than a small parse, so for anything but a one-off call build an instance
once and keep it:

```rust
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let parser = tabnas_csv::make();
    let value = parser.parse("a,b\n\"x,y\",\"say \"\"hi\"\"\"")?;
    assert_eq!(
        value.to_json().to_string(),
        r#"[{"a":"x,y","b":"say \"hi\""}]"#
    );
    Ok(())
}
```

Options are a struct, with the defaults the TypeScript plugin documents.
`object: false` gives arrays, `header: false` keeps the first row as data,
`field.separation` and `record.separators` change the separators, and
`strict: false` reads a field body as jsonic:

```rust
use tabnas_csv::{CsvOptions, FieldOptions};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let parser = tabnas_csv::make_with(CsvOptions {
        header: false,
        object: false,
        field: FieldOptions {
            separation: Some("|".to_string()),
            ..Default::default()
        },
        ..Default::default()
    });
    assert_eq!(
        parser.parse("a|b\nc|d")?.to_json().to_string(),
        r#"[["a","b"],["c","d"]]"#
    );

    let relaxed = tabnas_csv::make_with(CsvOptions {
        strict: false,
        ..Default::default()
    });
    assert_eq!(
        relaxed.parse("a,b,c\ntrue,[1,2],{x:1}")?.to_string(),
        r#"[{"a":true,"b":[1,2],"c":{"x":1}}]"#
    );
    Ok(())
}
```

The same options travel as a JSON bag through the engine's plugin
mechanism, which is how the shared fixtures configure a parser per row
and how a derived instance rebuilds the grammar:

```rust
use tabnas::Value;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut parser = tabnas_jsonic::make();
    let options = Value::from_json(&serde_json::json!({
        "field": { "exact": true },
    }));
    parser.use_plugin(tabnas_csv::plugin(), Some(options))?;
    assert_eq!(parser.parse("a,b\n1,2,3").unwrap_err().code, "csv_extra_field");
    Ok(())
}
```

To stream records instead of collecting them, give the options a
`Stream`; the result is then an empty array:

```rust
use std::sync::{Arc, Mutex};
use tabnas_csv::{CsvOptions, Stream, StreamEvent};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let seen = Arc::new(Mutex::new(Vec::new()));
    let sink = Arc::clone(&seen);
    let parser = tabnas_csv::make_with(CsvOptions {
        stream: Some(Stream::new(move |event| {
            if let StreamEvent::Record(record) = event {
                sink.lock().unwrap().push(record.to_json().to_string());
            }
        })),
        ..Default::default()
    });
    parser.parse("a,b\n1,2\n3,4")?;
    assert_eq!(
        seen.lock().unwrap().as_slice(),
        [r#"{"a":"1","b":"2"}"#, r#"{"a":"3","b":"4"}"#]
    );
    Ok(())
}
```

Parse errors are the engine's `TabnasError`, re-exported as `CsvError`,
with `code`, `row`, `col` and a report that shows the offending source.
The codes this plugin adds are `csv_extra_field` and `csv_missing_field`,
raised under `field.exact`; `unexpected` and `unterminated_string` come
from the engine.

## Install

Neither the engine nor the jsonic base is published to a registry, so
both are consumed as **sibling checkouts**, the standard tabnas
development model. Clone `https://github.com/tabnas/parser`,
`https://github.com/tabnas/json` and `https://github.com/tabnas/jsonic`
next to this repository and point at them:

```toml
[dependencies]
tabnas-csv = { path = "../csv/rs" }
tabnas-jsonic = { path = "../jsonic/rs" }
tabnas = { path = "../parser/rs" }
serde_json = "1"
```

All four entries are needed. A crate's dependencies are not passed on to
its dependents, so `tabnas-csv` alone does not put `tabnas`,
`tabnas-jsonic` or `serde_json` in your extern prelude, and the examples
above that name `tabnas::Value`, `tabnas_jsonic::make` or
`serde_json::json!` would not resolve. Only `CsvError` is re-exported.
`serde_json` is declared because the option bag is a `tabnas::Value` and
`Value::from_json` takes a `serde_json::Value`; a program that uses only
the `CsvOptions` struct can leave it out. The test suite additionally
needs `https://github.com/tabnas/support` beside the repository, for the
shared fixture runner.

## Differences from the canonical TypeScript

Every parse result is the TypeScript one: the shared fixtures in
[`../test/spec`](../test/spec), the manifest corpus in
[`../test/fixtures`](../test/fixtures) and the two third-party
conformance corpora hold all three runtimes to it. What differs is the
shape of the API and a few points where the host language has no way to
say what JavaScript says:

- **Options are a struct, or a JSON bag.** `make_with` takes a
  `CsvOptions`; `plugin()` reads the bag `use_plugin` merges over the
  defaults. The three-state `trim`, `comment`, `number` and `value` are
  `Option<bool>`, with `None` standing for the TypeScript `null`.
- **Streaming is a typed callback.** A closure cannot travel in the
  option bag, so `Stream` sits on `CsvOptions` outside serialization and
  reaches the plugin through `plugin_with` and `make_with`. There is no
  `'error'` event: a failed parse returns `Err`, as it does in Go.
- **`parse` is a convenience the other runtimes lack.** It keeps one
  default instance behind a `OnceLock`, which is the reuse the TypeScript
  and Go suites tell a caller to arrange by hand.
- **Records keep document order.** An object record is a `Value::Object`
  over an `IndexMap`, so the columns come back in header order without
  the JavaScript object's own ordering rules.
- **No token descriptions.** The TypeScript plugin registers human
  descriptions of the CSV tokens for the railroad diagram tool. The Rust
  engine has no such table and no diagram tool reads it, so that
  registration has no counterpart.
- **Lone surrogates fold to U+FFFD**, and the regular expression dialect
  is the `regex` crate's. Both come from the engine, and both are
  recorded there.

## Build and test

The engine, the jsonic base, the JSON core it needs, and the fixture
runner are path dependencies on sibling checkouts, so there is nothing
to fetch for the build:

```bash
cargo test --all-targets && cargo test --doc
```

Or, from the repository root, `make test-rs`. For what CI would say,
including formatting, clippy and the lockfile check, run
`ci/rust/run.sh`.

The suite runs every shared `../test/spec/*.tsv` fixture through the
shared runner, building a fresh parser from each row's `opts` column, and
the whole `../test/fixtures` manifest corpus. The conformance tests judge
the two third-party corpora (csv-spectrum and Go's `encoding/csv`
`readTests`) with the same divergence table the TypeScript and Go suites
carry; they run `../scripts/fetch-csv-suites.sh` themselves and FAIL,
never skip, when a corpus cannot be obtained. Beside them are the
in-language tests: the API surface, streaming, typed option values, the
`field.exact` messages, the exported string matcher on a plain jsonic
instance, the shared default parser under threads, instance reuse, the
version sites, and the embedded grammar against the file on disk.

## License

MIT.
