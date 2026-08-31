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

See `docs/ARCHITECTURE.md`, `docs/TYPESCRIPT_LS.md`, and `TODO.md`.
