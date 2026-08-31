package main

import (
	"context"
	"fmt"
	"os"
	"time"

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
	client, err := tsls.Start(".")
	if err != nil {
		return err
	}
	if err := client.Initialize(ctx); err != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_ = client.Close(cleanupCtx)
		return err
	}
	if err := client.Close(ctx); err != nil {
		return err
	}
	fmt.Printf("Go compiler frontend: ready\nTypeScript: %s\nTypeScript-LS: ready\nLSP command: %s --lsp --stdio\n", version, toolchain.TSCPath)
	return nil
}

func usage() {
	fmt.Println("tsnative <command>")
	fmt.Println("  doctor   validate Go/TypeScript 7 frontend toolchain")
	fmt.Println("  version  print compiler version")
}
