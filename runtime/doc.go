// Package runtime implements the pure-Go native runtime and runtime libraries for ts-pro.
//
// In accordance with the ts-pro architecture, Go is reserved strictly for the native runtime
// engine and native libraries behind stable ABI boundaries. All handwritten C, cgo, and assembly
// have been retired; runtime compiles under CGO_ENABLED=0 with zero external C dependencies.
//
// All function names in this package follow idiomatic camelCase (e.g. heapAlloc, gcCollect,
// taskSpawn, stringNew, channelF64Send) and are package-private within runtime.
//
// # Subsystems
//
// The runtime is structured across 4 primary subsystems within a single flat package to
// enable aggressive cross-file inlining without interface indirection or cyclic imports:
//
// 1. Memory & Garbage Collection:
//   - allocator.go: Thread-local spans, free lists, and size classes.
//   - heap.go: Central heap state, major collections, and safepoints (heapAlloc, gcCollect).
//   - nursery.go: Young-generation nursery allocation and minor GC cycles.
//   - roots.go: Explicit GC root stacks and persistent registration (gcRootRegister).
//   - remembered.go: Generational write barriers (gcStoreRef) and remembered set.
//   - gcmark.go: Iterative and parallel work-stealing mark phase.
//   - blocktable.go: Fast block metadata and span ownership lookup.
//   - gc_trace_metrics.go: GC telemetry and atomic/conservative block tracing counters.
//
// 2. Concurrency & Cooperative Task Scheduler:
//   - scheduler.go: Multi-worker work-stealing scheduler (schedulerInit, schedulerSubmit).
//   - task.go: Lightweight cooperative tasks, promises, and awaiting (taskSpawn, taskJoin).
//   - taskgroup.go: Structured concurrency task groups (taskGroupNew, taskGroupSpawn*).
//   - channel.go: Typed synchronous and buffered channels (channelF64New, channelF64Send).
//   - timer.go: Asynchronous timers and cooperative sleep (sleepTask).
//   - blocking.go: Offloaded blocking operations pool.
//   - taskcontext.go: Task-local context propagation and cleanup.
//   - promise_aggregate.go: Promise combinators (promiseAllF64, promiseRaceF64).
//
// 3. Types, Data Structures & Dynamic JSValue:
//   - string.go: Length-prefixed UTF-8 native strings and concatenation (stringNew, stringConcat).
//   - array.go: Specialized flat arrays for float64, references, and booleans (arrayF64New).
//   - object.go: Fixed-shape struct object allocation (objectAlloc).
//   - jsvalue.go: NaN-boxed dynamic JavaScript values and coercion (jsValueBoxF64, jsValueAdd).
//   - handle.go: Opaque pointer handles and reference tokens.
//
// 4. Platform & Host Integration:
//   - thread_darwin.go / thread_linux.go / thread_other.go: OS thread identification.
//   - runtime.go: Top-level console output coordination (consoleLogF64).
//   - lockmetrics.go: Spinlock contention and acquisition telemetry.
package runtime
