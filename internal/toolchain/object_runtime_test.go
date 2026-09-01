package toolchain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGoRuntimeObjectAllocationABI(t *testing.T) {
	clang, err := DiscoverClang()
	if err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "object.c")
	program := `#include <assert.h>
#include <stddef.h>
extern void *tsnative_object_alloc(size_t size);
extern size_t tsnative_heap_live_allocations(void);
extern void tsnative_gc_collect(void);
extern void *tsnative_gc_enter(void **slots, size_t count);
extern void tsnative_gc_leave(void *frame);
extern void tsnative_heap_shutdown(void);
int main(void) {
  void *object = tsnative_object_alloc(24);
  assert(object);
  assert(tsnative_heap_live_allocations() == 1);
  void *slots[1] = {object};
  void *frame = tsnative_gc_enter(slots, 1);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 1);
  tsnative_gc_leave(frame);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 0);
  tsnative_heap_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	obj := filepath.Join(dir, "object.o")
	bin := filepath.Join(dir, "object-test")
	if err := clang.CompileC(ctx, source, obj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{obj, goRuntime}, bin); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, bin).CombinedOutput(); err != nil {
		t.Fatalf("run Go object runtime test: %v: %s", err, output)
	}
}
