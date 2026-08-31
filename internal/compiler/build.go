package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	repranalysis "github.com/projectthorn/tsv7-bin/internal/analysis/repr"
	llvmcodegen "github.com/projectthorn/tsv7-bin/internal/codegen/llvm"
	"github.com/projectthorn/tsv7-bin/internal/frontend"
	"github.com/projectthorn/tsv7-bin/internal/lowering"
	"github.com/projectthorn/tsv7-bin/internal/toolchain"
	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

type BuildOptions struct {
	Root         string
	Input        string
	Output       string
	Config       string
	Optimization string
}

type BuildResult struct {
	Output    string
	Functions int
}

func Build(ctx context.Context, options BuildOptions) (BuildResult, error) {
	options, err := normalizeOptions(options)
	if err != nil {
		return BuildResult{}, err
	}
	if err := validateTypeScript(ctx, options.Root); err != nil {
		return BuildResult{}, err
	}
	client, err := tsls.StartAPI(options.Root)
	if err != nil {
		return BuildResult{}, err
	}
	defer func() { _ = client.Close() }()
	if _, err := client.Initialize(ctx); err != nil {
		return BuildResult{}, err
	}
	snapshot, err := client.UpdateSnapshot(ctx, options.Config)
	if err != nil {
		return BuildResult{}, err
	}
	defer func() { _ = client.ReleaseSnapshot(context.Background(), snapshot.Snapshot) }()
	project, err := selectProject(snapshot.Projects, options.Input)
	if err != nil {
		return BuildResult{}, err
	}
	semantic, err := frontend.ExtractFile(ctx, client, snapshot.Snapshot, project.ID, options.Input)
	if err != nil {
		return BuildResult{}, err
	}
	hirModule, err := lowering.LowerHIR(semantic, moduleName(options.Input))
	if err != nil {
		return BuildResult{}, err
	}
	if diagnostics := repranalysis.Analyze(&hirModule); len(diagnostics) != 0 {
		return BuildResult{}, fmt.Errorf("native representation analysis failed: %s", formatRepresentationDiagnostics(diagnostics))
	}
	mirModule, err := lowering.LowerMIR(hirModule)
	if err != nil {
		return BuildResult{}, err
	}
	llvmIR, err := llvmcodegen.Emit(mirModule)
	if err != nil {
		return BuildResult{}, err
	}
	tc, err := toolchain.DiscoverClang()
	if err != nil {
		return BuildResult{}, err
	}
	workDir, err := os.MkdirTemp("", "tsnative-build-*")
	if err != nil {
		return BuildResult{}, fmt.Errorf("create build directory: %w", err)
	}
	defer os.RemoveAll(workDir)
	llPath := filepath.Join(workDir, "module.ll")
	moduleObj := filepath.Join(workDir, "module.o")
	runtimeObj := filepath.Join(workDir, "runtime.o")
	if err := os.WriteFile(llPath, []byte(llvmIR), 0o644); err != nil {
		return BuildResult{}, fmt.Errorf("write LLVM IR: %w", err)
	}
	if err := tc.CompileLLVM(ctx, llPath, moduleObj, options.Optimization); err != nil {
		return BuildResult{}, err
	}
	runtimeSource := filepath.Join(options.Root, "runtime", "core", "console.c")
	if err := tc.CompileC(ctx, runtimeSource, runtimeObj, options.Optimization); err != nil {
		return BuildResult{}, err
	}
	if err := toolchain.EnsureParent(options.Output); err != nil {
		return BuildResult{}, fmt.Errorf("create output directory: %w", err)
	}
	if err := tc.Link(ctx, []string{moduleObj, runtimeObj}, options.Output); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{Output: options.Output, Functions: len(mirModule.Functions)}, nil
}

func normalizeOptions(options BuildOptions) (BuildOptions, error) {
	if options.Root == "" {
		options.Root = "."
	}
	root, err := filepath.Abs(options.Root)
	if err != nil {
		return options, fmt.Errorf("resolve project root: %w", err)
	}
	options.Root = root
	if options.Input == "" {
		return options, fmt.Errorf("input TypeScript file is required")
	}
	if !filepath.IsAbs(options.Input) {
		options.Input = filepath.Join(root, options.Input)
	}
	options.Input = filepath.Clean(options.Input)
	if options.Config == "" {
		options.Config = filepath.Join(root, "tsconfig.json")
	} else if !filepath.IsAbs(options.Config) {
		options.Config = filepath.Join(root, options.Config)
	}
	if options.Output == "" {
		base := filepath.Base(options.Input)
		ext := filepath.Ext(base)
		options.Output = filepath.Join(root, base[:len(base)-len(ext)])
	} else if !filepath.IsAbs(options.Output) {
		options.Output = filepath.Join(root, options.Output)
	}
	if options.Optimization == "" {
		options.Optimization = "-O2"
	}
	if !validOptimization(options.Optimization) {
		return options, fmt.Errorf("unsupported optimization %q", options.Optimization)
	}
	return options, nil
}
func validateTypeScript(ctx context.Context, root string) error {
	toolchain, err := tsls.Discover(root)
	if err != nil {
		return err
	}
	return toolchain.Validate(ctx)
}

func selectProject(projects []tsls.APIProject, input string) (tsls.APIProject, error) {
	cleanInput := filepath.Clean(input)
	for _, project := range projects {
		for _, rootFile := range project.RootFiles {
			if filepath.Clean(rootFile) == cleanInput {
				return project, nil
			}
		}
	}
	if len(projects) == 1 {
		return projects[0], nil
	}
	return tsls.APIProject{}, fmt.Errorf("no TypeScript project contains %s", input)
}

func validOptimization(value string) bool {
	switch value {
	case "-O0", "-O1", "-O2", "-O3", "-Oz":
		return true
	default:
		return false
	}
}
func moduleName(input string) string {
	base := filepath.Base(input)
	ext := filepath.Ext(base)
	return base[:len(base)-len(ext)]
}

func formatRepresentationDiagnostics(diagnostics []repranalysis.Diagnostic) string {
	result := ""
	for i, diagnostic := range diagnostics {
		if i != 0 {
			result += "; "
		}
		result += fmt.Sprintf("f%d: %s", diagnostic.Function, diagnostic.Message)
	}
	return result
}
