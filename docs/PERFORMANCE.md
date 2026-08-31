# Performance Strategy

Performance is split into frontend/compile-time and generated-code runtime goals.

## Frontend and compile-time priorities

1. Keep one TypeScript-LS process alive per workspace.
2. Reuse TypeScript 7 incremental project state instead of reparsing through another frontend.
3. Batch semantic extraction and avoid one LSP round-trip per AST node.
4. Cache normalized semantic snapshots by source/project version.
5. Use Go worker pools only after semantic snapshots are stable.
6. Cache HIR/MIR/object files with deterministic content keys.
7. Parallelize LLVM modules and use ThinLTO for release builds.

The compiler must not spawn a fresh `tsc`/LS process per source file. LSP/JSON-RPC latency is acceptable for project-level batches, but chatty node-by-node protocols are not.

## Runtime priorities

1. Unboxed scalar representations.
2. Direct function calls and devirtualization.
3. Specialized typed arrays (implemented for `number[]`) and closed object shapes (in progress).
4. Monomorphized generics and specialized functions.
5. Escape analysis and scalar replacement.
6. Runtime calls only for dynamic/complex semantics.
7. LLVM optimization after high-quality MIR exists.

LLVM cannot recover performance lost by boxing every value or hiding hot operations behind opaque runtime calls. The current compiler already keeps numeric loops in SSA, uses phi nodes for loop-carried values, stores `number[]` as contiguous doubles, and represents strings as native references rather than `JSValue`.

## Target behavior

- IDE and compiler share TypeScript 7 project state and semantics.
- Incremental native builds should avoid full TypeScript project cold starts.
- Typed numeric code should approach systems-language performance.
- Startup and base RSS should be substantially lower than Node.js.
- Mixed code should keep static regions native and isolate dynamic regions.
- Highly dynamic JavaScript may remain slower than mature JIT engines.

## Measure separately

Track TypeScript-LS startup, project load, semantic extraction, HIR/MIR lowering, LLVM codegen, link time, binary startup, steady-state throughput, RSS, boxing sites, and dynamic-dispatch sites as separate metrics.


## Current representation policy

TypeScript `number` remains `F64` unless a future range analysis proves that `I32`/`I64` narrowing is semantics-preserving. Integer-looking syntax alone is not sufficient proof. Specialized `number[]` uses contiguous F64 storage; strings use a length-aware UTF-8 native representation. Closed object shapes are the active next representation milestone.
