# Go Project Layout

The active compiler implementation follows a Go-first layout similar in spirit to TypeScript 7's native implementation: small packages, explicit ownership, immutable IDs where practical, and parallel work only behind deterministic interfaces.

```text
cmd/tsnative/              CLI entry point
internal/tsls/             TypeScript 7 LSP + compiler API transports
internal/tsast/            TypeScript 7 binary AST decoder
internal/frontend/         normalized compiler semantic DTOs
internal/hir/              typed high-level IR
internal/analysis/         representation, escape, effects, call graph
internal/mir/              native-oriented SSA/MIR
internal/lowering/         semantic DTO -> HIR -> MIR lowering
internal/codegen/llvm/     deterministic textual LLVM generation
internal/toolchain/        clang compile/link orchestration
internal/compiler/         end-to-end native build pipeline
runtime/                   native runtime sources/ABI
pkg/diagnostic/            stable user-facing diagnostics if needed
```

## Package boundaries

`internal/tsls` is the only package allowed to know LSP/API request names, JSON-RPC envelopes, TypeScript process lifecycle details, or version-specific compiler API calls. `internal/tsast` owns only the pinned binary AST decoding contract.

`internal/frontend` converts TypeScript API/AST results into stable compiler concepts. HIR and later layers must not import `internal/tsls` or depend on raw TypeScript protocol structures.

`internal/hir` keeps TypeScript semantic type IDs separate from native `Repr`. `internal/mir` contains only representation-proven operations suitable for code generation.

## Concurrency target

The JSON-RPC client already multiplexes requests and supports cancellation. Persistent per-workspace LS ownership and restart policy remain planned. The target model is:

- One long-lived TypeScript-LS process per IDE workspace.
- One JSON-RPC reader loop and serialized writer path per LS connection.
- Requests are multiplexed by request ID into Go channels/futures.
- Every request carries `context.Context` for cancellation/deadlines.
- HIR/MIR optimization and LLVM module generation may run in worker pools after semantic snapshots are frozen.
- Build output must remain deterministic regardless of goroutine scheduling.

## Migration status

The Rust prototype has been removed after Go parity was established for the active pipeline. New compiler work is Go-only unless a runtime component explicitly requires another implementation language.
