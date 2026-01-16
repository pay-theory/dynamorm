#!/usr/bin/env bash
set -euo pipefail

bash scripts/verify-go-modules.sh
bash scripts/verify-typescript-build.sh

echo "builds: PASS"

