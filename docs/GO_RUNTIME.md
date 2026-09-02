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

## Current boundary: Pure Go (`CGO_ENABLED=0`)

The native runtime under `runtime/` is **100% pure Go**:
- Zero `import "C"`, zero `//export`, and zero assembly files (`.s`).
- All 220 runtime functions follow idiomatic `camelCase` naming (e.g. `heapAlloc`, `gcCollect`, `schedulerSpawn`, `channelF64Send`). The legacy `tsnative_*` prefix has been eliminated.
- Virtual memory mapping is cleanly decoupled by target OS: POSIX platforms (Darwin, Linux) use `syscall.Mmap` in `runtime/mmap_posix.go`; Windows uses `VirtualAlloc`/`VirtualFree` in `runtime/mmap_windows.go`.
- Code generation defaults to pure-Go (`internal/codegen/golang`), outputting executable Go packages that compile under `CGO_ENABLED=0`.
- All 48 runtime unit tests and repository-wide test suites pass cleanly under `CGO_ENABLED=0 go test ./...`.
