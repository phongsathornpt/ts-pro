# Roadmap

## Milestone 0 — Rebase architecture on Go ✅

- [x] Replace the superseded Rust prototype with the Go implementation and remove Rust from the active repository.
- [x] Add a Go module and `cmd/tsnative` CLI.
- [x] Pin TypeScript 7 as the language-service/compiler dependency.
- [x] Validate `tsc --lsp --stdio` from the Go driver.
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
- [x] Emit textual LLVM IR from Go.
- [x] Use clang for object generation/link orchestration and the first native executable.
- [x] Compile the `fib.ts` acceptance program without Node/V8 at runtime.

## Milestone 5 — Native data model 🚧

- [x] Native strings and specialized `number[]`.
- [ ] Add conservative integer narrowing only after range proof.
- [~] Closed object shapes with fixed offsets; class reuse follows.
- [ ] Closures and function values.
- [ ] Monomorphized generics and direct-call specialization.
- [ ] Escape analysis and scalar replacement.

## Milestone 6 — Dynamic boundary and scale

- Tagged `JSValue` only for genuinely dynamic values.
- Checked conversions, dynamic operators, and property slow paths.
- Heap allocator and initial mark/sweep GC.
- Exceptions, Promise, and async/await.
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

The stable checkpoint is `6bad298`: native recursion, scalar/integer fast paths, mutable SSA control flow, strings/`number[]`, closures, classes/inheritance/dispatch, initial generics, shared heap/mark-sweep GC, object caching, and performance reporting. The active working tree is the first partial `JSValue` dynamic-boundary milestone. Native concurrency is planned next as an isolated subsystem after that work reaches a clean checkpoint. See `STATUS.md`, `TODO.md`, and `CONCURRENCY.md`.
