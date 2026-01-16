#!/usr/bin/env bash
set -euo pipefail

make test-unit

if [[ -f "ts/package.json" ]]; then
  if [[ ! -d "ts/node_modules" ]]; then
    bash scripts/verify-typescript-deps.sh
  fi
  npm --prefix ts run test:unit
fi

echo "unit-tests: PASS"

