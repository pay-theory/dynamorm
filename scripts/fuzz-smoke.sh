#!/usr/bin/env bash
set -euo pipefail

# Bounded fuzzing pass for panic/crash detection.
#
# This verifier is intentionally short-running. It is expected to be expanded with
# targeted fuzz functions over time.

failures=0

if ! rg -n --no-heading --glob '**/*_test.go' '^func\\s+Fuzz[A-Za-z0-9_]+' internal/expr pkg/marshal pkg/query >/dev/null 2>&1; then
  echo "fuzz-smoke: FAIL (no fuzz targets found; add at least one Fuzz* in internal/expr, pkg/marshal, or pkg/query)"
  exit 1
fi

echo "fuzz-smoke: running bounded fuzz pass (10s per package group)"

go test ./internal/expr -run '^$' -fuzz Fuzz -fuzztime=10s || failures=$((failures + 1))
go test ./pkg/marshal -run '^$' -fuzz Fuzz -fuzztime=10s || failures=$((failures + 1))
go test ./pkg/query -run '^$' -fuzz Fuzz -fuzztime=10s || failures=$((failures + 1))

if [[ "${failures}" -ne 0 ]]; then
  echo "fuzz-smoke: FAIL (${failures} package group(s) failed)"
  exit 1
fi

echo "fuzz-smoke: PASS"

