# Architecture

## Compiler pipeline

```text
TypeScript 7 source
       |
       v
TypeScript 7 validation + semantic frontend
       |
       v
TS7 adapter boundary
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

The backend must never depend directly on TypeScript AST node shapes. All TS-version-specific details live behind the frontend adapter.

## Frontend boundary

TypeScript 7 owns syntax, module resolution, binding, narrowing, overload resolution, generic semantics, and diagnostics. The native compiler consumes only normalized semantic facts through an adapter.

The initial adapter may invoke the official TypeScript 7 CLI where no stable public programmatic API exists. That subprocess boundary is temporary and isolated; backend IR must not depend on it.

## Backend ownership

The native compiler owns:

- HIR and MIR definitions.
- Runtime representation selection.
- Native object/array layouts.
- Monomorphization and function specialization.
- Devirtualization and escape analysis.
- LLVM code generation.
- Runtime ABI, allocator, GC, and native APIs.

The central rule is: a TypeScript type is evidence, not by itself a proof of runtime representation.
