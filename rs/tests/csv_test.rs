// In-language behaviour, ported from go/csv_test.go and ts/test/csv.test.ts:
// what the shared `test/spec` fixtures cannot express (the API surface,
// streaming, typed option values, the `test/fixtures` manifest corpus with
// its engine-level `jsonicOpt`, error messages, the plugin under
// `derive`), plus the same value checks the other two suites make so the
// three cannot drift on them.

mod common;

use std::fs;
use std::sync::{Arc, Mutex};

use serde_json::{json, Value as Json};
use tabnas::Value;
use tabnas_csv::{CsvOptions, FieldOptions, RecordOptions, Stream, StreamEvent, StringOptions};

use common::{
    fixture_parser, fixtures_dir, parse_default, parse_json, parser_for, parser_with, to_json,
};

fn must(src: &str, options: Json) -> Json {
    parse_json(src, options.clone())
        .unwrap_or_else(|error| panic!("parse {src:?} with {options}: {error}"))
}

fn must_default(src: &str) -> Json {
    parse_default(src).unwrap_or_else(|error| panic!("parse {src:?}: {error}"))
}

fn code_of(src: &str, options: Json) -> String {
    match parse_json(src, options.clone()) {
        Ok(value) => panic!("parse {src:?} with {options} succeeded with {value}"),
        Err(error) => error.code,
    }
}

// ---------------------------------------------------------------------------
// The test/fixtures manifest corpus (TestFixtures / `fixtures`)
// ---------------------------------------------------------------------------

#[test]
fn fixtures() {
    let dir = fixtures_dir();
    let manifest: serde_json::Map<String, Json> = serde_json::from_str(
        &fs::read_to_string(dir.join("manifest.json")).expect("manifest.json is readable"),
    )
    .expect("manifest.json is a JSON object");
    assert!(!manifest.is_empty(), "the manifest names no fixtures");

    let mut failures = Vec::new();
    let mut judged = 0;
    for (key, entry) in &manifest {
        judged += 1;
        let name = entry["name"].as_str().unwrap_or(key);
        let csv_file = entry
            .get("csvFile")
            .and_then(Json::as_str)
            .unwrap_or(key.as_str());
        let source = fs::read_to_string(dir.join(format!("{csv_file}.csv")))
            .unwrap_or_else(|error| panic!("{name}: read {csv_file}.csv: {error}"));

        let options = entry.get("opt").cloned().unwrap_or_else(|| json!({}));
        let parser = fixture_parser(&options, entry.get("jsonicOpt"));
        let result = parser.parse(&source);

        if let Some(expected_code) = entry.get("err").and_then(Json::as_str) {
            match result {
                Ok(value) => failures.push(format!(
                    "{name}: expected error {expected_code}, parsed {}",
                    to_json(&value)
                )),
                Err(error) if error.code != expected_code => failures.push(format!(
                    "{name}: expected error {expected_code}, got {}: {error}",
                    error.code
                )),
                Err(_) => {}
            }
            continue;
        }

        let expected: Json = serde_json::from_str(
            &fs::read_to_string(dir.join(format!("{key}.json")))
                .unwrap_or_else(|error| panic!("{name}: read {key}.json: {error}")),
        )
        .unwrap_or_else(|error| panic!("{name}: {key}.json: {error}"));
        match result {
            Ok(value) => {
                let got = to_json(&value);
                if got != expected {
                    failures.push(format!("{name}:\n  got      {got}\n  expected {expected}"));
                }
            }
            Err(error) => failures.push(format!("{name}: unexpected error: {error}")),
        }
    }
    assert_eq!(judged, manifest.len());
    assert!(
        failures.is_empty(),
        "{} of {} manifest fixtures failed:\n  - {}",
        failures.len(),
        manifest.len(),
        failures.join("\n  - ")
    );
}

// ---------------------------------------------------------------------------
// The API surface (TestPlugin, TestPluginWithOptions, TestPluginEmpty,
// TestUsePlugin, TestObjectOutputIsMap)
// ---------------------------------------------------------------------------

#[test]
fn plugin() {
    assert_eq!(
        must_default("a,b\n1,2\n3,4"),
        json!([{"a":"1","b":"2"},{"a":"3","b":"4"}])
    );
}

#[test]
fn plugin_with_options() {
    assert_eq!(
        must("a,b\n1,2", json!({"object": false})),
        json!([["1", "2"]])
    );
}

#[test]
fn plugin_empty() {
    assert_eq!(must_default(""), json!([]));
}

#[test]
fn the_documented_stacks_agree() {
    // Every documented way of building the parser reproduces the TypeScript
    // `new Tabnas().use(jsonic).use(Csv)` result.
    let wanted = json!([{"a":"1","b":"2"}]);
    assert_eq!(
        to_json(&tabnas_csv::parse("a,b\n1,2").expect("parses")),
        wanted
    );
    assert_eq!(
        to_json(&tabnas_csv::make().parse("a,b\n1,2").expect("parses")),
        wanted
    );
    assert_eq!(
        to_json(
            &parser_with(CsvOptions::default())
                .parse("a,b\n1,2")
                .expect("parses")
        ),
        wanted
    );
    assert_eq!(
        to_json(&parser_for(json!({})).parse("a,b\n1,2").expect("parses")),
        wanted
    );

    let mut manual = tabnas_jsonic::make();
    tabnas_csv::csv(&mut manual, &CsvOptions::default()).expect("installs");
    assert_eq!(to_json(&manual.parse("a,b\n1,2").expect("parses")), wanted);
}

#[test]
fn the_plugin_bag_merges_over_the_defaults() {
    // `use_plugin` deep-merges the caller's bag over the plugin defaults,
    // so a partial bag keeps every default it does not name.
    let parser = parser_for(json!({"field": {"nonameprefix": "col"}}));
    let options = parser
        .plugin_options("csv")
        .expect("the csv options are recorded");
    let bag = to_json(options);
    assert_eq!(bag["field"]["nonameprefix"], "col");
    assert_eq!(bag["field"]["empty"], "");
    assert_eq!(bag["header"], true);
    assert_eq!(bag["strict"], true);
    assert_eq!(
        to_json(&parser.parse("a\n1,2").expect("parses")),
        json!([{"a":"1","col1":"2"}])
    );
}

#[test]
fn the_defaults_are_the_typescript_ones() {
    let defaults = serde_json::to_value(CsvOptions::default()).expect("serializes");
    assert_eq!(
        defaults,
        json!({
            "trim": null, "comment": null, "number": null, "value": null,
            "header": true, "object": true, "strict": true,
            "field": {"separation": null, "nonameprefix": "field~", "empty": "", "names": null, "exact": false},
            "record": {"separators": null, "empty": false},
            "string": {"quote": "\"", "csv": null},
        })
    );
    let round_trip =
        CsvOptions::from_value(&CsvOptions::default().to_value()).expect("round-trips");
    assert_eq!(
        serde_json::to_value(round_trip).expect("serializes"),
        defaults
    );
}

#[test]
fn installing_twice_does_not_double_the_grammar() {
    let mut parser = tabnas_jsonic::make();
    tabnas_csv::csv(&mut parser, &CsvOptions::default()).expect("installs");
    tabnas_csv::csv(&mut parser, &CsvOptions::default()).expect("a re-run is a no-op");
    assert_eq!(
        to_json(&parser.parse("a,b\n1,2").expect("parses")),
        json!([{"a":"1","b":"2"}])
    );
}

#[test]
fn a_derived_instance_rebuilds_the_grammar() {
    let parser = parser_for(json!({"object": false}));
    let child = parser.derive(|_options| {}).expect("derives");
    assert_eq!(
        to_json(&child.parse("a,b\n1,2").expect("parses")),
        json!([["1", "2"]])
    );
}

#[test]
fn object_output_is_a_map_in_document_order() {
    let value = tabnas_csv::parse("name,age\nAlice,30\nBob,25").expect("parses");
    let Value::Array(records) = &value else {
        panic!("expected an array, got {value:?}");
    };
    let Value::Object(first) = &records[0] else {
        panic!("expected an object record, got {:?}", records[0]);
    };
    let keys: Vec<&String> = first.keys().collect();
    assert_eq!(keys, ["name", "age"]);
    assert_eq!(first["name"], Value::String("Alice".to_string()));
    assert_eq!(first["age"], Value::String("30".to_string()));
}

// ---------------------------------------------------------------------------
// Behaviour (TestEmptyRecords, TestHeader, TestComma, TestDoubleQuotes,
// TestTrim, TestComment, TestNumber, TestValue, TestSeparators,
// TestRecordSeparators, TestUnstrict, TestEmptyAnyType, TestFieldExact)
// ---------------------------------------------------------------------------

#[test]
fn empty_records() {
    assert_eq!(
        must_default("a\n1\n\n2\n3\n\n\n4\n"),
        json!([{"a":"1"},{"a":"2"},{"a":"3"},{"a":"4"}])
    );
    assert_eq!(
        must("a\n1\n\n2\n3\n\n\n4\n", json!({"record": {"empty": true}})),
        json!([{"a":"1"},{"a":""},{"a":"2"},{"a":"3"},{"a":""},{"a":""},{"a":"4"}])
    );
    assert_eq!(must_default("\n"), json!([]));
    assert_eq!(
        must_default("\r\n\r\na,b\r\nA,B\r\n\r\n"),
        json!([{"a":"A","b":"B"}])
    );
    assert_eq!(
        must("a#X\n1\n#Y\n2\n3\n\n#Z\n4\n#Q", json!({"comment": true})),
        json!([{"a":"1"},{"a":"2"},{"a":"3"},{"a":"4"}])
    );
    assert_eq!(
        must(
            "a#X\n1\n#Y\n2\n3\n\n#Z\n4\n#Q",
            json!({"comment": true, "record": {"empty": true}})
        ),
        json!([{"a":"1"},{"a":""},{"a":"2"},{"a":"3"},{"a":""},{"a":""},{"a":"4"}])
    );
}

#[test]
fn header() {
    assert_eq!(must_default("\na,b\nA,B"), json!([{"a":"A","b":"B"}]));
    assert_eq!(
        must("\na,b\nA,B", json!({"header": false})),
        json!([{"field~0":"a","field~1":"b"},{"field~0":"A","field~1":"B"}])
    );
    assert_eq!(
        must("\na,b\nA,B", json!({"header": false, "object": false})),
        json!([["a", "b"], ["A", "B"]])
    );
    assert_eq!(
        must(
            "\na,b\nA,B",
            json!({"header": false, "field": {"names": ["a", "b"]}})
        ),
        json!([{"a":"a","b":"b"},{"a":"A","b":"B"}])
    );
}

#[test]
fn comma() {
    let cases = [
        ("a\n1,", json!([{"a":"1","field~1":""}])),
        ("a\n,1", json!([{"a":"","field~1":"1"}])),
        ("a,b\n1,2,", json!([{"a":"1","b":"2","field~2":""}])),
        ("a,b\n,1,2", json!([{"a":"","b":"1","field~2":"2"}])),
        ("a\n1,\n", json!([{"a":"1","field~1":""}])),
        ("a,b\n1,2,\n", json!([{"a":"1","b":"2","field~2":""}])),
    ];
    for (src, wanted) in cases {
        assert_eq!(must_default(src), wanted, "comma {src:?}");
    }
    assert_eq!(must_default("\na"), json!([]));
    assert_eq!(must("a\n1,", json!({"object": false})), json!([["1", ""]]));
    assert_eq!(
        must("a,b\n,1,2", json!({"object": false})),
        json!([["", "1", "2"]])
    );
}

#[test]
fn double_quotes() {
    let cases = [
        ("a\n\"b\"", "b"),
        ("a\n\"\"\"b\"", "\"b"),
        ("a\n\"b\"\"\"", "b\""),
        ("a\n\"\"\"b\"\"\"", "\"b\""),
        ("a\n\"b\"\"c\"", "b\"c"),
        ("a\n\"b\"\"c\"\"d\"", "b\"c\"d"),
        ("a\n\"b\"\"c\"\"d\"\"e\"", "b\"c\"d\"e"),
        ("a\n\"\"\"\"\"b\"", "\"\"b"),
        ("a\n\"b\"\"\"\"\"", "b\"\""),
        ("a\n\"\"\"\"\"b\"\"\"\"\"", "\"\"b\"\""),
    ];
    for (src, wanted) in cases {
        assert_eq!(must_default(src), json!([{"a": wanted}]), "quotes {src:?}");
    }
}

#[test]
fn trim() {
    assert_eq!(must_default("a\n b"), json!([{"a":" b"}]));
    assert_eq!(must_default("a\nb "), json!([{"a":"b "}]));
    assert_eq!(must_default("a\n b "), json!([{"a":" b "}]));
    assert_eq!(must("a\n b", json!({"trim": true})), json!([{"a":"b"}]));
    assert_eq!(must("a\nb ", json!({"trim": true})), json!([{"a":"b"}]));
    assert_eq!(
        must("a\n b c ", json!({"trim": true})),
        json!([{"a":"b c"}])
    );
}

#[test]
fn comment() {
    assert_eq!(must_default("a\n# b"), json!([{"a":"# b"}]));
    assert_eq!(must("a\n# b", json!({"comment": true})), json!([]));
    assert_eq!(
        must("a\n b #c", json!({"comment": true})),
        json!([{"a":" b "}])
    );
    // Non-strict mode enables comment (and trim) by default.
    assert_eq!(must("a\n# b", json!({"strict": false})), json!([]));
    assert_eq!(must("a\n b ", json!({"strict": false})), json!([{"a":"b"}]));
}

#[test]
fn number() {
    assert_eq!(must_default("a\n1"), json!([{"a":"1"}]));
    assert_eq!(must("a\n1", json!({"number": true})), json!([{"a":1.0}]));
    assert_eq!(
        must("a\n1e2", json!({"number": true})),
        json!([{"a":100.0}])
    );
    assert_eq!(must_default("a\n1e2"), json!([{"a":"1e2"}]));
    assert_eq!(
        must("a\n1e2", json!({"strict": false})),
        json!([{"a":100.0}])
    );
}

// A numeric header cell becomes a column NAME, and the canonical
// TypeScript names it with JavaScript's object-key coercion, which is
// `String(number)`. Rust's own f64 formatting disagrees in three places
// that reach a key: negative zero keeps its sign (`-0`), the exponent
// thresholds at 1e21 and 1e-7 are never taken, and a large integral float
// prints its exact binary value (`123456789012345683968`) rather than its
// shortest round-tripping digits. Every expectation below was taken from
// ts/src/csv.ts.
//
// This is deliberately NOT a shared `test/spec` row. The Go port names
// EVERY numeric header cell "" -- `names[i], _ = v.(string)` in
// go/csv.go drops a non-string -- so a shared row would fail there. It
// belongs in test/spec the moment Go is fixed.
#[test]
fn a_numeric_header_cell_is_named_the_way_javascript_names_it() {
    for (src, key) in [
        ("-0\nx", "0"),
        ("0\nx", "0"),
        ("1\nx", "1"),
        ("1e2\nx", "100"),
        ("-1.5\nx", "-1.5"),
        // The upper exponent threshold: 1e21 switches to exponent form,
        // everything below it spells out.
        ("1e20\nx", "100000000000000000000"),
        ("1e21\nx", "1e+21"),
        ("1e22\nx", "1e+22"),
        // The lower one: 1e-7 switches, 1e-6 does not.
        ("0.000001\nx", "0.000001"),
        ("1e-7\nx", "1e-7"),
        ("1.5e-7\nx", "1.5e-7"),
        // Shortest round-tripping digits, not the exact binary value.
        ("123456789012345680000\nx", "123456789012345680000"),
    ] {
        let mut record = serde_json::Map::new();
        record.insert(key.to_string(), Json::String("x".to_string()));
        assert_eq!(
            must(src, json!({"number": true})),
            Json::Array(vec![Json::Object(record)]),
            "header key for {src:?}"
        );
    }
}

// The same spelling reaches a field body, through the text rules that
// concatenate a token value with the text around it (the TypeScript
// `'' + r.o0.val`). Checked against ts/src/csv.ts on the same inputs.
#[test]
fn a_number_in_field_text_is_spelled_the_way_javascript_spells_it() {
    for (src, field) in [
        ("a\n-0 x", "0 x"),
        ("a\n1e21 x", "1e+21 x"),
        ("a\n1e-7 z", "1e-7 z"),
    ] {
        assert_eq!(
            must(src, json!({"number": true})),
            json!([{ "a": field }]),
            "field text for {src:?}"
        );
    }
}

#[test]
fn value() {
    assert_eq!(must_default("a\ntrue"), json!([{"a":"true"}]));
    assert_eq!(must("a\ntrue", json!({"value": true})), json!([{"a":true}]));
    assert_eq!(
        must("a\nfalse", json!({"value": true})),
        json!([{"a":false}])
    );
    assert_eq!(must("a\nnull", json!({"value": true})), json!([{"a":null}]));
}

#[test]
fn separators() {
    assert_eq!(
        must(
            "a|b|c\nA|B|C\nAA|BB|CC",
            json!({"field": {"separation": "|"}})
        ),
        json!([{"a":"A","b":"B","c":"C"},{"a":"AA","b":"BB","c":"CC"}])
    );
    assert_eq!(
        must("a~~b~~c\nA~~B~~C", json!({"field": {"separation": "~~"}})),
        json!([{"a":"A","b":"B","c":"C"}])
    );
}

#[test]
fn record_separators() {
    assert_eq!(
        must(
            "a,b,c%A,B,C%AA,BB,CC",
            json!({"record": {"separators": "%"}})
        ),
        json!([{"a":"A","b":"B","c":"C"},{"a":"AA","b":"BB","c":"CC"}])
    );
}

#[test]
fn unstrict() {
    assert_eq!(
        must("a,b\n1,2", json!({"strict": false})),
        json!([{"a":1.0,"b":2.0}])
    );
    assert_eq!(
        must("a,b,c\ntrue,[1,2],{x:1}", json!({"strict": false})),
        json!([{"a":true,"b":[1.0,2.0],"c":{"x":1.0}}])
    );
    let src = "a,b,c\ntrue,[1,2],{x:{y:\"q\\\"w\"}}\n x , 'y\\'y', \"z\\\"z\"\n";
    assert_eq!(
        must(src, json!({"strict": false})),
        json!([
            {"a":true,"b":[1.0,2.0],"c":{"x":{"y":"q\"w"}}},
            {"a":"x","b":"y'y","c":"z\"z"},
        ])
    );
    // Trailing content after a complete embedded value is a syntax error.
    assert_eq!(code_of("a\n{x:1}y", json!({"strict": false})), "unexpected");
}

#[test]
fn empty_takes_any_value() {
    assert_eq!(
        must("a,b,c\n1,,3", json!({"field": {"empty": null}})),
        json!([{"a":"1","b":null,"c":"3"}])
    );
    assert_eq!(
        must("a,b\n1,", json!({"field": {"empty": false}})),
        json!([{"a":"1","b":false}])
    );
    assert_eq!(
        must("a,b\n1,", json!({"field": {"empty": 42}})),
        json!([{"a":"1","b":42.0}])
    );
    // And through the typed options.
    let typed = parser_with(CsvOptions {
        field: FieldOptions {
            empty: json!(false),
            ..Default::default()
        },
        ..Default::default()
    });
    assert_eq!(
        to_json(&typed.parse("a,b\n1,").expect("parses")),
        json!([{"a":"1","b":false}])
    );
}

#[test]
fn field_exact() {
    let exact = json!({"field": {"exact": true}});
    assert_eq!(code_of("a,b\n1,2,3", exact.clone()), "csv_extra_field");
    assert_eq!(code_of("a,b\n1", exact.clone()), "csv_missing_field");
    assert_eq!(must("a,b\n1,2", exact), json!([{"a":"1","b":"2"}]));
}

#[test]
fn field_exact_error_messages_interpolate_their_placeholders() {
    // The error and hint templates use {braces}; the engine's injector
    // fills them from the details the plugin supplies with the bad token.
    // Row counts from 1 and includes the header, so the second data row
    // reports as row 3, the line number an editor would show.
    let plain = |message: String| -> String {
        let mut out = String::new();
        let mut chars = message.chars().peekable();
        while let Some(c) = chars.next() {
            if c == '\x1b' && chars.peek() == Some(&'[') {
                for code in chars.by_ref() {
                    if code == 'm' {
                        break;
                    }
                }
            } else {
                out.push(c);
            }
        }
        out
    };
    let message = |src: &str| -> String {
        match parse_json(src, json!({"field": {"exact": true}})) {
            Ok(value) => panic!("{src:?} parsed: {value}"),
            Err(error) => plain(error.to_string()),
        }
    };

    let extra = message("a,b\n1,2,3");
    assert!(extra.contains("unexpected extra field value: 3"), "{extra}");
    assert!(
        extra.contains("Row 2 has too many fields (the first of which is: 3)"),
        "{extra}"
    );
    assert!(extra.contains("Only 2"), "{extra}");

    let missing = message("a,b\n1");
    assert!(
        missing.contains("Row 2 has too few fields. 2 fields per row are expected."),
        "{missing}"
    );

    let later = message("a,b\n1,2\n3,4,5");
    assert!(later.contains("Row 3 has too many fields"), "{later}");

    for text in [&extra, &missing, &later] {
        for name in ["fsrc", "row", "len"] {
            assert!(
                !text.contains(&format!("{{{name}}}")),
                "no {{{name}}} left: {text}"
            );
            assert!(
                !text.contains(&format!("${name}")),
                "no ${name} left: {text}"
            );
        }
    }
}

// ---------------------------------------------------------------------------
// Streaming (TestStream / `stream`)
// ---------------------------------------------------------------------------

#[test]
fn stream() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let sink = Arc::clone(&events);
    let parser = parser_with(CsvOptions {
        stream: Some(Stream::new(move |event| {
            sink.lock().expect("the event log").push(event);
        })),
        ..Default::default()
    });

    let result = parser.parse("a,b\n1,2\n3,4\n5,6").expect("parses");
    // Streamed records are not stored in the result.
    assert_eq!(to_json(&result), json!([]));

    let events = events.lock().expect("the event log");
    assert_eq!(events.first(), Some(&StreamEvent::Start));
    assert_eq!(events.last(), Some(&StreamEvent::End));
    let records: Vec<Json> = events
        .iter()
        .filter_map(|event| match event {
            StreamEvent::Record(record) => Some(to_json(record)),
            _ => None,
        })
        .collect();
    assert_eq!(
        records,
        vec![
            json!({"a":"1","b":"2"}),
            json!({"a":"3","b":"4"}),
            json!({"a":"5","b":"6"}),
        ]
    );
    assert_eq!(events.len(), 5);
}

// ---------------------------------------------------------------------------
// Typed options
// ---------------------------------------------------------------------------

#[test]
fn typed_options_match_the_bag() {
    let typed = parser_with(CsvOptions {
        header: false,
        object: false,
        trim: Some(true),
        field: FieldOptions {
            separation: Some("|".to_string()),
            ..Default::default()
        },
        record: RecordOptions {
            separators: Some("%".to_string()),
            empty: false,
        },
        string: StringOptions {
            quote: "'".to_string(),
            csv: None,
        },
        ..Default::default()
    });
    let bag = json!({
        "header": false, "object": false, "trim": true,
        "field": {"separation": "|"}, "record": {"separators": "%"}, "string": {"quote": "'"},
    });
    let src = "a|'b|c' |d%e|f|g";
    assert_eq!(
        to_json(&typed.parse(src).expect("parses")),
        must(src, bag.clone())
    );
    assert_eq!(must(src, bag), json!([["a", "b|c", "d"], ["e", "f", "g"]]));
}

#[test]
fn field_names_through_typed_options() {
    let parser = parser_with(CsvOptions {
        header: false,
        field: FieldOptions {
            names: Some(vec!["x".to_string(), "y".to_string()]),
            exact: true,
            ..Default::default()
        },
        ..Default::default()
    });
    assert_eq!(
        to_json(&parser.parse("1,2\n3,4").expect("parses")),
        json!([{"x":"1","y":"2"},{"x":"3","y":"4"}])
    );
    assert_eq!(parser.parse("1,2,3").unwrap_err().code, "csv_extra_field");
}

// ---------------------------------------------------------------------------
// Prototype pollution (ts/test/prototype-pollution.test.ts)
// ---------------------------------------------------------------------------

#[test]
fn a_header_column_named_proto_is_an_ordinary_key() {
    // A Rust map has no prototype chain, so the hazard the TypeScript
    // test guards against cannot arise; what is pinned is that the column
    // survives as an ordinary key rather than being dropped.
    assert_eq!(
        must_default("__proto__,b\n1,2\n"),
        json!([{"__proto__":"1","b":"2"}])
    );
    assert_eq!(
        must_default("constructor,b\n1,2\n"),
        json!([{"constructor":"1","b":"2"}])
    );
}

// ---------------------------------------------------------------------------
// The shared default parser under threads
// ---------------------------------------------------------------------------

#[test]
fn parse_is_safe_to_share_across_threads() {
    let handles: Vec<_> = (0..8)
        .map(|index| {
            std::thread::spawn(move || {
                for row in 0..20 {
                    let src = format!("a,b\n{index},{row}");
                    let value = tabnas_csv::parse(&src).expect("parses");
                    assert_eq!(
                        to_json(&value),
                        json!([{"a": index.to_string(), "b": row.to_string()}])
                    );
                }
            })
        })
        .collect();
    for handle in handles {
        handle.join().expect("a thread panicked");
    }
}
