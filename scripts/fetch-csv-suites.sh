#!/usr/bin/env bash
#
# Fetch the third-party CSV conformance corpora into test/suites/.
#
# NOTHING THIS SCRIPT DOWNLOADS IS EVER COMMITTED. test/suites/ is gitignored.
# Only this script and the pinned commit SHAs below live in the repo. The same
# pattern is used by sibling repos (toml/ts/test/toml-test, xml/test/xmlconf).
#
# Idempotent: re-running is safe and cheap. Pass --force to re-fetch from
# scratch.
#
# Usage:
#   scripts/fetch-csv-suites.sh [--force]
#
# ---------------------------------------------------------------------------
# SUITE 1 - max-mapper/csv-spectrum
#   https://github.com/max-mapper/csv-spectrum
#   "A bunch of different CSV files to serve as an acid test for CSV parsing
#   libraries." The most widely cited third-party CSV corpus. 12 documents,
#   each with an expected JSON value. VALID DOCUMENTS ONLY - csv-spectrum has
#   no must-fail half whatsoever.
#
# SUITE 2 - golang/go src/encoding/csv/reader_test.go
#   https://github.com/golang/go
#   The Go standard library's own RFC 4180 reader conformance table: 68 cases
#   carrying both expected values AND expected errors (ErrQuote, ErrBareQuote,
#   ErrFieldCount). This supplies the must-fail half that csv-spectrum lacks.
#   Converted to a runtime-neutral cases.json by scripts/extract-go-csv-cases.mjs.
#
# WHY NOT W3C csvw. The W3C CSV-on-the-Web suite has 145 NegativeValidationTest
# entries, but every one of them is a JSON-LD metadata/schema violation rather
# than a CSV syntax violation - checked against tests/manifest-validation.jsonld,
# where the count of negative tests whose action is a bare .csv document is
# exactly ZERO. It cannot serve as a must-fail CSV syntax corpus.
# ---------------------------------------------------------------------------

set -euo pipefail

# --- PINNED UPSTREAM COMMITS. Never a branch, never "latest". ---------------

CSV_SPECTRUM_REPO="https://github.com/max-mapper/csv-spectrum.git"
CSV_SPECTRUM_SHA="d30e80f8b99d2eecb3778f1d7b9ed1cb425502ec"   # master, 2019-06-06

# golang/go, tag go1.24.0
GO_STDLIB_SHA="3901409b5d0fb7c85a3e6730a59943cc93b2835c"
GO_STDLIB_PATH="src/encoding/csv/reader_test.go"

# ---------------------------------------------------------------------------

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
suites="$here/test/suites"

force=0
[ "${1:-}" = "--force" ] && force=1

mkdir -p "$suites"

# --- suite 1: csv-spectrum -------------------------------------------------

spectrum="$suites/csv-spectrum"
if [ "$force" = 1 ]; then rm -rf "$spectrum"; fi

if [ -d "$spectrum/.git" ] &&
   [ "$(git -C "$spectrum" rev-parse HEAD 2>/dev/null || true)" = "$CSV_SPECTRUM_SHA" ]; then
  echo "csv-spectrum: already at $CSV_SPECTRUM_SHA"
else
  rm -rf "$spectrum"
  echo "csv-spectrum: cloning $CSV_SPECTRUM_REPO @ $CSV_SPECTRUM_SHA"
  git clone --quiet "$CSV_SPECTRUM_REPO" "$spectrum"
  git -C "$spectrum" checkout --quiet "$CSV_SPECTRUM_SHA"
fi

# Sanity: the corpus must actually contain what we expect. A silently empty
# or renamed corpus is the exact failure mode this exercise exists to stop.
n_csv=$(find "$spectrum/csvs" -name '*.csv' | wc -l | tr -d ' ')
n_json=$(find "$spectrum/json" -name '*.json' | wc -l | tr -d ' ')
if [ "$n_csv" != "12" ] || [ "$n_json" != "12" ]; then
  echo "FATAL: csv-spectrum @ $CSV_SPECTRUM_SHA should have 12 csv + 12 json," \
       "found $n_csv + $n_json. Re-pin the SHA; do not relax this check." >&2
  exit 1
fi
echo "csv-spectrum: $n_csv documents at $spectrum"

# --- suite 2: golang/go encoding/csv ---------------------------------------

gostd="$suites/go-encoding-csv"
mkdir -p "$gostd"
raw="$gostd/reader_test.go"

if [ "$force" = 1 ]; then rm -f "$raw" "$gostd/cases.json"; fi

if [ ! -s "$raw" ]; then
  url="https://raw.githubusercontent.com/golang/go/$GO_STDLIB_SHA/$GO_STDLIB_PATH"
  echo "go-encoding-csv: downloading $url"
  curl --fail --silent --show-error --location -o "$raw.tmp" "$url"
  mv "$raw.tmp" "$raw"
fi
echo "$GO_STDLIB_SHA  golang/go $GO_STDLIB_PATH" > "$gostd/PINNED"

# Convert the Go table to a runtime-neutral JSON corpus. The extractor aborts
# if the case counts drift from the pinned expectation.
node "$here/scripts/extract-go-csv-cases.mjs" "$raw" "$gostd/cases.json"

echo
echo "Corpora ready under $suites (gitignored, never committed)."
