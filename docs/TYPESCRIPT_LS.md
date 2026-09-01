# TypeScript-LS Frontend

## Definition

In this project, **TypeScript-LS** means the official TypeScript 7 native language server started with:

```bash
tsc --lsp --stdio
```

It does not mean the legacy `typescript-language-server` wrapper around `tsserver`.

## Why it replaces SWC

The compiler needs TypeScript semantics, not only a fast syntax tree. Using SWC would create a second parser/AST/type interpretation beside TypeScript 7 and introduce compatibility drift.

TypeScript-LS gives the project one authority for:

- source/project discovery;
- syntax and semantic diagnostics;
- module resolution;
- symbols and type information;
- editor navigation and incremental workspace state.

The same `tsconfig.json` must drive both IDE behavior and native compilation.

## Transitional client architecture

```text
internal/tsls/
  jsonrpc.go         JSON-RPC transport
  lsp.go             native LSP initialize/open/diagnostics lifecycle
  api_client.go      `tsc --api --async` process/session
  api_semantic.go    symbols/types/checker queries
  api_diagnostics.go compiler diagnostic queries
  api_object.go      object/property checker queries
```

The current Go tooling keeps one LS alive per workspace instead of spawning TypeScript for every file. This is transitional; final compiler/workspace ownership belongs in the TypeScript 7 implementation. Requests still use IDs, cancellation, deadlines, and bounded concurrency during migration.

## Semantic extraction rule

Standard LSP is used for IDE/workspace behavior, but native lowering uses the pinned official TypeScript 7 compiler API (`tsc --api --async`). That API supplies project snapshots, binary AST payloads, exact node handles, symbols, types, diagnostics, and checker queries.

`internal/tsls` owns the version-specific protocol. `internal/tsast` decodes the TypeScript 7 binary AST, and `internal/frontend` converts those facts into stable compiler DTOs. HIR/MIR never depend on raw LSP/API JSON. No fallback to SWC is planned.

## TypeScript 7.0.2 lifecycle note

The pinned server responds normally to `initialize`, but may not answer the standard `shutdown` request. The Go client therefore treats `shutdown` as best-effort with a short deadline and always sends `exit`, which terminates the server cleanly.

This behavior is covered by an integration test and must be rechecked on each TypeScript upgrade.

## Compiler semantic API

TypeScript 7.0.2 also ships `tsc --api --async`, an official JSON-RPC compiler API exposing snapshots, projects, source files, AST handles, symbols, types, signatures, and checker queries. The native compiler uses this API for lowering data that standard LSP does not expose.

The IDE/diagnostic path remains `tsc --lsp --stdio`. Both processes use the same pinned TypeScript 7 implementation and the same `tsconfig.json`; no SWC/Babel/Oxc parser is introduced. When LSP-hosted `custom/initializeAPISession` becomes reliable for the pinned release, the API transport can be switched to the shared LSP session without changing frontend DTOs or HIR.
