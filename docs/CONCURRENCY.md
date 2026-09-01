# Native Concurrency Plan

## Goal

Add Go-like lightweight concurrency to the native TypeScript runtime without mapping one TypeScript task to one OS thread.

Target model:

```text
TypeScript tasks (M)
        ↓
M:N scheduler
        ↓
OS worker threads (N ≈ CPU cores)
```

The runtime is stackless. Async functions and blocking task operations lower to resumable state machines instead of allocating a large native stack per task.

## Implementation-language boundary

Concurrency APIs are compiler-known TypeScript features, but their runtime implementation belongs in Go. Scheduler, tasks, channels, timers, blocking-call management, cancellation, and GC coordination must live in `runtimego/`. Do not move compiler HIR/MIR/LLVM ownership into Go merely because the runtime uses goroutines.

## Design rules

- One task must not imply one pthread.
- Static native types stay unboxed across task/channel operations whenever possible.
- Blocking native calls must not block scheduler workers.
- Scheduler concurrency must be bounded by CPU and memory budgets.
- GC, scheduler safepoints, cancellation, and async suspension share one runtime lifecycle model.
- Linux x86-64 is the first scheduler target; OS primitives are abstracted before additional platforms.

## TypeScript surface

Initial native declarations should expose explicit concurrency rather than changing JavaScript semantics implicitly:

```ts
const task = spawn(() => compute());
const result = task.join();

yieldNow();

const ch = channel<number>(64);
ch.send(42);
const value = ch.recv();
```

Later phases add `TaskGroup`, cancellation, timers, `select`, and compiler-backed `async`/`await`.

Suggested native modules:

```text
@tsnative/task
@tsnative/channel
@tsnative/sync
@tsnative/time
@tsnative/atomic
```

These imports are compiler-known native libraries, not dynamic Node module lookups.

## Runtime layout

```text
runtimego/
  scheduler.go     # bounded workers, queues, stealing, park/wake, metrics
  task.go          # task handles, completion, roots, results, cancellation
  taskgroup.go     # structured child ownership and group cancellation
  taskcontext.go   # task-local context ABI backed by task-owned roots
  channel.go       # typed F64/bool/reference channels
  timer.go         # task-aware timers/sleep
  blocking.go      # bounded blocking-call isolation
  heap.go          # native allocation and mark/sweep roots
```

The runtime is linked as a cached Go `c-archive`. Its generated header is cached beside the archive and is the ABI source used by native toolchain tests. There are no handwritten runtime `.c` implementation files or legacy scheduler/task headers in the active build path.

A worker owns hot local state:

```text
Worker
├─ local task deque
├─ scheduler counters
├─ local allocation cache
├─ local GC work queue
└─ later: local nursery
```

Fast paths should avoid global locks. Cross-worker operations use stealing, an injection queue, and targeted wakeups.

## Compiler IR

Concurrency must enter through compiler-owned IR rather than source-to-runtime special cases.

HIR operations:

```text
TaskSpawnOp
TaskJoinOp
TaskYieldOp
TaskCancelOp
ChannelNewOp
ChannelSendOp
ChannelRecvOp
SleepOp
```

MIR lowers these to typed runtime contracts such as:

```text
task.spawn
task.join
task.yield
channel.send.f64
channel.recv.f64
channel.send.ref
channel.recv.ref
```

A `channel<number>` therefore remains an F64 channel and does not become `JSValue` merely because it crosses a task boundary.

## Implementation and commit sequence

Steps 1-13 and 16 are implemented. Steps 14-15 remain active work. Each additional step must compile, test, and commit independently.

1. `runtime: add bounded native scheduler core`
   - Worker lifecycle, CPU-count default, `TSNATIVE_WORKERS`, shutdown.
   - No TypeScript API yet.
   - Tests: create/start/stop workers repeatedly; zero leaked threads.

2. `runtime: add lightweight task lifecycle`
   - Task states: runnable/running/waiting/done/cancelled/failed.
   - Global injection queue and simple FIFO scheduling.
   - Tests: 1, 100, and 10k no-op tasks.

3. `compiler: lower native spawn and join intrinsics`
   - Reuse existing closure `{code, env}` representation.
   - Add semantic → HIR → MIR → LLVM/runtime contracts.
   - Acceptance: spawn a numeric closure, join it, print result.

4. `runtime: add per-worker task deques`
   - Local push/pop fast path.
   - Keep the global queue only for external injection/fallback.
   - Add scheduler queue metrics.

5. `runtime: add work stealing and targeted wakeups`
   - Idle workers steal half-ranges/batches from peers.
   - Linux wake path should avoid broadcast wakeups.
   - Stress: uneven task fan-out across all workers.

6. `compiler: add yield and scheduler safepoints`
   - Explicit `yieldNow()` first.
   - Later compiler polls on selected loop backedges.
   - CPU-bound tasks must not monopolize one worker indefinitely.

7. `runtime: add typed channels`
   - Buffered and unbuffered `channel<number>` first.
   - Ring buffer plus parked sender/receiver queues.
   - Channel contention parks tasks, never OS workers.

8. `compiler: specialize channel representations`
   - F64, StringRef/ObjectRef, then JSValue channels.
   - Differential and high-contention tests.

9. `runtime: add timers and sleep`
   - Timer min-heap for MVP; timer wheel only after profiling.
   - Sleeping tasks are parked and workers remain available.

10. `runtime: add blocking native-call pool`
    - Separate bounded blocking threads from scheduler workers.
    - Completion wakes the suspended task.

11. `compiler: lower async functions to task state machines`
    - Stackless suspension points and resumable state objects.
    - `await` parks the current task instead of blocking its worker.

12. `runtime: add structured concurrency and cancellation`
    - Task groups, child ownership, join-on-scope-exit, cancellation tokens.
    - Cancellation is observed at compiler/runtime safepoints.

13. `runtime: integrate scheduler with GC` ✅
    - Task state/result/context/failure roots and queued reference-channel values have explicit lifetime roots; timer waiters retain opaque task handles whose task-owned state remains rooted.
    - Heap-threshold GC requests defer while foreign native root stacks are active and retry at task-return, wait, park, and execution-budget safepoints.
    - Idle-worker GC assistance remains an optional later optimization; correctness no longer depends on it.

14. `runtime: add per-worker allocator caches` 🟡
    - Small allocations up to 2 KiB now use worker-owned size-class spans with zeroed slot reuse and bounded reusable-span caching.
    - Finalizer-bearing slots remain pinned until finalizer completion; heap shutdown reclaims cached spans deterministically.
    - Bump-pointer nursery metadata and explicit remote ownership transfer accounting remain.

15. `runtime: add Green-Tea-style local mark-page work`
    - Page/span marking queues per worker after size-class spans and precise pointer metadata exist.
    - Work stealing applies to GC pages as well as runnable tasks.

16. `runtime: add execution budgets and preemption polling`
    - Cooperative budget first; no arbitrary signal-time stack surgery.
    - Benchmark fairness and throughput before stronger preemption.

## Required metrics

Extend `--report-performance` with:

```text
scheduler workers
tasks spawned / completed
peak runnable tasks
local queue pushes/pops
steal attempts / successful steals
worker parks / wakeups
channel sends / receives / parks
blocking jobs
scheduler CPU time
GC assist CPU time
peak task memory
```

Environment controls:

```text
TSNATIVE_WORKERS
TSNATIVE_MAX_TASKS
TSNATIVE_BLOCKING_WORKERS
TSNATIVE_SCHED_TRACE
```

Defaults must be bounded and safe for servers and command-line programs.

## Acceptance gates

Before calling the concurrency runtime production-ready:

- 100k parked/lightweight tasks must not create 100k OS threads.
- Worker count must remain bounded by configuration.
- Idle scheduler CPU must remain near zero.
- No scheduler operation may lose a GC root during collection.
- Static typed channels must show zero JSValue boxing on their fast path.
- Blocking-pool saturation must not stall CPU scheduler workers.
- Cancellation and shutdown must leave no runnable/waiting task leaks.
- Differential tests cover user-visible ordering only where the API promises ordering.
- ThreadSanitizer/ASan runtime tests should be runnable in CI/debug builds.

Initial memory targets:

```text
10k parked tasks: single-digit MB to low tens of MB overhead
100k parked tasks: tens of MB, not hundreds of MB from stacks
worker threads: approximately CPU cores, independent of task count
```

Optimization comes after correctness: FIFO scheduler → local queues → stealing → allocator/GC locality. This prevents debugging a clever scheduler whose primary feature is occasionally eating tasks.
