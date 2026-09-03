package e2e_test

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/phongsathornpt/ts-pro/pkg/tspro"
)

type linuxAMD64Case struct {
	name     string
	source   string
	expected string
}

func runLinuxAMD64(t *testing.T, tc linuxAMD64Case) {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "linux_amd64_bin")
	compiler := tspro.New(tspro.Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	bin, diags, err := compiler.CompileSource(tc.name+".ts", []byte(tc.source))
	if err != nil {
		t.Fatalf("linux/amd64 compile failed: %v, diagnostics: %s", err, diags.Format(compiler.FileSet()))
	}
	if err := os.WriteFile(binPath, bin, 0o755); err != nil {
		t.Fatalf("write linux/amd64 executable: %v", err)
	}
	assertLinuxAMD64ELF(t, binPath)

	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Logf("linux/amd64 execution skipped on host %s/%s; ELF validation still ran", runtime.GOOS, runtime.GOARCH)
		return
	}
	out, err := exec.Command(binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("linux/amd64 execution failed: %v\nOutput:\n%s", err, string(out))
	}
	if string(out) != tc.expected {
		t.Fatalf("linux/amd64 stdout: got %q, want %q", string(out), tc.expected)
	}
}

func assertLinuxAMD64ELF(t *testing.T, path string) {
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

func TestLinuxAMD64ExplicitTarget(t *testing.T) {
	cases := []linuxAMD64Case{
		{
			name:     "function_only_entry",
			source:   `function add(a: number, b: number): number { return a + b; }`,
			expected: "",
		},
		{
			name: "sysv_six_register_arguments",
			source: `
function sum6(a: number, b: number, c: number, d: number, e: number, f: number): number {
  return a + b + c + d + e + f;
}
console.log(sum6(1, 2, 3, 4, 5, 6));
`,
			expected: "21\n",
		},
		{
			name: "control_flow_and_strings",
			source: `
let prefix = "Linux ";
if (1) {
  console.log(prefix + "AMD64");
} else {
  console.log("wrong branch");
}
`,
			expected: "Linux AMD64\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runLinuxAMD64(t, tc) })
	}
}

func TestLinuxAMD64F64Semantics(t *testing.T) {
	cases := []linuxAMD64Case{
		{
			name: "f64_arithmetic_and_special_values",
			source: `
function mod(a: number, b: number): number { return a % b; }
console.log(1.5 + 2.25);
console.log(5 / 2);
console.log(mod(5.5, 2));
console.log(1 / 0);
console.log(0 / 0);
console.log(-0);
`,
			expected: "3.75\n2.5\n1.5\nInfinity\nNaN\n-0\n",
		},
		{
			name: "f64_truthiness_and_nan_comparisons",
			source: `
function truth(x: number): number { if (x) { return 1; } return 0; }
function neg(x: number): number { return -x; }
console.log(truth(-0));
console.log(truth(0 / 0));
console.log(neg(0));
console.log((0 / 0) == (0 / 0));
console.log((0 / 0) != (0 / 0));
`,
			expected: "0\n1\n-0\n0\n1\n",
		},
		{
			name: "sysv_ten_sse_arguments",
			source: `
function sum10(a:number,b:number,c:number,d:number,e:number,f:number,g:number,h:number,i:number,j:number): number {
  return a+b+c+d+e+f+g+h+i+j;
}
console.log(sum10(1,2,3,4,5,6,7,8,9,10));
`,
			expected: "55\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runLinuxAMD64(t, tc) })
	}
}

func TestLinuxAMD64SysVStackIntegerClass(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "sysv_seven_integer_class_arguments",
		source: `
function pick7(a:string,b:string,c:string,d:string,e:string,f:string,g:string): string {
  return g;
}
console.log(pick7("a","b","c","d","e","f","stack-ok"));
`,
		expected: "stack-ok\n",
	})
}
