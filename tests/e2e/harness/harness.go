package harness

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/pkg/tspro"
)

type LinuxAMD64Case struct {
	Name     string
	Source   string
	Expected string
}

type HappyCase struct {
	Name     string
	Source   string
	Expected string
}

type BadCase struct {
	Name         string
	Source       string
	ExpectedCode string
	ExpectedSub  string
}

func MustReadExample(t *testing.T, path string) string {
	t.Helper()
	if data, err := os.ReadFile(path); err == nil {
		return string(data)
	}

	dir, err := os.Getwd()
	if err == nil {
		cleanRel := path
		for strings.HasPrefix(cleanRel, "../") {
			cleanRel = strings.TrimPrefix(cleanRel, "../")
		}
		for i := 0; i < 6; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				candidate := filepath.Join(dir, cleanRel)
				if data, err := os.ReadFile(candidate); err == nil {
					return string(data)
				}
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read example %s: %v", path, err)
	}
	return string(data)
}

func RunLinuxAMD64(t *testing.T, tc LinuxAMD64Case) {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "linux_amd64_bin")
	compiler := tspro.New(tspro.Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	bin, diags, err := compiler.CompileSource(tc.Name+".ts", []byte(tc.Source))
	if err != nil {
		t.Fatalf("linux/amd64 compile failed: %v, diagnostics: %s", err, diags.Format(compiler.FileSet()))
	}
	if err := os.WriteFile(binPath, bin, 0o755); err != nil {
		t.Fatalf("write linux/amd64 executable: %v", err)
	}
	AssertLinuxAMD64ELF(t, binPath)

	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Logf("linux/amd64 execution skipped on host %s/%s; ELF validation still ran", runtime.GOOS, runtime.GOARCH)
		return
	}
	out, err := exec.Command(binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("linux/amd64 execution failed: %v\nOutput:\n%s", err, string(out))
	}
	if string(out) != tc.Expected {
		t.Fatalf("linux/amd64 stdout: got %q, want %q", string(out), tc.Expected)
	}
}

func AssertLinuxAMD64ELF(t *testing.T, path string) {
	t.Helper()
	f, err := elf.Open(path)
	if err != nil {
		t.Fatalf("open generated ELF: %v", err)
	}
	defer f.Close()

	if f.Class != elf.ELFCLASS64 || f.Data != elf.ELFDATA2LSB || f.Machine != elf.EM_X86_64 || f.Type != elf.ET_EXEC {
		t.Fatalf("unexpected ELF header: class=%v data=%v machine=%v type=%v", f.Class, f.Data, f.Machine, f.Type)
	}
	if f.Entry == 0 {
		t.Fatal("generated ELF has zero entrypoint")
	}

	entryInExecLoad := false
	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			t.Fatal("linux/amd64 output must be a standalone executable without PT_INTERP")
		}
		if prog.Type == elf.PT_LOAD && prog.Flags&elf.PF_X != 0 && f.Entry >= prog.Vaddr && f.Entry < prog.Vaddr+prog.Memsz {
			entryInExecLoad = true
		}
	}
	if !entryInExecLoad {
		t.Fatalf("entrypoint %#x is not inside an executable PT_LOAD segment", f.Entry)
	}
}

func RunHappy(t *testing.T, tc HappyCase) {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "e2e_bin")

	opts := tspro.DefaultOptions()
	opts.OptLevel = 2
	compiler := tspro.New(opts)

	bin, diags, err := compiler.CompileSource(tc.Name+".ts", []byte(tc.Source))
	if err != nil {
		t.Fatalf("compile failed: %v, diagnostics: %s", err, diags.Format(compiler.FileSet()))
	}

	if err := os.WriteFile(binPath, bin, 0755); err != nil {
		t.Fatalf("write binary failed: %v", err)
	}

	if runtime.GOOS == "darwin" {
		_ = exec.Command("codesign", "-s", "-", binPath).Run()
	}

	cmd := exec.Command(binPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("execution failed: %v\nOutput:\n%s", err, string(out))
	}

	if string(out) != tc.Expected {
		t.Errorf("expected stdout:\n%q\ngot:\n%q", tc.Expected, string(out))
	}
}

func RunBad(t *testing.T, tc BadCase) {
	t.Helper()
	compiler := tspro.New(tspro.DefaultOptions())
	bin, diags, err := compiler.CompileSource(tc.Name+".ts", []byte(tc.Source))

	if err == nil {
		t.Fatalf("expected compile failure for bad case %q, but compilation succeeded with binary size %d", tc.Name, len(bin))
	}

	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics to report errors for %q, got none", tc.Name)
	}

	diagStr := diags.Format(compiler.FileSet())
	foundCode := false
	for _, d := range diags {
		if d.Code == tc.ExpectedCode {
			foundCode = true
			break
		}
	}

	if !foundCode {
		t.Errorf("expected diagnostic code %q, but got diagnostics:\n%s", tc.ExpectedCode, diagStr)
	}

	if tc.ExpectedSub != "" && !strings.Contains(diagStr, tc.ExpectedSub) {
		t.Errorf("expected message snippet %q in diagnostics:\n%s", tc.ExpectedSub, diagStr)
	}
}
