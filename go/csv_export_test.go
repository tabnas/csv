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
