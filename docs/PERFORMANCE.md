# Performance Strategy

Performance is split into compile-time and runtime goals.

## Runtime priorities

1. Unboxed scalar representations.
2. Direct function calls and devirtualization.
3. Typed arrays and closed object shapes.
4. Monomorphized generics and specialized functions.
5. Escape analysis and scalar replacement.
6. Runtime calls only for dynamic/complex semantics.
7. LLVM optimization after high-quality MIR exists.

LLVM cannot recover performance lost by boxing every value or hiding hot operations behind opaque runtime calls.

## Target behavior

- Typed numeric code should approach systems-language performance.
- Startup and base RSS should be substantially lower than Node.js.
- Mixed code should keep static regions native and isolate dynamic regions.
- Highly dynamic JavaScript may remain slower than mature JIT engines.
