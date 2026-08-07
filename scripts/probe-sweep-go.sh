#!/usr/bin/env bash
#
# Run the Go half of the Phase-1 divergence probe.
#
# The probe is NOT a committed _test.go file: it asserts nothing about
# conformance, and an assert-nothing test living in the live suite is exactly
# the defect class this work exists to remove. It is copied in, run, removed.
#
#   scripts/probe-sweep-go.sh /tmp/go-sweep.json
#
# Then diff against the TypeScript half:
#
#   node scripts/probe-sweep.mjs > /tmp/ts-sweep.json
#   node scripts/probe-sweep.mjs --compare /tmp/ts-sweep.json /tmp/go-sweep.json

set -euo pipefail

out="${1:-/tmp/go-sweep.json}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$here/go/zz_probe_sweep_test.go"

cleanup() { rm -f "$tmp"; }
trap cleanup EXIT

cp "$here/scripts/probe_sweep_test.go.txt" "$tmp"
cd "$here/go"
TABNAS_SWEEP_OUT="$out" go test -run TestProbeSweep -count=1 .
echo "wrote $out"
