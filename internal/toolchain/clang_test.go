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

func TestClangCompilesAndLinksExecutable(t *testing.T) {
	clang, err := DiscoverClang()
	if err != nil {
		t.Skip(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	ll := filepath.Join(dir, "value.ll")
	c := filepath.Join(dir, "main.c")
	llObj := filepath.Join(dir, "value.o")
	cObj := filepath.Join(dir, "main.o")
	bin := filepath.Join(dir, "app")
	if err := os.WriteFile(ll, []byte("define i32 @tsnative_value() { ret i32 42 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainSource := "#include <stdio.h>\nextern int tsnative_value(void);\nint main(void){ printf(\"%d\\n\", tsnative_value()); return 0; }\n"
	if err := os.WriteFile(c, []byte(mainSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileLLVM(ctx, ll, llObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.CompileC(ctx, c, cObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := clang.Link(ctx, []string{llObj, cObj}, bin); err != nil {
		t.Fatal(err)
	}
	output, err := exec.CommandContext(ctx, bin).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(output)); got != "42" {
		t.Fatalf("output = %q", got)
	}
}
