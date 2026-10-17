#!/usr/bin/env bash
# Runs every scripts/tests/*.test.sh (or the ones given: run.sh next-version) and prints one summary line.
# Exit: 0 if all passed, 1 otherwise. Needs bash and git; workflow.test.sh also needs python3 with PyYAML
# (skipped with a notice when PyYAML is missing, unless REQUIRE_YAML=1).
set -Eeuo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
files=("$@")
((${#files[@]})) || files=(*.test.sh)
failed=0
for f in "${files[@]}"; do
  f="${f%.test.sh}"; f="${f%.sh}.test.sh"
  echo "== ${f%.test.sh}"
  ./"$f" || failed=$((failed + 1))
done
if ((failed == 0)); then echo "ALL PASSED (${#files[@]} test file(s))"; else echo "FAILED: $failed of ${#files[@]} test file(s)"; fi
((failed == 0))
