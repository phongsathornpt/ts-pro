package toolchain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGoRuntimeF64ArrayABI(t *testing.T) {
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
	source := filepath.Join(dir, "array.c")
	program := `#include <assert.h>
#include <math.h>
#include <stdint.h>
#include <stddef.h>
extern void *tsnative_array_f64_new(uint64_t len);
extern void tsnative_array_f64_set(void *raw, uint64_t index, double value);
extern double tsnative_array_f64_len(void *raw);
extern double tsnative_array_f64_get(void *raw, double index);
extern void tsnative_array_f64_set_checked(void *raw, double index, double value);
extern void *tsnative_gc_enter(void **slots, size_t count);
extern void tsnative_gc_leave(void *frame);
extern void tsnative_gc_collect(void);
extern size_t tsnative_heap_live_allocations(void);
extern void tsnative_heap_shutdown(void);
`
	program += `int main(void) {
  void *array = tsnative_array_f64_new(3);
  assert(array);
  assert(tsnative_array_f64_len(array) == 3.0);
  tsnative_array_f64_set(array, 0, 1.5);
  tsnative_array_f64_set_checked(array, 1.0, 42.0);
  tsnative_array_f64_set(array, 2, -7.25);
  assert(tsnative_array_f64_get(array, 0.0) == 1.5);
  assert(tsnative_array_f64_get(array, 1.0) == 42.0);
  assert(tsnative_array_f64_get(array, 2.0) == -7.25);
  assert(isnan(tsnative_array_f64_get(array, -1.0)));
  assert(isnan(tsnative_array_f64_get(array, 0.5)));
  assert(isnan(tsnative_array_f64_get(array, 3.0)));
  assert(isnan(tsnative_array_f64_get(array, INFINITY)));
  void *slots[1] = {array};
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
	programObj := filepath.Join(dir, "array.o")
	binary := filepath.Join(dir, "array-test")
	if err := clang.CompileC(ctx, source, programObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{programObj, goRuntime}, binary); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("run Go F64 array runtime test: %v: %s", err, output)
	}
}
