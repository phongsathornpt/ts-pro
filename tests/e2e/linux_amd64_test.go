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

func TestLinuxAMD64SwitchControlFlow(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "closed_switch_number_and_string_cases",
		source: `
function classify(value: number): string {
  switch (value) {
    case 1: return "one";
    case 2: return "two";
    default: return "other";
  }
}
function action(value: string): number {
  let score = 0;
  switch (value) {
    case "start": score = 100; break;
    case "pause": score = 50; break;
    default: score = -1; break;
  }
  return score;
}
console.log(classify(0));
console.log(classify(1));
console.log(classify(2));
console.log(action("start"));
console.log(action("pause"));
console.log(action("unknown"));
`,
		expected: "other\none\ntwo\n100\n50\n-1\n",
	})
}

func TestLinuxAMD64ArrayForOf(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "array_for_of_number_boolean_string_elements",
		source: `
function sum(xs: number[]): number {
  let total = 0;
  for (const x of xs) { total = total + x; }
  return total;
}
function count(xs: boolean[]): number {
  let total = 0;
  for (const x of xs) { if (x) { total = total + 1; } }
  return total;
}
const words: string[] = ["a", "b", "c"];
console.log(sum([10, 20, 30, 40]));
console.log(count([true, false, true]));
for (const word of words) { console.log(word); }
`,
		expected: "100\n2\na\nb\nc\n",
	})
}

func TestLinuxAMD64GenericIdentitySpecialization(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "generic_identity_specialization",
		source: `
function identity<T>(x: T): T { return x; }
console.log(identity<number>(42));
console.log(identity<string>("hello"));
console.log(identity(7));
console.log(identity("inferred"));
`,
		expected: "42\nhello\n7\ninferred\n",
	})
}

func TestLinuxAMD64GenericTupleSpecialization(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "generic_tuple_specialization",
		source: `
function pair<A, B>(first: A, second: B): [A, B] { return [first, second]; }
const p = pair<string, number>("answer", 42);
console.log(p[0]);
console.log(p[1]);
console.log(p.length);
p[1] += 8;
console.log(p[1]);
const keep = pair("keep-" + "alive", 7);
for (let i = 0; i < 50000; i++) { const garbage = "ab" + "cd"; }
console.log(keep[0]);
console.log(keep[1]);
`,
		expected: "answer\n42\n2\n50\nkeep-alive\n7\n",
	})
}

func TestLinuxAMD64NativeClasses(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "native_classes_initializers_methods_mutation_and_gc",
		source: `
class Point {
  x: number = 20;
  y: number = 22;
  sum(): number { return this.x + this.y; }
}
class Counter {
  value: number = 1;
  bump(): number { this.value += 1; return this.value; }
}
class Label {
  constructor(public text: string) {}
  get(): string { return this.text; }
}
const p = new Point();
console.log(p.sum());
const c = new Counter();
console.log(c.bump());
console.log(c.bump());
const label = new Label("keep-" + "alive");
for (let i = 0; i < 50000; i++) { const garbage = "ab" + "cd"; }
console.log(label.get());
`,
		expected: "42\n2\n3\nkeep-alive\n",
	})
}

func TestLinuxAMD64ClassInheritanceAndVirtualDispatch(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "class_inheritance_virtual_dispatch_and_gc",
		source: `
class BaseItem {
  constructor(public value: number, public label: string) {}
  score(): number { return this.value; }
  name(): string { return this.label; }
}
class DerivedItem extends BaseItem {
  bonus: number = 2;
  constructor(value: number, label: string) { super(value, label); }
  override score(): number { return this.value + this.bonus; }
}
function readScore(item: BaseItem): number { return item.score(); }
function readName(item: BaseItem): string { return item.name(); }
const item: BaseItem = new DerivedItem(40, "keep-" + "alive");
for (let i = 0; i < 50000; i++) { const garbage = "ab" + "cd"; }
console.log(readScore(item));
console.log(readName(item));
`,
		expected: "42\nkeep-alive\n",
	})
}

func TestLinuxAMD64GenericClasses(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "generic_class_specialization",
		source: `
class Box<T> {
  value: T;
  constructor(v: T) { this.value = v; }
  get(): T { return this.value; }
}
class Stack<T> {
  items: T[];
  constructor() { this.items = []; }
  pushItem(item: T): void { this.items.push(item); }
  popItem(): T | undefined { return this.items.pop(); }
  size(): number { return this.items.length; }
}
const n = new Box<number>(42);
const s = new Box<string>("keep-" + "alive");
const stack = new Stack<number>();
stack.pushItem(10);
stack.pushItem(20);
for (let i = 0; i < 50000; i++) { const garbage = "ab" + "cd"; }
console.log(n.get());
console.log(s.get());
console.log(stack.size());
console.log(stack.popItem());
`,
		expected: "42\nkeep-alive\n2\n20\n",
	})
}

func TestLinuxAMD64Destructuring(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "array_object_tuple_destructuring",
		source: `
function point(): [number, string] { return [7, "seven"]; }
const nums = [10, 20];
const [a, b] = nums;
const cfg = { host: "localhost", port: 8080 };
const { host, port: p } = cfg;
const [id, label] = point();
console.log(a);
console.log(b);
console.log(host);
console.log(p);
console.log(id);
console.log(label);
`,
		expected: "10\n20\nlocalhost\n8080\n7\nseven\n",
	})
}

func TestLinuxAMD64DefaultsAndUndefined(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "default_optional_and_undefined_abi",
		source: `
function probe(a: number = 1, b?: string): void {
  console.log(a);
  console.log(b === undefined);
}
probe();
probe(2, "x");
console.log(undefined);
`,
		expected: "1\n1\n2\n0\nundefined\n",
	})
}

func TestLinuxAMD64NumericEnums(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "numeric_enum_constants",
		source: `
enum Status { Ok = 200, NotFound = 404 }
enum Direction { Up, Down, Left, Right }
function isUp(dir: Direction): boolean { return dir === Direction.Up; }
console.log(Status.Ok);
console.log(Status.NotFound);
console.log(Direction.Up);
console.log(Direction.Down);
console.log(Direction.Left);
console.log(Direction.Right);
console.log(isUp(Direction.Up));
console.log(isUp(Direction.Down));
`,
		expected: "200\n404\n0\n1\n2\n3\n1\n0\n",
	})
}

func TestLinuxAMD64TemplateInterpolation(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "template_interpolation_typed_coercion",
		source: "function render(name: string, age: number, active: boolean, title?: string): string {\n" +
			"  return `${name}:${age + 1}:${active}:${title}`;\n" +
			"}\n" +
			"console.log(render(\"alice\", 41, true, \"Dr.\"));\n" +
			"console.log(render(\"bob\", 9, false));\n" +
			"console.log(`value=${0.1}, square=${3 * 3}`);\n",
		expected: "alice:42:true:Dr.\nbob:10:false:undefined\nvalue=0.1, square=9\n",
	})
}

func TestLinuxAMD64RestParameters(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "typed_rest_parameter_array_packing",
		source: `
function sumAll(...nums: number[]): number {
  let total = 0;
  for (const n of nums) { total = total + n; }
  return total;
}
function formatList(prefix: string, ...items: string[]): string {
  let out = prefix;
  for (const item of items) { out = out + ":" + item; }
  return out;
}
console.log(sumAll(10, 20, 30));
console.log(sumAll());
console.log(formatList("items", "a", "b", "c"));
console.log(formatList("empty"));
`,
		expected: "60\n0\nitems:a:b:c\nempty\n",
	})
}

func TestLinuxAMD64ArraySpread(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "typed_array_spread",
		source: `
const a = [10, 20];
const b = [0, ...a, 30, 40];
for (const x of b) { console.log(x); }
const s1 = ["first", "second"];
const s2 = ["third"];
const combined = [...s1, ...s2];
for (const s of combined) { console.log(s); }
`,
		expected: "0\n10\n20\n30\n40\nfirst\nsecond\nthird\n",
	})
}

func TestLinuxAMD64ObjectSpread(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "closed_object_spread_override_order",
		source: `
const defaults = { host: "localhost", port: 8080, secure: false };
const custom = { port: 9000, secure: true };
const config = { ...defaults, ...custom, env: "prod" };
console.log(config.host);
console.log(config.port);
console.log(config.secure);
console.log(config.env);
`,
		expected: "localhost\n9000\n1\nprod\n",
	})
}

func TestLinuxAMD64NullishCoalescing(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "nullish_coalescing_preserves_falsy_values",
		source: `
function name(v: string | null): string { return v ?? "Anonymous"; }
function count(v: number | null): number { return v ?? 42; }
function flag(v: boolean | null): boolean { return v ?? true; }
console.log(name(null));
console.log(name("Alice"));
console.log(count(null));
console.log(count(0));
console.log(count(100));
console.log(flag(null));
console.log(flag(false));
console.log(flag(true));
`,
		expected: "Anonymous\nAlice\n42\n0\n100\n1\n0\n1\n",
	})
}

func TestLinuxAMD64OptionalChaining(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "nested_optional_chaining",
		source: `
type User = { name: string; profile?: { bio: string } };
function bio(user: User | null): string { return user?.profile?.bio ?? "none"; }
const full: User = { name: "Bob", profile: { bio: "Hello" } };
console.log(bio(full));
console.log(bio(null));
`,
		expected: "Hello\nnone\n",
	})
}

func TestLinuxAMD64OptionalProperties(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "optional_object_slots_and_union_properties",
		source: `
interface Config { host: string; port?: number; secure?: boolean; }
interface Circle { kind: string; radius: number; }
interface Square { kind: string; size: number; }
type Shape = Circle | Square;
const c1: Config = { host: "localhost", port: 8080, secure: true };
const c2: Config = { host: "remote" };
console.log(c1.host);
console.log(c1.port);
console.log(c1.secure);
console.log(c2.host);
console.log(c2.port);
console.log(c2.secure);
const s1: Shape = { kind: "circle", radius: 10 };
const s2: Shape = { kind: "square", size: 20 };
console.log(s1.kind);
console.log(s2.kind);
`,
		expected: "localhost\n8080\n1\nremote\nundefined\nundefined\ncircle\nsquare\n",
	})
}

func TestLinuxAMD64ComputedPropertyKeys(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "computed_property_keys_with_concrete_provenance",
		source: `
interface User { id: number; name: string; }
const u: User = { id: 101, name: "Alice" };
console.log(u["id"]);
console.log(u["name"]);
const k = "name";
const u2: any = u;
console.log(u2[k]);
u2["id"] = 202;
console.log(u2["id"]);
const headers = { "content-type": "application/json", accept: "text/html" };
console.log(headers["content-type"]);
console.log(headers["accept"]);
const arr = [10, 20, 30];
const idx: any = 1;
console.log(arr[idx]);
arr[idx] = 99;
console.log(arr[idx]);
`,
		expected: "101\nAlice\nAlice\n202\napplication/json\ntext/html\n20\n99\n",
	})
}

func TestLinuxAMD64JSValueArrays(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "jsvalue_any_union_arrays_gc",
		source: `
const values: any[] = [1, "two", true];
values[1] = "up" + "dated";
console.log(values.length);
console.log(values[0]);
console.log(values[1]);
console.log(values[2]);
const mixed: (number | string)[] = [1, "two"];
mixed[1] = 7;
console.log(mixed[0]);
console.log(mixed[1]);
const grown: any[] = [];
grown[200000] = "keep-" + "alive";
for (let i = 0; i < 50000; i++) { const garbage = "ab" + "cd"; }
console.log(grown[200000]);
`,
		expected: "3\n1\nupdated\ntrue\n1\n7\nkeep-alive\n",
	})
}

func TestLinuxAMD64EvolvingDynamicShapes(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "evolving_dynamic_object_growth_and_gc",
		source: `
const obj: any = {};
obj.a = 1;
obj.b = "keep-" + "alive";
obj.c = true;
obj.d = 4;
obj.e = 5;
obj.c = false;
for (let i = 0; i < 50000; i++) { const garbage = "ab" + "cd"; }
console.log(obj.a);
console.log(obj.b);
console.log(obj.c);
console.log(obj.e);
console.log(obj.missing);
`,
		expected: "1\nkeep-alive\nfalse\n5\nundefined\n",
	})
}

func TestLinuxAMD64MultiModuleRelativeImports(t *testing.T) {
	dir := t.TempDir()
	modules := filepath.Join(dir, "modules")
	if err := os.MkdirAll(modules, 0o755); err != nil {
		t.Fatal(err)
	}
	helper := `
export function multiply(a: number, b: number): number { return a * b; }
export function power(base: number, exp: number): number {
  let result = 1;
  for (let i = 0; i < exp; i++) { result = result * base; }
  return result;
}
export class Counter {
  count: number;
  constructor(initial: number) { this.count = initial; }
  increment(): number { this.count = this.count + 1; return this.count; }
  value(): number { return this.count; }
}
`
	main := `
import { multiply, power as pow, Counter } from "./modules/helper";
console.log(multiply(6, 7));
console.log(pow(2, 8));
const c = new Counter(10);
c.increment(); c.increment();
console.log(c.value());
`
	if err := os.WriteFile(filepath.Join(modules, "helper.ts"), []byte(helper), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.ts")
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(dir, "app")
	compiler := tspro.New(tspro.Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	if diags, err := compiler.CompileFile(mainPath, binPath); err != nil {
		t.Fatalf("multi-module compile failed: %v, diagnostics: %s", err, diags.Format(compiler.FileSet()))
	}
	assertLinuxAMD64ELF(t, binPath)
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Logf("linux/amd64 execution skipped on host %s/%s", runtime.GOOS, runtime.GOARCH)
		return
	}
	out, err := exec.Command(binPath).CombinedOutput()
	if err != nil {
		t.Fatalf("multi-module execution failed: %v\n%s", err, out)
	}
	if got, want := string(out), "42\n256\n12\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestLinuxAMD64MapSetCollections(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "map_set_growth_delete_clear_and_gc",
		source: `
const m = new Map<string, number>();
m.set("a", 1).set("b", 2).set("c", 3).set("d", 4).set("e", 5).set("f", 6);
console.log(m.size);
console.log(m.get("a"));
console.log(m.get("f"));
console.log(m.has("missing"));
console.log(m.delete("c"));
console.log(m.size);
const heapKey = "heap-" + "key";
m.set(heapKey, 99);
for (let i = 0; i < 50000; i = i + 1) { const garbage = "g" + "c"; }
console.log(m.has(heapKey));
console.log(m.get(heapKey));
m.clear();
console.log(m.size);
const s = new Set<number>();
s.add(1).add(2).add(3).add(4).add(5);
console.log(s.size);
console.log(s.has(5));
console.log(s.delete(2));
console.log(s.has(2));
const z = new Map<number, number>();
z.set(-0, 7);
console.log(z.has(0));
console.log(z.get(0));
const textKeys = new Map<string, number>();
textKeys.set("same-" + "key", 77);
console.log(textKeys.get("same-" + "key"));
`,
		expected: "6\n1\n6\n0\n1\n5\n1\n99\n0\n5\n1\n1\n0\n1\n7\n77\n",
	})
}

func TestLinuxAMD64DateUTC(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "date_now_numeric_constructor_leap_day_and_iso",
		source: `
function iso(ms: number): string { return new Date(ms).toISOString(); }
console.log(Date.now() > 0);
const epoch = new Date(0);
console.log(epoch.toISOString());
console.log(epoch.getUTCFullYear());
console.log(epoch.getUTCMonth());
console.log(epoch.getUTCDate());
console.log(epoch.getUTCHours());
console.log(epoch.getUTCMinutes());
console.log(epoch.getUTCSeconds());
const leap = new Date(1709210096789);
console.log(iso(1709210096789));
console.log(leap.getUTCFullYear());
console.log(leap.getUTCMonth());
console.log(leap.getUTCDate());
console.log(leap.getUTCHours());
console.log(leap.getUTCMinutes());
console.log(leap.getUTCSeconds());
`,
		expected: "1\n1970-01-01T00:00:00.000Z\n1970\n0\n1\n0\n0\n0\n2024-02-29T12:34:56.789Z\n2024\n1\n29\n12\n34\n56\n",
	})
}

func TestLinuxAMD64JSONAPI(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "json_static_shapes_and_runtime_scalars",
		source: `
function payload(count: number, key: string): string {
  return JSON.stringify({ count: count, key: key });
}
function parseRuntime(text: string): any { return JSON.parse(text); }
console.log(JSON.stringify({ message: "hello" }));
console.log(payload(5, "items"));
console.log(JSON.stringify([10, 20, 30]));
console.log(JSON.stringify(["a", "b", "c"]));
console.log(JSON.stringify([true, false]));
console.log(JSON.stringify(123.45));
console.log(JSON.stringify("pure-Go"));
console.log(JSON.stringify(true));
console.log(JSON.parse("123.45"));
console.log(JSON.parse("\"parsed string\""));
console.log(JSON.parse("true"));
console.log(JSON.parse("false"));
console.log(parseRuntime("-12.5"));
console.log(parseRuntime("true"));
console.log(JSON.stringify(JSON.parse("[1, 2, 3]")));
console.log(JSON.stringify(JSON.parse("{\"id\":42}")));
`,
		expected: "{\"message\":\"hello\"}\n{\"count\":5,\"key\":\"items\"}\n[10,20,30]\n[\"a\",\"b\",\"c\"]\n[true,false]\n123.45\n\"pure-Go\"\ntrue\n123.45\nparsed string\ntrue\nfalse\n-12.5\ntrue\n[1,2,3]\n{\"id\":42}\n",
	})
}

func TestLinuxAMD64RegExpAPI(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "regexp_literals_constructor_flags_and_division",
		source: `
const ci = new RegExp("world", "i");
console.log(ci.test("Hello World"));
console.log(ci.test("Hello Earth"));
console.log(ci.source);
const digits = /abc\d+/;
console.log(digits.test("xxabc1234yy"));
console.log(digits.test("abcdef"));
const prefix = new RegExp("^foo");
console.log(prefix.test("foobar"));
console.log(prefix.test("barfoo"));
const plain = new RegExp("cat");
console.log(plain.test("xxcatxx"));
console.log(8 / 2);
`,
		expected: "1\n0\nworld\n1\n0\n1\n0\n1\n4\n",
	})
}

func TestLinuxAMD64JSValuePrimitiveBoundaries(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "jsvalue_primitive_boundaries",
		source: `
function identityAny(value: any): any { return value; }
function asNumber(value: any): number { return value; }
let n: any = 41;
let s: any = "hello";
let b: any = true;
let z: any = null;
let u: any = undefined;
console.log(n);
console.log(s);
console.log(b);
console.log(z);
console.log(u);
let typedN: number = n;
let typedS: string = s;
let typedB: boolean = b;
console.log(typedN + 1);
console.log(typedS);
console.log(typedB);
n = "forty-two";
console.log(n);
console.log(identityAny(42));
console.log(identityAny("boxed"));
console.log(identityAny(false));
console.log(asNumber(42) + 1);
`,
		expected: "41\nhello\ntrue\nnull\nundefined\n42\nhello\n1\nforty-two\n42\nboxed\nfalse\n43\n",
	})
}

func TestLinuxAMD64JSValueArrayAndClosureBoundaries(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "jsvalue_array_closure_boundaries",
		source: `
function inc(value: number): number { return value + 1; }
function identityAny(value: any): any { return value; }
let arrayAny: any = [40, 2];
let closureAny: any = inc;
let returnedClosureAny: any = identityAny(inc);
let churn = "";
for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
let arrayNative: number[] = arrayAny;
let closureNative: (value: number) => number = closureAny;
let returnedClosureNative: (value: number) => number = returnedClosureAny;
console.log(arrayNative[0]! + arrayNative[1]!);
console.log(closureNative(41));
console.log(returnedClosureNative(41));
`,
		expected: "42\n42\n42\n",
	})
}

func TestLinuxAMD64JSValueClosedObjectProvenance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "jsvalue_closed_object_provenance",
		source: `
interface Child { value: number; }
interface Holder { count: number; label: string; active: boolean; child: Child; }
const typed: Holder = { count: 42, label: "before", active: true, child: { value: 7 } };
const alias: any = typed;
let churn = "";
for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
console.log(alias.count);
console.log(alias.label);
console.log(alias.active);
console.log(alias.child.value);
console.log(alias.missing);
const replacement: Child = { value: 77 };
alias.count = 99;
alias.label = "after";
alias.active = false;
alias.child = replacement;
console.log(typed.count);
console.log(typed.label);
console.log(typed.active);
console.log(typed.child.value);
`,
		expected: "42\nbefore\n1\n7\nundefined\n99\nafter\n0\n77\n",
	})
}

func TestLinuxAMD64DynamicAdditionCoercion(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "dynamic_addition_coercion",
		source: `
function add(left: any, right: any): any { return left + right; }
console.log(add(20, 22));
console.log(add("value=", 42));
console.log(add(true, 2));
console.log(add("bool=", true));
console.log(add(null, 2));
console.log(add(undefined, 2));
console.log(add("value=", null));
console.log(add("value=", undefined));
console.log(add({ value: 1 }, 2));
console.log(add("array=", [1, 2]));
console.log(add([1, 2], 3));
`,
		expected: "42\nvalue=42\n3\nbool=true\n2\nNaN\nvalue=null\nvalue=undefined\n[object Object]2\narray=1,2\n1,23\n",
	})
}
