package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/projectthorn/tsv7-bin/internal/frontend"
	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tsnative:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	switch args[0] {
	case "doctor":
		return doctor()
	case "check":
		if len(args) != 2 {
			return fmt.Errorf("usage: tsnative check <file.ts>")
		}
		return checkFile(args[1])
	case "version", "--version", "-version":
		fmt.Println("tsnative dev")
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func doctor() error {
	toolchain, err := tsls.Discover(".")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := toolchain.Validate(ctx); err != nil {
		return err
	}
	version, err := toolchain.Version(ctx)
	if err != nil {
		return err
	}
	client, err := startInitializedClient(ctx)
	if err != nil {
		return err
	}
	if err := closeClient(client); err != nil {
		return err
	}
	fmt.Printf("Go compiler frontend: ready\nTypeScript: %s\nTypeScript-LS: ready\nLSP command: %s --lsp --stdio\n", version, toolchain.TSCPath)
	return nil
}
func checkFile(path string) error {
	text, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := startInitializedClient(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closeClient(client) }()

	diagnostics, err := frontend.CheckSource(ctx, client, path, string(text))
	if err != nil {
		return err
	}
	for _, diagnostic := range diagnostics {
		fmt.Fprintln(os.Stderr, diagnostic.Render(path))
	}
	if frontend.HasErrors(diagnostics) {
		return fmt.Errorf("TypeScript check failed with %d diagnostic(s)", len(diagnostics))
	}
	fmt.Printf("TypeScript check passed: %s\n", path)
	return nil
}

func startInitializedClient(ctx context.Context) (*tsls.Client, error) {
	client, err := tsls.Start(".")
	if err != nil {
		return nil, err
	}
	if err := client.Initialize(ctx); err != nil {
		_ = closeClient(client)
		return nil, err
	}
	return client, nil
}
func closeClient(client *tsls.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return client.Close(ctx)
}

func usage() {
	fmt.Println("tsnative <command>")
	fmt.Println("  doctor        validate Go/TypeScript 7 frontend toolchain")
	fmt.Println("  check <file>  type-check a TypeScript file through TypeScript-LS")
	fmt.Println("  version       print compiler version")
}
