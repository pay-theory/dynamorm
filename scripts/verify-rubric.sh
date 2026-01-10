#!/usr/bin/env bash
set -euo pipefail

bash scripts/verify-planning-docs.sh
bash scripts/verify-threat-controls-parity.sh
bash scripts/verify-doc-integrity.sh
bash scripts/fmt-check.sh
golangci-lint config verify -c .golangci-v2.yml
make lint

make test-unit
make integration
bash scripts/verify-coverage-threshold.sh
bash scripts/verify-coverage.sh

bash scripts/verify-go-modules.sh
bash scripts/verify-ci-toolchain.sh
bash scripts/verify-ci-rubric-enforced.sh
bash scripts/verify-dynamodb-local-pin.sh

bash scripts/verify-no-panics.sh
bash scripts/verify-safe-defaults.sh
bash scripts/verify-network-hygiene.sh
bash scripts/verify-validation-parity.sh
bash scripts/fuzz-smoke.sh

bash scripts/sec-gosec.sh
bash scripts/sec-govulncheck.sh
go mod verify

echo "rubric: PASS"
