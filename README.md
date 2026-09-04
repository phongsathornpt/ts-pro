# ts-pro

`ts-pro` is a pure Go native TypeScript compiler and embedded runtime toolchain. It compiles strongly-typed TypeScript programs directly into standalone native machine code executables (Linux ELF64, macOS Mach-O 64, Windows PE/COFF 64) with `CGO_ENABLED=0` and zero external dependencies (no Node.js, V8, external C toolchains, or Clang).

## Current Status

The active Linux AMD64 raw-native compiler roadmap is complete for the repository acceptance surface:

- **125 / 125** native fixtures compile and run.
- **52 / 52** Node-comparable deterministic fixtures match stdout and exit status.
- Dynamic, classes/generics, modules, standard fixture APIs, stackful concurrency, async/await, and Promise/thenable/aggregate coverage are included.
- The exact validation commands and scope are documented in [`docs/STATUS.md`](docs/STATUS.md) and [`TODO.md`](TODO.md).

Future work is optional scope expansion and optimization, not unfinished fixture acceptance. See [`docs/ROADMAP.md`](docs/ROADMAP.md).

## Project Structure

```text
ts-pro/
├── cmd/
│   └── ts-pro/                    # CLI entry point (main.go)
├── internal/
│   ├── core/                      # Language Model & Intermediate Representations
│   │   ├── token/                 # Token definitions & source locations (Span)
│   │   ├── ast/                   # Abstract Syntax Tree nodes
│   │   ├── types/                 # Static type representations (interfaces, unions, generics)
│   │   └── ir/                    # SSA-form 3-address Intermediate Representation
│   │
│   ├── frontend/                  # Syntactic & Semantic Analysis
│   │   ├── lexer/                 # Pure Go scanner (UTF-8 bytes -> tokens)
│   │   ├── parser/                # Recursive descent parser (tokens -> AST)
│   │   └── sema/                  # Type inference, type checking, symbol resolution
│   │
│   ├── midend/                    # Platform-Agnostic Transformations
│   │   ├── irgen/                 # Lowers AST to Linear SSA IR
│   │   └── opt/                   # Dead-code elimination, constant folding, devirtualization
│   │
│   ├── backend/                   # Pure Go Code Generation
│   │   ├── regalloc/              # Register allocation (Linear Scan)
│   │   ├── asm/                   # Instruction definitions & encoders
│   │   │   ├── amd64/             # x86-64 opcode table & instruction encoding
│   │   │   └── arm64/             # AArch64 instruction encodings
│   │   ├── lower/                 # Lowers IR -> Target Assembly Instructions
│   │   └── obj/                   # Executable format emitters (Pure Go writers)
│   │       ├── elf/               # Linux ELF64 binary generator
│   │       ├── macho/             # macOS Mach-O 64-bit generator
│   │       └── pe/                # Windows PE/COFF 64-bit generator
│   │
│   ├── runtime/                   # Embedded TypeScript Runtime (Pure Go / Embedded ASM)
│   │   ├── src/                   # Runtime primitives statically embedded via //go:embed
│   │   │   ├── gc/                # Minimal Mark-Sweep or Arena memory allocator
│   │   │   ├── string/            # UTF-16 / UTF-8 string layout & slice operations
│   │   │   ├── array/             # Dynamic heap-allocated backing arrays
│   │   │   ├── closure/           # Upvalue & closure environment management
│   │   │   └── sys/               # Raw OS syscall wrappers (no libc dependency)
│   │   └── runtime.go             # Bundles and links the runtime into generated code
│   │
│   └── support/                   # Diagnostic & File Utilities
│       ├── diag/                  # Error reporting with source line/column visualizers
│       └── source/                # Virtual file system & source memory buffers
├── pkg/
│   └── tspro/                     # Embeddable Go API for external tools
├── examples/                      # TypeScript samples and fixtures
├── go.mod
└── Makefile
```

## Quick Start

### Build Compiler

```bash
make build
```

The compiled CLI binary is produced at `build/ts-pro`.

### Compile TypeScript to Native Executable

```bash
./build/ts-pro build examples/basics/fib.ts -o build/fib -O2
```

### Type Check

```bash
./build/ts-pro check examples/basics/fib.ts
```

### Doctor Verification

```bash
./build/ts-pro doctor
```

### Quality Assurance

```bash
make check
```
Runs `go fmt`, `go vet`, `go test ./...`, and `go build`.
