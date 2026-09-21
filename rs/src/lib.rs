// Copyright (c) 2021-2026 Richard Rodger, MIT License

// The engine's error carries a code, position, hint and a formatted
// report, so it is large by design and `Result<_, TabnasError>` trips
// clippy's `result_large_err`. The engine allows the lint at its own
// crate root for the same reason, and so does tabnas-jsonic; boxing here
// would make `parse` return a different shape from `Tabnas::parse` and
// from the other two ports.
#![allow(clippy::result_large_err)]

//! The CSV grammar plugin for the `tabnas` parsing engine.
//!
//! CSV text becomes an array of objects (one per row, keyed by the header
//! row) or an array of arrays, with RFC 4180 `""`-escaped quoting, custom
//! field and record separators, streaming, and a strict / non-strict mode.
//! In strict mode (the default) every field is raw text; in non-strict
//! mode a field body can hold embedded jsonic (`[1,2]`, `{x:1}`).
//!
//! The plugin is not standalone: it layers on the relaxed-JSON grammar of
//! [`tabnas_jsonic`] (the `val` / `map` / `list` / `pair` / `elem` rules)
//! and reuses the engine's lexer, comment handling and lifecycle hooks,
//! exactly as the canonical TypeScript plugin in `ts/src/csv.ts` does.
//!
//! ```
//! let value = tabnas_csv::parse("name,age\nAlice,30\nBob,25")?;
//! assert_eq!(
//!     value.to_json().to_string(),
//!     r#"[{"name":"Alice","age":"30"},{"name":"Bob","age":"25"}]"#
//! );
//! # Ok::<(), tabnas_csv::CsvError>(())
//! ```
//!
//! TypeScript is canonical: `ts/src/csv.ts` defines behaviour and option
//! defaults, and `csv-grammar.jsonic` at the repository root is the
//! grammar every runtime embeds. The shared fixtures in `test/spec/*.tsv`
//! are the parity contract across TypeScript, Go and Rust.

use std::cell::RefCell;
use std::fmt;
use std::rc::Rc;
use std::sync::{Arc, OnceLock};

use indexmap::IndexMap;
use serde::{Deserialize, Serialize};
use serde_json::json;
use tabnas::{
    ActionError, AltSpec, Context, GrammarSetting, GrammarSpec, ImperativeLexMatcher, Options,
    Plugin, PluginError, Rule, RuleSpec, Tabnas, Tin, Token, Value, TIN_BD, TIN_ST,
};

/// This crate's version. It MUST equal `ts/package.json` "version": the
/// release orchestrator rewrites both, and `tests/version_test.rs` fails
/// the build if they drift. Mirrors `VERSION` in `ts/src/csv.ts` and
/// `const VERSION` in `go/csv.go`.
pub const VERSION: &str = "0.5.7";

/// The README's Rust examples run as doctests, so a stale one fails the
/// gate rather than misleading the reader. Its `toml` and `bash` fences
/// are skipped; rustdoc runs only the `rust` ones.
#[cfg(doctest)]
#[doc = include_str!("../README.md")]
mod readme_examples {}

/// The error a failed parse produces, re-exported so callers need not
/// depend on the engine crate directly. Its `code` is the contract the
/// shared fixtures pin: `csv_extra_field`, `csv_missing_field`, and the
/// engine's own `unexpected` and `unterminated_string`.
pub use tabnas::TabnasError as CsvError;

// --- BEGIN EMBEDDED csv-grammar.jsonic ---
const GRAMMAR_TEXT: &str = r##"
# CSV Grammar Definition
# Parsed by a standard Jsonic instance and passed to jsonic.grammar()
# Function references (@ prefixed) are resolved against the refs map
#
# Token naming:
#   #LN - line ending (removed from per-instance IGNORE set)
#   #SP - whitespace  (removed from per-instance IGNORE set in strict mode)
#   #CA - comma / field separator
#   #ZZ - end of input
#   #VAL - token set: text, string, number, value literals
#
# Rules csv, newline, record, text are fully defined here.
# Rules list, elem, val are modified in code (strict mode defines from scratch;
# non-strict prepends to existing defaults to preserve JSON parsing).

{
  options: rule: { start: csv }
  options: lex: { emptyResult: [] }
  # Placeholders are {braces}: the engine's error injector (strinject in
  # @tabnas/parser) substitutes {name} from the details object passed to
  # token.bad(). A $name is left in the message verbatim.
  options: error: {
    csv_extra_field: 'unexpected extra field value: {fsrc}'
    csv_missing_field: 'missing field'
  }
  options: hint: {
    csv_extra_field: 'Row {row} has too many fields (the first of which is: {fsrc}). Only {len}\nfields per row are expected.'
    csv_missing_field: 'Row {row} has too few fields. {len} fields per row are expected.'
  }

  rule: csv: open: [
    { s: '#ZZ' }
    { s: '#LN' p: newline c: '@not-record-empty' }
    { p: record }
  ]

  rule: newline: open: [
    { s: '#LN #LN' r: newline }
    { s: '#LN' r: newline }
    { s: '#ZZ' }
    { r: record }
  ]
  rule: newline: close: [
    { s: '#LN #LN' r: newline g: end }
    { s: '#LN' r: newline g: end }
    { s: '#ZZ' g: end }
    { r: record }
  ]

  rule: record: open: [
    { p: list }
  ]
  rule: record: close: [
    { s: '#ZZ' g: end }
    { s: '#LN #ZZ' b: 1 g: end }
    { s: '#LN' r: '@record-close-next' g: end }
  ]

  rule: text: open: [
    { s: ['#VAL' '#SP'] b: 1 r: text n: { text: 1 } g: 'csv,space,follows' a: '@text-follows' }
    { s: ['#SP' '#VAL'] r: text n: { text: 1 } g: 'csv,space,leads' a: '@text-leads' }
    { s: ['#SP' '#CA #LN #ZZ'] b: 1 n: { text: 1 } g: 'csv,end' a: '@text-end' }
    { s: '#SP' n: { text: 1 } g: 'csv,space' a: '@text-space' p: '@text-space-push' }
    {}
  ]
}
"##;
// --- END EMBEDDED csv-grammar.jsonic ---

/// The grammar source every runtime embeds, verbatim: the text of
/// `csv-grammar.jsonic` at the repository root. Exposed so a test can hold
/// the embedded copy to the file on disk.
pub fn grammar_text() -> &'static str {
    GRAMMAR_TEXT
}

/// The name every csv plugin registers under, and so the namespace of its
/// plugin options (`parser.plugin_options("csv")`).
const PLUGIN_NAME: &str = "csv";

// ---------------------------------------------------------------------------
// Named function references.
//
// The grammar names every closure, and the closures are registered on the
// instance BEFORE the document is installed, because the document is what
// looks them up. The lifecycle names follow the engine's `@<rule>-<phase>`
// convention and are wired onto their rule automatically.
// ---------------------------------------------------------------------------

const CSV_BO: &str = "@csv-bo";
const CSV_AC: &str = "@csv-ac";
const RECORD_BC: &str = "@record-bc";
const TEXT_BC: &str = "@text-bc";
const TEXT_FOLLOWS: &str = "@text-follows";
const TEXT_LEADS: &str = "@text-leads";
const TEXT_END: &str = "@text-end";
const TEXT_SPACE: &str = "@text-space";
const NOT_RECORD_EMPTY: &str = "@not-record-empty";
const RECORD_CLOSE_NEXT: &str = "@record-close-next";
const TEXT_SPACE_PUSH: &str = "@text-space-push";
/// The `options.lex.match.stringcsv.make` reference: the RFC 4180 string
/// matcher factory.
const CSV_STRING: &str = "@csv-string";

/// The matcher's priority. Below the engine's own bands (fixed literals sit
/// at 2e6, quoted strings at 5e6), so a `""`-escaped field is cut before
/// the standard string matcher can see its opening quote. The same order
/// the TypeScript and Go plugins register.
const CSV_STRING_ORDER: f64 = 100_000.0;

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

/// What a streaming parse reports, in order: `Start`, one `Record` per
/// data row, then `End`. The counterpart of the TypeScript `stream(what,
/// record)` callback's `'start'`, `'record'` and `'end'` events.
#[derive(Debug, Clone, PartialEq)]
pub enum StreamEvent {
    /// The parse has begun.
    Start,
    /// One parsed record, shaped by `object` exactly as it would have been
    /// stored in the result.
    Record(Value),
    /// The parse has finished.
    End,
}

/// A streaming callback: records are handed to it as they are parsed and
/// are not stored in the result, which comes back empty.
#[derive(Clone)]
pub struct Stream(Arc<dyn Fn(StreamEvent) + Send + Sync>);

impl Stream {
    /// Wrap a callback.
    pub fn new(callback: impl Fn(StreamEvent) + Send + Sync + 'static) -> Self {
        Stream(Arc::new(callback))
    }

    fn send(&self, event: StreamEvent) {
        (self.0)(event);
    }
}

impl fmt::Debug for Stream {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str("Stream(<function>)")
    }
}

/// Field options. See [`CsvOptions`].
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct FieldOptions {
    /// The field separator, when it is not a comma. May be several
    /// characters (`"~~"`).
    pub separation: Option<String>,
    /// The key prefix for a field beyond the header's columns: the third
    /// field of a two-column document is `field~2`.
    pub nonameprefix: String,
    /// The value an empty field takes. Any JSON value; the default is the
    /// empty string.
    pub empty: serde_json::Value,
    /// Explicit column names, used in place of a header row.
    pub names: Option<Vec<String>>,
    /// Fail the parse with `csv_extra_field` or `csv_missing_field` when a
    /// row's field count differs from the header's (or from
    /// `names.len()`). Inert without a known field list.
    pub exact: bool,
}

impl Default for FieldOptions {
    fn default() -> Self {
        FieldOptions {
            separation: None,
            nonameprefix: "field~".to_string(),
            empty: serde_json::Value::String(String::new()),
            names: None,
            exact: false,
        }
    }
}

/// Record options. See [`CsvOptions`].
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
pub struct RecordOptions {
    /// The record separator characters, when they are not the default
    /// `\n` / `\r\n` / `\r`.
    pub separators: Option<String>,
    /// Keep an empty line as a record of empty fields, rather than
    /// skipping it.
    pub empty: bool,
}

/// String options. See [`CsvOptions`].
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
pub struct StringOptions {
    /// The quote character of a quoted field.
    pub quote: String,
    /// Whether to use the RFC 4180 `""`-escaping string matcher. `None`
    /// means on in strict mode and off in non-strict mode; `Some(false)`
    /// in strict mode leaves quoted fields to the jsonic string matcher
    /// with its backslash escapes.
    pub csv: Option<bool>,
}

impl Default for StringOptions {
    fn default() -> Self {
        StringOptions {
            quote: "\"".to_string(),
            csv: None,
        }
    }
}

/// The plugin options: the counterpart of the TypeScript `CsvOptions` and
/// `Csv.defaults`, and of the Go `Defaults` map. [`Default`] is the
/// documented default set.
///
/// The three-state options (`trim`, `comment`, `number`, `value`) are
/// `Option<bool>`: `None` is the TypeScript `null`, which means "off in
/// strict mode, on in non-strict mode".
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(default)]
pub struct CsvOptions {
    /// Drop the whitespace around an unquoted field.
    pub trim: Option<bool>,
    /// Lex `#` comments, so a comment line is no record and a trailing
    /// comment is not field text.
    pub comment: Option<bool>,
    /// Lex numbers, so `1e2` is the number `100` rather than text.
    pub number: Option<bool>,
    /// Lex the value keywords, so `true`, `false` and `null` are values
    /// rather than text.
    pub value: Option<bool>,
    /// The first row names the columns.
    pub header: bool,
    /// Each record is an object keyed by column name; `false` gives an
    /// array of field values.
    pub object: bool,
    /// Every field is raw text. `false` lets a field body hold embedded
    /// jsonic (`[1,2]`, `{x:1}`, a backslash-escaped string) and turns
    /// `trim`, `comment`, `number` and `value` on unless set.
    pub strict: bool,
    /// Field options.
    pub field: FieldOptions,
    /// Record options.
    pub record: RecordOptions,
    /// String options.
    pub string: StringOptions,
    /// Stream records to a callback instead of collecting them. A closure
    /// cannot travel in the plugin option bag, so it is carried here,
    /// outside serialization, and reaches the plugin through
    /// [`plugin_with`] and [`make_with`].
    #[serde(skip)]
    pub stream: Option<Stream>,
}

impl Default for CsvOptions {
    fn default() -> Self {
        CsvOptions {
            trim: None,
            comment: None,
            number: None,
            value: None,
            header: true,
            object: true,
            strict: true,
            field: FieldOptions::default(),
            record: RecordOptions::default(),
            string: StringOptions::default(),
            stream: None,
        }
    }
}

impl CsvOptions {
    /// Read options out of a plugin option bag, the form
    /// [`Tabnas::use_plugin`] merges and hands to the plugin. A missing key
    /// takes its default; an unknown key is ignored, as in the other two
    /// runtimes.
    pub fn from_value(value: &Value) -> Result<Self, String> {
        serde_json::from_value(value.to_json()).map_err(|error| format!("csv options: {error}"))
    }

    /// The options as a plugin option bag, the form [`Plugin::with_defaults`]
    /// and [`Tabnas::use_plugin`] take. The `stream` callback does not
    /// travel; see [`plugin_with`].
    pub fn to_value(&self) -> Value {
        let json = serde_json::to_value(self).expect("the option struct serializes");
        Value::from_json(&json)
    }
}

/// The settings the closures capture: the options normalised the way the
/// TypeScript plugin normalises them at the top of `Csv`.
#[derive(Clone)]
struct Settings {
    strict: bool,
    objres: bool,
    header: bool,
    trim: bool,
    record_empty: bool,
    empty: Value,
    nonameprefix: String,
    names: Option<Vec<String>>,
    exact: bool,
    stream: Option<Stream>,
}

// ---------------------------------------------------------------------------
// Node helpers
// ---------------------------------------------------------------------------

/// Assign a rule's node: the Rust spelling of TypeScript `r.node = v`.
///
/// A pushed or replaced rule SHARES its parent's node cell, so writing
/// through `rule.node.borrow_mut()` would overwrite the parent's node too.
/// Assigning installs a fresh cell instead. The cell is borrowed directly
/// only to mutate a container the rule genuinely shares: pushing a field
/// onto the enclosing record, or a record onto the document.
fn set_node(rule: &mut Rule, value: Value) {
    rule.node = Rc::new(RefCell::new(value));
}

fn list_push(node: &mut Value, value: Value) {
    match node {
        Value::Array(items) => Arc::make_mut(items).push(value),
        Value::ListRef(list) => Arc::make_mut(list).value.push(value),
        _ => {}
    }
}

fn items_of(node: &Value) -> Vec<Value> {
    match node {
        Value::Array(items) => items.as_ref().clone(),
        Value::ListRef(list) => list.value.clone(),
        _ => Vec::new(),
    }
}

fn flag(rule: &Rule, name: &str) -> bool {
    matches!(rule.u.get(name), Some(Value::Bool(true)))
}

fn counter(rule: &Rule, name: &str) -> i32 {
    rule.n.get(name).copied().unwrap_or(0)
}

/// JavaScript's `Number::toString` (ECMA-262 6.1.6.1.20), which is what
/// the canonical TypeScript `'' + value` spells, and so what a numeric cell
/// becomes as field text or as a column name.
///
/// Rust's own `f64` formatting differs from it in three ways that reach a
/// header key. It keeps the sign of negative zero (`-0`, where JavaScript
/// says `0`). It never switches to exponent form, where JavaScript does so
/// at `1e21` and at `1e-7`. And `format!("{n:.0}")` prints a large integral
/// float's exact binary value (`123456789012345683968`) rather than its
/// shortest round-tripping digits (`123456789012345680000`).
fn js_number_to_string(number: f64) -> String {
    if number.is_nan() {
        return "NaN".to_string();
    }
    // Catches -0.0 as well: JavaScript spells both zeros "0".
    if number == 0.0 {
        return "0".to_string();
    }
    if number < 0.0 {
        return format!("-{}", js_number_to_string(-number));
    }
    if number.is_infinite() {
        return "Infinity".to_string();
    }

    // The specification wants the shortest digit string `s` that round-trips
    // (length `k`), and `n`, the position of the decimal point relative to
    // it. Rust's `{:e}` yields digits of exactly that shortest length.
    let shortest = format!("{number:e}");
    let shortest_k = shortest
        .split_once('e')
        .map(|(mantissa, _)| mantissa.chars().filter(char::is_ascii_digit).count())
        .expect("a finite f64 always formats with an exponent");

    // Re-render to that same length to settle a tie. Where two digit
    // strings of length `k` are equally close to `number`, the
    // specification takes the one ending in an even digit; Rust's shortest
    // form does not, but its exactly-rounded fixed-precision form does.
    let exponential = format!("{:.*e}", shortest_k - 1, number);
    let (mantissa, exponent) = exponential
        .split_once('e')
        .expect("a finite f64 always formats with an exponent");
    // Rounding can leave trailing zeros (and, on a carry, one digit too
    // many); dropping them keeps `s` shortest, which is what `k` means.
    let digits = mantissa
        .chars()
        .filter(|digit| *digit != '.')
        .collect::<String>();
    let digits = digits.trim_end_matches('0');
    let digits = if digits.is_empty() { "0" } else { digits };
    let k = digits.len() as i32;
    let n = exponent
        .parse::<i32>()
        .expect("a formatted exponent is an integer")
        + 1;

    // The four cases of the specification, in its order. The range bounds
    // are `k <= n <= 21`, `0 < n <= 21` and `-6 < n <= 0`.
    if (k..=21).contains(&n) {
        // Integral, with n - k trailing zeros to restore.
        let mut text = digits.to_string();
        text.push_str(&"0".repeat((n - k) as usize));
        text
    } else if (1..=21).contains(&n) {
        let point = n as usize;
        format!("{}.{}", &digits[..point], &digits[point..])
    } else if (-5..=0).contains(&n) {
        format!("0.{}{}", "0".repeat(-n as usize), digits)
    } else {
        // Exponent form. `n - 1` is never 0 here, so the sign is never "+0".
        let sign = if n - 1 < 0 { '-' } else { '+' };
        let power = (n - 1).abs();
        if k == 1 {
            format!("{digits}e{sign}{power}")
        } else {
            format!("{}.{}e{sign}{power}", &digits[..1], &digits[1..])
        }
    }
}

/// A JavaScript `'' + value`: what the text actions concatenate. Text and
/// string tokens carry their text; a number or keyword (`number` /
/// `value` on) spells itself the way JavaScript does.
fn value_text(value: &Value) -> String {
    match value {
        Value::Undefined => String::new(),
        Value::Null => "null".to_string(),
        Value::Bool(flag) => flag.to_string(),
        Value::Number(number) => js_number_to_string(*number),
        Value::String(text) => text.clone(),
        Value::Text(text) => text.string.clone(),
        other => other.to_json().to_string(),
    }
}

/// A token's value as text (`r.o0.val`), or its source when it carries no
/// value at all.
fn token_text(token: &Token) -> String {
    if token.val.is_undefined() {
        token.src.as_str().to_string()
    } else {
        value_text(&token.val)
    }
}

/// A header cell as a column name: `obj[cell] = ...` applies
/// ToPropertyKey, which for anything but a symbol is ToString.
///
/// `None` means JavaScript cannot make a primitive of the value at all,
/// which is every OBJECT. A jsonic object is allocated with a null
/// prototype, so it inherits neither `toString` nor `Symbol.toPrimitive`
/// and the canonical runtime throws
/// `TypeError: Cannot convert object to primitive value` instead of
/// naming the column. This port cannot raise a JavaScript `TypeError`, so
/// it refuses the document; `DIVERGENCE.md` records that choice.
///
/// Falling through to [`value_text`] is what made that necessary: its
/// last-resort arm renders the value as JSON, which named the column
/// `{"x":1.0}` for `{x:1}` and `[1.0,2.0]` for `[1,2]`, neither of which
/// the canonical runtime produces for any input.
fn key_text(value: &Value) -> Option<String> {
    match value {
        Value::Undefined
        | Value::Null
        | Value::Bool(_)
        | Value::Number(_)
        | Value::String(_)
        | Value::Text(_) => Some(value_text(value)),
        Value::Array(items) => join_text(items),
        // An object, and the two reference wrappers the engine keeps for
        // its own bookkeeping, have no ToString to apply.
        _ => None,
    }
}

/// `Array.prototype.toString`, which is `join(',')` with no separator
/// argument (ECMA-262 23.1.3.17 and 23.1.3.34): every element is
/// converted by the same rules, `null` and `undefined` become the empty
/// string, and a nested array joins recursively, so `[1,[2,3]]` flattens
/// to `1,2,3` and `[]` is the empty string.
///
/// A null ELEMENT is the empty string while a null CELL is `null`: the
/// empty string comes from `join`, not from ToString, so it applies only
/// inside an array.
///
/// The recursion needs no depth bound and no seen-set. A parsed value is
/// a TREE: the engine folds each finished rule's value into its parent
/// and never stores a reference to an ancestor, so no element can reach
/// its own array and the walk always terminates. Its depth is the
/// document's bracket nesting, which `tabnas-jsonic` already bounds at
/// 127 containers with the parse budget this plugin inherits.
fn join_text(items: &[Value]) -> Option<String> {
    let mut joined = String::new();
    for (index, item) in items.iter().enumerate() {
        if 0 < index {
            joined.push(',');
        }
        match item {
            Value::Null | Value::Undefined => {}
            other => joined.push_str(&key_text(other)?),
        }
    }
    Some(joined)
}

fn number(value: usize) -> Value {
    Value::Number(value as f64)
}

// ---------------------------------------------------------------------------
// The closures the grammar names
// ---------------------------------------------------------------------------

/// The text of the previous text rule's node: the accumulator a chained
/// text rule (`r: text`) extends.
fn prev_text(rule: &Rule) -> String {
    rule.prev_rule
        .as_ref()
        .map(|prev| value_text(&prev.node.borrow()))
        .unwrap_or_default()
}

/// `r.node = v.node = text`, where `v` is the rule itself on the first
/// text rule of a chain and the previous one afterwards.
fn assign_text(rule: &mut Rule, chained: bool, text: String) {
    let value = Value::String(text);
    if chained {
        if let Some(prev) = rule.prev_rule.as_ref() {
            *prev.node.borrow_mut() = value.clone();
        }
    }
    set_node(rule, value);
}

/// Text then space: `x ` continues a field, so keep the text and read on.
fn text_follows(rule: &mut Rule, _context: &mut Context) -> Result<(), ActionError> {
    let chained = counter(rule, "text") != 1;
    let head = if chained {
        prev_text(rule)
    } else {
        String::new()
    };
    let text = head + &rule.o0().map(token_text).unwrap_or_default();
    assign_text(rule, chained, text);
    Ok(())
}

/// Space then text: the space is field text unless it leads a trimmed
/// field.
fn text_leads(trim: bool) -> impl Fn(&mut Rule, &mut Context) -> Result<(), ActionError> {
    move |rule, _context| {
        let count = counter(rule, "text");
        let chained = count != 1;
        let head = if chained {
            prev_text(rule)
        } else {
            String::new()
        };
        let space = if 2 <= count || !trim {
            rule.o0().map(|token| token.src.as_str().to_string())
        } else {
            None
        };
        let text = head
            + &space.unwrap_or_default()
            + &rule
                .o1()
                .map(|token| token.src.as_str().to_string())
                .unwrap_or_default();
        assign_text(rule, chained, text);
        Ok(())
    }
}

/// Trailing space before a separator, line end or end of source: field
/// text unless trimmed.
fn text_end(trim: bool) -> impl Fn(&mut Rule, &mut Context) -> Result<(), ActionError> {
    move |rule, _context| {
        let chained = counter(rule, "text") != 1;
        let head = if chained {
            prev_text(rule)
        } else {
            String::new()
        };
        let space = if trim {
            None
        } else {
            rule.o0().map(|token| token.src.as_str().to_string())
        };
        assign_text(rule, chained, head + &space.unwrap_or_default());
        Ok(())
    }
}

/// A lone space: in strict mode it is field text (unless trimmed); in
/// non-strict mode the action does nothing and the alternate pushes `val`
/// to read an embedded value.
fn text_space(
    strict: bool,
    trim: bool,
) -> impl Fn(&mut Rule, &mut Context) -> Result<(), ActionError> {
    let end = text_end(trim);
    move |rule, context| {
        if strict {
            end(rule, context)
        } else {
            Ok(())
        }
    }
}

/// `text` before-close: hand the field text (or the embedded value a
/// pushed `val` produced) to the enclosing `val`. This is `r.parent.node =
/// ...` in the canonical grammar, and it writes into the parent's cell.
fn text_before_close(rule: &mut Rule, _context: &mut Context) -> Result<(), ActionError> {
    let value = if rule.child_node.is_undefined() {
        rule.node.borrow().clone()
    } else {
        rule.child_node.clone()
    };
    if let Some(parent) = rule.parent_node.as_ref() {
        *parent.borrow_mut() = value;
    }
    Ok(())
}

fn record_index(context: &Context) -> usize {
    match context.u.get("recordI") {
        Some(Value::Number(index)) => *index as usize,
        _ => 0,
    }
}

/// `record` before-close: the header row names the columns; every later
/// row becomes a record, checked against the field count under
/// `field.exact`, shaped by `object`, and either streamed or pushed onto
/// the document.
fn record_before_close(
    settings: Settings,
) -> impl Fn(
    &mut Rule,
    &mut Context,
    Option<&tabnas::RuleSnapshot>,
    Option<Token>,
) -> Result<Option<Token>, ActionError> {
    move |rule, context, _next, out| {
        let record_i = record_index(context);
        let fields: Option<Vec<String>> = match context.u.get("fields") {
            // The header branch below stores these as strings, so every
            // one of them converts; `key_text` answers `None` only for a
            // composite, which cannot be here.
            Some(Value::Array(names)) => names.iter().map(key_text).collect(),
            _ => settings.names.clone(),
        };

        if record_i == 0 && settings.header {
            let mut names: Vec<Value> = Vec::new();
            for cell in items_of(&rule.child_node) {
                // An OBJECT cell has no ToString, and the canonical
                // runtime throws a TypeError rather than naming the
                // column. This port cannot raise one, so it refuses the
                // document with the engine's inherited `unexpected` code
                // instead of inventing a name. `DIVERGENCE.md` records
                // that choice.
                let Some(name) = key_text(&cell) else {
                    let mut token = context.t0().cloned().unwrap_or_else(|| {
                        Token::new("#BD", TIN_BD, Value::Undefined, "", Default::default())
                    });
                    token.bad("unexpected");
                    return Ok(Some(token));
                };
                names.push(Value::String(name));
            }
            context.u.insert("fields".to_string(), Value::array(names));
        } else {
            let mut record = items_of(&rule.child_node);

            // The field-count check is independent of the result SHAPE: it
            // only needs a known field list (from the header row or
            // `field.names`), so it fires for arrays as well as objects.
            if let Some(fields) = fields.as_ref() {
                if settings.exact && record.len() != fields.len() {
                    let code = if record.len() > fields.len() {
                        "csv_extra_field"
                    } else {
                        "csv_missing_field"
                    };
                    // The messages interpolate {row}, {len} and {fsrc}, so
                    // the details are supplied. `row` counts from 1 and
                    // includes the header line, so it matches the line
                    // number a spreadsheet or editor reports.
                    let mut details = vec![
                        ("row".to_string(), number(record_i + 1)),
                        ("len".to_string(), number(fields.len())),
                    ];
                    if let Some(extra) = record.get(fields.len()) {
                        details.push(("fsrc".to_string(), extra.clone()));
                    }
                    let mut token = context.t0().cloned().unwrap_or_else(|| {
                        Token::new("#BD", TIN_BD, Value::Undefined, "", Default::default())
                    });
                    token.bad_with_details(code, details);
                    return Ok(Some(token));
                }
            }

            let filled = |cell: Option<&Value>| match cell {
                Some(value) if !value.is_undefined() => value.clone(),
                _ => settings.empty.clone(),
            };

            let shaped = if settings.objres {
                let mut object = IndexMap::new();
                let mut index = 0;
                if let Some(fields) = fields.as_ref() {
                    for (position, name) in fields.iter().enumerate() {
                        object.insert(name.clone(), filled(record.get(position)));
                    }
                    index = fields.len();
                }
                while index < record.len() {
                    object.insert(
                        format!("{}{index}", settings.nonameprefix),
                        filled(record.get(index)),
                    );
                    index += 1;
                }
                Value::object(object)
            } else {
                for cell in record.iter_mut() {
                    if cell.is_undefined() {
                        *cell = settings.empty.clone();
                    }
                }
                Value::array(record)
            };

            match settings.stream.as_ref() {
                Some(stream) => stream.send(StreamEvent::Record(shaped)),
                None => list_push(&mut rule.node.borrow_mut(), shaped),
            }
        }

        context
            .u
            .insert("recordI".to_string(), number(record_i + 1));
        Ok(out)
    }
}

/// Register every closure the grammar names, and the string matcher
/// factory. Before the documents, because the documents look them up.
fn register_refs(parser: &mut Tabnas, settings: &Settings, quote: &str) {
    let stream = settings.stream.clone();
    parser.state_action_ref(CSV_BO, move |rule, context| {
        context.u.insert("recordI".to_string(), number(0));
        if let Some(stream) = stream.as_ref() {
            stream.send(StreamEvent::Start);
        }
        set_node(rule, Value::array(Vec::new()));
        Ok(())
    });

    let stream = settings.stream.clone();
    parser.state_action_ref(CSV_AC, move |_rule, _context| {
        if let Some(stream) = stream.as_ref() {
            stream.send(StreamEvent::End);
        }
        Ok(())
    });

    parser.state_action_with_next_ref(RECORD_BC, record_before_close(settings.clone()));
    parser.state_action_ref(TEXT_BC, text_before_close);

    parser.action_with_context(TEXT_FOLLOWS, text_follows);
    parser.action_with_context(TEXT_LEADS, text_leads(settings.trim));
    parser.action_with_context(TEXT_END, text_end(settings.trim));
    parser.action_with_context(TEXT_SPACE, text_space(settings.strict, settings.trim));

    let record_empty = settings.record_empty;
    parser.alt_condition(NOT_RECORD_EMPTY, move |_rule, _context| !record_empty);
    parser.alt_replace(RECORD_CLOSE_NEXT, move |_rule, _context| {
        Some(if record_empty { "record" } else { "newline" }.to_string())
    });

    let strict = settings.strict;
    parser.alt_push(TEXT_SPACE_PUSH, move |_rule, _context| {
        Some(if strict { "" } else { "val" }.to_string())
    });

    parser.lex_match_factory_ref(CSV_STRING, csv_string_matcher(quote));
}

// ---------------------------------------------------------------------------
// The RFC 4180 string matcher
// ---------------------------------------------------------------------------

/// The custom CSV string matcher factory: `"a""b"` is the text `a"b`.
///
/// A reduced copy of the standard jsonic string matcher, with the doubled
/// quote as the only escape. It is the counterpart of the TypeScript
/// `buildCsvStringMatcher` export and the Go `BuildCsvStringMatcher`, in
/// the shape the engine's `options.lex.match.<name>.make` reference takes:
/// register it with [`Tabnas::lex_match_factory_ref`] and name it from an
/// options document.
///
/// ```
/// use tabnas::GrammarSpec;
///
/// let mut parser = tabnas_jsonic::make();
/// parser.lex_match_factory_ref("@csv-string", tabnas_csv::csv_string_matcher("\""));
/// let spec = GrammarSpec::from_value(serde_json::json!({
///     "options": { "lex": { "match": {
///         "stringcsv": { "order": 100000, "make": "@csv-string" },
///     } } },
/// }))?;
/// parser.grammar(&spec)?;
/// assert_eq!(parser.parse(r#""a""b""#)?, tabnas::Value::String("a\"b".to_string()));
/// # Ok::<(), Box<dyn std::error::Error>>(())
/// ```
///
/// The matcher runs only where the engine wants a string token, and only
/// at the quote character. A line character inside the quotes is field
/// text (a quoted field may span lines); any other control character is
/// `unprintable`; a quote left open at the end of the source is
/// `unterminated_string`. Loop exhaustion is what detects the open quote,
/// so an odd number of quotes (`"""`, `"""""`) is caught rather than read
/// as a terminated string.
pub fn csv_string_matcher(
    quote: impl Into<String>,
) -> impl Fn(&Options) -> Option<ImperativeLexMatcher> + Send + Sync + 'static {
    let quote: String = quote.into();
    move |options: &Options| {
        if quote.is_empty() {
            return None;
        }
        let quote = quote.clone();
        let line_chars: Vec<char> = options.line.chars.chars().collect();
        Some(Arc::new(
            move |lexer: &mut tabnas::Lexer<'_>, _rule: &mut Rule, _context: &mut Context| {
                let rest = lexer.remaining();
                if !rest.starts_with(quote.as_str()) {
                    return None;
                }
                let point = lexer.point();
                let mut text = String::new();
                let mut at = quote.len();
                let mut closed = false;

                while at < rest.len() {
                    let tail = &rest[at..];
                    if tail.starts_with(quote.as_str()) {
                        at += quote.len();
                        // A doubled quote is one literal quote; a single
                        // one closes the field.
                        if rest[at..].starts_with(quote.as_str()) {
                            text.push_str(&quote);
                            at += quote.len();
                        } else {
                            closed = true;
                            break;
                        }
                        continue;
                    }
                    let character = tail.chars().next().expect("inside the source");
                    if line_chars.contains(&character) {
                        text.push(character);
                    } else if (character as u32) < 32 {
                        // The diagnostic points at the control character,
                        // as it does in TypeScript.
                        lexer.advance_chars(rest[..at].chars().count());
                        return Some(lexer.bad("unprintable"));
                    } else {
                        text.push(character);
                    }
                    at += character.len_utf8();
                }

                if !closed {
                    let start = point.site.pos;
                    let end = start + rest[..at].chars().count();
                    return Some(lexer.bad_span("unterminated_string", start, end));
                }

                let source = rest[..at].to_string();
                let token = lexer.token("#ST", TIN_ST, Value::String(text), source.as_str(), point);
                lexer.advance_chars(source.chars().count());
                Some(token)
            },
        ))
    }
}

// ---------------------------------------------------------------------------
// The documents
// ---------------------------------------------------------------------------

/// Whole numbers as integers. The grammar text is parsed by jsonic, whose
/// numbers are all `f64`, and the engine's document loader reads an
/// alternate's `b` and `n` as integers.
fn integral(value: &mut serde_json::Value) {
    match value {
        serde_json::Value::Number(number) => {
            if let Some(float) = number.as_f64() {
                if float.fract() == 0.0 && float.abs() < 9.0e15 {
                    if float < 0.0 {
                        *value = serde_json::Value::from(float as i64);
                    } else {
                        *value = serde_json::Value::from(float as u64);
                    }
                }
            }
        }
        serde_json::Value::Array(items) => items.iter_mut().for_each(integral),
        serde_json::Value::Object(entries) => entries.values_mut().for_each(integral),
        _ => {}
    }
}

/// The embedded grammar as a serialized document: parsed by a standard
/// jsonic instance, as the TypeScript `new Tabnas().use(jsonic).parse(
/// grammarText)` and the Go `parseGrammarText` do.
fn grammar_document() -> Result<serde_json::Value, PluginError> {
    let parsed = tabnas_jsonic::parse(GRAMMAR_TEXT).map_err(|error| {
        PluginError(format!("csv: the embedded grammar does not parse: {error}"))
    })?;
    let mut document = parsed.to_json();
    if !document.is_object() || document.get("rule").is_none() {
        return Err(PluginError(
            "csv: the embedded grammar has no `rule` table".to_string(),
        ));
    }
    integral(&mut document);
    Ok(document)
}

/// The per-instance engine options, mirroring `jsonicOptions` in
/// `ts/src/csv.ts`. The static options (`rule.start`, `lex.emptyResult`,
/// `error`, `hint`) live in the grammar text.
fn options_document(
    options: &CsvOptions,
    settings: &Settings,
    flags: &LexFlags,
) -> serde_json::Value {
    let strict = settings.strict;

    // Fixed-token overrides: in strict mode the JSON structural tokens and
    // the `:` key separator are switched off; a configured field separator
    // rebinds `#CA`.
    let mut fixed = serde_json::Map::new();
    if strict {
        for name in ["#OB", "#CB", "#OS", "#CS", "#CL"] {
            fixed.insert(name.to_string(), serde_json::Value::Null);
        }
    }
    if let Some(separation) = options
        .field
        .separation
        .as_deref()
        .filter(|separation| !separation.is_empty())
    {
        fixed.insert("#CA".to_string(), json!(separation));
    }

    // IGNORE set: `#LN` leaves it so row breaks are significant; in strict
    // mode `#SP` leaves it too so whitespace inside a field survives. This
    // engine REPLACES a token set outright, as the Go engine does, so the
    // survivors are listed.
    let ignore = if strict {
        json!(["#CM"])
    } else {
        json!(["#SP", "#CM"])
    };

    let mut line = json!({ "single": settings.record_empty });
    if let Some(separators) = options.record.separators.as_deref() {
        line["chars"] = json!(separators);
        line["rowChars"] = json!(separators);
    }

    let mut document = json!({
        "options": {
            "rule": { "exclude": if strict { "jsonic,imp" } else { "imp" } },
            "fixed": { "token": fixed },
            "tokenSet": { "IGNORE": ignore },
            "number": { "lex": flags.number },
            "value": { "lex": flags.value },
            "comment": { "lex": flags.comment },
            "line": line,
        },
    });
    if flags.csv_strings {
        document["options"]["lex"] = json!({
            "match": {
                "stringcsv": { "order": CSV_STRING_ORDER, "make": CSV_STRING },
            },
        });
    }
    document
}

/// The lexer switches the mode decides.
struct LexFlags {
    number: bool,
    value: bool,
    comment: bool,
    csv_strings: bool,
}

fn apply_document(parser: &mut Tabnas, document: serde_json::Value) -> Result<(), PluginError> {
    let spec = GrammarSpec::from_value(document).map_err(|error| PluginError(error.0))?;
    parser
        .grammar(&spec)
        .map_err(|error| PluginError(error.0))?;
    Ok(())
}

// ---------------------------------------------------------------------------
// The list / elem / val rules, modified in code
// ---------------------------------------------------------------------------

/// An alternate over token sequences, with the fields the CSV rules use.
fn alt(s: Vec<Vec<Tin>>, b: usize, g: &str) -> AltSpec {
    let mut alt = AltSpec::new();
    alt.s = s;
    alt.b = b;
    alt.g = g.to_string();
    alt
}

fn push_alt(name: &str) -> AltSpec {
    let mut alt = AltSpec::new();
    alt.p = Some(name.to_string());
    alt
}

fn with_push(mut alt: AltSpec, name: &str) -> AltSpec {
    alt.p = Some(name.to_string());
    alt
}

fn with_replace(mut alt: AltSpec, name: &str) -> AltSpec {
    alt.r = Some(name.to_string());
    alt
}

/// `rs.open([...])` / `rs.close([...])` with no `append`: the whole list
/// goes in front of the existing alternates, in its own order.
fn prepend_open(spec: &mut RuleSpec, alts: Vec<AltSpec>) {
    for alt in alts.into_iter().rev() {
        spec.prepend_open(alt);
    }
}

fn prepend_close(spec: &mut RuleSpec, alts: Vec<AltSpec>) {
    for alt in alts.into_iter().rev() {
        spec.prepend_close(alt);
    }
}

/// The tokens the CSV rules match.
struct Tokens {
    ln: Tin,
    ca: Tin,
    sp: Tin,
    zz: Tin,
    val: Vec<Tin>,
}

/// An empty element before a separator: `,` pushes the empty value and
/// marks the element done, so `elem` before-close does not push again.
fn empty_before_comma(empty: Value) -> impl Fn(&mut Rule, &mut Context) + Send + Sync + 'static {
    move |rule, _context| {
        list_push(&mut rule.node.borrow_mut(), empty.clone());
        rule.u_mut().insert("done".to_string(), Value::Bool(true));
    }
}

/// An empty element at the end of the line: `,\n` / `,<eof>`.
fn empty_at_end(empty: Value) -> impl Fn(&mut Rule, &mut Context) + Send + Sync + 'static {
    move |rule, _context| {
        list_push(&mut rule.node.borrow_mut(), empty.clone());
    }
}

/// Allocate the per-record field array. Under the engine's node model a
/// pushed child rule is seeded with the parent's node, and jsonic's
/// `list` before-open only allocates for an implicit list, which the CSV
/// `record -> list` path never is: without this the fields would land on
/// the inherited document array.
fn allocate_fields(rule: &mut Rule, _context: &mut Context) {
    set_node(rule, Value::array(Vec::new()));
}

/// Strict mode: `list`, `elem` and `val` are replaced entirely. The JSON
/// structural tokens are off, so only the CSV alternates are needed.
fn strict_rules(parser: &mut Tabnas, tokens: &Tokens, empty: &Value) {
    let Tokens {
        ln,
        ca,
        sp,
        zz,
        val,
    } = tokens;
    let (ln, ca, sp, zz) = (*ln, *ca, *sp, *zz);

    parser.define_rule("list", |spec| {
        spec.clear();
        spec.add_bo(allocate_fields);
        // Do not consume the LN that closes an empty record.
        spec.add_open(alt(vec![vec![ln]], 1, ""));
        spec.add_open(push_alt("elem"));
        // LN ends the record.
        spec.add_close(alt(vec![vec![ln]], 1, "end"));
        spec.add_close(alt(vec![vec![zz]], 0, "end"));
    });

    let before_comma = empty.clone();
    let at_end = empty.clone();
    parser.define_rule("elem", move |spec| {
        spec.clear();
        let mut comma = alt(vec![vec![ca]], 1, "");
        comma.add_action(empty_before_comma(before_comma));
        spec.add_open(comma);
        spec.add_open(push_alt("val"));

        let mut trailing = alt(vec![vec![ca], vec![ln, zz]], 1, "comma");
        trailing.add_action(empty_at_end(at_end));
        spec.add_close(trailing);
        // Next element.
        spec.add_close(with_replace(alt(vec![vec![ca]], 0, "comma"), "elem"));
        // LN ends the record.
        spec.add_close(alt(vec![vec![ln]], 1, "end"));
        spec.add_close(alt(vec![vec![zz]], 0, "end"));

        // Push the parsed field value, unless an open alternate already did.
        spec.add_bc(|rule, _context| {
            if !flag(rule, "done") && !rule.child_node.is_undefined() {
                let child = rule.child_node.clone();
                list_push(&mut rule.node.borrow_mut(), child);
            }
        });
    });

    let val = val.clone();
    parser.define_rule("val", move |spec| {
        spec.clear();
        // Reset the parent-seeded node so an empty value resolves to no
        // value, not to the inherited field array.
        spec.add_bo(|rule, _context| set_node(rule, Value::Undefined));
        // Text and space concatenation.
        spec.add_open(with_push(alt(vec![val.clone(), vec![sp]], 2, ""), "text"));
        spec.add_open(with_push(alt(vec![vec![sp]], 1, ""), "text"));
        // A plain value token.
        spec.add_open(alt(vec![val.clone()], 0, ""));
        // LN ends the record.
        spec.add_open(alt(vec![vec![ln]], 1, ""));
        // Coalesce: a child (text) node wins; else the matched scalar
        // token; else no value (an implicit empty field).
        spec.add_bc(|rule, context| {
            let unset = rule.node.borrow().is_undefined();
            if unset {
                if !rule.child_node.is_undefined() {
                    let child = rule.child_node.clone();
                    set_node(rule, child);
                } else if rule.os() != 0 {
                    let value = rule.resolve_open_value(0, context);
                    set_node(rule, value);
                }
            }
        });
    });
}

/// Non-strict mode: the CSV alternates go in FRONT of jsonic's, so the
/// default value alternates (handling `[1,2]`, `{x:1}` and the like) stay
/// available for an embedded value.
fn relaxed_rules(parser: &mut Tabnas, tokens: &Tokens, empty: &Value) {
    let Tokens {
        ln,
        ca,
        sp,
        zz,
        val,
    } = tokens;
    let (ln, ca, sp, zz) = (*ln, *ca, *sp, *zz);

    parser.define_rule("list", |spec| {
        spec.add_bo(allocate_fields);
        // Do not consume the LN that closes an empty record.
        prepend_open(spec, vec![alt(vec![vec![ln]], 1, "")]);
        // The elem-push fallback goes after the jsonic defaults.
        spec.add_open(push_alt("elem"));
        // LN ends the record; in front so these win over jsonic's close
        // alternates.
        prepend_close(
            spec,
            vec![alt(vec![vec![ln]], 1, "end"), alt(vec![vec![zz]], 0, "end")],
        );
    });

    let before_comma = empty.clone();
    let at_end = empty.clone();
    parser.define_rule("elem", move |spec| {
        let mut comma = alt(vec![vec![ca]], 1, "");
        comma.add_action(empty_before_comma(before_comma));
        prepend_open(spec, vec![comma]);

        let mut trailing = alt(vec![vec![ca], vec![ln, zz]], 1, "comma");
        trailing.add_action(empty_at_end(at_end));
        prepend_close(spec, vec![trailing, alt(vec![vec![ln]], 1, "end")]);
    });

    let val = val.clone();
    parser.define_rule("val", move |spec| {
        prepend_open(
            spec,
            vec![
                with_push(alt(vec![val.clone(), vec![sp]], 2, ""), "text"),
                with_push(alt(vec![vec![sp]], 1, ""), "text"),
                alt(vec![vec![ln]], 1, ""),
            ],
        );
    });
}

// ---------------------------------------------------------------------------
// Public surface
// ---------------------------------------------------------------------------

/// Whether the CSV grammar is already on this instance. A plugin re-run
/// (a derived instance rebuilds its plugins on a fresh rule set; an
/// instance that already carries the grammar must not gain a second copy
/// of every alternate) is judged by the one rule only csv installs.
fn grammar_installed(parser: &Tabnas) -> bool {
    parser.rule_names().iter().any(|name| name == "csv")
}

/// Install the CSV plugin on `parser`, which must already carry the jsonic
/// grammar: the counterpart of the TypeScript `Csv` plugin function and
/// the Go `Csv`.
///
/// ```
/// let mut parser = tabnas_jsonic::make();
/// tabnas_csv::csv(&mut parser, &tabnas_csv::CsvOptions::default())?;
/// let value = parser.parse("a,b\n1,2")?;
/// assert_eq!(value.to_json().to_string(), r#"[{"a":"1","b":"2"}]"#);
/// # Ok::<(), Box<dyn std::error::Error>>(())
/// ```
///
/// In order: the closures the grammar names are registered, the
/// per-instance engine options are applied, the embedded grammar is
/// installed with every alternate tagged `csv`, and the `list`, `elem` and
/// `val` rules are modified in code (replaced in strict mode, extended in
/// non-strict mode, so embedded jsonic values keep working).
pub fn csv(parser: &mut Tabnas, options: &CsvOptions) -> Result<(), PluginError> {
    if grammar_installed(parser) {
        return Ok(());
    }

    let strict = options.strict;

    // In strict mode jsonic value parsing is off and every field is raw
    // text. In non-strict mode the three-state options default to on.
    let (trim, comment, number, value) = if strict {
        (
            options.trim.unwrap_or(false),
            options.comment.unwrap_or(false),
            options.number.unwrap_or(false),
            options.value.unwrap_or(false),
        )
    } else {
        (
            options.trim.unwrap_or(true),
            options.comment.unwrap_or(true),
            options.number.unwrap_or(true),
            options.value.unwrap_or(true),
        )
    };
    let csv_strings = if strict {
        options.string.csv != Some(false)
    } else {
        options.string.csv == Some(true)
    };

    let settings = Settings {
        strict,
        objres: options.object,
        header: options.header,
        trim,
        record_empty: options.record.empty,
        empty: Value::from_json(&options.field.empty),
        nonameprefix: options.field.nonameprefix.clone(),
        names: options.field.names.clone(),
        exact: options.field.exact,
        stream: options.stream.clone(),
    };
    let flags = LexFlags {
        number,
        value,
        comment,
        csv_strings,
    };

    register_refs(parser, &settings, &options.string.quote);
    apply_document(parser, options_document(options, &settings, &flags))?;

    let spec =
        GrammarSpec::from_value(grammar_document()?).map_err(|error| PluginError(error.0))?;
    parser
        .grammar_with_setting(&spec, &GrammarSetting::groups("csv"))
        .map_err(|error| PluginError(format!("csv: failed to apply the grammar: {}", error.0)))?;

    // Usually [#TX, #ST, #NR, #VL].
    let tokens = Tokens {
        ln: parser.token("#LN"),
        ca: parser.token("#CA"),
        sp: parser.token("#SP"),
        zz: parser.token("#ZZ"),
        val: parser.token_set("VAL").unwrap_or_default(),
    };
    if strict {
        strict_rules(parser, &tokens, &settings.empty);
    } else {
        relaxed_rules(parser, &tokens, &settings.empty);
    }
    Ok(())
}

/// The plugin form of [`csv`], for [`Tabnas::use_plugin`], with the
/// documented defaults: the counterpart of `tn.use(Csv, options)` in
/// TypeScript and `j.UseDefaults(Csv, Defaults, options)` in Go. The
/// option bag `use_plugin` takes is deep-merged over the defaults and
/// read as a [`CsvOptions`].
///
/// ```
/// use tabnas::Value;
///
/// let mut parser = tabnas_jsonic::make();
/// let options = Value::from_json(&serde_json::json!({ "object": false }));
/// parser.use_plugin(tabnas_csv::plugin(), Some(options))?;
/// assert_eq!(parser.parse("a,b\n1,2")?.to_json().to_string(), r#"[["1","2"]]"#);
/// # Ok::<(), Box<dyn std::error::Error>>(())
/// ```
pub fn plugin() -> Plugin {
    Plugin::new(PLUGIN_NAME, |parser, options| {
        let options = CsvOptions::from_value(options).map_err(PluginError)?;
        csv(parser, &options)
    })
    .with_defaults(CsvOptions::default().to_value())
}

/// The plugin over typed options, including a [`Stream`] callback, which
/// cannot travel in the option bag. The bag `use_plugin` passes is
/// recorded on the instance as the plugin's options but the typed ones
/// are what the grammar is built from.
pub fn plugin_with(options: CsvOptions) -> Plugin {
    let defaults = options.to_value();
    Plugin::new(PLUGIN_NAME, move |parser, _bag| csv(parser, &options)).with_defaults(defaults)
}

/// Build a CSV parser with the given options: the counterpart of
/// `new Tabnas().use(jsonic).use(Csv, options)` and the Go
/// `jsonic.Make()` + `UseDefaults(Csv, Defaults, options)`.
///
/// Infallible by design: the jsonic base and the embedded grammar are
/// fixed, so a failure here is a bug in this crate rather than anything a
/// caller did.
///
/// ```
/// use tabnas_csv::CsvOptions;
///
/// let parser = tabnas_csv::make_with(CsvOptions { header: false, object: false, ..Default::default() });
/// assert_eq!(parser.parse("a,b\n1,2")?.to_json().to_string(), r#"[["a","b"],["1","2"]]"#);
/// # Ok::<(), tabnas_csv::CsvError>(())
/// ```
pub fn make_with(options: CsvOptions) -> Tabnas {
    let mut parser = tabnas_jsonic::make();
    parser
        .use_plugin(plugin_with(options), None)
        .expect("the csv grammar documents are fixed and valid");
    parser
}

/// Build a CSV parser with the default options.
///
/// ```
/// let parser = tabnas_csv::make();
/// assert_eq!(parser.parse("a\n\"b\"\"c\"")?.to_json().to_string(), r#"[{"a":"b\"c"}]"#);
/// assert_eq!(parser.parse("a\n\"b").unwrap_err().code, "unterminated_string");
/// # Ok::<(), tabnas_csv::CsvError>(())
/// ```
pub fn make() -> Tabnas {
    make_with(CsvOptions::default())
}

/// Parse a CSV source string with the shared default parser.
///
/// The engine is built once, on first use, and reused after that. Reuse
/// is safe: [`Tabnas::parse`] takes `&self` and builds a fresh parse
/// context per call, and `Tabnas` is `Send + Sync`, so concurrent callers
/// share one installed grammar instead of each rebuilding it, which is
/// what dominates a small parse.
///
/// Use [`make`] or [`make_with`] instead when the parser needs
/// configuring: that returns a fresh instance and leaves this one alone.
///
/// ```
/// let value = tabnas_csv::parse("a,b\n1,2\n3,4")?;
/// assert_eq!(value.to_json().to_string(), r#"[{"a":"1","b":"2"},{"a":"3","b":"4"}]"#);
/// # Ok::<(), tabnas_csv::CsvError>(())
/// ```
pub fn parse(src: &str) -> Result<Value, CsvError> {
    static DEFAULT: OnceLock<Tabnas> = OnceLock::new();
    DEFAULT.get_or_init(make).parse(src)
}
