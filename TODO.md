# TODO

Legend: `[ ]` planned, `[~]` in progress, `[x]` complete, `[S]` superseded.

## Project completion goal

- [~] Reach 100% of the compiler roadmap tracked in this file, with every completed capability covered by native acceptance/regression tests and differential tests where TypeScript/JavaScript observable behavior applies.
- [ ] Do not mark the project 100% until all `[ ]` and `[~]` roadmap items below are either completed (`[x]`) or explicitly superseded (`[S]`) with a documented replacement.

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
- [x] Add conservative overflow-safe integer range proof with phi propagation and safe-integer guards.
- [~] Enable `I32`/`I64` lowering only where range proof and boundary conversions preserve TypeScript `number` semantics.
  - [x] Proven local arithmetic fast path computes with i32/i64 while preserving F64 TypeScript-number boundaries.
  - [ ] Keep integer SSA representation across larger subgraphs/calls/loops to eliminate redundant conversions.
  - [ ] Add integer comparisons, overflow-aware specialization guards, and ABI-specialized internal functions where profitable.
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
  - [x] Per-instance property initializers for closed-shape fields.
  - [x] Constructor bodies using the supported native statement/expression subset, preserving initializer and parameter-property order.
  - [x] Fixed-offset mutable instance field stores.
  - [~] Closed-world inheritance and virtual dispatch.
    - [x] Single-inheritance frontend: exact TypeScript 7 heritage-clause decoding, base-class resolution, derived-shape prefix layout, and `super(...)` constructor chaining.
    - [x] Native inheritance acceptance + differential coverage for inherited fields/methods and base-constructor effects.
    - [x] Base-typed references holding derived instances with provenance-based devirtualization when concrete type is proven.
    - [x] Single-module closed-world virtual dispatch using hidden class tags and typed native function-pointer selection when devirtualization cannot prove one target.
    - [ ] Cross-module subclass discovery/dispatch-table extension once multi-module compilation is implemented.
- [x] Closures and captured environments, with direct-call conversion for non-escaping closures and native `{code, env}` function values for escaping closures.
- [~] Generic monomorphization and call-site specialization.
  - [x] Direct type-parameter scalar/string call-site specializations (`identity<T>(x: T): T`).
  - [ ] Nested generic types, generic recursion, constrained structural generics, and specialization caching across modules.
- [ ] Exceptions, Promise, and async/await.

## Native concurrency management

- [x] Add bounded lazy native scheduler core with `TSNATIVE_WORKERS`, parked idle workers, and deterministic restartable shutdown.
- [x] Add lightweight stackless task lifecycle, bounded `TSNATIVE_MAX_TASKS` active-task accounting, FIFO runnable execution, join/release, and 10k-task churn coverage.
- [x] Add compiler-known non-capturing `spawn`, `join`, and `yieldNow` native intrinsics through semantic DTO → HIR → MIR → LLVM, with `TaskRef` representation, task-entry wrappers, static task metrics, native acceptance, LLVM regression, differential observable-order coverage, and full-suite validation.
- [x] Add GC-safe captured task state with compiler-generated task environments pinned as persistent runtime roots until task release.
- [x] Add typed task-result storage/join ABI for returning spawned closures across currently proven native representations.
  - [x] Unboxed `Task<number>` result slots and `join()` returning native F64 with captured-task support and differential/native regression coverage.
  - [x] GC-rooted `Task<string>` result transfer with persistent task-result roots until `join()` transfers ownership to the caller shadow root.
  - [x] Extend rooted reference results to object/array/function values and GC-managed `JSValue` results using TypeScript checker `getTypeArguments` instead of generic type-string parsing.
  - [x] Add unboxed `Task<boolean>` result slots and native bool join ABI; boolean literals now lower natively as `i1`.
- [x] Add per-worker intrusive deques, work stealing, targeted worker wakeups, worker-helping joins, and scheduler steal/park/wakeup metrics.
- [~] Add typed channels with task parking, starting with unboxed `channel<number>`.
  - [x] Add heap-owned F64 channel storage primitives with buffered ring-buffer, unbuffered rendezvous, and nonblocking `try_send`/`try_recv` transitions plus pthread regression coverage.
  - [~] Add resumable task parking/wakeup so blocking send/recv never consumes an OS worker, including workers=1 correctness.
    - [x] Runtime `WAITING -> RUNNABLE` park/wake lifecycle with race-safe pending wake and worker=1 resume regression.
    - [x] Wire F64 channel sender/receiver waiter queues onto park/wake, with operation-complete-before-wake handoff and worker=1 buffered/unbuffered regressions.
    - [ ] Add compiler-generated continuation state so source-level blocking channel operations can suspend and resume arbitrary task bodies.
  - [~] Add compiler-known `channel<number>` operations and scheduler/channel metrics.
    - [x] Compiler-known `channel<number>`, `channelTrySend`, and `channelTryRecvOr` through semantic DTO → HIR → MIR → LLVM, with ChannelRef GC roots, native metrics, differential coverage, and zero-boxing acceptance.
    - [ ] Add source-level blocking `channelSend`/`channelRecv` lowering onto compiler-generated resumable continuation state.
- [ ] Add timers/sleep and a separate bounded blocking-call pool.
- [ ] Lower `async`/`await` to resumable task state machines rather than blocking OS workers.
- [ ] Add structured concurrency, task groups, cancellation, and task-local context.
- [ ] Integrate task/channel/timer state with precise GC roots and scheduler safepoints.
- [ ] Add per-worker allocation caches/nurseries and later Green-Tea-style per-worker mark-page queues.
- [ ] Add cooperative execution budgets/preemption polling after scheduler correctness is stable.
- [ ] Stress-test 1/100/10k/100k tasks, CPU fan-out, channel contention, cancellation, GC churn, and blocking calls.

## Remaining native language/runtime coverage

- [~] Expand native array coverage beyond read-only `number[]`.
  - [x] Bounds-checked in-place indexed writes for specialized `number[]`.
  - [ ] String/object arrays and typed generic array specializations.
  - [ ] JS-compatible growth semantics and common mutators such as push/pop where representation contracts permit.
- [ ] Expand object semantics: optional properties, union shapes, computed/dynamic keys, shape transitions, and broader structural compatibility.
- [ ] Broaden TypeScript syntax coverage: tuples, destructuring, rest/spread, optional chaining, nullish coalescing, switch/do-while/for-of, templates, default/optional params, enums, and module linking.
- [ ] Add selected standard-library/runtime APIs such as JSON, Map/Set, Date, and RegExp after their representation contracts are defined.

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
- [ ] Improve memory optimization with precise object metadata/root maps, escape analysis, stack allocation, scalar replacement, arenas, and eventually generational collection.
- [~] Add parallel LLVM module compilation and deterministic object cache (deterministic LLVM/runtime object cache and parallel runtime compilation implemented; multi-module LLVM scheduling pending).

## Dynamic boundary, correctness, and performance

- [~] Add tagged `JSValue` only for values that cannot keep a proven native representation.
  - [x] Heap-backed GC-managed JSValue boundary for `any` number/string values.
  - [x] Explicit number/string boxing at variable, return, and direct-call parameter boundaries.
  - [ ] Boolean, null/undefined, object/function, and tagged-union JSValue variants.
- [~] Add checked conversions and dynamic operator/property slow paths.
  - [x] Dynamic `+` for number/string JSValue operands and `console.log(any)`.
  - [ ] Dynamic comparisons, arithmetic beyond `+`, property get/set, calls, and checked unboxing/conversions.
- [x] Add differential tests against the TypeScript 7 → JavaScript reference path.
- [x] Add native-coverage, boxing, dynamic-dispatch, and runtime-call reports.
- [x] Add compile-stage timing for TS API, HIR/MIR, LLVM, link, and object-cache hit rate.
- [ ] Add ThinLTO after module/object caching is established.
- [ ] Add PGO after MIR quality and benchmark coverage are stable.
- [ ] Add cross compilation after the Linux x86-64 runtime ABI is stable.

## Current critical path

1. Add resumable task parking/wakeup and compiler-known `channel<number>` send/recv intrinsics on top of the completed F64 channel storage runtime.
2. Add typed channels with task parking, starting with unboxed `channel<number>`, then reference/JSValue channel specializations.
3. Add timers/sleep and a bounded blocking-call pool, then lower Promise/async/await onto resumable task state machines.
4. Add cancellation/task groups, task-local context, exception propagation, and cooperative execution budgets/preemption polling.
5. Integrate task/channel/timer roots with precise GC metadata, per-worker allocation caches/nurseries, and Green-Tea-style local mark-page work.
6. Complete the dynamic boundary: remaining JSValue variants, checked conversions, dynamic arithmetic/comparisons, property access, and calls.
7. Finish advanced generics, integer SSA across calls/loops, remaining array/object semantics, and broader TypeScript syntax/standard-library coverage.
8. Finish multi-module compilation/linking and cross-module dispatch/specialization, then ThinLTO, PGO, and cross-compilation.
