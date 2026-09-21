// Cross-runtime conformance, driven by the shared `test/spec/*.tsv`
// fixtures at the repository root (see ../../test/AGENTS.md).
//
// The fixture loader, the escape codec, the `ERROR:<code>` contract and
// the row loop all come from tabnas_support, whose TypeScript half
// `ts/test/parity.test.ts` and Go half `go/parity_test.go` run the SAME
// files, so the three implementations cannot drift without one of them
// going red, and neither can the loaders.
//
// What is left here is only what is specific to csv: how to build the
// parser for a row's `opts` column, and how to flatten a result.

mod common;

use std::path::Path;

use tabnas_support::{find_spec_dir, Failure, Runner, Value};

use common::{parser_for, to_failure, to_json};

#[test]
fn spec() {
    let dir = find_spec_dir(Some(Path::new(env!("CARGO_MANIFEST_DIR"))))
        .expect("a test/spec directory above rs/");

    Runner::new_with_row(|input, row| {
        // A fresh parser per row: the `opts` column is per-case, and
        // plugin options must not leak from one row into the next.
        let raw = row.named("opts");
        let options: serde_json::Value = if raw.trim().is_empty() {
            serde_json::json!({})
        } else {
            serde_json::from_str(raw)
                .map_err(|error| Failure::message(format!("opts column: {error}")))?
        };
        parser_for(options)
            .parse(input)
            .map(|value| Value::from(to_json(&value)))
            .map_err(to_failure)
    })
    // Every `*.tsv` in the directory, discovered by listing, so adding a
    // fixture runs it in every runtime without touching a runner.
    .dir(&dir);
}
