# tsv7-bin

`tsv7-bin` is an experimental TypeScript 7 to native-binary compiler written in Go.

Goal: compile strongly typed TypeScript 7 programs into native binaries without embedding Node.js, V8, or another JavaScript engine in the normal runtime path.

## Core architecture

- Go is the implementation language for the compiler, tooling, runtime tooling, and build driver.
- TypeScript 7 is the language and type-system authority.
- TypeScript-LS is the semantic frontend and IDE service.
- `TypeScript-LS` means the official TypeScript 7 native LSP (`tsc --lsp --stdio`).
- SWC is not used for parsing, semantic analysis, or compiler lowering.
- Typed semantic facts are normalized into compiler-owned HIR and MIR.
- Native representation proof decides between unboxed values and dynamic `JSValue`.
- LLVM is the initial machine-code backend.

## Current native MVP

The end-to-end native path now covers typed scalars, mutable SSA control flow, specialized `number[]`, and native strings. From the project root:

```bash
go build -o build/tsnative ./cmd/tsnative
./build/tsnative build examples/fib.ts -o build/fib -O2
./build/fib
```

Expected output:

```text
6765
```

The generated program is a native executable linked against the small tsnative C runtime and the platform C library; Node.js and V8 are not part of the runtime path.

Committed native coverage includes direct/recursive functions, numeric arithmetic and comparisons, mutable locals, `if`/`while`/`for` with SSA phi nodes, contiguous `number[]`, UTF-8 strings and concatenation, and native number/string console output. Closed object shapes are the active in-progress milestone.

Build optimization flags currently accepted are `-O0`, `-O1`, `-O2`, `-O3`, and `-Oz`. Use `-p <tsconfig.json>` to select the TypeScript project configuration.

## Design goals

- One TypeScript semantic source for editor and compiler.
- Preserve TypeScript 7 diagnostics and project resolution.
- Reuse a long-lived TypeScript-LS process for incremental development.
- Keep TypeScript/LSP details behind a Go frontend adapter.
- Prefer direct calls, closed shapes, typed arrays, and monomorphization.
- Keep dynamic runtime operations off typed hot paths.

## Non-goals for early milestones

- Reimplementing the TypeScript parser or type checker.
- Using SWC/Babel/Oxc as a parallel semantic frontend.
- Full JavaScript compatibility on day one.
- `eval`, `new Function`, Proxy, prototype mutation, or runtime-created modules.
- Embedding Node.js/V8 as the default execution engine.

See `docs/STATUS.md` for the current implementation boundary, then `docs/ARCHITECTURE.md`, `docs/ROADMAP.md`, `docs/TYPESCRIPT_LS.md`, and `TODO.md`.
