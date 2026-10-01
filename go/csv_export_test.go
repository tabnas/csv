package tabnascsv

import (
	"testing"

	jsonic "github.com/tabnas/jsonic/go"
)

// TestBuildCsvStringMatcherExported exercises the exported
// BuildCsvStringMatcher factory (TS: buildCsvStringMatcher) by
// registering the matcher on a plain jsonic instance and parsing a
// double-quote-escaped string.
func TestBuildCsvStringMatcherExported(t *testing.T) {
	j := jsonic.Make()
	j.SetOptions(jsonic.Options{Lex: &jsonic.LexOptions{
		Match: map[string]*jsonic.MatchSpec{
			"stringcsv": {Order: 1e5, Make: BuildCsvStringMatcher(map[string]any{
				"quote": `"`,
			})},
		},
	}})

	out, err := j.Parse(`"a""b"`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got, want := out, `a"b`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestBuildCsvStringMatcherHonoursAllowControl: a control character between
// the quotes is field text when the engine's String.AllowControl is on, as
// the plugin sets it, and refused when it is off, as it is on a plain
// instance. The shared rows in ../test/spec/quoted-control*.tsv cover the
// plugin. With the option on, `"a\tb""c"` reads through the doubled quote,
// which only this matcher does: the jsonic one would stop at it.
func TestBuildCsvStringMatcherHonoursAllowControl(t *testing.T) {
	makeQuoted := func(str *jsonic.StringOptions, quote string) *jsonic.Jsonic {
		j := jsonic.Make()
		j.SetOptions(jsonic.Options{
			String: str,
			Lex: &jsonic.LexOptions{
				Match: map[string]*jsonic.MatchSpec{
					"stringcsv": {Order: 1e5, Make: BuildCsvStringMatcher(map[string]any{
						"quote": quote,
					})},
				},
			},
		})
		return j
	}
	makeWith := func(str *jsonic.StringOptions) *jsonic.Jsonic {
		return makeQuoted(str, `"`)
	}

	// A quote that is itself a control character closes the field with
	// the option off too: it is the quote, not field text.
	if out, err := makeQuoted(nil, "\x1e").Parse("\x1ex y\x1e"); err != nil || out != "x y" {
		t.Fatalf("control-character quote: got %q, %v; want %q", out, err, "x y")
	}

	for _, str := range []*jsonic.StringOptions{nil, {AllowControl: boolPtr(false)}} {
		_, err := makeWith(str).Parse("\"a\tb\"")
		je, ok := err.(*jsonic.JsonicError)
		if !ok || je.Code != "unprintable" {
			t.Fatalf("allowControl off: got %v, want the unprintable error", err)
		}
	}

	on := &jsonic.StringOptions{AllowControl: boolPtr(true)}
	for src, want := range map[string]string{
		"\"a\tb\"\"c\"":    "a\tb\"c",
		"\"\x00\x1e\x1f\"": "\x00\x1e\x1f",
	} {
		out, err := makeWith(on).Parse(src)
		if err != nil {
			t.Fatalf("allowControl on, %q: parse error: %v", src, err)
		}
		if out != want {
			t.Fatalf("allowControl on, %q: got %q, want %q", src, out, want)
		}
	}
}
