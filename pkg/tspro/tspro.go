package tspro

import (
	"fmt"
	"os"
	"runtime"

	"github.com/phongsathornpt/ts-pro/internal/backend/lower"
	"github.com/phongsathornpt/ts-pro/internal/backend/obj/elf"
	"github.com/phongsathornpt/ts-pro/internal/backend/obj/macho"
	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
	"github.com/phongsathornpt/ts-pro/internal/midend/irgen"
	"github.com/phongsathornpt/ts-pro/internal/midend/opt"
	"github.com/phongsathornpt/ts-pro/internal/support/diag"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
	"github.com/phongsathornpt/ts-pro/internal/target"
)

// Options configures compiler output.
type Options struct {
	TargetOS   string // currently: "linux" or "darwin"
	TargetArch string // currently: "amd64" or "arm64" where supported by TargetOS
	OptLevel   int    // 0, 1, 2, 3
}

// DefaultOptions returns default options targeting the host OS and architecture.
func DefaultOptions() Options {
	return Options{
		TargetOS:   runtime.GOOS,
		TargetArch: runtime.GOARCH,
		OptLevel:   2,
	}
}

// Compiler orchestrates the pure-Go compilation pipeline.
type Compiler struct {
	opts    Options
	fileSet *source.FileSet
}

// New creates a new Compiler instance with the given options.
func New(opts Options) *Compiler {
	if opts.TargetOS == "" {
		opts.TargetOS = runtime.GOOS
	}
	if opts.TargetArch == "" {
		opts.TargetArch = runtime.GOARCH
	}
	return &Compiler{
		opts:    opts,
		fileSet: source.NewFileSet(),
	}
}

// FileSet returns the source file registry used by this compiler.
func (c *Compiler) FileSet() *source.FileSet {
	return c.fileSet
}

// Check validates TypeScript source and returns semantic diagnostics.
func (c *Compiler) Check(filename string, src []byte) diag.DiagnosticList {
	file := c.fileSet.AddFile(filename, src)
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		return diags
	}
	semaRes := sema.Check(prog)
	return append(diags, semaRes.Diagnostics...)
}

// CompileSource compiles TypeScript source code into an executable binary image.
func (c *Compiler) CompileSource(filename string, src []byte) ([]byte, diag.DiagnosticList, error) {
	file := c.fileSet.AddFile(filename, src)

	// 1. Parsing
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		return nil, diags, fmt.Errorf("parsing failed with %d diagnostics", len(diags))
	}

	// 2. Semantic Analysis
	semaRes := sema.Check(prog)
	allDiags := append(diags, semaRes.Diagnostics...)
	if semaRes.Diagnostics.HasErrors() {
		return nil, allDiags, fmt.Errorf("type checking failed with %d diagnostics", len(semaRes.Diagnostics))
	}

	// 3. SSA IR Generation
	irProg, err := irgen.Generate(prog, semaRes)
	if err != nil {
		return nil, allDiags, fmt.Errorf("ir generation failed: %w", err)
	}

	// 4. Optimization
	opt.Optimize(irProg, opt.Options{Level: c.opts.OptLevel})

	// 5. Validate the target before instruction selection.
	tgt, err := target.Parse(c.opts.TargetOS, c.opts.TargetArch)
	if err != nil {
		return nil, allDiags, err
	}

	code, err := lower.LowerTarget(irProg, tgt)
	if err != nil {
		return nil, allDiags, fmt.Errorf("lowering failed: %w", err)
	}

	// 6. Object / Executable Emission
	var bin []byte
	isARM64 := tgt.Arch == target.ArchARM64
	switch tgt.OS {
	case target.OSLinux:
		bin, err = elf.CreateExecutable(code, isARM64)
	case target.OSDarwin:
		bin, err = macho.CreateExecutable(code, isARM64)
	default:
		return nil, allDiags, fmt.Errorf("unsupported target OS: %s", tgt.OS)
	}
	if err != nil {
		return nil, allDiags, fmt.Errorf("executable emission failed: %w", err)
	}

	return bin, allDiags, nil
}

// CompileFile compiles a TypeScript source file on disk to a native executable file.
func (c *Compiler) CompileFile(inputPath string, outputPath string) (diag.DiagnosticList, error) {
	src, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read input file %q: %w", inputPath, err)
	}

	bin, diags, err := c.CompileSource(inputPath, src)
	if err != nil {
		return diags, err
	}

	if err := os.WriteFile(outputPath, bin, 0o755); err != nil {
		return diags, fmt.Errorf("write output binary %q: %w", outputPath, err)
	}

	return diags, nil
}
