# Roadmap

## Architecture correction — TypeScript 7 compiler / Go runtime only

- [ ] Move compiler-owned semantic normalization, HIR/MIR, representation analysis, LLVM emission, and build orchestration out of Go into the TypeScript 7 compiler implementation.
- [ ] Freeze compile-time Go work to migration/parity fixes only.
- [x] Complete `runtimego/` migration so Go is the sole handwritten runtime/native-library language.
- [x] Remove handwritten runtime C after Go ABI parity; ABI consumers use the generated Go `c-archive` header.
- [ ] Retire transitional `cmd/` and `internal/` Go compiler packages after TypeScript-native parity tests pass.

## Milestone 0 — Historical Go compiler rebase [superseded target]

- [S] Replace the Rust prototype with the current Go implementation as an intermediate step; the final architecture moves compile-time ownership to TypeScript 7 and keeps Go only for runtime/native libraries.
- [x] Add a Go module and `cmd/tsnative` CLI as transitional compiler infrastructure; retire it after TypeScript 7 parity.
- [x] Pin TypeScript 7 as the language-service/compiler dependency.
- [x] Validate `tsc --lsp --stdio` from the transitional Go driver.
- [x] Keep strict native `tsconfig` as the project contract.

## Milestone 1 — TypeScript-LS client

- Implement JSON-RPC/LSP stdio transport in Go.
- Initialize one long-lived TypeScript-LS per workspace.
- Implement document/project lifecycle and cancellation.
- Collect diagnostics and project configuration.
- Add health/restart handling for crashed LS processes.
- Add integration tests against pinned TypeScript 7.

## Milestone 2 — Semantic bridge ✅

- [x] Define stable compiler-owned symbol/type/source DTOs.
- [x] Extract function signatures, declarations, references, and exact checker types.
- [x] Use the pinned official `tsc --api --async` compiler API for semantic extraction.
- [x] Decode the TypeScript 7 binary AST protocol in Go.
- [x] Ensure no SWC/Babel/Oxc parser exists in the compiler path.

## Milestone 3 — Typed HIR in Go ✅

- [x] Define module/function/block/value IDs and semantic type IDs.
- [x] Lower literals, locals, arithmetic, comparisons, calls, branches, returns, mutable loops, and phi merges.
- [x] Keep TypeScript semantic type separate from native representation.
- [x] Add HIR verifier and stable textual dump.

## Milestone 4 — MIR and LLVM ✅

- [x] Add representation proof for Bool, F64, strings, arrays, references, and static functions.
- [x] Lower HIR into MIR/SSA, including loop phi nodes.
- [x] Emit textual LLVM IR from the current Go compiler; port this emitter to the TypeScript 7 compiler before retiring Go compile-time code.
- [x] Use clang for object generation/link orchestration and the first native executable.
- [x] Compile the `fib.ts` acceptance program without Node/V8 at runtime.

## Milestone 5 — Native data model 🚧

- [x] Native strings and specialized `number[]`.
- [x] Add conservative integer narrowing only after range proof.
- [x] Closed object shapes with fixed offsets, including class reuse/inheritance support.
- [x] Closures and function values.
- [~] Monomorphized generics and direct-call specialization; initial scalar/string call-site specialization is complete, advanced/cross-module cases remain.
- [~] Escape analysis and scalar replacement; numeric-only non-escaping objects support stack allocation and mutable scalar replacement across nested acyclic CFG merges, fully scalarizable reference-bearing objects can be eliminated while their field values remain GC-rooted, and proven local closures use stack-resident closure/env cells while captures stay independently rooted. Physical non-scalarizable reference-bearing objects still require precise stack-root/interior-pointer handling.

## Milestone 6 — Dynamic boundary and scale

- Tagged `JSValue` only for genuinely dynamic values.
- Checked conversions, dynamic operators, and property slow paths.
- Heap allocator and initial mark/sweep GC.
- [~] Exceptions, Promise, and async/await: native rejection recovery and immediate `Promise.resolve` / `Promise.reject<T>` settlement are implemented; Promise adoption and aggregate combinators remain.
- Incremental object cache, parallel codegen, ThinLTO, PGO, and cross compilation.


## Milestone 7 — Native concurrency management

- Bounded M:N scheduler with worker count near CPU cores.
- Lightweight stackless tasks with spawn/join/yield.
- Per-worker queues and work stealing.
- Typed channels, timers, cancellation, structured concurrency, and blocking-call isolation.
- Async/await state machines that park tasks instead of OS threads.
- Scheduler/GC integration, per-worker allocation, and later Green-Tea-style mark-page work.

See `CONCURRENCY.md` for the step-by-step commit plan and acceptance gates.

## Current checkpoint

The historical compiler checkpoint `6bad298` predates the current runtime/concurrency work. The current branch now has the Go-only handwritten native runtime, stackless tasks/async continuations, typed channels, timers/blocking pool, structured concurrency, cancellation, task-local context, and dynamic JSValue coverage described in `STATUS.md` and `TODO.md`. The remaining critical path is precise scheduler/GC integration, allocator locality, incomplete dynamic/Promise/language coverage, multi-module work, and migration of compile-time ownership into TypeScript 7.
