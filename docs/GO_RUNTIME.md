# Go native runtime migration

Goal: make Go the **only handwritten native runtime/native-library implementation language**, replacing every handwritten runtime `.c` implementation while preserving the LLVM C ABI and native memory layouts during migration. Go must not expand upward into compiler/frontend/HIR/MIR/codegen ownership.

## Rules

- Migrate one subsystem at a time and delete its C source only after native regression coverage passes.
- Keep ABI names stable so LLVM lowering does not change merely because the runtime implementation language changes.
- Use a cached Go `c-archive` for exported ABI functions. Generated cgo glue is toolchain output; the repository should end with no handwritten runtime `.c` files.
- Keep raw layouts native-compatible until LLVM stops directly reading/writing those layouts. Do not expose movable Go heap pointers as long-lived native pointers.

## Migration order

1. [x] Go c-archive build/cache path and numeric console ABI; remove `core/console.c`.
2. [x] Heap/native allocator and GC ABI; remove `core/heap.c`.
3. [~] Strings, arrays, objects, and JSValue.
   - [x] String allocation/concat/logging; remove `core/string.c`.
   - [ ] F64 arrays.
   - [ ] Objects.
   - [ ] JSValue.
4. [ ] Scheduler and lightweight tasks using Go concurrency primitives where ABI-safe.
5. [ ] Channels, timers, and blocking-call pool.
6. [ ] Remove legacy C headers/tests and the C compilation path once no runtime C sources remain.
