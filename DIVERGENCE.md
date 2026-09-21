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
[`test/spec/`](test/spec/), because the divergence below cannot be
written as a row of one. A fixture row compares a parsed value, or an
`ERROR:<code>` for a parse that fails with that code. The canonical
runtime neither returns a value here nor fails with a code: it throws a
raw JavaScript `TypeError` out of the plugin. There is no cell that says
that, so the entry is pinned by a test in each port instead, per
[`AGENTS.md`](AGENTS.md).

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
`unexpected` code, at the token that ends the header row. That is the
behaviour that is most honest about the input: the canonical does not
produce a column name for it either, and a port that invented one would
hand back a record whose keys no other runtime agrees on.

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

None of this is reachable in strict mode, which is the default: there no
field body is parsed, so `a,{x:1}` names a column `{x:1}` in every
runtime, and that is asserted alongside the divergence in both ports.

**Pinned by.** `TestAnObjectHeaderCellRefusesTheDocument` and
`TestAnObjectHeaderCellIsTextInStrictMode` in
[`go/js_key_test.go`](go/js_key_test.go), and
`an_object_header_cell_refuses_the_document` and
`an_object_header_cell_is_text_in_strict_mode` in
[`rs/tests/csv_test.rs`](rs/tests/csv_test.rs).

**Owner.** The canonical TypeScript. The repair is for `ts/src/csv.ts` to
decide what an object header cell means, either a defined name or a
declared CSV error code, instead of letting the language throw. When it
does, both ports follow it and this entry goes.
