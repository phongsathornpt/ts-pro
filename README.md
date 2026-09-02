# ts-pro

`ts-pro` is an experimental TypeScript 7 to native-binary compiler. TypeScript 7 owns the compiler implementation; Go is reserved for the native runtime and native libraries.

Goal: compile strongly typed TypeScript 7 programs into native binaries without embedding Node.js, V8, or another JavaScript engine in the normal runtime path.

## Core architecture

- TypeScript 7 is the compiler implementation, language, parser, binder, checker, and semantic authority.
- Go is used only for the native runtime and native libraries behind stable native ABI boundaries.
- Compile-time HIR/MIR/representation analysis/LLVM generation/build orchestration must not depend on Go in the target architecture.
- TypeScript-LS is the semantic frontend and IDE service.
- `TypeScript-LS` means the official TypeScript 7 native LSP (`tsc --lsp --stdio`).
- SWC is not used for parsing, semantic analysis, or compiler lowering.
- Typed semantic facts are normalized into compiler-owned HIR and MIR.
- Native representation proof decides between unboxed values and dynamic `JSValue`.
- LLVM is the initial machine-code backend.

## Current native MVP

The current transitional end-to-end path still uses the existing Go compiler driver while the TypeScript 7 compiler implementation is brought to parity. It covers typed scalars, mutable SSA control flow, specialized `number[]`, and native strings. From the project root:

```bash
go build -o bin/tspro ./cmd/tspro
./bin/tspro build examples/basics/fib.ts -o build/fib -O2
./build/fib
```

Expected output:

```text
6765
```

The generated program is a native executable built under `CGO_ENABLED=0` without cgo, assembly, or external C toolchain dependencies; Node.js and V8 are not part of the runtime path. Handwritten C and assembly have been completely eliminated from the repository.

Committed native coverage includes direct/recursive functions, numeric arithmetic and comparisons, mutable locals, `if`/`while`/`for` with SSA phi nodes, contiguous `number[]`, UTF-8 strings and concatenation, and native number/string console output. Closed object shapes are the active in-progress milestone.

Build optimization flags currently accepted are `-O0`, `-O1`, `-O2`, `-O3`, and `-Oz`. Use `-p <tsconfig.json>` to select the TypeScript project configuration.

## Design goals

- One TypeScript semantic source for editor and compiler.
- Preserve TypeScript 7 diagnostics and project resolution.
- Reuse a long-lived TypeScript-LS process for incremental development.
- Keep compile-time logic inside the TypeScript 7 compiler implementation; do not introduce a second Go compiler frontend.
- Prefer direct calls, closed shapes, typed arrays, and monomorphization.
- Keep dynamic runtime operations off typed hot paths.

## Non-goals for early milestones

- Reimplementing the TypeScript parser or type checker.
- Using SWC/Babel/Oxc as a parallel semantic frontend.
- Full JavaScript compatibility on day one.
- `eval`, `new Function`, Proxy, prototype mutation, or runtime-created modules.
- Embedding Node.js/V8 as the default execution engine.

See `docs/STATUS.md` for the current implementation boundary, then `docs/ARCHITECTURE.md`, `docs/ROADMAP.md`, `docs/TYPESCRIPT_LS.md`, and `TODO.md`.
