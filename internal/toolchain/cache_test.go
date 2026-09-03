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

func TestObjectCacheInvalidatesWhenLocalHeaderChanges(t *testing.T) {
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
	header := filepath.Join(root, "value.h")
	source := filepath.Join(root, "value.c")
	if err := os.WriteFile(header, []byte("#define VALUE 41\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("#include \"value.h\"\nint value(void) { return VALUE; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, hit, err := cache.CompileC(context.Background(), source, "-O2")
	if err != nil || hit {
		t.Fatalf("first compile path=%q hit=%v err=%v", first, hit, err)
	}
	if err := os.WriteFile(header, []byte("#define VALUE 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, hit, err := cache.CompileC(context.Background(), source, "-O2")
	if err != nil || hit {
		t.Fatalf("header-change compile path=%q hit=%v err=%v", second, hit, err)
	}
	if first == second {
		t.Fatalf("header change reused stale object %q", first)
	}
}

func TestObjectCacheCompileLLVMParallel(t *testing.T) {
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
	ctx := context.Background()

	m1 := filepath.Join(root, "m1.ll")
	m2 := filepath.Join(root, "m2.ll")
	m3 := filepath.Join(root, "m3.ll")

	ll1 := `define i32 @fn1() { ret i32 1 }`
	ll2 := `define i32 @fn2() { ret i32 2 }`
	ll3 := `define i32 @fn3() { ret i32 3 }`

	_ = os.WriteFile(m1, []byte(ll1), 0o644)
	_ = os.WriteFile(m2, []byte(ll2), 0o644)
	_ = os.WriteFile(m3, []byte(ll3), 0o644)

	objs, hits, misses, err := cache.CompileLLVMParallel(ctx, []string{m1, m2, m3}, "-O2")
	if err != nil {
		t.Fatalf("parallel compilation failed: %v", err)
	}
	if len(objs) != 3 || misses != 3 || hits != 0 {
		t.Fatalf("expected 3 misses, got len=%d hits=%d misses=%d", len(objs), hits, misses)
	}

	// Recompile should hit cache for all 3
	objs2, hits2, misses2, err := cache.CompileLLVMParallel(ctx, []string{m1, m2, m3}, "-O2")
	if err != nil {
		t.Fatalf("parallel recompilation failed: %v", err)
	}
	if len(objs2) != 3 || hits2 != 3 || misses2 != 0 {
		t.Fatalf("expected 3 hits, got len=%d hits=%d misses=%d", len(objs2), hits2, misses2)
	}
	for i := range objs {
		if objs[i] != objs2[i] {
			t.Fatalf("mismatch at index %d: %s != %s", i, objs[i], objs2[i])
		}
	}
}

func TestObjectCacheThinLTOAndTargetOptions(t *testing.T) {
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
	ctx := context.Background()
	ll := filepath.Join(root, "mod.ll")
	_ = os.WriteFile(ll, []byte(`define i32 @answer() { ret i32 42 }`), 0o644)

	obj1, hit1, err := cache.CompileLLVMWithOptions(ctx, ll, ClangCompileOptions{Optimization: "-O2"})
	if err != nil || hit1 {
		t.Fatalf("normal compile failed: hit=%v err=%v", hit1, err)
	}

	objThin, hitThin, err := cache.CompileLLVMWithOptions(ctx, ll, ClangCompileOptions{Optimization: "-O2", ThinLTO: true})
	if err != nil || hitThin {
		t.Fatalf("ThinLTO compile failed: hit=%v err=%v", hitThin, err)
	}
	if objThin == obj1 {
		t.Fatalf("ThinLTO cache key collided with normal cache key")
	}

	_, hitThin2, err := cache.CompileLLVMWithOptions(ctx, ll, ClangCompileOptions{Optimization: "-O2", ThinLTO: true})
	if err != nil || !hitThin2 {
		t.Fatalf("ThinLTO second compile expected hit, got hit=%v err=%v", hitThin2, err)
	}
}

