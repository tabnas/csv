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
