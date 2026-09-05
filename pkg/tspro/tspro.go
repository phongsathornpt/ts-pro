package tspro

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/backend/lower"
	"github.com/phongsathornpt/ts-pro/internal/backend/obj/elf"
	"github.com/phongsathornpt/ts-pro/internal/backend/obj/macho"
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
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
// Relative imports require CompileFile so the compiler has a filesystem module root.
func (c *Compiler) CompileSource(filename string, src []byte) ([]byte, diag.DiagnosticList, error) {
	file := c.fileSet.AddFile(filename, src)
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		return nil, diags, fmt.Errorf("parsing failed with %d diagnostics", len(diags))
	}
	for _, stmt := range prog.Statements {
		if _, ok := stmt.(*ast.ImportDecl); ok {
			return nil, diags, fmt.Errorf("relative imports require CompileFile")
		}
	}
	return c.compileProgram(prog, diags)
}

func (c *Compiler) compileProgram(prog *ast.Program, diags diag.DiagnosticList) ([]byte, diag.DiagnosticList, error) {
	semaRes := sema.Check(prog)
	allDiags := append(diags, semaRes.Diagnostics...)
	if semaRes.Diagnostics.HasErrors() {
		return nil, allDiags, fmt.Errorf("type checking failed with %d diagnostics", len(semaRes.Diagnostics))
	}
	irProg, err := irgen.Generate(prog, semaRes)
	if err != nil {
		return nil, allDiags, err
	}
	opt.Optimize(irProg, opt.Options{Level: c.opts.OptLevel})
	tgt, err := target.Parse(c.opts.TargetOS, c.opts.TargetArch)
	if err != nil {
		return nil, allDiags, err
	}
	code, err := lower.LowerTarget(irProg, tgt)
	if err != nil {
		return nil, allDiags, err
	}
	var bin []byte
	isARM64 := tgt.Arch == target.ArchARM64
	if tgt.OS == target.OSDarwin {
		bin, err = macho.CreateExecutable(code, isARM64)
	} else {
		bin, err = elf.CreateExecutable(code, isARM64)
	}
	if err != nil {
		return nil, allDiags, err
	}
	return bin, allDiags, nil
}

type moduleLoadState struct {
	loaded   map[string]bool
	visiting map[string]bool
}

func resolveModulePath(importer, spec string) (string, error) {
	if !strings.HasPrefix(spec, ".") {
		return "", fmt.Errorf("only relative TypeScript imports are supported, got %q", spec)
	}
	base := filepath.Clean(filepath.Join(filepath.Dir(importer), spec))
	candidates := []string{base}
	if filepath.Ext(base) == "" {
		candidates = append(candidates, base+".ts", filepath.Join(base, "index.ts"))
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return filepath.Abs(candidate)
		}
	}
	return "", fmt.Errorf("cannot resolve module %q from %q", spec, importer)
}

func (c *Compiler) loadModule(path string, state *moduleLoadState) ([]ast.Stmt, diag.DiagnosticList, error) {
	abs, _ := filepath.Abs(path)
	if state.visiting[abs] {
		return nil, nil, fmt.Errorf("cyclic module import involving %q", abs)
	}
	if state.loaded[abs] {
		return nil, nil, nil
	}
	state.visiting[abs] = true
	defer delete(state.visiting, abs)

	src, err := os.ReadFile(abs)
	if err != nil {
		return nil, nil, fmt.Errorf("read module %q: %w", abs, err)
	}
	file := c.fileSet.AddFile(abs, src)
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		return nil, diags, fmt.Errorf("parsing module %q failed with %d diagnostics", abs, len(diags))
	}

	var flattened []ast.Stmt
	allDiags := append(diag.DiagnosticList(nil), diags...)
	for _, stmt := range prog.Statements {
		imp, ok := stmt.(*ast.ImportDecl)
		if !ok {
			flattened = append(flattened, stmt)
			continue
		}
		resolved, err := resolveModulePath(abs, imp.Module)
		if err != nil {
			return nil, allDiags, err
		}
		deps, childDiags, err := c.loadModule(resolved, state)
		allDiags = append(allDiags, childDiags...)
		if err != nil {
			return nil, allDiags, err
		}
		flattened = append(flattened, deps...)
		flattened = append(flattened, imp)
	}
	state.loaded[abs] = true
	return flattened, allDiags, nil
}

func (c *Compiler) loadModuleProgram(inputPath string) (*ast.Program, diag.DiagnosticList, error) {
	stmts, diags, err := c.loadModule(inputPath, &moduleLoadState{
		loaded: make(map[string]bool), visiting: make(map[string]bool),
	})
	if err != nil {
		return nil, diags, err
	}
	var span source.Span
	if len(stmts) > 0 {
		span = source.Span{Start: stmts[0].Span().Start, End: stmts[len(stmts)-1].Span().End}
	}
	return &ast.Program{SourceSpan: span, Statements: stmts}, diags, nil
}

// CompileFile compiles a TypeScript source file on disk to a native executable file.
func (c *Compiler) CompileFile(inputPath string, outputPath string) (diag.DiagnosticList, error) {
	prog, diags, err := c.loadModuleProgram(inputPath)
	if err != nil {
		return diags, err
	}
	bin, allDiags, err := c.compileProgram(prog, diags)
	if err != nil {
		return allDiags, err
	}
	if err := os.WriteFile(outputPath, bin, 0o755); err != nil {
		return allDiags, fmt.Errorf("write output binary %q: %w", outputPath, err)
	}
	return allDiags, nil
}
