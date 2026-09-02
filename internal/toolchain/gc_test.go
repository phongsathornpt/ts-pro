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
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	header := goRuntimeHeaderForTest(t, goRuntime)
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
  assert(tsnative_gc_major_collections() == 2);
  assert(tsnative_gc_minor_collections() == 0);
  assert(tsnative_gc_nursery_bytes() == 0);
  void *young = tsnative_heap_alloc(65536);
  assert(young);
  assert(tsnative_gc_nursery_bytes() >= 65536);
  assert(tsnative_gc_nursery_blocks() == 1);
  tsnative_gc_safepoint();
  assert(tsnative_heap_live_allocations() == 0);
  assert(tsnative_gc_collections() == 3);
  assert(tsnative_gc_major_collections() == 2);
  assert(tsnative_gc_minor_collections() == 1);
  assert(tsnative_gc_nursery_bytes() == 0);
  assert(tsnative_gc_nursery_blocks() == 0);
  assert(tsnative_gc_minor_nursery_scans() == 1);
  assert(tsnative_heap_lock_acquisitions() > 0);
  assert(tsnative_root_lock_acquisitions() > 0);
  assert(tsnative_world_read_lock_acquisitions() > 0);
  assert(tsnative_world_write_lock_acquisitions() >= 2);
  assert(tsnative_block_read_lock_acquisitions() > 0);
  assert(tsnative_block_write_lock_acquisitions() > 0);
  assert(tsnative_gc_trace_words() > 0);
  assert(tsnative_gc_trace_conservative_blocks() > 0);

  void *parent = tsnative_heap_alloc(32);
  void *parent_slots[1] = {parent};
  void *parent_frame = tsnative_gc_enter(parent_slots, 1);
  tsnative_gc_collect();
  void *child = tsnative_heap_alloc(65536);
  tsnative_gc_store_ref(parent, parent, child);
  assert(tsnative_gc_remembered_parents() == 1);
  assert(tsnative_gc_barrier_stores() >= 1);
  assert(tsnative_gc_remembered_records() >= 1);
  tsnative_gc_safepoint();
  assert(tsnative_heap_live_allocations() == 2);
  assert(tsnative_gc_remembered_parents() == 0);
  assert(tsnative_gc_promoted_bytes() >= 65536);
  assert(tsnative_gc_old_bytes() >= 65568);
  tsnative_gc_leave(parent_frame);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 0);
  assert(tsnative_gc_old_bytes() == 0);

  void *atomic_child = tsnative_heap_alloc_atomic(16);
  void *atomic_parent = tsnative_heap_alloc_atomic(sizeof(void *));
  ((void **)atomic_parent)[0] = atomic_child;
  void *atomic_slots[1] = {atomic_parent};
  void *atomic_frame = tsnative_gc_enter(atomic_slots, 1);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 1);
  tsnative_gc_leave(atomic_frame);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 0);

  size_t ref_offsets[1] = {sizeof(void *)};
  void *precise_child = tsnative_heap_alloc_atomic(16);
  void *precise_parent = tsnative_heap_alloc_refs(2 * sizeof(void *), ref_offsets, 1);
  ((void **)precise_parent)[0] = (void *)0xdeadbeef;
  ((void **)precise_parent)[1] = precise_child;
  void *precise_slots[1] = {precise_parent};
  void *precise_frame = tsnative_gc_enter(precise_slots, 1);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 2);
  assert(tsnative_gc_trace_atomic_blocks() > 0);
  assert(tsnative_gc_trace_precise_blocks() > 0);
  tsnative_gc_leave(precise_frame);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 0);

  tsnative_heap_shutdown();
  return 0;
}
`, header)
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	programObj := filepath.Join(dir, "gc.o")
	bin := filepath.Join(dir, "gc-test")
	if err := clang.CompileC(ctx, source, programObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{programObj, goRuntime}, bin); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, bin).CombinedOutput(); err != nil {
		t.Fatalf("run GC runtime test: %v: %s", err, output)
	}
}
func TestRuntimeHeapSupportsConcurrentRootFrames(t *testing.T) {
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
	source := filepath.Join(dir, "gc_threads.c")
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	header := goRuntimeHeaderForTest(t, goRuntime)
	program := fmt.Sprintf(`#include %q
#include <assert.h>
#include <pthread.h>
#include <stdatomic.h>
static atomic_int ready;
static void *worker(void *unused) {
  (void)unused;
  void *root = tsnative_heap_alloc(32);
  void *slots[1] = {root};
  void *frame = tsnative_gc_enter(slots, 1);
  atomic_fetch_add(&ready, 1);
  while (atomic_load(&ready) != 4) {}
  for (int i = 0; i < 2000; i++) {
    (void)tsnative_heap_alloc(24);
    if ((i & 31) == 0) tsnative_gc_safepoint();
  }
  ((unsigned char *)root)[0] = 7;
  assert(((unsigned char *)root)[0] == 7);
  tsnative_gc_leave(frame);
  return 0;
}
int main(void) {
  pthread_t threads[4];
  for (int i = 0; i < 4; i++) assert(pthread_create(&threads[i], 0, worker, 0) == 0);
  for (int i = 0; i < 4; i++) assert(pthread_join(threads[i], 0) == 0);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 0);
  void *persistent = tsnative_heap_alloc(16);
  void *token = tsnative_gc_root_register(&persistent);
  assert(token);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 1);
  tsnative_gc_root_unregister(token);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 0);
  tsnative_heap_shutdown();
  return 0;
}
`, header)
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}

	programObj := filepath.Join(dir, "gc_threads.o")
	bin := filepath.Join(dir, "gc-threads-test")
	if err := clang.CompileC(ctx, source, programObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{programObj, goRuntime}, bin); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, bin).CombinedOutput(); err != nil {
		t.Fatalf("run concurrent GC runtime test: %v: %s", err, output)
	}
}
func TestRuntimeGCHandoffDefersCollection(t *testing.T) {
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
	source := filepath.Join(dir, "gc_handoff.c")
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	header := goRuntimeHeaderForTest(t, goRuntime)
	program := fmt.Sprintf(`#include %q
#include <assert.h>
int main(void) {
  (void)tsnative_heap_alloc(32);
  assert(tsnative_heap_live_allocations() == 1);
  tsnative_gc_handoff_begin();
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 1);
  tsnative_gc_handoff_end();
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 0);
  tsnative_heap_shutdown();
  return 0;
}
`, header)
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	programObj := filepath.Join(dir, "handoff.o")

	binary := filepath.Join(dir, "gc-handoff-test")
	if err := clang.CompileC(ctx, source, programObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{programObj, goRuntime}, binary); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("run GC handoff test: %v: %s", err, output)
	}
}
