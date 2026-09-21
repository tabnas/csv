# Divergences

TypeScript in [`ts/`](ts/) is canonical; the Go port in [`go/`](go/) and
the Rust port in [`rs/`](rs/) track it. This file records where a runtime
produces a **different result for the same input**, and why that
difference is allowed to stand.

Every row below was MEASURED, on 2026-09-21, by running all three
implementations over the input in its first column. Nothing here is
inferred from reading the source.

Each entry names who owns the repair, and is pinned by a test in every
port that has it, asserted so that it fails on REPAIR as loudly as on
regression. Repairing one means deleting its entry here and its tests in
the same change.

## Where these are pinned

This repository has no executable divergence register in
[`test/spec/`](test/spec/), because neither part of the divergence below
can be written as a row of one. A fixture row compares a parsed value, or
an `ERROR:<code>` for a parse that fails with that code, and it holds ONE
expected cell for all three runtimes.

- For a parsed object cell, the canonical runtime neither returns a value
  nor fails with a code: it throws a raw JavaScript `TypeError` out of the
  plugin. There is no cell that says that.
- For a `field.names` element that is not a string, the canonical returns
  a value, Go returns the same value, and Rust refuses the OPTIONS before
  a document is parsed. Expressing that needs a register with a per-port
  column, which this repository does not have, and a refusal that happens
  at `use` time has no document to put in a row either.

Both are pinned by a test in each port instead, per
[`AGENTS.md`](AGENTS.md). What the three runtimes DO agree on is in the
shared fixtures, and the entry below says which rows those are.

## An object header cell

In non-strict mode a field body is parsed, so a header cell is not always
a string. TypeScript names the column with `obj[fields[fI]] = ...`, which
applies ToPropertyKey, which for anything but a symbol is ToString. An
ARRAY has one; an OBJECT parsed by jsonic does not.

| input | TypeScript | Go | Rust |
|---|---|---|---|
| `a,[1,2]\nx,y` | `[{"a":"x","1,2":"y"}]` | the same | the same |
| `a,[1,[2,3]]\nx,y` | `[{"a":"x","1,2,3":"y"}]` | the same | the same |
| `a,[]\nx,y` | `[{"a":"x","":"y"}]` | the same | the same |
| `a,[null,1]\nx,y` | `[{"a":"x",",1":"y"}]` | the same | the same |
| `a,{x:1}\nx,y` | throws `TypeError: Cannot convert object to primitive value` | `ERROR:unexpected` | `ERROR:unexpected` |

Only the last row diverges. The four array rows are
`Array.prototype.toString`, which is `join(',')`: each element is
converted by the same rules, `null` and `undefined` become the empty
string, and a nested array joins recursively. Both ports implement that
now, and the rows are pinned for all three runtimes in
[`test/spec/unstrict.tsv`](test/spec/unstrict.tsv).

They did not always agree. Before the fix that added this entry, the same
four inputs measured as `[1 2]`, `[1 [2 3]]`, `[]` and `[<nil> 1]` in Go,
and `[1.0,2.0]`, `[1.0,[2.0,3.0]]`, `[]` and `[null,1.0]` in Rust: Go's
own bracket form and a JSON render, neither of which the canonical
produces for any input. The object cell measured as
`&{[x] map[x:1] false}` in Go, which is the engine's internal
`OrderedMap` struct, and as `{"x":1.0}` in Rust.

**Reason.** A jsonic object is allocated with a null prototype, so it
inherits neither `toString` nor `Symbol.toPrimitive`, and the language
raises before the plugin sees anything. Neither Go nor Rust can raise a
JavaScript `TypeError`, so neither can reproduce the canonical result.

Each port therefore refuses the document, with the engine's inherited
`unexpected` code, at the point where the canonical takes the name and
throws. That is the behaviour that is most honest about the input: the
canonical does not produce a column name for it either, and a port that
invented one would hand back a record whose keys no other runtime agrees
on.

**Scope.** The refusal fires only where the canonical actually takes a
column name from the cell. The canonical converts a header cell at one
site, `obj[fields[fI]] = ...` in `@record-bc`, and reaches it only when
all four of these hold, in this order: the row is a DATA record and not
the header row; the `field.exact` length check has not already returned a
bad token, because that check sits before `if (objres)`; `object` is on;
and a field list exists, from the header row or `field.names`. Both ports
now keep the header row raw, exactly as the canonical keeps
`ctx.u.fields = r.child.node`, and convert a cell at that one site, so
every other reading of the same document returns a value in all three
runtimes:

| input, non-strict | options | all three |
|---|---|---|
| `a,{x:1}\nx,y` | `object: false` | `[["x","y"]]` |
| `a,{x:1}` | default | `[]` |
| `a,{x:1}` | `object: false` | `[]` |
| `a,{x:1}\n` | default | `[]` |
| `a,{x:1}\nx,y` | `header: false` | the cell is a field VALUE |
| `a,{x:1}\nx,y,z` | `object: false`, `field.exact` | `ERROR:csv_extra_field` |
| `a,{x:1}\nx,y,z` | `field.exact` | `ERROR:csv_extra_field` |
| `a,{x:1}\nx` | `field.exact` | `ERROR:csv_missing_field` |

The last two are the ordering guard: `field.exact` is measured against
the LENGTH of a header the canonical never converted, so it fires first
and its code wins. A port that refused because an object cell was present
in the field list, rather than because a name was being taken from it,
would answer `unexpected` for both and still pass every other case here.
Every row of that table is a row of
[`test/spec/unstrict.tsv`](test/spec/unstrict.tsv), where all three
runtimes run it.

A short data row is NOT one of the exceptions: the name loop runs to the
header's length whatever the record holds, filling the missing cell with
`field.empty`, so `a,{x:1}\nx` with `field.exact` off still takes the
name and still refuses.

Three alternatives were weighed and rejected:

- **Render the cell.** This is what both ports did before, and it is the
  defect this entry replaces. A struct dump or a JSON render is a column
  name the canonical never produces for any input, in a result that looks
  successful.
- **Name the column `[object Object]`.** That is what a JavaScript object
  with the standard prototype would give, and it is not what happens here:
  the canonical throws. It invents a name too, and hides that the input is
  unsupported.
- **Declare a CSV-specific error code.** The error catalogue lives in
  [`csv-grammar.jsonic`](csv-grammar.jsonic), which is embedded verbatim
  into all three runtimes, and `errorCodes` in
  [`tabnas.plugin.json`](tabnas.plugin.json) is kept in step with it. A
  new code would therefore be declared for the canonical runtime, and
  advertised to tooling, while the canonical can never raise it. An
  inherited code that two runtimes raise and one does not is the smaller
  untruth.

The cost of the inherited code is the message: the engine renders
`unexpected` as `unexpected character(s): {src}`, with a hint about
characters not matching a rule alternative, which is not why this parse
stopped. The tests pin the CODE and the refusal, not that wording.

No PARSED object reaches this in strict mode, which is the default: there
no field body is parsed, so `a,{x:1}` names a column `{x:1}` in every
runtime. That is asserted alongside the divergence in both ports, and it
is the one row of [`test/spec/unstrict.tsv`](test/spec/unstrict.tsv) with
strict ON, kept there so the bound sits beside what it bounds.

**The same cell from an option value, and an entry that was wrong.** A
parsed cell is not the only way an object reaches the conversion.
`field.empty` is dropped into a syntactically empty cell before any rule
runs, and the header row is a row like any other, so `,a` under
`{field: {empty: <value>}}` takes its first column name from that option;
`field.names` supplies the whole field list the same way. The canonical
answers an object from there DIFFERENTLY, and what decides is
PROVENANCE, not anything the caller chose. The engine deep-merges an
option bag, so a plain source object is rebuilt as a fresh `{}` on the
way in and carries `Object.prototype` out the other side. `String`
therefore gives it `[object Object]` and nothing throws, even for an
object the caller built with `Object.create(null)`. A PARSED cell keeps
the null prototype the engine allocates it with, inherits no `toString`,
and throws.

Two earlier versions of this entry got the port side wrong, in opposite
directions, and both were repaired by MEASURING rather than by reading a
type. The first claimed that neither port could tell the two apart,
"since a cell is a cell by the time the name is taken", and recorded the
resulting refusal as a divergence owned jointly with the canonical. The
second refuted that with `%T`, on the claim that the Go type carries the
provenance: a parsed object is `*jsonic.OrderedMap`, an option object is
the `map[string]any` the caller wrote.

**Both claims were false.** The first was false because provenance IS
available. The second was false because the TYPE is not where it is
available: `OrderedMap` is a PUBLIC type with a public constructor, so
`field.empty: jsonic.NewOrderedMap()` is an option value of exactly the
type the lexer builds. Measured on 2026-09-21, under the code that the
`%T` claim shipped: `field.empty: map[string]any{"q":1}` named the column
`[object Object]`, and `field.empty: jsonic.NewOrderedMap()` with the
same contents refused the document with `ERROR:unexpected`, for one
JavaScript value the canonical names one way.

Provenance is recorded where the OPTIONS ARE READ instead, in both ports,
and travels to the name site with the value:

- Go walks the option bag once in `tagOptions` and keeps the identity of
  every object it finds. An object in that set came from an option; one
  that is not came from the lexer. A Go map needs no entry, because there
  the type does settle it: the lexer has no way to build one.
- Rust carries a `from_option` flag from the one place `field.empty` is
  read, and into `join_text`, so an object nested in an option array is
  answered like one at the top.

Identity, not equality: a parsed `{x:1}` compares EQUAL to an option
`{x:1}`, and only one of them may be named. Both ports pin that case.

| input | options | TypeScript | Go | Rust |
|---|---|---|---|---|
| `,a\nx,y` | `field.empty: {q:1}` | `[{"[object Object]":"x","a":"y"}]` | the same | the same |
| `,a\nx,y` | `field.empty: {}` | `[{"[object Object]":"x","a":"y"}]` | the same | the same |
| `,a\nx,y` | `field.empty: Object.create(null)` with `q` set | `[{"[object Object]":"x","a":"y"}]` | no such spelling | no such spelling |
| `,a\nx,y` | `field.empty: [{q:1}]` | `[{"[object Object]":"x","a":"y"}]` | the same | the same |
| `,a\nx,y` | `field.empty: ['a','b']` | `[{"a,b":"x","a":"y"}]` | the same | the same |
| `,{x:1}\nq,r` | `strict: false`, `field.empty: {x:1}` | throws `TypeError` | `ERROR:unexpected` | `ERROR:unexpected` |
| `x,y` | `header: false`, `field.names: [{q:1}]` | `[{"[object Object]":"x","field~1":"y"}]` | the same | refused when the options are read |
| `x,y` | `header: false`, `field.names: [1,2]` | `[{"1":"x","2":"y"}]` | the same | refused when the options are read |

The sixth row is the bound: the option object names column 0 and the
PARSED object in column 1 still refuses, in the same parse, with the same
contents in both.

Measured in the DEFAULT mode, so this route is not bounded by strict mode
the way a parsed cell is. Every other `field.empty` value names its column
in all three, numbers included, and
[`test/spec/header.tsv`](test/spec/header.tsv) runs `field.empty: 42` in
all three.

A Go caller can also spell a value the canonical has no spelling for at
all, and those are NOT divergences, because both ports now answer them as
the canonical answers the JavaScript value they denote. A DECLARED type
over a primitive (`type Column string`, `type Count int`) is the same
string or the same double to JavaScript, and names the same column. A
self-referential value is refused where the options are read, at the same
call the canonical throws a `RangeError` out of.
[`AGENTS.md`](AGENTS.md) carries both, with what is still open beside
them.

The null-prototype row has no Go or Rust spelling: neither value model
has a prototype to leave off, so there is one object kind in each port
and the row above it already measures it. It is in the table because it
is the case that looks like it should throw and does not, and a reader
who assumes the prototype travels with the caller's object will predict
the wrong answer for it.

**Owner of what is left here: the Rust port, and it is a PORT DEFECT
rather than an impossibility.** One thing keeps Rust in the table now.
`field.names` is typed `Option<Vec<String>>`, so a non-string element is
refused when the options are read, before any document is parsed. That is
the type the CANONICAL declares for the same option
(`names: undefined | string[]` in `ts/src/csv.ts`), and a JavaScript
caller who passes `[1, 2]` is outside it; widening the Rust type is a
breaking change to a public struct field, so it waits for the next major.
`field.empty` is typed `serde_json::Value`, CAN hold an object, and used
to be refused at the name site exactly as Go was. It is not any more: the
repair was to carry the provenance from the read site, and both ports now
do it. Do not copy the old refusal into a new port.

**Pinned by.** `TestAnObjectHeaderCellRefusesTheDocument`,
`TestAnObjectHeaderCellIsTextInStrictMode`,
`TestTheObjectRefusalReachesOnlyABuiltColumnName`,
`TestFieldExactUsesAHeaderThatWasNeverConverted` and
`TestTheParsedObjectRefusalSurvivesTheOptionObjectFix` in
[`go/js_key_test.go`](go/js_key_test.go), and
`an_object_header_cell_refuses_the_document`,
`an_object_header_cell_is_text_in_strict_mode`,
`the_object_refusal_reaches_only_a_built_column_name`,
`field_exact_uses_a_header_that_was_never_converted` and
`field_names_are_strings_in_this_port_alone` in
[`rs/tests/csv_test.rs`](rs/tests/csv_test.rs). The scope table above is
pinned for all three runtimes in
[`test/spec/unstrict.tsv`](test/spec/unstrict.tsv).

The option-value table is pinned per port, because a fixture row carries
ONE expected cell and only the `field.empty: ['a','b']` row has a value
all three runtimes agree on. Go:
`TestAnObjectOptionValueNamesTheColumnAsTheCanonicalDoes`,
`TestAnOptionObjectIsNamedWhateverGoTypeHoldsIt`,
`TestAParsedObjectIsRefusedBesideAnOptionObjectOfTheSameType`,
`TestAnArrayOptionValueNamesTheColumnWhateverGoSliceItIs`,
`TestADeclaredPrimitiveTypeNamesAColumnAsItsValueDoes` and
`TestFieldNamesKeepsEveryElementWhateverItsType` in
[`go/js_key_test.go`](go/js_key_test.go). Rust:
`an_object_option_value_names_the_column_as_the_canonical_does` and
`a_parsed_object_is_refused_beside_an_option_object` in
[`rs/tests/csv_test.rs`](rs/tests/csv_test.rs). The Rust row that remains
a divergence, `field.names`, is pinned by
`field_names_are_strings_in_this_port_alone`, which asserts the refusal
and the message, so repairing it fails as loudly as regressing it.

**Owner.** The canonical TypeScript, for the PARSED object cell. The
repair is for `ts/src/csv.ts` to decide what an object header cell means,
either a defined name or a declared CSV error code, instead of letting
the language throw. When it does, both ports follow it and that part of
this entry goes. The one option-value row left is the Rust port's alone,
and goes when its `field.names` option type widens at the next major.
