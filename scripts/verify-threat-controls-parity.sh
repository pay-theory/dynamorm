#!/usr/bin/env bash
set -euo pipefail

# Deterministic parity check:
# - Every THR-* ID listed in the threat model must appear at least once in the controls matrix.

threat_model="docs/development/planning/dynamorm-threat-model.md"
controls_matrix="docs/development/planning/dynamorm-controls-matrix.md"

if [[ ! -f "${threat_model}" ]]; then
  echo "threat-parity: FAIL (missing ${threat_model})"
  exit 1
fi
if [[ ! -f "${controls_matrix}" ]]; then
  echo "threat-parity: FAIL (missing ${controls_matrix})"
  exit 1
fi

threats="$(rg -o 'THR-[0-9]+' "${threat_model}" | sort -u || true)"
if [[ -z "${threats}" ]]; then
  echo "threat-parity: FAIL (no THR-* IDs found in threat model; add stable threat IDs)"
  exit 1
fi

missing=0
while IFS= read -r tid; do
  if ! rg -q "${tid}" "${controls_matrix}"; then
    echo "threat-parity: missing mapping for ${tid} in ${controls_matrix}"
    missing=$((missing + 1))
  fi
done <<< "${threats}"

if [[ "${missing}" -ne 0 ]]; then
  echo "threat-parity: FAIL (${missing} threat(s) unmapped)"
  exit 1
fi

echo "threat-parity: PASS"

