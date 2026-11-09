// A parse from a reader (`Tabnas::parse_reader`, admin ADR-30). `make()`
// declares `lex.window`, and the quoted field, the one token this grammar
// cuts itself, scans the lexer's window and asks it to fill where a field
// runs past it. Every row of the shared fixtures parses from a reader to
// the row's expected value; every row, every committed fixture and every
// document of the two vendored suites parses the same from a reader, in
// pieces of 1, 3 and 64 bytes, under windows of a few characters as well
// as the engine's own, as from its text; quoted fields parse the same
// from a reader under windows of a few characters, so that every field
// meets the window's edge; and a field longer than the window, and a long
// document under a window of 256 characters, do too.

mod common;

use std::fs;
use std::io::Read;
use std::path::Path;

use serde_json::{json, Value as Json};
use tabnas::{Tabnas, WindowSizes};
use tabnas_support::{find_spec_dir, load_spec_dir, Failure, Runner, SpecOptions, Value};

use common::{fixture_parser, fixtures_dir, parser_for, repo_root, to_failure, to_json};

/// A reader that hands its bytes over at most `size` at a time.
struct Pieces<'a> {
    bytes: &'a [u8],
    at: usize,
    size: usize,
}

impl Read for Pieces<'_> {
    fn read(&mut self, buf: &mut [u8]) -> std::io::Result<usize> {
        let n = self.size.min(buf.len()).min(self.bytes.len() - self.at);
        buf[..n].copy_from_slice(&self.bytes[self.at..self.at + n]);
        self.at += n;
        Ok(n)
    }
}

/// `parser_for` with `lex.window` set, as `make()` sets it.
fn windowed(options: serde_json::Value) -> Tabnas {
    let mut parser = parser_for(options);
    parser
        .set_options(|options| options.lex.window = true)
        .expect("lex.window sets");
    parser
}

fn outcome(parsed: Result<tabnas::Value, tabnas::TabnasError>) -> String {
    match parsed {
        Ok(value) => format!("ok {}", to_json(&value)),
        Err(e) => format!(
            "err {} at {} {}:{} {:?}",
            e.code, e.pos, e.row, e.col, e.src
        ),
    }
}

#[test]
fn make_reads_a_reader_a_window_at_a_time() {
    assert!(tabnas_csv::make().options.lex.window);
}

/// The shared fixtures, each row's input from a reader.
#[test]
fn spec_from_a_reader() {
    let dir = find_spec_dir(Some(Path::new(env!("CARGO_MANIFEST_DIR"))))
        .expect("a test/spec directory above rs/");
    Runner::new_with_row(|input, row| {
        let raw = row.named("opts");
        let options: serde_json::Value = if raw.trim().is_empty() {
            serde_json::json!({})
        } else {
            serde_json::from_str(raw)
                .map_err(|error| Failure::message(format!("opts column: {error}")))?
        };
        let reader = Pieces {
            bytes: input.as_bytes(),
            at: 0,
            size: 3,
        };
        windowed(options)
            .parse_reader(reader)
            .map(|value| Value::from(to_json(&value)))
            .map_err(to_failure)
    })
    .dir(&dir);
}

/// Quoted fields, from a reader under windows of a few characters, so that
/// a field, a doubled quote and a closing quote each meet the window's
/// edge, parse as their text does.
#[test]
fn quoted_fields_parse_the_same_under_small_windows() {
    let documents = [
        "a,b\n\"x\",\"y\"",
        "a\n\"b\"\"c\"",
        "a\n\"\"\"\"",
        "a\n\"\"",
        "a,b\n\"one, two\",\"three\nfour\"\n\"5\",6",
        "a\n\"ñ\"\"é\"\"\"",
        "a\n\"long field with \"\"quotes\"\" and, commas, and\nlines\"",
        "a\n\"x\"",
        "a\n\"x\"\"",
        "a\n\"b",
        "a\n\"b\"\"",
        "a,b\n\"\",\"\"\n\"\"\"\",x",
    ];
    let parser = windowed(serde_json::json!({}));
    let mut compared = 0;
    for document in documents {
        let whole = outcome(parser.parse(document));
        for size in [1, 2, 3] {
            for (margin, block) in [(1, 1), (2, 2), (3, 2), (8, 8)] {
                let sizes = WindowSizes {
                    margin,
                    block,
                    chunk: size,
                };
                let reader = Pieces {
                    bytes: document.as_bytes(),
                    at: 0,
                    size,
                };
                let read = outcome(parser.parse_reader_with_sizes(reader, sizes));
                assert_eq!(
                    read, whole,
                    "{document:?} in pieces of {size}, margin {margin}, block {block}"
                );
                compared += 1;
            }
        }
    }
    assert_eq!(compared, documents.len() * 12);
}

/// A quoted field longer than the window, with doubled quotes and line
/// breaks through it, parses the same from a reader as from its text.
#[test]
fn a_field_longer_than_the_window_parses_the_same_from_a_reader() {
    let mut field = String::new();
    for i in 0..20_000 {
        field.push_str(&format!("{i} \"\"q\"\" ñ,\n"));
    }
    let text = format!("a,b\n\"{field}\",tail\n\"x\",y\n");
    assert!(text.len() > 200 * 1024);
    let parser = tabnas_csv::make();
    let whole = outcome(parser.parse(&text));
    assert!(
        whole.starts_with("ok "),
        "{}",
        &whole[..80.min(whole.len())]
    );
    let reader = Pieces {
        bytes: text.as_bytes(),
        at: 0,
        size: 4096,
    };
    assert_eq!(outcome(parser.parse_reader(reader)), whole);
}

/// The first `n` bytes of `s`, cut back to a character boundary.
fn cut(s: &str, n: usize) -> &str {
    let mut end = n.min(s.len());
    while !s.is_char_boundary(end) {
        end -= 1;
    }
    &s[..end]
}

/// `parser` with `lex.window` set, as `make()` sets it.
fn with_window(mut parser: Tabnas) -> Tabnas {
    parser
        .set_options(|options| options.lex.window = true)
        .expect("lex.window sets");
    parser
}

/// `text` parsed by `parser` whole, and from a reader: a short one in
/// pieces of 1, 3 and 64 bytes, under the engine's window and windows of a
/// few characters (margin and block 1 and 1, 3 and 2, 64 and 64); one over
/// 4 KiB in pieces of 4096 bytes and of a little over the window, under the
/// engine's window. How many reader parses were compared, and where they
/// differ.
fn same_from_reader(parser: &Tabnas, at: &str, text: &str) -> (usize, Vec<String>) {
    let whole = outcome(parser.parse(text));
    let bytes = text.as_bytes();
    let mut reads: Vec<(String, String)> = Vec::new();
    if bytes.len() > 4096 {
        for size in [4096, 64 * 1024 + 3] {
            let reader = Pieces { bytes, at: 0, size };
            reads.push((
                format!("pieces of {size}"),
                outcome(parser.parse_reader(reader)),
            ));
        }
    } else {
        for size in [1, 3, 64] {
            let reader = Pieces { bytes, at: 0, size };
            reads.push((
                format!("pieces of {size}"),
                outcome(parser.parse_reader(reader)),
            ));
            for (margin, block) in [(1, 1), (3, 2), (64, 64)] {
                let sizes = WindowSizes {
                    margin,
                    block,
                    chunk: size,
                };
                let reader = Pieces { bytes, at: 0, size };
                reads.push((
                    format!("pieces of {size}, margin {margin}, block {block}"),
                    outcome(parser.parse_reader_with_sizes(reader, sizes)),
                ));
            }
        }
    }
    let failures = reads
        .iter()
        .filter(|(_, read)| *read != whole)
        .map(|(how, read)| {
            format!(
                "{at}: {:?} from a reader in {how}\n  whole:  {}\n  reader: {}",
                cut(text, 80),
                cut(&whole, 200),
                cut(read, 200)
            )
        })
        .collect();
    (reads.len(), failures)
}

/// Every row of the shared fixtures, under its options, parses the same
/// from a reader as from its text.
#[test]
fn every_row_parses_the_same_from_a_reader() {
    let dir = find_spec_dir(Some(Path::new(env!("CARGO_MANIFEST_DIR"))))
        .expect("a test/spec directory above rs/");
    let specs = load_spec_dir(&dir, &SpecOptions::default()).expect("the fixtures load");
    let mut compared = 0;
    let mut failures = Vec::new();
    for spec in &specs {
        for row in &spec.rows {
            let raw = row.named("opts");
            let options: Json = if raw.trim().is_empty() {
                json!({})
            } else {
                serde_json::from_str(raw).expect("an opts cell is JSON")
            };
            let (count, found) = same_from_reader(
                &windowed(options),
                &row.location(),
                &row.unesc_named("input"),
            );
            compared += count;
            failures.extend(found);
        }
    }
    assert!(compared > 1_000, "{compared}");
    assert!(failures.is_empty(), "{}", failures.join("\n"));
}

/// Every committed fixture, under the parser its options build, parses the
/// same from a reader as from its text.
#[test]
fn every_fixture_parses_the_same_from_a_reader() {
    let dir = fixtures_dir();
    let manifest: serde_json::Map<String, Json> = serde_json::from_str(
        &fs::read_to_string(dir.join("manifest.json")).expect("manifest.json is readable"),
    )
    .expect("manifest.json is a JSON object");
    let mut compared = 0;
    let mut failures = Vec::new();
    for (key, entry) in &manifest {
        let csv_file = entry
            .get("csvFile")
            .and_then(Json::as_str)
            .unwrap_or(key.as_str());
        let text = fs::read_to_string(dir.join(format!("{csv_file}.csv")))
            .unwrap_or_else(|error| panic!("{key}: {error}"));
        let options = entry.get("opt").cloned().unwrap_or_else(|| json!({}));
        let parser = with_window(fixture_parser(&options, entry.get("jsonicOpt")));
        let (count, found) = same_from_reader(&parser, key, &text);
        compared += count;
        failures.extend(found);
    }
    assert!(compared > 1_000, "{compared}");
    assert!(failures.is_empty(), "{}", failures.join("\n"));
}

/// Every csv-spectrum document and every go/encoding/csv case, under the
/// parser conformance_test.rs builds for it, parses the same from a reader
/// as from its text.
#[test]
fn every_suite_document_parses_the_same_from_a_reader() {
    let suites = repo_root().join("test").join("suites");
    let mut compared = 0;
    let mut failures = Vec::new();
    let mut names: Vec<_> = fs::read_dir(suites.join("csv-spectrum").join("csvs"))
        .expect("csv-spectrum is vendored")
        .filter_map(|entry| entry.ok())
        .map(|entry| entry.path())
        .filter(|path| path.extension().is_some_and(|ext| ext == "csv"))
        .collect();
    names.sort();
    let spectrum = tabnas_csv::make();
    for path in &names {
        let text = fs::read_to_string(path).expect("a csv-spectrum document reads");
        let (count, found) = same_from_reader(
            &spectrum,
            &format!("csv-spectrum {}", path.display()),
            &text,
        );
        compared += count;
        failures.extend(found);
    }
    let corpus: Json = serde_json::from_str(
        &fs::read_to_string(suites.join("go-encoding-csv").join("cases.json"))
            .expect("the go/encoding/csv corpus is vendored"),
    )
    .expect("cases.json is JSON");
    let cases = corpus["cases"].as_array().expect("cases.json has cases");
    for case in cases {
        let jsonic_opts = case["jsonicOpts"]
            .is_object()
            .then_some(&case["jsonicOpts"]);
        let parser = with_window(fixture_parser(&case["opts"], jsonic_opts));
        let (count, found) = same_from_reader(
            &parser,
            &format!("go/encoding/csv {}", case["name"]),
            case["input"].as_str().expect("a case has an input"),
        );
        compared += count;
        failures.extend(found);
    }
    assert!(!names.is_empty() && !cases.is_empty());
    assert!(compared > 500, "{compared}");
    assert!(failures.is_empty(), "{}", failures.join("\n"));
}

/// A long document of quoted fields with doubled quotes and line breaks,
/// under a window of 256 characters, which moves on thousands of times
/// before the end, parses the same as its text; so does the same document
/// with a field left open at its end, which the reader places where the
/// whole text does.
#[test]
fn a_long_document_parses_the_same_under_a_small_window() {
    let mut text = String::from("id,name,note\n");
    for i in 0..4_000 {
        text.push_str(&format!("{i},\"ñ{i} \"\"q\"\"\",\"line\nbreak, {i}\"\n"));
    }
    let broken = format!("{text}z,\"open");
    assert!(text.len() > 128 << 10, "{}", text.len());
    let parser = tabnas_csv::make();
    let small = WindowSizes {
        margin: 256,
        block: 256,
        chunk: 4096,
    };
    for (document, ok) in [(&text, true), (&broken, false)] {
        let whole = outcome(parser.parse(document));
        assert_eq!(whole.starts_with("ok "), ok, "{}", cut(&whole, 200));
        let reader = Pieces {
            bytes: document.as_bytes(),
            at: 0,
            size: 4096,
        };
        let read = outcome(parser.parse_reader_with_sizes(reader, small));
        assert!(
            read == whole,
            "whole:  {}\nreader: {}",
            cut(&whole, 200),
            cut(&read, 200)
        );
    }
}
