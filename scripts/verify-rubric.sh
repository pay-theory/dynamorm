#!/usr/bin/env bash
set -euo pipefail

bash scripts/verify-planning-docs.sh
bash scripts/verify-threat-controls-parity.sh
bash scripts/verify-doc-integrity.sh

# Install TS deps once for all TS verifiers.
bash scripts/verify-typescript-deps.sh

# Install Python deps once for all Python verifiers.
bash scripts/verify-python-deps.sh

bash scripts/verify-dms-first-workflow.sh

bash scripts/verify-formatting.sh
golangci-lint config verify -c .golangci-v2.yml
bash scripts/verify-lint.sh
bash scripts/verify-public-api-contracts.sh

bash scripts/verify-builds.sh
bash scripts/verify-unit-tests.sh
bash scripts/verify-integration-tests.sh
bash scripts/verify-coverage-threshold.sh
bash scripts/verify-coverage.sh

bash scripts/verify-ci-toolchain.sh
bash scripts/verify-ci-rubric-enforced.sh
bash scripts/verify-branch-release-supply-chain.sh
bash scripts/verify-branch-version-sync.sh
bash scripts/verify-dynamodb-local-pin.sh

bash scripts/verify-no-panics.sh
bash scripts/verify-safe-defaults.sh
bash scripts/verify-network-hygiene.sh
bash scripts/verify-expression-hardening.sh
bash scripts/verify-encrypted-tag-implemented.sh
bash scripts/verify-file-size.sh
bash scripts/verify-maintainability-roadmap.sh
bash scripts/verify-query-singleton.sh
bash scripts/verify-validation-parity.sh
bash scripts/fuzz-smoke.sh

bash scripts/sec-gosec.sh
bash scripts/sec-dependency-scans.sh
go mod verify

echo "rubric: PASS"
