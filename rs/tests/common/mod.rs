// Shared test helpers. Cargo compiles this module into EVERY integration
// test binary, so an item only one binary uses is dead code in the
// others; the allow keeps that from being a warning rather than hiding
// anything real.
#![allow(dead_code)]
// The engine's error is large by design (it carries the whole report), and
// the engine allows this lint at its own crate root for the same reason.
#![allow(clippy::result_large_err)]

use std::path::{Path, PathBuf};

use serde_json::Value as Json;
use tabnas::{CommentDef, Tabnas, Value, ValueDef};
use tabnas_csv::CsvOptions;
use tabnas_support::Failure;

/// The repository root: the parent of `rs/`.
pub fn repo_root() -> &'static Path {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .expect("rs/ has a parent")
}

/// The shared `test/fixtures` directory of `.csv` / `.json` pairs.
pub fn fixtures_dir() -> PathBuf {
    repo_root().join("test").join("fixtures")
}

/// A fresh parser with the plugin options given as a JSON bag, the way
/// the Go runner builds one (`jsonic.Make()` then `UseDefaults(Csv,
/// Defaults, opts)`): the bag is deep-merged over the plugin's defaults.
pub fn parser_for(options: serde_json::Value) -> Tabnas {
    let mut parser = tabnas_jsonic::make();
    parser
        .use_plugin(tabnas_csv::plugin(), Some(Value::from_json(&options)))
        .unwrap_or_else(|error| panic!("csv plugin: {error}"));
    parser
}

/// A fresh parser over typed options.
pub fn parser_with(options: CsvOptions) -> Tabnas {
    tabnas_csv::make_with(options)
}

/// An engine value flattened through JSON, as the Go runner's
/// `jsonFlatten` does: `Undefined` and non-finite numbers become `null`,
/// and the `MapRef` / `ListRef` / `Text` wrappers become their plain
/// values. CSV's vocabulary is JSON's, so nothing is lost.
pub fn to_json(value: &Value) -> serde_json::Value {
    value.to_json()
}

/// A parse error as the runner's failure: the code the fixture pins, and
/// the rendered report for the failure message.
pub fn to_failure(error: tabnas::TabnasError) -> Failure {
    Failure::new(error.code.clone())
        .at(error.row, error.col)
        .with_message(error.to_string())
}

/// Parse with a fresh parser over a JSON option bag, flattened to JSON.
pub fn parse_json(
    src: &str,
    options: serde_json::Value,
) -> Result<serde_json::Value, tabnas::TabnasError> {
    parser_for(options).parse(src).map(|value| to_json(&value))
}

/// Parse with the default options, flattened to JSON.
pub fn parse_default(src: &str) -> Result<serde_json::Value, tabnas::TabnasError> {
    parse_json(src, serde_json::json!({}))
}

/// Apply the manifest's engine-level `jsonicOpt` (`value.def`,
/// `comment.def`) to a jsonic instance before the csv plugin goes on, as
/// `parseFixture` does in Go and `j.options(entry.jsonicOpt)` in TS.
pub fn apply_jsonic_options(parser: &mut Tabnas, jsonic_opt: &Json) {
    parser
        .set_options(|options| {
            if let Some(defs) = jsonic_opt.pointer("/value/def").and_then(Json::as_object) {
                for (name, def) in defs {
                    match def {
                        Json::Null => {
                            options.value.definitions.shift_remove(name);
                        }
                        Json::Object(fields) => {
                            options.value.definitions.insert(
                                name.clone(),
                                ValueDef {
                                    val: Some(Value::from_json(
                                        fields.get("val").unwrap_or(&Json::Null),
                                    )),
                                    matcher: None,
                                    transform: None,
                                    consume: false,
                                },
                            );
                        }
                        other => panic!("value.def.{name}: unsupported {other}"),
                    }
                }
            }
            if let Some(defs) = jsonic_opt.pointer("/comment/def").and_then(Json::as_object) {
                for (name, def) in defs {
                    let fields = def
                        .as_object()
                        .unwrap_or_else(|| panic!("comment.def.{name}"));
                    let start = fields.get("start").and_then(Json::as_str).unwrap_or("");
                    let end = fields.get("end").and_then(Json::as_str);
                    options.comment.definitions.insert(
                        name.clone(),
                        CommentDef {
                            line: end.is_none(),
                            start: start.to_string(),
                            end: end.unwrap_or("").to_string(),
                            lex: true,
                            suffixes: Vec::new(),
                            suffix_matcher: None,
                            eat_line: false,
                        },
                    );
                }
            }
        })
        .expect("the jsonic options apply");
}

pub fn fixture_parser(options: &Json, jsonic_opt: Option<&Json>) -> Tabnas {
    let mut parser = tabnas_jsonic::make();
    if let Some(jsonic_opt) = jsonic_opt {
        apply_jsonic_options(&mut parser, jsonic_opt);
    }
    parser
        .use_plugin(tabnas_csv::plugin(), Some(Value::from_json(options)))
        .expect("the csv plugin installs");
    parser
}
