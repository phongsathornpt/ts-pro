# Implementation Status

## Stable committed baseline

The committed compiler currently supports an end-to-end TypeScript 7 native path through commit `5d3d5cb`.

```text
TypeScript 7.0.2
  -> diagnostics + semantic API
  -> TypeScript binary AST decoded in Go
  -> compiler semantic DTOs
  -> typed HIR
  -> representation proof
  -> MIR / SSA
  -> LLVM IR
  -> clang + small native runtime
  -> native executable
```

The normal generated-program runtime path does not embed Node.js, V8, SWC, Babel, or Oxc.

## Stable language/runtime coverage

- typed functions, recursion, return values, and direct calls;
- numeric `+ - * /` and `< <= > >= == !=`;
- mutable locals and assignment;
- `if`, `while`, and `for`, including SSA phi nodes;
- specialized contiguous `number[]`, `.length`, and indexed reads;
- UTF-8 string literals, parameters/returns, and string concatenation;
- native `console.log(number)` and `console.log(string)`;
- optimization profiles `-O0`, `-O1`, `-O2`, `-O3`, and `-Oz`.

## Acceptance programs

Current committed examples prove these native paths:

```text
examples/fib.ts      -> 6765
examples/scalars.ts  -> scalar arithmetic/comparison results
examples/loops.ts    -> 45 / 45
examples/arrays.ts   -> 15
examples/strings.ts  -> Hello, TypeScript 7!
```

`number` remains IEEE-754 `F64` by default. `I32`/`I64` narrowing is intentionally deferred until range analysis can prove that narrowing preserves TypeScript/JavaScript number semantics.

## In-progress working tree

Closed object-shape support is currently under implementation and is not part of the stable committed baseline yet. The work already introduces checker-derived shape/property metadata and HIR groundwork for object allocation and fixed fields. MIR/LLVM allocation and field access still need to be completed and accepted before the feature is marked complete.

## Next major stages

1. Closed objects and classes with deterministic layouts.
2. Closures and captured environments.
3. Function/generic specialization and monomorphization.
4. Shared heap allocation and initial GC.
5. Dynamic `JSValue` fallback paths.
6. Differential correctness and performance reporting.
7. Incremental object cache, parallel LLVM codegen, ThinLTO, and PGO.
