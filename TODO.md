# TODO

Legend: `[ ]` planned, `[~]` in progress, `[x]` complete, `[S]` superseded.

## Architecture invariant

- [~] Enforce the target language split: **TypeScript 7 implements the compiler; Go implements the native runtime/native libraries only**.
  - [x] Document Go runtime ownership and stable C-compatible ABI boundaries.
  - [ ] Stop adding new compiler/frontend/HIR/MIR/codegen features to the transitional Go compiler path.
  - [ ] Port semantic DTO/HIR/MIR/representation/lowering/LLVM/build orchestration from `cmd/` + `internal/` Go packages into the TypeScript 7 compiler implementation with parity tests.
  - [ ] Retire transitional compile-time Go packages after TypeScript 7 parity is complete.
  - [x] Keep Go under `runtimego/` (and native-library packages) for allocator/GC, scheduler/tasks, channels, timers, blocking pool, JSValue, strings/arrays/objects, and native libraries.
  - [x] Remove all handwritten runtime C after Go runtime parity; the cached Go `c-archive` now publishes its generated C-compatible ABI header for toolchain/tests.

## Project completion goal

- [~] Reach 100% of the compiler roadmap tracked in this file, with every completed capability covered by native acceptance/regression tests and differential tests where TypeScript/JavaScript observable behavior applies.
- [ ] Do not mark the project 100% until all `[ ]` and `[~]` roadmap items below are either completed (`[x]`) or explicitly superseded (`[S]`) with a documented replacement.

## Foundation and frontend

- [S] Historical migration from Rust to the current Go compiler path; superseded as target architecture by the TypeScript-7 compiler / Go-runtime-only split.
- [x] Remove the superseded Rust workspace from the repository.
- [x] Pin `typescript@7.0.2` and strict native `tsconfig`.
- [~] Keep the existing Go `cmd/tsnative`/compiler tooling only as transitional parity infrastructure until the TypeScript 7 compiler path replaces it.
- [x] Implement TypeScript 7 LSP JSON-RPC transport and lifecycle.
- [x] Implement TypeScript 7 `tsc --api --async` semantic transport.
- [~] Preserve the existing Go binary-AST decoder only as transitional parity infrastructure; the target TypeScript 7 compiler consumes its own AST/checker data directly.
- [x] Query exact AST-node symbols/types through the TypeScript checker.
- [x] Gate native builds on TypeScript diagnostics before lowering.
- [x] Regression-test that SWC/Babel/Oxc are absent from the active compiler path.
- [~] Preserve the existing Go TypeScript-LS workspace manager only as transitional tooling; move compiler/workspace ownership into the TypeScript 7 implementation.
- [x] Preserve TypeScript-LS crash detection and generation-based restart behavior during compiler migration.

## HIR, representation, and MIR

- [~] Preserve the current Go semantic DTO/HIR implementation as a reference while porting compiler-owned DTO/HIR into the TypeScript 7 compiler.
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
- [x] `if`, `while`, and `for` control flow in functions and at source-file top level.
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
- [~] Exceptions, Promise, and async/await.

## Native concurrency management

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
  - [ ] Migrate future blocking native-library adapters onto the blocking-call pool as those libraries are added.
- [~] Lower `async`/`await` to resumable task state machines rather than blocking OS workers.
  - [x] Native `Promise<T>` uses TaskRef ABI for compiled async functions; async calls spawn typed tasks and TS7 `await` lowers to task join with worker=1 acceptance.
  - [~] Replace cooperative await joins with compiler-generated task-completion suspension/resume and spill live values across await points.
    - [x] F64 async-await completion parking with race-safe child completion waiters, child-result transfer into parent task state, worker=1 stackless resume, and LLVM/native regressions.
    - [x] Bool/reference/JSValue await-result transfer with typed continuation spill slots, GC-safe reference handoff, worker=1 native coverage, and task-spawn parameter coercion matching direct-call ABI rules.
    - [~] Spill arbitrary live SSA values across await points and support branch/loop continuation CFGs.
      - [x] Linear F64 arithmetic, proven-integer arithmetic, and Bool comparison results spill into typed task-state slots and survive later await/channel/sleep suspension points.
      - [x] Linear StringRef/JSValue creation, string concatenation, boxing/dynamic-add, and F64 array-read results spill into typed task state with allocation safepoints before suspension.
      - [x] Multi-block Branch/Jump/Return continuation CFGs without Phi nodes, including stackless async `if` on a single worker.
      - [x] Phi-backed continuation merges use typed task-state slots with parallel edge copies, including loop-carried numeric `while` state on a single worker.
      - [x] Generic resumable native-op steps cover object/array allocation and mutation, field access, direct/virtual/closure calls, intrinsics, task yield, and nonblocking channel operations using typed spills and allocation safepoints.
      - [x] Remove the remaining cooperative/manual task patterns and expand richer reference Phi/state stress coverage.
        - [x] Spill delayed nested `TaskRef` handles across suspension; typed joins await the existing handle and void joins use completion parking followed by explicit release, including worker=1 regression coverage.
        - [x] Stress StringRef/ObjectRef/JSValue state across branch/loop Phi merges and suspension on a single worker, including GC-managed dynamic values.
        - [x] Remove remaining spawned-task cooperative channel/sleep fallback shapes with compile-time continuation enforcement.
  - [~] Add Promise rejection/exception propagation and standard Promise combinators where selected for the native runtime.
    - [x] Async `throw` lowers to a GC-rooted JSValue task failure, propagates through stackless await completion waiters, and terminates an uncaught top-level join with the rejection value.
    - [x] Add non-aborting failed-task inspection with GC-root lifetime tests, and release context/failure persistent roots when task handles are destroyed.
    - [~] Add native `try`/`catch`/`finally`, rejection recovery, and selected Promise combinators.
      - [x] Native async `try/catch` catches local `throw` through explicit CFG exception edges and JSValue catch bindings without runtime unwind/setjmp.
      - [x] Caught `await` uses stackless task-status suspension, branches failed child results into GC-rooted JSValue catch Phi state, and releases child handles without poisoning the parent task.
      - [~] Add `finally`, broader nested rejection recovery tests, and selected Promise combinators.
        - [x] Native async `finally` runs after normal/caught completion, including recovered awaited rejection on a single worker.
        - [x] Preserve pending return/rethrow completion through `finally`, evaluating the completion value before finalizer side effects and propagating catch rethrows to outer handlers.
        - [x] Add completion override from `return`/`throw` inside `finally`, including nested propagation into outer async catch handlers.
        - [~] Add broader nested recovery tests and selected Promise combinators.
          - [x] Cover nested awaited rejection -> catch rethrow -> inner finally -> outer catch/finally recovery on a single scheduler worker.
          - [x] Add immediate native `Promise.resolve(value)` and `Promise.reject<T>(reason)` settlement without scheduler work; number/bool/reference results use typed TaskRef slots and rejected reasons retain persistent GC roots until handle release.
          - [~] Add Promise adoption/thenable assimilation plus selected aggregate combinators (`Promise.all` / `Promise.race`).
            - [~] Replace single-owner TaskRef completion with Promise-safe ownership and waiter fan-out.
              - [x] Add retain/release reference counting so consuming joins/awaits release ownership instead of unconditionally destroying task storage.
              - [x] Replace the single completion waiter slot with a cgo-safe external waiter table and fan out settlement to multiple parked tasks.
              - [x] Make compiled Promise awaits non-consuming for owned function-scope Promise locals, retaining settled result/failure roots until deterministic final local release; repeated numeric/reference awaits pass under worker=1 + 1 KiB nursery. Promise local copies now retain ownership; function-scope reassignment retains the incoming alias before releasing the previous handle, including self-assignment and identity adoption; `if` and loop-carried Promise reassignments now merge ownership through the same SSA/Phi state as locals.
            - [x] Add `Promise.resolve(existingPromise)` adoption with identity-preserving TaskRef lowering, retain-on-copy for function-scope Promise locals, direct Promise local alias ownership, and repeated-await number/reference GC coverage.
            - [ ] Add thenable assimilation after dynamic method/`this` call semantics are sufficient.
            - [ ] Add homogeneous `Promise.all` / `Promise.race`, then heterogeneous tuple results after tuple semantics land.
- [x] Add structured concurrency, task groups, cancellation, and task-local context.
  - [x] Add cooperative task cancellation request/query intrinsics with native runtime flags and worker=1 regression coverage.
  - [x] Add native task groups with group-owned child tracking, group join/close, cancellation propagation, compiler intrinsics, and worker=1 structured-concurrency regressions.
  - [x] Add GC-rooted task-local JSValue context with automatic parent-to-child inheritance across worker migration/suspension and worker=1 regression coverage.
- [x] Integrate task/channel/timer state with precise GC roots and scheduler safepoints.
  - [x] Keep task state/result/context/failure references rooted for the complete task lifetime; reference-channel buffered/queued values own persistent roots, while timer waiters retain opaque task handles whose task state remains rooted.
  - [x] Add deferred heap-threshold GC requests and retry collection at scheduler task-return, wait, park, and execution-budget safepoints; collection waits until foreign native root stacks are quiescent or the only active stack is the paused current thread.
- [~] Add per-worker allocation caches/nurseries and later Green-Tea-style per-worker mark-page queues.
  - [x] Add worker-owned size-class spans with bump-pointer slots, zeroed slot reuse, bounded recycled-span caching, and finalizer-safe recycling.
  - [x] Add page-to-span metadata for interior-pointer resolution plus remote-free and reusable-span ownership-transfer accounting.
  - [x] Add owner-drained remote-free inboxes so foreign collectors/workers do not directly recycle another worker’s span slots.
  - [x] Replace recursive heap tracing with iterative owner-local mark work queues and cross-owner queue switching.
  - [x] Batch mark work by native page within owner queues.
  - [x] Add bounded parallel mark-page assist (up to 8 workers, enabled for heaps with at least 512 blocks) with owner-local preference, cross-owner queue stealing, `TSNATIVE_GC_MARK_WORKERS`, and helper-work regression coverage.
  - [x] Let already-idle scheduler workers donate bounded GC mark work directly before creating fallback helper goroutines; donor-worker/page accounting has dedicated regression coverage.
  - [~] Profile contention and reduce the global heap lock, then add nursery/generational policy only after measured results justify it.
    - [x] Split root/token/thread-stack/handoff metadata onto its own mutex so root lifecycle operations no longer serialize with heap allocation; collection keeps `heap -> roots` lock order across mark/sweep.
    - [x] Instrument heap/root mutex acquisitions, contentions, and cumulative wait nanoseconds through the generated native ABI, with deterministic contention regressions.
    - [x] Use native stress measurements to move worker-local active-span allocation off the global heap mutex behind a GC world RW barrier, a 128-shard live-block index, and atomic live-byte/allocation counters. An 8-worker/40k-allocation stress regression drops heap-lock acquisitions from 40,000 to 24 and cumulative heap-lock wait from roughly 241-279 ms to roughly 0.28-1.34 ms.
    - [x] Reduce root/token lock contention with 16 independent root shards selected by thread/token page, shard-local token pools, and the GC world barrier as the stable snapshot boundary. Persistent tokens remain valid across worker migration; the same 80,000-root-operation stress case drops cumulative root-lock wait from roughly 224-236 ms to 0-0.115 ms in a five-run sample.
    - [x] Profile GC world-barrier and live-block-shard contention: steady-state world reads show zero contention; 128 live-block shards are the measured sweet spot between allocation-write contention and full-GC scan overhead (64/128/256 compared).
    - [x] Add a bounded per-worker nursery trigger (`TSNATIVE_GC_NURSERY_BYTES`, 64 KiB default) and conservative minor collector: trace young roots, scan old blocks for old→young references, sweep only nursery blocks, and promote survivors without requiring an incomplete compiler write barrier. Eight rooted 64 KiB nursery cycles complete as minors without a major before the 1 MiB major threshold; a 1,024-old-object sample measures roughly 151-168 µs/minor versus 303-523 µs/full major.
    - [x] Add compiler/runtime remembered-set write barriers for mutable native-heap reference stores, including object fields, task continuation spills, channel receives, and task completion outputs; minor GC now scans only remembered old parents instead of the full old generation. A 1,024-old-block/64 KiB nursery sample measures roughly 75-79 µs/minor with no remembered parent and 106-119 µs with one remembered parent, versus 151-168 µs for the earlier conservative old scan.
- [~] Add cooperative execution budgets/preemption polling after scheduler correctness is stable.
  - [x] Add true logical task yield/requeue as the scheduler suspension primitive.
  - [x] Inject bounded execution-budget polls at proven loop backedges and requeue when the budget expires; worker=1 fairness regression verifies CPU-heavy tasks yield to runnable peers.
- [x] Stress-test 1/100/10k/100k tasks, CPU fan-out/work stealing, channel contention/parking, cancellation, GC-root handoff/churn, and bounded blocking calls.

## Remaining native language/runtime coverage

- [~] Expand native array coverage beyond read-only `number[]`.
  - [x] Bounds-checked in-place indexed writes for specialized `number[]`.
  - [~] String/object arrays and typed generic array specializations.
    - [x] Add specialized `string[]` allocation/read/write/length with precise element GC metadata and remembered-set barriers.
    - [x] Add closed-object reference arrays with structural element compatibility, precise GC tracing, heap-store escape propagation, and GC-churn differential coverage.
    - [ ] Add function/JSValue/nested reference-array specializations with element tag/ownership validation.
  - [ ] JS-compatible growth semantics and common mutators such as push/pop where representation contracts permit.
- [ ] Expand object semantics: optional properties, union shapes, computed/dynamic keys, shape transitions, and broader structural compatibility.
- [~] Broaden TypeScript syntax coverage: tuples, destructuring, rest/spread, optional chaining, nullish coalescing, switch/for-of, templates, default/optional params, enums, and module linking.
  - [x] Native `do...while` lowers with mandatory first-body execution followed by the existing loop-carried SSA machinery; source-file top-level and function-body forms share the same semantic statement path.
- [ ] Add selected standard-library/runtime APIs such as JSON, Map/Set, Date, and RegExp after their representation contracts are defined.

## LLVM, runtime, and build

- [~] Preserve deterministic LLVM IR output while migrating LLVM emission from the transitional Go compiler into the TypeScript 7 compiler.
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
- [~] Improve memory optimization with precise object metadata/root maps, escape analysis, stack allocation, scalar replacement, arenas, and generational collection.
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
  - [~] Physically stack-allocate non-scalarizable reference-bearing objects with precise field roots.
    - [x] Allocate dedicated GC shadow-root slots for reference fields of physical stack objects.
    - [x] Synchronize reference field roots after `ObjectNew` initialization and every stack-object `FieldSet`; stack stores bypass the heap remembered-set barrier.
    - [x] Conservatively reject stack placement when allocation provenance gains an object alias; alias/interior-pointer support remains a later extension.
    - [x] Add native GC-stress coverage proving stack-held reference fields survive repeated collection with a 1 KiB nursery; `examples/stack_reference_object.ts` stays physical (`scalar=0`, `stack=1`) and preserves its field through sustained string churn.
    - [~] Extend stack placement to safe aliases/interior pointers with explicit provenance/root synchronization. Single-origin Phi aliases are now resolved back to their stack allocation and share field-root synchronization; mixed/unknown aliases remain heap-backed. True interior-pointer/address-taking support is still pending because MIR does not expose those operations yet.
- [x] Make `go vet ./...` clean across native ABI boundaries without suppressing `unsafeptr`: exported opaque task/group handles use mmap-backed pointer tokens, real native pointer fields stay `unsafe.Pointer`, and `uintptr` remains only for internal numeric lookup/queue keys.
- [~] Add parallel LLVM module compilation and deterministic object cache (deterministic LLVM/runtime object cache and parallel runtime compilation implemented; multi-module LLVM scheduling pending).

## Dynamic boundary, correctness, and performance

- [~] Add tagged `JSValue` only for values that cannot keep a proven native representation.
  - [x] Heap-backed GC-managed JSValue boundary for `any` number/string values.
  - [x] Explicit number/string boxing at variable, return, and direct-call parameter boundaries.
  - [x] Boolean, null/undefined, object/function, and tagged-union JSValue variants.
    - [x] Boolean JSValue tag/boxing, logging, and primitive `+` coercion with numbers/strings.
    - [x] Null/undefined JSValue literals/tags with console output and primitive `+` coercion (`null -> 0`, `undefined -> NaN`, string conversion).
    - [x] Object/array/function reference JSValue tags and explicit native boxing, with precise payload-slot GC tracing and differential/compiler regression coverage.
    - [x] Tagged unions lower to JSValue only when every constituent has a supported dynamic representation; TS7 constituent metadata is preserved and unsupported members fail representation analysis.
- [~] Add checked conversions and dynamic operator/property slow paths.
  - [x] Dynamic `+` for number/string JSValue operands and `console.log(any)`.
  - [x] Primitive dynamic `-`, `*`, `/`, relational comparison, loose equality, and strict equality with TypeScript-correct result representations and JS-style primitive coercion; object/function reference equality is supported without ToPrimitive coercion.
  - [~] Dynamic property get/set, calls, object/function ToPrimitive coercion, and checked unboxing/conversions.
    - [x] Checked JSValue unboxing to native number/string/boolean/number[] with distinct array tagging, MIR/LLVM/runtime ABI coverage, and differential native tests.
    - [~] Dynamic object/property get/set and calls, plus object/function ToPrimitive coercion.
      - [x] Add default native `ToPrimitive` fallback for boxed plain objects, F64 arrays, and function references. Object addition/string comparison uses `[object Object]`, F64 arrays stringify with comma-joined JS number text, numeric coercion flows through the primitive fallback, and loose equality no longer aborts on object/array/function operands.
      - [x] Preserve closed object shape identity through JSValue boxing. Object boxes encode `shapeID+1` in the existing non-GC metadata word, LLVM passes the semantic shape on every object box, MIR/HIR verification rejects unknown shapes, and GC stress verifies metadata/payload survival without growing JSValue.
      - [~] Use boxed shape identity for dynamic named property get/set dispatch with compiler-generated field layout dispatch.
        - [x] Dynamic named property reads generate deterministic LLVM helpers keyed by property name. The helper switches on boxed shape identity, performs exact typed GEP/load/boxing, preserves nested object shape metadata, and returns `undefined` for missing/non-object properties.
        - [x] Dynamic named property writes use the same boxed-shape dispatch, checked JSValue unboxing for scalar/reference representations, nested-object shape validation, and `tsnative_gc_store_ref` remembered-set barriers for reference fields. Missing fields on closed shapes fail explicitly instead of corrupting layout. A 1 KiB nursery regression promotes the holder, stores a young string through `any`, forces further minors, and verifies the typed alias still observes the live value.
      - [x] Dynamic callable dispatch preserves native closure target identity through JSValue boxing and closure cells, checked-unboxes arguments against each target ABI (including object-shape validation), invokes the existing closure wrapper ABI, and boxes native results back to JSValue. `any`/union callees stay dynamic even when their symbol refers to a known function, and contextually-`any` captured closures are explicitly boxed instead of leaking raw closure pointers across the dynamic boundary.
- [x] Add differential tests against the TypeScript 7 → JavaScript reference path.
- [x] Add native-coverage, boxing, dynamic-dispatch, and runtime-call reports.
- [x] Add compile-stage timing for TS API, HIR/MIR, LLVM, link, and object-cache hit rate.
- [ ] Add ThinLTO after module/object caching is established.
- [ ] Add PGO after MIR quality and benchmark coverage are stable.
- [ ] Add cross compilation after the Linux x86-64 runtime ABI is stable.

## Current critical path

1. Finish physical stack allocation for non-scalarizable reference-bearing objects only after precise stack-root/interior-pointer handling is covered. Fully scalarizable reference-bearing objects already avoid physical storage, proven local closures now use parent-frame storage, and task environments remain heap-backed when their lifetime crosses scheduler ownership.
2. Finish nested rejection recovery and selected Promise combinators.
3. Complete remaining dynamic object/property/call semantics and selected JavaScript coercion slow paths.
4. Finish advanced generics, integer SSA across calls/loops, remaining array/object semantics, and broader TypeScript syntax/standard-library coverage.
5. Finish multi-module compilation/linking and cross-module dispatch/specialization, then ThinLTO, PGO, and cross-compilation.
6. Port compiler-owned semantic/HIR/MIR/LLVM/build orchestration to TypeScript 7 and retire transitional compile-time Go packages.
