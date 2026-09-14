#!/usr/bin/env python3
from pathlib import Path
import os
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
STABLE_SEEDS = [1, 7, 42, 1337, 0xD1FFCAFE, 0x5EED5EED]


class XorShift64:
    def __init__(self, seed: int) -> None:
        self.state = seed & ((1 << 64) - 1)
        if self.state == 0:
            self.state = 1

    def next(self) -> int:
        x = self.state
        x ^= (x << 13) & ((1 << 64) - 1)
        x ^= x >> 7
        x ^= (x << 17) & ((1 << 64) - 1)
        self.state = x & ((1 << 64) - 1)
        return self.state

    def n(self, limit: int) -> int:
        return self.next() % limit

    def between(self, low: int, high: int) -> int:
        return low + self.n(high - low + 1)


def extra_seed() -> list[int]:
    raw = os.environ.get("TS_PRO_DIFF_SEED", "").strip()
    if not raw:
        return []
    try:
        seed = int(raw, 0)
    except ValueError as exc:
        raise SystemExit(f"invalid TS_PRO_DIFF_SEED {raw!r}: {exc}") from exc
    return [seed or 1]


def generate_program(seed: int) -> str:
    rng = XorShift64(seed)
    a = rng.between(1, 20)
    b = rng.between(1, 20)
    delta = rng.between(1, 9)
    fallback = rng.between(21, 40)
    loop_count = rng.between(3, 8)
    extra = rng.between(5, 30)
    array_values = [rng.between(1, 25) for _ in range(5)]
    array_index = rng.n(len(array_values))
    flag = "true" if rng.n(2) == 0 else "false"
    optional_flag = "true" if rng.n(2) == 0 else "false"

    values = ", ".join(str(v) for v in array_values)
    return f"""
function identity<T>(value: T): T {{
  return value;
}}

function mix(a: number, b: number): number {{
  let total = a;
  for (let i = 0; i < {loop_count}; i = i + 1) {{
    if (i % 2 === 0) {{
      total += b + i;
    }} else {{
      total -= a - i;
    }}
  }}
  return total;
}}

const values: number[] = [{values}];
let sum = 0;
for (const value of values) {{
  sum += value;
}}
values[{array_index}] += {delta};
console.log(sum);
console.log(values[{array_index}]);

const extended: number[] = [...values, {extra}];
console.log(extended[extended.length - 1]);

const point: {{ x: number; y: number }} = {{ x: {a}, y: {b} }};
point.x += {delta};
const copied = {{ ...point, y: {extra} }};
console.log(point.x + point.y);
console.log(copied.x + copied.y);

const maybe: number | null = {flag} ? {a + b} : null;
console.log(maybe ?? {fallback});

const wrapped: {{ inner?: {{ value: number }} }} = {optional_flag} ? {{ inner: {{ value: {extra} }} }} : {{}};
console.log(wrapped.inner?.value ?? {fallback});

const offset = {delta};
const add = (value: number): number => value + offset;
console.log(add({a}));
console.log(identity<number>({b}));

let dynamic: any = {a};
dynamic = dynamic + {b};
console.log(dynamic);
dynamic = "v" + dynamic;
console.log(dynamic);

console.log(mix({a}, {b}));
""".lstrip()


def run(cmd: list[str], *, cwd: Path, timeout: int = 15) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        cmd,
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=timeout,
    )


def main() -> int:
    seeds = STABLE_SEEDS + extra_seed()
    failures: list[tuple[int, str, str, str]] = []

    with tempfile.TemporaryDirectory() as td:
        tmp = Path(td)
        compiler = tmp / "ts-pro"
        build_compiler = run(
            ["go", "build", "-o", str(compiler), "./cmd/ts-pro"],
            cwd=ROOT,
            timeout=60,
        )
        if build_compiler.returncode != 0:
            sys.stderr.write(build_compiler.stderr or build_compiler.stdout)
            return build_compiler.returncode or 1

        for index, seed in enumerate(seeds):
            source = generate_program(seed)
            fixture = tmp / f"generated-{index}-{seed}.ts"
            binary = tmp / f"generated-{index}-{seed}"
            fixture.write_text(source)

            native_build = run(
                [str(compiler), "build", str(fixture), "--target=linux-amd64", "-o", str(binary)],
                cwd=ROOT,
            )
            if native_build.returncode != 0:
                failures.append((seed, "native build", native_build.stderr or native_build.stdout, source))
                continue

            native = run([str(binary)], cwd=ROOT)
            node = run(["node", "--experimental-strip-types", str(fixture)], cwd=ROOT)
            if node.returncode != 0:
                failures.append((seed, "node execution", node.stderr or node.stdout, source))
                continue

            if native.returncode != node.returncode or native.stdout != node.stdout:
                detail = (
                    f"native={native.returncode} {native.stdout!r}\n"
                    f"node={node.returncode} {node.stdout!r}"
                )
                failures.append((seed, "semantic mismatch", detail, source))
                continue

            print(f"PASS\tseed={seed}\tstdout={native.stdout!r}")

    if failures:
        for seed, kind, detail, source in failures:
            print(
                f"FAIL\tseed={seed}\t{kind}\n{detail[:2000]}\n"
                f"--- generated source ---\n{source}",
                file=sys.stderr,
            )
        return 1

    print(f"generated differential: {len(seeds)} seeds matched")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
