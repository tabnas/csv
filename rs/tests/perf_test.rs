// The performance trap this package exposes, pinned the way
// ts/test/perf.test.ts and go/perf_test.go pin it: building the instance
// (parsing the embedded grammar, installing it over jsonic) dominates a
// small parse, so a caller must build ONE instance and reuse it. The
// crate's own `parse` does exactly that behind a `OnceLock`; this test
// asserts that reuse is dramatically cheaper than rebuilding per call, so
// a change that rebuilds per parse (or a convenience entry point that
// forgets to cache) is caught.
//
// The check is machine-INDEPENDENT: it compares rebuild-per-call against
// instance reuse on the SAME machine in the SAME run, so a slow box cannot
// make it flaky (both sides scale together). There is deliberately NO
// wall-clock budget.
//
// It is also ONE test, deliberately, and that is load-bearing. The two
// measurements used to be two `#[test]` functions in this one integration
// binary, and the test harness runs a binary's tests in parallel unless it
// is told otherwise -- `ci/rust/run.sh` runs a plain `cargo test
// --all-targets`, with no `--test-threads`. One test's expensive
// rebuild loop then overlapped the other's cheap reuse loop, inflating the
// reuse side by however the scheduler happened to interleave them and
// making the asserted ratio a function of core count rather than of the
// code. Splitting them again reintroduces that flake; a harness flag would
// not, because it would only protect CI and not a developer typing
// `cargo test`.

use std::time::{Duration, Instant};

use tabnas::Tabnas;

const SRC: &str = "a,b,c\n1,2,3";
const N: usize = 120;

/// The ratio reuse has to beat. Rebuilding parses the whole embedded
/// grammar and installs it over jsonic, so the real margin is two orders
/// of magnitude; 4x is the floor a regression has to stay above.
const WANT: u32 = 4;

fn make_csv_parser() -> Tabnas {
    tabnas_csv::make()
}

/// `N` parses through `parse_once`, timed. Every measurement in this test
/// goes through here so the loops are identical apart from what they call.
fn time_parses(mut parse_once: impl FnMut()) -> Duration {
    let started = Instant::now();
    for _ in 0..N {
        parse_once();
    }
    started.elapsed()
}

#[test]
fn reusing_one_instance_is_far_cheaper_than_rebuilding_per_parse() {
    // Warm every path so the comparison is steady-state.
    for _ in 0..20 {
        make_csv_parser().parse(SRC).expect("parses");
    }
    let shared = make_csv_parser();
    for _ in 0..50 {
        shared.parse(SRC).expect("parses");
    }
    tabnas_csv::parse(SRC).expect("parses");

    // Anti-pattern: rebuild the instance (and grammar) on every parse.
    // Measured once; both correct patterns are judged against it.
    let rebuild = time_parses(|| {
        make_csv_parser().parse(SRC).expect("rebuild parse");
    });

    // Correct pattern: build once, reuse for every parse.
    let reuse = time_parses(|| {
        shared.parse(SRC).expect("reuse parse");
    });

    // The same, through the crate's own entry point, which caches its
    // instance in a `OnceLock`: a second call must not cost a rebuild.
    let convenience = time_parses(|| {
        tabnas_csv::parse(SRC).expect("shared parse");
    });

    // Guard the baseline before dividing by it. If the rebuild loop
    // measured as no time at all then nothing was measured, and every
    // ratio below would be vacuously true -- a green test asserting
    // nothing, which is the one outcome worse than a red one.
    assert!(
        rebuild > Duration::ZERO,
        "{N} rebuild-per-call parses measured as zero elapsed time: this is \
         testing the clock, not the code"
    );

    let reuse_ratio = rebuild.as_secs_f64() / reuse.as_secs_f64().max(f64::EPSILON);
    let convenience_ratio = rebuild.as_secs_f64() / convenience.as_secs_f64().max(f64::EPSILON);
    eprintln!(
        "perf reuses-instance: rebuild-per-call={rebuild:?} reuse={reuse:?} \
         ratio={reuse_ratio:.2}x | tabnas_csv::parse={convenience:?} \
         ratio={convenience_ratio:.2}x"
    );

    assert!(
        rebuild >= WANT * reuse,
        "reusing one csv instance is not meaningfully faster than rebuilding per parse: \
         {N} rebuild-per-call parses took {rebuild:?} vs {reuse:?} reusing one instance \
         (ratio {reuse_ratio:.1}x, want >={WANT}x). Building the CSV grammar should dominate; \
         reuse one tabnas_csv::make() instance (or tabnas_csv::parse) instead of rebuilding \
         per parse."
    );

    assert!(
        rebuild >= WANT * convenience,
        "tabnas_csv::parse is rebuilding its instance: {rebuild:?} rebuild-per-call vs \
         {convenience:?} through tabnas_csv::parse (ratio {convenience_ratio:.1}x, \
         want >={WANT}x). `parse` must build the default instance once, behind its OnceLock."
    );
}
