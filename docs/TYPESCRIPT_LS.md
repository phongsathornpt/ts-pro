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

## Go client architecture

```text
internal/tsls/
  process.go       start/stop/restart LS
  jsonrpc.go       JSON-RPC transport
  lsp.go           initialize/open/change/diagnostics
  workspace.go     project and document lifecycle
  semantic.go      compiler semantic extraction
  dto.go           stable compiler-owned DTOs
```

The Go process should keep one LS alive per workspace instead of spawning TypeScript for every file. Requests use IDs, cancellation, deadlines, and bounded concurrency.

## Semantic extraction rule

Standard LSP is sufficient for IDE features but not a complete compiler IR API. HIR lowering must therefore consume stable DTOs from `internal/tsls`, never raw LSP JSON and never TypeScript AST nodes.

If richer AST/type/control-flow data is unavailable through public LSP methods, implement the smallest possible version-pinned extension on top of TypeScript 7. Keep that extension isolated and covered by compatibility tests.

No fallback to SWC is planned.
