# Roadmap

## Milestone 0 — Bootstrap

- Initialize repository and project metadata.
- Pin TypeScript 7 as the frontend dependency.
- Verify TypeScript 7 diagnostics through the compiler driver.
- Add architecture, type-model, performance, and native-config docs.

## Milestone 1 — Frontend adapter

- Define stable compiler-owned semantic DTOs.
- Add TypeScript 7 adapter boundary.
- Discover project files from `tsconfig.json`.
- Capture diagnostics and reject invalid input before lowering.

## Milestone 2 — Typed HIR

- Define modules, functions, blocks, values, expressions, and control flow.
- Lower literals, locals, arithmetic, comparisons, calls, branches, and returns.
- Add HIR verifier and textual dump format.

## Milestone 3 — MIR and LLVM

- Add representation analysis for Bool, F64, integers, and static functions.
- Lower HIR to MIR/SSA.
- Emit textual LLVM IR.
- Use clang/lld to produce the first executable binary.

## Milestone 4 — Native data model

- Strings and typed arrays.
- Closed object/class shapes with fixed field offsets.
- Closures and function values.
- Monomorphization and direct-call specialization.

## Milestone 5 — Dynamic boundary

- Tagged `JSValue` representation.
- Checked conversions and dynamic operators.
- Dynamic property access slow paths.
- Clear diagnostics for unsupported JavaScript semantics.

## Milestone 6 — Runtime and scale

- Heap allocator and initial mark/sweep GC.
- Exceptions, Promise, and async/await.
- Incremental object cache and parallel LLVM codegen.
- ThinLTO, PGO, debug information, and cross compilation.
