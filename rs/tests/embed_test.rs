// The embedded grammar is the grammar. `csv-grammar.jsonic` at the
// repository root is the source of truth, and `ts/embed-grammar.js`
// copies it verbatim into `ts/src/csv.ts`, `go/csv.go` and
// `rs/src/lib.rs` between BEGIN/END markers. This holds the Rust copy to
// the file on disk, so an edit to the grammar that forgets the embed
// step (or a hand edit between the markers) fails here.

use std::fs;
use std::path::Path;

#[test]
fn the_embedded_grammar_is_the_file_on_disk() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .expect("rs/ has a parent");
    let on_disk = fs::read_to_string(root.join("csv-grammar.jsonic"))
        .expect("csv-grammar.jsonic is readable at the repository root");

    // The embed script writes a newline, then the file, into the raw
    // string; the file's own trailing newline closes it.
    let embedded = tabnas_csv::grammar_text();
    assert_eq!(
        embedded.strip_prefix('\n').unwrap_or(embedded),
        on_disk,
        "rs/src/lib.rs GRAMMAR_TEXT differs from csv-grammar.jsonic: run `node ts/embed-grammar.js`"
    );
}

#[test]
fn the_grammar_text_fits_the_raw_string() {
    // The embed script refuses a grammar holding `"##`, which would end
    // the r##"..."## literal early. Pinned here so the two agree.
    assert!(!tabnas_csv::grammar_text().contains("\"##"));
}
