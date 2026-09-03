package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/phongsathornpt/ts-pro/internal/compiler"
	"github.com/phongsathornpt/ts-pro/internal/frontend"
	"github.com/phongsathornpt/ts-pro/internal/tsls"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ts-pro:", err)
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
			return fmt.Errorf("usage: ts-pro check <file.ts>")
		}
		return checkFile(args[1])
	case "build":
		return buildFile(args[1:])
	case "version", "--version", "-version":
		fmt.Println("ts-pro dev")
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
	if options.ReportPerformance {
		printBuildReport(result)
	}
	return nil
}

func parseBuildArgs(args []string) (compiler.BuildOptions, error) {
	options := compiler.BuildOptions{Root: ".", Optimization: "-O2", PureGo: true}
	if len(args) == 0 {
		return options, fmt.Errorf("usage: ts-pro build <file.ts> [-o output] [-O0|-O1|-O2|-O3|-Oz] [-p tsconfig.json] [--pure-go] [--llvm] [--thin-lto] [--pgo=path] [--pgo-gen=path] [--target=triple] [--no-cache] [--clean-cache] [--report-performance]")
	}
	options.Input = args[0]
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-o" || arg == "--output":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("%s requires an output path", arg)
			}
			options.Output = args[i]
		case arg == "-p" || arg == "--project":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("%s requires a tsconfig path", arg)
			}
			options.Config = args[i]
		case arg == "-O0" || arg == "-O1" || arg == "-O2" || arg == "-O3" || arg == "-Oz":
			options.Optimization = arg
		case arg == "--pure-go":
			options.PureGo = true
		case arg == "--llvm":
			options.PureGo = false
			options.DisablePureGo = true
		case arg == "--thin-lto":
			options.ThinLTO = true
		case arg == "--pgo" || arg == "--pgo-profile":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("%s requires a profile path", arg)
			}
			options.PGOProfile = args[i]
		case strings.HasPrefix(arg, "--pgo="):
			options.PGOProfile = strings.TrimPrefix(arg, "--pgo=")
		case strings.HasPrefix(arg, "--pgo-profile="):
			options.PGOProfile = strings.TrimPrefix(arg, "--pgo-profile=")
		case arg == "--pgo-gen" || arg == "--pgo-generate":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("%s requires a profile output path", arg)
			}
			options.PGOGenerate = args[i]
		case strings.HasPrefix(arg, "--pgo-gen="):
			options.PGOGenerate = strings.TrimPrefix(arg, "--pgo-gen=")
		case strings.HasPrefix(arg, "--pgo-generate="):
			options.PGOGenerate = strings.TrimPrefix(arg, "--pgo-generate=")
		case arg == "--target":
			i++
			if i >= len(args) {
				return options, fmt.Errorf("%s requires a target specification", arg)
			}
			options.Target = args[i]
		case strings.HasPrefix(arg, "--target="):
			options.Target = strings.TrimPrefix(arg, "--target=")
		case arg == "--no-cache":
			options.NoCache = true
		case arg == "--clean-cache":
			options.CleanCache = true
		case arg == "--report-performance":
			options.ReportPerformance = true
		default:
			return options, fmt.Errorf("unknown build option %q", arg)
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
	fmt.Println("ts-pro <command>")
	fmt.Println("  doctor        validate Go/TypeScript 7 frontend toolchain")
	fmt.Println("  check <file>  type-check a TypeScript file through TypeScript-LS")
	fmt.Println("  build <file>  compile TypeScript 7 to a native executable (pure-Go default)")
	fmt.Println("                use --llvm for legacy LLVM backend")
	fmt.Println("  version       print compiler version")
}
