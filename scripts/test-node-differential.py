#!/usr/bin/env python3
from pathlib import Path
import subprocess
import tempfile
import sys

ROOT = Path(__file__).resolve().parents[1]
DIRS = ["basics", "dynamic", "objects", "arrays"]
SKIP = {
    "examples/basics/enums_and_defaults.ts": "Node strip-types does not transform enums",
    "examples/basics/multi_module.ts": "Node does not resolve extensionless TypeScript imports",
    "examples/objects/class_constructor_effects.ts": "Node strip-types rejects parameter properties",
    "examples/objects/classes.ts": "Node strip-types rejects parameter properties",
    "examples/objects/inheritance.ts": "Node strip-types rejects parameter properties",
    "examples/objects/override_dispatch.ts": "Node strip-types rejects parameter properties",
    "examples/objects/virtual_dispatch.ts": "Node strip-types rejects parameter properties",
}

fixtures = []
for directory in DIRS:
    fixtures.extend(sorted((ROOT / "examples" / directory).rglob("*.ts")))

failures = []
compared = 0
with tempfile.TemporaryDirectory() as td:
    tmp = Path(td)
    compiler = tmp / "ts-pro"
    subprocess.run(
        ["go", "build", "-o", str(compiler), "./cmd/ts-pro"],
        cwd=ROOT,
        check=True,
    )

    for index, fixture in enumerate(fixtures):
        rel = fixture.relative_to(ROOT).as_posix()
        if rel in SKIP:
            print(f"SKIP\t{rel}\t{SKIP[rel]}")
            continue

        binary = tmp / f"fixture-{index}"
        build = subprocess.run(
            [str(compiler), "build", str(fixture), "--target=linux-amd64", "-o", str(binary)],
            cwd=ROOT,
            capture_output=True,
            text=True,
            timeout=10,
        )
        if build.returncode != 0:
            failures.append((rel, "native build", build.stderr or build.stdout))
            continue
        native = subprocess.run(
            [str(binary)],
            cwd=ROOT,
            capture_output=True,
            text=True,
            timeout=10,
        )
        node = subprocess.run(
            ["node", "--experimental-strip-types", str(fixture)],
            cwd=ROOT,
            capture_output=True,
            text=True,
            timeout=10,
        )
        if node.returncode != 0:
            failures.append((rel, "node execution", node.stderr or node.stdout))
            continue

        compared += 1
        if native.returncode != node.returncode or native.stdout != node.stdout:
            failures.append(
                (rel, "semantic mismatch", f"native={native.returncode} {native.stdout!r}\nnode={node.returncode} {node.stdout!r}")
            )

print(f"node differential: {compared} fixtures matched")
if failures:
    for rel, kind, detail in failures:
        print(f"FAIL\t{rel}\t{kind}\n{detail[:1200]}", file=sys.stderr)
    sys.exit(1)

if compared != 52:
    print(f"expected 52 comparable fixtures, got {compared}", file=sys.stderr)
    sys.exit(1)
