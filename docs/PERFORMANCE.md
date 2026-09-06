# Performance Strategy

This document records the measured performance policy for the current raw-native compiler and Linux AMD64 runtime. Optimizations are accepted only when targeted benchmarks improve and the full correctness gates remain green.

## Performance gates

Every behavior-changing optimization must pass:

```text
go test ./...
./scripts/test-native-fixtures.sh
python3 scripts/test-node-differential.py
```

The current acceptance gates are 125/125 native fixtures and 52/52 Node differential fixtures. Run the reproducible local benchmark sweep with:

```text
make bench-performance
```

Runtime process benchmarks include executable launch overhead, so compare the same benchmark on the same machine and use repeated samples for performance decisions.

## 2026-09 optimization sweep

### Allocation and GC

- Allocation now uses bump space as the O(1) common path. Reclaimed free-list blocks are searched only under bump-space pressure.
- Major tracing uses an intrusive mark worklist rather than repeated full-heap fixed-point rescans.
- Heap-pointer validation caches the most recently matching chunk, preserving validation semantics while avoiding repeated linked-list walks for local reference graphs.
- `ts_alloc` reports whether memory is fresh or reclaimed. Arrays, dynamic-property tables, and Map/Set tables skip redundant clearing for zero-filled fresh bump/mmap memory.
- A native allocation-churn benchmark exercises 50,000 allocations per executable run.

An exact-reference GC marker was prototyped and rejected. The current runtime can contain allocator-owned references, static string references, boxed JSValues, and layouts shared at dynamic boundaries. Broad exact classification caused native crashes, including Map/Set cases. The validated marker plus chunk-locality cache remains the safer measured design.

### Dynamic objects

Dynamic property tables use power-of-two open addressing with FNV-1a key hashes and a last-successful-slot L1 cache. Entries are `{hash, key, value}` and grow before load exceeds 75%.

For 800,000 mixed property reads across a 64-property object on the Ryzen 9 7940HS test host:

| implementation | runtime |
| --- | ---: |
| linear scan + last-slot cache | ~46.8-50.6 ms |
| open-addressed hash table + last-slot cache | ~8.28-8.45 ms |

The mixed-key workload improves by roughly 5.6-6.1x while repeated same-key access retains the last-slot fast path.

### Arrays and native copies

Array growth copies the live prefix first and clears only unused reclaimed capacity. Fresh backing stores skip clearing entirely. Array and Map/Set growth share a four-qword-unrolled native copy helper.

For 1,638,400 scalar array pushes per executable run:

| implementation | runtime |
| --- | ---: |
| scalar qword copy loop | ~8.37-9.04 ms |
| four-qword unrolled growth copy | ~7.56-8.72 ms |

The gain is modest but repeatable, approximately 5-10% around the median. The same unrolled helper was tested for short string concatenation and rejected because setup overhead made that workload substantially slower.

### String concatenation

Binary native string concatenation copies qwords plus a short byte tail. Chained native string additions with three or four operands are fused into `ts_string_concat3` / `ts_string_concat4`, calculating total length and allocating the result once. Longer chains are grouped into fixed-arity fused calls.

IR fusion preserves evaluation order and does not flatten dynamic JSValue addition. Numeric subexpressions also remain grouped, so `1 + 2 + "x"` keeps JavaScript/TypeScript addition semantics and produces `"3x"`.

For 100,000 four-part concatenation chains:

| implementation | runtime |
| --- | ---: |
| three binary result allocations per chain | ~5.53-6.33 ms |
| one fused result allocation per chain | ~2.36-2.38 ms |

This is roughly a 2.4x improvement for the measured chain workload.

Loop-carried `s = s + x` still has immutable-string O(n^2) aggregate copying. Solving that requires a proven unique/temporary builder or rope representation and is separate representation work, not a safe local runtime micro-optimization.

### Register allocation

The linear-scan allocator now compacts its active set in place and maintains end-order without allocating a fresh active slice and sorting it for every interval.

For a synthetic 2,048-value loop-heavy function:

| metric | before | after |
| --- | ---: | ---: |
| time | ~1.43-1.54 ms/op | ~0.60-0.62 ms/op |
| heap | ~2.04 MB/op | ~665 KB/op |
| allocations | 14,591/op | 92/op |

The measured throughput improves by about 2.4x and allocator-side Go allocations fall by more than 99%.

A full block-liveness bitset dataflow replacement for the loop-backedge heuristic was implemented experimentally. After the active-set fix it regressed the same benchmark to ~0.72-0.73 ms/op and increased allocations from 92 to 344, so it was rejected.

### Optimizer

Folded numeric constants are stored unboxed in a `map[int]float64` rather than boxed as `ir.Operand` interface values. At 2,048 synthetic instructions, optimizer memory falls from about 234 KB/op to about 165 KB/op. Large-workload throughput is approximately neutral while the medium workload improves by roughly 10%.

Instruction-slice reuse was also benchmarked and rejected because it did not materially improve time or allocation counts.

### Compiler lifetime and modules

- Each public compilation starts a new `FileSet`, preventing a long-lived `Compiler` from retaining source bytes and line tables from every previous invocation.
- Relative module resolution is cached within one build, avoiding repeated filesystem candidate probes in diamond/repeated imports while deliberately avoiding stale cross-build filesystem state.

## Representation policy

- TypeScript `number` remains F64 unless range analysis proves narrower integer storage semantics-preserving.
- Specialized `number[]` uses contiguous 64-bit elements.
- Native strings use length-prefixed UTF-8 storage and remain immutable.
- Static regions stay in native representations; boxed JSValue paths are reserved for dynamic boundaries.
- Runtime helpers should exist only where semantics or dynamic representation require them.

## Benchmark policy

Benchmark changes using before/after commits or worktrees, not memory or intuition. Keep an optimization only when its target workload improves enough to justify code and correctness complexity. A rejected optimization with recorded numbers is a completed experiment, not unfinished work.
