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

// The rest of the vocabulary a parsed field can hold.
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
	}
	for _, c := range cases {
		if got := jsKey(c.value); got != c.expected {
			t.Errorf("jsKey(%#v) = %q, want %q", c.value, got, c.expected)
		}
	}
}
