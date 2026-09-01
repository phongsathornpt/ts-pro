package toolchain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeTaskSleepParksAndResumes(t *testing.T) {
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
	source := filepath.Join(dir, "timer_test.c")
	program := `#include <assert.h>
#include <stdatomic.h>
#include <time.h>
typedef struct tsnative_task tsnative_task;
typedef void (*tsnative_task_entry)(void *, void *);
tsnative_task *tsnative_task_spawn(tsnative_task_entry, void *);
int tsnative_task_join(tsnative_task *);
void tsnative_task_release(tsnative_task *);
void tsnative_scheduler_shutdown(void);
int tsnative_sleep_task(double);
void tsnative_timer_shutdown(void);
static atomic_int phase;
static void sleeper(void *state, void *result) {
  (void)state; (void)result;
  int value = atomic_load(&phase);
  if (value == 0) {
    atomic_store(&phase, 1);
    assert(tsnative_sleep_task(20) == 0);
    return;
  }
  assert(value == 1);
  atomic_store(&phase, 2);
}
static long elapsed_ms(struct timespec a, struct timespec b) {
  return (b.tv_sec-a.tv_sec)*1000 + (b.tv_nsec-a.tv_nsec)/1000000;
}
int main(void) {
  struct timespec start, end;
  clock_gettime(CLOCK_MONOTONIC, &start);
  tsnative_task *task = tsnative_task_spawn(sleeper, 0);
  assert(task);
  assert(tsnative_task_join(task) == 0);
  clock_gettime(CLOCK_MONOTONIC, &end);
  assert(atomic_load(&phase) == 2);
  assert(elapsed_ms(start, end) >= 10);
  tsnative_task_release(task);
  tsnative_timer_shutdown();
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
	binary := filepath.Join(dir, "timer_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "scheduler.c"), schedulerObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "task.c"), taskObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, schedulerObj, taskObj, goRuntime}, binary); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run timer test: %v: %s", err, output)
	}
}
