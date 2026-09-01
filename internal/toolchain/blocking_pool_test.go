package toolchain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeBlockingPoolBoundAndTaskWake(t *testing.T) {
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
	source := filepath.Join(dir, "blocking_pool_test.c")
	program := `#include <assert.h>
#include <sched.h>
#include <stdatomic.h>
#include <stddef.h>
#include <stdint.h>
#include <time.h>
typedef struct tsnative_task tsnative_task;
typedef struct tsnative_blocking_job tsnative_blocking_job;
typedef void (*task_entry)(void *, void *);
typedef void (*blocking_entry)(void *);
tsnative_task *tsnative_task_spawn(task_entry, void *);
int tsnative_task_join(tsnative_task *);
void tsnative_task_release(tsnative_task *);
void tsnative_scheduler_shutdown(void);
tsnative_blocking_job *tsnative_blocking_submit(blocking_entry, void *);
int tsnative_blocking_job_wait_task(tsnative_blocking_job *);
void tsnative_blocking_job_wait_cooperative(tsnative_blocking_job *);
void tsnative_blocking_job_release(tsnative_blocking_job *);
void tsnative_blocking_pool_shutdown(void);
size_t tsnative_blocking_worker_count(void);
size_t tsnative_blocking_active_jobs(void);
size_t tsnative_blocking_peak_active_jobs(void);
uint64_t tsnative_blocking_submitted_jobs(void);
uint64_t tsnative_blocking_completed_jobs(void);
static atomic_int gate;
static atomic_int done;
static void gated_job(void *state) {
  (void)state;
  while (!atomic_load(&gate)) sched_yield();
  atomic_fetch_add(&done, 1);
}
static void slow_job(void *state) {
  (void)state;
  struct timespec ts = {.tv_sec = 0, .tv_nsec = 20000000};
  nanosleep(&ts, 0);
}
typedef struct {
  tsnative_blocking_job *job;
  atomic_int phase;
} task_state;
static void waiting_task(void *raw, void *result) {
  (void)result;
  task_state *state = raw;
  int phase = atomic_load(&state->phase);
  if (phase == 0) {
    state->job = tsnative_blocking_submit(slow_job, 0);
    assert(state->job);
    atomic_store(&state->phase, 1);
    assert(tsnative_blocking_job_wait_task(state->job) == 0);
    return;
  }
  assert(phase == 1);
  tsnative_blocking_job_release(state->job);
  atomic_store(&state->phase, 2);
}
int main(void) {
  assert(tsnative_blocking_worker_count() == 2);
  tsnative_blocking_job *jobs[4];
  for (int i = 0; i < 4; i++) {
    jobs[i] = tsnative_blocking_submit(gated_job, 0);
    assert(jobs[i]);
  }
  assert(tsnative_blocking_submit(gated_job, 0) == 0);
  atomic_store(&gate, 1);
  for (int i = 0; i < 4; i++) {
    tsnative_blocking_job_wait_cooperative(jobs[i]);
    tsnative_blocking_job_release(jobs[i]);
  }
  assert(atomic_load(&done) == 4);
  assert(tsnative_blocking_active_jobs() == 0);
  assert(tsnative_blocking_peak_active_jobs() == 4);

  task_state state = {0};
  tsnative_task *task = tsnative_task_spawn(waiting_task, &state);
  assert(task);
  assert(tsnative_task_join(task) == 0);
  assert(atomic_load(&state.phase) == 2);
  tsnative_task_release(task);
  assert(tsnative_blocking_submitted_jobs() == 5);
  assert(tsnative_blocking_completed_jobs() == 5);
  tsnative_blocking_pool_shutdown();
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
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	blockingObj := filepath.Join(dir, "blocking_pool.o")
	binary := filepath.Join(dir, "blocking_pool_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "scheduler.c"), schedulerObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "task.c"), taskObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "blocking_pool.c"), blockingObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, schedulerObj, taskObj, goRuntime, blockingObj}, binary); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, binary)
	command.Env = append(os.Environ(),
		"TSNATIVE_WORKERS=1",
		"TSNATIVE_BLOCKING_WORKERS=2",
		"TSNATIVE_MAX_BLOCKING_JOBS=4",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run blocking pool test: %v: %s", err, output)
	}
}
