// A parse from a reader (`Tabnas::parse_reader`, admin ADR-30). `make()`
// declares `lex.window`, and the quoted field, the one token this grammar
// cuts itself, scans the lexer's window and asks it to fill where a field
// runs past it. Every row of the shared fixtures parses from a reader to
// the row's expected value; quoted fields parse the same from a reader
// under windows of a few characters, so that every field meets the
// window's edge, as from their text; and a field longer than the window
// does too.

mod common;

use std::io::Read;
use std::path::Path;

use tabnas::{Tabnas, WindowSizes};
use tabnas_support::{find_spec_dir, Failure, Runner, Value};

use common::{parser_for, to_failure, to_json};

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
