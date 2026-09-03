package e2e_test

import (
	"debug/elf"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func TestLinuxAMD64ArenaAllocator(t *testing.T) {
	for _, n := range []int{1000, 2500} {
		t.Run(fmt.Sprintf("concat_%d", n), func(t *testing.T) {
			runLinuxAMD64(t, linuxAMD64Case{
				name: fmt.Sprintf("arena_concat_%d", n),
				source: fmt.Sprintf(`
let s = "";
for (let i = 0; i < %d; i = i + 1) { s = s + "x"; }
console.log(s);
`, n),
				expected: strings.Repeat("x", n) + "\n",
			})
		})
	}
}

func TestLinuxAMD64NumberToStringShortestRoundTrip(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "number_to_string_shortest_roundtrip",
		source: `
console.log(0.1);
console.log(0.2);
console.log(1.1);
console.log(1.2345);
console.log(0.1 + 0.2);
console.log(100000000000000000000);
console.log(1e21);
console.log(0.000001);
console.log(0.0000001);
console.log(1.2345678901234567);
console.log(1000000000000000100);
console.log(2.2250738585072014e-308);
console.log(5e-324);
`,
		expected: "0.1\n0.2\n1.1\n1.2345\n0.30000000000000004\n100000000000000000000\n1e+21\n0.000001\n1e-7\n1.2345678901234567\n1000000000000000100\n2.2250738585072014e-308\n5e-324\n",
	})
}

func TestLinuxAMD64GCPreservesLiveStringRoots(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "gc_preserves_live_string_roots",
		source: `
function churn(keep: string): string {
  for (let i = 0; i < 50000; i = i + 1) {
    let garbage = "ab" + "cd";
  }
  return keep;
}
let keep = "keep-" + "alive";
console.log(churn(keep));
`,
		expected: "keep-alive\n",
	})
}

func TestLinuxAMD64NumberArrays(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "number_array_read_write_push_pop",
		source: `
function sum(xs: number[]): number {
  let total = 0;
  for (let i = 0; i < xs.length; i++) total = total + xs[i]!;
  xs[1] = 10;
  console.log(xs.push(20));
  console.log(xs.length);
  console.log(xs[1]);
  console.log(xs.pop()!);
  return total;
}
console.log(sum([1, 2, 3]));
`,
		expected: "4\n4\n10\n20\n6\n",
	})
}

func TestLinuxAMD64ReferenceArraysSurviveGC(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "reference_arrays_gc_graph",
		source: `
let xs: string[] = ["seed"];
xs[0] = "keep-" + "alive";
for (let i = 0; i < 50000; i++) { const garbage = "ab" + "cd"; }
console.log(xs[0]);
let inner: string[] = ["nested-" + "alive"];
let outer: string[][] = [inner];
for (let i = 0; i < 50000; i++) { const garbage = "ef" + "gh"; }
console.log(outer[0]![0]);
`,
		expected: "keep-alive\nnested-alive\n",
	})
}

func TestLinuxAMD64ArrayCapacityGrowth(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "array_capacity_growth_preserves_payloads",
		source: `
let nums: number[] = [1, 2, 3, 4];
console.log(nums.push(5));
console.log(nums[0]);
console.log(nums[4]);
let refs: string[] = ["a", "b", "c", "d"];
refs[0] = "keep-" + "alive";
for (let i = 0; i < 50000; i++) { const garbage = "ab" + "cd"; }
console.log(refs.push("e"));
console.log(refs[0]);
console.log(refs.pop()!);
console.log(refs.length);
`,
		expected: "5\n1\n5\n5\nkeep-alive\ne\n4\n",
	})
}

func TestLinuxAMD64ArrayIndexedWriteGrowsCapacity(t *testing.T) {
	cases := []linuxAMD64Case{
		{
			name: "number_array_indexed_write_growth",
			source: `
let xs: number[] = [1, 2];
xs[8] = 9;
console.log(xs.length);
console.log(xs[0]);
console.log(xs[8]);
`,
			expected: "9\n1\n9\n",
		},
		{
			name: "reference_array_large_index_growth_preserves_value",
			source: `
let xs: string[] = ["seed"];
let keep = "keep-" + "alive";
xs[200000] = keep;
console.log(xs.length);
console.log(xs[200000]);
`,
			expected: "200001\nkeep-alive\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runLinuxAMD64(t, tc) })
	}
}

func TestLinuxAMD64ClosedObjectsAndGC(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "closed_object_fields_mutation_and_gc",
		source: `
type Point = { x: number; name: string };
type Box = { label: string; child: Point };
let p: Point = { x: 1, name: "one" };
console.log(p.x);
console.log(p.name);
p.x = 2.5;
p.name = "two";
console.log(p.x);
console.log(p.name);
let box: Box = { label: "keep-" + "alive", child: p };
for (let i = 0; i < 50000; i++) { const garbage = "ab" + "cd"; }
console.log(box.label);
console.log(box.child.name);
`,
		expected: "1\none\n2.5\ntwo\nkeep-alive\ntwo\n",
	})
}

func TestLinuxAMD64CompoundAssignments(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "compound_assignment_locals_arrays_objects",
		source: `
let x = 10;
x += 5;
x *= 2;
x -= 4;
x /= 2;
console.log(x);
let s = "a";
s += "b";
console.log(s);
let xs: number[] = [1, 2];
xs[1] += 3;
console.log(xs[1]);
let refs: string[] = ["r"];
refs[0] += "!";
console.log(refs[0]);
type Box = { n: number; label: string };
let box: Box = { n: 2, label: "hi" };
box.n *= 4;
box.label += "!";
console.log(box.n);
console.log(box.label);
`,
		expected: "13\nab\n5\nr!\n8\nhi!\n",
	})
}

func TestLinuxAMD64ClosuresFunctionArraysAndGC(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "closures_function_arrays_and_gc",
		source: `
function plusOne(x: number): number { return x + 1; }
const offset = 5;
const addOffset = (x: number): number => x + offset;
const funcs: Array<(x: number) => number> = [plusOne, addOffset];
console.log(funcs.length);
console.log(funcs[0]!(3));
console.log(funcs[1]!(7));

function makeAdder(base: number): (x: number) => number {
  const add = (x: number): number => base + x;
  return add;
}
const add5 = makeAdder(5);
console.log(add5(7));

function makePrefix(prefix: string): (x: string) => string {
  const add = (x: string): string => prefix + x;
  return add;
}
const prefix = makePrefix("keep-" + "");
for (let i = 0; i < 50000; i++) { const garbage = "ab" + "cd"; }
console.log(prefix("alive"));
`,
		expected: "2\n4\n12\n12\nkeep-alive\n",
	})
}

func TestLinuxAMD64ClosureReferenceGraphAndOverflowArgs(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "closure_reference_graph_and_overflow_args",
		source: `
const state = { nums: [40], label: "keep-" + "alive" };
const readNumber = (x: number): number => state.nums[0]! + x;
const readLabel = (): string => state.label;
for (let i = 0; i < 50000; i++) { const garbage = "ab" + "cd"; }
console.log(readNumber(2));
console.log(readLabel());

const add10 = (a: number, b: number, c: number, d: number, e: number, f: number, g: number, h: number, i: number, j: number): number =>
  a + b + c + d + e + f + g + h + i + j;
console.log(add10(1, 2, 3, 4, 5, 6, 7, 8, 9, 10));

const pickSixth = (a: string, b: string, c: string, d: string, e: string, f: string): string => f;
console.log(pickSixth("a", "b", "c", "d", "e", "stack-ok"));
`,
		expected: "42\nkeep-alive\n55\nstack-ok\n",
	})
}

func TestLinuxAMD64DoWhile(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "do_while_executes_body_before_condition",
		source: `
let count: number = 0;
let total: number = 0;
do {
  total = total + count;
  count++;
} while (count < 5);
let once: number = 0;
do { once++; } while (false);
console.log(total);
console.log(once);
`,
		expected: "10\n1\n",
	})
}

func TestLinuxAMD64StringEqualityByValue(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "string_equality_compares_bytes_not_addresses",
		source: `
let a = "keep-" + "alive";
let b = "keep-" + "alive";
let c = "different";
console.log(a === b);
console.log(a !== b);
console.log(a === c);
console.log(a !== c);
`,
		expected: "1\n0\n0\n1\n",
	})
}
