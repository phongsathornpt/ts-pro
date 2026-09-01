# Implementation Status

## Target architecture correction

The target architecture is **TypeScript 7 for the compiler and Go only for the native runtime/native libraries**. The existing Go compiler under `cmd/` and `internal/` is transitional parity/reference code and must be retired as compile-time stages move into TypeScript 7. Handwritten runtime C is also transitional and is being replaced by `runtimego/`.

## Stable committed baseline

The stable native compiler checkpoint is commit `6bad298`.

```text
TypeScript 7.0.2
  -> diagnostics + checker/API snapshot
  -> TypeScript 7 AST/checker integration (current Go decoder is transitional)
  -> compiler semantic DTOs
  -> typed HIR + representation/range analysis
  -> MIR / SSA
  -> LLVM IR
  -> native linker + Go runtime ABI
  -> native executable
```

The generated executable does not embed Node.js or V8.

## Stable coverage

- functions, recursion, direct calls, closures, and escaping function values;
- arithmetic/comparisons plus proven integer arithmetic fast paths;
- mutable locals, top-level bindings, `if`, `while`, and `for` with SSA phi nodes;
- strings and specialized `number[]`, including checked indexed writes;
- closed objects, fixed field reads/writes, classes, constructors, property initializers;
- single inheritance, `super()`, provenance devirtualization, and class-tag dispatch;
- direct generic `T` specialization for scalar/string call sites;
- shared heap ownership and initial root-aware mark/sweep GC;
- differential TypeScript 7 -> JavaScript reference tests;
- deterministic object cache, parallel runtime compilation, and performance reports;
- resilient long-lived TypeScript LSP workspaces.

## In-progress working tree

The current uncommitted compiler work is the first `JSValue` dynamic-boundary milestone. It includes partial semantic/HIR/MIR plumbing for `any`, boxing, and dynamic addition. It is intentionally separate from the concurrency plan and must be completed or checkpointed before concurrency implementation changes begin.

## Planned concurrency subsystem

`docs/CONCURRENCY.md` defines the implementation order for:

```text
bounded worker scheduler
-> lightweight tasks
-> spawn/join/yield
-> per-worker queues + work stealing
-> typed channels
-> timers + blocking pool
-> async/await state machines
-> structured concurrency/cancellation
-> scheduler/GC integration
```

## Immediate sequencing rule

Concurrency implementation starts only from a clean compiler checkpoint. The partial `JSValue` work must not be mixed into scheduler commits.

Expected first concurrency commits:

1. bounded runtime worker lifecycle;
2. lightweight task lifecycle and FIFO injection queue;
3. compiler spawn/join intrinsics;
4. per-worker deques;
5. work stealing and wakeups;
6. scheduler metrics and stress acceptance.

Every milestone must pass `go test ./...`, native acceptance where applicable, and `git diff --check` before commit.
