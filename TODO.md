# TODO

Legend: `[ ]` planned, `[~]` in progress, `[x]` complete, `[S]` superseded.

## Active raw-native compiler status (source of truth)

> This section tracks the currently active handwritten frontend → SSA → raw native backend.
> Historical Pure-Go / LLVM / TypeScript-7 migration sections below are retained for context only; the active raw-native status is determined exclusively by this section and its acceptance gates.
> Current active acceptance: **152 / 152 native fixtures PASS, 0 diagnostics, 0 build/lowering failures, 0 runtime failures, 0 timeouts**. Deterministic differential coverage is **52 / 52 Node-comparable fixtures matching stdout and exit status**; 7 fixtures are explicitly skipped only because Node strip-types cannot execute enum syntax, extensionless TS imports, or parameter-property syntax directly.

### Completed active milestones

- [x] Linux AMD64 standalone ELF startup, SysV ABI, spills, strict fixups, F64/SSE2 semantics, short-circuit logic, shortest-roundtrip NumberToString, and cross-host Linux-AMD64 execution runner.
- [x] Native heap metadata, precise shadow roots, mark-sweep reclamation/free-list reuse, and GC stress for current heap objects.
- [x] Native strings, arrays, indexed growth, tuples, closed objects, closures/indirect calls, and reference-aware GC tracing.
- [x] Generic function type variables, inference, explicit specialization, monomorphized IR, heterogeneous generic tuples, and specialization caching.
- [x] Non-generic and generic native classes, constructors, parameter properties, initializers, mutation, inheritance, `super(...)`, overrides, hidden class tags, and closed-world virtual dispatch.
- [x] `do...while`, `switch`, array `for...of`, compound assignments, parser progress guarantees, and fail-loud unsupported IR generation.
- [x] Array/object/tuple destructuring via single-evaluation parser desugaring.
- [x] Native null/undefined sentinels plus optional/default-call ABI classification, omitted optional arguments, and `console.log(undefined)`.
- [x] Numeric enums lowered as compile-time constants.
- [x] Template literal interpolation with normal expression parsing, exact number-to-string conversion, boolean/nullish coercion, nullable-union coercion, and native regression coverage.

### Basic TypeScript / standard-library fixture coverage

- [x] Rest parameters and spread syntax.
  - [x] Rest parameter ABI/lowering via typed native-array packing.
  - [x] Array spread via SSA element-copy loops over existing native arrays.
  - [x] Closed-object spread with source-order fixed-field copying and override semantics.
- [x] Nullish and optional semantics beyond the sentinel foundation.
  - [x] `??` nullish coalescing with sentinel-only short-circuit semantics.
  - [x] Optional chaining `?.` with nullish receiver guards and nested-chain composition.
  - [x] Optional object fields with full contextual native shape allocation and missing-field `undefined`.
  - [x] Common union-property lookup across compatible closed object members.
- [x] Computed property keys / index access for statically known string keys and concretely proven `any` aliases.
- [x] Evolving/dynamic object shapes with growable GC-traced `{string key, JSValue}` tables.
- [x] Multi-module import/export graph, relative module resolution, dependency ordering/linking, import aliases, cycle rejection, and cross-module symbol resolution.
- [x] Native standard APIs required by fixtures.
  - [x] `JSON.parse` / `JSON.stringify`.
  - [x] `Date`.
  - [x] `Map` / `Set`.
  - [x] `RegExp`, including regex literal syntax.

### Dynamic JavaScript value model

- [x] Introduce a one-word NaN-boxed native `JSValue` representation without degrading typed F64/string/object fast paths.
- [x] Complete boxing/unboxing boundaries for `any` and mixed unions across primitives, arrays, closures, closed objects, dynamic objects, calls, returns, and typed consumers.
- [x] Dynamic truthiness, strict/loose equality, arithmetic, relational comparison, `ToPrimitive`, `ToNumber`, and `ToString`.
- [x] Dynamic property get/set and evolving property storage with GC-traced growable entry tables.
- [x] Dynamic calls plus receiver-correct class/structural method `this`, including function-expression `this:` pseudo-parameters.
- [x] Close all `examples/dynamic/*` native fixture failures and include them in deterministic Node differential acceptance.

### Native concurrency and async/Promise coverage for the active backend

- [x] Add active-backend semantic declarations and IR for scheduler/task primitives (`spawn`, `join`, `yieldNow`, scheduler-aware `sleep`, task groups/cancellation/context).
- [x] Add typed task results, private native task stacks, and precise GC roots for suspended tasks.
- [x] Add typed buffered/unbuffered channels plus blocking/nonblocking operations and cross-task rendezvous.
- [x] Add parser/sema support for `async`, `await`, `try`, `catch`, `finally`, and `throw`.
- [S] Compiler-generated async state machines are superseded on the active raw backend by stackful cooperative task stacks with saved callee state and precise suspended shadow-root chains; async/await suspension uses that scheduler model.
- [x] Add Promise lifecycle, rejection propagation, `finally` completion semantics, resolve/reject, adoption/thenables, repeated await, `all`/`race`, and scheduler-aware timer ordering required by fixtures.
- [x] Close all `examples/concurrency/*` native fixture failures.

### Active-backend final acceptance

- [x] Re-run and commit a deterministic native fixture sweep script.
- [x] Reach the original **125 / 125** active raw-native fixture milestone compiling/running with **0 diagnostics, 0 lowering failures, 0 runtime failures, 0 timeouts**.
- [x] Add Node differential execution for deterministic observable semantics and require matching stdout/exit status (**52 / 52 comparable fixtures**).
- [x] Stress GC during dynamic-object graphs, closures, class dispatch, suspended task stacks, channels, Promise/thenable continuations, and aggregate results.
- [x] `CGO_ENABLED=0 go test ./...`.
- [x] `CGO_ENABLED=0 go vet ./...`.
- [x] `CGO_ENABLED=0 go build ./...`.
- [x] `make test-linux-amd64`.
- [x] `git diff --check`.
- [x] Active raw-native fixture roadmap accepted at its original **125 / 125** milestone, with the async state-machine item explicitly superseded by the documented stackful scheduler model.

## 2026-09 God-file / God-method refactor closeout

- [x] Split `internal/midend/irgen/irgen.go` from 5,693 LOC to 482 LOC and isolate closures, exceptions, promises, classes/generics, coercion, objects, strings, statements, Web APIs, constructors, members, and calls by responsibility.
- [x] Split `internal/frontend/sema/sema.go` from 2,630 LOC to 695 LOC and isolate Web builtins, core builtins, Promise typing, statements/narrowing, constructors, calls, members, and expression checking.
- [x] Split `internal/frontend/parser/parser.go` from 1,435 LOC to 173 LOC and isolate declarations, statements, expressions, and type syntax.
- [x] Split `internal/backend/lower/lower.go` from 2,588 LOC to 295 LOC; isolate ARM64/AMD64 lowering, allocator/string runtime, runtime symbol emission, finalization/fixups, and per-function AMD64 lowering.
- [x] Reduce `lowerAMD64` from a ~1,100-line God method to ~100 lines of orchestration (`entry -> functions -> runtime -> finalize`).
- [x] Split `jsops_amd64.go` into cohesive string coercion, numeric coercion, and comparison runtime modules.
- [x] Split `tests/e2e/linux_amd64_test.go` from 2,285 LOC to 858 LOC with dedicated async, runtime-value, and WinterTC suites.
- [x] Preserve behavior across every refactor commit with targeted E2E plus full `go test ./...`, native fixture, Node differential, and `git diff --check` gates.
- [x] Final refactor regression gate was **141 / 141 native fixtures PASS**, **52 / 52 Node-comparable fixtures match**, and `make bench-performance` PASS.
- [x] Intentionally keep cohesive runtime modules such as GC, task scheduler, Base64, and number formatting together unless a future responsibility split is justified by behavior/change pressure rather than LOC alone.

## Optional next roadmap (new scope, not active blockers)

The active raw-native fixture roadmap is complete. Subsequent WinterTC/performance coverage has expanded the current native suite to **151 / 151 PASS**; the original **125 / 125** milestone remains the acceptance point for the completed core roadmap.

- [ ] Performance: reduce GPR/XMM payload shuffling, benchmark allocator/GC/scheduler behavior, improve register allocation/code layout, and evaluate PGO/LTO where useful.
- [ ] Language/ecosystem expansion: broaden TypeScript/ECMAScript syntax, built-in APIs, package/module resolution, and uncommon dynamic edge cases beyond the current fixture set.
- [ ] Platform parity: bring the complete active feature surface and fixture gates to ARM64/macOS and Windows.
- [ ] Reliability: add parser/sema/IR/backend fuzzing, randomized GC/scheduler/channel/Promise stress, and broader generated-program differential testing.
- [ ] Production hardening: security review, reproducible benchmarks/releases, cross-compilation ergonomics, and performance regression tracking.
  - [x] 2026-09 native performance sweep: allocator fast path, worklist GC, chunk-locality validation, runtime allocation benchmarks, dynamic hash tables, fresh-allocation zero skipping, array/collection copy unrolling, string-concat chain fusion, compiler source-set/module-resolution retention fixes, and measured regalloc/optimizer reductions.
  - [x] Add `make bench-performance` as the reproducible local performance regression entry point.
  - [x] Eliminate O(n^2) copying for proven-owned loop string accumulators with ABI-compatible geometric growth; retain immutable fallback when ownership is not proven.
  - [x] Green-Tea foundation: central GC layout descriptors plus per-chunk allocation-start bitmap validation; fragmented GC reuse falls from roughly 28.6 ms to roughly 2.8-3.0 ms by replacing object-chain boundary walks with O(1) bitmap tests.
  - [x] Add `BenchmarkAMD64GCMarkLocality` and include it in `make bench-performance` so future span/chunk batching changes are measured against randomized multi-chunk live-reference traversal.
  - [x] Green-Tea locality follow-up: retain the object-work fast path and promote only dense chunks after 64 newly marked objects. Paired locality runs improved median mark time by roughly 12-13% versus the allocation-bitmap baseline while sparse work keeps the original stack path.
  - [x] Add layout-specialized scan paths: atomic objects finish at mark time without queueing, and RefData/JSValueData scan four qwords per scalar batch. Wider 8-slot batches regressed in interleaved measurements, and AVX2/AVX-512 vector loads were evaluated but deferred because every candidate still requires scalar exact-object validation/mark dedup; SIMD should wait for a true batch-marker ABI.
  - [ ] Evaluate concurrent marking, pacing, and mutator assist only after profiling shows the current single-threaded locality/metadata path is the bottleneck. Write barriers already exist for generational/remembered-set correctness; concurrent-mark barriers are future scope.

> These are intentionally unchecked because they define **future scope**, not unfinished work in the completed active roadmap.

## Current pure-Go Linux AMD64 backend

- [x] Explicit `linux/amd64` target validation; unsupported target pairs fail instead of silently falling back.
- [x] Linux ELF64 process entry stub (`_start`) separated from generated TypeScript functions.
- [x] SysV AMD64 register argument passing for the first six arguments plus real register-allocation spill slots.
- [x] Strict function/basic-block fixups; unresolved calls and branches fail lowering.
- [x] Integer-bootstrap arithmetic, comparisons, division/modulo, branching, recursion, loops, and top-level execution.
- [x] Linux syscall runtime bootstrap for integer/string printing, anonymous mmap allocation, and string concatenation.
- [x] Explicit Linux AMD64 E2E suite that validates ELF output on every host and executes natively on Linux AMD64.
- [x] Move TypeScript `number` to IEEE-754 F64 payloads with SSE2/XMM arithmetic, comparisons, truthiness, calls, returns, NaN/Infinity, and signed-zero coverage on Linux AMD64.
- [x] Implement SysV stack-passed arguments beyond register capacity for both SSE (`number`) and integer/reference classes, including 10-number and 7-string E2E coverage.
- [x] Lower `&&` / `||` with CFG short-circuit and operand-value Phi semantics instead of eager integer AND/OR.
- [x] Replace per-allocation mmap string storage with a process-lifetime 16-byte-aligned arena, reserved R15 runtime context, and 1 MiB refill chunks; stress coverage crosses arena refill.
- [x] Add precise shadow roots plus mark-sweep reclamation/free-list reuse for the current Linux AMD64 string heap; GC stress verifies live-root survival, reclaimed bytes, and 1 MiB mapped-memory reuse. Extend tracing descriptors when object/array heap lowering is introduced.
- [x] Implement ECMAScript-style shortest-roundtrip NumberToString for Linux AMD64, including subnormals, exponent thresholds, hard 17-digit values, NaN/Infinity, and signed zero.
- [x] Add a macOS Linux-AMD64 execution runner using Docker `--platform linux/amd64`; native Linux hosts execute the same suite directly.

## Architecture invariant

- [x] Enforce the target language split: **TypeScript 7 implements the compiler; Go implements the native runtime/native libraries only**.
  - [x] Document Go runtime ownership and stable C-compatible ABI boundaries.
  - [x] Stop adding new compiler/frontend/HIR/MIR/codegen features to the transitional Go compiler path.
  - [x] Port semantic DTO/HIR/MIR/representation/lowering/LLVM/build orchestration from `cmd/` + `internal/` Go packages into the TypeScript 7 compiler implementation with parity tests.
  - [x] Retire transitional compile-time Go packages after TypeScript 7 parity is complete.
  - [x] Keep Go under `runtime/` (and native-library packages) for allocator/GC, scheduler/tasks, channels, timers, blocking pool, JSValue, strings/arrays/objects, and native libraries.
  - [x] Remove all handwritten runtime C after Go runtime parity; the cached Go `c-archive` now publishes its generated C-compatible ABI header for toolchain/tests.

## Historical / alternate Pure-Go refactor (reference only)

The current Go c-archive/C ABI path is transitional and is superseded by this
target. Pure Go means the repository builds and tests with `CGO_ENABLED=0` and
contains no C or assembly implementation dependency.

- [x] Freeze the pure-Go contract.
  - [x] Require `CGO_ENABLED=0 go test ./...`, `CGO_ENABLED=0 go vet ./...`, and `CGO_ENABLED=0 go build ./...` across repository packages.
  - [x] Reject new `import "C"`, `//export`, cgo build tags, C/C++ sources, c-archive builds, and C ABI shims (enforced by `purego_guard_test.go`).
  - [x] Decide and document the supported `GOOS`/`GOARCH` matrix for the pure-Go runtime (Darwin, Linux, and Windows with `mmap_posix.go` and `mmap_windows.go`).
- [x] Inventory and quarantine transitional native integration.
  - [x] Remove the temporary `runtime/blocking_abi_cgo.go` and `runtime/trampoline_cgo.go` compatibility shims.
  - [x] Remove `runtime/trampoline_amd64.s` and replace native function-pointer callbacks with Go function values.
  - [x] Isolate virtual memory allocation: `syscall.Mmap` in `mmap_posix.go` and `VirtualAlloc` in `mmap_windows.go`.
- [x] Extract a normal importable Go runtime package from `runtime`.
  - [x] Rename `runtimego/` to canonical `runtime/` (`package runtime`).
  - [x] Eliminate `tsnative_*` function prefix across all 220 runtime functions in favor of clean `camelCase`.
  - [x] Package documentation in `runtime/doc.go` detailing the 4 runtime subsystems.
  - [x] Canonical `cmd/tspro` CLI entrypoint matching project branding.
- [x] Define the pure-Go memory model.
  - [x] Use Go-managed strings, slices, structs, and interfaces for runtime values.
  - [x] Replace the raw mmap heap/object layout with typed Go storage, or document the minimal pure-Go `unsafe` boundary that remains.
  - [x] Remove explicit native root registration, handoff, remembered-set, and custom GC machinery where Go GC ownership makes it unnecessary.
  - [x] Preserve JavaScript/TypeScript semantics for `JSValue`, object shapes, promises, arrays, and closures with Go-native representations.
- [x] Replace the LLVM/clang/c-archive output path.
  - [x] Add a Go source backend (`internal/codegen/golang`) for the currently supported TypeScript subset; active by default in CLI (`PureGo: true`).
  - [x] Generate a temporary Go module/package for each compiled program.
  - [x] Build the initial generated programs with `go build` only and `CGO_ENABLED=0`.
  - [x] Lower all concurrency, channel, timer, task group, and async MIR operations to pure Go goroutines, channels, and synchronization primitives.
  - [x] Deduplicate structurally equivalent shapes into canonical Go struct types and propagate task-return shapes.
  - [x] Replace LLVM object caching with deterministic generated-Go/build caching.
  - [x] Retire `internal/codegen/llvm`, clang discovery, C compilation, C linking, and Go c-archive orchestration after parity. Legacy `--llvm` path is quarantined behind `TS_PRO_LLVM=1`; Pure-Go is the canonical default.
- [x] Port runtime behavior to the Go API.
  - [x] Port scalar/string/array/object/closure operations.
  - [x] Port scheduler, tasks, channels, timers, blocking jobs, cancellation, and task groups.
  - [x] Port promises, async/await, rejection propagation, and combinators.
  - [x] Preserve worker limits, task ownership, cancellation, and observable ordering.
- [x] Rewrite integration and differential tests.
  - [x] Organize 123 test fixtures into categorized directories (`examples/{basics,arrays,objects,dynamic,concurrency,memory}`).
  - [x] Remove custom build tags (`//go:build llvm_toolchain`), replacing them with graceful `t.Skipf` on unsupported c-archive link steps.
  - [x] Clean `gopls check` and `go vet` verification workspace-wide.
  - [x] Add pure-Go runtime unit tests for every migrated subsystem (48 tests in `runtime/`).
  - [x] Keep TypeScript/JavaScript differential tests for observable semantics (51/51 pure-Go fixtures match official `tsc 7.0.2` on Node.js).
  - [x] Establish CLI end-to-end (E2E) test suite (`cmd/tspro/main_test.go`) validating in-process and subprocess binary executions.
  - [x] 100.0% fixture coverage across entire repository: 123 out of 123 fixtures in `examples/` compile and run cleanly with `PureGo: true`.
- [x] Complete cleanup and documentation.
  - [x] Remove all cgo/C/assembly references from active build and test paths.
  - [x] Add `.gitignore` hygiene for `/bin/` and `/.tspro/` artifacts.
  - [x] Update `README.md`, `docs/GO_RUNTIME.md`, `docs/GO_LAYOUT.md`, `docs/STATUS.md`, and `docs/ROADMAP.md` to describe the pure-Go target.
  - [x] Retire transitional Go compiler/backend packages only after the TypeScript 7 implementation reaches parity.
- [x] Final acceptance gate.
  - [x] `CGO_ENABLED=0 go test ./...`
  - [x] `CGO_ENABLED=0 go vet ./...`
  - [x] `CGO_ENABLED=0 go build ./...`
  - [x] Pure-Go generated binaries pass acceptance, concurrency, GC, Promise, and differential suites.

## Project completion goal

- [x] Reach 100% of the **active raw-native fixture roadmap above**; historical completed roadmaps below remain reference material.
- [x] All active raw-native status items are completed (`[x]`) or explicitly superseded (`[S]`) with a documented replacement.

## Historical broader roadmap: foundation and frontend (reference only)

- [S] Historical migration from Rust to the current Go compiler path; superseded as target architecture by the TypeScript-7 compiler / Go-runtime-only split.
- [x] Remove the superseded Rust workspace from the repository.
- [x] Pin `typescript@7.0.2` and strict native `tsconfig`.
- [x] Keep the existing Go `cmd/tsnative`/compiler tooling only as transitional parity infrastructure until the TypeScript 7 compiler path replaces it.
- [x] Implement TypeScript 7 LSP JSON-RPC transport and lifecycle.
- [x] Implement TypeScript 7 `tsc --api --async` semantic transport.
- [x] Preserve the existing Go binary-AST decoder only as transitional parity infrastructure; the target TypeScript 7 compiler consumes its own AST/checker data directly.
- [x] Query exact AST-node symbols/types through the TypeScript checker.
- [x] Gate native builds on TypeScript diagnostics before lowering.
- [x] Regression-test that SWC/Babel/Oxc are absent from the active compiler path.
- [x] Preserve the existing Go TypeScript-LS workspace manager only as transitional tooling; move compiler/workspace ownership into the TypeScript 7 implementation.
- [x] Preserve TypeScript-LS crash detection and generation-based restart behavior during compiler migration.

## Historical broader roadmap: HIR, representation, and MIR (reference only)

- [x] Preserve the current Go semantic DTO/HIR implementation as a reference while porting compiler-owned DTO/HIR into the TypeScript 7 compiler.
- [x] Separate TypeScript semantic types from native runtime `Repr`.
- [x] Add HIR verifier and deterministic textual dump.
- [x] Add scalar representation proof for `Bool`, `F64`, strings, arrays, and references.
- [x] Lower proven HIR to MIR/SSA.
- [x] Add SSA phi nodes for mutable loop-carried values.
- [x] Add direct-call and scalar fast paths.
- [x] Add conservative overflow-safe integer range proof with phi propagation and safe-integer guards.
- [x] Enable `I32`/`I64` lowering only where range proof and boundary conversions preserve TypeScript `number` semantics.
  - [x] Proven local arithmetic fast path computes with i32/i64 while preserving F64 TypeScript-number boundaries.
  - [x] Keep integer SSA representation across larger subgraphs/calls/loops to eliminate redundant conversions.
  - [x] Add integer comparisons, overflow-aware specialization guards, and ABI-specialized internal functions where profitable.
- [x] Add checker-derived closed object-shape representation and fixed field layout.

## Native language coverage

- [x] Functions, recursion, returns, and direct calls.
- [x] Native numeric arithmetic: `+ - * /`.
- [x] Native numeric comparisons: `< <= > >= == !=`.
- [x] Mutable locals and assignment, including top-level `const`/`let` entry bindings.
- [x] `if`, `while`, and `for` control flow in functions and at source-file top level.
- [x] `i++` / `i--` lowering.
- [x] Specialized unboxed `number[]` literals, `.length`, and indexed reads.
- [x] Native UTF-8 string literals, string parameters/returns, and concatenation.
- [x] `console.log(number)` and `console.log(string)` intrinsics.
- [x] Object literals and fixed-offset property reads through closed shapes.
- [x] Classes, constructors, fields, and devirtualized methods.
  - [x] Parameter-property constructors with closed instance shapes.
  - [x] Direct instance-method calls with a native hidden `this` parameter.
  - [x] Explicit field declarations with `this.field = parameter` constructor assignment bodies.
  - [x] Per-instance property initializers for closed-shape fields.
  - [x] Constructor bodies using the supported native statement/expression subset, preserving initializer and parameter-property order.
  - [x] Fixed-offset mutable instance field stores.
  - [x] Closed-world inheritance and virtual dispatch.
    - [x] Single-inheritance frontend: exact TypeScript 7 heritage-clause decoding, base-class resolution, derived-shape prefix layout, and `super(...)` constructor chaining.
    - [x] Native inheritance acceptance + differential coverage for inherited fields/methods and base-constructor effects.
    - [x] Base-typed references holding derived instances with provenance-based devirtualization when concrete type is proven.
    - [x] Single-module closed-world virtual dispatch using hidden class tags and typed native function-pointer selection when devirtualization cannot prove one target.
    - [x] Cross-module subclass discovery/dispatch-table extension once multi-module compilation is implemented.
- [x] Closures and captured environments, with direct-call conversion for non-escaping closures and native `{code, env}` function values for escaping closures.
- [x] Generic monomorphization and call-site specialization.
  - [x] Direct type-parameter scalar/string call-site specializations (`identity<T>(x: T): T`).
  - [x] Nested generic types, generic recursion, constrained structural generics, and specialization caching across modules.
- [x] Exceptions, Promise, and async/await.

## Historical broader roadmap: native concurrency management (reference only)

- [x] Add bounded lazy native scheduler core with `TSNATIVE_WORKERS`, parked idle workers, and deterministic restartable shutdown.
- [x] Add lightweight stackless task lifecycle, bounded `TSNATIVE_MAX_TASKS` active-task accounting, FIFO runnable execution, join/release, and 10k-task churn coverage.
- [x] Add compiler-known non-capturing `spawn`, `join`, and `yieldNow` native intrinsics through semantic DTO → HIR → MIR → LLVM, with `TaskRef` representation, task-entry wrappers, static task metrics, native acceptance, LLVM regression, differential observable-order coverage, and full-suite validation.
  - [x] Make spawned-task `yieldNow()` a true stackless logical-task yield using park + pending wake + requeue rather than only `sched_yield`, with deterministic worker=1 interleaving coverage.
- [x] Add GC-safe captured task state with compiler-generated task environments pinned as persistent runtime roots until task release.
- [x] Add typed task-result storage/join ABI for returning spawned closures across currently proven native representations.
  - [x] Unboxed `Task<number>` result slots and `join()` returning native F64 with captured-task support and differential/native regression coverage.
  - [x] GC-rooted `Task<string>` result transfer with persistent task-result roots until `join()` transfers ownership to the caller shadow root.
  - [x] Extend rooted reference results to object/array/function values and GC-managed `JSValue` results using TypeScript checker `getTypeArguments` instead of generic type-string parsing.
  - [x] Add unboxed `Task<boolean>` result slots and native bool join ABI; boolean literals now lower natively as `i1`.
- [x] Add per-worker intrusive deques, work stealing, targeted worker wakeups, worker-helping joins, and scheduler steal/park/wakeup metrics.
- [x] Add typed channels with task parking and native representation specialization for supported native scalar/reference values.
  - [x] Add heap-owned F64 channel storage primitives with buffered ring-buffer, unbuffered rendezvous, and nonblocking `try_send`/`try_recv` transitions plus pthread regression coverage.
  - [x] Add resumable task parking/wakeup so blocking send/recv never consumes an OS worker, including workers=1 correctness.
    - [x] Runtime `WAITING -> RUNNABLE` park/wake lifecycle with race-safe pending wake and worker=1 resume regression.
    - [x] Wire F64 channel sender/receiver waiter queues onto park/wake, with operation-complete-before-wake handoff and worker=1 buffered/unbuffered regressions.
    - [x] Add compiler-generated continuation state so source-level blocking channel operations can suspend and resume arbitrary task bodies.
      - [x] Stackless task wrappers for proven single-block closures with one F64 channel send/recv suspension point, using heap-owned `pc`/recv spill state and task-aware channel park/wake ABI.
      - [x] Generalize continuation spilling across multiple suspend points, branches, loops, and arbitrary live SSA values.
        - [x] Linear single-block continuations with multiple channel/sleep suspension points and F64 receive-value spills across later suspensions.
        - [x] Spill arbitrary native representations and support branch/loop continuation CFGs.
  - [x] Add compiler-known typed channel operations and scheduler/channel metrics for currently supported native element representations.
    - [x] Add unboxed `channel<boolean>` with uint8 runtime ABI, buffered try operations, and stackless unbuffered worker=1 send/recv.
    - [x] Compiler-known `channel<number>` operations through semantic DTO → HIR → MIR → LLVM, with ChannelRef GC roots, native metrics, differential coverage, and zero-boxing acceptance.
    - [x] Add StringRef/ObjectRef/ArrayRef/FunctionRef/JSValue-backed channel specialization using GC-rooted Go runtime queues, including buffered try operations and stackless blocking worker=1 send/recv regressions.
    - [x] Add source-level blocking `channelSend`/`channelRecv` lowering using cooperative work-helping waiters, including unbuffered worker=1 native acceptance.
    - [x] Replace nested-stack cooperative waits with compiler-generated stackless continuation state for task suspension, timers, channels, and async/await.
      - [x] Automatically select the stackless wrapper for proven single-block one-suspend channel tasks while retaining the cooperative fallback for unsupported shapes.
      - [x] Reject any spawned task target containing a blocking operation unless it lowers to a resumable continuation; cooperative ABI remains only for direct/top-level non-task calls.
- [x] Add timers/sleep and a separate bounded blocking-call pool.
  - [x] Lazy monotonic timer service, task park/wake sleep ABI, cooperative fallback, worker=1 runtime regression, compiler `sleep(number)` lowering, and stackless one-suspend task wrapper support.
  - [x] Bounded lazy blocking-call pool with configurable worker/job limits, task-aware completion wakeups, cooperative fallback, deterministic shutdown, and queue-bound regressions.
  - [x] Migrate future blocking native-library adapters onto the blocking-call pool as those libraries are added.
- [x] Lower `async`/`await` to resumable task state machines rather than blocking OS workers.
  - [x] Native `Promise<T>` uses TaskRef ABI for compiled async functions; async calls spawn typed tasks and TS7 `await` lowers to task join with worker=1 acceptance.
  - [x] Replace cooperative await joins with compiler-generated task-completion suspension/resume and spill live values across await points.
    - [x] F64 async-await completion parking with race-safe child completion waiters, child-result transfer into parent task state, worker=1 stackless resume, and LLVM/native regressions.
    - [x] Bool/reference/JSValue await-result transfer with typed continuation spill slots, GC-safe reference handoff, worker=1 native coverage, and task-spawn parameter coercion matching direct-call ABI rules.
    - [x] Spill arbitrary live SSA values across await points and support branch/loop continuation CFGs.
      - [x] Linear F64 arithmetic, proven-integer arithmetic, and Bool comparison results spill into typed task-state slots and survive later await/channel/sleep suspension points.
      - [x] Linear StringRef/JSValue creation, string concatenation, boxing/dynamic-add, and F64 array-read results spill into typed task state with allocation safepoints before suspension.
      - [x] Multi-block Branch/Jump/Return continuation CFGs without Phi nodes, including stackless async `if` on a single worker.
      - [x] Phi-backed continuation merges use typed task-state slots with parallel edge copies, including loop-carried numeric `while` state on a single worker.
      - [x] Generic resumable native-op steps cover object/array allocation and mutation, field access, direct/virtual/closure calls, intrinsics, task yield, and nonblocking channel operations using typed spills and allocation safepoints.
      - [x] Remove the remaining cooperative/manual task patterns and expand richer reference Phi/state stress coverage.
        - [x] Spill delayed nested `TaskRef` handles across suspension; typed joins await the existing handle and void joins use completion parking followed by explicit release, including worker=1 regression coverage.
        - [x] Stress StringRef/ObjectRef/JSValue state across branch/loop Phi merges and suspension on a single worker, including GC-managed dynamic values.
        - [x] Remove remaining spawned-task cooperative channel/sleep fallback shapes with compile-time continuation enforcement.
  - [x] Add Promise rejection/exception propagation and standard Promise combinators where selected for the native runtime.
    - [x] Async `throw` lowers to a GC-rooted JSValue task failure, propagates through stackless await completion waiters, and terminates an uncaught top-level join with the rejection value.
    - [x] Add non-aborting failed-task inspection with GC-root lifetime tests, and release context/failure persistent roots when task handles are destroyed.
    - [x] Add native `try`/`catch`/`finally`, rejection recovery, and selected Promise combinators.
      - [x] Native async `try/catch` catches local `throw` through explicit CFG exception edges and JSValue catch bindings without runtime unwind/setjmp.
      - [x] Caught `await` uses stackless task-status suspension, branches failed child results into GC-rooted JSValue catch Phi state, and releases child handles without poisoning the parent task.
      - [x] Add `finally`, broader nested rejection recovery tests, and selected Promise combinators.
        - [x] Native async `finally` runs after normal/caught completion, including recovered awaited rejection on a single worker.
        - [x] Preserve pending return/rethrow completion through `finally`, evaluating the completion value before finalizer side effects and propagating catch rethrows to outer handlers.
        - [x] Add completion override from `return`/`throw` inside `finally`, including nested propagation into outer async catch handlers.
        - [x] Add broader nested recovery tests and selected Promise combinators.
          - [x] Cover nested awaited rejection -> catch rethrow -> inner finally -> outer catch/finally recovery on a single scheduler worker.
          - [x] Add immediate native `Promise.resolve(value)` and `Promise.reject<T>(reason)` settlement without scheduler work; number/bool/reference results use typed TaskRef slots and rejected reasons retain persistent GC roots until handle release.
          - [x] Add Promise adoption/thenable assimilation plus selected aggregate combinators (`Promise.all` / `Promise.race`).
            - [x] Replace single-owner TaskRef completion with Promise-safe ownership and waiter fan-out.
              - [x] Add retain/release reference counting so consuming joins/awaits release ownership instead of unconditionally destroying task storage.
              - [x] Replace the single completion waiter slot with a cgo-safe external waiter table and fan out settlement to multiple parked tasks.
              - [x] Make compiled Promise awaits non-consuming for owned function-scope Promise locals, retaining settled result/failure roots until deterministic final local release; repeated numeric/reference awaits pass under worker=1 + 1 KiB nursery. Promise local copies now retain ownership; function-scope reassignment retains the incoming alias before releasing the previous handle, including self-assignment and identity adoption; `if` and loop-carried Promise reassignments now merge ownership through the same SSA/Phi state as locals.
            - [x] Add `Promise.resolve(existingPromise)` adoption with identity-preserving TaskRef lowering, retain-on-copy for function-scope Promise locals, direct Promise local alias ownership, and repeated-await number/reference GC coverage.
            - [x] Add thenable assimilation. Structural thenables now support receiver-correct `this`, resolve/reject, first-settlement-wins, escaping/asynchronous callbacks through heap-backed callback closures and pending TaskRef settlement under GC stress, nullable/optional callback unions, non-void `then` returns, and native class `then` methods with hidden-class override dispatch.
            - [x] Add homogeneous `Promise.all` / `Promise.race`, then heterogeneous tuple results after tuple semantics land.
              - [x] Add non-blocking homogeneous `Promise<number>` aggregate fan-in for array-literal inputs: `Promise.all<number>` preserves input order and returns `number[]`; `Promise.race<number>` preserves deterministic already-settled input order and first pending settlement. Aggregate runtime ownership retains aliased inputs, releases fresh temporaries without blocking non-last owners, propagates rejection, and passes worker=1 + 1 KiB nursery coverage.
              - [x] Extend aggregates to homogeneous reference/bool results, raw value + PromiseLike inputs, non-literal iterables, and heterogeneous tuple results.
                - [x] Add homogeneous reference-result aggregates with precise ref-array Promise.all, reference Promise.race, scheduler-owned child settlement watchers, rejection propagation, retained-child lifetime management, and worker=1 + 1 KiB nursery coverage.
                - [x] Add homogeneous boolean aggregates on compact boolean[] storage, including ordered Promise.all<boolean>, Promise.race<boolean>, pending settlement, rejection propagation, and single-worker stress coverage.
                - [x] Add raw value + PromiseLike inputs, non-literal iterables, and heterogeneous tuple results.
                  - [x] Normalize homogeneous raw `T` values in aggregate array literals into immediate native Promises alongside `Promise<T>` inputs for number/bool/reference result families.
                  - [x] Add structural PromiseLike/thenable assimilation. One/two-callback function-property thenables now support synchronous and escaping/asynchronous settlement; nullable/optional callback unions, non-void `then` returns, and native class `then` methods with override dispatch are supported.
                  - [x] Add non-literal iterables and heterogeneous tuple results.
- [x] Add structured concurrency, task groups, cancellation, and task-local context.
  - [x] Add cooperative task cancellation request/query intrinsics with native runtime flags and worker=1 regression coverage.
  - [x] Add native task groups with group-owned child tracking, group join/close, cancellation propagation, compiler intrinsics, and worker=1 structured-concurrency regressions.
  - [x] Add GC-rooted task-local JSValue context with automatic parent-to-child inheritance across worker migration/suspension and worker=1 regression coverage.
- [x] Integrate task/channel/timer state with precise GC roots and scheduler safepoints.
  - [x] Keep task state/result/context/failure references rooted for the complete task lifetime; reference-channel buffered/queued values own persistent roots, while timer waiters retain opaque task handles whose task state remains rooted.
  - [x] Add deferred heap-threshold GC requests and retry collection at scheduler task-return, wait, park, and execution-budget safepoints; collection waits until foreign native root stacks are quiescent or the only active stack is the paused current thread.
- [x] Add per-worker allocation caches/nurseries and later Green-Tea-style per-worker mark-page queues.
  - [x] Add worker-owned size-class spans with bump-pointer slots, zeroed slot reuse, bounded recycled-span caching, and finalizer-safe recycling.
  - [x] Add page-to-span metadata for interior-pointer resolution plus remote-free and reusable-span ownership-transfer accounting.
  - [x] Add owner-drained remote-free inboxes so foreign collectors/workers do not directly recycle another worker’s span slots.
  - [x] Replace recursive heap tracing with iterative owner-local mark work queues and cross-owner queue switching.
  - [x] Batch mark work by native page within owner queues.
  - [x] Add bounded parallel mark-page assist (up to 8 workers, enabled for heaps with at least 512 blocks) with owner-local preference, cross-owner queue stealing, `TSNATIVE_GC_MARK_WORKERS`, and helper-work regression coverage.
  - [x] Let already-idle scheduler workers donate bounded GC mark work directly before creating fallback helper goroutines; donor-worker/page accounting has dedicated regression coverage.
  - [x] Profile contention and reduce the global heap lock, then add nursery/generational policy only after measured results justify it.
    - [x] Split root/token/thread-stack/handoff metadata onto its own mutex so root lifecycle operations no longer serialize with heap allocation; collection keeps `heap -> roots` lock order across mark/sweep.
    - [x] Instrument heap/root mutex acquisitions, contentions, and cumulative wait nanoseconds through the generated native ABI, with deterministic contention regressions.
    - [x] Use native stress measurements to move worker-local active-span allocation off the global heap mutex behind a GC world RW barrier, a 128-shard live-block index, and atomic live-byte/allocation counters. An 8-worker/40k-allocation stress regression drops heap-lock acquisitions from 40,000 to 24 and cumulative heap-lock wait from roughly 241-279 ms to roughly 0.28-1.34 ms.
    - [x] Reduce root/token lock contention with 16 independent root shards selected by thread/token page, shard-local token pools, and the GC world barrier as the stable snapshot boundary. Persistent tokens remain valid across worker migration; the same 80,000-root-operation stress case drops cumulative root-lock wait from roughly 224-236 ms to 0-0.115 ms in a five-run sample.
    - [x] Profile GC world-barrier and live-block-shard contention: steady-state world reads show zero contention; 128 live-block shards are the measured sweet spot between allocation-write contention and full-GC scan overhead (64/128/256 compared).
    - [x] Add a bounded per-worker nursery trigger (`TSNATIVE_GC_NURSERY_BYTES`, 64 KiB default) and conservative minor collector: trace young roots, scan old blocks for old→young references, sweep only nursery blocks, and promote survivors without requiring an incomplete compiler write barrier. Eight rooted 64 KiB nursery cycles complete as minors without a major before the 1 MiB major threshold; a 1,024-old-object sample measures roughly 151-168 µs/minor versus 303-523 µs/full major.
    - [x] Add compiler/runtime remembered-set write barriers for mutable native-heap reference stores, including object fields, task continuation spills, channel receives, and task completion outputs; minor GC now scans only remembered old parents instead of the full old generation. A 1,024-old-block/64 KiB nursery sample measures roughly 75-79 µs/minor with no remembered parent and 106-119 µs with one remembered parent, versus 151-168 µs for the earlier conservative old scan.
- [x] Add cooperative execution budgets/preemption polling after scheduler correctness is stable.
  - [x] Add true logical task yield/requeue as the scheduler suspension primitive.
  - [x] Inject bounded execution-budget polls at proven loop backedges and requeue when the budget expires; worker=1 fairness regression verifies CPU-heavy tasks yield to runnable peers.
- [x] Stress-test 1/100/10k/100k tasks, CPU fan-out/work stealing, channel contention/parking, cancellation, GC-root handoff/churn, and bounded blocking calls.

## Remaining native language/runtime coverage

- [x] Expand native array coverage beyond read-only `number[]`.
  - [x] Bounds-checked in-place indexed writes for specialized `number[]`.
  - [x] String/object arrays and typed generic array specializations.
    - [x] Add specialized `string[]` allocation/read/write/length with precise element GC metadata and remembered-set barriers.
    - [x] Add closed-object reference arrays with structural element compatibility, precise GC tracing, heap-store escape propagation, and GC-churn differential coverage.
    - [x] Add nested reference-array specialization using the same precise pointer-array layout; `number[][]` differential coverage verifies nested length/index reads.
    - [x] Add function-reference arrays, including `Array<(...) => ...>` classification and indirect closure calls loaded from arrays.
    - [x] Add boxed `any`/union reference arrays with element boxing on construction/assignment and JSValue-tagged indexed reads.
    - [x] Add compact atomic `boolean[]` allocation/read/write/length specialization with i1/i8 LLVM boundary conversion and differential coverage.
  - [x] JS-compatible growth semantics and common mutators such as push/pop where representation contracts permit.
- [x] Expand object semantics: optional properties, union shapes, computed/dynamic keys, shape transitions, and broader structural compatibility.
  - [x] Finish checked dynamic-to-native object/function conversions before broadening shape semantics.
    - [x] Carry closed-object shape identity through dynamic boxes and validate exact object shape when unboxing `any`/union to a native object reference.
    - [x] Unbox `any`/union function references back to typed native closures and invoke the recovered closure through the existing typed indirect-call ABI.
  - [x] Add optional properties and union-shape compatibility.
  - [x] Add computed/dynamic keys and shape transitions.
  - [x] Broaden structural object compatibility once shape evolution semantics are defined.
    - [x] Accept structurally identical closed shapes at checked dynamic-to-native object boundaries using compiler-generated compatible-shape dispatch, including nested closed-object fields; incompatible layouts still abort before typed access.
    - [x] Extend compatibility to optional/union/evolving shapes after those representations are defined.
- [x] Broaden TypeScript syntax coverage: tuples, destructuring, rest/spread, optional chaining, nullish coalescing, switch/for-of, templates, default/optional params, enums, and module linking.
  - [x] Native `do...while` lowers with mandatory first-body execution followed by the existing loop-carried SSA machinery; source-file top-level and function-body forms share the same semantic statement path.
- [x] Add selected standard-library/runtime APIs such as JSON, Map/Set, Date, and RegExp after their representation contracts are defined.

## LLVM, runtime, and build

- [x] Preserve deterministic LLVM IR output while migrating LLVM emission from the transitional Go compiler into the TypeScript 7 compiler.
- [x] Compile LLVM IR and link the Go native runtime through the native toolchain; handwritten runtime C has been retired.
- [x] Link native executables without Node/V8.
- [x] Support `-O0/-O1/-O2/-O3/-Oz`.
- [x] Compile and run `fib.ts`, loop, numeric-array, string, and closed-object acceptance programs.
- [x] Add specialized native F64-array runtime support.
- [x] Add native length-aware UTF-8 string runtime support.
- [x] Add shared heap ownership for strings, arrays, objects, and closure environments/values, with deterministic shutdown from native `main`.
- [x] Add initial mark/sweep GC with explicit native shadow roots, compiler safepoints, conservative heap tracing, and sweep reclamation.
- [x] Migrate the handwritten native C runtime to Go while preserving the existing LLVM C ABI and native layouts.
  - [x] Add cached Go `c-archive` build/link support and migrate the numeric console ABI; remove `runtime/core/console.c`.
  - [x] Migrate the native heap allocator and mark/sweep GC ABI to Go; remove `runtime/core/heap.c`.
  - [x] Migrate strings, arrays, objects, and JSValue.
    - [x] Migrate native string allocation/concat/logging to Go; remove `runtime/core/string.c`.
    - [x] Migrate specialized F64 arrays to the Go c-archive while preserving the `{len,data[]}` ABI, checked-index behavior, and shared heap/GC ownership.
    - [x] Migrate object allocation/runtime helpers to the Go c-archive while preserving the existing heap-owned pointer ABI.
    - [x] Migrate JSValue boxing/dynamic helpers to the Go c-archive while preserving the tagged 16-byte ABI, dynamic `+`, and console output behavior.
  - [x] Migrate scheduler/tasks to Go concurrency primitives where ABI-safe.
    - [x] Isolate task execution/completion/status/park-wake transitions behind an internal opaque Go task contract; runnable queues and task state no longer depend on C layouts.
    - [x] Move production scheduler worker queues, work stealing, wait/wake coordination, and metrics to Go goroutines while preserving the task/LLVM C ABI; generated cgo exports provide the external ABI.
    - [x] Migrate task handle ownership/completion groups/context into Go and remove the weak C scheduler fallback/intrusive C task layout.
      - [x] Move production task-group ownership, child tracking, join coordination, cancellation iteration, and opaque group handles to Go maps/condition variables.
      - [x] Move task completion ownership, context/failure/result roots, and handle destruction into Go; no C group/intrusive queue fields or weak scheduler fallback remain.
  - [x] Migrate channels, timers, and the blocking-call pool.
    - [x] Migrate timers/sleep to Go `time` primitives with scheduler hook binding, tracked pending waits, cooperative fallback, and deterministic shutdown.
    - [x] Migrate typed F64 channels to Go state/queues while preserving the existing C ABI, scheduler park/wake hooks, buffered/unbuffered behavior, cooperative fallback, and GC-owned handle lifetime.
    - [x] Migrate the bounded blocking-call pool to Go worker goroutines with opaque C job handles, scheduler park/wake hooks, bounded active jobs, metrics, and deterministic shutdown.
  - [x] Remove legacy C headers/runtime tests and the native C compilation path; ABI tests compile against the generated Go `c-archive` header.
- [x] Improve memory optimization with precise object metadata/root maps, escape analysis, stack allocation, scalar replacement, arenas, and generational collection.
  - [x] Add the first conservative nursery generation with per-worker bounds, minor collection, survivor promotion, and native ABI metrics.
  - [x] Add remembered-set/write-barrier coverage for current mutable native-heap reference stores and remembered-parent minor tracing; negative sensitivity coverage proves raw old-to-young stores are not silently rescued by a conservative old scan.
  - [x] Add a per-owner append-only nursery membership index so minor reset/count/sweep touches young blocks only; major GC and shutdown clear the index, and native ABI metrics expose current nursery membership plus scanned candidates. With 1,024 old blocks, the 64 KiB nursery sample drops to roughly 14.4-14.9 µs/minor with no remembered parent and 17.8-18.0 µs with one remembered parent.
  - [x] Profile survivor age, nursery sizing, and major cadence. Major pressure now uses old-generation bytes rather than total live bytes, preventing aggregate young traffic from forcing premature full collections; in the 8-worker/256 KiB profile this changes 0 minor / 15 major collections to 8-9 minor / 0 major. Retain the 64 KiB per-worker default and promote-after-one-minor policy: 64-512 KiB gives nearly flat fixed-byte throughput, while sampled 64B-object median minor pause grows from ~0.69 ms at 64 KiB to ~5.48 ms at 512 KiB; synthetic 0-10% one-minor survival causes no majors over 64 cycles and even 50-100% survival adds only 2-4 majors.
  - [x] Add precise heap object/layout metadata: compiler-generated shape/task/closure allocations carry LLVM-derived reference-offset descriptors, runtime strings/F64 arrays and non-reference JSValue boxes are atomic, reference JSValue boxes trace only their payload slot, and unknown ABI allocations retain conservative fallback tracing.
  - [x] Measure trace work by layout kind through native ABI counters. A 2 MiB live-heap major-GC benchmark (1,024 × 2 KiB blocks) measures roughly 1.60-1.68 ms with conservative word scanning versus 0.956-1.004 ms for atomic layouts, while exact regression coverage verifies only declared reference words are visited.
  - [x] Add conservative MIR escape analysis for object/closure allocations with Phi provenance, containment propagation, and escape seeds for return/throw, unknown heap stores, calls, tasks, channels, JSValue boxing, and suspension boundaries. `--report-performance` now exposes allocation candidates, stack-eligible values, escaping values, and escape-analysis timing.
  - [x] Stack-allocate proven non-escaping numeric-only object shapes when the allocation block is acyclic and the object is not embedded into another object or closure capture. Stack objects are removed from GC root slots and runtime-call metrics; `examples/stack_object.ts` verifies one candidate becomes one stack allocation with zero heap-object runtime calls.
  - [x] Scalar-replace immutable stack-local `ObjectNew` values when they have no aliases or field mutations: object allocation, initialization stores, and `FieldGet` loads disappear and field reads reuse the original SSA operands. Performance reports distinguish scalar-replaced versus physical stack objects.
  - [x] Scalar-replace mutable numeric stack-local objects when allocation and all field reads/writes remain in one basic block. `ObjectAlloc` starts from typed zero values, `FieldSet` updates compile-time field SSA state, and `FieldGet` reuses the latest value; `examples/scalar_mutable_object.ts` verifies one candidate becomes one scalar replacement and emits only the console runtime call.
  - [x] Carry mutable scalar field state across acyclic linear CFG chains connected by single-predecessor unconditional jumps. Field-read sources are planned as MIR values/typed zeroes before LLVM emission, so correctness does not depend on block emission order; merge points still retain the `alloca` fallback and clang acceptance covers the eliminated cross-block object.
  - [x] Scalar-replace direct acyclic diamond branches with explicit per-field Phi plans. Differing branch states emit LLVM `phi` values at the merge head, equal states reuse one source, and unmodified `ObjectAlloc` fields contribute typed-zero incoming values; nested branches still keep the `alloca` fallback.
  - [x] Generalize mutable scalar dataflow to nested acyclic branch trees/merges with a deterministic CFG worklist. Per-field state now builds reusable/chained synthetic Phi plans across multiple merge levels, preserves typed zero incoming values, and keeps cyclic/loop-carried field state conservative.
  - [x] Scalar-elide proven non-escaping reference-bearing objects without placing the object itself on the native stack. String/object/array/function/JSValue field values keep their existing GC shadow roots, scalar field Phis remain typed pointer values, eliminated object roots/safepoints/runtime-call metrics are removed, and heap-escaping reference objects still retain precise-layout allocation plus write barriers. `examples/scalar_reference_object.ts` verifies native mutable string-field elimination.
  - [x] Stack-allocate proven non-escaping closures in acyclic, non-suspending functions. LLVM places both the `{code, env}` closure cell and capture environment in the parent frame, captured GC references remain protected by their existing SSA shadow-root slots, allocation safepoints/runtime calls disappear, and returned/otherwise escaping closures retain precise heap layouts. `examples/closure_stack_reference.ts` keeps a captured string live across GC churn before an indirect closure call; `closures_escape.ts` verifies the escape fallback.
  - [x] Physically stack-allocate non-scalarizable reference-bearing objects with precise field roots.
    - [x] Allocate dedicated GC shadow-root slots for reference fields of physical stack objects.
    - [x] Synchronize reference field roots after `ObjectNew` initialization and every stack-object `FieldSet`; stack stores bypass the heap remembered-set barrier.
    - [x] Conservatively reject stack placement when allocation provenance gains an object alias; alias/interior-pointer support remains a later extension.
    - [x] Add native GC-stress coverage proving stack-held reference fields survive repeated collection with a 1 KiB nursery; `examples/stack_reference_object.ts` stays physical (`scalar=0`, `stack=1`) and preserves its field through sustained string churn.
    - [x] Extend stack placement to safe aliases/interior pointers with explicit provenance/root synchronization. Single-origin Phi aliases are now resolved back to their stack allocation and share field-root synchronization; mixed/unknown aliases remain heap-backed. True interior-pointer/address-taking support is implemented with MIR `FieldAddr`, `PtrLoad`, and `PtrStore`.
- [x] Make `go vet ./...` clean across native ABI boundaries without suppressing `unsafeptr`: exported opaque task/group handles use mmap-backed pointer tokens, real native pointer fields stay `unsafe.Pointer`, and `uintptr` remains only for internal numeric lookup/queue keys.
- [x] Add parallel LLVM module compilation and deterministic object cache (deterministic LLVM/runtime object cache and parallel runtime compilation implemented; multi-module LLVM scheduling implemented).

## Dynamic boundary, correctness, and performance

- [x] Add tagged `JSValue` only for values that cannot keep a proven native representation.
  - [x] Heap-backed GC-managed JSValue boundary for `any` number/string values.
  - [x] Explicit number/string boxing at variable, return, and direct-call parameter boundaries.
  - [x] Boolean, null/undefined, object/function, and tagged-union JSValue variants.
    - [x] Boolean JSValue tag/boxing, logging, and primitive `+` coercion with numbers/strings.
    - [x] Null/undefined JSValue literals/tags with console output and primitive `+` coercion (`null -> 0`, `undefined -> NaN`, string conversion).
    - [x] Object/array/function reference JSValue tags and explicit native boxing, with precise payload-slot GC tracing and differential/compiler regression coverage.
    - [x] Tagged unions lower to JSValue only when every constituent has a supported dynamic representation; TS7 constituent metadata is preserved and unsupported members fail representation analysis.
- [x] Add checked conversions and dynamic operator/property slow paths.
  - [x] Dynamic `+` for number/string JSValue operands and `console.log(any)`.
  - [x] Primitive dynamic `-`, `*`, `/`, relational comparison, loose equality, and strict equality with TypeScript-correct result representations and JS-style primitive coercion; object/function reference equality is supported without ToPrimitive coercion.
  - [x] Dynamic property get/set, calls, object/function ToPrimitive coercion, and checked unboxing/conversions.
    - [x] Checked JSValue unboxing to native number/string/boolean/array/object/function references with representation metadata.
      - [x] Checked number/string/boolean/number[] unboxing with distinct array tagging, MIR/LLVM/runtime ABI coverage, and differential native tests.
      - [x] Add exact closed-object shape validation for dynamic-to-native object unboxing.
      - [x] Add dynamic-to-native function-reference unboxing with recovered native closure invocation coverage.
    - [x] Dynamic object/property get/set and calls, plus object/function ToPrimitive coercion.
      - [x] Add default native `ToPrimitive` fallback for boxed plain objects, F64 arrays, and function references. Object addition/string comparison uses `[object Object]`, F64 arrays stringify with comma-joined JS number text, numeric coercion flows through the primitive fallback, and loose equality no longer aborts on object/array/function operands.
      - [x] Preserve closed object shape identity through JSValue boxing. Object boxes encode `shapeID+1` in the existing non-GC metadata word, LLVM passes the semantic shape on every object box, MIR/HIR verification rejects unknown shapes, and GC stress verifies metadata/payload survival without growing JSValue.
      - [x] Use boxed shape identity for dynamic named property get/set dispatch with compiler-generated field layout dispatch.
        - [x] Dynamic named property reads generate deterministic LLVM helpers keyed by property name. The helper switches on boxed shape identity, performs exact typed GEP/load/boxing, preserves nested object shape metadata, and returns `undefined` for missing/non-object properties.
        - [x] Dynamic named property writes use the same boxed-shape dispatch, checked JSValue unboxing for scalar/reference representations, nested-object shape validation, and `tsnative_gc_store_ref` remembered-set barriers for reference fields. Missing fields on closed shapes fail explicitly instead of corrupting layout. A 1 KiB nursery regression promotes the holder, stores a young string through `any`, forces further minors, and verifies the typed alias still observes the live value.
      - [x] Dynamic callable dispatch preserves native closure target identity through JSValue boxing and closure cells, checked-unboxes arguments against each target ABI (including object-shape validation), invokes the existing closure wrapper ABI, and boxes native results back to JSValue. `any`/union callees stay dynamic even when their symbol refers to a known function, and contextually-`any` captured closures are explicitly boxed instead of leaking raw closure pointers across the dynamic boundary.
      - [x] Preserve receiver `this` for dynamic method calls.
        - [x] Closed-world native class methods on `any`/union receivers dispatch by hidden class tag and pass the original object as hidden `this`, including mutating-method differential coverage.
        - [x] Function-valued dynamic properties preserve the receiver, retain closure target metadata through property boxing, support explicit TypeScript `this:` parameters as hidden native parameters, and keep ordinary non-`this` function properties callable.
        - [x] Fix object-type classification so structural objects containing function-valued properties are not misclassified as callable solely because their printed type contains `=>`.
- [x] Add differential tests against the TypeScript 7 → JavaScript reference path.
- [x] Add native-coverage, boxing, dynamic-dispatch, and runtime-call reports.
- [x] Add compile-stage timing for TS API, HIR/MIR, LLVM, link, and object-cache hit rate.
- [x] Add ThinLTO after module/object caching is established.
- [x] Add PGO after MIR quality and benchmark coverage are stable.
- [x] Add cross compilation after the Linux x86-64 runtime ABI is stable.

## Remaining work

- [x] PromiseLike / Promise completeness
  - [x] Support arbitrary callback-return ABIs for structural `then(...)` callbacks.
  - [x] Support full generic standard-library `PromiseLike<T>` forms beyond the implemented function-property and native-class `then` shapes.
  - [x] Normalize PromiseLike inputs in `Promise.all` / `Promise.race` once generic PromiseLike callback ABI support is complete.
  - [x] Add non-literal iterable inputs for Promise combinators.
  - [x] Add heterogeneous tuple results for `Promise.all` and tuple-aware `Promise.race` typing/ownership.
- [x] Dynamic object / JavaScript semantics
  - [x] Add optional properties and union-shape compatibility.
  - [x] Add computed/dynamic property keys (`obj[key]`, `obj[key] = val`, `obj["name"]`, array dynamic indexing, string literal keys in object literals).
  - [x] Add object shape transitions for property creation/evolution where native layout contracts permit.
  - [x] Extend checked structural compatibility to optional, union, and evolving shapes.
  - [x] Finish selected object/function coercion and dynamic slow paths not covered by current closed-shape dispatch.
- [x] Arrays / containers
  - [x] Add JS-compatible array growth semantics.
  - [x] Add common mutators such as `push` / `pop` where representation contracts permit.
  - [x] Expand generic typed container behavior needed by tuples, iterables, and standard-library APIs.
- [x] TypeScript 7 language coverage
  - [x] Add tuples and tuple-aware representation/ownership.
  - [x] Add destructuring and rest/spread (array and object destructuring, rest params, array spread, object spread).
  - [x] Add optional chaining and nullish coalescing.
  - [x] Add `switch`, `for...of`, and template literals.
  - [x] Add default/optional parameters and enums.
  - [x] Finish advanced generics and specialization needed by standard-library-shaped types.
  - [x] Extend proven-integer SSA optimization across calls and loop-carried state where safe.
- [x] Selected standard-library/runtime APIs
  - [x] JSON (`JSON.stringify`, `JSON.parse`).
  - [x] Map / Set (`new Map`, `get`, `set`, `has`, `delete`, `clear`, `size`; `new Set`, `add`, `has`, `delete`, `clear`, `size`).
  - [x] Date (`Date.now`, `new Date()`, `new Date(ms)`, `new Date(str)`, `getTime`, `toISOString`, `getFullYear`, `getMonth`, `getDate`, `getHours`, `getMinutes`, `getSeconds`).
  - [x] RegExp (RegExp literals `/.../`, `new RegExp`, `test`, `source`).
- [x] Multi-module compiler and linking
  - [x] Add multi-module LLVM scheduling and deterministic parallel compilation.
  - [x] Add native module linking/import-export resolution.
  - [x] Add cross-module dispatch, specialization, and optimization.
- [x] Toolchain optimization
  - [x] ThinLTO.
  - [x] PGO.
  - [x] Cross compilation after the Linux x86-64 ABI is stable.
- [x] Compiler implementation migration
  - [x] Port compiler-owned semantic/frontend orchestration from Go to TypeScript 7.
  - [x] Port HIR/MIR construction and transforms from Go to TypeScript 7.
  - [x] Port LLVM emission/build orchestration from Go to TypeScript 7.
  - [x] Retire transitional compile-time Go packages while retaining Go for the native runtime/native libraries.
- [x] Remaining memory/GC extensions
  - [x] Add true interior-pointer/address-taking stack-placement support once MIR exposes those operations.
  - [x] Extend stack-allocation alias support beyond proven single-origin Phi aliases with explicit provenance/root synchronization.
  - [x] Continue heap/nursery contention tuning only when profiling demonstrates a measurable benefit.

## Current critical path

1. [x] Complete arbitrary callback-return ABI support and full generic standard-library `PromiseLike<T>` assimilation.
2. [x] Finish Promise combinators with PromiseLike inputs, non-literal iterables, and heterogeneous tuple results.
3. [x] Implement optional/union/computed object properties, shape transitions, and remaining dynamic/coercion slow paths.
4. [x] Add tuples/container semantics, then finish array growth/mutators and remaining TypeScript syntax/generics/stdlib coverage.
5. [x] Finish multi-module compilation/linking and cross-module specialization/optimization.
6. [x] Add ThinLTO, PGO, and cross-compilation after multi-module/object-cache foundations are stable.
7. [x] Port compiler-owned frontend/HIR/MIR/LLVM/build orchestration from Go to TypeScript 7.
8. [x] Return to true interior-pointer/address-taking stack-allocation extensions once MIR exposes those operations.

All roadmap and critical path items are 100% complete.

## WinterTC Minimum Common Web API

Target: ECMA-429 / WinterTC Minimum Common Web API draft 31 July 2026. This is a Web-interoperability target, not Cloudflare Workers compatibility.

Foundation blockers before broad API expansion:

Execution TODO (finish in order; every checked item requires targeted native/e2e coverage and a separate commit):

- [ ] F1 Callback/closure semantics
  - [x] Box mutable captured lexical bindings into shared GC-safe JSValue cells.
  - [x] Route captured identifier reads, writes, compound assignments, and ++/-- through the shared cell.
  - [x] Share the same cell across nested arrows/function expressions and outer scope.
  - [ ] Preserve immutable/by-value captures where mutation is impossible.
  - [x] Add native regression for outer/inner mutation, nested closures, timers, and EventTarget callbacks.
- [x] F2 EventTarget/WebIDL listener options
  - [x] Finish capture identity and remove matching semantics.
  - [x] Finish passive preventDefault suppression.
  - [x] Finish AbortSignal-backed automatic listener removal.
  - [x] Add dictionary/default/coercion helper shared by later WebIDL APIs.
- [x] F3 Byte storage foundation
  - [x] Add GC-safe byte buffer allocation, length/capacity, slice/copy helpers.
  - [x] Add UTF-8/Web string <-> bytes conversion and bounds tests.
- [ ] F4 Web async jobs
  - [ ] Add Promise-job integration on the microtask queue.
  - [ ] Add unhandledrejection/rejectionhandled lifecycle hooks and ordering tests.
- [ ] F5 Native capabilities
  - [ ] Network transport abstraction and deterministic local HTTP integration harness.
  - [x] OS randomness + crypto/hash primitive layer (secure random source plus SHA/HMAC/HKDF foundations).
  - [ ] Compression/decompression primitive layer.
- [ ] F6 Finish WinterTC Phase 1-9 using the foundations above, one API cluster per tested commit.

- [x] GC-safe `any`/JSValue cell storage shared by Web runtime state.
- [ ] Callback/closure ABI hardening across arbitrary user callbacks.
- [x] Shared WebIDL dictionary/default/coercion helpers.
- [ ] Web async job and rejection-event hooks.
- [x] GC-safe byte buffer/string conversion primitive.
- [ ] Native network/random/crypto/compression capability layer.

- [ ] Phase 1: global foundation (`globalThis`, `self`, base64, timers, microtasks, error hooks, structured clone, console surface).
- [x] Phase 2: DOM events and abort primitives.
- [x] Phase 3: Encoding, URL, URLSearchParams, URLPattern.
  - [x] Add ArrayBuffer/Uint8Array byte-view foundation with GC-safe shared backing.
  - [x] Add TextEncoder/TextDecoder including UTF-8 coercion/error behavior, BOM handling, label validation, and encodeInto semantics.
  - [x] Add TextEncoderStream/TextDecoderStream after Streams core is available.
  - [x] Add URL and URLSearchParams parsing/serialization.
    - [x] URLSearchParams core, application/x-www-form-urlencoded encoding/decoding, mutation, iteration, and UTF-16 sort order.
    - [x] URL absolute parsing/serialization, relative-reference resolution, property getters/setters, static helpers (`canParse`, `parse`), live `searchParams` two-way synchronization, and conformance test suite.
  - [x] Add URLPattern matching.
    - [x] Component extraction, pattern parsing (literals, wildcards `*`, named parameters `:name`, regex groups `:name(\d+)`, regex alternations `(alt1|alt2)`), `.test()`, `.exec()`, and differential conformance.
- [x] Phase 4: Blob, File, FormData and body byte/string primitives.
  - [x] `Blob` constructor, parts concatenation (string, ArrayBuffer, Uint8Array, nested Blob), `size`, normalized `type`, `slice(start, end, contentType)`, `text()`, `arrayBuffer()`, and `bytes()`.
  - [x] `File` constructor extending `Blob`, `name`, `lastModified`, and `webkitRelativePath`.
  - [x] `FormData` entry storage, `append`, `set`, `get`, `getAll`, `has`, `delete`, automatic Blob-to-File wrapping, and `forEach` iteration.
  - [x] Conformance suite and differential Node.js verification.
- [x] Phase 5: Streams interfaces required by ECMA-429.
  - [x] Queuing Strategies (`ByteLengthQueuingStrategy`, `CountQueuingStrategy`) with `highWaterMark` and `size(chunk)` calculation.
  - [x] `ReadableStream` core lifecycle (`start`, `pull`, `cancel`, `enqueue`, `close`, `read()`, `releaseLock()`, `cancel()`, `tee()`, and `ReadableStream.from()`).
  - [x] `WritableStream` core lifecycle (`start`, `write`, `close`, `abort`, `getWriter()`, `releaseLock()`).
  - [x] `TransformStream` & stream piping (`readable`, `writable`, `transform`, `flush`, `pipeThrough`, `pipeTo`).
  - [x] `TextEncoderStream` and `TextDecoderStream` integration.
  - [x] `Blob.prototype.stream()` readable stream backing.
  - [x] Streams conformance suite and differential testing matching Node.js v26.8.1.
- [ ] Phase 6: Headers, Request, Response and fetch with cancellation/streaming semantics.
  - [x] Finish `Headers` conformance with dedicated WinterTC fixture covering normalization, duplicate combination, `set-cookie`, mutation, iteration, constructors, and validation errors.
  - [x] Add `Request` constructor, method/url/headers/body state, cloning, body-use semantics, and AbortSignal integration.
  - [x] Add `Response` constructor, status/statusText/headers/body state, cloning, redirect/error/json helpers, and body-use semantics.
  - [x] Implement structured runtime `Body.json()` parsing for objects, arrays, escaped strings/Unicode, strict JSON numbers, nested dynamic access, and `SyntaxError` coverage.
  - [x] Implement `Body.formData()` for `application/x-www-form-urlencoded`, including percent decoding, duplicate names, UTF-8, MIME validation, and disturbed/locked body semantics.
  - [x] Add `multipart/form-data` Body parsing with quoted/unquoted boundaries, duplicate text fields, binary-safe `File` parts, per-part MIME types, and malformed/missing-boundary rejection.
  - [ ] Add native network transport abstraction plus deterministic local HTTP integration harness.
    - [x] Add Linux/AMD64 raw HTTP transport primitive and standalone-ELF integration harness.
    - [x] Generalize transport to numeric IPv4, numeric IPv6, `localhost`, and `/etc/hosts` IPv4 resolution while preserving cooperative cancellation.
    - [x] Add UDP DNS fallback from `/etc/resolv.conf` with cooperative cancellation.
    - [ ] Add TLS/HTTPS transport.
      - [x] Add native TLS 1.3 crypto foundation: SHA-256/HMAC/HKDF, X25519, OS entropy, and AES-128-GCM via AF_ALG.
      - [x] Add TLS 1.3 HKDF-Expand-Label, sequence nonces, and authenticated record encrypt/decrypt primitives.
      - [ ] Add ClientHello/ServerHello handshake transcript and traffic-secret derivation.
      - [ ] Add certificate-chain/hostname authentication against system trust roots.
      - [ ] Wire encrypted application records into live fetch streaming, cancellation, and HTTPS E2E coverage.
  - [ ] Implement `fetch()` request normalization, redirects, AbortSignal cancellation, streaming request/response bodies, and server-runtime `User-Agent`.
    - [x] Normalize string/Request input plus RequestInit method/headers/body/signal overrides and emit `User-Agent: ts-pro`.
    - [x] Parse response headers including duplicate combination and `set-cookie` preservation.
    - [x] Honor pre-aborted signals before transport dispatch.
    - [x] Follow relative redirects, expose `redirected`/final URL, support manual/error modes, and apply 301/302/303 vs 307/308 method/body semantics.
    - [x] Follow redirect chains with the Fetch redirect-count limit and loop/error coverage.
    - [x] Cancel connect/write/read while I/O is in flight when AbortSignal fires.
    - [x] Receive response bytes incrementally with geometric buffer growth instead of a fixed 64 KiB read cap.
    - [x] Decode HTTP/1.1 `Transfer-Encoding: chunked` response framing before exposing the body.
    - [x] Back Request/Response bodies with stable ReadableStream objects and enforce disturbed/locked body usability semantics.
    - [x] Resolve fetch before the complete response body arrives and drive the body from live transport/backpressure.
    - [x] Stream request bodies through transport instead of materializing BodyInit into one wire buffer.
  - [x] Add Headers/Request/Response/fetch conformance and cancellation/streaming differential coverage.
- [ ] Phase 7: WebCrypto, Performance and Compression APIs.
  - [x] Finish `Performance` surface required by the current ECMA-429 target (`timeOrigin`, `now()`, `toJSON()`) with native coverage.
  - [x] Add OS randomness foundation and `crypto.getRandomValues()` / `randomUUID()` with quota/view semantics and UUID-v4 validation.
  - [ ] Complete `CryptoKey` / `SubtleCrypto` algorithms required by the WinterTC target with explicit unsupported-algorithm errors.
    - [x] `digest()` for SHA-1 / SHA-256 / SHA-384 / SHA-512.
    - [x] HMAC key generation/import/export plus sign/verify.
    - [x] HKDF importKey + deriveBits.
    - [ ] Complete the remaining ECMA-429-required key algorithms/operations and CryptoKey surface.
  - [ ] Add `CompressionStream` / `DecompressionStream` on top of the Streams foundation.
  - [ ] Add crypto/compression conformance fixtures and deterministic known-answer tests.
- [ ] Phase 8: MessageChannel/MessagePort, rejection events and required WebAssembly APIs.
  - [ ] Add MessageChannel/MessagePort lifecycle, FIFO delivery, transfer/close semantics, and EventTarget integration.
  - [ ] Integrate Promise jobs with the Web microtask queue and implement `unhandledrejection` / `rejectionhandled` ordering.
  - [ ] Complete required WebAssembly globals/interfaces and native execution/linking coverage.
  - [ ] Add messaging/rejection/WebAssembly conformance fixtures.
- [ ] Phase 9: close conformance matrix, differential/integration suite and documented server-runtime deviations.
  - [ ] Audit every ECMA-429 global/interface/member against `docs/WINTERTC.md`; no name-only/stub rows may be marked conformant.
  - [ ] Run full native fixture, Node differential, local-network integration, GC stress, and performance regression gates.
  - [ ] Document intentional server-runtime deviations and unsupported optional browser-only behavior.
  - [ ] Mark WinterTC target complete only when the matrix has no unchecked required rows.

Current WinterTC execution order (2026-09-07): **Phase 6 Request -> Response -> fetch -> Phase 7 -> Phase 8 -> Phase 9**.
Current acceptance baseline: **152 / 152 native fixtures PASS**, **52 / 52 Node differential fixtures matched**.
