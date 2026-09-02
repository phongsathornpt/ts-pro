package compiler

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitecturePureGoEnforcement(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{"cmd", "internal", "runtime"} {
		scanDir := filepath.Join(root, dir)
		err := filepath.WalkDir(scanDir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}

			ext := filepath.Ext(path)

			// Zero assembly (.s) files allowed anywhere in active code.
			if ext == ".s" {
				t.Errorf("assembly file forbidden in pure-Go codebase: %s", path)
				return nil
			}

			// In runtime, handwritten C or header files are completely banned.
			if dir == "runtime" && (ext == ".c" || ext == ".h") {
				t.Errorf("C source/header forbidden in pure-Go runtime: %s", path)
				return nil
			}

			if ext != ".go" {
				return nil
			}

			// Skip test files that specifically assert rejection of legacy C shims.
			if strings.HasSuffix(path, "purego_guard_test.go") ||
				strings.HasSuffix(path, "goarchive_test.go") ||
				strings.HasSuffix(path, "emitter_test.go") {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(data)

			if strings.Contains(content, `import "C"`) {
				t.Errorf("forbidden cgo reference (import \"C\") in %s", path)
			}
			if strings.Contains(content, "//export ") {
				t.Errorf("forbidden cgo export annotation (//export) in %s", path)
			}

			if dir == "runtime" && strings.HasPrefix(content, "package main") {
				t.Errorf("runtime files must declare package runtime, found package main in %s", path)
			}

			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
