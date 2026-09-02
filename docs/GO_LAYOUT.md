# Go Runtime Layout

Go is reserved for the **native runtime and native libraries only**. It is not the target implementation language for the TypeScript compiler.

## Target ownership

```text
TypeScript 7 compiler side
  parser/checker/project semantics
  semantic DTOs
  HIR / MIR / SSA
  representation proof and optimization
  Pure-Go code generation and build orchestration

Go runtime side (package runtimego, CGO_ENABLED=0 pure-Go)
  allocator, spans, heap, nursery, roots, barriers, blocktable, gcmark
  strings, arrays, objects, JSValue, handles
  scheduler, task, channels, timers, blocking pool, task groups, context
  platform thread identification (thread_darwin.go, thread_linux.go, thread_other.go)
  runtime.go: runtime coordinator and public symbols
```

## Hard boundary

- Do not add new compiler/frontend/HIR/MIR/codegen features in Go as target architecture.
- Current `cmd/` and `internal/` Go compiler code is transitional implementation debt while the TypeScript 7 compiler path reaches parity.
- New Go code belongs under runtime/native-library ownership unless it is temporary migration glue explicitly marked for deletion.
- Go runtime code must not own TypeScript parsing, checker semantics, compiler IR, LLVM lowering policy, or project resolution.
- The runtime ABI remains native and stable so compiler implementation language changes do not affect generated-code contracts.

## Migration rule

Retire compile-time Go packages incrementally after equivalent TypeScript 7 stages pass differential/native regressions. Do not remove a working Go stage before TypeScript parity exists, but do not treat that Go stage as the final design.
