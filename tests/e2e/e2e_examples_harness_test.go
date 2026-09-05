package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/ts-pro/pkg/tspro"
)

func TestE2ECompileAllExamplesInProcess(t *testing.T) {
	tmpDir := t.TempDir()
	c := tspro.New(tspro.Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})

	examplesRoot := filepath.Join("..", "..", "examples")
	err := filepath.Walk(examplesRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".ts" {
			return nil
		}
		outBin := filepath.Join(tmpDir, filepath.Base(path)+".bin")
		_, _ = c.CompileFile(path, outBin)
		return nil
	})
	if err != nil {
		t.Fatalf("walk examples failed: %v", err)
	}
}
