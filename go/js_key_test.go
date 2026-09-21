/* Copyright (c) 2021-2025 Richard Rodger, MIT License */

package tabnascsv

import (
	"encoding/json"
	"math"
	"testing"

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

// An object has no ToString to apply, at the top of a cell or anywhere
// inside one, so jsKey refuses it rather than inventing a name. Before
// this, the last-resort `fmt.Sprintf("%v", v)` put the engine's internal
// struct into a column name: `&{[x] map[x:1] false}`.
func TestJsKeyRefusesAnObject(t *testing.T) {
	object := jsonic.NewOrderedMap()
	object.Set("x", float64(1))
	for _, value := range []any{
		object,
		map[string]any{"x": float64(1)},
		[]any{object},
		[]any{float64(1), object},
		[]any{[]any{object}},
	} {
		if got, ok := jsKey(value); ok {
			t.Errorf("jsKey(%#v) = %q, want a refusal", value, got)
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
