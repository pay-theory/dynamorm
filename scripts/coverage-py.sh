#!/usr/bin/env bash
set -euo pipefail

suite="${1:-unit}"

if [[ ! -f "py/pyproject.toml" ]]; then
  echo "coverage-py: SKIP (py/pyproject.toml not found)"
  exit 0
fi

if [[ ! -d "py/.venv" ]]; then
  bash scripts/verify-python-deps.sh
fi

export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-us-east-1}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-dummy}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-dummy}"
export DYNAMODB_ENDPOINT="${DYNAMODB_ENDPOINT:-http://localhost:8000}"

outdir="py"
log="${outdir}/coverage-${suite}.txt"

tests=(tests/unit)
if [[ "${suite}" == "all" ]]; then
  skip="${SKIP_INTEGRATION:-}"
  if [[ "${skip}" == "1" || "${skip}" == "true" ]]; then
    echo "coverage-py: SKIP (SKIP_INTEGRATION=${skip})"
    exit 0
  fi
  tests=(tests/unit tests/integration)
elif [[ "${suite}" != "unit" ]]; then
  echo "coverage-py: FAIL (unknown suite: ${suite}; expected 'unit' or 'all')"
  exit 1
fi

uv --directory py run pytest -q "${tests[@]}" \
  --cov=dynamorm_py \
  --cov-report=term \
  --cov-report=xml:coverage.xml | tee "${log}"

echo "coverage-py: PASS (${suite})"

