package tsls

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const SupportedVersion = "7.0.2"

type Toolchain struct {
	Root    string
	TSCPath string
}

func Discover(root string) (*Toolchain, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}
	path := filepath.Join(abs, "node_modules", ".bin", "tsc")
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return nil, fmt.Errorf("TypeScript 7 CLI not found at %s", path)
	}
	return &Toolchain{Root: abs, TSCPath: path}, nil
}
func (t *Toolchain) Version(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, t.TSCPath, "--version")
	cmd.Dir = t.Root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run TypeScript --version: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimPrefix(strings.TrimSpace(string(output)), "Version "), nil
}

func (t *Toolchain) Validate(ctx context.Context) error {
	version, err := t.Version(ctx)
	if err != nil {
		return err
	}
	if version != SupportedVersion {
		return fmt.Errorf("unsupported TypeScript version %s; expected %s", version, SupportedVersion)
	}
	return nil
}

func (t *Toolchain) LSPCommand(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(ctx, t.TSCPath, "--lsp", "--stdio")
	cmd.Dir = t.Root
	return cmd
}
