// Copyright (c) 2026 Richard Rodger and other contributors, MIT License

package tabnascsv

// make_test.go — Make(options) is jsonic.Make() + UseDefaults(Csv,
// Defaults, options) behind one call. These tests hold it to that: every
// committed fixture parses the same through both routes, with the
// fixture's own options, and a handful of non-default options are checked
// directly. The TypeScript half is ts/test/make.test.ts.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"
)

// makeOutcome is what a parse produced: the value flattened through JSON,
// or the error's code. Comparing codes rather than messages is the
// fixture contract (test/AGENTS.md).
type makeOutcome struct {
	Value any
	Error string
}

func outcomeOf(j *jsonic.Jsonic, src string) makeOutcome {
	out, err := j.Parse(src)
	if err != nil {
		var je *jsonic.JsonicError
		if errors.As(err, &je) {
			return makeOutcome{Error: je.Code}
		}
		return makeOutcome{Error: err.Error()}
	}
	return makeOutcome{Value: jsonFlatten(out)}
}

func viaPlugin(t *testing.T, opts ...map[string]any) *jsonic.Jsonic {
	t.Helper()
	j := jsonic.Make()
	if err := j.UseDefaults(Csv, Defaults, opts...); err != nil {
		t.Fatalf("plugin route: %v", err)
	}
	return j
}

func viaMake(t *testing.T, opts ...map[string]any) *jsonic.Jsonic {
	t.Helper()
	j, err := Make(opts...)
	if err != nil {
		t.Fatalf("Make: %v", err)
	}
	return j
}

func TestMakeReturnsAParser(t *testing.T) {
	j := viaMake(t)
	got := outcomeOf(j, "a,b\n1,2\n3,4")
	want := makeOutcome{Value: jsonFlatten([]any{
		map[string]any{"a": "1", "b": "2"},
		map[string]any{"a": "3", "b": "4"},
	})}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestMakeDefaultsMatchThePlugin(t *testing.T) {
	for _, src := range []string{"", "\n", "a\n\"b\"\"c\"", "a,b\n1,2,3", "a\n\"b"} {
		want := outcomeOf(viaPlugin(t), src)
		if got := outcomeOf(viaMake(t), src); !reflect.DeepEqual(got, want) {
			t.Errorf("Make() %q: got %#v, want %#v", src, got, want)
		}
		if got := outcomeOf(viaMake(t, map[string]any{}), src); !reflect.DeepEqual(got, want) {
			t.Errorf("Make({}) %q: got %#v, want %#v", src, got, want)
		}
	}
	if got := outcomeOf(viaMake(t), "a\n\"b"); got.Error != "unterminated_string" {
		t.Fatalf("unterminated quote: got %#v", got)
	}
}

func TestMakeOptionsMatchThePlugin(t *testing.T) {
	cases := []struct {
		opts map[string]any
		src  string
		want string
	}{
		{map[string]any{"header": false, "object": false}, "a,b\n1,2", `[["a","b"],["1","2"]]`},
		{map[string]any{"field": map[string]any{"separation": ";"}}, "a;b\n1;2", `[{"a":"1","b":"2"}]`},
		{map[string]any{"record": map[string]any{"empty": true}}, "a\n1\n\n2", `[{"a":"1"},{"a":""},{"a":"2"}]`},
		{map[string]any{"strict": false}, "a,b\n{x:1},true", `[{"a":{"x":1},"b":true}]`},
		{map[string]any{"trim": true}, "a\n 1 ", `[{"a":"1"}]`},
		{map[string]any{"header": false, "field": map[string]any{"names": []any{"x", "y"}}}, "1,2", `[{"x":"1","y":"2"}]`},
		{map[string]any{"string": map[string]any{"quote": "'"}}, "a\n'x''y'", `[{"a":"x'y"}]`},
	}
	for _, c := range cases {
		var want any
		if err := json.Unmarshal([]byte(c.want), &want); err != nil {
			t.Fatal(err)
		}
		got := outcomeOf(viaMake(t, c.opts), c.src)
		if !reflect.DeepEqual(got, makeOutcome{Value: want}) {
			t.Errorf("Make(%v) %q: got %#v, want %s", c.opts, c.src, got, c.want)
		}
		if plug := outcomeOf(viaPlugin(t, c.opts), c.src); !reflect.DeepEqual(got, plug) {
			t.Errorf("Make(%v) %q: got %#v, plugin gave %#v", c.opts, c.src, got, plug)
		}
	}
}

func TestMakeRejectionCodesMatchThePlugin(t *testing.T) {
	opts := map[string]any{"field": map[string]any{"exact": true}}
	for src, code := range map[string]string{
		"a,b\n1,2,3": "csv_extra_field",
		"a,b\n1":     "csv_missing_field",
	} {
		got := outcomeOf(viaMake(t, opts), src)
		if got.Error != code {
			t.Errorf("%q: got %#v, want code %s", src, got, code)
		}
		if plug := outcomeOf(viaPlugin(t, opts), src); !reflect.DeepEqual(got, plug) {
			t.Errorf("%q: Make gave %#v, plugin gave %#v", src, got, plug)
		}
	}
}

func TestMakeStream(t *testing.T) {
	var seen []string
	j := viaMake(t, map[string]any{
		"stream": func(what string, record any) {
			seen = append(seen, what)
		},
	})
	if _, err := j.Parse("a\n1\n2"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"start", "record", "record", "end"}; !reflect.DeepEqual(seen, want) {
		t.Fatalf("got %v, want %v", seen, want)
	}
}

// More than one map merges in order, a later key winning, and a nil map
// is skipped.
func TestMakeMergesSeveralOptionMaps(t *testing.T) {
	j := viaMake(t,
		map[string]any{"header": false, "object": false},
		nil,
		map[string]any{"object": true, "field": map[string]any{"names": []any{"x"}}},
	)
	got := outcomeOf(j, "1")
	want := outcomeOf(viaPlugin(t, map[string]any{
		"header": false, "object": true, "field": map[string]any{"names": []any{"x"}},
	}), "1")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, plugin gave %#v", got, want)
	}
	if exp := jsonFlatten([]any{map[string]any{"x": "1"}}); !reflect.DeepEqual(got.Value, exp) {
		t.Fatalf("got %#v, want %#v", got.Value, exp)
	}
}

// The plugin's own refusals come back as an error, not a panic.
func TestMakeReturnsThePluginsError(t *testing.T) {
	ring := make(optionRing, 1)
	ring[0] = ring
	j, err := Make(map[string]any{"field": map[string]any{"empty": ring}})
	if !errors.Is(err, ErrCyclicOption) || j != nil {
		t.Fatalf("got %v, %v; want ErrCyclicOption and no parser", j, err)
	}
}

func TestMakeFreshInstances(t *testing.T) {
	arrays := viaMake(t, map[string]any{"object": false})
	objects := viaMake(t)
	if arrays == objects {
		t.Fatal("Make returned the same instance twice")
	}
	if got := outcomeOf(arrays, "a\n1"); !reflect.DeepEqual(got.Value, jsonFlatten([]any{[]any{"1"}})) {
		t.Fatalf("arrays: %#v", got)
	}
	if got := outcomeOf(objects, "a\n1"); !reflect.DeepEqual(got.Value, jsonFlatten([]any{map[string]any{"a": "1"}})) {
		t.Fatalf("objects: %#v", got)
	}
}

func TestMakeEveryFixtureMatchesThePlugin(t *testing.T) {
	dir := fixturesDir()
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]fixtureEntry
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	judged := 0
	for key, entry := range manifest {
		// JsonicOpt sets ENGINE options between Make and Use, which Make
		// does not take: those fixtures are the plugin route's alone.
		if entry.JsonicOpt != nil {
			continue
		}
		file := entry.CsvFile
		if file == "" {
			file = key
		}
		raw, err := os.ReadFile(filepath.Join(dir, file+".csv"))
		if err != nil {
			t.Fatal(err)
		}
		got := outcomeOf(viaMake(t, entry.Opt), string(raw))
		if want := outcomeOf(viaPlugin(t, entry.Opt), string(raw)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: Make gave %#v, plugin gave %#v", entry.Name, got, want)
		}
		judged++
	}
	if judged <= 20 {
		t.Fatalf("judged only %d fixtures", judged)
	}
}
