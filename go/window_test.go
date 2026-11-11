package tabnascsv

// A parse from a reader (Tabnas.ParseReader, admin ADR-30 and ADR-33).
//
// Make declares lex.window, so that a reader is read a window at a time.
// The quoted field, the one token this grammar cuts itself, reads the
// source only through the window, and jsonic's base reads it through the
// window too. These tests hold it to that: Make declares the window and
// the plugin alone does not; every row of the shared fixtures, every
// committed fixture and every document of the two vendored suites parses
// the same from a reader, in pieces of 1, 3, 64 and 4096 bytes, as from
// its text; quoted fields longer than the window, with what the matcher
// looks for shifted across the window's edge a byte at a time, parse the
// same; and so does a long document, with an error at its end.
//
// Go's window sizes are fixed at 64 KiB (the engine's small-window hook is
// unexported), and before each token the window reaches 64 KiB past the
// cursor, so a token meets the window's edge only where it is longer than
// that.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"
	tabnas "github.com/tabnas/parser/go"
	support "github.com/tabnas/support/go"
)

// readerMargin is how far past the cursor the window reaches before each
// token is cut: a token longer than this meets the window's edge.
const readerMargin = 64 << 10

// pieces is a reader that hands its bytes over at most size at a time.
type pieces struct {
	text []byte
	at   int
	size int
}

func (p *pieces) Read(buf []byte) (int, error) {
	if p.at == len(p.text) {
		return 0, io.EOF
	}
	n := min(p.size, len(buf), len(p.text)-p.at)
	copy(buf, p.text[p.at:p.at+n])
	p.at += n
	return n, nil
}

// windowed sets lex.window on j, as Make sets it.
func windowed(j *tabnas.Tabnas) *tabnas.Tabnas {
	yes := true
	j.SetOptions(tabnas.Options{Lex: &tabnas.LexOptions{Window: &yes}})
	return j
}

// readerOutcome is what a parse shows: its value, or its error's code,
// position, row, column and token text. An error's excerpt is left out: a
// parse from a reader shows a long line only as far back as the window
// reaches (tabnas/parser DIVERGENCE.md), the one difference allowed.
type readerOutcome struct {
	value any
	err   string
}

func readerOutcomeOf(v any, err error) readerOutcome {
	if err == nil {
		return readerOutcome{value: v}
	}
	var te *tabnas.TabnasError
	if errors.As(err, &te) {
		return readerOutcome{err: fmt.Sprintf("%s at %d %d:%d %q", te.Code, te.Pos, te.Row, te.Col, te.Src)}
	}
	return readerOutcome{err: "not a TabnasError: " + err.Error()}
}

func (o readerOutcome) String() string {
	if o.err != "" {
		if len(o.err) > 200 {
			return "error " + o.err[:200] + "..."
		}
		return "error " + o.err
	}
	s := fmt.Sprintf("value %v", o.value)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

func (o readerOutcome) same(p readerOutcome) bool {
	return o.err == p.err && reflect.DeepEqual(o.value, p.value)
}

// sameFromReader parses text whole and from a reader in pieces of each
// size, and reports where the two differ. It returns the number of reader
// parses compared.
func sameFromReader(t *testing.T, j *tabnas.Tabnas, at, text string, sizes []int) int {
	t.Helper()
	whole := readerOutcomeOf(j.Parse(text))
	for _, size := range sizes {
		read := readerOutcomeOf(j.ParseReader(&pieces{text: []byte(text), size: size}))
		if !read.same(whole) {
			t.Errorf("%s in pieces of %d:\n  whole:  %s\n  reader: %s", at, size, whole, read)
		}
	}
	return len(sizes)
}

// readerSizes are the pieces a document is handed over in: down to a byte
// for a short one, and from 64 bytes for one over 8 KiB, whose every
// one-byte read the Go window answers with a copy of itself
// (tabnas/parser#321).
func readerSizes(text string) []int {
	if len(text) > 8<<10 {
		return []int{64, 4096, readerMargin + 3}
	}
	return []int{1, 3, 64, 4096}
}

// TestMakeDeclaresTheWindow: Make declares lex.window; the plugin alone,
// which a host may install beneath matchers of its own, declares nothing.
func TestMakeDeclaresTheWindow(t *testing.T) {
	j, err := Make()
	if err != nil {
		t.Fatal(err)
	}
	if !j.Config().Window {
		t.Error("Make does not declare lex.window")
	}
	// jsonic's base declares the window itself (tabnas/jsonic#121), so the
	// plugin goes on that base with the window turned off.
	no := false
	plain := jsonic.Make(tabnas.Options{Lex: &tabnas.LexOptions{Window: &no}})
	if err := plain.UseDefaults(Csv, Defaults); err != nil {
		t.Fatal(err)
	}
	if plain.Config().Window {
		t.Error("the Csv plugin declares lex.window")
	}
}

// TestSpecFromAReader: every row of the shared fixtures parses the same
// from a reader as from its text, under Make with the row's options.
func TestSpecFromAReader(t *testing.T) {
	dir, err := support.FindSpecDir("")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.tsv"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures in %s: %v", dir, err)
	}
	compared := 0
	for _, path := range paths {
		spec, err := support.LoadSpec(path, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range spec.Rows {
			opts := map[string]any{}
			if raw := row.Named("opts"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &opts); err != nil {
					t.Fatalf("%s:%d: opts: %v", filepath.Base(path), row.Line, err)
				}
			}
			j, err := Make(opts)
			if err != nil {
				t.Fatalf("%s:%d: %v", filepath.Base(path), row.Line, err)
			}
			input := support.Unescape(row.Named("input"))
			at := fmt.Sprintf("%s:%d", filepath.Base(path), row.Line)
			compared += sameFromReader(t, j, at, input, readerSizes(input))
		}
	}
	t.Logf("reader spec: %d reader parses (100%%)", compared)
	if compared < 400 {
		t.Errorf("only %d reader parses", compared)
	}
}

// TestFixturesFromAReader: every committed fixture parses the same from a
// reader as from its text, under the parser its options build.
func TestFixturesFromAReader(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixturesDir(), "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]fixtureEntry
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(manifest))
	for key := range manifest {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	compared := 0
	for _, key := range keys {
		entry := manifest[key]
		file := entry.CsvFile
		if file == "" {
			file = key
		}
		b, err := os.ReadFile(filepath.Join(fixturesDir(), file+".csv"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		compared += sameFromReader(t, windowed(fixtureParser(entry.Opt, entry.JsonicOpt)), key, text, readerSizes(text))
	}
	t.Logf("reader fixtures: %d of %d (100%%), %d reader parses", len(keys), len(keys), compared)
}

// TestSuitesFromAReader: every csv-spectrum document and every
// go/encoding/csv case parses the same from a reader as from its text.
func TestSuitesFromAReader(t *testing.T) {
	csvDir, _ := spectrumDirs(t)
	names, err := filepath.Glob(filepath.Join(csvDir, "*.csv"))
	if err != nil || len(names) == 0 {
		t.Fatalf("no csv-spectrum documents in %s: %v", csvDir, err)
	}
	compared := 0
	for _, name := range names {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		j, err := Make()
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		compared += sameFromReader(t, j, "csv-spectrum "+filepath.Base(name), text, readerSizes(text))
	}
	corpus := loadGoCorpus(t)
	for _, c := range corpus.Cases {
		j := windowed(fixtureParser(c.Opts, c.JsonicOpt))
		compared += sameFromReader(t, j, "go/encoding/csv "+c.Name, c.Input, readerSizes(c.Input))
	}
	t.Logf("reader suites: %d documents (100%%), %d reader parses", len(names)+len(corpus.Cases), compared)
}

// TestLongFieldsFromAReader: quoted fields longer than the window, with
// what the matcher looks for (a closing quote, a doubled quote, a line
// break, the source ending inside the field) shifted across the window's
// edge a byte at a time, parse the same from a reader as from their text.
// Each document begins with its field, so the edge lies readerMargin bytes
// past the opening quote in pieces of 4096 bytes, which divide it, and 3
// bytes further in pieces of readerMargin+3.
func TestLongFieldsFromAReader(t *testing.T) {
	pad := func(n int) string { return strings.Repeat("a", n) }
	var docs []string
	for shift := -12; shift <= 12; shift++ {
		edge := readerMargin + shift
		docs = append(docs,
			`"`+pad(edge-1)+`",b`+"\n",
			`"`+pad(edge-2)+`""b",c`+"\n",
			`"`+pad(edge-1)+`"""`+"\n",
			`"`+pad(edge-1)+"\nb\",c\n",
			`"`+pad(edge-1)+"\r\nb\",c\n",
			`"`+pad(edge-1)+"\tb\",c\n",
			`"`+pad(edge),
			`"`+pad(edge-1)+`"x,c`+"\n",
			pad(edge+5)+",b\n",
		)
	}
	j, err := Make(map[string]any{"header": false, "object": false})
	if err != nil {
		t.Fatal(err)
	}
	compared := 0
	for n, doc := range docs {
		at := fmt.Sprintf("document %d (%q...%q, %d bytes)", n, doc[:2], doc[len(doc)-4:], len(doc))
		compared += sameFromReader(t, j, at, doc, []int{4096, readerMargin + 3})
		if (n+1)%50 == 0 || n+1 == len(docs) {
			t.Logf("reader long fields: %d of %d documents (%d%%), %d reader parses",
				n+1, len(docs), (n+1)*100/len(docs), compared)
		}
	}
}

// TestLongDocumentFromAReader: a document of over a megabyte, its records'
// quoted fields carrying doubled quotes and line breaks, two of its fields
// longer than the window, parses the same from a reader as from its text,
// and so does the same document with a field left open at its end, which a
// parse from a reader places where the whole text does after the window
// has moved on many times. Every quoted field the matcher reads after the
// first window move tests its look back at the separator before the quote.
func TestLongDocumentFromAReader(t *testing.T) {
	long := readerMargin + 2<<10
	var b strings.Builder
	b.WriteString("id,name,note\n")
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&b, "%d,\"ñ%d \"\"q\"\"\",\"line\nbreak, %d\"\n", i, i, i)
		if i == 7000 || i == 14000 {
			fmt.Fprintf(&b, "x,\"%s\",%s\n", strings.Repeat("s \"\"q\"\"\n", long/9), strings.Repeat("t", long))
		}
	}
	text := b.String()
	broken := text + "z,\"open"
	if len(text) < 1<<20 {
		t.Fatalf("the document is %d bytes", len(text))
	}
	j, err := Make()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.Parse(text); err != nil {
		t.Fatalf("the long document does not parse: %v", err)
	}
	if _, err := j.Parse(broken); err == nil || err.(*tabnas.TabnasError).Code != "unterminated_string" {
		t.Fatalf("the broken document is not unterminated_string: %v", err)
	}
	sizes := []int{64, 4096, readerMargin + 3}
	sameFromReader(t, j, "the long document", text, sizes)
	t.Logf("reader long document: 1 of 2 (50%%), %d bytes", len(text))
	sameFromReader(t, j, "the long document with a field left open", broken, sizes)
	t.Logf("reader long document: 2 of 2 (100%%)")
}
