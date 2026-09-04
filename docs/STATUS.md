# Implementation Status

## Active compiler status

The active handwritten TypeScript frontend → SSA → raw-native backend is complete for the repository acceptance surface.

Current acceptance:

- **125 / 125** native fixtures compile and run successfully.
- **0** diagnostics, lowering/build failures, runtime failures, or timeouts.
- **52 / 52** Node-comparable deterministic fixtures match stdout and exit status.
- 7 differential fixtures are explicitly skipped only because Node strip-types cannot directly execute enum syntax, extensionless TypeScript imports, or parameter-property syntax.
- `CGO_ENABLED=0 go test ./...`, `go vet ./...`, `go build ./...`, `make test-linux-amd64`, native fixture sweep, Node differential sweep, and `git diff --check` pass.

The generated Linux AMD64 executable is standalone and does not embed Node.js or V8.

## Implemented language/runtime surface

The active backend covers the fixture-tested surface for:

- functions, recursion, closures, escaping function values, generics, tuples, classes, inheritance, `super`, overrides, and closed-world virtual dispatch;
- arrays, strings, closed objects, evolving dynamic objects, destructuring, rest/spread, optional/nullish semantics, enums, templates, and multi-module relative imports;
- NaN-boxed `JSValue`, `any`/union boxing boundaries, coercion, dynamic arithmetic/comparison/equality, properties, calls, and receiver-correct `this`;
- JSON, Date UTC operations, Map/Set, and the supported RegExp subset;
- precise native GC roots for strings, arrays, objects, closures, dynamic values, suspended task stacks, channels, and Promise/thenable state;
- stackful cooperative tasks, spawn/join/yield, scheduler-aware timers, typed channels, cancellation, task context, task groups, async/await, throw/catch/finally, Promise resolve/reject/adoption/thenables, repeated await, `all`, and `race`.

## Async execution model

The active raw backend intentionally uses **stackful cooperative task stacks** rather than compiler-generated async state machines. Each task owns saved native execution state and a precise suspended shadow-root chain. This design supersedes the earlier stackless/state-machine roadmap item for the active backend.

## Validation commands

```bash
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go build ./...
make test-linux-amd64
./scripts/test-native-fixtures.sh
./scripts/test-node-differential.py
git diff --check
```

## Historical architecture notes

Older LLVM, c-archive, pure-Go generated-code, TypeScript-7 migration, and stackless-scheduler plans in historical documents/commits are reference material only. `TODO.md` defines the source of truth for the active raw-native backend.

## Optional next roadmap

The completed fixture roadmap does not imply the compiler implements all TypeScript/ECMAScript APIs. Future work is optional scope expansion rather than unfinished acceptance work:

- broader TypeScript/ECMAScript syntax and standard-library coverage;
- ARM64/macOS/Windows execution parity for the full active feature surface;
- fuzzing and differential-property testing beyond curated fixtures;
- allocator, GC, scheduler, register-allocation, and code-size performance work;
- stronger module/package resolution and ecosystem compatibility;
- production hardening, security review, benchmark baselines, PGO/LTO, and cross-compilation ergonomics.
