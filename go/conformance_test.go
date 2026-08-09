// Copyright (c) 2026 Richard Rodger and other contributors, MIT License

package tabnascsv

// conformance_test.go — third-party conformance corpora, the Go half of the
// pair. ts/test/conformance.test.ts runs the SAME two corpora with the SAME
// divergence table, so TypeScript and Go cannot drift without one of them
// going red.
//
//   valid   -> must parse AND produce the corpus's expected VALUE
//   invalid -> must be REJECTED with an error
//
// The corpora are NOT committed. `scripts/fetch-csv-suites.sh` fetches them at
// pinned upstream commits into `test/suites/`, which is gitignored. `go test`
// has no `pretest` hook the way npm does, so this file runs that script itself
// (once per process, and only when a corpus is actually absent) rather than
// relying on the caller to have run `make test` first. That is what makes the
// suite RUN in CI, where the go job is a bare `go test ./...` over a fresh
// checkout.
//
// If the fetch cannot happen — no network, no `node` for the case extractor —
// the affected suite FAILS LOUDLY with the fetch command in the message. It
// never skips. A conformance suite that quietly does not run reports green
// while measuring nothing, which is worse than having no suite at all; the
// TypeScript half throws for the same reason. Once a corpus is present, every
// case in it is judged and nothing is silently exempt.
//
// On DIVERGENCES: go/encoding/csv is a strict RFC 4180 reader. @tabnas/csv is
// deliberately a lenient, PapaParse-compatible reader with RFC 4180 quoting
// (see AGENTS.md "Conformance"). Where the two differ ON PURPOSE, the case is
// NOT skipped — the @tabnas/csv result is pinned as a positive assertion, AND
// the test re-checks that the case really still disagrees with the corpus.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"
)

func suitesDir() string { return filepath.Join("..", "test", "suites") }

const conformanceMissing = "conformance corpus missing under ../test/suites, " +
	"and scripts/fetch-csv-suites.sh could not supply it (it needs network, " +
	"and node for the go/encoding/csv case extractor). Run that script by " +
	"hand — or `make test` — to judge this suite."

var fetchOnce sync.Once
var fetchErr error

// fetchCorpora runs the pinned-commit fetch script, at most once per test
// process. The script is idempotent and returns immediately when a corpus is
// already present, so calling it costs nothing on a warm tree; it is invoked
// only from the miss path anyway.
func fetchCorpora() error {
	fetchOnce.Do(func() {
		cmd := exec.Command("bash", filepath.Join("..", "scripts", "fetch-csv-suites.sh"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			fetchErr = fmt.Errorf("%w\n%s", err, out)
		}
	})
	return fetchErr
}

// requireCorpus makes `path` present or FAILS the test. It fetches the corpora
// first if the path is absent, and calls t.Fatalf if it is still absent after
// that. It never calls t.Skip: "the corpus could not be obtained" would leave
// the suite reporting green while judging nothing, so it is a failure here,
// exactly as it is in the TypeScript half.
func requireCorpus(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		return
	}
	if err := fetchCorpora(); err != nil {
		t.Fatalf("%s\n  expected: %s\n  fetch failed: %v", conformanceMissing, path, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s\n  expected: %s", conformanceMissing, path)
	}
}

// jsonOf renders a parse result in the same canonical form as the corpus
// expectations: Go's encoder sorts map keys, and the corpus expectations are
// decoded and re-encoded the same way, so comparison is order-insensitive.
func jsonOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "<unmarshalable:" + err.Error() + ">"
	}
	return string(b)
}

func canonJSON(raw []byte) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "<bad-json:" + err.Error() + ">"
	}
	return jsonOf(v)
}

func show(s string) string {
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

// --------------------------------------------------------------------------
// SUITE 1 — max-mapper/csv-spectrum @ d30e80f8b99d2eecb3778f1d7b9ed1cb425502ec
// --------------------------------------------------------------------------

// One csv-spectrum case is internally inconsistent UPSTREAM: its .json is a
// bare object where all 11 others are arrays of records, and its phone number
// was scrubbed in the .json but not in the .csv. It cannot judge any parser,
// so it is pinned by TestConformanceSpectrumUpstreamDefect instead.
const spectrumUpstreamDefect = "location_coordinates"

func spectrumDirs(t *testing.T) (string, string) {
	t.Helper()
	dir := filepath.Join(suitesDir(), "csv-spectrum")
	csvDir := filepath.Join(dir, "csvs")
	jsonDir := filepath.Join(dir, "json")
	for _, d := range []string{dir, csvDir, jsonDir} {
		requireCorpus(t, d)
	}
	return csvDir, jsonDir
}

func TestConformanceSpectrum(t *testing.T) {
	csvDir, jsonDir := spectrumDirs(t)

	entries, err := os.ReadDir(csvDir)
	if err != nil {
		t.Fatalf("%s\n  %v", conformanceMissing, err)
	}

	names := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".csv") {
			names = append(names, strings.TrimSuffix(e.Name(), ".csv"))
		}
	}
	sort.Strings(names)
	if 0 == len(names) {
		t.Fatalf("%s\n  found no .csv documents in %s", conformanceMissing, csvDir)
	}

	failures := []string{}
	judged := 0

	for _, name := range names {
		if spectrumUpstreamDefect == name {
			continue // judged by TestConformanceSpectrumUpstreamDefect
		}
		judged++

		src, err := os.ReadFile(filepath.Join(csvDir, name+".csv"))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		want, err := os.ReadFile(filepath.Join(jsonDir, name+".json"))
		if err != nil {
			t.Fatalf("read %s.json: %v", name, err)
		}

		j := jsonic.Make()
		j.UseDefaults(Csv, Defaults)
		res, perr := j.Parse(string(src))
		if perr != nil {
			failures = append(failures, name+": threw "+perr.Error())
			continue
		}

		got := jsonOf(normalizeValue(res))
		exp := canonJSON(want)
		if got != exp {
			failures = append(failures,
				name+": expected "+show(exp)+" got "+show(got))
		}
	}

	if judged != len(names)-1 {
		t.Fatalf("every corpus document must be judged: %d of %d", judged, len(names))
	}
	report(t, "csv-spectrum valid", judged, failures)
}

func TestConformanceSpectrumUpstreamDefect(t *testing.T) {
	csvDir, jsonDir := spectrumDirs(t)

	src, err := os.ReadFile(filepath.Join(csvDir, spectrumUpstreamDefect+".csv"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(jsonDir, spectrumUpstreamDefect+".json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var expected any
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	obj, isObj := expected.(map[string]any)
	if !isObj {
		t.Fatalf("upstream csv-spectrum fixed the shape of %s.json — delete "+
			"spectrumUpstreamDefect and judge this case normally",
			spectrumUpstreamDefect)
	}
	if obj["Contact Phone Number"] != "1234567890" {
		t.Fatalf("upstream csv-spectrum changed the scrubbed phone number — re-check")
	}
	if !strings.Contains(string(src), "2095257564") {
		t.Fatalf("upstream csv-spectrum changed %s.csv — re-check", spectrumUpstreamDefect)
	}

	// What @tabnas/csv actually does with it, pinned so a regression shows up:
	// the correct reading, one record, faithful to the .csv.
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults)
	res, perr := j.Parse(string(src))
	if perr != nil {
		t.Fatalf("parse failed: %v", perr)
	}
	got := jsonOf(normalizeValue(res))
	want := `[{"Cities":"Modesto","Contact Phone Number":"2095257564",` +
		`"Counties":"Stanislaus","Location Coordinates":"37�36'37.8\"N 121�2'17.9\"W"}]`
	if got != want {
		t.Errorf("location_coordinates:\n got  %s\n want %s", got, want)
	}
}

// --------------------------------------------------------------------------
// SUITE 2 — golang/go src/encoding/csv @ 3901409b5d0fb7c85a3e6730a59943cc93b2835c
// --------------------------------------------------------------------------

type goCase struct {
	Name      string         `json:"name"`
	Input     string         `json:"input"`
	MustFail  bool           `json:"mustFail"`
	Expected  any            `json:"expected"`
	Opts      map[string]any `json:"opts"`
	JsonicOpt map[string]any `json:"jsonicOpts"`
	Profile   string         `json:"profile"`
	Notes     []string       `json:"notes"`
}

type goCorpus struct {
	Cases    []goCase `json:"cases"`
	Excluded []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"excluded"`
}

type divergence struct {
	why string
	// Canonical JSON of what @tabnas/csv actually produces, or "ERROR".
	result string
}

// Deliberate, documented departures from go/encoding/csv. Each entry pins what
// @tabnas/csv actually produces, so the behaviour is asserted rather than
// waived. Keep in step with AGENTS.md "Conformance" and the DIVERGENCES table
// in ts/test/conformance.test.ts.
var divergences = map[string]divergence{
	// (1) A bare CR is a record separator (PapaParse-compatible, and what
	// `record.separators: null` documents: "\n / \r\n / \r"). go/encoding/csv
	// treats a CR not followed by LF as ordinary field data. Pinned by the
	// committed fixture test/fixtures/papa-two-rows-just-r.csv.
	"BareCR":               {"bare-CR-is-a-record-separator", `[["a","b"],["c","d"]]`},
	"FieldCR":              {"bare-CR-is-a-record-separator", `[["field"],["field"]]`},
	"FieldCRCR":            {"bare-CR-is-a-record-separator", `[["field"],["field"]]`},
	"FieldCRCRLF":          {"bare-CR-is-a-record-separator", `[["field"],["field"]]`},
	"FieldCRCRLFCR":        {"bare-CR-is-a-record-separator", `[["field"],["field"]]`},
	"FieldCRCRLFCRCR":      {"bare-CR-is-a-record-separator", `[["field"],["field"]]`},
	"MultiFieldCRCRLFCRCR": {"bare-CR-is-a-record-separator", `[["field1","field2"],["field1","field2"],["",""]]`},
	"QuotedTrailingCRCR":   {"bare-CR-is-a-record-separator", `[["field"]]`},

	// (2) A CRLF inside a quoted field is field data and survives verbatim.
	// go/encoding/csv normalises it to a bare LF, which is a Go convenience,
	// not an RFC 4180 requirement (RFC 4180 §2.6: CRLF inside quotes is data).
	"CRLFInQuotedField": {"quoted-CRLF-is-preserved-verbatim", `[["A","Hello\r\nHi","B"]]`},

	// (3) Stray quotes inside an unquoted field are ordinary text, not an
	// error (PapaParse-compatible lenience). go/encoding/csv rejects them with
	// ErrBareQuote. Pinned by test/fixtures/papa-unquoted-field-with-quotes-*.
	"BadDoubleQuotes":  {"stray-quotes-are-literal-text", `[["a\"\"b","c"]]`},
	"BadBareQuote":     {"stray-quotes-are-literal-text", `[["a \"word\"","b"]]`},
	"BadTrailingQuote": {"stray-quotes-are-literal-text", `[["a word","b\""]]`},

	// (4) `trim` trims surrounding whitespace but does NOT then re-read the
	// remainder as a quoted field, so ` "a"` is the three-character text `"a"`.
	// Go's TrimLeadingSpace trims first and unquotes after. Pinned by
	// test/fixtures/papa-quoted-field-with-whitespace-around-quotes.csv.
	"TrimQuote": {"whitespace-then-quote-is-literal-text", `[["\"a\""," b","c"]]`},

	// (5) `field.exact` is header-relative by documentation ("error when a
	// record's field count differs from the header's" — go/doc/reference.md).
	// These cases run with `header: false` and no `field.names`, so there is no
	// expected count and the option is correctly inert. Go's FieldsPerRecord
	// needs no header; @tabnas/csv has no equivalent uniform-count mode.
	"BadFieldCount":         {"field.exact-is-header-relative", `[["a","b","c"],["d","e"]]`},
	"BadFieldCountMultiple": {"field.exact-is-header-relative", `[["a","b","c"],["d","e"],["f"]]`},
	"BadFieldCount1":        {"field.exact-is-header-relative", `[["a","b","c"]]`},
}

// conformanceScore is the number of go/encoding/csv cases @tabnas/csv matches
// outright. Asserted so the headline number in AGENTS.md/README cannot drift.
const conformanceScore = 39

func loadGoCorpus(t *testing.T) goCorpus {
	t.Helper()
	file := filepath.Join(suitesDir(), "go-encoding-csv", "cases.json")
	requireCorpus(t, file)
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("corpus at %s is not readable: %v", file, err)
	}
	var corpus goCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("corpus at %s is not readable: %v", file, err)
	}
	if 0 == len(corpus.Cases) {
		t.Fatalf("%s\n  corpus at %s has no cases", conformanceMissing, file)
	}
	return corpus
}

// runGoCase parses a corpus case and returns its canonical JSON, or "ERROR".
func runGoCase(c goCase) string {
	res, err := parseFixture(c.Input, c.Opts, c.JsonicOpt)
	if err != nil {
		return "ERROR"
	}
	return jsonOf(normalizeResult(res))
}

func report(t *testing.T, label string, total int, failures []string) {
	t.Helper()
	if 0 == len(failures) {
		t.Logf("%s: %d/%d passed", label, total, total)
		return
	}
	t.Errorf("%s: %d/%d passed. FAILING (%d):\n  - %s",
		label, total-len(failures), total, len(failures),
		strings.Join(failures, "\n  - "))
}

func TestConformanceGoEncodingCsvValid(t *testing.T) {
	corpus := loadGoCorpus(t)
	failures := []string{}
	total := 0

	for _, c := range corpus.Cases {
		if c.MustFail {
			continue
		}
		total++
		got := runGoCase(c)
		want := jsonOf(c.Expected)

		if d, isDiv := divergences[c.Name]; isDiv {
			if got != d.result {
				failures = append(failures, c.Name+" [divergence "+d.why+
					"]: pinned "+show(d.result)+" but got "+show(got)+
					" — update divergences or fix the regression")
			} else if got == want {
				failures = append(failures, c.Name+
					": listed as a divergence but now MATCHES the corpus — "+
					"delete the divergences entry")
			}
			continue
		}

		if got != want {
			failures = append(failures, c.Name+" ["+c.Profile+"]: expected "+
				show(want)+" got "+show(got))
		}
	}

	report(t, "go/encoding/csv valid", total, failures)
}

func TestConformanceGoEncodingCsvInvalid(t *testing.T) {
	corpus := loadGoCorpus(t)
	failures := []string{}
	total := 0

	for _, c := range corpus.Cases {
		if !c.MustFail {
			continue
		}
		total++
		got := runGoCase(c)

		if d, isDiv := divergences[c.Name]; isDiv {
			if got != d.result {
				failures = append(failures, c.Name+" [divergence "+d.why+
					"]: pinned "+show(d.result)+" but got "+show(got)+
					" — update divergences or fix the regression")
			} else if "ERROR" == got {
				failures = append(failures, c.Name+
					": listed as a divergence but is now REJECTED like the "+
					"corpus requires — delete the divergences entry")
			}
			continue
		}

		if "ERROR" != got {
			failures = append(failures, c.Name+" ["+c.Profile+"]: was ACCEPTED as "+
				show(got)+" but RFC 4180 / encoding/csv rejects it")
		}
	}

	report(t, "go/encoding/csv invalid-rejected", total, failures)
}

func TestConformanceScore(t *testing.T) {
	corpus := loadGoCorpus(t)
	known := map[string]bool{}
	for _, c := range corpus.Cases {
		known[c.Name] = true
	}
	for name := range divergences {
		if !known[name] {
			t.Errorf("divergences names a case not in the corpus: %s", name)
		}
	}
	if got := len(corpus.Cases) - len(divergences); got != conformanceScore {
		t.Errorf("documented in AGENTS.md as %d/%d go/encoding/csv cases "+
			"conforming, measured %d/%d — update AGENTS.md and README.md "+
			"together with this number",
			conformanceScore, len(corpus.Cases), got, len(corpus.Cases))
	}
	if 16 != len(divergences) {
		t.Errorf("AGENTS.md documents 16 divergences, table has %d", len(divergences))
	}
}

func TestConformanceExcludedSet(t *testing.T) {
	corpus := loadGoCorpus(t)
	got := []string{}
	for _, e := range corpus.Excluded {
		got = append(got, e.Name)
	}
	sort.Strings(got)

	want := []string{
		// LazyQuotes: a deliberately non-RFC-4180 lenient mode with no
		// @tabnas/csv equivalent, so there is no behaviour to assert.
		"BareDoubleQuotes",
		"BareQuotes",
		"LazyOddQuotes",
		"LazyQuoteWithTrailingCRLF",
		"LazyQuotes",
		// No Input at all: these assert that Go's NewReader rejects a bad
		// Comma/Comment *rune*. An API validation test, not a document.
		"BadComma1",
		"BadComma2",
		"BadComma3",
		"BadComma4",
		"BadCommaComment",
		"BadComment1",
		"BadComment2",
		"BadComment3",
	}
	sort.Strings(want)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("excluded set drifted:\n got  %v\n want %v", got, want)
	}
}
