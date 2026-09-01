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

func TestNativeSchedulerRespectsWorkerBoundAndShutdown(t *testing.T) {
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
	source := filepath.Join(dir, "scheduler_test.c")
	program := `#include <assert.h>
#include <stddef.h>
#include <stdio.h>
int tsnative_scheduler_init(void);
void tsnative_scheduler_shutdown(void);
size_t tsnative_scheduler_worker_count(void);
int tsnative_scheduler_is_running(void);
int main(void) {
  assert(!tsnative_scheduler_is_running());
  assert(tsnative_scheduler_init() == 0);
  assert(tsnative_scheduler_is_running());
  printf("workers=%zu\n", tsnative_scheduler_worker_count());
  tsnative_scheduler_shutdown();
  assert(!tsnative_scheduler_is_running());
  assert(tsnative_scheduler_init() == 0);
  tsnative_scheduler_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	testObj := filepath.Join(dir, "test.o")
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	binary := filepath.Join(dir, "scheduler_test")
	if err := clang.CompileC(ctx, source, testObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{testObj, goRuntime}, binary); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=3")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run scheduler test: %v: %s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "workers=3" {
		t.Fatalf("output = %q", got)
	}
}
