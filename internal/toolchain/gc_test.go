package toolchain

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeMarkSweepPreservesExplicitRoots(t *testing.T) {
	clang, err := DiscoverClang()
	if err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "gc.c")
	header := filepath.Join(root, "runtime", "core", "heap.h")
	program := fmt.Sprintf(`#include %q
#include <assert.h>
int main(void) {
  void *root = tsnative_heap_alloc(32);
  void *slots[1] = {root};
  void *frame = tsnative_gc_enter(slots, 1);
  for (int i = 0; i < 32; i++) (void)tsnative_heap_alloc(64);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 1);
  tsnative_gc_leave(frame);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 0);
  assert(tsnative_gc_collections() == 2);
  tsnative_heap_shutdown();
  return 0;
}
`, header)
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	heapSource := filepath.Join(root, "runtime", "core", "heap.c")
	programObj := filepath.Join(dir, "gc.o")
	heapObj := filepath.Join(dir, "heap.o")
	bin := filepath.Join(dir, "gc-test")
	if err := clang.CompileC(ctx, source, programObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, heapSource, heapObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{programObj, heapObj}, bin); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, bin).CombinedOutput(); err != nil {
		t.Fatalf("run GC runtime test: %v: %s", err, output)
	}
}
