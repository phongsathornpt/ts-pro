# Refactoring boundaries

The September 2026 cleanup was a behavior-preserving decomposition pass before expanding WinterTC further.

## Primary results

| Area | Before | After core file |
| --- | ---: | ---: |
| IR generation | `irgen.go` 5,693 LOC | 482 LOC |
| Semantic checker | `sema.go` 2,630 LOC | 695 LOC |
| Parser | `parser.go` 1,435 LOC | 173 LOC |
| Native lowerer | `lower.go` 2,588 LOC | 295 LOC |
| Linux AMD64 main E2E | 2,285 LOC | 858 LOC |

The AMD64 lowering pipeline is now staged as entry generation, per-function lowering, runtime-symbol emission, and final fixup/linking. Web-facing IR generation and semantic checking are split into constructor, member, call, event, abort, and buffer modules so new WinterTC work does not grow the orchestration files again.

## Guardrails

Refactors should remain behavior-preserving and avoid combining feature work, GC algorithm changes, runtime ABI changes, or optimizer changes in the same commit. Each structural commit must run targeted tests and the full native/differential regression gates.

Large cohesive files are not automatically defects. GC, scheduler/tasks, Base64, number formatting, and similar single-domain runtime modules should be split only when responsibilities diverge or change pressure justifies another boundary.
