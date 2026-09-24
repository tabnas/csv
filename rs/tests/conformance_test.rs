// Third-party conformance corpora, the Rust third of the set.
// ts/test/conformance.test.ts and go/conformance_test.go run the SAME two
// corpora with the SAME divergence table, so the three runtimes cannot
// drift without one of them going red.
//
//   valid   -> must parse AND produce the corpus's expected VALUE
//   invalid -> must be REJECTED with an error
//
// The corpora are NOT committed. `scripts/fetch-csv-suites.sh` fetches
// them at pinned upstream commits into `test/suites/`, which is
// gitignored. `cargo test` has no `pretest` hook the way npm does, so this
// file runs that script itself, once per test binary, before anything
// reads a corpus; it is idempotent, verifies the pins on every run, and
// touches the network only for what is missing. That is what makes the
// suite RUN in CI, where the job is a bare `cargo test` over a fresh
// checkout.
//
// If the fetch cannot happen (no network, no `node` for the case
// extractor) the affected suite FAILS LOUDLY with the fetch command in
// the message. It never skips. A conformance suite that quietly does not
// run reports green while measuring nothing, which is worse than having
// no suite at all; the TypeScript and Go halves fail for the same reason.
// Once a corpus is present, every case in it is judged and nothing is
// silently exempt.
//
// On DIVERGENCES: go/encoding/csv is a strict RFC 4180 reader. @tabnas/csv
// is deliberately a lenient, PapaParse-compatible reader with RFC 4180
// quoting (see AGENTS.md "Conformance"). Where the two differ ON PURPOSE,
// the case is NOT skipped: the @tabnas/csv result is pinned as a positive
// assertion, AND the test re-checks that the case really still disagrees
// with the corpus, so a divergence cannot rot into a silent exemption.

mod common;

use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::OnceLock;

use serde::Deserialize;
use serde_json::{json, Value as Json};

use common::{fixture_parser, repo_root, to_json};

fn suites_dir() -> PathBuf {
    repo_root().join("test").join("suites")
}

const MISSING: &str = "conformance corpus missing under test/suites, and \
scripts/fetch-csv-suites.sh could not supply it (it needs network, and node for the \
go/encoding/csv case extractor). Run that script by hand, or `make test`, to judge \
this suite. This fails rather than skips on purpose: a conformance suite that quietly \
does not run reports green while measuring nothing.";

/// Run the pinned-commit fetch script, at most once per test binary. It
/// is idempotent and verifies the pins whether or not it fetched, so a
/// truncated or tampered corpus fails here rather than grading.
fn fetch_corpora() -> Result<(), String> {
    static FETCHED: OnceLock<Result<(), String>> = OnceLock::new();
    FETCHED
        .get_or_init(|| {
            let script = repo_root().join("scripts").join("fetch-csv-suites.sh");
            let output = Command::new("bash")
                .arg(&script)
                .current_dir(repo_root())
                .output()
                .map_err(|error| format!("could not run {}: {error}", script.display()))?;
            if output.status.success() {
                Ok(())
            } else {
                Err(format!(
                    "{} exited {}\n{}{}",
                    script.display(),
                    output.status,
                    String::from_utf8_lossy(&output.stdout),
                    String::from_utf8_lossy(&output.stderr)
                ))
            }
        })
        .clone()
}

/// Make `path` present or FAIL the test. Never a skip.
fn require_corpus(path: &Path) {
    if let Err(error) = fetch_corpora() {
        assert!(
            path.exists(),
            "{MISSING}\n  expected: {}\n  fetch failed: {error}",
            path.display()
        );
        panic!("{MISSING}\n  the corpus at {} is present but its pins could not be verified:\n  {error}", path.display());
    }
    assert!(path.exists(), "{MISSING}\n  expected: {}", path.display());
}

/// A file as text. The corpora carry a byte or two that is not UTF-8
/// (csv-spectrum's `location_coordinates`), which Go's encoder and
/// Node's `utf8` reader both turn into U+FFFD; so does this.
fn read_lossy(path: &Path) -> String {
    let bytes = fs::read(path).unwrap_or_else(|error| panic!("read {}: {error}", path.display()));
    String::from_utf8_lossy(&bytes).into_owned()
}

/// Canonical JSON: object keys sorted, so a record compares by content
/// as it does under Go's encoder and the TypeScript `JSON.stringify` of
/// an ordered object read back from the corpus. Numbers keep the shape
/// serde gives them.
fn canon(value: &Json) -> String {
    match value {
        Json::Array(items) => {
            let inner: Vec<String> = items.iter().map(canon).collect();
            format!("[{}]", inner.join(","))
        }
        Json::Object(entries) => {
            let ordered: BTreeMap<&String, &Json> = entries.iter().collect();
            let inner: Vec<String> = ordered
                .iter()
                .map(|(key, value)| format!("{}:{}", Json::String((*key).clone()), canon(value)))
                .collect();
            format!("{{{}}}", inner.join(","))
        }
        Json::Number(number) => number
            .as_f64()
            .map(|float| {
                if float.fract() == 0.0 && float.abs() < 1e15 {
                    format!("{float:.0}")
                } else {
                    float.to_string()
                }
            })
            .unwrap_or_else(|| number.to_string()),
        scalar => scalar.to_string(),
    }
}

fn show(text: &str) -> String {
    if text.chars().count() > 160 {
        let cut: String = text.chars().take(160).collect();
        format!("{cut}…")
    } else {
        text.to_string()
    }
}

fn report(label: &str, total: usize, failures: &[String]) {
    assert!(
        failures.is_empty(),
        "{label}: {}/{total} passed. FAILING ({}):\n  - {}",
        total - failures.len(),
        failures.len(),
        failures.join("\n  - ")
    );
    eprintln!("{label}: {total}/{total} passed");
}

// --------------------------------------------------------------------------
// SUITE 1: max-mapper/csv-spectrum @ d30e80f8b99d2eecb3778f1d7b9ed1cb425502ec
// --------------------------------------------------------------------------

/// One csv-spectrum case is internally inconsistent UPSTREAM: its .json is
/// a bare object where all 11 others are arrays of records, and its phone
/// number was scrubbed in the .json but not in the .csv. It cannot judge
/// any parser, so it is pinned by `spectrum_upstream_defect` instead.
const SPECTRUM_UPSTREAM_DEFECT: &str = "location_coordinates";

fn spectrum_dirs() -> (PathBuf, PathBuf) {
    let dir = suites_dir().join("csv-spectrum");
    let csv_dir = dir.join("csvs");
    let json_dir = dir.join("json");
    for path in [&dir, &csv_dir, &json_dir] {
        require_corpus(path);
    }
    (csv_dir, json_dir)
}

#[test]
fn spectrum_valid_documents_parse_to_the_expected_value() {
    let (csv_dir, json_dir) = spectrum_dirs();
    let mut names: Vec<String> = fs::read_dir(&csv_dir)
        .unwrap_or_else(|error| panic!("{MISSING}\n  {error}"))
        .filter_map(|entry| entry.ok())
        .map(|entry| entry.file_name().to_string_lossy().into_owned())
        .filter_map(|name| name.strip_suffix(".csv").map(str::to_string))
        .collect();
    names.sort();
    assert!(
        !names.is_empty(),
        "{MISSING}\n  found no .csv documents in {}",
        csv_dir.display()
    );

    let parser = tabnas_csv::make();
    let mut failures = Vec::new();
    let mut judged = 0;
    for name in &names {
        if name == SPECTRUM_UPSTREAM_DEFECT {
            continue; // judged by spectrum_upstream_defect
        }
        judged += 1;
        let src = read_lossy(&csv_dir.join(format!("{name}.csv")));
        let want: Json = serde_json::from_str(&read_lossy(&json_dir.join(format!("{name}.json"))))
            .unwrap_or_else(|error| panic!("{name}.json: {error}"));
        match parser.parse(&src) {
            Err(error) => failures.push(format!("{name}: threw {}", error.code)),
            Ok(value) => {
                let got = canon(&to_json(&value));
                let expected = canon(&want);
                if got != expected {
                    failures.push(format!(
                        "{name}: expected {} got {}",
                        show(&expected),
                        show(&got)
                    ));
                }
            }
        }
    }
    assert_eq!(
        judged,
        names.len() - 1,
        "every corpus document must be judged"
    );
    report("csv-spectrum valid", judged, &failures);
}

#[test]
fn spectrum_upstream_defect_location_coordinates_contradicts_itself() {
    let (csv_dir, json_dir) = spectrum_dirs();
    let src = read_lossy(&csv_dir.join(format!("{SPECTRUM_UPSTREAM_DEFECT}.csv")));
    let expected: Json = serde_json::from_str(&read_lossy(
        &json_dir.join(format!("{SPECTRUM_UPSTREAM_DEFECT}.json")),
    ))
    .expect("the expectation is JSON");

    let Json::Object(object) = &expected else {
        panic!(
            "upstream csv-spectrum fixed the shape of {SPECTRUM_UPSTREAM_DEFECT}.json: delete \
             SPECTRUM_UPSTREAM_DEFECT and judge this case normally"
        );
    };
    assert_eq!(
        object.get("Contact Phone Number"),
        Some(&json!("1234567890")),
        "upstream csv-spectrum changed the scrubbed phone number: re-check"
    );
    assert!(
        src.contains("2095257564"),
        "upstream csv-spectrum changed {SPECTRUM_UPSTREAM_DEFECT}.csv: re-check"
    );

    // What @tabnas/csv actually does with it, pinned so a regression shows
    // up: the correct reading, one record, faithful to the .csv, in the
    // .csv's own column order.
    let value = tabnas_csv::make().parse(&src).expect("parses");
    let got = to_json(&value);
    let Json::Array(records) = &got else {
        panic!("expected an array of records, got {got}");
    };
    assert_eq!(records.len(), 1);
    let Json::Object(record) = &records[0] else {
        panic!("expected an object record, got {}", records[0]);
    };
    let keys: Vec<&String> = record.keys().collect();
    assert_eq!(
        keys,
        [
            "Contact Phone Number",
            "Location Coordinates",
            "Cities",
            "Counties"
        ]
    );
    assert_eq!(record["Contact Phone Number"], json!("2095257564"));
    assert_eq!(
        canon(&got),
        r#"[{"Cities":"Modesto","Contact Phone Number":"2095257564","Counties":"Stanislaus","Location Coordinates":"37�36'37.8\"N 121�2'17.9\"W"}]"#
    );
}

// --------------------------------------------------------------------------
// SUITE 2: golang/go src/encoding/csv @ 862c888e612ac346c7c4d99c9392bdfd265f33b0
// --------------------------------------------------------------------------

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct GoCase {
    name: String,
    input: String,
    must_fail: bool,
    #[serde(default)]
    expected: Json,
    #[serde(default)]
    opts: Json,
    #[serde(default)]
    jsonic_opts: Json,
    #[serde(default)]
    profile: String,
}

#[derive(Debug, Deserialize)]
struct Excluded {
    name: String,
}

#[derive(Debug, Deserialize)]
struct GoCorpus {
    cases: Vec<GoCase>,
    #[serde(default)]
    excluded: Vec<Excluded>,
}

/// Deliberate, documented departures from go/encoding/csv. Each entry
/// pins what @tabnas/csv actually produces, so the behaviour is asserted
/// rather than waived. Keep in step with AGENTS.md "Conformance",
/// ts/test/conformance.test.ts and go/conformance_test.go.
fn divergences() -> BTreeMap<&'static str, (&'static str, &'static str)> {
    let bare_cr = "bare-CR-is-a-record-separator";
    let stray = "stray-quotes-are-literal-text";
    let exact = "field.exact-is-header-relative";
    BTreeMap::from([
        // (1) A bare CR is a record separator (PapaParse-compatible, and
        // what `record.separators: null` documents: "\n / \r\n / \r").
        // go/encoding/csv treats a CR not followed by LF as ordinary field
        // data. Pinned by test/fixtures/papa-two-rows-just-r.csv.
        ("BareCR", (bare_cr, r#"[["a","b"],["c","d"]]"#)),
        ("FieldCR", (bare_cr, r#"[["field"],["field"]]"#)),
        ("FieldCRCR", (bare_cr, r#"[["field"],["field"]]"#)),
        ("FieldCRCRLF", (bare_cr, r#"[["field"],["field"]]"#)),
        ("FieldCRCRLFCR", (bare_cr, r#"[["field"],["field"]]"#)),
        ("FieldCRCRLFCRCR", (bare_cr, r#"[["field"],["field"]]"#)),
        (
            "MultiFieldCRCRLFCRCR",
            (
                bare_cr,
                r#"[["field1","field2"],["field1","field2"],["",""]]"#,
            ),
        ),
        ("QuotedTrailingCRCR", (bare_cr, r#"[["field"]]"#)),
        // (2) A CRLF inside a quoted field is field data and survives
        // verbatim. go/encoding/csv normalises it to a bare LF, which is a
        // Go convenience, not an RFC 4180 requirement (section 2.6: CRLF
        // inside quotes is data).
        (
            "CRLFInQuotedField",
            (
                "quoted-CRLF-is-preserved-verbatim",
                r#"[["A","Hello\r\nHi","B"]]"#,
            ),
        ),
        // (3) Stray quotes inside an unquoted field are ordinary text, not
        // an error (PapaParse-compatible lenience). go/encoding/csv rejects
        // them with ErrBareQuote. Pinned by
        // test/fixtures/papa-unquoted-field-with-quotes-*.
        ("BadDoubleQuotes", (stray, r#"[["a\"\"b","c"]]"#)),
        ("BadBareQuote", (stray, r#"[["a \"word\"","b"]]"#)),
        ("BadTrailingQuote", (stray, r#"[["a word","b\""]]"#)),
        // (4) `trim` trims surrounding whitespace but does NOT then
        // re-read the remainder as a quoted field, so ` "a"` is the
        // three-character text `"a"`. Go's TrimLeadingSpace trims first
        // and unquotes after. Pinned by
        // test/fixtures/papa-quoted-field-with-whitespace-around-quotes.csv.
        (
            "TrimQuote",
            (
                "whitespace-then-quote-is-literal-text",
                r#"[["\"a\""," b","c"]]"#,
            ),
        ),
        // (5) `field.exact` is header-relative by documentation. These
        // cases run with `header: false` and no `field.names`, so there is
        // no expected count and the option is correctly inert. Go's
        // FieldsPerRecord needs no header; @tabnas/csv has no equivalent
        // uniform-count mode.
        ("BadFieldCount", (exact, r#"[["a","b","c"],["d","e"]]"#)),
        (
            "BadFieldCountMultiple",
            (exact, r#"[["a","b","c"],["d","e"],["f"]]"#),
        ),
        ("BadFieldCount1", (exact, r#"[["a","b","c"]]"#)),
    ])
}

/// The number of go/encoding/csv cases @tabnas/csv matches outright.
/// Asserted so the headline number in AGENTS.md and README.md cannot
/// drift.
const CONFORMANCE_SCORE: usize = 39;

fn load_go_corpus() -> GoCorpus {
    let file = suites_dir().join("go-encoding-csv").join("cases.json");
    require_corpus(&file);
    let corpus: GoCorpus = serde_json::from_str(&read_lossy(&file))
        .unwrap_or_else(|error| panic!("corpus at {} is not readable: {error}", file.display()));
    assert!(
        !corpus.cases.is_empty(),
        "{MISSING}\n  corpus at {} has no cases",
        file.display()
    );
    corpus
}

/// Parse a corpus case and return its canonical JSON, or "ERROR".
fn run_go_case(case: &GoCase) -> String {
    let options = if case.opts.is_object() {
        case.opts.clone()
    } else {
        json!({})
    };
    let jsonic_opts = case.jsonic_opts.is_object().then_some(&case.jsonic_opts);
    match fixture_parser(&options, jsonic_opts).parse(&case.input) {
        Ok(value) => canon(&to_json(&value)),
        Err(_) => "ERROR".to_string(),
    }
}

#[test]
fn go_encoding_csv_valid_documents_parse_to_the_expected_value() {
    let corpus = load_go_corpus();
    let divergences = divergences();
    let mut failures = Vec::new();
    let mut total = 0;

    for case in corpus.cases.iter().filter(|case| !case.must_fail) {
        total += 1;
        let got = run_go_case(case);
        let want = canon(&case.expected);

        if let Some((why, pinned)) = divergences.get(case.name.as_str()) {
            if got != *pinned {
                failures.push(format!(
                    "{} [divergence {why}]: pinned {} but got {}: update divergences or fix the regression",
                    case.name,
                    show(pinned),
                    show(&got)
                ));
            } else if got == want {
                failures.push(format!(
                    "{}: listed as a divergence but now MATCHES the corpus: delete the divergences entry",
                    case.name
                ));
            }
            continue;
        }

        if got != want {
            failures.push(format!(
                "{} [{}]: expected {} got {}",
                case.name,
                case.profile,
                show(&want),
                show(&got)
            ));
        }
    }
    report("go/encoding/csv valid", total, &failures);
}

#[test]
fn go_encoding_csv_invalid_documents_are_rejected() {
    let corpus = load_go_corpus();
    let divergences = divergences();
    let mut failures = Vec::new();
    let mut total = 0;

    for case in corpus.cases.iter().filter(|case| case.must_fail) {
        total += 1;
        let got = run_go_case(case);

        if let Some((why, pinned)) = divergences.get(case.name.as_str()) {
            if got != *pinned {
                failures.push(format!(
                    "{} [divergence {why}]: pinned {} but got {}: update divergences or fix the regression",
                    case.name,
                    show(pinned),
                    show(&got)
                ));
            } else if got == "ERROR" {
                failures.push(format!(
                    "{}: listed as a divergence but is now REJECTED like the corpus requires: delete the divergences entry",
                    case.name
                ));
            }
            continue;
        }

        if got != "ERROR" {
            failures.push(format!(
                "{} [{}]: was ACCEPTED as {} but RFC 4180 / encoding/csv rejects it",
                case.name,
                case.profile,
                show(&got)
            ));
        }
    }
    report("go/encoding/csv invalid-rejected", total, &failures);
}

#[test]
fn the_conformance_score_is_exactly_the_documented_one() {
    let corpus = load_go_corpus();
    let divergences = divergences();
    for name in divergences.keys() {
        assert!(
            corpus.cases.iter().any(|case| case.name == *name),
            "divergences names a case not in the corpus: {name}"
        );
    }
    assert_eq!(
        corpus.cases.len() - divergences.len(),
        CONFORMANCE_SCORE,
        "documented in AGENTS.md as {CONFORMANCE_SCORE}/{} go/encoding/csv cases conforming; \
         update AGENTS.md and README.md together with this number",
        corpus.cases.len()
    );
    assert_eq!(divergences.len(), 16, "AGENTS.md documents 16 divergences");
}

#[test]
fn the_excluded_set_is_exactly_the_documented_one() {
    let corpus = load_go_corpus();
    let mut got: Vec<&str> = corpus
        .excluded
        .iter()
        .map(|excluded| excluded.name.as_str())
        .collect();
    got.sort_unstable();

    let mut want = vec![
        // LazyQuotes: a deliberately non-RFC-4180 lenient mode with no
        // @tabnas/csv equivalent, so there is no behaviour to assert.
        "BareDoubleQuotes",
        "BareQuotes",
        "LazyOddQuotes",
        "LazyQuoteWithTrailingCRLF",
        "LazyQuotes",
        // No Input at all: these assert that Go's NewReader rejects a bad
        // Comma/Comment rune. An API validation test, not a document.
        "BadComma1",
        "BadComma2",
        "BadComma3",
        "BadComma4",
        "BadCommaComment",
        "BadComment1",
        "BadComment2",
        "BadComment3",
    ];
    want.sort_unstable();
    assert_eq!(got, want, "excluded set drifted");
}
