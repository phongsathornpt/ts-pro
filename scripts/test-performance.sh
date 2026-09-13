#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out_dir="${TEST_PERF_OUT_DIR:-$repo_dir/build/test-performance}"
suite="${1:-all}"
mkdir -p "$out_dir"

parallel_args=()
if [[ -n "${TEST_PARALLELISM:-}" ]]; then
  parallel_args=(-parallel "$TEST_PARALLELISM")
fi

run_suite() {
  local label="$1"
  shift
  local log="$out_dir/$label.json"

  echo "== $label =="
  (
    cd "$repo_dir"
    CGO_ENABLED=0 go test -json -count=1 "${parallel_args[@]}" "$@"
  ) | tee "$log"

  echo
  echo "Package elapsed times:"
  awk '
    /"Action":"pass"/ && /"Package":/ && !/"Test":/ && /"Elapsed":/ {
      pkg=$0
      sub(/^.*"Package":"/, "", pkg)
      sub(/".*$/, "", pkg)
      elapsed=$0
      sub(/^.*"Elapsed":/, "", elapsed)
      sub(/[,}].*$/, "", elapsed)
      printf "%8.3fs  %s\n", elapsed + 0, pkg
    }
  ' "$log" | sort -nr
  echo "raw: $log"
}

case "$suite" in
  cases)
    run_suite e2e-cases ./tests/e2e/cases
    ;;
  e2e)
    run_suite e2e ./tests/e2e/...
    ;;
  all)
    run_suite e2e-cases ./tests/e2e/cases
    run_suite full ./...
    ;;
  *)
    echo "usage: $0 [cases|e2e|all]" >&2
    exit 2
    ;;
esac
