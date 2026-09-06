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
- Heap-pointer validation caches the most recently matching chunk and validates exact object starts through a per-chunk allocation bitmap. This replaces the previous object-chain walk with O(1) boundary checks while retaining interior-pointer rejection.
- GC object types are described by a single layout descriptor table used to generate tracing dispatch. Atomic layouts, reference arrays, JSValue data, closures, tasks, channels, collections, and dynamic-entry tables therefore share one type-to-trace-kind source of truth.
- Reclaimed blocks are split, adjacent free blocks are coalesced, and free search is segregated into size classes. A fragmented-reuse benchmark improved from roughly 42.3-42.8 ms before size classes to roughly 28.7 ms, then to roughly 2.8-3.0 ms after allocation-start bitmap validation removed repeated object-chain walks.
- `ts_alloc` reports whether memory is fresh or reclaimed. Arrays, dynamic-property tables, and Map/Set tables skip redundant clearing for zero-filled fresh bump/mmap memory.
- Native GC benchmarks now cover 50,000-allocation churn, fragmented reclaimed-block reuse, and randomized multi-chunk mark locality.

An earlier broad exact-reference classifier was rejected because runtime values may mix allocator-owned references, static strings, boxed JSValues, and dynamic-boundary layouts. The current design instead validates exact heap object starts with allocation metadata and then dispatches tracing from explicit layout descriptors.

Green-Tea-style experiments are benchmark-gated rather than accepted on architecture alone. Moving marked state from object headers into a chunk mark bitmap increased measured GC cost and was reverted. A naive policy that forced every discovered object through a chunk-local pending queue also regressed the dedicated mark-locality benchmark by roughly 11-20% and was reverted.

The accepted hybrid keeps ordinary object work as the first-priority fast path and promotes a chunk only after 64 newly marked objects. In paired baseline/hybrid locality runs, median time improved from roughly 3.82 ms to 3.33 ms per 20 GC cycles (about 12-13%) while sparse heaps retain the original work-stack behavior. Atomic layouts are now completed directly in the marker instead of entering scan work, bringing representative locality runs into roughly the 2.9 ms range. RefData and JSValueData share a four-qword scalar batch scanner, which trims another few percent in stable samples.

Vectorization was evaluated on the Zen 4 benchmark host, which exposes AVX2 and AVX-512. An interleaved 8-slot scalar widening experiment was consistently slower than the four-slot scanner, showing that the dominant cost remains each candidate's scalar marker call, allocation-bitmap validation, and mark dedup rather than memory loads alone. A SIMD scanner is therefore deferred until the runtime has a genuine batch-marker ABI that can validate and mark several candidates without immediately scalarizing them again. The unused 8 KiB experimental mark bitmap was removed from every chunk after that design was rejected.

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

Loop-carried string accumulation now has a conservative owned-buffer optimization for compiler-proven patterns such as `let s = ""; for (...) { s = s + "x"; }`. The compiler seeds a private heap string, derives spare capacity from the hidden allocator object size, and grows geometrically. The visible string ABI remains `[len][bytes]`, and loops with observable reads/calls or unsupported aliasing shapes retain immutable concatenation.

For 20,000 single-character loop appends on the Ryzen 9 7940HS test host:

| implementation | runtime |
| --- | ---: |
| immutable prefix copy each iteration | ~361-364 ms |
| proven-owned geometric append | ~0.188-0.193 ms |

The measured hot pattern is roughly 1,880-1,930x faster because aggregate copying changes from O(n^2) to amortized O(n). A 100,000-append e2e case and number-to-string suffix coercion case cover repeated growth and allocation/rooting behavior. General alias-aware builder conversion remains future representation work; the optimization deliberately falls back when ownership is not proven.

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
