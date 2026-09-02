package compiler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	escapeanalysis "github.com/phongsathornpt/ts-pro/internal/analysis/escape"
	rangeanalysis "github.com/phongsathornpt/ts-pro/internal/analysis/range"
	repranalysis "github.com/phongsathornpt/ts-pro/internal/analysis/repr"
	golangcodegen "github.com/phongsathornpt/ts-pro/internal/codegen/golang"
	llvmcodegen "github.com/phongsathornpt/ts-pro/internal/codegen/llvm"
	"github.com/phongsathornpt/ts-pro/internal/frontend"
	"github.com/phongsathornpt/ts-pro/internal/lowering"
	"github.com/phongsathornpt/ts-pro/internal/mir"
	"github.com/phongsathornpt/ts-pro/internal/toolchain"
	"github.com/phongsathornpt/ts-pro/internal/tsls"
)

type BuildOptions struct {
	Root              string
	Input             string
	Output            string
	Config            string
	Optimization      string
	PureGo            bool
	DisablePureGo     bool
	ReportPerformance bool
}

type BuildResult struct {
	Output    string
	Functions int
	Metrics   BuildMetrics
	Timings   BuildTimings
}

func Build(ctx context.Context, options BuildOptions) (BuildResult, error) {
	totalStart := time.Now()
	var timings BuildTimings
	options, err := normalizeOptions(options)
	if err != nil {
		return BuildResult{}, err
	}
	tsStart := time.Now()
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
	if err := validateDiagnostics(ctx, client, snapshot.Snapshot, project.ID, options.Input); err != nil {
		return BuildResult{}, err
	}
	semantic, err := frontend.ExtractFile(ctx, client, snapshot.Snapshot, project.ID, options.Input)
	if err != nil {
		return BuildResult{}, err
	}
	timings.TypeScript = time.Since(tsStart)
	hirStart := time.Now()
	hirModule, err := lowering.LowerHIR(semantic, moduleName(options.Input))
	if err != nil {
		return BuildResult{}, err
	}
	timings.HIR = time.Since(hirStart)
	reprStart := time.Now()
	if diagnostics := repranalysis.Analyze(&hirModule); len(diagnostics) != 0 {
		return BuildResult{}, fmt.Errorf("native representation analysis failed: %s", formatRepresentationDiagnostics(diagnostics))
	}
	timings.Repr = time.Since(reprStart)
	mirStart := time.Now()
	ranges := rangeanalysis.Analyze(hirModule)
	mirModule, err := lowering.LowerMIRWithRanges(hirModule, ranges)
	if err != nil {
		return BuildResult{}, err
	}
	timings.MIR = time.Since(mirStart)
	escapeStart := time.Now()
	escapes := escapeanalysis.Analyze(mirModule)
	timings.Escape = time.Since(escapeStart)
	metrics := collectBuildMetrics(hirModule, mirModule, escapes)
	if options.PureGo {
		return buildPureGo(ctx, options, mirModule, metrics, timings, totalStart)
	}
	llvmStart := time.Now()
	llvmIR, err := llvmcodegen.EmitWithEscapeAnalysis(mirModule, escapes)
	if err != nil {
		return BuildResult{}, err
	}
	timings.LLVM = time.Since(llvmStart)
	tc, err := toolchain.DiscoverClang()
	if err != nil {
		return BuildResult{}, err
	}
	cache, err := toolchain.NewObjectCache(options.Root, tc)
	if err != nil {
		return BuildResult{}, err
	}
	workDir, err := os.MkdirTemp("", "tsnative-build-*")
	if err != nil {
		return BuildResult{}, fmt.Errorf("create build directory: %w", err)
	}
	defer os.RemoveAll(workDir)
	llPath := filepath.Join(workDir, "module.ll")
	if err := os.WriteFile(llPath, []byte(llvmIR), 0o644); err != nil {
		return BuildResult{}, fmt.Errorf("write LLVM IR: %w", err)
	}
	codegenStart := time.Now()
	moduleObj, hit, err := cache.CompileLLVM(ctx, llPath, options.Optimization)
	if err != nil {
		return BuildResult{}, err
	}
	recordCacheResult(&metrics, hit)
	timings.Codegen = time.Since(codegenStart)
	runtimeStart := time.Now()
	runtimeObjects, hits, misses, err := compileRuntimeObjects(ctx, cache, options.Root, options.Optimization)
	if err != nil {
		return BuildResult{}, err
	}
	metrics.CacheHits += hits
	metrics.CacheMisses += misses
	timings.Runtime = time.Since(runtimeStart)
	objects := append([]string{moduleObj}, runtimeObjects...)
	if err := toolchain.EnsureParent(options.Output); err != nil {
		return BuildResult{}, fmt.Errorf("create output directory: %w", err)
	}
	linkStart := time.Now()
	if err := tc.Link(ctx, objects, options.Output); err != nil {
		return BuildResult{}, err
	}
	timings.Link = time.Since(linkStart)
	timings.Total = time.Since(totalStart)
	return BuildResult{Output: options.Output, Functions: len(mirModule.Functions), Metrics: metrics, Timings: timings}, nil
}

func buildPureGo(ctx context.Context, options BuildOptions, module mir.Module, metrics BuildMetrics, timings BuildTimings, totalStart time.Time) (BuildResult, error) {
	goStart := time.Now()
	source, err := golangcodegen.Emit(module)
	if err != nil {
		return BuildResult{}, err
	}
	timings.Go = time.Since(goStart)

	goPath, err := exec.LookPath("go")
	if err != nil {
		return BuildResult{}, fmt.Errorf("go toolchain not found: %w", err)
	}
	workDir, err := os.MkdirTemp("", "tsnative-pure-go-*")
	if err != nil {
		return BuildResult{}, fmt.Errorf("create pure-Go build directory: %w", err)
	}
	defer os.RemoveAll(workDir)
	if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte("module tsnative.generated\n\ngo 1.27\n"), 0o644); err != nil {
		return BuildResult{}, fmt.Errorf("write generated Go module: %w", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "main.go"), []byte(source), 0o644); err != nil {
		return BuildResult{}, fmt.Errorf("write generated Go source: %w", err)
	}
	if err := toolchain.EnsureParent(options.Output); err != nil {
		return BuildResult{}, fmt.Errorf("create output directory: %w", err)
	}
	linkStart := time.Now()
	command := exec.CommandContext(ctx, goPath, "build", "-trimpath", "-buildvcs=false", "-o", options.Output, ".")
	command.Dir = workDir
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, runErr := command.CombinedOutput(); runErr != nil {
		return BuildResult{}, fmt.Errorf("build generated pure-Go program: %w: %s", runErr, output)
	}
	timings.Link = time.Since(linkStart)
	timings.Total = time.Since(totalStart)
	return BuildResult{Output: options.Output, Functions: len(module.Functions), Metrics: metrics, Timings: timings}, nil
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
	if _, err := os.Stat(options.Input); err != nil {
		examplesDir := filepath.Join(root, "examples")
		if strings.HasPrefix(options.Input, examplesDir) {
			baseName := filepath.Base(options.Input)
			for _, sub := range []string{"basics", "arrays", "objects", "dynamic", "concurrency", "memory"} {
				candidate := filepath.Join(examplesDir, sub, baseName)
				if _, err2 := os.Stat(candidate); err2 == nil {
					options.Input = candidate
					break
				}
			}
		}
	}
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
	if options.DisablePureGo {
		options.PureGo = false
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

func validateDiagnostics(ctx context.Context, client *tsls.APIClient, snapshot uint64, project, file string) error {
	var diagnostics []tsls.APIDiagnostic
	collect := func(items []tsls.APIDiagnostic, err error) error {
		if err != nil {
			return err
		}
		diagnostics = append(diagnostics, items...)
		return nil
	}
	if err := collect(client.GetConfigDiagnostics(ctx, snapshot, project)); err != nil {
		return err
	}
	if err := collect(client.GetProgramDiagnostics(ctx, snapshot, project)); err != nil {
		return err
	}
	if err := collect(client.GetGlobalDiagnostics(ctx, snapshot, project)); err != nil {
		return err
	}
	if err := collect(client.GetSyntacticDiagnostics(ctx, snapshot, project, file)); err != nil {
		return err
	}
	if err := collect(client.GetSemanticDiagnostics(ctx, snapshot, project, file)); err != nil {
		return err
	}
	var errors []tsls.APIDiagnostic
	for _, diagnostic := range diagnostics {
		if diagnostic.IsError() {
			errors = append(errors, diagnostic)
		}
	}
	if len(errors) == 0 {
		return nil
	}
	message := "TypeScript 7 check failed"
	for _, diagnostic := range errors {
		message += "\n" + renderAPIDiagnostic(diagnostic)
	}
	return fmt.Errorf("%s", message)
}

func renderAPIDiagnostic(diagnostic tsls.APIDiagnostic) string {
	fileName := diagnostic.FileName
	if fileName == "" {
		fileName = "<project>"
		return fmt.Sprintf("%s: error TS%d: %s", fileName, diagnostic.Code, diagnostic.Text)
	}
	data, err := os.ReadFile(fileName)
	if err != nil || diagnostic.Pos < 0 || diagnostic.Pos > len(data) {
		return fmt.Sprintf("%s:%d: error TS%d: %s", fileName, diagnostic.Pos, diagnostic.Code, diagnostic.Text)
	}
	prefix := string(data[:diagnostic.Pos])
	line := strings.Count(prefix, "\n") + 1
	lastNewline := strings.LastIndex(prefix, "\n")
	column := diagnostic.Pos + 1
	if lastNewline >= 0 {
		column = diagnostic.Pos - lastNewline
	}
	return fmt.Sprintf("%s:%d:%d: error TS%d: %s", fileName, line, column, diagnostic.Code, diagnostic.Text)
}

func recordCacheResult(metrics *BuildMetrics, hit bool) {
	if hit {
		metrics.CacheHits++
	} else {
		metrics.CacheMisses++
	}
}

func compileRuntimeObjects(ctx context.Context, cache *toolchain.ObjectCache, root, opt string) ([]string, int, int, error) {
	archive, hit, err := cache.BuildGoArchive(ctx, root, "./runtime")
	if err != nil {
		return nil, 0, 0, err
	}
	if hit {
		return []string{archive}, 1, 0, nil
	}
	return []string{archive}, 0, 1, nil
}
