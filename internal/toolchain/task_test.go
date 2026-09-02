package toolchain

import (
	"context"
	"fmt"
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
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	binary := filepath.Join(dir, "task_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, goRuntime}, binary); err != nil {
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
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	binary := filepath.Join(dir, "steal_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, goRuntime}, binary); err != nil {
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
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	binary := filepath.Join(dir, "task_ref_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, goRuntime}, binary); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=2")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run task ref result test: %v: %s", err, output)
	}
}

func TestNativeTaskParkWakeResume(t *testing.T) {
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
	source := filepath.Join(dir, "park_test.c")
	program := `#include <assert.h>
#include <sched.h>
#include <stdatomic.h>
typedef struct tsnative_task tsnative_task;
typedef void (*tsnative_task_entry)(void *, void *);
typedef enum { RUNNABLE=1, RUNNING, WAITING, DONE, CANCELLED, FAILED } task_status;
tsnative_task *tsnative_task_spawn(tsnative_task_entry, void *);
int tsnative_task_join(tsnative_task *);
void tsnative_task_release(tsnative_task *);
task_status tsnative_task_get_status(tsnative_task *);
void tsnative_scheduler_shutdown(void);
int tsnative_scheduler_prepare_park(void);
tsnative_task *tsnative_scheduler_current_task(void);
int tsnative_scheduler_wake(tsnative_task *);
static atomic_int phase;
static atomic_int fast_phase;
static void parked(void *state, void *result) {
  (void)state; (void)result;
  int value = atomic_load(&phase);
  if (value == 0) {
    atomic_store(&phase, 1);
    assert(tsnative_scheduler_prepare_park() == 0);
    return;
  }
  assert(value == 1);
  atomic_store(&phase, 2);
}
static void fast_wake(void *state, void *result) {
  (void)state; (void)result;
  int value = atomic_load(&fast_phase);
  if (value == 0) {
    atomic_store(&fast_phase, 1);
    assert(tsnative_scheduler_prepare_park() == 0);
    assert(tsnative_scheduler_wake(tsnative_scheduler_current_task()) == 0);
    return;
  }
  assert(value == 1);
  atomic_store(&fast_phase, 2);
}
int main(void) {
  tsnative_task *task = tsnative_task_spawn(parked, 0);
  assert(task);
  while (tsnative_task_get_status(task) != WAITING) sched_yield();
  assert(atomic_load(&phase) == 1);
  assert(tsnative_scheduler_wake(task) == 0);
  assert(tsnative_task_join(task) == 0);
  assert(atomic_load(&phase) == 2);
  tsnative_task_release(task);
  tsnative_task *fast = tsnative_task_spawn(fast_wake, 0);
  assert(fast);
  assert(tsnative_task_join(fast) == 0);
  assert(atomic_load(&fast_phase) == 2);
  tsnative_task_release(fast);
  tsnative_scheduler_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	testObj := filepath.Join(dir, "test.o")
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	binary := filepath.Join(dir, "park_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, goRuntime}, binary); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run task park/wake test: %v: %s", err, output)
	}
}

func TestNativeTaskCompletionAwaitParksParent(t *testing.T) {
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
	source := filepath.Join(dir, "task_await_test.c")
	program := `#include <assert.h>
#include <stdatomic.h>
typedef struct tsnative_task tsnative_task;
typedef void (*task_entry)(void *, void *);
tsnative_task *tsnative_task_spawn(task_entry, void *);
int tsnative_task_await_task(tsnative_task *);
int tsnative_task_join(tsnative_task *);
void tsnative_task_release(tsnative_task *);
void tsnative_scheduler_shutdown(void);
static atomic_int phase;
static tsnative_task *child;
static void child_entry(void *state, void *result) { (void)state; (void)result; atomic_store(&phase, 2); }
static void parent_entry(void *state, void *result) {
  (void)state; (void)result;
  int p = atomic_load(&phase);
  if (p == 0) {
    child = tsnative_task_spawn(child_entry, 0); assert(child);
    atomic_store(&phase, 1);
    assert(tsnative_task_await_task(child) == 0);
    return;
  }
  assert(p == 2);
  assert(tsnative_task_join(child) == 0);
  tsnative_task_release(child);
  atomic_store(&phase, 3);
}
int main(void) {
  tsnative_task *parent = tsnative_task_spawn(parent_entry, 0); assert(parent);
  assert(tsnative_task_join(parent) == 0);
  assert(atomic_load(&phase) == 3);
  tsnative_task_release(parent);
  tsnative_scheduler_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	testObj := filepath.Join(dir, "test.o")
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	binary := filepath.Join(dir, "task_await_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, goRuntime}, binary); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run task await: %v: %s", err, output)
	}
}

func TestNativeReferenceAwaitTransferStaysRooted(t *testing.T) {
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
	source := filepath.Join(dir, "await_ref_test.c")
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	header := goRuntimeHeaderForTest(t, goRuntime)
	program := fmt.Sprintf(`#include %q
#include <assert.h>
#include <stdint.h>
#include <stdlib.h>
typedef void tsnative_task;
typedef struct { int phase; void *value; tsnative_task *child; } parent_state;
static void child(void *state, void *result) {
  (void)state;
  uint64_t *value = tsnative_heap_alloc(sizeof(uint64_t));
  *value = 0x12345678ULL;
  *(void **)result = value;
}
static void parent(void *raw, void *result) {
  (void)result;
  parent_state *state = raw;
  if (state->phase == 0) {
    state->child = tsnative_task_spawn_ref(child, 0);
    assert(state->child);
    state->phase = 1;
    assert(tsnative_task_await_ref_task(state->child, &state->value) == 0);
    return;
  }
  assert(state->phase == 1);
  tsnative_gc_collect();
  assert(state->value);
  assert(*(uint64_t *)state->value == 0x12345678ULL);
  state->phase = 2;
}
int main(void) {
  parent_state *state = tsnative_heap_alloc(sizeof(*state));
  *state = (parent_state){0};
  tsnative_task *task = tsnative_task_spawn(parent, state);
  assert(task);
  assert(tsnative_task_join(task) == 0);
  assert(state->phase == 2);
  tsnative_task_release(task);
  tsnative_scheduler_shutdown();
  tsnative_heap_shutdown();
  return 0;
}`, header)
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	obj := filepath.Join(dir, "test.o")
	bin := filepath.Join(dir, "test")
	if err := clang.CompileC(ctx, source, obj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{obj, goRuntime}, bin); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run: %v: %s", err, output)
	}
}

func TestNativeTaskHundredThousandStress(t *testing.T) {
	clang, err := DiscoverClang()
	if err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "task_100k_test.c")
	program := `#include <assert.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdlib.h>
typedef struct tsnative_task tsnative_task;
typedef void (*tsnative_task_entry)(void *, void *);
tsnative_task *tsnative_task_spawn(tsnative_task_entry, void *);
int tsnative_task_join(tsnative_task *);
void tsnative_task_release(tsnative_task *);
void tsnative_scheduler_shutdown(void);
uint64_t tsnative_scheduler_spawned_tasks(void);
uint64_t tsnative_scheduler_completed_tasks(void);
static atomic_uint done;
static void job(void *state, void *result_slot) {
  (void)state; (void)result_slot;
  atomic_fetch_add_explicit(&done, 1u, memory_order_relaxed);
}
int main(void) {
  enum { N = 100000 };
  tsnative_task **tasks = calloc(N, sizeof(*tasks));
  assert(tasks);
  for (int i = 0; i < N; i++) { tasks[i] = tsnative_task_spawn(job, 0); assert(tasks[i]); }
  for (int i = 0; i < N; i++) { assert(tsnative_task_join(tasks[i]) == 0); tsnative_task_release(tasks[i]); }
  assert(atomic_load_explicit(&done, memory_order_relaxed) == N);
  assert(tsnative_scheduler_spawned_tasks() == N);
  assert(tsnative_scheduler_completed_tasks() == N);
  free(tasks);
  tsnative_scheduler_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	testObj := filepath.Join(dir, "test.o")
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	binary := filepath.Join(dir, "task_100k_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, goRuntime}, binary); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=8", "TSNATIVE_MAX_TASKS=120000")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run 100k task stress: %v: %s", err, output)
	}
}

func TestNativeTaskFailureCanBeInspectedWithoutAbort(t *testing.T) {
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
	source := filepath.Join(dir, "task_failure_test.c")
	program := `#include <assert.h>
#include <stddef.h>
typedef struct tsnative_task tsnative_task;
typedef void (*tsnative_task_entry)(void *, void *);
tsnative_task *tsnative_task_spawn(tsnative_task_entry, void *);
int tsnative_task_join(tsnative_task *);
int tsnative_task_get_failure(tsnative_task *, void **);
void tsnative_task_fail_current(void *);
void tsnative_task_release(tsnative_task *);
void tsnative_scheduler_shutdown(void);
void *tsnative_heap_alloc(size_t);
void tsnative_gc_collect(void);
size_t tsnative_heap_live_allocations(void);
static void fail_job(void *state, void *result_slot) {
  (void)state; (void)result_slot;
  void *payload = tsnative_heap_alloc(16);
  assert(payload);
  tsnative_task_fail_current(payload);
}
int main(void) {
  tsnative_task *task = tsnative_task_spawn(fail_job, 0);
  assert(task);
  assert(tsnative_task_join(task) == -1);
  void *failure = 0;
  assert(tsnative_task_get_failure(task, &failure) == 1);
  assert(failure);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 1);
  tsnative_task_release(task);
  tsnative_gc_collect();
  assert(tsnative_heap_live_allocations() == 0);
  tsnative_scheduler_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	testObj := filepath.Join(dir, "test.o")
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	binary := filepath.Join(dir, "task_failure_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, goRuntime}, binary); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run task failure inspection: %v: %s", err, output)
	}
}
func TestNativeTaskCompletionFansOutToMultipleWaiters(t *testing.T) {
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
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	header := goRuntimeHeaderForTest(t, goRuntime)
	source := filepath.Join(dir, "task_multi_waiter_test.c")
	program := fmt.Sprintf(`#include %q
#include <assert.h>
#include <stdatomic.h>
#include <sched.h>
typedef void tsnative_task;
typedef struct { int phase; } waiter_state;
static tsnative_task *child_task;
static atomic_int child_parked;
static atomic_int registered_waiters;
static atomic_int resumed_waiters;
static void child(void *state, void *result) {
  (void)state; (void)result;
  if (atomic_load(&child_parked) == 0) {
    atomic_store(&child_parked, 1);
    assert(tsnative_scheduler_prepare_park() == 0);
    return;
  }
}
static void waiter(void *raw, void *result) {
  (void)result;
  waiter_state *state = raw;
  if (state->phase == 0) {
    state->phase = 1;
    assert(tsnative_task_await_task(child_task) == 0);
    atomic_fetch_add(&registered_waiters, 1);
    return;
  }
  assert(state->phase == 1);
  state->phase = 2;
  atomic_fetch_add(&resumed_waiters, 1);
}
int main(void) {
  waiter_state one = {0}, two = {0};
  child_task = tsnative_task_spawn(child, 0); assert(child_task);
  while (atomic_load(&child_parked) == 0) sched_yield();
  tsnative_task *first = tsnative_task_spawn(waiter, &one); assert(first);
  tsnative_task *second = tsnative_task_spawn(waiter, &two); assert(second);
  while (atomic_load(&registered_waiters) != 2) sched_yield();
  assert(tsnative_scheduler_wake(child_task) == 0);
  assert(tsnative_task_join(first) == 0);
  assert(tsnative_task_join(second) == 0);
  assert(tsnative_task_join(child_task) == 0);
  assert(atomic_load(&resumed_waiters) == 2);
  assert(one.phase == 2 && two.phase == 2);
  tsnative_task_release(first);
  tsnative_task_release(second);
  tsnative_task_release(child_task);
  tsnative_scheduler_shutdown();
  return 0;
}`, header)
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	obj := filepath.Join(dir, "test.o")
	binary := filepath.Join(dir, "task_multi_waiter_test")
	if err := clang.CompileC(ctx, source, obj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{obj, goRuntime}, binary); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run multi-waiter completion test: %v: %s", err, output)
	}
}
