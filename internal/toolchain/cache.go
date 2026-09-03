package toolchain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type ObjectCache struct {
	Dir   string
	Clang *Clang
}

func NewObjectCache(root string, clang *Clang) (*ObjectCache, error) {
	dir := filepath.Join(root, ".tsnative", "cache", "objects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create object cache: %w", err)
	}
	return &ObjectCache{Dir: dir, Clang: clang}, nil
}

func (c *ObjectCache) CompileLLVM(ctx context.Context, input, opt string) (string, bool, error) {
	return c.CompileLLVMWithOptions(ctx, input, ClangCompileOptions{Optimization: opt})
}

func (c *ObjectCache) CompileLLVMWithOptions(ctx context.Context, input string, opts ClangCompileOptions) (string, bool, error) {
	optKey := opts.Optimization
	if opts.ThinLTO {
		optKey += "+thinlto"
	}
	if opts.PGOProfile != "" {
		optKey += "+pgo:" + opts.PGOProfile
	}
	if opts.PGOGenerate != "" {
		optKey += "+pgogen:" + opts.PGOGenerate
	}
	if opts.Target != "" {
		optKey += "+target:" + opts.Target
	}
	return c.compile(ctx, "llvm", input, optKey, func(ctx context.Context, in, out, _ string) error {
		return c.Clang.CompileLLVMWithOptions(ctx, in, out, opts)
	})
}

// CompileLLVMParallel schedules and compiles multiple LLVM IR modules concurrently with deterministic ordering.
func (c *ObjectCache) CompileLLVMParallel(ctx context.Context, inputs []string, opt string) ([]string, int, int, error) {
	return c.CompileLLVMParallelWithOptions(ctx, inputs, ClangCompileOptions{Optimization: opt})
}

// CompileLLVMParallelWithOptions schedules and compiles multiple LLVM IR modules concurrently with compiler options.
func (c *ObjectCache) CompileLLVMParallelWithOptions(ctx context.Context, inputs []string, opts ClangCompileOptions) ([]string, int, int, error) {
	if len(inputs) == 0 {
		return nil, 0, 0, nil
	}
	type compileResult struct {
		index  int
		output string
		hit    bool
		err    error
	}
	results := make([]string, len(inputs))
	hits := 0
	misses := 0

	concurrency := runtime.GOMAXPROCS(0)
	if concurrency > len(inputs) {
		concurrency = len(inputs)
	}
	if concurrency < 1 {
		concurrency = 1
	}

	workCh := make(chan int, len(inputs))
	resCh := make(chan compileResult, len(inputs))

	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range workCh {
				out, hit, err := c.CompileLLVMWithOptions(ctx, inputs[idx], opts)
				resCh <- compileResult{index: idx, output: out, hit: hit, err: err}
			}
		}()
	}

	for i := range inputs {
		workCh <- i
	}
	close(workCh)

	wg.Wait()
	close(resCh)

	for r := range resCh {
		if r.err != nil {
			return nil, 0, 0, r.err
		}
		results[r.index] = r.output
		if r.hit {
			hits++
		} else {
			misses++
		}
	}

	return results, hits, misses, nil
}

func (c *ObjectCache) CompileC(ctx context.Context, input, opt string) (string, bool, error) {
	return c.compile(ctx, "c", input, opt, c.Clang.CompileC)
}

type compileFunc func(context.Context, string, string, string) error

func (c *ObjectCache) compile(ctx context.Context, kind, input, opt string, compile compileFunc) (string, bool, error) {
	data, err := os.ReadFile(input)
	if err != nil {
		return "", false, fmt.Errorf("read cache input %s: %w", input, err)
	}
	identity, err := clangIdentity(c.Clang.Path)
	if err != nil {
		return "", false, err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(kind))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(opt))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(identity))
	_, _ = h.Write([]byte{0})
	if kind == "c" {
		if err := hashLocalCDependencies(h, input, data, map[string]struct{}{}); err != nil {
			return "", false, err
		}
	}
	_, _ = h.Write(data)
	key := hex.EncodeToString(h.Sum(nil))
	output := filepath.Join(c.Dir, key+".o")
	if info, err := os.Stat(output); err == nil && info.Size() > 0 {
		return output, true, nil
	}
	temp, err := os.CreateTemp(c.Dir, key+"-*.o")
	if err != nil {
		return "", false, fmt.Errorf("create cache temp object: %w", err)
	}
	tempPath := temp.Name()
	_ = temp.Close()
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)
	if err := compile(ctx, input, tempPath, opt); err != nil {
		return "", false, err
	}
	if err := os.Rename(tempPath, output); err != nil {
		if info, statErr := os.Stat(output); statErr != nil || info.Size() == 0 {
			return "", false, fmt.Errorf("publish cached object: %w", err)
		}
	}
	return output, false, nil
}

func hashLocalCDependencies(h interface{ Write([]byte) (int, error) }, path string, data []byte, seen map[string]struct{}) error {
	for _, line := range strings.Split(string(data), "\n") {
		include, ok := localQuotedInclude(line)
		if !ok {
			continue
		}
		dependency := filepath.Clean(filepath.Join(filepath.Dir(path), include))
		if _, exists := seen[dependency]; exists {
			continue
		}
		dependencyData, err := os.ReadFile(dependency)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read C dependency %s: %w", dependency, err)
		}
		seen[dependency] = struct{}{}
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(dependency))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(dependencyData)
		if err := hashLocalCDependencies(h, dependency, dependencyData, seen); err != nil {
			return err
		}
	}
	return nil
}

func localQuotedInclude(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#") {
		return "", false
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
	if !strings.HasPrefix(line, "include") {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "include"))
	if len(rest) < 3 || rest[0] != '"' {
		return "", false
	}
	end := strings.IndexByte(rest[1:], '"')
	if end < 0 {
		return "", false
	}
	return rest[1 : end+1], true
}

func clangIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat clang: %w", err)
	}
	return fmt.Sprintf("%s:%d:%d", path, info.Size(), info.ModTime().UnixNano()), nil
}
