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

func TestNativeF64ChannelStoragePrimitives(t *testing.T) {
	clang, err := DiscoverClang()
	if err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "channel_test.c")
	program := `#include <assert.h>
#include <pthread.h>
#include <stddef.h>
typedef struct tsnative_channel_f64 tsnative_channel_f64;
tsnative_channel_f64 *tsnative_channel_f64_new(size_t);
void tsnative_channel_f64_send(tsnative_channel_f64 *, double);
double tsnative_channel_f64_recv(tsnative_channel_f64 *);
void tsnative_heap_shutdown(void);
static tsnative_channel_f64 *ch;
static void *producer(void *unused) {
  (void)unused;
  for (int i = 0; i < 1000; i++) tsnative_channel_f64_send(ch, (double)i);
  return 0;
}
static void run_case(size_t capacity) {
  ch = tsnative_channel_f64_new(capacity);
  assert(ch);
  pthread_t thread;
  assert(pthread_create(&thread, 0, producer, 0) == 0);
  double sum = 0;
  for (int i = 0; i < 1000; i++) sum += tsnative_channel_f64_recv(ch);
  assert(pthread_join(thread, 0) == 0);
  assert(sum == 499500.0);
}
int main(void) {
  run_case(0);
  run_case(16);
  tsnative_heap_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	testObj := filepath.Join(dir, "test.o")
	schedulerObj := filepath.Join(dir, "scheduler.o")
	channelObj := filepath.Join(dir, "channel.o")
	heapObj := filepath.Join(dir, "heap.o")
	binary := filepath.Join(dir, "channel_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "scheduler.c"), schedulerObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "channel_f64.c"), channelObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "core", "heap.c"), heapObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, schedulerObj, channelObj, heapObj}, binary); err != nil {
		t.Fatal(err)
	}
	output, err := exec.CommandContext(ctx, binary).CombinedOutput()
	if err != nil {
		t.Fatalf("run channel storage test: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != "" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestNativeF64ChannelTryPrimitives(t *testing.T) {
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
	source := filepath.Join(dir, "channel_try_test.c")
	program := `#include <assert.h>
#include <stddef.h>
typedef struct tsnative_channel_f64 tsnative_channel_f64;
tsnative_channel_f64 *tsnative_channel_f64_new(size_t);
int tsnative_channel_f64_try_send(tsnative_channel_f64 *, double);
int tsnative_channel_f64_try_recv(tsnative_channel_f64 *, double *);
void tsnative_heap_shutdown(void);
int main(void) {
  double out = -1;
  tsnative_channel_f64 *buffered = tsnative_channel_f64_new(2);
  assert(tsnative_channel_f64_try_recv(buffered, &out) == 0);
  assert(tsnative_channel_f64_try_send(buffered, 10) == 1);
  assert(tsnative_channel_f64_try_send(buffered, 20) == 1);
  assert(tsnative_channel_f64_try_send(buffered, 30) == 0);
  assert(tsnative_channel_f64_try_recv(buffered, &out) == 1 && out == 10);
  assert(tsnative_channel_f64_try_recv(buffered, &out) == 1 && out == 20);
  assert(tsnative_channel_f64_try_recv(buffered, &out) == 0);
  tsnative_channel_f64 *unbuffered = tsnative_channel_f64_new(0);
  assert(tsnative_channel_f64_try_recv(unbuffered, &out) == 0);
  assert(tsnative_channel_f64_try_send(unbuffered, 42) == 1);
  assert(tsnative_channel_f64_try_send(unbuffered, 43) == 0);
  assert(tsnative_channel_f64_try_recv(unbuffered, &out) == 1 && out == 42);
  assert(tsnative_channel_f64_try_recv(unbuffered, &out) == 0);
  tsnative_heap_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	testObj := filepath.Join(dir, "test.o")
	schedulerObj := filepath.Join(dir, "scheduler.o")
	channelObj := filepath.Join(dir, "channel.o")
	heapObj := filepath.Join(dir, "heap.o")
	binary := filepath.Join(dir, "channel_try_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "scheduler.c"), schedulerObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "channel_f64.c"), channelObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "core", "heap.c"), heapObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, schedulerObj, channelObj, heapObj}, binary); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("run channel try test: %v: %s", err, output)
	}
}

func TestNativeF64ChannelTaskParkingSingleWorker(t *testing.T) {
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
	source := filepath.Join(dir, "channel_task_test.c")
	program := `#include <assert.h>
#include <stddef.h>
typedef struct tsnative_channel_f64 tsnative_channel_f64;
typedef struct tsnative_task tsnative_task;
typedef void (*tsnative_task_entry)(void *, void *);
tsnative_channel_f64 *tsnative_channel_f64_new(size_t);
int tsnative_channel_f64_send_task(tsnative_channel_f64 *, double);
int tsnative_channel_f64_recv_task(tsnative_channel_f64 *, double *);
tsnative_task *tsnative_task_spawn(tsnative_task_entry, void *);
int tsnative_task_join(tsnative_task *);
void tsnative_task_release(tsnative_task *);
void tsnative_scheduler_shutdown(void);
typedef struct { tsnative_channel_f64 *ch; int phase; double a; double b; } state;
static void recv_one(void *raw, void *result) {
  (void)result; state *s = raw;
  if (s->phase == 0) {
    s->phase = 1;
    int r = tsnative_channel_f64_recv_task(s->ch, &s->a);
    assert(r >= 0); if (r == 0) return;
  }
  assert(s->phase == 1); s->phase = 2;
}
static void send_one(void *raw, void *result) {
  (void)result; state *s = raw;
  if (s->phase == 0) {
    s->phase = 1;
    int r = tsnative_channel_f64_send_task(s->ch, 42);
    assert(r >= 0); if (r == 0) return;
  }
  assert(s->phase == 1); s->phase = 2;
}
static void send_two(void *raw, void *result) {
  (void)result; state *s = raw;
  if (s->phase == 0) { s->phase = 1; int r = tsnative_channel_f64_send_task(s->ch, 10); assert(r >= 0); if (r == 0) return; }
  if (s->phase == 1) { s->phase = 2; int r = tsnative_channel_f64_send_task(s->ch, 20); assert(r >= 0); if (r == 0) return; }
  assert(s->phase == 2); s->phase = 3;
}
static void recv_two(void *raw, void *result) {
  (void)result; state *s = raw;
  if (s->phase == 0) { s->phase = 1; int r = tsnative_channel_f64_recv_task(s->ch, &s->a); assert(r >= 0); if (r == 0) return; }
  if (s->phase == 1) { s->phase = 2; int r = tsnative_channel_f64_recv_task(s->ch, &s->b); assert(r >= 0); if (r == 0) return; }
  assert(s->phase == 2); s->phase = 3;
}
static void join_release(tsnative_task *task) { assert(tsnative_task_join(task) == 0); tsnative_task_release(task); }
int main(void) {
  state recv_first = {.ch = tsnative_channel_f64_new(0)}, send_after = {.ch = recv_first.ch};
  tsnative_task *r1 = tsnative_task_spawn(recv_one, &recv_first);
  tsnative_task *s1 = tsnative_task_spawn(send_one, &send_after);
  join_release(r1); join_release(s1);
  assert(recv_first.phase == 2 && send_after.phase == 2 && recv_first.a == 42);
  state send_first = {.ch = tsnative_channel_f64_new(0)}, recv_after = {.ch = send_first.ch};
  tsnative_task *s2 = tsnative_task_spawn(send_one, &send_first);
  tsnative_task *r2 = tsnative_task_spawn(recv_one, &recv_after);
  join_release(s2); join_release(r2);
  assert(send_first.phase == 2 && recv_after.phase == 2 && recv_after.a == 42);
  state p = {.ch = tsnative_channel_f64_new(1)}, c = {.ch = p.ch};
  tsnative_task *pt = tsnative_task_spawn(send_two, &p);
  tsnative_task *ct = tsnative_task_spawn(recv_two, &c);
  join_release(pt); join_release(ct);
  assert(p.phase == 3 && c.phase == 3 && c.a == 10 && c.b == 20);
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
	channelObj := filepath.Join(dir, "channel.o")
	heapObj := filepath.Join(dir, "heap.o")
	binary := filepath.Join(dir, "channel_task_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "scheduler.c"), schedulerObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "task.c"), taskObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "channel_f64.c"), channelObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, filepath.Join(root, "runtime", "core", "heap.c"), heapObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, schedulerObj, taskObj, channelObj, heapObj}, binary); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1", "TSNATIVE_MAX_TASKS=32")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run channel task parking test: %v: %s", err, output)
	}
}
