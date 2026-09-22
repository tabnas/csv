// The exported string matcher factory, exercised on a plain jsonic
// instance the way go/csv_export_test.go exercises `BuildCsvStringMatcher`
// (and ts exports `buildCsvStringMatcher`): registered by name, named
// from an options document, and asked to read a `""`-escaped string.

use tabnas::GrammarSpec;

#[test]
fn the_string_matcher_is_usable_on_a_plain_jsonic_instance() {
    let mut parser = tabnas_jsonic::make();
    parser.lex_match_factory_ref("@csv-string", tabnas_csv::csv_string_matcher("\""));
    let spec = GrammarSpec::from_value(serde_json::json!({
        "options": { "lex": { "match": {
            "stringcsv": { "order": 100000, "make": "@csv-string" },
        } } },
    }))
    .expect("the options document is valid");
    parser.grammar(&spec).expect("the options apply");

    let value = parser
        .parse(r#""a""b""#)
        .expect("a doubled quote is an escape");
    assert_eq!(value, tabnas::Value::String("a\"b".to_string()));
}

#[test]
fn the_quote_character_is_configurable() {
    let mut parser = tabnas_jsonic::make();
    parser.lex_match_factory_ref("@csv-string", tabnas_csv::csv_string_matcher("|"));
    let spec = GrammarSpec::from_value(serde_json::json!({
        "options": { "lex": { "match": {
            "stringcsv": { "order": 100000, "make": "@csv-string" },
        } } },
    }))
    .expect("the options document is valid");
    parser.grammar(&spec).expect("the options apply");

    let value = parser.parse("|a,b||c|").expect("a pipe-quoted string");
    assert_eq!(value, tabnas::Value::String("a,b|c".to_string()));
}

#[test]
fn an_empty_quote_disables_the_matcher() {
    // The factory returns no matcher for an empty quote, which the engine
    // reads as "leave this matcher out", so the jsonic string lexer is
    // what reads the quotes.
    let mut parser = tabnas_jsonic::make();
    parser.lex_match_factory_ref("@csv-string", tabnas_csv::csv_string_matcher(""));
    let spec = GrammarSpec::from_value(serde_json::json!({
        "options": { "lex": { "match": {
            "stringcsv": { "order": 100000, "make": "@csv-string" },
        } } },
    }))
    .expect("the options document is valid");
    parser.grammar(&spec).expect("the options apply");

    let value = parser.parse(r#""a\"b""#).expect("a backslash escape");
    assert_eq!(value, tabnas::Value::String("a\"b".to_string()));
}

/// A quote of more than one UTF-16 code unit leaves the matcher inert,
/// as it is in the canonical: `quoteMap[src[sI]]` compares ONE code
/// unit, so a multi-character quote and an astral character (a surrogate
/// PAIR in JavaScript) can never open a field there. Measured against
/// the canonical on the same three inputs, which
/// `../test/spec/double-quote.tsv` now runs in all three runtimes; the
/// factory is asserted here because an inert matcher is the same shape
/// as an absent one from the outside.
#[test]
fn a_degenerate_quote_leaves_the_matcher_out() {
    let options = tabnas::Options::default();
    for quote in ["", "ab", "\"\"", "\u{1F600}"] {
        assert!(
            tabnas_csv::csv_string_matcher(quote)(&options).is_none(),
            "quote {quote:?} should install no matcher"
        );
    }
    for quote in ["\"", "|", "\u{20AC}"] {
        assert!(
            tabnas_csv::csv_string_matcher(quote)(&options).is_some(),
            "quote {quote:?} is one code unit and should install a matcher"
        );
    }
}
