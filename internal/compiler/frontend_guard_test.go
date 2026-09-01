package compiler

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActiveCompilerHasNoParallelTypeScriptFrontend(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"swc", "babel", "oxc"}
	for _, dir := range []string{"cmd", "internal", "runtimego"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if ext := filepath.Ext(path); ext != ".go" && ext != ".c" && ext != ".h" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := strings.ToLower(string(data))
			for _, name := range forbidden {
				if strings.Contains(text, name) {
					t.Fatalf("active compiler source %s references forbidden frontend %s", path, name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
