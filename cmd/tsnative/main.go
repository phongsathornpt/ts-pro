package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/projectthorn/tsv7-bin/internal/compiler"
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
	case "build":
		return buildFile(args[1:])
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

func buildFile(args []string) error {
	options, err := parseBuildArgs(args)
	if err != nil {
		return err
	}
	result, err := compiler.Build(context.Background(), options)
	if err != nil {
		return err
	}
	fmt.Printf("Built %s (%d native functions)\n", result.Output, result.Functions)
	return nil
}

func parseBuildArgs(args []string) (compiler.BuildOptions, error) {
	options := compiler.BuildOptions{Root: ".", Optimization: "-O2"}
	if len(args) == 0 {
		return options, fmt.Errorf("usage: tsnative build <file.ts> [-o output] [-O0|-O1|-O2|-O3|-Oz] [-p tsconfig.json]")
	}
	options.Input = args[0]
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-o", "--output":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("%s requires an output path", args[i-1])
			}
			options.Output = args[i]
		case "-p", "--project":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("%s requires a tsconfig path", args[i-1])
			}
			options.Config = args[i]
		case "-O0", "-O1", "-O2", "-O3", "-Oz":
			options.Optimization = args[i]
		default:
			return options, fmt.Errorf("unknown build option %q", args[i])
		}
	}
	return options, nil
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
	fmt.Println("  build <file>  compile TypeScript 7 to a native executable")
	fmt.Println("  version       print compiler version")
}
