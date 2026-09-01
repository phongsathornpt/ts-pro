package toolchain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeTaskLifecycleAndBound(t *testing.T) {
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
	source := filepath.Join(dir, "task_test.c")
	program := `#include <assert.h>
#include <stdatomic.h>
#include <stddef.h>
typedef struct tsnative_task tsnative_task;
typedef void (*tsnative_task_entry)(void *, void *);
tsnative_task *tsnative_task_spawn(tsnative_task_entry, void *);
int tsnative_task_join(tsnative_task *);
void tsnative_task_release(tsnative_task *);
void tsnative_scheduler_shutdown(void);
size_t tsnative_scheduler_peak_active_tasks(void);
size_t tsnative_scheduler_active_tasks(void);
unsigned long long tsnative_scheduler_spawned_tasks(void);
unsigned long long tsnative_scheduler_completed_tasks(void);
static atomic_int gate;
static atomic_int done;
static void job(void *state, void *result_slot) {
  (void)state;
  (void)result_slot;
  while (!atomic_load(&gate)) {}
  atomic_fetch_add(&done, 1);
}
int main(void) {
  tsnative_task *tasks[4];
  for (int i = 0; i < 4; i++) {
    tasks[i] = tsnative_task_spawn(job, 0);
    assert(tasks[i]);
  }
  assert(tsnative_task_spawn(job, 0) == 0);
  atomic_store(&gate, 1);
  for (int i = 0; i < 4; i++) {
    assert(tsnative_task_join(tasks[i]) == 0);
    tsnative_task_release(tasks[i]);
  }
  assert(atomic_load(&done) == 4);
  assert(tsnative_scheduler_peak_active_tasks() == 4);
  for (int i = 0; i < 10000; i++) {
    tsnative_task *task = tsnative_task_spawn(job, 0);
    assert(task);
    assert(tsnative_task_join(task) == 0);
    tsnative_task_release(task);
  }
  assert(atomic_load(&done) == 10004);
  assert(tsnative_scheduler_spawned_tasks() == 10004);
  assert(tsnative_scheduler_completed_tasks() == 10004);
  assert(tsnative_scheduler_active_tasks() == 0);
  tsnative_scheduler_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	testObj := filepath.Join(dir, "test.o")
	schedulerObj := filepath.Join(dir, "scheduler.o")
	taskObj := filepath.Join(dir, "task.o")
	heapObj := filepath.Join(dir, "heap.o")
	binary := filepath.Join(dir, "task_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "scheduler.c"), schedulerObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "task.c"), taskObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "core", "heap.c"), heapObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, schedulerObj, taskObj, heapObj}, binary); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=2", "TSNATIVE_MAX_TASKS=4")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run task test: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != "" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestNativeTaskWorkStealing(t *testing.T) {
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
	source := filepath.Join(dir, "steal_test.c")
	program := `#include <assert.h>
#include <stdatomic.h>
#include <stdint.h>
typedef struct tsnative_task tsnative_task;
typedef void (*tsnative_task_entry)(void *, void *);
tsnative_task *tsnative_task_spawn(tsnative_task_entry, void *);
int tsnative_task_join(tsnative_task *);
void tsnative_task_release(tsnative_task *);
void tsnative_scheduler_shutdown(void);
uint64_t tsnative_scheduler_successful_steals(void);
size_t tsnative_scheduler_worker_count(void);
uint64_t tsnative_scheduler_spawned_tasks(void);
uint64_t tsnative_scheduler_completed_tasks(void);
static atomic_int done;
static void leaf(void *state, void *result_slot) {
  (void)state;
  (void)result_slot;
  atomic_fetch_add(&done, 1);
}
static void root_job(void *state, void *result_slot) {
  (void)state;
  (void)result_slot;
  tsnative_task *children[256];
  for (int i = 0; i < 256; i++) {
    children[i] = tsnative_task_spawn(leaf, 0);
    assert(children[i]);
  }
  for (int i = 0; i < 256; i++) {
    assert(tsnative_task_join(children[i]) == 0);
    tsnative_task_release(children[i]);
  }
}
int main(void) {
  tsnative_task *root = tsnative_task_spawn(root_job, 0);
  assert(root);
  assert(tsnative_task_join(root) == 0);
  tsnative_task_release(root);
  assert(atomic_load(&done) == 256);
  if (tsnative_scheduler_worker_count() > 1) {
    assert(tsnative_scheduler_successful_steals() > 0);
  }
  assert(tsnative_scheduler_spawned_tasks() == 257);
  assert(tsnative_scheduler_completed_tasks() == 257);
  tsnative_scheduler_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	testObj := filepath.Join(dir, "test.o")
	schedulerObj := filepath.Join(dir, "scheduler.o")
	taskObj := filepath.Join(dir, "task.o")
	heapObj := filepath.Join(dir, "heap.o")
	binary := filepath.Join(dir, "steal_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "scheduler.c"), schedulerObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "task.c"), taskObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "core", "heap.c"), heapObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, schedulerObj, taskObj, heapObj}, binary); err != nil {
		t.Fatal(err)
	}
	for _, workers := range []string{"4", "1"} {
		cmd := exec.CommandContext(ctx, binary)
		cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS="+workers, "TSNATIVE_MAX_TASKS=1024")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("run work-stealing test with %s workers: %v: %s", workers, err, output)
		}
		if strings.TrimSpace(string(output)) != "" {
			t.Fatalf("unexpected output with %s workers: %q", workers, output)
		}
	}
}

func TestNativeTaskReferenceResultStaysRootedUntilRelease(t *testing.T) {
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
	source := filepath.Join(dir, "task_ref_test.c")
	program := `#include <assert.h>
#include <stddef.h>
typedef struct tsnative_task tsnative_task;
typedef void (*tsnative_task_entry)(void *, void *);
tsnative_task *tsnative_task_spawn_ref(tsnative_task_entry, void *);
int tsnative_task_join(tsnative_task *);
void *tsnative_task_join_ref_release(tsnative_task *);
void tsnative_scheduler_shutdown(void);
void *tsnative_heap_alloc(size_t);
void tsnative_heap_shutdown(void);
void tsnative_gc_collect(void);
size_t tsnative_heap_live_allocations(void);
static void make_ref(void *state, void *result_slot) {
  (void)state;
  unsigned char *value = tsnative_heap_alloc(16);
  value[0] = 99;
  *(void **)result_slot = value;
}
int main(void) {
  tsnative_task *task = tsnative_task_spawn_ref(make_ref, 0);
  assert(task);
  assert(tsnative_task_join(task) == 0);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 1);
  unsigned char *result = tsnative_task_join_ref_release(task);
  assert(result && result[0] == 99);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 0);
  tsnative_scheduler_shutdown();
  tsnative_heap_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	testObj := filepath.Join(dir, "test.o")
	schedulerObj := filepath.Join(dir, "scheduler.o")
	taskObj := filepath.Join(dir, "task.o")
	heapObj := filepath.Join(dir, "heap.o")
	binary := filepath.Join(dir, "task_ref_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "scheduler.c"), schedulerObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "task.c"), taskObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "core", "heap.c"), heapObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, schedulerObj, taskObj, heapObj}, binary); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=2")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run task ref result test: %v: %s", err, output)
	}
}
