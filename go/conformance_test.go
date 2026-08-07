package tabnascsv

// Third-party conformance corpora, run against the documented Go stack
// (jsonic.Make() + UseDefaults(Csv, Defaults)), exercising BOTH halves:
//
//	valid   -> must parse AND produce the corpus's expected VALUE
//	invalid -> must be REJECTED with an error
//
// The corpora are NOT committed. scripts/fetch-csv-suites.sh fetches them at
// pinned upstream commits into test/suites/, which is gitignored. This file
// runs that script itself when the corpus is absent, and FAILS LOUDLY if the
// corpus is still missing afterwards. It must never skip: a conformance test
// that quietly does not run is worse than no test at all.
//
// ts/test/conformance.test.ts runs the same two corpora, so the two runtimes
// cannot drift without one of them going red.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func suitesDir() string { return filepath.Join("..", "test", "suites") }

// ensureCorpus makes the corpus present or fails the test loudly. It never
// calls t.Skip.
func ensureCorpus(t *testing.T, probe string) {
	t.Helper()
	if _, err := os.Stat(probe); err == nil {
		return
	}

	script := filepath.Join("..", "scripts", "fetch-csv-suites.sh")
	cmd := exec.Command("bash", script)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()

	if _, err := os.Stat(probe); err != nil {
		t.Fatalf(
			"CONFORMANCE CORPUS MISSING: %s\n"+
				"  scripts/fetch-csv-suites.sh was run automatically and did not produce it (%v).\n"+
				"  Run it by hand and re-run the tests. This test must never skip.",
			probe, runErr)
	}
}

// canon renders a value through JSON so the comparison is representation
// independent (OrderedMap, map[string]any and []any all normalise).
func canon(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<unmarshalable %v>", v)
	}
	return string(b)
}

func short(s string) string {
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

// reportConformance turns a list of failures into one loud assertion carrying
// the dial reading, rather than stopping at the first red case.
func reportConformance(t *testing.T, label string, total int, failures []string) {
	t.Helper()
	if len(failures) == 0 {
		t.Logf("%s: %d/%d passed", label, total, total)
		return
	}
	t.Errorf("%s: %d/%d passed. FAILING (%d):\n  - %s",
		label, total-len(failures), total, len(failures),
		strings.Join(failures, "\n  - "))
}

// ---------------------------------------------------------------------------
// SUITE 1 - max-mapper/csv-spectrum @ d30e80f8b99d2eecb3778f1d7b9ed1cb425502ec
// Valid documents only; the corpus has no must-fail half.
// ---------------------------------------------------------------------------

// spectrumUpstreamDefect is judged by TestCsvSpectrumUpstreamDefect instead of
// by the main loop, because the upstream expectation contradicts its own
// input. See that test for the full reasoning.
const spectrumUpstreamDefect = "location_coordinates"

func TestCsvSpectrum(t *testing.T) {
	dir := filepath.Join(suitesDir(), "csv-spectrum")
	csvDir := filepath.Join(dir, "csvs")
	ensureCorpus(t, filepath.Join(csvDir, "simple.csv"))

	entries, err := os.ReadDir(csvDir)
	if err != nil {
		t.Fatalf("cannot read %s: %v", csvDir, err)
	}

	names := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".csv") {
			names = append(names, strings.TrimSuffix(e.Name(), ".csv"))
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatalf("CONFORMANCE CORPUS EMPTY: no .csv documents under %s", csvDir)
	}

	failures := []string{}
	judged := 0
	for _, name := range names {
		if name == spectrumUpstreamDefect {
			continue
		}
		judged++

		src, err := os.ReadFile(filepath.Join(csvDir, name+".csv"))
		if err != nil {
			t.Fatalf("cannot read %s: %v", name, err)
		}
		expRaw, err := os.ReadFile(filepath.Join(dir, "json", name+".json"))
		if err != nil {
			t.Fatalf("cannot read expected json for %s: %v", name, err)
		}
		var expected any
		if err := json.Unmarshal(expRaw, &expected); err != nil {
			t.Fatalf("bad expected json for %s: %v", name, err)
		}

		actual, err := csvParse(string(src))
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: threw %v", name, err))
			continue
		}
		if canon(actual) != canon(expected) {
			failures = append(failures, fmt.Sprintf("%s: expected %s got %s",
				name, short(canon(expected)), short(canon(actual))))
		}
	}

	if judged != len(names)-1 {
		t.Fatalf("every corpus document must be judged: judged %d of %d", judged, len(names))
	}
	reportConformance(t, "csv-spectrum valid", judged, failures)
}

// csv-spectrum's location_coordinates expectation contradicts its own input in
// two independent ways, so it cannot judge any parser:
//
//  1. json/location_coordinates.json is a bare OBJECT; all 11 other
//     expectations are ARRAYS of record objects, and the .csv is a header row
//     plus one data row.
//  2. The expected "Contact Phone Number" is "1234567890"; the .csv says
//     "2095257564" - the JSON was scrubbed of a real number and the CSV was not.
//
// The defect is PINNED rather than excluded: if upstream fixes either half,
// this test goes red and the case moves into the judged set above.
func TestCsvSpectrumUpstreamDefect(t *testing.T) {
	dir := filepath.Join(suitesDir(), "csv-spectrum")
	csvPath := filepath.Join(dir, "csvs", spectrumUpstreamDefect+".csv")
	ensureCorpus(t, csvPath)

	src, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v", csvPath, err)
	}
	expRaw, err := os.ReadFile(filepath.Join(dir, "json", spectrumUpstreamDefect+".json"))
	if err != nil {
		t.Fatalf("cannot read expected json: %v", err)
	}

	var expected any
	if err := json.Unmarshal(expRaw, &expected); err != nil {
		t.Fatalf("bad expected json: %v", err)
	}
	obj, isObj := expected.(map[string]any)
	if !isObj {
		t.Fatalf("upstream csv-spectrum fixed the shape of %s.json - delete "+
			"spectrumUpstreamDefect and judge this case normally", spectrumUpstreamDefect)
	}
	if obj["Contact Phone Number"] != "1234567890" {
		t.Fatalf("upstream csv-spectrum changed the scrubbed phone number - re-check")
	}
	if !strings.Contains(string(src), "2095257564") {
		t.Fatalf("upstream csv-spectrum changed %s.csv - re-check", spectrumUpstreamDefect)
	}

	// What @tabnas/csv actually does with it, pinned so a regression shows up:
	// the correct reading, one record, faithful to the .csv.
	actual, err := csvParse(string(src))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(actual) != 1 {
		t.Fatalf("expected 1 record, got %d: %s", len(actual), canon(actual))
	}
	m := toMap(actual[0])
	if m["Contact Phone Number"] != "2095257564" {
		t.Errorf("expected phone 2095257564, got %v", m["Contact Phone Number"])
	}
	if m["Cities"] != "Modesto" || m["Counties"] != "Stanislaus" {
		t.Errorf("unexpected record: %s", canon(actual))
	}
}

// ---------------------------------------------------------------------------
// SUITE 2 - golang/go src/encoding/csv/reader_test.go @ go1.24.0
// The reference RFC 4180 implementation's own conformance table. Supplies the
// must-fail half that csv-spectrum lacks.
// ---------------------------------------------------------------------------

type goCsvCase struct {
	Name       string         `json:"name"`
	Input      string         `json:"input"`
	MustFail   bool           `json:"mustFail"`
	Expected   any            `json:"expected"`
	Opts       map[string]any `json:"opts"`
	JsonicOpts map[string]any `json:"jsonicOpts"`
	Profile    string         `json:"profile"`
	Notes      []string       `json:"notes"`
}

type goCsvCorpus struct {
	Cases    []goCsvCase `json:"cases"`
	Excluded []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"excluded"`
}

func loadGoCorpus(t *testing.T) goCsvCorpus {
	t.Helper()
	path := filepath.Join(suitesDir(), "go-encoding-csv", "cases.json")
	ensureCorpus(t, path)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}
	var corpus goCsvCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("bad corpus json at %s: %v", path, err)
	}
	if len(corpus.Cases) == 0 {
		t.Fatalf("CONFORMANCE CORPUS EMPTY: %s has no cases", path)
	}
	return corpus
}

func TestGoEncodingCsvValid(t *testing.T) {
	corpus := loadGoCorpus(t)

	failures := []string{}
	total := 0
	for _, c := range corpus.Cases {
		if c.MustFail {
			continue
		}
		total++
		actual, err := parseFixture(c.Input, c.Opts, c.JsonicOpts)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s [%s]: threw %v", c.Name, c.Profile, err))
			continue
		}
		if canon(actual) != canon(c.Expected) {
			failures = append(failures, fmt.Sprintf(
				"%s [%s]: input %s expected %s got %s",
				c.Name, c.Profile, short(canon(c.Input)),
				short(canon(c.Expected)), short(canon(actual))))
		}
	}
	reportConformance(t, "go/encoding/csv valid", total, failures)
}

func TestGoEncodingCsvInvalid(t *testing.T) {
	corpus := loadGoCorpus(t)

	failures := []string{}
	total := 0
	for _, c := range corpus.Cases {
		if !c.MustFail {
			continue
		}
		total++
		actual, err := parseFixture(c.Input, c.Opts, c.JsonicOpts)
		if err != nil {
			continue // rejected, as required
		}
		failures = append(failures, fmt.Sprintf(
			"%s [%s]: input %s was ACCEPTED as %s but RFC 4180 / encoding/csv rejects it",
			c.Name, c.Profile, short(canon(c.Input)), short(canon(actual))))
	}
	reportConformance(t, "go/encoding/csv invalid-rejected", total, failures)
}

// The excluded set is asserted, not merely documented, so a future change to
// the extractor cannot quietly grow it.
func TestGoEncodingCsvExcluded(t *testing.T) {
	corpus := loadGoCorpus(t)

	got := []string{}
	for _, e := range corpus.Excluded {
		got = append(got, e.Name)
	}
	sort.Strings(got)

	want := []string{
		// LazyQuotes: a deliberately non-RFC-4180 lenient mode with no
		// @tabnas/csv equivalent, so there is no behaviour to assert.
		"BareDoubleQuotes", "BareQuotes", "LazyOddQuotes",
		"LazyQuoteWithTrailingCRLF", "LazyQuotes",
		// No Input at all: these assert that Go's NewReader rejects a bad
		// Comma/Comment *rune*. An API validation test, not a document.
		"BadComma1", "BadComma2", "BadComma3", "BadComma4",
		"BadCommaComment", "BadComment1", "BadComment2", "BadComment3",
	}
	sort.Strings(want)

	if canon(got) != canon(want) {
		t.Errorf("excluded set drifted:\n  want %s\n  got  %s", canon(want), canon(got))
	}
}
