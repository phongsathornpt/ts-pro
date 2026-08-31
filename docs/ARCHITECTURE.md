# Architecture

## Compiler pipeline

```text
.ts / .tsx / .js
       |
       v
TypeScript 7 native frontend
IDE: `tsc --lsp --stdio`
compile: `tsc --api --async`
       |
       v
Go TypeScript client / semantic bridge
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

The compiler uses the pinned TypeScript 7 native API process (`tsc --api --async`) for project snapshots, diagnostics, binary AST payloads, exact AST-node handles, symbols, and checker type queries. The IDE path remains the official LSP process. Both use `typescript@7.0.2` and the same `tsconfig.json`.

```text
TypeScript 7 API (`tsc --api --async`)
    |
    +-- project snapshot + diagnostics
    +-- binary TypeScript AST
    +-- exact node handles
    +-- symbols and checker types
    |
    v
Go AST decoder + semantic DTOs -> HIR
```

Standard LSP is not treated as a compiler IR API. If a later TypeScript release makes a shared LSP-hosted API session reliable, `internal/tsls` may switch transports without changing HIR/MIR contracts.

## Backend ownership

The Go native compiler owns HIR/MIR, representation proof, native layouts, monomorphization, devirtualization, escape analysis, LLVM generation, linker orchestration, runtime ABI, allocator, GC, and native APIs.

The central rule remains: a TypeScript type is evidence, not by itself a proof of runtime representation.


## Current native data model

Committed fast paths use `F64` for TypeScript `number`, SSA `Bool` conditions, `StringRef` for length-aware UTF-8 strings, and specialized contiguous `ArrayRef<F64>` storage. Mutable loops remain in SSA through phi nodes. `I32`/`I64` narrowing is reserved for future range proofs rather than inferred from the TypeScript `number` annotation alone. Closed object shapes are currently being added from checker-derived property metadata.
