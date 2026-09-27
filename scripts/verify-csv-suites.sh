#!/usr/bin/env bash
# Copyright (c) 2026 Richard Rodger and other contributors, MIT License
#
# Verify the vendored third-party CSV conformance corpora under test/suites/
# against their pinned upstream revisions:
#
#   test/suites/csv-spectrum/       max-mapper/csv-spectrum (BSD-2-Clause)
#   test/suites/go-encoding-csv/    golang/go src/encoding/csv (BSD-3-Clause)
#
# test/suites/README.md says where each came from, under which licence, and
# how to move a pin. The corpora are committed, so this touches no network:
# it checks that what is committed is still exactly the pinned content, and
# that cases.json is still what scripts/extract-go-csv-cases.mjs derives from
# reader_test.go. `npm test` runs it via the pretest hook. Any mismatch exits
# non-zero, and a missing corpus is a mismatch: an empty corpus scoring 0/0
# is precisely the failure the conformance suites exist to prevent.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(dirname "$HERE")"
SUITES="$ROOT/test/suites"

# --- Pins -----------------------------------------------------------------

SPECTRUM_REPO="max-mapper/csv-spectrum"
SPECTRUM_COMMIT="d30e80f8b99d2eecb3778f1d7b9ed1cb425502ec" # v2.0.0

GO_REPO="golang/go"
GO_COMMIT="862c888e612ac346c7c4d99c9392bdfd265f33b0" # go1.27.1
GO_PATH="src/encoding/csv/reader_test.go"
GO_SHA256="290e930b31250102589928f953c39b4fee0159f794f7b68d38f90862c3b72f38"

fail() {
  echo "$*" >&2
  echo "  The corpora are vendored: restore them with" \
    "\`git checkout -- test/suites\`, or see test/suites/README.md." >&2
  exit 1
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

sha256_stream() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | cut -d' ' -f1
  else
    shasum -a 256 | cut -d' ' -f1
  fi
}

# Each corpus directory carries a PINNED file naming its revision.
check_pinned() {
  local file="$1" want="$2"
  [ -f "$file" ] || fail "missing $file"
  [ "$(cat "$file")" = "$want" ] ||
    fail "$file names '$(cat "$file")', not the pin '$want'"
}

# --- csv-spectrum ---------------------------------------------------------

SPECTRUM_DIR="$SUITES/csv-spectrum"
check_pinned "$SPECTRUM_DIR/PINNED" "$SPECTRUM_COMMIT  $SPECTRUM_REPO"

# The pin is over the CORPUS CONTENT: the sorted list of document paths
# interleaved with their bytes. That catches a truncated or emptied corpus,
# a renamed document, and an edited one alike, line endings included. If
# upstream legitimately moves, re-pin SPECTRUM_COMMIT and this digest
# together; never relax the check to get green.
SPECTRUM_DOCS=12
SPECTRUM_DIGEST="0b1f4ca7b8a30ddf3f6fd2db8294a7924998a7c80f85cc7b71b25cc3bb73209a"

spectrum_digest() {
  (
    cd "$SPECTRUM_DIR" || exit 1
    LC_ALL=C find csvs json -type f \( -name '*.csv' -o -name '*.json' \) |
      LC_ALL=C sort |
      while IFS= read -r f; do
        printf '%s\n' "$f"
        cat "$f"
      done
  ) | sha256_stream
}

[ -d "$SPECTRUM_DIR/csvs" ] && [ -d "$SPECTRUM_DIR/json" ] ||
  fail "csv-spectrum: no csvs/ and json/ under $SPECTRUM_DIR"

n_csv=$(LC_ALL=C find "$SPECTRUM_DIR/csvs" -type f -name '*.csv' | wc -l | tr -d '[:space:]')
n_json=$(LC_ALL=C find "$SPECTRUM_DIR/json" -type f -name '*.json' | wc -l | tr -d '[:space:]')
if [ "$n_csv" != "$SPECTRUM_DOCS" ] || [ "$n_json" != "$SPECTRUM_DOCS" ]; then
  fail "csv-spectrum: expected $SPECTRUM_DOCS .csv + $SPECTRUM_DOCS .json at" \
    "${SPECTRUM_COMMIT:0:12}, found $n_csv + $n_json"
fi

got="$(spectrum_digest)"
if [ "$got" != "$SPECTRUM_DIGEST" ]; then
  fail "csv-spectrum: content digest mismatch at $SPECTRUM_DIR
  expected $SPECTRUM_DIGEST
  got      $got"
fi
echo "csv-spectrum: $n_csv documents verified at ${SPECTRUM_COMMIT:0:12}"

# --- go/encoding/csv ------------------------------------------------------

GO_DIR="$SUITES/go-encoding-csv"
GO_SRC="$GO_DIR/reader_test.go"
check_pinned "$GO_DIR/PINNED" "$GO_COMMIT  $GO_REPO $GO_PATH"

[ -f "$GO_SRC" ] || fail "go/encoding/csv: missing $GO_SRC"
got="$(sha256_of "$GO_SRC")"
if [ "$got" != "$GO_SHA256" ]; then
  fail "go/encoding/csv: sha256 mismatch for $GO_SRC
  expected $GO_SHA256
  got      $got"
fi
echo "go/encoding/csv: reader_test.go verified at ${GO_COMMIT:0:12}"

# cases.json is committed so the Go and Rust suites need no Node. Deriving it
# again must give the same bytes, so it cannot drift from the source it
# claims to come from.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
if ! node "$HERE/extract-go-csv-cases.mjs" "$GO_SRC" "$tmp/cases.json" \
  >"$tmp/extract.log" 2>&1; then
  cat "$tmp/extract.log" >&2
  exit 1
fi
if ! cmp -s "$tmp/cases.json" "$GO_DIR/cases.json"; then
  echo "go/encoding/csv: $GO_DIR/cases.json is not what" \
    "scripts/extract-go-csv-cases.mjs derives from reader_test.go." >&2
  echo "  Regenerate it: node scripts/extract-go-csv-cases.mjs" \
    "test/suites/go-encoding-csv/reader_test.go" \
    "test/suites/go-encoding-csv/cases.json" >&2
  exit 1
fi
echo "go/encoding/csv: cases.json matches its extraction"

echo "conformance corpora verified under $SUITES"
