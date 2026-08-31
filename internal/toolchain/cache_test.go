package toolchain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestObjectCacheReusesCompiledCObject(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
	clang, err := DiscoverClang()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cache, err := NewObjectCache(root, clang)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "tiny.c")
	if err := os.WriteFile(source, []byte("int answer(void) { return 42; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, hit, err := cache.CompileC(context.Background(), source, "-O2")
	if err != nil || hit {
		t.Fatalf("first compile path=%q hit=%v err=%v", first, hit, err)
	}
	second, hit, err := cache.CompileC(context.Background(), source, "-O2")
	if err != nil || !hit {
		t.Fatalf("second compile path=%q hit=%v err=%v", second, hit, err)
	}
	if first != second {
		t.Fatalf("cache paths differ: %q != %q", first, second)
	}
	if info, err := os.Stat(second); err != nil || info.Size() == 0 {
		t.Fatalf("cached object invalid: info=%v err=%v", info, err)
	}
}
