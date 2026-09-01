package toolchain

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGoArchiveCacheTracksRuntimeSources(t *testing.T) {
	clang, err := DiscoverClang()
	if err != nil {
		t.Skip("clang not installed")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/runtimecache\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(root, "runtimego")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(pkg, "main.go")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(source, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package main\n/* #include <stdint.h> */\nimport \"C\"\n//export cache_probe\nfunc cache_probe() C.int { return 1 }\nfunc main() {}\n")
	cache, err := NewObjectCache(root, clang)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, hit, err := cache.BuildGoArchive(ctx, root, "./runtimego")
	if err != nil || hit {
		t.Fatalf("first build hit=%v err=%v", hit, err)
	}
	second, hit, err := cache.BuildGoArchive(ctx, root, "./runtimego")
	if err != nil || !hit || second != first {
		t.Fatalf("second build path=%q hit=%v err=%v", second, hit, err)
	}
	write("package main\n/* #include <stdint.h> */\nimport \"C\"\n//export cache_probe\nfunc cache_probe() C.int { return 2 }\nfunc main() {}\n")
	third, hit, err := cache.BuildGoArchive(ctx, root, "./runtimego")
	if err != nil || hit || third == first {
		t.Fatalf("changed build path=%q hit=%v err=%v", third, hit, err)
	}
}
