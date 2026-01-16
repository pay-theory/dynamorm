#!/usr/bin/env bash
set -euo pipefail

make integration

if [[ -f "ts/package.json" ]]; then
  bash scripts/verify-typescript-integration.sh
fi

echo "integration-tests: PASS"

