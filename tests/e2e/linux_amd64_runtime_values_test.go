package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/phongsathornpt/ts-pro/pkg/tspro"
)

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
		expected: "6\n1\n6\nfalse\ntrue\n5\ntrue\n99\n0\n5\ntrue\ntrue\nfalse\ntrue\n7\n77\n",
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
		expected: "true\n1970-01-01T00:00:00.000Z\n1970\n0\n1\n0\n0\n0\n2024-02-29T12:34:56.789Z\n2024\n1\n29\n12\n34\n56\n",
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
		expected: "true\nfalse\nworld\ntrue\nfalse\ntrue\nfalse\ntrue\n4\n",
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
		expected: "41\nhello\ntrue\nnull\nundefined\n42\nhello\ntrue\nforty-two\n42\nboxed\nfalse\n43\n",
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
		expected: "42\nbefore\ntrue\n7\nundefined\n99\nafter\nfalse\n77\n",
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

func TestLinuxAMD64DynamicNumericCoercion(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "dynamic_numeric_coercion",
		source: `
function sub(a: any, b: any): number { return a - b; }
function mul(a: any, b: any): number { return a * b; }
function div(a: any, b: any): number { return a / b; }
function mod(a: any, b: any): number { return a % b; }
console.log(sub("6", 1));
console.log(mul("6", 7));
console.log(div(84, 7));
console.log(mod("7", 4));
console.log(sub([5], 2));
console.log(sub(true, false));
console.log(sub(null, 2));
console.log(sub(undefined, 2));
`,
		expected: "5\n42\n12\n3\n3\n1\n-2\nNaN\n",
	})
}

func TestLinuxAMD64DynamicEquality(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "dynamic_strict_and_loose_equality",
		source: `
function loose(a: any, b: any): any { return a == b; }
function strict(a: any, b: any): any { return a === b; }
function looseNot(a: any, b: any): any { return a != b; }
function strictNot(a: any, b: any): any { return a !== b; }
console.log(loose("6", 6));
console.log(strict("6", 6));
console.log(looseNot("6", 6));
console.log(strictNot("6", 6));
console.log(loose(null, undefined));
console.log(strict(null, undefined));
console.log(loose(true, 1));
let nan: any = 0 / 0;
console.log(strict(nan, nan));
console.log(strict(0, -0));
let s1: any = "same" + "";
let s2: any = "sa" + "me";
console.log(strict(s1, s2));
const shared = { value: 42 };
let oa: any = shared;
let ob: any = shared;
console.log(strict(oa, ob));
console.log(loose({ value: 1 }, "[object Object]"));
console.log(loose([5], 5));
`,
		expected: "true\nfalse\nfalse\ntrue\ntrue\nfalse\ntrue\nfalse\ntrue\ntrue\ntrue\ntrue\ntrue\n",
	})
}

func TestLinuxAMD64DynamicRelationalComparison(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "dynamic_relational_comparison",
		source: `
function less(a: any, b: any): any { return a < b; }
function lessEq(a: any, b: any): any { return a <= b; }
function greater(a: any, b: any): any { return a > b; }
function greaterEq(a: any, b: any): any { return a >= b; }
console.log(less("6", 7));
console.log(less("10", "2"));
console.log(lessEq("6", 6));
console.log(greater(7, "6"));
console.log(greaterEq(7, 7));
console.log(less(undefined, 1));
console.log(lessEq(null, 0));
console.log(greater([5], 4));
`,
		expected: "true\ntrue\ntrue\ntrue\ntrue\nfalse\ntrue\ntrue\n",
	})
}

func TestLinuxAMD64JSValueStructuralObjectUnbox(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "jsvalue_structural_object_unbox",
		source: `
interface Child { value: number; }
interface Holder { value: number; label: string; child: Child; }
let dynamic: any = { value: 42, label: "materialized", child: { value: 7 } };
let churn = "";
for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
let typed: Holder = dynamic;
console.log(typed.value);
console.log(typed.label);
console.log(typed.child.value);
`,
		expected: "42\nmaterialized\n7\n",
	})
}

func TestLinuxAMD64DynamicCallableProvenance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "dynamic_callable_provenance",
		source: `
interface Box { value: number; }
function addOne(value: number): number { return value + 1; }
function readBox(box: Box): number { return box.value; }
const addAny: any = addOne;
const readAny: any = readBox;
const box: Box = { value: 42 };
const offset = 5;
const captured: any = (value: number): number => value + offset;
console.log(addAny(41));
console.log(readAny(box));
console.log(captured(37));
`,
		expected: "42\n42\n42\n",
	})
}

func TestLinuxAMD64DynamicClassMethodThis(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "dynamic_class_method_this",
		source: `
class Counter {
  value: number;
  constructor(value: number) { this.value = value; }
  add(delta: number): number { this.value = this.value + delta; return this.value; }
}
const dynamicCounter: any = new Counter(40);
console.log(dynamicCounter.add(2));
let churn = "";
for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
console.log(dynamicCounter.add(8));
`,
		expected: "42\n50\n",
	})
}

func TestLinuxAMD64DynamicStructuralFunctionThis(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "dynamic_structural_function_this",
		source: `
interface Box {
  value: number;
  add: (this: Box, delta: number) => number;
  plain: (delta: number) => number;
}
const box: Box = {
  value: 40,
  add: function (this: Box, delta: number): number { return this.value + delta; },
  plain: (delta: number): number => 40 + delta,
};
const dynamicBox: any = box;
let churn = "";
for (let i = 0; i < 50000; i = i + 1) { churn = "ab" + "cd"; }
console.log(dynamicBox.add(2));
console.log(dynamicBox.plain(2));
`,
		expected: "42\n42\n",
	})
}
