package toolchain

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Clang struct {
	Path string
}

func DiscoverClang() (*Clang, error) {
	path, err := exec.LookPath("clang")
	if err != nil {
		return nil, fmt.Errorf("clang not found: %w", err)
	}
	return &Clang{Path: path}, nil
}

type ClangCompileOptions struct {
	Optimization string
	ThinLTO      bool
	PGOProfile   string
	PGOGenerate  string
	Target       string
}

type ClangLinkOptions struct {
	ThinLTO     bool
	PGOProfile  string
	PGOGenerate string
	Target      string
}

func TargetTriple(target string) string {
	switch strings.ToLower(target) {
	case "linux/amd64", "linux-amd64", "x86_64-linux", "x86_64-unknown-linux-gnu":
		return "x86_64-unknown-linux-gnu"
	case "linux/arm64", "linux-arm64", "aarch64-linux", "aarch64-unknown-linux-gnu":
		return "aarch64-unknown-linux-gnu"
	case "darwin/amd64", "darwin-amd64", "x86_64-apple-darwin":
		return "x86_64-apple-darwin"
	case "darwin/arm64", "darwin-arm64", "aarch64-apple-darwin":
		return "aarch64-apple-darwin"
	case "windows/amd64", "windows-amd64", "x86_64-windows", "x86_64-w64-mingw32":
		return "x86_64-w64-mingw32"
	default:
		return target
	}
}

func (c *Clang) CompileLLVM(ctx context.Context, input, output string, opt string) error {
	return c.CompileLLVMWithOptions(ctx, input, output, ClangCompileOptions{Optimization: opt})
}

func (c *Clang) CompileLLVMWithOptions(ctx context.Context, input, output string, opts ClangCompileOptions) error {
	args := []string{"-c", input, "-o", output}
	if opts.Optimization != "" {
		args = append(args, opts.Optimization)
	}
	if opts.ThinLTO {
		args = append(args, "-flto=thin")
	}
	if opts.PGOProfile != "" {
		args = append(args, "-fprofile-instr-use="+opts.PGOProfile)
	}
	if opts.PGOGenerate != "" {
		args = append(args, "-fprofile-instr-generate="+opts.PGOGenerate)
	}
	if opts.Target != "" {
		args = append(args, "--target="+TargetTriple(opts.Target))
	}
	return c.run(ctx, args...)
}

func (c *Clang) CompileC(ctx context.Context, input, output string, opt string) error {
	args := []string{"-c", input, "-o", output}
	if opt != "" {
		args = append(args, opt)
	}
	return c.run(ctx, args...)
}

func (c *Clang) Link(ctx context.Context, objects []string, output string) error {
	return c.LinkWithOptions(ctx, objects, output, ClangLinkOptions{})
}

func (c *Clang) LinkWithOptions(ctx context.Context, objects []string, output string, opts ClangLinkOptions) error {
	args := append([]string{}, objects...)
	if opts.ThinLTO {
		args = append(args, "-flto=thin")
	}
	if opts.PGOProfile != "" {
		args = append(args, "-fprofile-instr-use="+opts.PGOProfile)
	}
	if opts.PGOGenerate != "" {
		args = append(args, "-fprofile-instr-generate="+opts.PGOGenerate)
	}
	if opts.Target != "" {
		args = append(args, "--target="+TargetTriple(opts.Target))
	}
	args = append(args, "-pthread", "-o", output)
	return c.run(ctx, args...)
}

func (c *Clang) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, c.Path, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("clang %v failed: %w: %s", args, err, string(output))
	}
	return nil
}

func EnsureParent(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
