# tsv7-bin

`tsv7-bin` is an experimental TypeScript 7 native compiler project.

Goal: compile strongly typed TypeScript 7 programs into native binaries without embedding Node.js, V8, or another JavaScript engine in the normal runtime path.

## Design goals

- TypeScript 7 is the language/type authority.
- Preserve TypeScript diagnostics before native lowering.
- Lower typed programs into compiler-owned HIR and MIR.
- Prefer unboxed native representations over generic JS values.
- Use dynamic runtime values only where static representation cannot be proven.
- Emit LLVM IR and link native executables.
- Optimize for startup time, memory use, and typed hot-path performance.

## Non-goals for the first milestones

- Full JavaScript compatibility.
- `eval`, `new Function`, Proxy, prototype mutation, or dynamic module loading.
- Reimplementing the TypeScript type checker.
- Embedding Node.js/V8 as the default execution engine.

See `docs/` for architecture and `TODO.md` for implementation status.
