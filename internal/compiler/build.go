package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	repranalysis "github.com/projectthorn/tsv7-bin/internal/analysis/repr"
	llvmcodegen "github.com/projectthorn/tsv7-bin/internal/codegen/llvm"
	"github.com/projectthorn/tsv7-bin/internal/frontend"
	"github.com/projectthorn/tsv7-bin/internal/lowering"
	"github.com/projectthorn/tsv7-bin/internal/toolchain"
	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

type BuildOptions struct {
	Root              string
	Input             string
	Output            string
	Config            string
	Optimization      string
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
	mirModule, err := lowering.LowerMIR(hirModule)
	if err != nil {
		return BuildResult{}, err
	}
	timings.MIR = time.Since(mirStart)
	metrics := collectBuildMetrics(hirModule, mirModule)
	llvmStart := time.Now()
	llvmIR, err := llvmcodegen.Emit(mirModule)
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

type runtimeCompileResult struct {
	index int
	path  string
	hit   bool
	err   error
}

func compileRuntimeObjects(ctx context.Context, cache *toolchain.ObjectCache, root, opt string) ([]string, int, int, error) {
	sources := []string{"console.c", "array_f64.c", "string.c", "object.c"}
	results := make(chan runtimeCompileResult, len(sources))
	for i, source := range sources {
		go func(index int, name string) {
			path, hit, err := cache.CompileC(ctx, filepath.Join(root, "runtime", "core", name), opt)
			results <- runtimeCompileResult{index: index, path: path, hit: hit, err: err}
		}(i, source)
	}
	objects := make([]string, len(sources))
	hits, misses := 0, 0
	for range sources {
		result := <-results
		if result.err != nil {
			return nil, hits, misses, result.err
		}
		objects[result.index] = result.path
		if result.hit {
			hits++
		} else {
			misses++
		}
	}
	return objects, hits, misses, nil
}
