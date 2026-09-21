/* Copyright (c) 2021-2025 Richard Rodger, MIT License */

package tabnascsv

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	jsonic "github.com/tabnas/jsonic/go"
)

// A header cell is not always a string. In non-strict mode the field
// body is parsed, so `1,2,3` arrives as three float64s; a dropped type
// assertion named all three "" and they then collapsed onto one key,
// returning {"":6} and losing two columns. Every expectation here is
// what the canonical runtime prints for the same source.
func TestHeaderCellsAreNamedAsJavaScriptNamesThem(t *testing.T) {
	cases := []struct {
		src      string
		expected string
	}{
		{"1,2,3\n4,5,6\n", `[{"1":4,"2":5,"3":6}]`},
		{"a,2,c\nx,y,z\n", `[{"2":"y","a":"x","c":"z"}]`},
		{"true,null,1.5\np,q,r\n", `[{"1.5":"r","null":"q","true":"p"}]`},
	}
	for _, c := range cases {
		j := jsonic.Make()
		j.UseDefaults(Csv, Defaults, map[string]any{"strict": false})
		result, err := j.Parse(c.src)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		// Marshal sorts an object's keys, so this compares the SET of
		// names and their values, which is what the defect destroyed.
		got, _ := json.Marshal(result)
		if string(got) != c.expected {
			t.Errorf("%q\n got %s\nwant %s", c.src, got, c.expected)
		}
	}
}

// jsNumberToString is ECMAScript Number::toString with radix 10. The
// cases below are the ones no CSV fixture row can reach: the switch to
// exponent form at either end, and an exact decimal midpoint, where the
// shortest round-tripping form rounds away from zero and the
// specification takes the even digit. Each expectation is `String(n)`
// under Node.
func TestJsNumberToString(t *testing.T) {
	cases := []struct {
		value    float64
		expected string
	}{
		{0, "0"},
		{math.Copysign(0, -1), "0"},
		{1, "1"},
		{-1, "-1"},
		{1.5, "1.5"},
		{100, "100"},
		{1e19, "10000000000000000000"},
		{1e20, "100000000000000000000"},
		{1e21, "1e+21"},
		{1e-6, "0.000001"},
		{1e-7, "1e-7"},
		{9007199254740991, "9007199254740991"},
		{137839762462415.62, "137839762462415.62"},
		{math.MaxFloat64, "1.7976931348623157e+308"},
		{math.SmallestNonzeroFloat64, "5e-324"},
		{math.NaN(), "NaN"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
	}
	for _, c := range cases {
		if got := jsNumberToString(c.value); got != c.expected {
			t.Errorf("jsNumberToString(%v) = %q, want %q", c.value, got, c.expected)
		}
	}
}

// The rest of the vocabulary a parsed field can hold. The array rows are
// Array.prototype.toString, which is join(',') with no separator
// argument: null and undefined become the empty string, and a nested
// array joins recursively. Every expectation is what `String(v)` answers
// in the canonical runtime.
func TestJsKey(t *testing.T) {
	cases := []struct {
		value    any
		expected string
	}{
		{"a", "a"},
		{"", ""},
		{float64(2), "2"},
		{float64(1.5), "1.5"},
		{true, "true"},
		{false, "false"},
		{nil, "null"},
		{[]any{}, ""},
		{[]any{float64(1), float64(2)}, "1,2"},
		{[]any{float64(1), []any{float64(2), float64(3)}}, "1,2,3"},
		{[]any{[]any{}}, ""},
		{[]any{[]any{}, []any{}}, ","},
		{[]any{nil, float64(1)}, ",1"},
		{[]any{nil}, ""},
		{[]any{jsonic.Undefined, float64(1)}, ",1"},
		{[]any{"a", "b"}, "a,b"},
		{[]any{"a,b", "c"}, "a,b,c"},
		{[]any{true, false}, "true,false"},
		{[]any{math.Copysign(0, -1), float64(1)}, "0,1"},
		{[]any{1e21, 1e-7}, "1e+21,1e-7"},
		// The wider vocabulary an OPTION value can arrive in. The lexer
		// only ever makes a float64, but `field.empty` is whatever the
		// caller wrote, and JavaScript has one number type, so every one
		// of these is the same double to the canonical runtime.
		{42, "42"},
		{int8(-8), "-8"},
		{int16(-16), "-16"},
		{int32(-32), "-32"},
		{int64(7), "7"},
		{uint(1), "1"},
		{uint8(3), "3"},
		{uint16(16), "16"},
		{uint32(32), "32"},
		{uint64(1 << 53), "9007199254740992"},
		{float32(1.5), "1.5"},
		{json.Number("2.5"), "2.5"},
		{[]any{1, int64(2)}, "1,2"},
	}
	for _, c := range cases {
		got, ok := jsKey(c.value)
		if !ok {
			t.Errorf("jsKey(%#v) refused the value, want %q", c.value, c.expected)
			continue
		}
		if got != c.expected {
			t.Errorf("jsKey(%#v) = %q, want %q", c.value, got, c.expected)
		}
	}
}

// A PARSED object has no ToString to apply, at the top of a cell or
// anywhere inside one, so jsKey refuses it rather than inventing a name.
// Before this, the last-resort `fmt.Sprintf("%v", v)` put the engine's
// internal struct into a column name: `&{[x] map[x:1] false}`.
//
// The refusal is keyed on the TYPE, which is what carries provenance
// here: the lexer allocates a parsed object as *jsonic.OrderedMap, and
// only that is refused.
func TestJsKeyRefusesAParsedObject(t *testing.T) {
	object := jsonic.NewOrderedMap()
	object.Set("x", float64(1))
	for _, value := range []any{
		object,
		[]any{object},
		[]any{float64(1), object},
		[]any{[]any{object}},
	} {
		if got, ok := jsKey(value); ok {
			t.Errorf("jsKey(%#v) = %q, want a refusal", value, got)
		}
	}
}

// A plain Go MAP is the other route to the same site, and it is never a
// parsed cell: it is an option value the caller wrote. The canonical's
// option merge rebuilds it onto Object.prototype, so `String` names the
// column "[object Object]" and nothing throws. jsKey answers that, for
// every map spelling, and inside an array too, where join converts each
// element by the same rules.
func TestJsKeyNamesAnOptionObject(t *testing.T) {
	for _, c := range []struct {
		value    any
		expected string
	}{
		{map[string]any{"x": float64(1)}, "[object Object]"},
		{map[string]any{}, "[object Object]"},
		{map[string]int{"x": 1}, "[object Object]"},
		{map[string]string{"x": "y"}, "[object Object]"},
		{[]any{map[string]any{"x": float64(1)}}, "[object Object]"},
		{[]any{"a", map[string]any{}}, "a,[object Object]"},
	} {
		got, ok := jsKey(c.value)
		if !ok {
			t.Errorf("jsKey(%#v) refused the value, want %q", c.value, c.expected)
			continue
		}
		if got != c.expected {
			t.Errorf("jsKey(%#v) = %q, want %q", c.value, got, c.expected)
		}
	}
}

// DIVERGENCE (see ../DIVERGENCE.md): an OBJECT header cell.
//
// The canonical TypeScript throws a raw JavaScript
// `TypeError: Cannot convert object to primitive value`, because a jsonic
// object is allocated with a null prototype and so has no `toString`. Go
// cannot raise a JavaScript TypeError, so it refuses the document with
// the engine's inherited `unexpected` code instead of inventing a column
// name the canonical never produces.
//
// Asserted in both directions, so a repair fails as loudly as a
// regression: the parse must FAIL (no name is invented) and the code must
// be the one recorded. Before this, the document parsed and the column
// was named `&{[x] map[x:1] false}`, the engine's internal struct.
func TestAnObjectHeaderCellRefusesTheDocument(t *testing.T) {
	for _, src := range []string{
		"a,{x:1}\nx,y",
		"a,[{x:1}]\nx,y",
		"a,[1,{x:1}]\nx,y",
		"{x:1},a\nx,y",
		// The cell is the whole header.
		"{x:1}\nx",
		// A SHORT data row still runs the name loop to the header's
		// length, filling the missing cell with field.empty, so the name
		// is still taken. field.exact is off here; with it on the length
		// check wins instead, which is
		// TestFieldExactUsesAHeaderThatWasNeverConverted.
		"a,{x:1}\nx",
		// The first data record is where it stops.
		"a,{x:1}\nx,y\np,q",
	} {
		j := jsonic.Make()
		j.UseDefaults(Csv, Defaults, map[string]any{"strict": false})
		result, err := j.Parse(src)
		if err == nil {
			t.Errorf("%q: parsed to %#v, want a refusal", src, result)
			continue
		}
		assertErrCode(t, src, err, "unexpected")
	}
}

// The same cell is ordinary text in strict mode, where no field body is
// parsed, so the refusal above cannot reach a default-options document.
func TestAnObjectHeaderCellIsTextInStrictMode(t *testing.T) {
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults)
	result, err := j.Parse("a,{x:1}\nx,y")
	if err != nil {
		t.Fatalf("strict parse: %v", err)
	}
	got, _ := json.Marshal(result)
	if want := `[{"a":"x","{x:1}":"y"}]`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// The refusal an object cell provokes must reach only a column name that
// is actually BUILT. The canonical keeps the header row RAW
// (`ctx.u.fields = r.child.node`) and applies ToPropertyKey in one place,
// `obj[fields[fI]] = ...`, which runs only under `object: true` and only
// once there is a record to key.
//
// Refusing at the header row instead was too early, and these are the
// documents it wrongly refused: measured against the canonical runtime,
// which answers [["x","y"]] for the first and [] for the rest, while this
// port answered `unexpected` for every one of them.
func TestTheObjectRefusalReachesOnlyABuiltColumnName(t *testing.T) {
	cases := []struct {
		src      string
		opts     map[string]any
		expected string
	}{
		// object:false never applies ToPropertyKey at all: the header is
		// kept for the field COUNT and later rows come back as arrays.
		{"a,{x:1}\nx,y", map[string]any{"strict": false, "object": false}, `[["x","y"]]`},
		{"{x:1},a\nx,y", map[string]any{"strict": false, "object": false}, `[["x","y"]]`},
		{"a,[1,{x:1}]\nx,y", map[string]any{"strict": false, "object": false}, `[["x","y"]]`},
		// A header-only document has no record to key, under either shape.
		{"a,{x:1}", map[string]any{"strict": false}, `[]`},
		{"a,{x:1}", map[string]any{"strict": false, "object": false}, `[]`},
		{"a,{x:1}\n", map[string]any{"strict": false}, `[]`},
		// header:false: there is no header row, so the object is a field
		// VALUE and no name is ever taken from it.
		{"a,{x:1}\nx,y", map[string]any{"strict": false, "header": false},
			`[{"field~0":"a","field~1":{"x":1}},{"field~0":"x","field~1":"y"}]`},
		{"a,{x:1}\nx,y", map[string]any{"strict": false, "header": false, "object": false},
			`[["a",{"x":1}],["x","y"]]`},
		{"a,{x:1}\nx,y", map[string]any{
			"strict": false, "header": false,
			"field": map[string]any{"names": []string{"p", "q"}},
		}, `[{"p":"a","q":{"x":1}},{"p":"x","q":"y"}]`},
	}
	for _, c := range cases {
		j := jsonic.Make()
		j.UseDefaults(Csv, Defaults, c.opts)
		result, err := j.Parse(c.src)
		if err != nil {
			t.Errorf("%q %v: refused with %v, want %s", c.src, c.opts, err, c.expected)
			continue
		}
		got, _ := json.Marshal(result)
		if string(got) != c.expected {
			t.Errorf("%q %v\n got %s\nwant %s", c.src, c.opts, got, c.expected)
		}
	}
}

// field.exact is measured against the LENGTH of the header row, which the
// canonical reads off a header it has not converted. So it fires, and
// fires first, on a header holding a cell that can never be named -- under
// either result shape. Both codes are what the canonical raises.
func TestFieldExactUsesAHeaderThatWasNeverConverted(t *testing.T) {
	cases := []struct {
		src  string
		opts map[string]any
		code string
	}{
		{"a,{x:1}\nx,y,z", map[string]any{
			"strict": false, "object": false,
			"field": map[string]any{"exact": true},
		}, "csv_extra_field"},
		{"a,{x:1}\nx", map[string]any{
			"strict": false, "object": false,
			"field": map[string]any{"exact": true},
		}, "csv_missing_field"},
		{"a,{x:1}\nx,y,z", map[string]any{
			"strict": false,
			"field":  map[string]any{"exact": true},
		}, "csv_extra_field"},
		{"a,{x:1}\nx", map[string]any{
			"strict": false,
			"field":  map[string]any{"exact": true},
		}, "csv_missing_field"},
	}
	for _, c := range cases {
		j := jsonic.Make()
		j.UseDefaults(Csv, Defaults, c.opts)
		result, err := j.Parse(c.src)
		if err == nil {
			t.Errorf("%q %v: parsed to %#v, want %s", c.src, c.opts, result, c.code)
			continue
		}
		assertErrCode(t, c.src, err, c.code)
	}
}

// field.empty is dropped into a syntactically empty cell, and the header
// row is a row like any other, so an OPTION value can name a column
// without ever passing the lexer. The lexer only makes a float64; a Go
// caller writes whatever Go spells a number with. JavaScript has one
// number type, so the canonical names the column "42" for every one of
// these -- where this port answered `unexpected` for all but the float64,
// because the conversion accepted that one type alone.
func TestFieldEmptyNamesAColumnWhateverNumberGoWasGiven(t *testing.T) {
	for _, empty := range []any{
		42, int8(42), int16(42), int32(42), int64(42),
		uint(42), uint8(42), uint16(42), uint32(42), uint64(42),
		float32(42), float64(42), json.Number("42"),
	} {
		j := jsonic.Make()
		j.UseDefaults(Csv, Defaults, map[string]any{
			"field": map[string]any{"empty": empty},
		})
		result, err := j.Parse(",a\nx,y")
		if err != nil {
			t.Errorf("field.empty %#v: refused with %v", empty, err)
			continue
		}
		got, _ := json.Marshal(result)
		if want := `[{"42":"x","a":"y"}]`; string(got) != want {
			t.Errorf("field.empty %#v\n got %s\nwant %s", empty, got, want)
		}
	}

	// The same widening inside an array cell, which joins its elements by
	// the same rules.
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults, map[string]any{
		"field": map[string]any{"empty": []any{1, int64(2)}},
	})
	result, err := j.Parse(",a\nx,y")
	if err != nil {
		t.Fatalf("field.empty [1,2]: refused with %v", err)
	}
	got, _ := json.Marshal(result)
	if want := `[{"1,2":"x","a":"y"}]`; string(got) != want {
		t.Errorf("field.empty [1,2]\n got %s\nwant %s", got, want)
	}
}

// `field.names` is the other source of a field list, and TS names a
// column from it with the same `obj[fields[fI]] = ...`, which CONVERTS
// the element rather than requiring a string. Keeping only the strings
// dropped the rest, which renamed every column after the dropped one and
// shortened the count `field.exact` compares a record against. The
// expectations are the canonical runtime's.
func TestFieldNamesKeepsEveryElementWhateverItsType(t *testing.T) {
	cases := []struct {
		names    []any
		src      string
		expected string
	}{
		{[]any{1, 2}, "x,y", `[{"1":"x","2":"y"}]`},
		{[]any{true, nil}, "x,y", `[{"null":"y","true":"x"}]`},
		{[]any{1.5, "a"}, "x,y", `[{"1.5":"x","a":"y"}]`},
		{[]any{[]any{1, 2}}, "x", `[{"1,2":"x"}]`},
		// The dropped element used to shorten the list, so the second
		// value fell through to the nonameprefix loop as `field~1`.
		{[]any{1, "a"}, "x,y", `[{"1":"x","a":"y"}]`},
	}
	for _, c := range cases {
		j := jsonic.Make()
		j.UseDefaults(Csv, Defaults, map[string]any{
			"header": false,
			"field":  map[string]any{"names": c.names},
		})
		result, err := j.Parse(c.src)
		if err != nil {
			t.Errorf("names %#v: refused with %v", c.names, err)
			continue
		}
		got, _ := json.Marshal(result)
		if string(got) != c.expected {
			t.Errorf("names %#v\n got %s\nwant %s", c.names, got, c.expected)
		}
	}

	// The COUNT is what `field.exact` compares against, so a kept element
	// is a column the record has to supply.
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults, map[string]any{
		"header": false,
		"field":  map[string]any{"names": []any{1, 2}, "exact": true},
	})
	if _, err := j.Parse("x"); err == nil {
		t.Error(`names [1 2] with field.exact: "x" parsed, want csv_missing_field`)
	} else {
		assertErrCode(t, "names [1 2] with field.exact", err, "csv_missing_field")
	}
}

// An OBJECT reaches the conversion from an option value too, and there
// the canonical does NOT throw. The option merge rebuilds a plain source
// object onto Object.prototype on the way into the bag, so `String`
// gives it "[object Object]" whatever the caller wrote, an object made
// with Object.create(null) included. A PARSED cell is allocated with a
// null prototype and inherits no toString at all, which is what throws.
//
// This port CAN tell one from the other, and the earlier claim in
// DIVERGENCE.md that it could not was wrong. The two routes have
// different Go types: the lexer builds *jsonic.OrderedMap, while an
// option value the caller wrote stays the map[string]any it was written
// as. So the option route is answered exactly as the canonical answers
// it, and only the parsed route is refused. Every expectation below is
// the canonical runtime's own output for the same input.
func TestAnObjectOptionValueNamesTheColumnAsTheCanonicalDoes(t *testing.T) {
	for _, c := range []struct {
		src      string
		opts     map[string]any
		expected string
	}{
		// Reached in the DEFAULT mode: `field.empty` is dropped into a
		// syntactically empty cell before any rule runs, and the header
		// row is a row like any other, so strict mode does not bound
		// this one.
		{",a\nx,y", map[string]any{
			"field": map[string]any{"empty": map[string]any{"q": 1}},
		}, `[{"[object Object]":"x","a":"y"}]`},
		// An object INSIDE an array joins as "[object Object]" too,
		// because join converts each element by the same rules.
		{",a\nx,y", map[string]any{
			"field": map[string]any{"empty": []any{map[string]any{"q": 1}}},
		}, `[{"[object Object]":"x","a":"y"}]`},
		// An empty object is still "[object Object]".
		{",a\nx,y", map[string]any{
			"field": map[string]any{"empty": map[string]any{}},
		}, `[{"[object Object]":"x","a":"y"}]`},
		// A map of some other Go type is the same one object to the
		// canonical, which has one object type.
		{",a\nx,y", map[string]any{
			"field": map[string]any{"empty": map[string]int{"q": 1}},
		}, `[{"[object Object]":"x","a":"y"}]`},
		// `field.names` is the other option route to the same site.
		{"x,y", map[string]any{
			"header": false,
			"field":  map[string]any{"names": []any{map[string]any{"q": 1}}},
		}, `[{"[object Object]":"x","field~1":"y"}]`},
	} {
		j := jsonic.Make()
		j.UseDefaults(Csv, Defaults, c.opts)
		result, err := j.Parse(c.src)
		if err != nil {
			t.Errorf("%v: refused with %v, want %s", c.opts, err, c.expected)
			continue
		}
		got, _ := json.Marshal(result)
		if string(got) != c.expected {
			t.Errorf("%v\n got %s\nwant %s", c.opts, got, c.expected)
		}
	}
}

// The bound on the fix above. A PARSED object cell keeps the null
// prototype the lexer allocates it with, the canonical throws a
// TypeError rather than naming the column, and this port still refuses
// the document there. The type is what separates the two routes, so this
// asserts the separation and not just the refusal: an option map names a
// column in the very same parse that refuses a parsed one.
func TestTheParsedObjectRefusalSurvivesTheOptionObjectFix(t *testing.T) {
	// A parsed object cell, refused.
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults, map[string]any{"strict": false})
	if result, err := j.Parse("a,{x:1}\nx,y"); err == nil {
		t.Errorf("parsed object cell: got %#v, want a refusal", result)
	} else {
		assertErrCode(t, "parsed object cell", err, "unexpected")
	}

	// The same document, with an option object supplying the OTHER
	// column name, in one parse: the option names its column and the
	// parsed cell still refuses.
	j2 := jsonic.Make()
	j2.UseDefaults(Csv, Defaults, map[string]any{
		"strict": false,
		"field":  map[string]any{"empty": map[string]any{"q": 1}},
	})
	if result, err := j2.Parse("a,{x:1}\nx,y"); err == nil {
		t.Errorf("parsed object cell beside an option object: got %#v, want a refusal", result)
	} else {
		assertErrCode(t, "parsed object cell beside an option object", err, "unexpected")
	}

	// And the option object alone, in the same non-strict mode, names
	// its column.
	j3 := jsonic.Make()
	j3.UseDefaults(Csv, Defaults, map[string]any{
		"strict": false,
		"field":  map[string]any{"empty": map[string]any{"q": 1}},
	})
	result, err := j3.Parse(",a\nx,y")
	if err != nil {
		t.Fatalf("option object in non-strict mode: %v", err)
	}
	got, _ := json.Marshal(result)
	if want := `[{"[object Object]":"x","a":"y"}]`; string(got) != want {
		t.Errorf("option object in non-strict mode\n got %s\nwant %s", got, want)
	}
}

// An ARRAY option value is whatever array Go spells most naturally, and
// that is almost never []any: a caller writes []string{"a", "b"}. The
// lexer only ever builds []any, so the shared fixtures reach the name
// site with that one shape and a type assertion on it passed them while
// refusing every native spelling. JavaScript has one array type and
// Array.prototype.toString is join(','), so each of these is the same
// array to the canonical, and each expectation below is its output.
func TestAnArrayOptionValueNamesTheColumnWhateverGoSliceItIs(t *testing.T) {
	for _, c := range []struct {
		name     string
		empty    any
		expected string
	}{
		{"[]string", []string{"a", "b"}, `[{"a":"y","a,b":"x"}]`},
		{"[]int", []int{1, 2}, `[{"1,2":"x","a":"y"}]`},
		{"[]float64", []float64{1.5, 2}, `[{"1.5,2":"x","a":"y"}]`},
		{"[]bool", []bool{true, false}, `[{"a":"y","true,false":"x"}]`},
		{"[]any", []any{"a", "b"}, `[{"a":"y","a,b":"x"}]`},
		// join recurses, so a nested array flattens.
		{"[][]string", [][]string{{"a", "b"}, {"c"}}, `[{"a":"y","a,b,c":"x"}]`},
		// A fixed-size array is an array to JavaScript too.
		{"[2]string", [2]string{"a", "b"}, `[{"a":"y","a,b":"x"}]`},
		// An EMPTY array joins to the empty string, which is a name.
		{"[]string{}", []string{}, `[{"":"x","a":"y"}]`},
		{"[]any{}", []any{}, `[{"":"x","a":"y"}]`},
	} {
		j := jsonic.Make()
		j.UseDefaults(Csv, Defaults, map[string]any{
			"field": map[string]any{"empty": c.empty},
		})
		result, err := j.Parse(",a\nx,y")
		if err != nil {
			t.Errorf("field.empty %s: refused with %v, want %s", c.name, err, c.expected)
			continue
		}
		got, _ := json.Marshal(result)
		if string(got) != c.expected {
			t.Errorf("field.empty %s\n got %s\nwant %s", c.name, got, c.expected)
		}
	}

	// `field.names` reaches the same conversion, and takes the same
	// widening: a native []int names columns "1" and "2", as
	// `names: [1, 2]` does in the canonical.
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults, map[string]any{
		"header": false,
		"field":  map[string]any{"names": []int{1, 2}},
	})
	result, err := j.Parse("x,y")
	if err != nil {
		t.Fatalf("field.names []int: %v", err)
	}
	got, _ := json.Marshal(result)
	if want := `[{"1":"x","2":"y"}]`; string(got) != want {
		t.Errorf("field.names []int\n got %s\nwant %s", got, want)
	}

	// A STRING is not a list of characters here. Its Kind is neither
	// Slice nor Array, so it never becomes a one-element list, and it
	// names the column as itself.
	js := jsonic.Make()
	js.UseDefaults(Csv, Defaults, map[string]any{
		"field": map[string]any{"empty": "ab"},
	})
	sres, err := js.Parse(",a\nx,y")
	if err != nil {
		t.Fatalf(`field.empty "ab": %v`, err)
	}
	sgot, _ := json.Marshal(sres)
	if want := `[{"a":"y","ab":"x"}]`; string(sgot) != want {
		t.Errorf("field.empty \"ab\"\n got %s\nwant %s", sgot, want)
	}
}

// An EXPLICITLY EMPTY `field.names` is a field list, not an absent one.
//
// TS reads the list as `ctx.u.fields || options.field.names`, and every
// array is truthy there, `[]` included, so `names: []` reaches the
// `field.exact` check with length 0 and a one-cell record is one field
// too many. Building the Go list with an append loop seeded from a nil
// slice left nil for an empty input, the `fields != nil` guard read that
// as "no field list", and the documented option silently did nothing.
// Both expectations are the canonical runtime's.
func TestAnExplicitlyEmptyFieldNamesListIsStillAFieldList(t *testing.T) {
	for _, names := range []any{[]string{}, []any{}, [0]string{}} {
		// With field.exact, one cell is one field too many.
		j := jsonic.Make()
		j.UseDefaults(Csv, Defaults, map[string]any{
			"header": false,
			"field":  map[string]any{"names": names, "exact": true},
		})
		if result, err := j.Parse("x"); err == nil {
			t.Errorf("names %#v with field.exact: parsed to %#v, want csv_extra_field",
				names, result)
		} else {
			assertErrCode(t, "empty field.names with field.exact", err, "csv_extra_field")
		}

		// An empty document has no record, so nothing is compared and
		// the parse succeeds, as it does in the canonical.
		je := jsonic.Make()
		je.UseDefaults(Csv, Defaults, map[string]any{
			"header": false,
			"field":  map[string]any{"names": names, "exact": true},
		})
		result, err := je.Parse("")
		if err != nil {
			t.Errorf("names %#v with field.exact on empty input: %v", names, err)
		} else if got, _ := json.Marshal(result); string(got) != "[]" {
			t.Errorf("names %#v on empty input\n got %s\nwant []", names, got)
		}

		// Without field.exact the empty list names no column, and every
		// cell falls through to the nonameprefix loop.
		jn := jsonic.Make()
		jn.UseDefaults(Csv, Defaults, map[string]any{
			"header": false,
			"field":  map[string]any{"names": names},
		})
		nres, err := jn.Parse("x")
		if err != nil {
			t.Errorf("names %#v without field.exact: %v", names, err)
			continue
		}
		got, _ := json.Marshal(nres)
		if want := `[{"field~0":"x"}]`; string(got) != want {
			t.Errorf("names %#v without field.exact\n got %s\nwant %s", names, got, want)
		}
	}

	// A value that is not a list at all IS absent, and field.exact has
	// nothing to compare against, so the same document parses.
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults, map[string]any{
		"header": false,
		"field":  map[string]any{"names": nil, "exact": true},
	})
	result, err := j.Parse("x")
	if err != nil {
		t.Fatalf("names nil with field.exact: %v", err)
	}
	got, _ := json.Marshal(result)
	if want := `[{"field~0":"x"}]`; string(got) != want {
		t.Errorf("names nil with field.exact\n got %s\nwant %s", got, want)
	}
}

// `string.quote` is a STRING option with the same shape of defect. The
// canonical opens a quoted field with `quoteMap[src[sI]]`, and `src[sI]`
// is ONE UTF-16 code unit, so its matcher can only ever fire for a quote
// that is exactly one code unit and is inert for every other value.
//
// strings.HasPrefix agreed with none of that. It matched a multi-
// character quote and an astral one, which the canonical cannot match,
// and for the EMPTY quote it returned true at every position, so the
// matcher consumed nothing and `string.quote: ""` HUNG the parse. Each
// expectation below is the canonical runtime's output, and the test is
// bounded so a regression fails rather than hangs the suite.
func TestADegenerateQuoteIsInertRatherThanGreedy(t *testing.T) {
	for _, c := range []struct {
		name     string
		quote    string
		src      string
		expected string
	}{
		// One code unit: the matcher fires, in both runtimes.
		{"single char", "|", "a,b\n|x y|,z", `[{"a":"x y","b":"z"}]`},
		// Two characters: inert, so the text stays as written.
		{"two chars", "ab", "a,b\nabx yab,z", `[{"a":"abx yab","b":"z"}]`},
		// An astral character is a surrogate PAIR in JavaScript, so it is
		// two code units and the canonical never matches it either.
		{"astral", "\U0001F600", "a,b\n\U0001F600x y\U0001F600,z",
			"[{\"a\":\"\U0001F600x y\U0001F600\",\"b\":\"z\"}]"},
	} {
		done := make(chan struct{})
		var got []byte
		var perr error
		go func() {
			defer close(done)
			j := jsonic.Make()
			j.UseDefaults(Csv, Defaults, map[string]any{
				"string": map[string]any{"quote": c.quote},
			})
			result, err := j.Parse(c.src)
			if err != nil {
				perr = err
				return
			}
			got, _ = json.Marshal(result)
		}()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Fatalf("string.quote %s: the parse did not finish", c.name)
		}
		if perr != nil {
			t.Errorf("string.quote %s: %v", c.name, perr)
			continue
		}
		if string(got) != c.expected {
			t.Errorf("string.quote %s\n got %s\nwant %s", c.name, got, c.expected)
		}
	}

	// The empty quote is the one that hung: every string has the empty
	// prefix, so the matcher matched at every position and consumed
	// nothing. It must now simply RETURN. What it returns is bounded by
	// a separate, pre-existing difference in how this port disables the
	// standard jsonic string lexer in strict mode, so this asserts
	// termination and a parse, not the canonical's exact value.
	done := make(chan struct{})
	go func() {
		defer close(done)
		j := jsonic.Make()
		j.UseDefaults(Csv, Defaults, map[string]any{
			"string": map[string]any{"quote": ""},
		})
		if _, err := j.Parse("a,b\n\"x y\",z"); err != nil {
			t.Errorf(`string.quote "": %v`, err)
		}
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal(`string.quote "": the parse did not finish`)
	}
}
