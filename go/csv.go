/* Copyright (c) 2021-2025 Richard Rodger, MIT License */

package tabnascsv

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	jsonic "github.com/tabnas/jsonic/go"
)

// VERSION is this module's version. It MUST equal ts/package.json
// "version": the release orchestrator rewrites both, and
// TestVersionMatchesPackageJSON fails the build if they drift.
const VERSION = "0.5.7"

// --- BEGIN EMBEDDED csv-grammar.jsonic ---
const grammarText = `
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
`

// --- END EMBEDDED csv-grammar.jsonic ---

// Csv is a jsonic plugin that adds CSV parsing support.
// Options are pre-merged with Defaults by jsonic.UseDefaults.
func Csv(j *jsonic.Jsonic, options map[string]any) error {
	// Guard against re-invocation: Use() re-runs plugins on SetOptions calls.
	if j.Decoration("csv-init") != nil {
		return nil
	}
	j.Decorate("csv-init", true)

	// READ THE OPTION BAG'S PROVENANCE ONCE, HERE, where the options are
	// read. tagOptions walks the bag and records the identity of every
	// OBJECT in it, because a Go type cannot say where an object came
	// from, and refuses a value that contains itself. Nothing in the bag
	// is rewritten: the values a caller supplied reach the records
	// unchanged.
	optionObjects, err := tagOptions(options)
	if err != nil {
		return err
	}

	strict := toBool(options["strict"])
	objres := toBool(options["object"])
	header := toBool(options["header"])

	trim := toBool(options["trim"])
	comment := toBool(options["comment"])
	opt_number := toBool(options["number"])
	opt_value := toBool(options["value"])

	fieldOpts := asOptionMap(options["field"])
	recordOpts := asOptionMap(options["record"])
	stringOpts := asOptionMap(options["string"])

	record_empty := toBool(recordOpts["empty"])

	stream := toStream(options["stream"])

	// In strict mode, Jsonic field content is not parsed.
	//
	// The jsonic string matcher is LEFT ON here, exactly as ts/src/csv.ts
	// leaves it on: the RFC 4180 matcher registered below runs first and
	// takes every field that opens with the configured quote, and the jsonic
	// matcher reads the quote characters that one does not own. So `'x y'`
	// and `` `x y` `` are the strings `x y` in a strict document, as they
	// are in the canonical, and a quote the RFC 4180 matcher cannot fire for
	// at all — `string.quote` empty, or more than one UTF-16 code unit —
	// falls through to the jsonic matcher rather than being read as text.
	//
	// This port used to switch it off (`String{Lex: false, Chars: ""}`), and
	// that answered six inputs differently from the canonical; the shared
	// rows at the end of ../test/spec/double-quote.tsv now hold all three
	// runtimes to the canonical answer. Do not reinstate it.
	if strict {
		if !isFalse(stringOpts["csv"]) {
			j.SetOptions(jsonic.Options{Lex: &jsonic.LexOptions{
				Match: map[string]*jsonic.MatchSpec{
					"stringcsv": {Order: 1e5, Make: BuildCsvStringMatcher(stringOpts)},
				},
			}})
		}
		j.SetOptions(jsonic.Options{Rule: &jsonic.RuleOptions{Exclude: "jsonic,imp"}})
	} else {
		// Fields may contain Jsonic content.
		if isTrue(stringOpts["csv"]) {
			j.SetOptions(jsonic.Options{Lex: &jsonic.LexOptions{
				Match: map[string]*jsonic.MatchSpec{
					"stringcsv": {Order: 1e5, Make: BuildCsvStringMatcher(stringOpts)},
				},
			}})
		}
		if options["trim"] == nil {
			trim = true
		}
		if options["comment"] == nil {
			comment = true
		}
		if options["number"] == nil {
			opt_number = true
		}
		if options["value"] == nil {
			opt_value = true
		}
		j.SetOptions(jsonic.Options{Rule: &jsonic.RuleOptions{Exclude: "imp"}})
	}

	fieldSep := toString(fieldOpts["separation"])
	recordSep := toString(recordOpts["separators"])

	// Jsonic option overrides (matching TS jsonicOptions). Static options
	// (rule.start, lex.emptyResult, error, hint) live in csv-grammar.jsonic.
	jsonicOptions := jsonic.Options{
		Number: &jsonic.NumberOptions{
			Lex: boolPtr(opt_number),
		},
		Value: &jsonic.ValueOptions{
			Lex: boolPtr(opt_value),
		},
		Comment: &jsonic.CommentOptions{
			Lex: boolPtr(comment),
		},
		Line: &jsonic.LineOptions{
			Single: boolPtr(record_empty),
		},
	}

	if recordSep != "" {
		jsonicOptions.Line.Chars = recordSep
		jsonicOptions.Line.RowChars = recordSep
	}

	// Fixed-token overrides: in strict mode disable JSON structural tokens
	// and the ':' key separator; swap the field separator when configured.
	if strict || fieldSep != "" {
		jsonicOptions.Fixed = &jsonic.FixedOptions{Token: map[string]*string{}}
		if strict {
			jsonicOptions.Fixed.Token["#OB"] = nil
			jsonicOptions.Fixed.Token["#CB"] = nil
			jsonicOptions.Fixed.Token["#OS"] = nil
			jsonicOptions.Fixed.Token["#CS"] = nil
			jsonicOptions.Fixed.Token["#CL"] = nil
		}
		if fieldSep != "" {
			sep := fieldSep
			jsonicOptions.Fixed.Token["#CA"] = &sep
		}
	}

	// IGNORE set: drop #LN so row breaks are significant; in strict mode
	// also drop #SP so whitespace inside fields is preserved.
	//
	// The override is written POSITION BY POSITION, not as the set to
	// install. From parser/go v0.11.0 on (parser#151) the engine overlays a
	// named token set onto the one already installed INDEX-WISE, as the
	// canonical TypeScript deep merge has always done: index i of the
	// override replaces index i of the installed set, an empty name clears
	// that index, and an override shorter than the set keeps the tail it
	// does not reach. Measured on that engine, the set this overlays is
	// [#SP #LN #CM], in that order, so every position is named rather than
	// left off. The empty string is Go's spelling of the `null` in the
	// canonical `tokenSet: { IGNORE: [...] }` of ts/src/csv.ts.
	//
	// Strict clears positions 0 and 1 (#SP, #LN) and keeps #CM; non-strict
	// clears position 1 (#LN) only, keeping #SP and #CM. go/go.mod requires
	// parser/go v0.12.0, so this overlay is the one a published build
	// resolves. Both spellings also hold under an engine before v0.11.0,
	// which installs the named set outright instead of overlaying it:
	// applyTokenSets skips an empty name there, so the same two slices
	// install the same two sets.
	if strict {
		jsonicOptions.TokenSet = map[string][]string{"IGNORE": {"", "", "#CM"}}
	} else {
		jsonicOptions.TokenSet = map[string][]string{"IGNORE": {"#SP", "", "#CM"}}
	}

	// jsonicOptions is applied AFTER Grammar() below so its TokenSet
	// override survives Grammar's internal SetOptions (which resets IgnoreSet
	// to defaults in buildConfig when the new options omit TokenSet).

	// Named function references for declarative grammar definition.
	var emptyField any = ""
	if v, ok := fieldOpts["empty"]; ok {
		emptyField = v
	}
	nonameprefix := toString(fieldOpts["nonameprefix"])
	fieldExact := toBool(fieldOpts["exact"])
	// Held as []any, not []string, because the header row is held that way
	// too: the canonical keeps `ctx.u.fields` as the RAW cells and names a
	// column only where it builds one. The two sources of a field list have
	// to be interchangeable, so this one widens rather than that one
	// narrowing.
	//
	// Every element is kept, whatever its type, because TS names a column
	// with `obj[fields[fI]] = ...`, which CONVERTS the element rather than
	// requiring a string: `names: [1, 2]` names columns "1" and "2".
	// Keeping only the strings shortened the list instead, which renamed
	// every column after the dropped one and changed the count
	// `field.exact` compares a record against.
	//
	// An EXPLICITLY EMPTY list stays non-nil. TS reads the field list as
	// `ctx.u.fields || options.field.names`, and every array is truthy
	// there, `[]` included, so `names: []` IS a field list: with
	// `field.exact` on, a one-cell record is then one field too many and
	// the parse fails with csv_extra_field. An append loop seeded from a
	// nil slice leaves nil for an empty input, which the `fields != nil`
	// guard below reads as "no field list at all" and the check is
	// skipped. asSlice allocates before it appends, so the empty case
	// keeps a non-nil zero-length slice.
	fieldNames, _ := asSlice(fieldOpts["names"])

	refs := map[jsonic.FuncRef]any{

		"@csv-bo": jsonic.StateAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if ctx.Meta == nil {
				ctx.Meta = make(map[string]any)
			}
			ctx.Meta["recordI"] = 0
			if stream != nil {
				stream("start", nil)
			}
			r.Node = make([]any, 0)
		}),

		"@csv-ac": jsonic.StateAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if stream != nil {
				stream("end", nil)
			}
		}),

		"@record-bc": jsonic.StateAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
			recordI, _ := ctx.Meta["recordI"].(int)
			var fields []any
			if fs, ok := ctx.Meta["fields"].([]any); ok {
				fields = fs
			}
			if fields == nil {
				fields = fieldNames
			}

			if recordI == 0 && header {
				// The header row is kept exactly as it was parsed, which
				// is what TS keeps: `ctx.u.fields = r.child.node`. A cell
				// is turned into a column NAME only where an object record
				// is built, so nothing is converted here. An empty array
				// is still a field list, as it is in TS where `[]` is
				// truthy, so a nil slice here would wrongly fall back to
				// field.names on the next row.
				if childArr, ok := r.Child.Node.([]any); ok {
					ctx.Meta["fields"] = childArr
				} else {
					ctx.Meta["fields"] = []any{}
				}
			} else {
				record, _ := r.Child.Node.([]any)
				if record == nil {
					record = []any{}
				}

				// The field-count check is independent of the result SHAPE: it
				// only needs a known field list (from the header row or
				// `field.names`). It used to sit inside the `objres` branch,
				// which silently disabled `field.exact` for every
				// `object: false` parse even when a header was present — a
				// documented option doing nothing. Mirrors the TS raise site.
				if fields != nil && fieldExact && len(record) != len(fields) {
					errCode := "csv_missing_field"
					if len(record) > len(fields) {
						errCode = "csv_extra_field"
					}
					// Mirror the TS `ctx.t0.bad(errCode)`: mark the current
					// lookahead token as bad with the CSV-specific error code
					// and halt the parse. The engine propagates a bad token's
					// custom code, so callers see csv_extra_field /
					// csv_missing_field here exactly as they do in TS.
					// The messages interpolate {row}, {len} and {fsrc}, so
					// the details have to be supplied — without them the
					// engine leaves the placeholders in the text it shows
					// the user. `row` counts from 1 and includes the header
					// line, so it matches the line number a spreadsheet or
					// editor reports.
					details := map[string]any{
						"row": recordI + 1,
						"len": len(fields),
					}
					if len(record) > len(fields) {
						details["fsrc"] = record[len(fields)]
					}
					if ctx.T0 != nil {
						ctx.ParseErr = ctx.T0.Bad(errCode, details)
					} else {
						ctx.ParseErr = (&jsonic.Token{
							Name: "#BD", Tin: jsonic.TinBD,
						}).Bad(errCode, details)
					}
					return
				}

				if objres {
					obj := make(map[string]any)
					i := 0

					if fields != nil {
						for fI := 0; fI < len(fields); fI++ {
							// This is the ONE place a header cell becomes
							// a column name, and the only place TS
							// converts one: `obj[fields[fI]] = ...` puts
							// the raw cell through the language's
							// ToPropertyKey, which for anything but a
							// symbol is ToString. In non-strict mode a
							// field body is parsed, so the cell can be a
							// float64, a bool, a nil or an array rather
							// than a string.
							//
							// An OBJECT has no ToString at all, and the
							// canonical runtime throws a TypeError here
							// rather than naming the column. This port
							// cannot raise one, so it refuses the document
							// with the engine's inherited `unexpected`
							// code instead of inventing a name;
							// DIVERGENCE.md records that choice. Refusing
							// at the header row instead was too early: it
							// also refused an `object: false` parse and a
							// header-only document, neither of which ever
							// asks for a name, and both of which the
							// canonical returns a value for.
							name, nameOk := jsKey(fields[fI], optionObjects)
							if !nameOk {
								if ctx.T0 != nil {
									ctx.ParseErr = ctx.T0.Bad("unexpected", nil)
								} else {
									ctx.ParseErr = (&jsonic.Token{
										Name: "#BD", Tin: jsonic.TinBD,
									}).Bad("unexpected", nil)
								}
								return
							}
							var val any = emptyField
							if fI < len(record) && !jsonic.IsUndefined(record[fI]) {
								val = record[fI]
							}
							obj[name] = val
						}
						i = len(fields)
					}

					for ; i < len(record); i++ {
						fname := nonameprefix + strconv.Itoa(i)
						val := record[i]
						if jsonic.IsUndefined(val) {
							val = emptyField
						}
						obj[fname] = val
					}

					if stream != nil {
						stream("record", obj)
					} else if arr, ok := r.Node.([]any); ok {
						r.Node = append(arr, obj)
						if r.Parent != jsonic.NoRule && r.Parent != nil {
							r.Parent.Node = r.Node
						}
					}
				} else {
					for i := range record {
						if jsonic.IsUndefined(record[i]) {
							record[i] = emptyField
						}
					}
					if stream != nil {
						stream("record", record)
					} else if arr, ok := r.Node.([]any); ok {
						r.Node = append(arr, record)
						if r.Parent != jsonic.NoRule && r.Parent != nil {
							r.Parent.Node = r.Node
						}
					}
				}
			}
			ctx.Meta["recordI"] = recordI + 1
		}),

		"@text-bc": jsonic.StateAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if !jsonic.IsUndefined(r.Child.Node) {
				r.Parent.Node = r.Child.Node
			} else {
				r.Parent.Node = r.Node
			}
		}),

		"@text-follows": jsonic.AltAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
			prev := ""
			if r.N["text"] != 1 && r.Prev != nil && r.Prev != jsonic.NoRule {
				prev, _ = r.Prev.Node.(string)
			}
			result := prev + tokenStr(r.O0)
			r.Node = result
			if r.N["text"] == 1 {
			} else if r.Prev != nil && r.Prev != jsonic.NoRule {
				r.Prev.Node = result
			}
		}),

		"@text-leads": jsonic.AltAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
			prev := ""
			if r.N["text"] != 1 && r.Prev != nil && r.Prev != jsonic.NoRule {
				prev, _ = r.Prev.Node.(string)
			}
			sp := ""
			if r.N["text"] >= 2 || !trim {
				sp = r.O0.Src
			}
			result := prev + sp + r.O1.Src
			r.Node = result
			if r.N["text"] == 1 {
			} else if r.Prev != nil && r.Prev != jsonic.NoRule {
				r.Prev.Node = result
			}
		}),

		"@text-end": jsonic.AltAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
			prev := ""
			if r.N["text"] != 1 && r.Prev != nil && r.Prev != jsonic.NoRule {
				prev, _ = r.Prev.Node.(string)
			}
			sp := ""
			if !trim {
				sp = r.O0.Src
			}
			result := prev + sp
			r.Node = result
			if r.N["text"] == 1 {
			} else if r.Prev != nil && r.Prev != jsonic.NoRule {
				r.Prev.Node = result
			}
		}),

		"@text-space": jsonic.AltAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if strict {
				prev := ""
				if r.N["text"] != 1 && r.Prev != nil && r.Prev != jsonic.NoRule {
					prev, _ = r.Prev.Node.(string)
				}
				sp := ""
				if !trim {
					sp = r.O0.Src
				}
				result := prev + sp
				r.Node = result
				if r.N["text"] == 1 {
				} else if r.Prev != nil && r.Prev != jsonic.NoRule {
					r.Prev.Node = result
				}
			}
		}),

		"@not-record-empty": jsonic.AltCond(func(r *jsonic.Rule, ctx *jsonic.Context) bool {
			return !record_empty
		}),

		"@record-close-next": func(r *jsonic.Rule, ctx *jsonic.Context) string {
			if record_empty {
				return "record"
			}
			return "newline"
		},

		"@text-space-push": func(r *jsonic.Rule, ctx *jsonic.Context) string {
			if strict {
				return ""
			}
			return "val"
		},
	}

	// Parse embedded grammar definition using a separate standard Jsonic instance.
	gs, err := parseGrammarText(grammarText, refs)
	if err != nil {
		return err
	}
	if err := j.Grammar(gs, &jsonic.GrammarSetting{
		Rule: &jsonic.GrammarSettingRule{
			Alt: &jsonic.GrammarSettingAlt{G: "csv"},
		},
	}); err != nil {
		return fmt.Errorf("failed to apply csv grammar: %w", err)
	}

	// Apply per-instance option overrides after Grammar so TokenSet/Fixed
	// tokens survive Grammar's internal SetOptions.
	j.SetOptions(jsonicOptions)

	// Rules list, elem, val are modified in code rather than the grammar file,
	// because in non-strict mode the default jsonic alternatives must be preserved
	// to support embedded JSON values like [1,2] and {x:1}.

	LN := j.Token("#LN")
	CA := j.Token("#CA")
	SP := j.Token("#SP")
	ZZ := j.Token("#ZZ")
	VAL := j.TokenSet("VAL")

	if strict {
		// Strict mode: replace list/elem/val rules entirely.
		// JSON-syntax tokens are disabled, so the only alternates we need
		// are the CSV-specific ones.
		j.Rule("list", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
			rs.Clear()
			rs.AddBO(func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = make([]any, 0)
			})
			rs.AddOpen(
				&jsonic.AltSpec{S: [][]jsonic.Tin{{LN}}, B: 1},
				&jsonic.AltSpec{P: "elem"},
			)
			rs.AddClose(
				&jsonic.AltSpec{S: [][]jsonic.Tin{{LN}}, B: 1, G: "end"},
				&jsonic.AltSpec{S: [][]jsonic.Tin{{ZZ}}, G: "end"},
			)
		})

		j.Rule("elem", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
			rs.Clear()
			rs.AddOpen(
				&jsonic.AltSpec{S: [][]jsonic.Tin{{CA}}, B: 1,
					A: jsonic.AltAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
						if arr, ok := r.Node.([]any); ok {
							r.Node = append(arr, emptyField)
							if r.Parent != jsonic.NoRule && r.Parent != nil {
								r.Parent.Node = r.Node
							}
						}
						r.EnsureU()["done"] = true
					})},
				&jsonic.AltSpec{P: "val"},
			)
			rs.AddClose(
				&jsonic.AltSpec{S: [][]jsonic.Tin{{CA}, {LN, ZZ}}, B: 1, G: "comma",
					A: jsonic.AltAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
						if arr, ok := r.Node.([]any); ok {
							r.Node = append(arr, emptyField)
							if r.Parent != jsonic.NoRule && r.Parent != nil {
								r.Parent.Node = r.Node
							}
						}
					})},
				&jsonic.AltSpec{S: [][]jsonic.Tin{{CA}}, R: "elem", G: "comma"},
				&jsonic.AltSpec{S: [][]jsonic.Tin{{LN}}, B: 1, G: "end"},
				&jsonic.AltSpec{S: [][]jsonic.Tin{{ZZ}}, G: "end"},
			)
			rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
				done, _ := r.U["done"].(bool)
				if !done && !jsonic.IsUndefined(r.Child.Node) {
					if arr, ok := r.Node.([]any); ok {
						r.Node = append(arr, r.Child.Node)
						if r.Parent != jsonic.NoRule && r.Parent != nil {
							r.Parent.Node = r.Node
						}
					}
				}
			})
		})

		j.Rule("val", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
			rs.Clear()
			rs.AddBO(func(r *jsonic.Rule, ctx *jsonic.Context) {
				r.Node = jsonic.Undefined
			})
			rs.AddOpen(
				&jsonic.AltSpec{S: [][]jsonic.Tin{VAL, {SP}}, B: 2, P: "text"},
				&jsonic.AltSpec{S: [][]jsonic.Tin{{SP}}, B: 1, P: "text"},
				&jsonic.AltSpec{S: [][]jsonic.Tin{VAL}},
				&jsonic.AltSpec{S: [][]jsonic.Tin{{LN}}, B: 1},
			)
			rs.AddBC(func(r *jsonic.Rule, ctx *jsonic.Context) {
				if jsonic.IsUndefined(r.Node) {
					if jsonic.IsUndefined(r.Child.Node) {
						if r.OS == 0 {
							r.Node = jsonic.Undefined
						} else {
							r.Node = r.O0.ResolveVal(r, ctx)
						}
					} else {
						r.Node = r.Child.Node
					}
				}
			})
		})
	} else {
		// Non-strict mode: prepend CSV alternates so default JSON-value
		// alternates (handling [1,2], {x:1}, etc.) remain available.
		j.Rule("list", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
			rs.PrependOpen(&jsonic.AltSpec{S: [][]jsonic.Tin{{LN}}, B: 1})
			rs.AddOpen(&jsonic.AltSpec{P: "elem"})
			rs.PrependClose(
				&jsonic.AltSpec{S: [][]jsonic.Tin{{LN}}, B: 1, G: "end"},
				&jsonic.AltSpec{S: [][]jsonic.Tin{{ZZ}}, G: "end"},
			)
		})

		j.Rule("elem", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
			rs.PrependOpen(&jsonic.AltSpec{
				S: [][]jsonic.Tin{{CA}}, B: 1,
				A: jsonic.AltAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
					if arr, ok := r.Node.([]any); ok {
						r.Node = append(arr, emptyField)
						if r.Parent != jsonic.NoRule && r.Parent != nil {
							r.Parent.Node = r.Node
						}
					}
					r.EnsureU()["done"] = true
				}),
			})
			rs.PrependClose(
				&jsonic.AltSpec{S: [][]jsonic.Tin{{CA}, {LN, ZZ}}, B: 1, G: "comma",
					A: jsonic.AltAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
						if arr, ok := r.Node.([]any); ok {
							r.Node = append(arr, emptyField)
							if r.Parent != jsonic.NoRule && r.Parent != nil {
								r.Parent.Node = r.Node
							}
						}
					})},
				&jsonic.AltSpec{S: [][]jsonic.Tin{{LN}}, B: 1, G: "end"},
			)
		})

		j.Rule("val", func(rs *jsonic.RuleSpec, _ *jsonic.Parser) {
			rs.PrependOpen(
				&jsonic.AltSpec{S: [][]jsonic.Tin{VAL, {SP}}, B: 2, P: "text"},
				&jsonic.AltSpec{S: [][]jsonic.Tin{{SP}}, B: 1, P: "text"},
				&jsonic.AltSpec{S: [][]jsonic.Tin{{LN}}, B: 1},
			)
		})
	}

	return nil
}

// BuildCsvStringMatcher is a custom CSV string matcher factory.
// It handles "a""b" -> a"b quoting.
// It mirrors the TS export `buildCsvStringMatcher(options)`, which
// returns make(cfg, opts) => matcher(lex).
func BuildCsvStringMatcher(stringOpts map[string]any) jsonic.MakeLexMatcher {
	quote := toString(stringOpts["quote"])

	// The canonical opens a quoted field with
	// `quoteMap = { [options.string.quote]: true }` and then tests
	// `quoteMap[src[sI]]`, and `src[sI]` is ONE UTF-16 code unit. So the
	// canonical matcher can only ever fire for a quote that IS one code
	// unit, and is inert for every other value of the option: the empty
	// string, a multi-character quote such as `""`, and an astral
	// character such as U+1F600, which JavaScript holds as a surrogate
	// PAIR. `strings.HasPrefix` agreed with none of those. It matched a
	// multi-character quote the canonical cannot match, matched an astral
	// quote the same way, and -- worst -- returned true at every position
	// for the EMPTY quote, because every string has the empty prefix, so
	// the matcher consumed nothing, the lexer made no progress, and
	// `string.quote: ""` HUNG the parse instead of returning.
	//
	// Measured against the canonical on `a,b\n"x y",z` and friends: the
	// one-character quote `|` matches in both, while the two-character
	// quote `""`, the two-character quote `ab`, the astral U+1F600 and
	// the empty quote are inert in both once this guard is here.
	quoteRune, quoteSize := utf8.DecodeRuneInString(quote)
	singleCodeUnit := 0 < quoteSize && quoteSize == len(quote) && quoteRune <= 0xFFFF

	return func(cfg *jsonic.LexConfig, opts *jsonic.Options) jsonic.LexMatcher {
		return func(lex *jsonic.Lex, rule *jsonic.Rule) *jsonic.Token {
			pnt := lex.Cursor()
			src := lex.Src
			sI := pnt.SI
			srclen := len(src)

			if !singleCodeUnit {
				return nil
			}

			if sI >= srclen || !strings.HasPrefix(src[sI:], quote) {
				return nil
			}

			// Only match when quote is at the start of a field.
			//
			// What precedes the quote must be tested as TEXT, not as a single
			// byte: a fixed token can be several characters long (a multi-char
			// `field.separation`) and a single character can be several bytes
			// long (`field.separation: "€"`). The old test read src[sI-1] and
			// widened that one byte to a rune, so after a multi-byte separator
			// it compared the separator's LAST BYTE (0xAC for €) against the
			// token table, never matched, and left the quoted field as literal
			// text — the go/encoding/csv NonASCIICommaAndCommentWithQuotes
			// case, where TypeScript (canonical) gets it right.
			if sI > 0 {
				fieldStart := false
				for tokenSrc := range cfg.FixedTokens {
					if 0 < len(tokenSrc) && len(tokenSrc) <= sI &&
						src[sI-len(tokenSrc):sI] == tokenSrc {
						fieldStart = true
						break
					}
				}
				if !fieldStart {
					prev, _ := utf8.DecodeLastRuneInString(src[:sI])
					fieldStart = cfg.LineChars[prev] || cfg.SpaceChars[prev]
				}
				if !fieldStart {
					return nil
				}
			}

			q := quote
			qLen := len(q)
			rI := pnt.RI
			cI := pnt.CI
			sI += qLen
			cI += qLen

			var s strings.Builder
			for sI < srclen {
				cI++
				if strings.HasPrefix(src[sI:], q) {
					sI += qLen
					cI += qLen - 1
					if sI < srclen && strings.HasPrefix(src[sI:], q) {
						s.WriteString(q)
						sI += qLen
						cI += qLen
						continue
					}
					val := s.String()
					ssrc := src[pnt.SI:sI]
					tkn := lex.Token("#ST", jsonic.TinST, val, ssrc)
					pnt.SI = sI
					pnt.RI = rI
					pnt.CI = cI
					return tkn
				}

				ch := src[sI]
				if cfg.LineChars[rune(ch)] {
					if cfg.RowChars[rune(ch)] {
						rI++
						pnt.RI = rI
					}
					cI = 1
					s.WriteByte(ch)
					sI++
					continue
				}
				if ch < 32 {
					return nil
				}

				bI := sI
				qFirst := q[0]
				for sI < srclen && src[sI] >= 32 && src[sI] != qFirst {
					if cfg.LineChars[rune(src[sI])] {
						break
					}
					sI++
					cI++
				}
				cI--
				s.WriteString(src[bI:sI])
			}

			badSrc := src[pnt.SI:sI]
			tkn := lex.Token("#BD", jsonic.TinBD, nil, badSrc)
			tkn.Why = "unterminated_string"
			pnt.SI = sI
			pnt.RI = rI
			pnt.CI = cI
			return tkn
		}
	}
}

// Defaults matches the TS Csv.defaults. Used with jsonic.UseDefaults.
var Defaults = map[string]any{
	"trim":    nil,
	"comment": nil,
	"number":  nil,
	"value":   nil,
	"header":  true,
	"object":  true,
	"stream":  nil,
	"strict":  true,
	"field": map[string]any{
		"separation":   nil,
		"nonameprefix": "field~",
		"empty":        "",
		"names":        nil,
		"exact":        false,
	},
	"record": map[string]any{
		"separators": nil,
		"empty":      false,
	},
	"string": map[string]any{
		"quote": `"`,
		"csv":   nil,
	},
}

// parseGrammarText parses grammar text and builds a GrammarSpec with Ref support.
func parseGrammarText(text string, refs map[jsonic.FuncRef]any) (*jsonic.GrammarSpec, error) {
	parsed, err := jsonic.Make().Parse(text)
	if err != nil {
		return nil, fmt.Errorf("failed to parse grammar text: %w", err)
	}
	parsedMap, ok := asStringMap(parsed)
	if !ok {
		return nil, fmt.Errorf("grammar text did not parse to a map")
	}
	gs := &jsonic.GrammarSpec{Ref: refs}
	if optionsMap, ok := asStringMap(parsedMap["options"]); ok {
		// The jsonic engine's MapToOptions / ResolveFuncRefs consume
		// OptionsMap and its nested objects as plain map[string]any, so
		// deeply convert any nested *OrderedMap before handing it over.
		gs.OptionsMap = toPlainMap(optionsMap)
	}
	ruleMap, ok := asStringMap(parsedMap["rule"])
	if !ok {
		return gs, nil
	}
	gs.Rule = make(map[string]*jsonic.GrammarRuleSpec, len(ruleMap))
	for name, rDef := range ruleMap {
		rd, ok := asStringMap(rDef)
		if !ok {
			continue
		}
		grs := &jsonic.GrammarRuleSpec{}
		if openDef, ok := rd["open"]; ok {
			grs.Open = buildGrammarAlts(openDef)
		}
		if closeDef, ok := rd["close"]; ok {
			grs.Close = buildGrammarAlts(closeDef)
		}
		gs.Rule[name] = grs
	}
	return gs, nil
}

func buildGrammarAlts(def any) []*jsonic.GrammarAltSpec {
	arr, ok := def.([]any)
	if !ok {
		return nil
	}
	alts := make([]*jsonic.GrammarAltSpec, 0, len(arr))
	for _, item := range arr {
		m, ok := asStringMap(item)
		if !ok {
			alts = append(alts, &jsonic.GrammarAltSpec{})
			continue
		}
		ga := &jsonic.GrammarAltSpec{}
		if s, ok := m["s"]; ok {
			switch sv := s.(type) {
			case string:
				ga.S = sv
			case []any:
				strs := make([]string, len(sv))
				for i, v := range sv {
					strs[i], _ = v.(string)
				}
				ga.S = strs
			}
		}
		if b, ok := m["b"]; ok {
			switch bv := b.(type) {
			case float64:
				ga.B = int(bv)
			case int:
				ga.B = bv
			}
		}
		if p, ok := m["p"].(string); ok {
			ga.P = p
		}
		if r, ok := m["r"].(string); ok {
			ga.R = r
		}
		if a, ok := m["a"].(string); ok {
			ga.A = jsonic.FuncRef(a)
		}
		if c, ok := m["c"]; ok {
			switch cv := c.(type) {
			case string:
				ga.C = cv
			default:
				if cm, ok := asStringMap(cv); ok {
					ga.C = toPlainMap(cm)
				}
			}
		}
		if n, ok := asStringMap(m["n"]); ok {
			ga.N = make(map[string]int, len(n))
			for k, v := range n {
				if nv, ok := v.(float64); ok {
					ga.N[k] = int(nv)
				} else if nv, ok := v.(int); ok {
					ga.N[k] = nv
				}
			}
		}
		if g, ok := m["g"].(string); ok {
			ga.G = g
		}
		alts = append(alts, ga)
	}
	return alts
}

func tokenStr(t *jsonic.Token) string {
	if t == nil || t.IsNoToken() {
		return ""
	}
	if t.Tin == jsonic.TinST {
		if s, ok := t.Val.(string); ok {
			return s
		}
	}
	return t.Src
}

// asStringMap returns the underlying key→value map for a parsed object,
// which may be a *jsonic.OrderedMap (the insertion-ordered parse result) or
// a plain map[string]any. Grammar consumers look values up by name and do
// not depend on key order, so exposing the underlying map is sufficient.
func asStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case *jsonic.OrderedMap:
		return m.Vals, true
	case jsonic.OrderedMap:
		return m.Vals, true
	case map[string]any:
		return m, true
	}
	return nil, false
}

// toPlain recursively converts any *jsonic.OrderedMap nodes in a parsed
// value tree into plain map[string]any (dropping key order). The jsonic
// engine's grammar-consuming helpers (MapToOptions, ResolveFuncRefs) only
// recurse through plain maps and slices, so grammar config must be plainified
// before it is handed back to the engine.
func toPlain(v any) any {
	switch val := v.(type) {
	case *jsonic.OrderedMap:
		out := make(map[string]any, len(val.Keys))
		for _, k := range val.Keys {
			out[k] = toPlain(val.Vals[k])
		}
		return out
	case jsonic.OrderedMap:
		out := make(map[string]any, len(val.Keys))
		for _, k := range val.Keys {
			out[k] = toPlain(val.Vals[k])
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = toPlain(item)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = toPlain(item)
		}
		return out
	default:
		return val
	}
}

// toPlainMap deep-converts a map that may hold nested *jsonic.OrderedMap
// values into a fully-plain map[string]any tree.
func toPlainMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = toPlain(v)
	}
	return out
}

// optionObjects is the set of OBJECTS an option supplied, held by
// identity. It is the provenance record tagOptions builds and jsKey
// consults, and it exists because the Go TYPE of an object does not say
// where the object came from: the lexer allocates a parsed object as
// *jsonic.OrderedMap, but OrderedMap is a PUBLIC type with a public
// constructor, so `field.empty: jsonic.NewOrderedMap()` is an OPTION
// value of exactly the type the lexer builds. An earlier round asserted
// that `%T` proved provenance and DIVERGENCE.md said so; measured on
// 2026-09-21, that was wrong, and the option object was refused where
// the canonical names the column "[object Object]".
//
// A Go map needs no entry here, because the lexer has no way to build
// one: there the type IS the provenance.
type optionObjects map[*jsonic.OrderedMap]struct{}

// optionRef identifies a container by the allocation it refers to, so a
// value that contains ITSELF is recognised rather than walked forever.
// The length is part of the key because two slices can share one data
// pointer: `a[:1]` and `a[:2]` are different nodes, and only a walk that
// re-enters the SAME header has found a cycle.
type optionRef struct {
	kind reflect.Kind
	ptr  uintptr
	len  int
}

func optionRefOf(rv reflect.Value) (optionRef, bool) {
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer:
		if rv.IsNil() {
			return optionRef{}, false
		}
		ref := optionRef{kind: rv.Kind(), ptr: rv.Pointer()}
		if rv.Kind() == reflect.Slice {
			ref.len = rv.Len()
		}
		return ref, true
	}
	return optionRef{}, false
}

// ErrCyclicOption refuses a configuration no runtime can name a column
// from. MEASURED on 2026-09-21: the canonical throws
// `RangeError: Maximum call stack size exceeded` out of
// `.use(Csv, opts)` for EVERY self-referential option value -- through an
// array, through an object, at depth 1 and nested deeper -- because the
// engine deep-copies the option bag before the plugin ever sees it. It
// never reaches `Array.prototype.join`, whose own cycle guard would have
// rendered the recursive occurrence as an empty segment, so there is no
// canonical column name for a cycle to be given.
//
// Go's engine copies the bag the same way and dies at the same site, but
// a Go stack overflow is FATAL and uncatchable, so it takes the process
// with it. That is a defect of `deepClone` in `tabnas/parser/go`, which
// has no cycle guard, and it is reachable only for the containers that
// function recurses into: []any, map[string]any and *OrderedMap. A cycle
// sheltered under any OTHER Go container -- a declared slice type, a
// declared map type, a fixed-size array -- is passed through untouched
// and arrives here intact. This is where it stops: the plugin refuses the
// configuration from the same call the canonical throws out of, rather
// than aborting the process later, in the middle of naming a column.
var ErrCyclicOption = fmt.Errorf(
	"csv: an option value contains itself, so no column name can be taken from it")

// tagOptions walks the option bag once, at the point the options are
// read, and returns the identity of every object in it.
//
// It rewrites NOTHING. A value a caller supplied for `field.empty` is
// dropped into an empty cell and reaches the caller again in the record,
// so this port hands it back exactly as it was given, down to its Go
// type. What the walk establishes is the two things a later site cannot
// work out for itself:
//
//   - PROVENANCE. An object is named "[object Object]" when an option
//     supplied it and refuses the document when the lexer built it, and
//     both are *jsonic.OrderedMap. See optionObjects.
//   - TERMINATION. A self-referential option value is refused here
//     rather than walked forever at the name site. See ErrCyclicOption.
//
// Containers are walked; everything else is left alone. That is enough,
// because an object or an array is the only thing either question can be
// asked about.
func tagOptions(options map[string]any) (optionObjects, error) {
	objects := optionObjects{}
	if err := tagOptionValue(options, objects, map[optionRef]struct{}{}); err != nil {
		return nil, err
	}
	return objects, nil
}

func tagOptionValue(val any, objects optionObjects, path map[optionRef]struct{}) error {
	if val == nil {
		return nil
	}
	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Pointer:
	default:
		// A string, a number, a bool, a function, a channel, a struct:
		// nothing jsKey looks inside, so nothing a name can be taken
		// from, and nothing that can hold a reference back to itself
		// without one of the kinds above on the way. The `OrderedMap`
		// VALUE form is a struct, and jsKey names it "[object Object]"
		// without reading a key of it.
		return nil
	}

	if ref, tracked := optionRefOf(rv); tracked {
		if _, cycling := path[ref]; cycling {
			return ErrCyclicOption
		}
		path[ref] = struct{}{}
		defer delete(path, ref)
	}

	if om, isObject := asOrderedMap(val); isObject {
		objects[om] = struct{}{}
		for _, k := range om.Keys {
			if err := tagOptionValue(om.Vals[k], objects, path); err != nil {
				return err
			}
		}
		return nil
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if err := tagOptionValue(rv.Index(i).Interface(), objects, path); err != nil {
				return err
			}
		}
	case reflect.Map:
		iter := rv.MapRange()
		for iter.Next() {
			if err := tagOptionValue(iter.Value().Interface(), objects, path); err != nil {
				return err
			}
		}
	}
	// A pointer to anything but an OrderedMap has no JavaScript spelling:
	// jsKey refuses it without looking inside, so there is nothing here
	// to walk either.
	return nil
}

// asOrderedMap is the POINTER form alone, which is the only object form
// whose provenance is in doubt: the lexer allocates one, and so can a
// caller through the public jsonic.NewOrderedMap. It is what an entry in
// optionObjects is keyed by.
//
// The VALUE form, `jsonic.OrderedMap{...}`, is deliberately not here.
// Taking its address copies it to the heap, so it would be a different
// object every time it was asked for and a tag on it would not carry;
// and it needs no tag, because the lexer cannot produce one. jsKey
// answers that form by its type.
func asOrderedMap(val any) (*jsonic.OrderedMap, bool) {
	if m, ok := val.(*jsonic.OrderedMap); ok && m != nil {
		return m, true
	}
	return nil, false
}

// asOptionMap reads one of the nested option maps -- `field`, `record`,
// `string` -- whatever a caller spelled it as. A type assertion on
// `map[string]any` alone dropped a declared map type
// (`type Field map[string]any`), and with it EVERY option inside it,
// silently: `field: Field{"nonameprefix": "F"}` named the columns "0"
// and "1".
//
// The returned map is only ever read from, so a converted copy is as good
// as the original. A nil result reads as an absent option throughout,
// which is what an absent map should do.
func asOptionMap(val any) map[string]any {
	switch m := val.(type) {
	case nil:
		return nil
	case map[string]any:
		return m
	case *jsonic.OrderedMap:
		if m == nil {
			return nil
		}
		return m.Vals
	case jsonic.OrderedMap:
		return m.Vals
	}
	rv := reflect.ValueOf(val)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil
	}
	out := make(map[string]any, rv.Len())
	iter := rv.MapRange()
	for iter.Next() {
		out[iter.Key().String()] = iter.Value().Interface()
	}
	return out
}

// isTrue and isFalse are JavaScript's `===` against a boolean literal,
// which is how the canonical tests `string.csv`:
// `false !== options.string.csv` turns the CSV string matcher on in
// strict mode, and `true === options.string.csv` turns it on outside
// strict mode. Nothing but a boolean satisfies either, so `csv: 0` is
// NOT `false` and `csv: 1` is NOT `true`, which is what the comparisons
// below say. A declared bool type is the same boolean to JavaScript, so
// the kind decides rather than the type.
func isTrue(v any) bool {
	b, ok := jsBool(v)
	return ok && b
}

func isFalse(v any) bool {
	b, ok := jsBool(v)
	return ok && !b
}

func jsBool(v any) (bool, bool) {
	if b, ok := v.(bool); ok {
		return b, true
	}
	rv := reflect.ValueOf(v)
	if rv.IsValid() && rv.Kind() == reflect.Bool {
		return rv.Bool(), true
	}
	return false, false
}

// toBool is JavaScript's `!!x`, which is how the canonical reads every
// boolean option it has: `const strict = !!options.strict`,
// `!!options.record?.empty`, and `options.field.exact && ...`. Falsy is
// undefined, null, false, zero, NaN and the empty string; everything
// else -- an empty array, an empty object, the string "false" -- is true.
//
// A type assertion on `bool` read all of those as false, so a caller who
// wrote `field.exact: 1` got a documented option that silently did
// nothing where the canonical raises csv_extra_field, and one who wrote a
// declared bool type (`type Flag bool`) got `strict: Flag(true)` read as
// NON-strict, which changes how every field in the document is lexed.
func toBool(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := jsBool(v); ok {
		return b
	}
	if f, ok := jsNumber(v); ok {
		return f != 0 && !math.IsNaN(f)
	}
	if s, ok := jsString(v); ok {
		return s != ""
	}
	return true
}

// toString reads a STRING option: `field.nonameprefix`,
// `field.separation`, `record.separators` and `string.quote`. A declared
// string type (`type Sep string`) is the same string to JavaScript, so
// the kind decides rather than the type; an exact-type assertion read
// `field.separation: Sep(";")` as ABSENT and left the whole line as one
// field.
//
// A value of some OTHER kind is not coerced here. The canonical builds a
// no-name column with JavaScript's `+`, which does numeric addition for a
// number and string concatenation otherwise, and this port does not
// implement that operator; AGENTS.md records what that leaves open.
func toString(v any) string {
	s, _ := jsString(v)
	return s
}

func jsString(v any) (string, bool) {
	if s, ok := v.(string); ok {
		return s, true
	}
	rv := reflect.ValueOf(v)
	if rv.IsValid() && rv.Kind() == reflect.String {
		return rv.String(), true
	}
	return "", false
}

// toStream reads the `stream` callback. A DECLARED function type
// (`type Sink func(string, any)`) has the same signature and a different
// dynamic type, and a type assertion dropped it, which turned streaming
// silently off: the parse then built and returned the records the caller
// asked to have streamed, and the callback never fired.
func toStream(v any) func(string, any) {
	if s, ok := v.(func(string, any)); ok {
		return s
	}
	rv := reflect.ValueOf(v)
	want := reflect.TypeOf((func(string, any))(nil))
	if rv.IsValid() && rv.Kind() == reflect.Func && rv.Type().ConvertibleTo(want) {
		s, _ := rv.Convert(want).Interface().(func(string, any))
		return s
	}
	return nil
}

func boolPtr(b bool) *bool {
	return &b
}

// jsKey renders a value as JavaScript renders it when it is used as an
// object key: `obj[v] = ...` applies ToPropertyKey, which for anything
// but a symbol is ToString. The vocabulary a parsed CSV field can hold is
// spelled out, and so is the wider one an OPTION value can hold: a field
// list is `ctx.u.fields` or `field.names`, and `field.empty` is dropped
// into a syntactically empty cell, so `,a` under
// `{field: {empty: 42}}` names a column with a value that never went
// through the lexer.
//
// ok is false when JavaScript cannot make a primitive of the value at
// all, which is a PARSED object: a jsonic object is allocated with a
// null prototype, so it inherits neither toString nor Symbol.toPrimitive
// and the canonical runtime throws
// `TypeError: Cannot convert object to primitive value` instead of
// naming the column. This port cannot raise a JavaScript TypeError, so
// it refuses the document instead; see DIVERGENCE.md.
//
// An object supplied as an OPTION value is the OTHER case, and the
// canonical answers it differently: the option merge rebuilds a plain
// source object onto Object.prototype on the way into the bag, so String
// names the column "[object Object]" there and nothing throws, even for
// one the caller made with Object.create(null).
//
// This function is TOLD which is which. It does not read the provenance
// off the concrete type, which cannot carry it: a Go map is never a
// parsed cell, but an *OrderedMap can be either, because OrderedMap is
// public and `jsonic.NewOrderedMap()` is a value a caller can hand to
// `field.empty`. normalizeOptions walks the option bag at the point the
// options are READ and records every object it finds there, and that set
// is what decides the two routes apart here. Two earlier rounds got this
// wrong in opposite directions: the first recorded the option route as
// unrefusable in DIVERGENCE.md, and the second refuted it with `%T`,
// which only looked right because no test handed the option route a
// type the lexer also builds.
//
// The last-resort `fmt.Sprintf("%v", v)` this used to end with is what
// made that necessary: it put the engine's internal struct into a column
// name (`{x:1}` became `&{[x] map[x:1] false}`) and an array into Go's
// own bracket form (`[1,2]` became `[1 2]`), neither of which the
// canonical runtime can produce for any input.
func jsKey(val any, objects optionObjects) (key string, ok bool) {
	switch v := val.(type) {
	case string:
		return v, true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	case nil:
		return "null", true
	case []any:
		return jsArrayKey(v, objects)
	case *jsonic.OrderedMap:
		// An object, and which route it came by is NOT written on it:
		// OrderedMap is public and `jsonic.NewOrderedMap()` is a value a
		// caller can put in `field.empty`. So ask the record tagOptions
		// built where the options were read. An object IN it came from an
		// option, and the canonical's option merge rebuilds a plain
		// source object onto Object.prototype, so String names the column
		// "[object Object]" and nothing throws. An object that is NOT in
		// it is one the lexer allocated, with a null prototype and no
		// toString, and the canonical throws rather than naming it, so
		// this port refuses the document.
		if v != nil {
			if _, fromOption := objects[v]; fromOption {
				return "[object Object]", true
			}
		}
		return "", false
	case jsonic.OrderedMap:
		// The VALUE form. The lexer allocates objects and hands out
		// pointers, so only a caller can write this, and only into an
		// option. Named as the option route is named. Listed here, ahead
		// of the reflect fallbacks, because an OrderedMap is a struct and
		// not a Go map, so nothing below would claim it.
		return "[object Object]", true
	default:
		if f, isNumber := jsNumber(val); isNumber {
			return jsNumberToString(f), true
		}
		// A DECLARED type over a primitive keeps its kind and has its own
		// dynamic type, so the exact-type cases above miss it:
		// `type Column string` is not `string` to a type switch. It IS
		// the same string to JavaScript, which has no such distinction,
		// so `field.names: []any{Column("x")}` names the column "x"
		// there, where this port refused the whole document as
		// `unexpected`. Read after the fast paths, so the shapes the
		// lexer builds never pay for reflection.
		if s, isString := jsString(val); isString {
			return s, true
		}
		if b, isBool := jsBool(val); isBool {
			if b {
				return "true", true
			}
			return "false", true
		}
		// A slice of any element type, not just []any. `field.empty` and
		// `field.names` are unconstrained Go options, so a caller writes
		// the array Go makes easiest: []string{"a", "b"}, []int{1, 2},
		// [][]string{...}. The lexer only ever builds []any, so the shared
		// fixtures reach this site with []any alone and a type assertion
		// on that one shape passed them while refusing every native
		// spelling. JavaScript has one array type, and
		// Array.prototype.toString joins whatever is in it, so each of
		// these is the same array to the canonical runtime.
		if items, isSlice := asSlice(val); isSlice {
			return jsArrayKey(items, objects)
		}
		// A Go MAP is an option value, and here the TYPE does settle it:
		// the lexer has no way to build one, so no parsed cell can be a
		// Go map. (An OrderedMap is not settled by its type, which is why
		// it is answered from the option record above.) The canonical's
		// option merge rebuilds a plain source object onto
		// Object.prototype on the way into the bag, so String gives it
		// "[object Object]" and nothing throws. Reached through reflection
		// rather than a `case map[string]any` so that every map spelling
		// a Go caller reaches for -- a declared map type, a
		// map[string]string, a map keyed by something JavaScript has no
		// spelling for at all -- is named the same way.
		if reflect.ValueOf(val).Kind() == reflect.Map {
			return "[object Object]", true
		}
		return "", false
	}
}

// asSlice widens any Go slice or array to the []any the name-building
// code works in. It reports false for everything else, INCLUDING a
// string, whose Kind is neither Slice nor Array, so a string never
// becomes a one-element list.
//
// The returned slice is non-nil whenever ok is true, the empty input
// included. That matters for `field.names`: TS treats `[]` as a field
// list, because every array is truthy there, and a nil slice here would
// be read as no list at all.
//
// It always COPIES, a []any input included, so that a field list read
// out of the option bag at install time cannot be changed afterwards by
// a caller who still holds the slice they passed. jsKey keeps its own
// []any case ahead of this one, so the hot path of naming a column from
// a parsed array does not pay for the copy.
func asSlice(val any) ([]any, bool) {
	if val == nil {
		return nil, false
	}
	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
	default:
		return nil, false
	}
	items := make([]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		items = append(items, rv.Index(i).Interface())
	}
	return items, true
}

// jsNumber widens Go's numeric spellings to the float64 the lexer always
// produces. The lexer only ever makes a float64, but an OPTION value is
// whatever the caller wrote: `field.empty: 42` is an int in a Go map
// literal, an int64 or a json.Number when the options were decoded, and a
// float32 when they came from a narrower field. JavaScript has one number
// type, so every one of these is the same double to the canonical
// runtime, which names the column "42" where this port used to refuse the
// document outright.
//
// A value too large for a float64's mantissa loses the same digits the
// canonical runtime loses, because a JavaScript number IS a double.
//
// A DECLARED type over a numeric kind (`type Count int`) is read through
// reflection after the exact types, for the reason spelled out there.
func jsNumber(val any) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	}
	// A DECLARED type over a numeric kind: `type Count int`, or the
	// `type Ratio float64` a caller reaches for to keep a unit straight.
	// It keeps the kind and has its own dynamic type, so the cases above
	// miss it, and `field.empty: Count(42)` was refused where the
	// canonical names the column "42". Read after them, so the float64
	// the lexer builds never pays for reflection.
	rv := reflect.ValueOf(val)
	if !rv.IsValid() {
		return 0, false
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return rv.Float(), true
	}
	// Uintptr, the two complex kinds and everything else have no
	// JavaScript spelling, and are left to be refused rather than given
	// a number's name.
	return 0, false
}

// jsArrayKey is Array.prototype.toString, which is join(',') with no
// separator argument (ECMA-262 23.1.3.17 and 23.1.3.34): every element
// is converted by the same rules, null and undefined become the empty
// string, and a nested array joins recursively, so `[1,[2,3]]` flattens
// to "1,2,3" and `[]` is "".
//
// Note that a null ELEMENT is "" while a null CELL is "null": the empty
// string comes from join, not from ToString, so it applies only inside
// an array.
//
// The recursion needs no seen-set HERE, because neither of the two
// things it can be handed contains itself. A parsed value is a TREE: the
// engine folds each finished rule's value into its parent and never
// stores a reference to an ancestor, so no element can reach its own
// array. An OPTION value can be spelled as one -- `a := make([]any, 1);
// a[0] = a` -- and normalizeOptions refuses that at the point the options
// are read, which is the call the canonical throws a RangeError out of.
// Its depth is the document's bracket nesting, which the caller's stack
// has already carried once while the engine built the value and carries
// again whenever the value is marshalled.
func jsArrayKey(items []any, objects optionObjects) (string, bool) {
	var joined strings.Builder
	for i, item := range items {
		if 0 < i {
			joined.WriteByte(',')
		}
		if item == nil || jsonic.IsUndefined(item) {
			continue
		}
		part, ok := jsKey(item, objects)
		if !ok {
			return "", false
		}
		joined.WriteString(part)
	}
	return joined.String(), true
}

// jsNumberToString is ECMAScript `Number::toString` with radix 10
// (ECMA-262 6.1.6.1.20), which is what `String(n)` gives and therefore
// what a numeric header cell is named in the canonical runtime.
//
// strconv.FormatFloat(v, 'f', -1, 64) is NOT a substitute. It has no
// switch to exponent form, so 1e21 comes out as twenty-two digits where
// JavaScript writes "1e+21", and 1e-7 as a string of zeros where
// JavaScript writes "1e-7".
//
// The digits come from a FIXED-precision render rather than the shortest
// one. Both round-trip, but they break an exact decimal midpoint
// differently: the shortest form rounds away from zero, while the
// specification takes the even digit, which is what a fixed-precision
// render does. The same defect has been found in six Rust crates of this
// fleet; ts/src/csv.ts needs no such code because the language does it.
func jsNumberToString(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "Infinity"
	}
	if math.IsInf(f, -1) {
		return "-Infinity"
	}
	// Covers -0, which JavaScript prints as "0".
	if f == 0 {
		return "0"
	}

	magnitude := math.Abs(f)

	// The specification's `s` (the digits) and `n` (where the decimal
	// point sits). Take the digit count from the shortest form, then take
	// the digits themselves at that fixed precision.
	shortest := strconv.FormatFloat(magnitude, 'e', -1, 64)
	mantissa, _, _ := strings.Cut(shortest, "e")
	k := len(strings.Replace(mantissa, ".", "", 1))

	fixed := strconv.FormatFloat(magnitude, 'e', k-1, 64)
	mantissa, exponentText, _ := strings.Cut(fixed, "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	exponent, err := strconv.Atoi(exponentText)
	if err != nil {
		// FormatFloat with 'e' always emits a signed integer exponent.
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	n := exponent + 1

	var body string
	switch {
	case k <= n && n <= 21:
		// 12 -> "12", 1e19 -> "10000000000000000000"
		body = digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		// 1.5 -> "1.5"
		body = digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		// 1e-6 -> "0.000001"
		body = "0." + strings.Repeat("0", -n) + digits
	default:
		// 1e21 -> "1e+21", 1e-7 -> "1e-7"
		e := n - 1
		head := digits
		if k > 1 {
			head = digits[:1] + "." + digits[1:]
		}
		sign := "+"
		if e < 0 {
			sign = "-"
			e = -e
		}
		body = head + "e" + sign + strconv.Itoa(e)
	}

	if f < 0 {
		return "-" + body
	}
	return body
}
