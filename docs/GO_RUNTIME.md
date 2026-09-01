# Go native runtime migration

Goal: keep Go as the **only handwritten native runtime/native-library implementation language** while preserving the LLVM C ABI. The handwritten runtime C migration is complete; cgo-generated glue/header output remains toolchain-facing only. Go must not expand upward into compiler/frontend/HIR/MIR/codegen ownership.

## Rules

- Migrate one subsystem at a time and delete its C source only after native regression coverage passes.
- Keep ABI names stable so LLVM lowering does not change merely because the runtime implementation language changes.
- Use a cached Go `c-archive` for exported ABI functions. The cache publishes the generated `.h` beside the archive so ABI tests do not require handwritten runtime headers.
- Keep raw layouts native-compatible until LLVM stops directly reading/writing those layouts. Do not expose movable Go heap pointers as long-lived native pointers.

## Migration order

1. [x] Go c-archive build/cache path and numeric console ABI; remove `core/console.c`.
2. [x] Heap/native allocator and GC ABI; remove `core/heap.c`.
3. [x] Strings, arrays, objects, and JSValue.
   - [x] String allocation/concat/logging; remove `core/string.c`.
   - [x] F64 arrays.
   - [x] Objects.
   - [x] JSValue.
4. [x] Scheduler and lightweight tasks using Go concurrency primitives with Go-owned handles, completion state, roots, groups, and task context.
5. [x] Channels, timers, and blocking-call pool.
6. [x] Remove legacy C headers/tests and the C compilation path; native ABI tests use the generated `c-archive` header.

## Current boundary

`runtimego/` owns the runtime implementation. C appears only as generated cgo ABI glue or tiny in-source cgo trampolines required to cross safely between Go and LLVM/SysV callbacks; there are no handwritten runtime `.c` implementation files in the repository.
