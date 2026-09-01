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
