# TODO

Legend: `[ ]` planned, `[~]` in progress, `[x]` complete.

## Bootstrap

- [x] Initialize Git repository.
- [x] Create architecture and implementation docs.
- [x] Add Rust workspace and compiler CLI.
- [x] Pin TypeScript 7 dependency.
- [x] Add native `tsconfig` fixture.
- [x] Add CI-style local check command.

## TypeScript 7 frontend

- [x] Locate and validate the official TypeScript 7 CLI.
- [~] Define frontend adapter trait/contracts.
- [x] Run TS7 type checking before native compilation.
- [~] Parse project config and entry points through the adapter.
- [ ] Normalize diagnostics into compiler-owned structures.
- [ ] Define semantic type/symbol DTOs without leaking TS AST internals.

## HIR

- [ ] Define HIR module/function/block/value IDs.
- [ ] Define primitive semantic types.
- [ ] Define expressions and terminators.
- [ ] Add HIR verifier.
- [ ] Add stable textual HIR dump.

## Native representation / MIR

- [ ] Separate TypeScript semantic type from native `Repr`.
- [ ] Add `Bool`, `I32`, `I64`, `F64`, and reference representations.
- [ ] Add representation proof diagnostics.
- [ ] Lower HIR to MIR/SSA.
- [ ] Add direct-call and scalar fast paths.

## LLVM / executable

- [ ] Emit textual LLVM IR.
- [ ] Compile LLVM IR with clang.
- [ ] Link an executable without Node/V8.
- [ ] Compile the first `fib.ts` acceptance test.
- [ ] Add `-O0/-O1/-O2/-O3/-Oz` profiles.

## Runtime and optimization

- [ ] Add strings and typed arrays.
- [ ] Add closed object/class shapes.
- [ ] Add closures and specialization.
- [ ] Add `JSValue` only for dynamic boundaries.
- [ ] Add heap allocator and mark/sweep GC.
- [ ] Add differential tests against TypeScript 7 + Node reference behavior.
- [ ] Add performance report for boxing and dynamic dispatch sites.
