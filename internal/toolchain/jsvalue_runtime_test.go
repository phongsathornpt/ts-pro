package toolchain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGoRuntimeJSValueABI(t *testing.T) {
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
	source := filepath.Join(dir, "jsvalue.c")
	program := `#include <stdint.h>
extern void *tsnative_string_new(const char *data, uint64_t len);
extern void *tsnative_jsvalue_box_f64(double value);
extern void *tsnative_jsvalue_box_string(void *value);
extern void *tsnative_jsvalue_box_bool(uint8_t value);
extern void *tsnative_jsvalue_null(void);
extern void *tsnative_jsvalue_undefined(void);
extern void *tsnative_jsvalue_add(void *left, void *right);
extern void tsnative_console_log_jsvalue(void *value);
extern void tsnative_heap_shutdown(void);
`
	program += `int main(void) {
  void *twenty = tsnative_jsvalue_box_f64(20.0);
  void *twenty_two = tsnative_jsvalue_box_f64(22.0);
  void *sum = tsnative_jsvalue_add(twenty, twenty_two);
  tsnative_console_log_jsvalue(sum);
  void *prefix_string = tsnative_string_new("value=", 6);
  void *prefix = tsnative_jsvalue_box_string(prefix_string);
  void *combined = tsnative_jsvalue_add(prefix, sum);
  tsnative_console_log_jsvalue(combined);
  void *truth = tsnative_jsvalue_box_bool(1);
  void *two = tsnative_jsvalue_box_f64(2.0);
  tsnative_console_log_jsvalue(truth);
  tsnative_console_log_jsvalue(tsnative_jsvalue_add(truth, two));
  void *bool_prefix_string = tsnative_string_new("bool=", 5);
  void *bool_prefix = tsnative_jsvalue_box_string(bool_prefix_string);
  tsnative_console_log_jsvalue(tsnative_jsvalue_add(bool_prefix, truth));
  void *null_value = tsnative_jsvalue_null();
  void *undefined_value = tsnative_jsvalue_undefined();
  tsnative_console_log_jsvalue(null_value);
  tsnative_console_log_jsvalue(undefined_value);
  tsnative_console_log_jsvalue(tsnative_jsvalue_add(null_value, two));
  void *null_prefix_string = tsnative_string_new("null=", 5);
  void *null_prefix = tsnative_jsvalue_box_string(null_prefix_string);
  tsnative_console_log_jsvalue(tsnative_jsvalue_add(null_prefix, null_value));
  tsnative_console_log_jsvalue(tsnative_jsvalue_add(undefined_value, two));
  tsnative_heap_shutdown();
  return 0;
}
`
	if err := os.WriteFile(source, []byte(program), 0o644); err != nil {
		t.Fatal(err)
	}
	goRuntime := buildGoRuntimeArchiveForTest(t, ctx, root, clang)
	obj := filepath.Join(dir, "jsvalue.o")
	binary := filepath.Join(dir, "jsvalue-test")
	if err := clang.CompileC(ctx, source, obj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{obj, goRuntime}, binary); err != nil {
		t.Fatal(err)
	}
	output, err := exec.CommandContext(ctx, binary).CombinedOutput()
	if err != nil {
		t.Fatalf("run Go JSValue runtime test: %v: %s", err, output)
	}
	if string(output) != "42\nvalue=42\ntrue\n3\nbool=true\nnull\nundefined\n2\nnull=null\nNaN\n" {
		t.Fatalf("output = %q", output)
	}
}
