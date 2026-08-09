#!/usr/bin/env bash
# Copyright (c) 2026 Richard Rodger and other contributors, MIT License
#
# Fetch the third-party CSV conformance corpora into test/suites/ at pinned
# upstream commits, and regenerate the runtime-neutral case file that both
# ts/test/conformance.test.ts and go/conformance_test.go read.
#
#   test/suites/csv-spectrum/       max-mapper/csv-spectrum (BSD-2-Clause)
#   test/suites/go-encoding-csv/    golang/go src/encoding/csv (BSD-3-Clause)
#
# The corpora are NOT committed (see .gitignore) — they are third-party test
# data with their own licences, and pinning them by commit keeps the repo
# honest about which revision the conformance numbers refer to.
#
# Idempotent: an already-fetched corpus at the right pin is left alone, so
# this is safe to run from a `pretest` hook and works offline once fetched.
# If a corpus is missing and cannot be fetched, this exits non-zero — the
# conformance tests then fail loudly rather than skipping.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(dirname "$HERE")"
SUITES="$ROOT/test/suites"

# --- Pins -----------------------------------------------------------------

SPECTRUM_REPO="max-mapper/csv-spectrum"
SPECTRUM_COMMIT="d30e80f8b99d2eecb3778f1d7b9ed1cb425502ec" # v2.0.0

GO_REPO="golang/go"
GO_COMMIT="3901409b5d0fb7c85a3e6730a59943cc93b2835c"
GO_PATH="src/encoding/csv/reader_test.go"
GO_SHA256="290e930b31250102589928f953c39b4fee0159f794f7b68d38f90862c3b72f38"

mkdir -p "$SUITES"

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

# --- csv-spectrum ---------------------------------------------------------

SPECTRUM_DIR="$SUITES/csv-spectrum"

spectrum_ok() {
  [ -d "$SPECTRUM_DIR/csvs" ] && [ -d "$SPECTRUM_DIR/json" ] &&
    [ -n "$(ls -A "$SPECTRUM_DIR/csvs" 2>/dev/null)" ]
}

if spectrum_ok; then
  echo "csv-spectrum: present ($SPECTRUM_DIR)"
else
  echo "csv-spectrum: fetching $SPECTRUM_REPO@${SPECTRUM_COMMIT:0:12}"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  curl -sfL \
    "https://codeload.github.com/$SPECTRUM_REPO/tar.gz/$SPECTRUM_COMMIT" |
    tar -xz -C "$tmp"
  src="$tmp/csv-spectrum-$SPECTRUM_COMMIT"
  mkdir -p "$SPECTRUM_DIR"
  cp -R "$src/csvs" "$src/json" "$SPECTRUM_DIR/"
  [ -f "$src/readme.md" ] && cp "$src/readme.md" "$SPECTRUM_DIR/"
  [ -f "$src/package.json" ] && cp "$src/package.json" "$SPECTRUM_DIR/"
fi
printf '%s  %s\n' "$SPECTRUM_COMMIT" "$SPECTRUM_REPO" >"$SPECTRUM_DIR/PINNED"

if ! spectrum_ok; then
  echo "csv-spectrum: FAILED to obtain corpus at $SPECTRUM_DIR" >&2
  exit 1
fi

# Verify the corpus, whether it was just fetched or was already on disk. The
# tarball itself is not checksummed (codeload re-gzips, so its bytes are not
# stable), so the pin is over the extracted CORPUS CONTENT, which is: the
# sorted list of document paths interleaved with their bytes. That catches a
# truncated download, a renamed/emptied corpus, and a tampered cache alike —
# an empty corpus scoring 0/0 is precisely the failure this suite exists to
# prevent. If upstream legitimately moves, re-pin SPECTRUM_COMMIT and this
# digest together; never relax the check to get green.
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

n_csv=$(LC_ALL=C find "$SPECTRUM_DIR/csvs" -type f -name '*.csv' | wc -l | tr -d '[:space:]')
n_json=$(LC_ALL=C find "$SPECTRUM_DIR/json" -type f -name '*.json' | wc -l | tr -d '[:space:]')
if [ "$n_csv" != "$SPECTRUM_DOCS" ] || [ "$n_json" != "$SPECTRUM_DOCS" ]; then
  echo "csv-spectrum: expected $SPECTRUM_DOCS .csv + $SPECTRUM_DOCS .json at" \
    "${SPECTRUM_COMMIT:0:12}, found $n_csv + $n_json" >&2
  exit 1
fi

got="$(spectrum_digest)"
if [ "$got" != "$SPECTRUM_DIGEST" ]; then
  echo "csv-spectrum: content digest mismatch at $SPECTRUM_DIR" >&2
  echo "  expected $SPECTRUM_DIGEST" >&2
  echo "  got      $got" >&2
  exit 1
fi
echo "csv-spectrum: $n_csv documents verified at ${SPECTRUM_COMMIT:0:12}"

# --- go/encoding/csv ------------------------------------------------------

GO_DIR="$SUITES/go-encoding-csv"
GO_SRC="$GO_DIR/reader_test.go"
mkdir -p "$GO_DIR"

if [ -f "$GO_SRC" ] && [ "$(sha256_of "$GO_SRC")" = "$GO_SHA256" ]; then
  echo "go/encoding/csv: present at pinned revision"
else
  echo "go/encoding/csv: fetching $GO_REPO@${GO_COMMIT:0:12} $GO_PATH"
  curl -sfL \
    "https://raw.githubusercontent.com/$GO_REPO/$GO_COMMIT/$GO_PATH" \
    -o "$GO_SRC"
  got="$(sha256_of "$GO_SRC")"
  if [ "$got" != "$GO_SHA256" ]; then
    echo "go/encoding/csv: sha256 mismatch" >&2
    echo "  expected $GO_SHA256" >&2
    echo "  got      $got" >&2
    exit 1
  fi
fi
printf '%s  %s %s\n' "$GO_COMMIT" "$GO_REPO" "$GO_PATH" >"$GO_DIR/PINNED"

# Regenerating is deterministic and fast, so it always runs — the case file
# can never drift from the pinned source it claims to come from.
node "$HERE/extract-go-csv-cases.mjs" "$GO_SRC" "$GO_DIR/cases.json"

echo "conformance corpora ready under $SUITES"
