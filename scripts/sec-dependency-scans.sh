#!/usr/bin/env bash
set -euo pipefail

bash scripts/sec-govulncheck.sh
bash scripts/sec-npm-audit.sh

echo "dependency-scans: PASS"

