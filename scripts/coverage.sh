#!/usr/bin/env bash
set -euo pipefail

profile="${1:-coverage_lib.out}"

# Measure "library coverage" (exclude repo-local examples and test harness packages).
# This avoids a low-signal denominator dominated by example modules.
pkgs="$(go list ./... | grep -Ev '/examples($|/)|/tests($|/)')"
if [[ -z "${pkgs}" ]]; then
  echo "no packages found"
  exit 1
fi

coverpkgs="$(echo "${pkgs}" | paste -sd, -)"

go test -short -count=1 -coverpkg="${coverpkgs}" -coverprofile="${profile}" ${pkgs} >/dev/null

go tool cover -func="${profile}" | tail -n 1

