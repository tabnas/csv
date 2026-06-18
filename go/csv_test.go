package tabnascsv

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"
)

// fixtureEntry represents one entry in the test manifest.
type fixtureEntry struct {
	Name      string         `json:"name"`
	CsvFile   string         `json:"csvFile,omitempty"`
	Opt       map[string]any `json:"opt,omitempty"`
	JsonicOpt map[string]any `json:"jsonicOpt,omitempty"`
	Err       string         `json:"err,omitempty"`
}

func fixturesDir() string {
	return filepath.Join("..", "test", "fixtures")
}

// csvParse creates a jsonic instance with the Csv plugin and parses src.
func csvParse(src string, opts ...map[string]any) ([]any, error) {
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults, opts...)

	result, err := j.Parse(src)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []any{}, nil
	}
	if arr, ok := result.([]any); ok {
		return arr, nil
	}
	return []any{}, nil
}

func TestFixtures(t *testing.T) {
	dir := fixturesDir()
	manifestPath := filepath.Join(dir, "manifest.json")

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("Failed to read manifest: %v", err)
	}

	var manifest map[string]fixtureEntry
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	for key, entry := range manifest {
		t.Run(entry.Name, func(t *testing.T) {
			csvFile := entry.CsvFile
			if csvFile == "" {
				csvFile = key
			}

			csvData, err := os.ReadFile(filepath.Join(dir, csvFile+".csv"))
			if err != nil {
				t.Fatalf("Failed to read CSV file %s: %v", csvFile, err)
			}

			result, err := parseFixture(string(csvData), entry.Opt, entry.JsonicOpt)
			if err != nil {
				if entry.Err != "" {
					return // expected error
				}
				t.Fatalf("Unexpected error: %v", err)
			}

			if entry.Err != "" {
				t.Fatalf("Expected error %s but got none", entry.Err)
			}

			expectedData, err := os.ReadFile(filepath.Join(dir, key+".json"))
			if err != nil {
				t.Fatalf("Failed to read expected JSON: %v", err)
			}

			var expected []any
			if err := json.Unmarshal(expectedData, &expected); err != nil {
				t.Fatalf("Failed to parse expected JSON: %v", err)
			}

			resultNorm := normalizeResult(result)
			expectedNorm := normalizeJSON(expected)

			if !reflect.DeepEqual(resultNorm, expectedNorm) {
				resultJSON, _ := json.MarshalIndent(resultNorm, "", "  ")
				expectedJSON, _ := json.MarshalIndent(expectedNorm, "", "  ")
				t.Errorf("Fixture %q mismatch:\nGot:      %s\nExpected: %s",
					entry.Name, string(resultJSON), string(expectedJSON))
			}
		})
	}
}

func TestPlugin(t *testing.T) {
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults)

	result, err := j.Parse("a,b\n1,2\n3,4")
	if err != nil {
		t.Fatalf("Plugin parse error: %v", err)
	}

	arr, ok := result.([]any)
	if !ok {
		t.Fatalf("Expected []any, got %T", result)
	}

	if len(arr) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(arr))
	}

	r0 := toMap(arr[0])
	if r0["a"] != "1" || r0["b"] != "2" {
		t.Errorf("Record 0: expected {a:1,b:2}, got %v", r0)
	}
}

func TestPluginWithOptions(t *testing.T) {
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults, map[string]any{"object": false})

	result, err := j.Parse("a,b\n1,2")
	if err != nil {
		t.Fatalf("Plugin parse error: %v", err)
	}

	arr, ok := result.([]any)
	if !ok {
		t.Fatalf("Expected []any, got %T", result)
	}

	if len(arr) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(arr))
	}

	inner, ok := arr[0].([]any)
	if !ok {
		t.Fatalf("Expected inner []any, got %T", arr[0])
	}

	if inner[0] != "1" || inner[1] != "2" {
		t.Errorf("Expected [1,2], got %v", inner)
	}
}

func TestPluginEmpty(t *testing.T) {
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults)

	result, err := j.Parse("")
	if err != nil {
		t.Fatalf("Plugin parse error: %v", err)
	}

	arr, ok := result.([]any)
	if !ok {
		t.Fatalf("Expected []any, got %T: %v", result, result)
	}

	if len(arr) != 0 {
		t.Errorf("Expected empty array, got %v", arr)
	}
}

func TestUsePlugin(t *testing.T) {
	j := jsonic.Make()
	j.Use(Csv, nil)

	result, err := j.Parse("a,b\n1,2")
	if err != nil {
		t.Logf("Plugin parse returned error (expected with basic plugin): %v", err)
	}
	_ = result
}

func TestEmptyRecords(t *testing.T) {
	result, _ := csvParse("a\n1\n\n2\n3\n\n\n4\n")
	assertRecords(t, "empty-ignored", result, []map[string]any{
		{"a": "1"}, {"a": "2"}, {"a": "3"}, {"a": "4"},
	})

	result2, _ := csvParse("a\n1\n\n2\n3\n\n\n4\n",
		map[string]any{"record": map[string]any{"empty": true}})
	assertRecords(t, "empty-preserved", result2, []map[string]any{
		{"a": "1"}, {"a": ""}, {"a": "2"}, {"a": "3"},
		{"a": ""}, {"a": ""}, {"a": "4"},
	})

	// Empty input yields no records.
	if r, _ := csvParse("\n"); len(r) != 0 {
		t.Errorf("empty-input: expected 0 records, got %v", r)
	}

	// Leading/trailing blank lines (CRLF) are ignored.
	result3, _ := csvParse("\r\n\r\na,b\r\nA,B\r\n\r\n")
	assertRecords(t, "empty-edges", result3, []map[string]any{
		{"a": "A", "b": "B"},
	})

	// Comment lines are dropped and do not become empty records.
	result4, _ := csvParse("a#X\n1\n#Y\n2\n3\n\n#Z\n4\n#Q",
		map[string]any{"comment": true})
	assertRecords(t, "empty-comments", result4, []map[string]any{
		{"a": "1"}, {"a": "2"}, {"a": "3"}, {"a": "4"},
	})

	// With record.empty, dropped comment lines still do not become records,
	// but genuine blank lines do.
	result5, _ := csvParse("a#X\n1\n#Y\n2\n3\n\n#Z\n4\n#Q",
		map[string]any{"comment": true, "record": map[string]any{"empty": true}})
	assertRecords(t, "empty-comments-preserved", result5, []map[string]any{
		{"a": "1"}, {"a": ""}, {"a": "2"}, {"a": "3"},
		{"a": ""}, {"a": ""}, {"a": "4"},
	})
}

// TestFieldExact checks that field.exact halts the parse on field-count
// mismatches. NOTE: jsonic-go surfaces the error under the generic
// "unexpected" code rather than csv_extra_field / csv_missing_field (see
// the note in csv.go and AGENTS.md); this test asserts the parse fails.
func TestFieldExact(t *testing.T) {
	if _, err := csvParse("a,b\n1,2,3",
		map[string]any{"field": map[string]any{"exact": true}}); err == nil {
		t.Errorf("exact extra: expected error, got none")
	}
	if _, err := csvParse("a,b\n1",
		map[string]any{"field": map[string]any{"exact": true}}); err == nil {
		t.Errorf("exact missing: expected error, got none")
	}
	// Matching field counts parse cleanly.
	if _, err := csvParse("a,b\n1,2",
		map[string]any{"field": map[string]any{"exact": true}}); err != nil {
		t.Errorf("exact match: unexpected error: %v", err)
	}
}

func TestHeader(t *testing.T) {
	result, _ := csvParse("\na,b\nA,B")
	assertRecords(t, "header-skip-leading", result, []map[string]any{
		{"a": "A", "b": "B"},
	})

	result2, _ := csvParse("\na,b\nA,B", map[string]any{"header": false})
	assertRecords(t, "no-header", result2, []map[string]any{
		{"field~0": "a", "field~1": "b"},
		{"field~0": "A", "field~1": "B"},
	})

	// header:false with object:false yields every row as a slice.
	resultArr, _ := csvParse("\na,b\nA,B",
		map[string]any{"header": false, "object": false})
	if !reflect.DeepEqual(resultArr, []any{
		[]any{"a", "b"}, []any{"A", "B"},
	}) {
		t.Errorf("no-header-array: got %#v", resultArr)
	}

	// header:false with explicit field.names yields named objects for
	// every row, including the one that would otherwise be the header.
	resultNames, _ := csvParse("\na,b\nA,B", map[string]any{
		"header": false,
		"field":  map[string]any{"names": []string{"a", "b"}},
	})
	assertRecords(t, "field-names", resultNames, []map[string]any{
		{"a": "a", "b": "b"},
		{"a": "A", "b": "B"},
	})
}

// TestComma mirrors the TS `comma` test: leading/trailing separators
// produce empty fields, and unnamed overflow columns get field~N keys.
func TestComma(t *testing.T) {
	cases := []struct {
		src  string
		want []map[string]any
	}{
		{"a\n1,", []map[string]any{{"a": "1", "field~1": ""}}},
		{"a\n,1", []map[string]any{{"a": "", "field~1": "1"}}},
		{"a,b\n1,2,", []map[string]any{{"a": "1", "b": "2", "field~2": ""}}},
		{"a,b\n,1,2", []map[string]any{{"a": "", "b": "1", "field~2": "2"}}},
		{"a\n1,\n", []map[string]any{{"a": "1", "field~1": ""}}},
		{"a,b\n1,2,\n", []map[string]any{{"a": "1", "b": "2", "field~2": ""}}},
	}
	for _, c := range cases {
		r, err := csvParse(c.src)
		if err != nil {
			t.Errorf("comma %q: %v", c.src, err)
			continue
		}
		assertRecords(t, "comma "+c.src, r, c.want)
	}

	// Leading blank line yields no records.
	if r, _ := csvParse("\na"); len(r) != 0 {
		t.Errorf("comma leading-blank: expected 0 records, got %v", r)
	}

	// object:false variants.
	ra, _ := csvParse("a\n1,", map[string]any{"object": false})
	if !reflect.DeepEqual(ra, []any{[]any{"1", ""}}) {
		t.Errorf("comma array trailing: got %#v", ra)
	}
	rb, _ := csvParse("a,b\n,1,2", map[string]any{"object": false})
	if !reflect.DeepEqual(rb, []any{[]any{"", "1", "2"}}) {
		t.Errorf("comma array leading: got %#v", rb)
	}
}

func TestDoubleQuotes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`a` + "\n" + `"b"`, "b"},
		{`a` + "\n" + `"""b"`, `"b`},
		{`a` + "\n" + `"b"""`, `b"`},
		{`a` + "\n" + `"""b"""`, `"b"`},
		{`a` + "\n" + `"b""c"`, `b"c`},
		{`a` + "\n" + `"b""c""d"`, `b"c"d`},
		{`a` + "\n" + `"b""c""d""e"`, `b"c"d"e`},
		{`a` + "\n" + `"""""b"`, `""b`},
		{`a` + "\n" + `"b"""""`, `b""`},
		{`a` + "\n" + `"""""b"""""`, `""b""`},
	}

	for _, tt := range tests {
		result, err := csvParse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q): error: %v", tt.input, err)
			continue
		}
		if len(result) != 1 {
			t.Errorf("Parse(%q): expected 1 record, got %d", tt.input, len(result))
			continue
		}
		m := toMap(result[0])
		if m["a"] != tt.expected {
			t.Errorf("Parse(%q): expected a=%q, got a=%q", tt.input, tt.expected, m["a"])
		}
	}
}

func TestTrim(t *testing.T) {
	r1, _ := csvParse("a\n b")
	assertField(t, "no-trim-leading", r1, "a", " b")

	r2, _ := csvParse("a\nb ")
	assertField(t, "no-trim-trailing", r2, "a", "b ")

	r3, _ := csvParse("a\n b ")
	assertField(t, "no-trim-both", r3, "a", " b ")

	r4, _ := csvParse("a\n b", map[string]any{"trim": true})
	assertField(t, "trim-leading", r4, "a", "b")

	r5, _ := csvParse("a\nb ", map[string]any{"trim": true})
	assertField(t, "trim-trailing", r5, "a", "b")

	r6, _ := csvParse("a\n b c ", map[string]any{"trim": true})
	assertField(t, "trim-internal", r6, "a", "b c")
}

func TestComment(t *testing.T) {
	r1, _ := csvParse("a\n# b")
	assertField(t, "no-comment", r1, "a", "# b")

	r2, _ := csvParse("a\n# b", map[string]any{"comment": true})
	if len(r2) != 0 {
		t.Errorf("comment-line: expected 0 records, got %d", len(r2))
	}

	r3, _ := csvParse("a\n b #c", map[string]any{"comment": true})
	assertField(t, "comment-inline", r3, "a", " b ")

	// Non-strict mode enables comment (and trim) by default.
	r4, _ := csvParse("a\n# b", map[string]any{"strict": false})
	if len(r4) != 0 {
		t.Errorf("comment-nonstrict: expected 0 records, got %d", len(r4))
	}
	r5, _ := csvParse("a\n b ", map[string]any{"strict": false})
	assertField(t, "comment-nonstrict-trim", r5, "a", "b")
}

func TestNumber(t *testing.T) {
	r1, _ := csvParse("a\n1")
	assertField(t, "no-number", r1, "a", "1")

	r2, _ := csvParse("a\n1", map[string]any{"number": true})
	m := toMap(r2[0])
	if m["a"] != float64(1) {
		t.Errorf("number: expected 1 (float64), got %v (%T)", m["a"], m["a"])
	}

	// Exponent notation is parsed when number is enabled.
	r3, _ := csvParse("a\n1e2", map[string]any{"number": true})
	if toMap(r3[0])["a"] != float64(100) {
		t.Errorf("number-exp: expected 100, got %v", toMap(r3[0])["a"])
	}

	// Strict mode keeps numbers as strings unless opted in.
	r4, _ := csvParse("a\n1e2")
	assertField(t, "number-strict-string", r4, "a", "1e2")

	// Non-strict mode coerces numbers by default.
	r5, _ := csvParse("a\n1e2", map[string]any{"strict": false})
	if toMap(r5[0])["a"] != float64(100) {
		t.Errorf("number-nonstrict: expected 100, got %v", toMap(r5[0])["a"])
	}
}

func TestValue(t *testing.T) {
	r1, _ := csvParse("a\ntrue")
	assertField(t, "no-value", r1, "a", "true")

	r2, _ := csvParse("a\ntrue", map[string]any{"value": true})
	m := toMap(r2[0])
	if m["a"] != true {
		t.Errorf("value-true: expected true, got %v (%T)", m["a"], m["a"])
	}

	r3, _ := csvParse("a\nfalse", map[string]any{"value": true})
	m3 := toMap(r3[0])
	if m3["a"] != false {
		t.Errorf("value-false: expected false, got %v (%T)", m3["a"], m3["a"])
	}

	r4, _ := csvParse("a\nnull", map[string]any{"value": true})
	m4 := toMap(r4[0])
	if m4["a"] != nil {
		t.Errorf("value-null: expected nil, got %v (%T)", m4["a"], m4["a"])
	}
}

func TestStream(t *testing.T) {
	var events []string
	var records []any

	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults, map[string]any{
		"stream": func(what string, record any) {
			events = append(events, what)
			if what == "record" {
				records = append(records, record)
			}
		},
	})
	j.Parse("a,b\n1,2\n3,4\n5,6")

	if len(events) < 3 {
		t.Fatalf("Expected at least 3 events, got %d", len(events))
	}
	if events[0] != "start" {
		t.Errorf("First event should be 'start', got %q", events[0])
	}
	if events[len(events)-1] != "end" {
		t.Errorf("Last event should be 'end', got %q", events[len(events)-1])
	}
	if len(records) != 3 {
		t.Errorf("Expected 3 records, got %d", len(records))
	}
}

func TestSeparators(t *testing.T) {
	result, _ := csvParse("a|b|c\nA|B|C\nAA|BB|CC",
		map[string]any{"field": map[string]any{"separation": "|"}})
	assertRecords(t, "pipe", result, []map[string]any{
		{"a": "A", "b": "B", "c": "C"},
		{"a": "AA", "b": "BB", "c": "CC"},
	})

	result2, _ := csvParse("a~~b~~c\nA~~B~~C",
		map[string]any{"field": map[string]any{"separation": "~~"}})
	assertRecords(t, "multi-char", result2, []map[string]any{
		{"a": "A", "b": "B", "c": "C"},
	})
}

func TestRecordSeparators(t *testing.T) {
	result, _ := csvParse("a,b,c%A,B,C%AA,BB,CC",
		map[string]any{"record": map[string]any{"separators": "%"}})
	assertRecords(t, "record-sep", result, []map[string]any{
		{"a": "A", "b": "B", "c": "C"},
		{"a": "AA", "b": "BB", "c": "CC"},
	})
}

func TestUnstrict(t *testing.T) {
	// scalar coercion (non-strict enables number by default; numbers
	// arrive as float64 from the underlying jsonic value lexer)
	r1, err := csvParse("a,b\n1,2", map[string]any{"strict": false})
	if err != nil {
		t.Fatalf("unstrict scalar: %v", err)
	}
	m1 := toMap(r1[0])
	if !reflect.DeepEqual(m1, map[string]any{"a": float64(1), "b": float64(2)}) {
		t.Errorf("unstrict scalar: got %#v", m1)
	}

	// embedded JSON values
	r2, err := csvParse("a,b,c\ntrue,[1,2],{x:1}",
		map[string]any{"strict": false})
	if err != nil {
		t.Fatalf("unstrict embedded: %v", err)
	}
	m2 := toMap(r2[0])
	if m2["a"] != true {
		t.Errorf("unstrict a: expected true, got %v", m2["a"])
	}
	if !reflect.DeepEqual(m2["b"], []any{float64(1), float64(2)}) {
		t.Errorf("unstrict b: expected [1 2], got %#v", m2["b"])
	}
	if !reflect.DeepEqual(m2["c"], map[string]any{"x": float64(1)}) {
		t.Errorf("unstrict c: expected {x:1}, got %#v", m2["c"])
	}

	// nested objects with quoted strings + Jsonic-style escapes
	src := "a,b,c\ntrue,[1,2],{x:{y:\"q\\\"w\"}}\n x , 'y\\'y', \"z\\\"z\"\n"
	r3, err := csvParse(src, map[string]any{"strict": false})
	if err != nil {
		t.Fatalf("unstrict full: %v", err)
	}
	if len(r3) != 2 {
		t.Fatalf("unstrict full: expected 2 records, got %d", len(r3))
	}
	row0 := toMap(r3[0])
	wantC0 := map[string]any{"x": map[string]any{"y": `q"w`}}
	if !reflect.DeepEqual(row0["c"], wantC0) {
		t.Errorf("unstrict full row0.c: expected %v, got %v", wantC0, row0["c"])
	}
	row1 := toMap(r3[1])
	if row1["a"] != "x" || row1["b"] != "y'y" || row1["c"] != `z"z` {
		t.Errorf("unstrict full row1: got %v", row1)
	}

	// Trailing content after a complete embedded value is a syntax error,
	// mirroring the TS `assert.throws(() => j('a\n{x:1}y'), /unexpected/)`.
	if _, err := csvParse("a\n{x:1}y", map[string]any{"strict": false}); err == nil {
		t.Errorf("unstrict trailing-garbage: expected error, got none")
	}
}

func TestEmptyAnyType(t *testing.T) {
	// nil empty value
	r1, _ := csvParse("a,b,c\n1,,3",
		map[string]any{"field": map[string]any{"empty": nil}})
	m1 := toMap(r1[0])
	if m1["b"] != nil {
		t.Errorf("empty=nil: expected nil, got %v (%T)", m1["b"], m1["b"])
	}

	// bool empty value
	r2, _ := csvParse("a,b\n1,",
		map[string]any{"field": map[string]any{"empty": false}})
	m2 := toMap(r2[0])
	if m2["b"] != false {
		t.Errorf("empty=false: expected false, got %v (%T)", m2["b"], m2["b"])
	}

	// numeric empty value
	r3, _ := csvParse("a,b\n1,",
		map[string]any{"field": map[string]any{"empty": 42}})
	m3 := toMap(r3[0])
	if m3["b"] != 42 {
		t.Errorf("empty=42: expected 42, got %v (%T)", m3["b"], m3["b"])
	}
}

func TestObjectOutputIsMap(t *testing.T) {
	// Records under default options must be plain map[string]any so
	// external callers can read them and json.Marshal them sensibly.
	r, err := csvParse("name,age\nAlice,30\nBob,25")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := r[0].(map[string]any); !ok {
		t.Fatalf("expected map[string]any, got %T", r[0])
	}
	m := r[0].(map[string]any)
	if m["name"] != "Alice" || m["age"] != "30" {
		t.Errorf("got %v", m)
	}
}

// parseFixture parses CSV with optional jsonic-level options for fixtures.
func parseFixture(src string, pluginOpts map[string]any, jsonicOpts map[string]any) ([]any, error) {
	if len(jsonicOpts) == 0 {
		return csvParse(src, pluginOpts)
	}

	j := jsonic.Make()

	// Apply jsonicOpt: value.def
	if valOpt, ok := jsonicOpts["value"].(map[string]any); ok {
		if defMap, ok := valOpt["def"].(map[string]any); ok {
			vopts := jsonic.Options{Value: &jsonic.ValueOptions{
				Def: map[string]*jsonic.ValueDef{
					"true":  {Val: true},
					"false": {Val: false},
					"null":  {Val: nil},
				},
			}}
			for k, v := range defMap {
				if v == nil {
					delete(vopts.Value.Def, k)
				} else if vm, ok := v.(map[string]any); ok {
					vopts.Value.Def[k] = &jsonic.ValueDef{Val: vm["val"]}
				}
			}
			j.SetOptions(vopts)
		}
	}

	// Apply jsonicOpt: comment.def
	if cmtOpt, ok := jsonicOpts["comment"].(map[string]any); ok {
		if defMap, ok := cmtOpt["def"].(map[string]any); ok {
			copts := jsonic.Options{Comment: &jsonic.CommentOptions{
				Def: make(map[string]*jsonic.CommentDef),
			}}
			for name, v := range defMap {
				if cm, ok := v.(map[string]any); ok {
					def := &jsonic.CommentDef{}
					if start, ok := cm["start"].(string); ok {
						def.Start = start
					}
					if end, ok := cm["end"].(string); ok {
						def.End = end
					} else {
						def.Line = true
					}
					copts.Comment.Def[name] = def
				}
			}
			j.SetOptions(copts)
		}
	}

	j.UseDefaults(Csv, Defaults, pluginOpts)

	result, err := j.Parse(src)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []any{}, nil
	}
	if arr, ok := result.([]any); ok {
		return arr, nil
	}
	return []any{}, nil
}

// Helpers

func assertRecords(t *testing.T, name string, result []any, expected []map[string]any) {
	t.Helper()
	if len(result) != len(expected) {
		t.Errorf("%s: expected %d records, got %d: %v", name, len(expected), len(result), result)
		return
	}
	for i, exp := range expected {
		m := toMap(result[i])
		for k, v := range exp {
			if fmt.Sprintf("%v", m[k]) != fmt.Sprintf("%v", v) {
				t.Errorf("%s: record %d, field %q: expected %v, got %v", name, i, k, v, m[k])
			}
		}
	}
}

func assertField(t *testing.T, name string, result []any, key string, expected string) {
	t.Helper()
	if len(result) != 1 {
		t.Errorf("%s: expected 1 record, got %d", name, len(result))
		return
	}
	m := toMap(result[0])
	if m[key] != expected {
		t.Errorf("%s: expected %q=%q, got %q=%q", name, key, expected, key, m[key])
	}
}

func toMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func normalizeResult(result []any) []any {
	out := make([]any, len(result))
	for i, r := range result {
		out[i] = normalizeValue(r)
	}
	return out
}

func normalizeValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		m := make(map[string]any)
		for k, v := range val {
			m[k] = normalizeValue(v)
		}
		return m
	case []any:
		out := make([]any, len(val))
		for i, v := range val {
			out[i] = normalizeValue(v)
		}
		return out
	default:
		return v
	}
}

func normalizeJSON(v any) any {
	switch val := v.(type) {
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = normalizeJSON(item)
		}
		return out
	case map[string]any:
		m := make(map[string]any)
		for k, v := range val {
			m[k] = normalizeJSON(v)
		}
		return m
	default:
		return v
	}
}
