#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
os="$(uname -s)"
arch="$(uname -m)"

run_native() {
  cd "$repo_dir"
  CGO_ENABLED=0 go test ./tests/e2e/... -run '^TestLinuxAMD64' -count=1
}

if [[ "$os" == "Linux" && "$arch" == "x86_64" ]]; then
  run_native
  exit 0
fi

if [[ "$os" == "Darwin" ]]; then
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker is required to execute linux/amd64 E2E from macOS" >&2
    exit 2
  fi
  exec docker run --rm --platform linux/amd64 \
    -e CGO_ENABLED=0 \
    -v "$repo_dir:/src" \
    -w /src \
    golang:1.27.1 \
    go test ./tests/e2e/... -run '^TestLinuxAMD64' -count=1
fi

echo "unsupported host for linux/amd64 execution runner: $os/$arch" >&2
exit 2
