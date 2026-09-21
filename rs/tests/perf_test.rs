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

use std::time::Instant;

use tabnas::Tabnas;

fn make_csv_parser() -> Tabnas {
    tabnas_csv::make()
}

#[test]
fn reusing_one_instance_is_far_cheaper_than_rebuilding_per_parse() {
    const SRC: &str = "a,b,c\n1,2,3";
    const N: usize = 120;

    // Warm both paths so the comparison is steady-state.
    for _ in 0..20 {
        make_csv_parser().parse(SRC).expect("parses");
    }
    let shared = make_csv_parser();
    for _ in 0..50 {
        shared.parse(SRC).expect("parses");
    }

    // Anti-pattern: rebuild the instance (and grammar) on every parse.
    let started = Instant::now();
    for _ in 0..N {
        make_csv_parser().parse(SRC).expect("rebuild parse");
    }
    let rebuild = started.elapsed();

    // Correct pattern: build once, reuse for every parse.
    let started = Instant::now();
    for _ in 0..N {
        shared.parse(SRC).expect("reuse parse");
    }
    let reuse = started.elapsed();

    let ratio = rebuild.as_secs_f64() / reuse.as_secs_f64().max(f64::EPSILON);
    assert!(
        rebuild >= 4 * reuse,
        "reusing one csv instance is not meaningfully faster than rebuilding per parse: \
         {N} rebuild-per-call parses took {rebuild:?} vs {reuse:?} reusing one instance \
         (ratio {ratio:.1}x, want >=4x). Building the CSV grammar should dominate; reuse \
         one tabnas_csv::make() instance (or tabnas_csv::parse) instead of rebuilding per parse."
    );
    eprintln!(
        "perf reuses-instance: rebuild-per-call={rebuild:?} reuse={reuse:?} ratio={ratio:.2}x"
    );
}

#[test]
fn the_shared_parse_reuses_its_instance() {
    // `parse` builds the default instance once: a second call must not
    // cost a rebuild. Measured the same way, against the same source.
    const SRC: &str = "a,b,c\n1,2,3";
    const N: usize = 120;
    tabnas_csv::parse(SRC).expect("parses");

    let started = Instant::now();
    for _ in 0..N {
        make_csv_parser().parse(SRC).expect("rebuild parse");
    }
    let rebuild = started.elapsed();

    let started = Instant::now();
    for _ in 0..N {
        tabnas_csv::parse(SRC).expect("shared parse");
    }
    let shared = started.elapsed();

    assert!(
        rebuild >= 4 * shared,
        "tabnas_csv::parse is rebuilding its instance: {rebuild:?} rebuild-per-call vs {shared:?} shared"
    );
}
