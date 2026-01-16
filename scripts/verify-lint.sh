#!/usr/bin/env bash
set -euo pipefail

make lint

if [[ -f "ts/package.json" ]]; then
  if [[ ! -d "ts/node_modules" ]]; then
    bash scripts/verify-typescript-deps.sh
  fi
  npm --prefix ts run lint
fi

echo "lint: PASS"

