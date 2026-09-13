#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out_dir="${TEST_PERF_OUT_DIR:-$repo_dir/build/test-performance}"
suite="${1:-all}"
top_n="${TEST_PERF_TOP_N:-20}"
raw_stdout="${TEST_PERF_RAW_STDOUT:-1}"
mkdir -p "$out_dir"

parallel_args=()
if [[ -n "${TEST_PARALLELISM:-}" ]]; then
  parallel_args=(-parallel "$TEST_PARALLELISM")
fi

print_summary() {
  local log="$1"

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

  echo
  echo "Slowest tests (top $top_n):"
  awk '
    /"Action":"pass"/ && /"Package":/ && /"Test":/ && /"Elapsed":/ {
      pkg=$0
      sub(/^.*"Package":"/, "", pkg)
      sub(/".*$/, "", pkg)
      test=$0
      sub(/^.*"Test":"/, "", test)
      sub(/".*$/, "", test)
      elapsed=$0
      sub(/^.*"Elapsed":/, "", elapsed)
      sub(/[,}].*$/, "", elapsed)
      printf "%8.3fs  %s  %s\n", elapsed + 0, pkg, test
    }
  ' "$log" | sort -nr | head -n "$top_n"

  echo "raw: $log"
}

run_suite() {
  local label="$1"
  shift
  local log="$out_dir/$label.json"
  local status

  echo "== $label =="
  set +e
  if [[ "$raw_stdout" == "1" ]]; then
    (
      cd "$repo_dir"
      CGO_ENABLED=0 go test -json -count=1 "${parallel_args[@]}" "$@"
    ) | tee "$log"
    status=${PIPESTATUS[0]}
  else
    (
      cd "$repo_dir"
      CGO_ENABLED=0 go test -json -count=1 "${parallel_args[@]}" "$@"
    ) >"$log"
    status=$?
  fi
  set -e

  print_summary "$log"
  if ((status != 0)); then
    if [[ "$raw_stdout" != "1" ]]; then
      echo
      echo "Failure tail:"
      tail -n 200 "$log"
    fi
    return "$status"
  fi
}

case "$suite" in
  cases)
    run_suite e2e-cases ./tests/e2e/cases
    ;;
  wintertc)
    run_suite wintertc ./tests/e2e/wintertc
    ;;
  e2e)
    run_suite e2e ./tests/e2e/...
    ;;
  all)
    run_suite e2e-cases ./tests/e2e/cases
    run_suite full ./...
    ;;
  *)
    echo "usage: $0 [cases|wintertc|e2e|all]" >&2
    exit 2
    ;;
esac
