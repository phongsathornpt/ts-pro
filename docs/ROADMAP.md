# Roadmap

## Active raw-native roadmap: complete

The repository-defined active raw-native roadmap is complete. The current acceptance state is:

```text
152 / 152 native fixtures PASS
0 diagnostics
0 build/lowering failures
0 runtime failures
0 timeouts
52 / 52 Node-comparable deterministic fixtures match
```

The original core-roadmap completion milestone was **125 / 125** native fixtures; later performance and WinterTC work expanded the suite without changing that historical acceptance point. The full current acceptance gate is documented in `docs/STATUS.md` and `TODO.md`.

### Completed milestones

1. Linux AMD64 standalone executable generation and SysV ABI.
2. F64/string/array/object/closure runtime and precise mark-sweep GC.
3. Generics, tuples, classes, inheritance, virtual dispatch, and multi-module relative linking.
4. Dynamic `JSValue` semantics including coercion, operators, properties, calls, and `this`.
5. Standard fixture APIs: JSON, Date, Map/Set, and RegExp subset.
6. Stackful cooperative scheduler with precise suspended roots.
7. Typed tasks/channels, cancellation, task context, task groups, and scheduler-aware timers.
8. Async/await, throw/catch/finally, Promise lifecycle, adoption, thenables, repeated await, `Promise.all`, and `Promise.race`.
9. Deterministic native fixture acceptance and Node differential validation.

### Superseded design item

The earlier plan for compiler-generated async state machines is superseded for the active raw backend by stackful cooperative task stacks. This is intentional and recorded as `[S]` in `TODO.md`.

## Optional post-completion roadmap

These are **new scope**, not blockers for the completed fixture roadmap.

### Performance

- keep numeric SSA values in XMM registers longer and reduce payload shuffling;
- benchmark allocator/GC pause and throughput under larger heaps;
- improve register allocation, code layout, code size, and branch quality;
- add PGO/LTO experiments where they measurably help raw-native output;
- establish Bun/Node/Go/native comparative benchmark suites for HTTP and compute workloads.

### Language and ecosystem expansion

- broaden TypeScript/ECMAScript syntax beyond the current fixture set;
- expand built-in objects and Web-standard APIs as separately defined specs require;
- strengthen npm/package/module resolution and cross-module optimization;
- improve dynamic object semantics and uncommon coercion/property edge cases beyond current differential coverage.

### Platform parity

- bring the complete active feature surface to ARM64/macOS and Windows targets;
- add cross-platform fixture and differential gates equivalent to Linux AMD64.

### Reliability and production hardening

- [x] parser/sema/IR/backend fuzzing with fixed seed corpora plus scheduled compiler fuzzing;
- [x] replayable seeded GC/scheduler/channel/Promise stress with a stable CI corpus and one fresh nightly seed;
- [ ] differential property testing against Node/TypeScript for broader generated programs;
- [ ] security review of executable writer, runtime memory handling, and dynamic boundaries;
- [ ] reproducible benchmark and release artifacts.

## Historical roadmaps

Older TypeScript-7 migration, LLVM, pure-Go generated-code, c-archive, stackless scheduler, and transitional compiler plans are retained in git history and the historical sections of `TODO.md`. They are not active completion criteria.
