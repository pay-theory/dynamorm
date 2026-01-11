#!/usr/bin/env bash
set -euo pipefail

required="90.0"
script="scripts/verify-coverage.sh"

if [[ ! -f "${script}" ]]; then
  echo "coverage-threshold: FAIL (${script} not found)"
  exit 1
fi

default="$(
  awk -F= '
    /^default_threshold=/ {
      v=$2
      gsub(/"/, "", v)
      gsub(/\047/, "", v)
      print v
      exit
    }
  ' "${script}"
)"

if [[ -z "${default}" ]]; then
  echo "coverage-threshold: FAIL (default_threshold not found in ${script})"
  exit 1
fi

awk -v d="${default}" -v r="${required}" 'BEGIN { exit !(d+0 >= r+0) }' || {
  echo "coverage-threshold: FAIL (default ${default}% < required ${required}%)"
  exit 1
}

echo "coverage-threshold: ok (default ${default}% >= ${required}%)"
