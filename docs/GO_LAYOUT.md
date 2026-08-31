# Go Project Layout

The active compiler implementation follows a Go-first layout similar in spirit to TypeScript 7's native implementation: small packages, explicit ownership, immutable IDs where practical, and parallel work only behind deterministic interfaces.

```text
cmd/tsnative/              CLI entry point
internal/tsls/             TypeScript 7 LSP process + semantic bridge
internal/frontend/         normalized compiler semantic DTOs
internal/hir/              typed high-level IR
internal/analysis/         representation, escape, effects, call graph
internal/mir/              native-oriented SSA/MIR
internal/codegen/llvm/     LLVM text/object generation
internal/linker/           clang/lld orchestration
internal/build/            graph, cache, parallel scheduling
runtime/                   native runtime sources/ABI
pkg/diagnostic/            stable user-facing diagnostics if needed
```

## Package boundaries

`internal/tsls` is the only package allowed to know LSP request names, JSON-RPC envelopes, TypeScript-LS lifecycle details, or version-specific semantic extensions.

`internal/frontend` converts LS results into stable compiler concepts. HIR and later layers must not import `internal/tsls`.

`internal/hir` keeps TypeScript semantic type IDs separate from native `Repr`. `internal/mir` contains only representation-proven operations suitable for code generation.

## Concurrency model

- One long-lived TypeScript-LS process per workspace.
- One JSON-RPC reader loop and serialized writer path per LS connection.
- Requests are multiplexed by request ID into Go channels/futures.
- Every request carries `context.Context` for cancellation/deadlines.
- HIR/MIR optimization and LLVM module generation may run in worker pools after semantic snapshots are frozen.
- Build output must remain deterministic regardless of goroutine scheduling.

## Migration rule

Do not mechanically port every Rust type. Preserve the architectural contracts, then implement idiomatic Go structures and tests. The Rust prototype remains a reference until equivalent Go tests pass, after which it can be removed from the active build.
