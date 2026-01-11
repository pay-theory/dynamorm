#!/usr/bin/env bash
set -euo pipefail

# Verifies required branch/release supply-chain artifacts exist and are wired for the expected flow:
# - `premain` -> prereleases
# - `main` -> stable releases
#
# This is a deterministic grep-based check (not a full YAML parser).

failures=0

required_files=(
  "docs/development/planning/dynamorm-branch-release-policy.md"
  ".github/workflows/prerelease.yml"
  ".github/workflows/release.yml"
)

for f in "${required_files[@]}"; do
  if [[ ! -f "${f}" ]]; then
    echo "branch-release: missing ${f}"
    failures=$((failures + 1))
  fi
done

if [[ -f ".github/workflows/prerelease.yml" ]]; then
  grep -Eq 'branches:.*premain' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must target premain"
    failures=$((failures + 1))
  }
  grep -Eq 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv4\b' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must pin release-please v4 by commit SHA"
    failures=$((failures + 1))
  }
  grep -Eq 'contents:\s*write' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must request contents: write"
    failures=$((failures + 1))
  }
fi

if [[ -f ".github/workflows/release.yml" ]]; then
  grep -Eq 'branches:.*main' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must target main"
    failures=$((failures + 1))
  }
  grep -Eq 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv4\b' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must pin release-please v4 by commit SHA"
    failures=$((failures + 1))
  }
  grep -Eq 'contents:\s*write' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must request contents: write"
    failures=$((failures + 1))
  }
fi

for wf in ".github/workflows/quality-gates.yml" ".github/workflows/codeql.yml"; do
  if [[ ! -f "${wf}" ]]; then
    continue
  fi
  grep -Eq 'branches:.*premain.*main|branches:.*main.*premain' "${wf}" || {
    echo "branch-release: ${wf}: expected triggers for both premain and main"
    failures=$((failures + 1))
  }
done

if [[ "${failures}" -ne 0 ]]; then
  echo "branch-release: FAIL (${failures} issue(s))"
  exit 1
fi

echo "branch-release: PASS"
