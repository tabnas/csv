package csv

import (
	"testing"
	"time"

	jsonic "github.com/tabnas/jsonic/go"
)

// makeCsvParser builds a fresh jsonic instance with the Csv plugin installed —
// the (expensive) work a caller would do once and then reuse. Building the
// grammar dominates a parse, so doing this per parse is many times slower than
// reusing one instance.
func makeCsvParser() *jsonic.Jsonic {
	j := jsonic.Make()
	j.UseDefaults(Csv, Defaults)
	return j
}

// TestParseReusesInstance guards against the performance trap this package
// exposes: there is no package-level convenience Parse() to cache, so callers
// must build ONE instance (jsonic.Make + UseDefaults(Csv, ...)) and reuse it.
// Rebuilding the instance — and therefore the CSV grammar — on every parse is
// many times slower. This test pins the representative "reuse one instance"
// usage and asserts it is dramatically faster than rebuilding per call, so a
// future change that accidentally rebuilds per parse (or a convenience entry
// point that forgets to cache) is caught.
//
// The check is machine-INDEPENDENT: it compares rebuild-per-call against
// instance reuse on the SAME machine in the SAME run, so a slow CI box cannot
// make it flaky (both sides scale together). There is deliberately NO
// wall-clock budget.
func TestParseReusesInstance(t *testing.T) {
	const src = "a,b,c\n1,2,3"
	const n = 600

	// Warm both paths so the comparison is steady-state.
	for i := 0; i < 50; i++ {
		j := makeCsvParser()
		_, _ = j.Parse(src)
	}
	shared := makeCsvParser()
	for i := 0; i < 50; i++ {
		_, _ = shared.Parse(src)
	}

	// Anti-pattern: rebuild the instance (and grammar) on every parse.
	t0 := time.Now()
	for i := 0; i < n; i++ {
		j := makeCsvParser()
		if _, err := j.Parse(src); err != nil {
			t.Fatalf("rebuild parse error: %v", err)
		}
	}
	rebuild := time.Since(t0)

	// Correct pattern: build once, reuse for every parse.
	t1 := time.Now()
	for i := 0; i < n; i++ {
		if _, err := shared.Parse(src); err != nil {
			t.Fatalf("reuse parse error: %v", err)
		}
	}
	reuse := time.Since(t1)

	// Reusing one instance must be far cheaper than rebuilding per call.
	// Rebuilding is many times slower here (grammar construction dominates),
	// so requiring reuse to be at least 4x faster catches a regression that
	// rebuilds per parse, without depending on absolute wall-clock speed.
	if rebuild < 4*reuse {
		t.Errorf("reusing one Csv instance is not meaningfully faster than "+
			"rebuilding per parse: %d rebuild-per-call parses took %v vs %v "+
			"reusing one instance (ratio %.1fx, want >=4x). Building the CSV "+
			"grammar should dominate — reuse one jsonic.Make()+UseDefaults(Csv) "+
			"instance instead of rebuilding per parse.",
			n, rebuild, reuse, float64(rebuild)/float64(reuse))
	}
	t.Logf("rebuild-per-call=%v  reuse=%v  ratio=%.2fx", rebuild, reuse, float64(rebuild)/float64(reuse))
}
