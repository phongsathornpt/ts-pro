# TODO

Legend: `[ ]` planned, `[~]` in progress, `[x]` complete, `[S]` superseded.

## Foundation and frontend

- [x] Migrate the active compiler from the superseded Rust prototype to Go.
- [x] Remove the superseded Rust workspace from the repository.
- [x] Pin `typescript@7.0.2` and strict native `tsconfig`.
- [x] Implement `cmd/tsnative`, Go tests, vet, build, and doctor commands.
- [x] Implement TypeScript 7 LSP JSON-RPC transport and lifecycle.
- [x] Implement TypeScript 7 `tsc --api --async` semantic transport.
- [x] Decode the TypeScript 7 binary AST protocol in Go.
- [x] Query exact AST-node symbols/types through the TypeScript checker.
- [x] Gate native builds on TypeScript diagnostics before lowering.
- [x] Regression-test that SWC/Babel/Oxc are absent from the active compiler path.
- [x] Keep one long-lived TypeScript-LS process per IDE workspace through the Go workspace manager.
- [x] Add TypeScript-LS crash detection and generation-based restart policy.

## HIR, representation, and MIR

- [x] Define compiler-owned semantic DTOs and typed Go HIR.
- [x] Separate TypeScript semantic types from native runtime `Repr`.
- [x] Add HIR verifier and deterministic textual dump.
- [x] Add scalar representation proof for `Bool`, `F64`, strings, arrays, and references.
- [x] Lower proven HIR to MIR/SSA.
- [x] Add SSA phi nodes for mutable loop-carried values.
- [x] Add direct-call and scalar fast paths.
- [ ] Add conservative integer range proof before enabling `I32`/`I64` narrowing.
- [x] Add checker-derived closed object-shape representation and fixed field layout.

## Native language coverage

- [x] Functions, recursion, returns, and direct calls.
- [x] Native numeric arithmetic: `+ - * /`.
- [x] Native numeric comparisons: `< <= > >= == !=`.
- [x] Mutable locals and assignment, including top-level `const`/`let` entry bindings.
- [x] `if`, `while`, and `for` control flow.
- [x] `i++` / `i--` lowering.
- [x] Specialized unboxed `number[]` literals, `.length`, and indexed reads.
- [x] Native UTF-8 string literals, string parameters/returns, and concatenation.
- [x] `console.log(number)` and `console.log(string)` intrinsics.
- [x] Object literals and fixed-offset property reads through closed shapes.
- [~] Classes, constructors, fields, and devirtualized methods.
  - [x] Parameter-property constructors with closed instance shapes.
  - [x] Direct instance-method calls with a native hidden `this` parameter.
  - [x] Explicit field declarations with `this.field = parameter` constructor assignment bodies.
  - [ ] Property initializers, arbitrary constructor side effects, inheritance, and virtual dispatch.
- [x] Closures and captured environments, with direct-call conversion for non-escaping closures and native `{code, env}` function values for escaping closures.
- [~] Generic monomorphization and call-site specialization.
  - [x] Direct type-parameter scalar/string call-site specializations (`identity<T>(x: T): T`).
  - [ ] Nested generic types, generic recursion, constrained structural generics, and specialization caching across modules.
- [ ] Exceptions, Promise, and async/await.

## LLVM, runtime, and build

- [x] Emit deterministic textual LLVM IR from Go.
- [x] Compile LLVM IR and runtime C sources through the clang toolchain wrapper.
- [x] Link native executables without Node/V8.
- [x] Support `-O0/-O1/-O2/-O3/-Oz`.
- [x] Compile and run `fib.ts`, loop, numeric-array, string, and closed-object acceptance programs.
- [x] Add specialized native F64-array runtime support.
- [x] Add native length-aware UTF-8 string runtime support.
- [x] Add shared heap ownership for strings, arrays, objects, and closure environments/values, with deterministic shutdown from native `main`.
- [x] Add initial mark/sweep GC with explicit native shadow roots, compiler safepoints, conservative heap tracing, and sweep reclamation.
- [~] Add parallel LLVM module compilation and deterministic object cache (deterministic LLVM/runtime object cache and parallel runtime compilation implemented; multi-module LLVM scheduling pending).

## Dynamic boundary, correctness, and performance

- [ ] Add tagged `JSValue` only for values that cannot keep a proven native representation.
- [ ] Add checked conversions and dynamic operator/property slow paths.
- [x] Add differential tests against the TypeScript 7 → JavaScript reference path.
- [x] Add native-coverage, boxing, dynamic-dispatch, and runtime-call reports.
- [x] Add compile-stage timing for TS API, HIR/MIR, LLVM, link, and object-cache hit rate.
- [ ] Add ThinLTO after module/object caching is established.
- [ ] Add PGO after MIR quality and benchmark coverage are stable.
- [ ] Add cross compilation after the Linux x86-64 runtime ABI is stable.

## Current critical path

1. Finish and commit checker-derived closed object shapes.
2. Add object allocation and fixed-offset field load/store through MIR/LLVM.
3. Reuse the same shape model for classes and direct/devirtualized methods.
4. Add closures and specialization/monomorphization.
5. Consolidate heap allocation and introduce mark/sweep GC.
6. Add dynamic `JSValue` boundaries only after static paths are mature.
7. Add differential/performance suites and incremental/parallel build infrastructure.
