# Roadmap

## Milestone 0 — Rebase architecture on Go

- Freeze the existing Rust prototype as reference only.
- Add a Go module and `cmd/tsnative` CLI.
- Pin TypeScript 7 as the language-service dependency.
- Validate `tsc --lsp --stdio` from the Go driver.
- Keep strict native `tsconfig` as the project contract.

## Milestone 1 — TypeScript-LS client

- Implement JSON-RPC/LSP stdio transport in Go.
- Initialize one long-lived TypeScript-LS per workspace.
- Implement document/project lifecycle and cancellation.
- Collect diagnostics and project configuration.
- Add health/restart handling for crashed LS processes.
- Add integration tests against pinned TypeScript 7.

## Milestone 2 — Semantic bridge

- Define stable compiler-owned symbol/type/source DTOs.
- Extract function signatures, declarations, references, and types.
- Add semantic extraction needed for native lowering.
- Version-pin any TypeScript 7 extension required beyond standard LSP.
- Ensure no SWC/Babel/Oxc parser exists in the compiler path.

## Milestone 3 — Typed HIR in Go

- Define module/function/block/value IDs and semantic type IDs.
- Lower literals, locals, arithmetic, comparisons, calls, branches, and returns.
- Keep TypeScript semantic type separate from native representation.
- Add HIR verifier and stable textual dump.

## Milestone 4 — MIR and LLVM

- Add representation proof for Bool, F64, integers, references, and static functions.
- Lower HIR into MIR/SSA.
- Emit textual LLVM IR from Go.
- Use clang/lld for the first native executable.
- Compile the `fib.ts` acceptance program without Node/V8 at runtime.

## Milestone 5 — Native data model

- Strings and specialized arrays.
- Closed object/class shapes with fixed offsets.
- Closures and function values.
- Monomorphized generics and direct-call specialization.
- Escape analysis and scalar replacement.

## Milestone 6 — Dynamic boundary and scale

- Tagged `JSValue` only for genuinely dynamic values.
- Checked conversions, dynamic operators, and property slow paths.
- Heap allocator and initial mark/sweep GC.
- Exceptions, Promise, and async/await.
- Incremental object cache, parallel codegen, ThinLTO, PGO, and cross compilation.
