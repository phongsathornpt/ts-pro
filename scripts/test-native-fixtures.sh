#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cd "$ROOT"
CGO_ENABLED=0 go build -o "$TMP/ts-pro" ./cmd/ts-pro

export TS_PRO_FIXTURE_ROOT="$ROOT"
export TS_PRO_FIXTURE_TMP="$TMP"
export TS_PRO_FIXTURE_TIMEOUT="${TS_PRO_FIXTURE_TIMEOUT:-30}"

python3 <<'PY'
from pathlib import Path
import os
import subprocess
import sys
import time

root = Path(os.environ["TS_PRO_FIXTURE_ROOT"])
tmp = Path(os.environ["TS_PRO_FIXTURE_TMP"])
timeout = float(os.environ["TS_PRO_FIXTURE_TIMEOUT"])
compiler = tmp / "ts-pro"
fixtures = sorted((root / "examples").rglob("*.ts"))
counts = {"PASS": 0, "DIAG": 0, "BUILD_FAIL": 0, "RUN_FAIL": 0, "TIMEOUT": 0}
failures = []
started = time.time()

for index, fixture in enumerate(fixtures):
    output = tmp / f"fixture-{index}"
    rel = fixture.relative_to(root)
    try:
        build = subprocess.run(
            [str(compiler), "build", str(fixture), "--target=linux-amd64", "-o", str(output)],
            cwd=root,
            capture_output=True,
            text=True,
            timeout=10,
        )
    except subprocess.TimeoutExpired:
        counts["TIMEOUT"] += 1
        failures.append(("BUILD_TIMEOUT", str(rel), ""))
        continue

    if build.returncode != 0:
        lines = (build.stderr or build.stdout).strip().splitlines()
        kind = "DIAG" if any("error[" in line or "type checking failed" in line for line in lines) else "BUILD_FAIL"
        counts[kind] += 1
        failures.append((kind, str(rel), lines[0] if lines else ""))
        continue

    try:
        run = subprocess.run(
            [str(output)],
            cwd=root,
            capture_output=True,
            text=True,
            timeout=timeout,
        )
    except subprocess.TimeoutExpired:
        counts["TIMEOUT"] += 1
        failures.append(("RUN_TIMEOUT", str(rel), ""))
        continue

    if run.returncode != 0:
        counts["RUN_FAIL"] += 1
        failures.append(("RUN_FAIL", str(rel), f"exit={run.returncode} {run.stderr.strip()[:160]}"))
        continue

    counts["PASS"] += 1
elapsed = time.time() - started
print(f"native fixtures: {counts['PASS']}/{len(fixtures)} PASS in {elapsed:.2f}s")
print(" ".join(f"{key}={value}" for key, value in counts.items()))

if failures:
    for kind, fixture, detail in failures:
        print(f"{kind}\t{fixture}\t{detail}", file=sys.stderr)
    sys.exit(1)

if counts["PASS"] != len(fixtures):
    sys.exit(1)
PY
