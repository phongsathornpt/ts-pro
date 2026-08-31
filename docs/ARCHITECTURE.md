# Architecture

## Compiler pipeline

```text
.ts / .tsx / .js
       |
       v
TypeScript 7 native language server
`tsc --lsp --stdio`
       |
       v
Go TypeScript-LS client / semantic bridge
       |
       v
compiler-owned semantic DTOs
       |
       v
Typed HIR
       |
       +--> representation analysis
       +--> call graph / specialization
       +--> escape/effect analysis
       |
       v
MIR / SSA
       |
       v
LLVM IR -> object files -> native linker -> executable
```

The compiler is Go-first. TypeScript 7 owns language semantics; our Go code owns normalization, native lowering, optimization, code generation, and runtime behavior.

## Frontend ownership

TypeScript 7 owns:

- parsing and syntax diagnostics;
- `tsconfig.json` and project discovery;
- module resolution;
- binding, control-flow narrowing, overload resolution, and generics;
- semantic diagnostics and language-service features.

The native compiler does **not** run SWC or another parser beside TypeScript-LS. A single TypeScript semantic source prevents AST/type drift between IDE diagnostics and native code generation.

## IDE path

```text
VS Code / Neovim / Helix / other LSP client
                 |
                 v
        TypeScript 7 LSP
        tsc --lsp --stdio
```

Editors talk directly to the official TypeScript 7 language server. The compiler must not implement a second editor language server.

## Compile path

The Go compiler starts or connects to a long-lived TypeScript-LS workspace process. Standard LSP is used for lifecycle, project state, diagnostics, and editor-compatible semantic queries.

Native lowering needs richer information than standard LSP guarantees. The frontend adapter therefore owns a semantic bridge layer that can evolve without leaking TypeScript internals into HIR. Until TypeScript exposes a stable semantic API, this bridge may require a small pinned TypeScript 7 extension/fork, but it must remain TypeScript-based rather than introducing SWC.

```text
TypeScript-LS
    |
    +-- diagnostics/project graph
    +-- symbols/types/control-flow facts
    +-- semantic extraction bridge
    |
    v
Go frontend DTOs -> HIR
```

## Backend ownership

The Go native compiler owns HIR/MIR, representation proof, native layouts, monomorphization, devirtualization, escape analysis, LLVM generation, linker orchestration, runtime ABI, allocator, GC, and native APIs.

The central rule remains: a TypeScript type is evidence, not by itself a proof of runtime representation.
