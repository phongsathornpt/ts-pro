package toolchain

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func (c *Clang) CompileLLVM(ctx context.Context, input, output string, opt string) error {
	args := []string{"-c", input, "-o", output}
	if opt != "" {
		args = append(args, opt)
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
	args := append([]string{}, objects...)
	args = append(args, "-o", output)
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
