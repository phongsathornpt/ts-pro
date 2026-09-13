#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mode="${1:-main}"

runner_unsupported='TestLinuxAMD64InternalAES128GCM|TestLinuxAMD64InternalTLS13EncryptRecord|TestLinuxAMD64InternalTLS13DecryptRecord'
wintertc_serial='TestLinuxAMD64WinterTCFetchRequestBodyUploadAbort|TestLinuxAMD64WinterTCFetchInFlightAbort|TestLinuxAMD64WinterTCFetchRedirectDoesNotWaitForBody|TestLinuxAMD64WinterTCFetchResolvesAfterHeaders|TestLinuxAMD64WinterTCFetchLiveBodyReader|TestLinuxAMD64WinterTCFetchChunkedLiveBodyBackpressure|TestLinuxAMD64WinterTCFetchLiveBodyReaderCancelClosesSocket|TestLinuxAMD64WinterTCFetchNodeDifferential'

cd "$repo_dir"

case "$mode" in
  main)
    CGO_ENABLED=0 go test ./... \
      -skip "^(${runner_unsupported}|${wintertc_serial})$"
    ;;
  wintertc-serial)
    CGO_ENABLED=0 go test ./tests/e2e/wintertc \
      -run "^(${wintertc_serial})$" \
      -count=1
    ;;
  *)
    echo "usage: $0 [main|wintertc-serial]" >&2
    exit 2
    ;;
esac
