package e2e_test

import (
	"strings"
	"fmt"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/backend/lower"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
	"github.com/phongsathornpt/ts-pro/internal/midend/irgen"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
	"github.com/phongsathornpt/ts-pro/internal/target"
)

func TestIRGenRemainingCorners(t *testing.T) {
	fs := source.NewFileSet()
	cases := []string{
		// 1. Tuple index assignment
		`function testTupleAssign() { const t: [number, string] = [1, "a"]; t[0] = 10; t[0] += 5; }`,
		// 2. Closed object computed string-key assignment with compound op
		`function testComputedObj() { const o = { x: 10 }; o["x"] += 5; }`,
		// 3. Any target with computed assignment
		`function testAnyComputed(obj: any) { obj["k"] = 1; }`,
		// 4. Array element compound assignment
		`function testArrayCompound(xs: number[]) { xs[0] += 10; }`,
		// 5. String concatenation compound assignment
		`function testStrCompound(s: string) { s += "world"; }`,
		// 6. Non-literal aggregate Promise.all with void return tasks
		`async function voidTask(): Promise<void> {} async function testVoidAll(): Promise<void> { await Promise.all([voidTask(), voidTask()]); }`,
		// Compound -=, *=, /= assignment
		`function testCompoundOps(x: number) { x -= 1; x *= 2; x /= 3; }`,
		// Try without catch or finally (syntax parsed directly or lowered)
		`function testTryDirect() { try { console.log("just try"); } }`,
		// Nullable union with boolean string coercion
		"function testNullableBool(b: boolean | null) { const s = `" + "b: ${b}`" + "; }",
		// Nullable union only nullish (fails coercion or handled)
		"function testOnlyNull(n: null | undefined) { const s = `" + "n: ${n}`" + "; }",
		// Class method on devirtualized instance
		`class BaseDev { m() { return 1; } } class SubDev extends BaseDev {} function testDev() { const s = new SubDev(); return s.m(); }`,
		// Arrow with expr body capturing unary, spread, assign, nested arrow
		`function testArrowExprAllCaptures(a: number, b: number): number {
			let x = a;
			const fn = (): number => -a + (x = b) + [...[a]][0];
			return fn();
		}`,
		// Unary plus operator
		`function testUnaryPlus(x: number): number { return +x; }`,
		// Binary ops in irgen: modulo, comparisons
		`function testBinaryAllOps(a: number, b: number): boolean {
			const m = a % b;
			const eq = a == b;
			const ne = a != b;
			const lt = a < b; const lt2 = b < a;
			const le = a <= b;
			const gt = a > b;
			const ge = a >= b;
			return eq && ne && lt && le && gt && ge && (m > 0);
		}`,
		// Thenable object literal with union then field (exercising rawType != fn and irFunctionMemberType)
		`type ThenFn = (res: (v: number) => void) => void;
		type ThenUnion = ThenFn | number;
		interface ThenObj { then: ThenUnion; }
		async function useThenUnionLit(obj: ThenObj): Promise<number> {
			return await Promise.resolve(obj);
		}`,
		// Console log with nullable union containing null
		`function testPrintNullable(x: number | null) { console.log(x); }`,
		// Error on adding property to proven closed shape through any
		`function testAddPropThroughAny(o: { a: number }) {
			const anyVal: any = o;
			anyVal.newProp = 10;
		}`,
		// Error on adding computed property to proven closed shape through any
		`function testAddComputedPropThroughAny(o: { a: number }) {
			const anyVal: any = o;
			anyVal["newProp"] = 10;
		}`,
		// Error on compound assignment through any alias
		`function testCompoundAnyProp(o: { a: number }) {
			const anyVal: any = o;
			anyVal.a += 1;
		}`,
		// Error on compound computed assignment through any alias
		`function testCompoundComputedAnyProp(o: { a: number }) {
			const anyVal: any = o;
			anyVal["a"] += 1;
		}`,
		// Error on dynamic compound assignment
		`function testDynCompound(dynObj: any) {
			dynObj.x += 1;
			dynObj["x"] += 1;
		}`,
		// Access non-existent property on concrete typed object
		`function testMissingPropConcreteAny(obj: any) {
			const m = (({ a: 1 }) as any).missingField;
		}`,
		// Tuple length and RegExp source property
		`function testTupleAndRegExpProps(tup: [number, string], r: RegExp) {
			const tLen = tup.length;
			const rSrc = r.source;
		}`,
		// RegExp patterns: anchored, digit, unsupported meta
		`function testRegExpPatterns() {
			const r1 = new RegExp("^abc");
			const r2 = new RegExp("prefix\\d+");
			const r3 = new RegExp("plain", "i");
			const r4 = new RegExp("^bad*pattern");
			const r5 = new RegExp("bad\\d+*pattern");
			const r6 = new RegExp("unsupported*meta");
			const r7 = new RegExp("pattern", "g");
			const r8 = new RegExp("\\d+");
		}`,
		// Binary ops with any/unknown (js_add, js_sub/mul/div/mod, js_eq)
		`function testAnyBinary(a: any, b: number) {
			const add = a + b; const add2 = b + a;
			const sub = a - b;
			const mul = a * b;
			const div = a / b;
			const mod = a % b;
			const eq = a == b;
			const seq = a === b;
			const ne = a != b;
			const sne = a !== b;
		}`,
		// Provenance assignment through any for Object and Function
		`function targetFn() {}
		function testAssignAnyProv(o: { a: number }) {
			let a1: any;
			a1 = o;
			let a2: any;
			a2 = targetFn;
		}`,
		// Prefix ++ and --
		`function testPrefixInc(x: number) {
			let a = x;
			const p1 = ++a;
			const p2 = --a;
		}`,
		// Call function with omitted params without default or optional (break loop)
		`function fnFewParams(a: number, b: number, c: number) {}
		function testFewParams() {
			fnFewParams(1, 2, 3);
		}`,
		// Array.push with boxed element
		`function testPushAny(xs: any[]) {
			xs.push(123);
		}`,
		// Bitwise & in binary expr
		`function testBitwiseAnd(a: number, b: number): number { return a & b; }`,
		// Relational operators on any
		`function testAnyRelational(a: any, b: number) {
			const lt = a < b; const lt2 = b < a;
			const le = a <= b;
			const gt = a > b;
			const ge = a >= b;
		}`,
		// Computed property indexing on dynamic object and missing on concrete any
		`function testComputedAnyIndexing(o: { a: number }, dyn: any) {
			const anyVal: any = o;
			const m = anyVal["missing"];
			const d = dyn["dynamicKey"];
		}`,
		// Closed object computed string-key assignment with = (not compound)
		`function testComputedAssignEq(o: { x: number }) {
			o["x"] = 20;
		}`,
		// Non-any object property access with fallback
		`function testNonAnyFallback(x: number) {
			const p = (x as any).prop;
		}`,
		// Nested generic calls to exercise g.typeBindings > 0
		`function innerGen<T>(val: T): T { return val; }
		function outerGen<U>(u: U): U {
			return innerGen<U>(u);
		}
		function testNestedGen(): number {
			return outerGen<number>(42);
		}`,
		// While, do-while, for, for-of, switch with modified variables inside
		`function testLoopModifiedVars(cond: boolean, xs: number[]) {
			let a = 0;
			while (cond) { a++; }
			do { a++; } while (cond);
			for (let i = 0; cond; i++) { a++; }
			for (const x of xs) { a++; }
			switch (a) {
			case 0:
				a++;
				break;
			}
		}`,
		// Derived class with constructor missing super(...)
		`class BaseSuper { constructor() {} }
		class DerivedNoSuper extends BaseSuper {
			constructor() { console.log("missing super"); }
		}`,
		// Dynamic object conversion error to class and dynamic object spread
		`class TargetClass { x: number; }
		function testDynamicToClass(dyn: any) {
			const c: TargetClass = dyn;
		}`,
		`function testDynamicSpread() {
			const o = { a: 1 };
			const dyn: any = { ...o, b: 2 };
		}`,
		// directCalleeForExpr with import alias and local non-callee
		`function targetDirect() {}
		import { targetDirect as importedDirect } from "./mod";
		function testDirectCallee() {
			let localFn = () => {};
			let a: any;
			a = importedDirect;
			a = targetDirect;
			a = localFn;
		}`,
		// Break and continue outside loops / switch (triggers error in lowerStatement)
		`function testBreakOutside() { break; }`,
		`function testContinueOutside() { continue; }`,
		// Loops with body modifying vars for findModifiedVars
		`function testWhileBodyMod(cond: boolean) {
			let x = 0;
			while (cond) {
				x++;
			}
		}`,
		`function testDoWhileBodyMod(cond: boolean) {
			let y = 0;
			do {
				y++;
			} while (cond);
		}`,
		`function testForOfBodyMod(arr: number[]) {
			let z = 0;
			for (const item of arr) {
				z += item;
			}
		}`,
		// Promise.race with void task
		`async function raceVoidTask(): Promise<void> {}
		async function testRaceVoid(): Promise<void> {
			await Promise.race([raceVoidTask()]);
		}`,
		// Shadowing loop variable in for-of and for loop without post or with post jump
		`function testForOfShadow(xs: number[]): number {
			let item = 10;
			for (const item of xs) {
				console.log(item);
			}
			return item;
		}`,
		`function testForPostJump(): number {
			let count = 0;
			for (let i = 0; i < 5; i += 1) {
				count += i;
			}
			return count;
		}`,
		`function testDoWhileCondMod(): void {
			let j = 0;
			do {
			} while ((j += 1) < 10);
		}`,
		// For loop without condition (cond == nil)
		`function testForNoCond(): number {
			let count = 0;
			for (let i = 0; ; i++) {
				if (i >= 5) break;
				count += i;
			}
			return count;
		}`,
		// Function with rest param of type any[]
		`function fnRestAny(...args: any[]) {}
		function testRestAny() {
			fnRestAny(1, "str", true);
		}`,
		// Spread into any[] to trigger irJSValueType(dstElemType)
		`function testSpreadIntoAny(xs: number[]): any[] {
			const arr: any[] = [...xs];
			return arr;
		}`,
		// Assign any variable with proven object to typed object variable
		`function testProvenUnbox(o: { a: number }) {
			const anyVal: any = o;
			const typedVal: { a: number } = anyVal;
		}`,
		// Assign non-any, non-dynamic object to object variable across JS boundary
		`function testCoerceJSBoundary(u: { a: number } | number) {
			const o: { a: number } = u as any;
		}`,
		// Generic class specialization called twice to hit emittedClassSpecs[info.Name]
		`class SpecBox<T> { x: T; }
		function testSpecBoxTwice() {
			const b1 = new SpecBox<number>();
			const b2 = new SpecBox<number>();
		}`,
		// Logical and / or binary lowering
		`function testLogicalAndOr(a: boolean, b: boolean): boolean { return (a && b) || (a || b); }`,
		// While loop with condition true
		`function testWhileTrue() { while (true) { break; } }`,
		// Console log with undefined, boolean, string
		`function testPrintPrimitives() { console.log(undefined); console.log("msg"); console.log(false); }`,
		// JSON constants and stringify of number, boolean, null, array
		`function testJSONConsts() {
			const n = JSON.parse("null");
			const b = JSON.parse("true");
			const s = JSON.parse("\"str\"");
			const arr = JSON.parse("[1, \"two\", null, true]");
			const objComplex = JSON.parse("{\"a\": null, \"b\": true, \"c\": \"str\", \"d\": 123}");
			const obj = JSON.parse("{\"x\": 1}");
			const str1 = JSON.stringify(10);
			const str2 = JSON.stringify(true);
			const str3 = JSON.stringify([1, 2]);
			const str4 = JSON.stringify([]);
			const str5 = JSON.stringify("hello");
			const str6 = JSON.stringify(null);
			const objWithNull = { x: null };
			const str7 = JSON.stringify(objWithNull);
		}`,
		// For-of with inner modified vars
		`function testForOfMod(xs: number[]) { let acc = 0; for (const x of xs) { acc += x; } return acc; }`,
		// Arrow with block body capturing vars across all AST statements and expressions
		`function testArrowBlockAllCaptures(a: number, b: number): number {
			const fn = (): number => {
				let local1 = a;
				for (const item of [b]) {
					local1 += item;
				}
				for (let i = 0; i < a; i += 1) {
					local1 += i;
				}
				if (a > 0) {
					local1 = b;
				} else {
					local1 = -b;
				}
				while (local1 < 10) {
					local1 += a;
				}
				do {
					local1 += b;
				} while (local1 < 20);
				switch (local1) {
				case a:
					local1 = 1;
					break;
				default:
					local1 = 2;
				}
				function nested() {}
				const arr = [a, ...[b]];
				const obj = { p: a };
				const val = (a ? b : 0);
				local1 = arr[0] + obj.p + val;
				return local1;
			};
			return fn();
		}`,
	}

	for i, src := range cases {
		f := fs.AddFile(fmt.Sprintf("corner_%d.ts", i), []byte(src))
		p := parser.New(f)
		prog, diags := p.Parse()
		if !diags.HasErrors() {
			res := sema.Check(prog)
			if !res.Diagnostics.HasErrors() {
				irProg, err := irgen.Generate(prog, res)
				if err == nil {
					_, _ = lower.Lower(irProg, lower.ArchAMD64)
				}
			}
		}
	}

	// 7. Lowering corner cases: isNumberType on unions without numbers
	_ = types.NewUnion(types.TypeString, types.TypeNull)
	_ = types.NewUnion(types.TypeString, types.TypeUndefined)

	// Lowering arch unsupported and target unsupported
	_, _ = lower.Lower(&ir.Program{}, lower.Arch("invalid_arch"))
	_, _ = lower.LowerTarget(&ir.Program{}, target.Target{OS: "invalid_os", Arch: "invalid_arch"})
}

func TestIRGenTemplateCoercionAndExceptions(t *testing.T) {
	fs := source.NewFileSet()
	cases := []string{
		// 1. Template coercion with null and undefined
		"function testTplNull(n: null, u: undefined) { const s = `a${n}b${u}c`; }",
		// 2. Nullable union string coercion with null and undefined
		"function testNullableUnion(val: number | null | undefined) { const s = `val: ${val}`; }",
		// 3. Nested throw inside finally
		`function testFinallyThrow() { try { throw 1; } finally { throw 2; } }`,
		// 4. Try without catch (only finally) throwing
		`function testTryFinThrow() { try { throw 1; } finally { console.log("fin"); } }`,
		// 5. Method virtual dispatch candidate sorting with hierarchy
		`class Animal { speak(): string { return "sound"; } }
		 class Dog extends Animal { override speak(): string { return "woof"; } }
		 class Cat extends Animal { override speak(): string { return "meow"; } }
		 function testPoly(a: Animal) { return a.speak(); }`,
	}

	for i, src := range cases {
		f := fs.AddFile(fmt.Sprintf("corner_m_%d.ts", i), []byte(src))
		p := parser.New(f)
		prog, diags := p.Parse()
		if !diags.HasErrors() {
			res := sema.Check(prog)
			if !res.Diagnostics.HasErrors() {
				irProg, err := irgen.Generate(prog, res)
				if err == nil {
					_, _ = lower.Lower(irProg, lower.ArchAMD64)
				}
			}
		}
	}

	thenCases := []string{
		// Thenable class with then(resolve, reject) method awaited
		`class MyThenable {
			then(resolve: (v: number) => void, reject: (e: any) => void): void {}
		}
		async function useThenable(): Promise<number> { const t = new MyThenable(); return await t; }`,
		// Thenable object literal with then field
		`async function useThenableLit(): Promise<string> {
			const obj = { then: (res: (v: string) => void) => { res("done"); } };
			return await obj;
		}`,
		// Promise.all with thenable and task mixed
		`async function numTask(): Promise<number> { return 1; }
		class Th {
			then(resolve: (v: number) => void, reject: (e: any) => void): void {}
		}
		async function mixedAll(): Promise<void> { await Promise.all([numTask(), new Th()]); }`,
		// Promise.race with thenable
		`class Th2 {
			then(resolve: (v: string) => void, reject: (e: any) => void): void {}
		}
		async function raceThenable(): Promise<void> { await Promise.race([new Th2()]); }`,
		// Promise.all with array variable of tasks
		`async function t1(): Promise<number> { return 1; }
		async function allArray(): Promise<void> {
			const arr: Promise<number>[] = [t1(), t1()];
			await Promise.all(arr);
		}`,
		// Promise.all with array variable of union task results
		`async function t2(): Promise<number> { return 1; }
		async function t3(): Promise<string> { return "x"; }
		function makeUnionArr(): (Promise<number> | Promise<string>)[] { return [t2(), t3()]; }
		async function allUnionArr(): Promise<void> {
			const arr = makeUnionArr();
			await Promise.all(arr);
		}`,
		// Promise.resolve with thenable argument
		`class Th3 {
			then(resolve: (v: number) => void, reject: (e: any) => void): void {}
		}
		async function resolveThenable(): Promise<void> { await Promise.resolve(new Th3()); }`,
		// Tuple with >64 ref elements is an error path; plain big tuple
		`function bigTuple(): [number, string, number, string] { return [1, "a", 2, "b"]; }`,
		// Union of string|number|boolean in template (multi-representation error)
		"function tplUnion(v: number | string | boolean) { const s = `v=${v}`; }",
		// Promise.all with immediate primitive scalar inputs
		`async function allImmediates(): Promise<void> { await Promise.all([1, "two", true]); }`,
		// Promise.race with immediate primitive scalar inputs
		`async function raceImmediates(): Promise<void> { await Promise.race([1, 2]); }`,
		// Promise.resolve with immediate scalar
		`async function resolveScalar(): Promise<void> { await Promise.resolve(42); }`,
		// Promise.reject with scalar
		`async function rejectScalar(): Promise<void> { await Promise.reject("err"); }`,
		// Promise.all with array variable of union including non-promise
		`async function allUnionNonPromise(): Promise<void> {
			const arr: (number | string)[] = [1, "two"];
			await Promise.all(arr);
		}`,
		// Promise.race with array variable of tasks
		`async function r1(): Promise<number> { return 1; }
		async function raceArr(): Promise<void> {
			const arr: Promise<number>[] = [r1(), r1()];
			await Promise.race(arr);
		}`,
	}
	for i, src := range thenCases {
		f := fs.AddFile(fmt.Sprintf("corner_n_%d.ts", i), []byte(src))
		p := parser.New(f)
		prog, diags := p.Parse()
		if !diags.HasErrors() {
			res := sema.Check(prog)
			if !res.Diagnostics.HasErrors() {
				irProg, err := irgen.Generate(prog, res)
				if err == nil {
					_, _ = lower.Lower(irProg, lower.ArchAMD64)
				}
			}
		}
	}
}

















func TestE2EIRGenErrorBranches(t *testing.T) {
	fs := source.NewFileSet()
	// 1. Switch fallthrough error in e2e
	switchSrc := `function testFallthrough(x: number): number {
	switch (x) {
	case 1:
		console.log(1);
	case 2:
		return 2;
	}
	return 0;
}`
	fSw := fs.AddFile("sw_fallthrough.ts", []byte(switchSrc))
	pSw := parser.New(fSw)
	progSw, _ := pSw.Parse()
	resSw := sema.Check(progSw)
	_, _ = irgen.Generate(progSw, resSw)

	// 2. 65 object reference fields overflow in e2e
	var sb strings.Builder
	sb.WriteString("const obj = {\n")
	for i := 0; i < 65; i++ {
		sb.WriteString(fmt.Sprintf("  f%02d: \"v%02d\",\n", i, i))
	}
	sb.WriteString("};\nconsole.log(obj.f00);\n")
	fHuge := fs.AddFile("huge.ts", []byte(sb.String()))
	pHuge := parser.New(fHuge)
	progHuge, _ := pHuge.Parse()
	resHuge := sema.Check(progHuge)
	_, _ = irgen.Generate(progHuge, resHuge)
}
