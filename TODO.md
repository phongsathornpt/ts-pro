# TODO

Legend: `[ ]` planned, `[~]` in progress, `[x]` complete, `[S]` superseded.

## Architecture migration: Rust -> Go

- [x] Create architecture/type/performance/native-config documentation.
- [S] Rust workspace, Rust CLI, and Rust HIR prototype.
- [x] Add `go.mod` and Go package layout.
- [x] Add `cmd/tsnative` Go CLI.
- [x] Add Go test/lint/build commands.
- [ ] Remove Rust from the active build after Go parity is reached.

## TypeScript 7 / TypeScript-LS

- [x] Pin TypeScript 7.0.2 dependency.
- [x] Add strict native `tsconfig` fixture.
- [x] Validate official `tsc --lsp --stdio` from Go.
- [x] Implement JSON-RPC 2.0 transport over stdio.
- [x] Implement LSP initialize/shutdown lifecycle.
- [ ] Keep one language-server process per workspace.
- [x] Add request cancellation and timeouts.
- [ ] Add LS crash detection and restart policy.
- [x] Capture TypeScript diagnostics into Go DTOs.
- [~] Resolve project files and config through TypeScript-LS.

## Semantic bridge

- [x] Define compiler-owned source/symbol/type/function DTOs.
- [ ] Determine the minimum semantic data required to lower TypeScript to HIR.
- [ ] Implement semantic extraction on top of TypeScript 7.
- [ ] Isolate version-specific/custom TypeScript-LS methods in `internal/tsls`.
- [ ] Add compatibility tests for the pinned TypeScript 7 version.
- [ ] Verify that the compiler path contains no SWC/Babel/Oxc frontend.

## HIR in Go

- [x] Port HIR module/function/block/value IDs from the Rust prototype.
- [x] Port semantic type and `Repr` separation.
- [x] Define expressions, instructions, and terminators.
- [x] Add HIR verifier.
- [x] Add deterministic textual HIR dump.
- [ ] Lower the first typed function from TypeScript-LS semantic DTOs.

## Native representation / MIR

- [ ] Add `Bool`, `I32`, `I64`, `F64`, and reference representations.
- [ ] Add representation-proof diagnostics.
- [ ] Lower HIR to MIR/SSA.
- [ ] Add direct-call and scalar fast paths.
- [ ] Add typed-array and closed-shape representation rules.

## LLVM / executable

- [ ] Emit textual LLVM IR from Go.
- [ ] Compile LLVM IR with clang.
- [ ] Link executable with lld/clang without Node/V8.
- [ ] Compile and run `examples/fib.ts`.
- [ ] Add `-O0/-O1/-O2/-O3/-Oz` profiles.
- [ ] Add parallel LLVM module compilation and object cache.

## Runtime and optimization

- [ ] Add strings and specialized arrays.
- [ ] Add closed object/class shapes.
- [ ] Add closures, specialization, and monomorphization.
- [ ] Add `JSValue` only for dynamic boundaries.
- [ ] Add heap allocator and mark/sweep GC.
- [ ] Add differential tests against TypeScript 7 reference behavior.
- [ ] Add native-coverage, boxing, and dynamic-dispatch performance reports.
- [ ] Add ThinLTO/PGO after MIR quality is established.
