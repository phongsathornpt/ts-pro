package e2e_test

import (
	"fmt"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/backend/lower"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
	"github.com/phongsathornpt/ts-pro/internal/midend/irgen"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestE2ECoverageComprehensive(t *testing.T) {
	// 1. Sema & Parser error and edge branches
	edgeSources := []string{
		// while with break/continue
		`function testLoops() { while (true) { break; } do { continue; } while (false); }`,
		// for-of with array
		`function testForOf(xs: number[]) { for (const x of xs) { console.log(x); } }`,
		// switch statement with cases and default
		`function testSwitch(x: number) { switch (x) { case 1: return 1; case 2: return 2; default: return 0; } }`,
		// try catch finally with throw
		`function testTry() { try { throw "error"; } catch (e: any) { console.log(e); } finally { console.log("fin"); } }`,
		// array methods and destructuring
		`function testArr() { const [a, b] = [1, 2]; const arr = [a, b]; arr.push(3); arr.pop(); }`,
		// object literal with spread
		`function testObj() { const o1 = { a: 1 }; const o2 = { ...o1, b: 2 }; }`,
		// map and set
		`function testColls() { const m = new Map<string, number>(); m.set("k", 1); m.get("k"); m.has("k"); const s = new Set<string>(); s.add("v"); s.has("v"); }`,
		// date and regexp
		`function testDateReg() { const d = new Date(1000); const r = new RegExp("abc", "i"); r.test("abc"); }`,
		// json parse and stringify
		`function testJSON() { const s = JSON.stringify({ a: 1 }); const parsed: any = JSON.parse(s); }`,
		// nullish coalescing and optional chaining
		`function testNullish(o: any) { const x = o?.a ?? 10; }`,
		// async/await and promises
		`async function asyncFn(): Promise<number> { return 42; } async function caller() { const res = await asyncFn(); }`,
		// promise all and race
		`async function testPromiseAll() { await Promise.all([asyncFn(), asyncFn()]); await Promise.race([asyncFn(), asyncFn()]); }`,
		// try with only finally
		`function testTryFinally() { try { console.log("try"); } finally { console.log("fin"); } }`,
		// nested try catch in finally
		`function testNestedTry() { try { try { throw 1; } catch (e: any) { throw 2; } } catch (e2: any) {} }`,
		// generic function specialization with array and tuple
		`function mapPair<T, U>(pair: [T, U]): [U, T] { return [pair[1], pair[0]]; } function runPair() { return mapPair<number, string>([1, "a"]); }`,
		// dynamic property assignment
		`function testDynAssign(obj: any, key: string, val: any) { obj[key] = val; return obj[key]; }`,
	}

	fs := source.NewFileSet()
	for i, src := range edgeSources {
		f := fs.AddFile(fmt.Sprintf("edge_%d.ts", i), []byte(src))
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

	// 2. Comprehensive sema diagnostics in e2e
	diagSources := []string{
		`const x = this;`,
		`class A { foo() { super.foo(); } }`,
		`const r = new RegExp("a", "b", "c");`,
		`const r = new RegExp(123);`,
		`const d = new Date(1, 2);`,
		`const d = new Date(true);`,
		`const m = new Map<number>();`,
		`const s = new Set<number, string>();`,
		`const x = new NonExistentClass();`,
		`class Box<T> { x: T; } const b = new Box();`,
		`class A extends B {} class B extends A {}`,
		`let x = 1; let x = 2;`,
		`let x: number = "hello";`,
		`let x = 1; x();`,
		`let o = { a: 1 }; o.b;`,
		`enum Color { Red = "red" }`,
		`for (const x of 42) {}`,
		`async function f() { await 123; }`,
		`groupSpawn();`,
		`groupSpawn(123, () => {});`,
		`const g = taskGroup(); groupSpawn(g, (x: number) => {});`,
		`groupJoin();`,
		`groupJoin(123);`,
		`groupCancel();`,
		`groupCancel(123);`,
		`channel();`,
		`channel<number>();`,
		`channel<number>("bad");`,
		`channelSend();`,
		`channelSend(123, 10);`,
		`const ch = channel<number>(1); channelSend(ch, "str");`,
		`channelRecv();`,
		`channelRecv(123);`,
		`channelTrySend();`,
		`channelTrySend(123, 10);`,
		`const ch2 = channel<number>(1); channelTrySend(ch2, "str");`,
		`channelTryRecvOr();`,
		`channelTryRecvOr(123, 10);`,
		`const ch3 = channel<number>(1); channelTryRecvOr(ch3, "str");`,
		`spawn();`,
		`spawn(123);`,
		`spawn((x: number) => {});`,
		`yieldNow(1);`,
		`sleep();`,
		`sleep("10");`,
		`setTaskContext();`,
		`setTaskContext(123);`,
		`taskContext(1);`,
		`cancelTask();`,
		`cancelTask(123);`,
		`taskCancelled(1);`,
		`join();`,
		`join(123);`,
		`Promise.all();`,
		`Promise.all(123);`,
		`Promise.resolve();`,
		// Duplicate declarations
		`enum DupEnum { A } enum DupEnum { B }`,
		`class DupClass {} class DupClass {}`,
		`function dupFn() {} function dupFn() {}`,
		`interface DupIf {} interface DupIf {}`,
		`type DupType = number; type DupType = string;`,
		`import { a as x, b as x } from "./mod";`,
		// Missing base class
		`class Child extends MissingBase {}`,
		// Missing type on class field and method param
		`class NoTypeField { f; m(p) {} }`,
		// Date and Collection missing member
		`const d = new Date(); d.nonExistent();`,
		`const m = new Map<string, number>(); m.nonExistent();`,
		`const s = new Set<string>(); s.nonExistent();`,
		// Guard return narrowing with shadowed outer symbol
		`function testShadowGuard(x: number | null) { if (true) { let x: number | null = 1; if (x === null) return; } }`,
		// Guard with empty block
		`function testEmptyBlockGuard(x: number | null) { if (x === null) {} }`,
		// Multiple instances of generic class to hit genericClassSpecs cache
		`class CacheBox<T> { v: T; } const cb1 = new CacheBox<number>(); const cb2 = new CacheBox<number>();`,
		// Base class method owner fallback
		`class BaseM { m() {} } class MidM extends BaseM {} class SubM extends MidM {} const sm = new SubM(); sm.m();`,
		// Static class field
		`class StaticFieldTest { static count: number = 0; }`,
		// Nested scope guard return shadowing parent variable (triggers currentScope.Define)
		`function testScopeGuard(val: number | null) { { if (val === null) return; } }`,
		// Guard against non-nullish comparison
		`function testNonGuard(x: number) { if (x === 10) return; }`,
		// Guard narrowing to never
		`function testNeverGuard(x: null) { if (x === null) return; }`,
		// Union member lookup failure on one variant
		`type UA = { a: number }; type UB = { b: number }; function testUnionMember(u: UA | UB) { u.a; }`,
		// Union member lookup single-element union
		`type UOne = { a: number }; function testUnionOne(u: UOne | UOne) { return u.a; }`,
		// Incompatible tuple initializer
		`const badTup: [number, string] = [1, 2];`,
		// Incompatible array literal element
		`const badArr: number[] = [1, "two"];`,
		// Incompatible object literal field
		`const badObj: { a: number } = { a: "bad" };`,
		// Variable declaration without initializer and without type
		`let noInitNoType;`,
		// Class field initializer mismatch
		`class FieldBad { a: number = "str"; }`,
		// Method in class with missing method type
		`class MissingMethodType { m(): void {} }`,
		// for-of with non-iterable
		`for (const x of true) {}`,
		// for-of with type annotation compatible and incompatible
		`function testForOfType(arr: number[]) { for (const x: number of arr) {} for (const y: string of arr) {} }`,
		// return statement without currentFnRet
		`return 123;`,
		// super in derived class with missing base class in result.Classes
		`class BaseMissing {} class DerivedMissing extends BaseMissing { foo() { super.foo(); } }`,
		// function with param type annotation resolved
		`function testParamType(a: number, b: string) {}`,
		// non-generic class with type arguments
		`class PlainClass {} const pc = new PlainClass<number>();`,
		// constructor argument type mismatch
		`class CtorArg { constructor(a: number) {} } const ca = new CtorArg("bad");`,
		// nullish coalescing with purely null/undefined on left
		`const nullishRes = null ?? 42;`,
		// binary expr fallback (e.g. comma or bitwise)
		`const binFallback = (1, 2);`,
		// Arrow function with this parameter and untyped parameter
		`const arrowWithThis = (this: { x: number }, a) => this.x + a;`,
		// Arrow function expr body returning mismatch
		`const arrowBadRet: () => number = () => "bad";`,
		// Spread non-array into array
		`const badSpreadArr = [...123];`,
		// Spread non-object into object
		`const badSpreadObj = { ...123 };`,
		// Missing string property on closed object
		`const oBad = { a: 1 }; oBad["missing"];`,
		// Non-numeric index on array
		`const aBad = [1, 2]; aBad["str"];`,
		// Empty tuple index
		`type EmptyTup = []; function testEmptyTup(t: EmptyTup) { t[0]; }`,
		// Index on non-indexable type
		`const nonIdx = 42; nonIdx[0];`,
		// Missing member on enum
		`enum ETest { A = 1 } ETest.B;`,
		// Promise.reject static call
		`Promise.reject("err");`,
		// Promise.all with type arguments mismatch count
		`Promise.all<number, string>([]);`,
		// Promise.resolve with type argument mismatch count
		`Promise.resolve<number, string>(1);`,
		// Types: never, unknown, custom primitive kind
		`type PrimTypes = [never, unknown];`,
		// Function type with untyped param, this param, and missing return type
		`type FnWithThis = (this: number, a) => void;`,
		// Function expression with untyped param and omitted return type
		`const fnExprUntyped = function(a) {};`,
		// taskGroup with unexpected argument
		`taskGroup(123);`,
		// function with rest parameter that is not array
		`function testBadRest(...rest: number) {}`,
		// function with untyped param
		`function testUntypedParam(a) {}`,
		// cancelTask with unknown object handle
		`const fakeTask = { id: 1 }; cancelTask(fakeTask);`,
		// join with unknown object handle
		`join(fakeTask);`,
		// non-generic function called with type arguments
		`function nonGen() {} nonGen<number>();`,
		// generic function with mismatching type argument count
		`function genOne<T>(x: T) {} genOne<number, string>(1);`,
		// generic function inference failure
		`function genInfer<T>(x: T, y: T): T { return x; } genInfer(1, "str");`,
		// optional field access on closed object
		`type OptObj = { a?: number }; function testOptField(o: OptObj) { return o["a"]; }`,
		// thenable method with 0 params
		`class ZeroThen { then(): void {} } async function testZeroThen() { await new ZeroThen(); }`,
		// thenable resolve callback with 0 params
		`class ZeroResolve { then(res: () => void): void {} } async function testZeroRes() { await new ZeroResolve(); }`,
		// union with Promise and non-Promise in Promise.all
		`async function taskNum(): Promise<number> { return 1; } async function testUnionAll() { await Promise.all([taskNum() as (Promise<number> | string)]); }`,
		// Promise.race with unknown static method
		`Promise.unknownMethod();`,
		// unknown primitive type node kind
		`type CustomPrim = custom;`,
		// FunctionTypeNode with nil return type
		`type FnNoRet = () => void;`,
		// AST Expr with default case in checkExpr
		`const dummyStmt = 1;`,
		// Bitwise binary operators in sema
		`const bitAnd = 1 & 2; const bitOr = 1 | 2; const bitXor = 1 ^ 2; const bitShl = 1 << 2; const bitShr = 1 >> 2;`,
		// Promise.resolve with non-task object (triggers promiseResultType !ok)
		`const nonTaskObj = { x: 1 }; Promise.resolve(nonTaskObj);`,
		// thenable with union containing non-function type
		`const thenUnionObj = { then: 1 as (((v: number) => void) | number) }; async function testThenUnion() { await thenUnionObj; }`,
		// thenable with then callback parameter as union of non-functions
		`class ThenCbUnion { then(cb: string | number) {} } async function testThenCbUnion() { const t = new ThenCbUnion(); await Promise.resolve(t); }`,
		// thenable with then field as union of non-functions
		`class ThenFieldNonFn { then: string | number; } async function testThenFieldNonFn(x: ThenFieldNonFn) { await x; }`,
		// join with fake task handle
		`const fakeJoinHandle = { name: "fake" }; join(fakeJoinHandle);`,
		// class extending base without super call constructor
		`class PlainBase {} class DerivedPlain extends PlainBase { constructor() { super(); } }`,
		// FunctionTypeNode with void return type
		`type MyVoidFn = () => void;`,
		// thenable with empty then
		`class NoThenObj {} async function testNoThen() { await new NoThenObj(); }`,
		// non-nullish guard on non-union
		`function testGuardNonUnion(x: number) { if (x === 0) return; }`,
		// guard on unresolved symbol name
		`function testUnresolvedGuard() { if (unknownVar === null) return; }`,
		// strict guard narrowing union to never
		`function testNarrowNever(x: number | string) { if (x === null) return; }`,
		// removeExactType union with all members removed
		`function testUnionAllRemoved(x: null | null) { if (x === null) return; }`,
		// strict guard with non-nullish comparison on right
		`function testGuardNonRight(x: any) { if (x === "str") return; }`,
		// removeNullishType with only null/undefined members
		`const nullUndefOnly: null | undefined = null; const resNoNull = nullUndefOnly ?? 1;`,
		// removeNullishType returning multi-element union without nullish
		`const numOrStrOrNull: number | string | null = 1; const resMultiUnion = numOrStrOrNull ?? true;`,
		// union member lookup with single element union
		`function testUnionSingle(u: { x: number } | { x: number }) { return u.x; }`,
		// await on non-object value (e.g. number)
		`async function testAwaitNum() { await 123; }`,
		// functionMemberType with union containing only non-functions
		`type NumOrStr = number | string; function testFMT(x: NumOrStr) {}`,
		// thenable method with empty return / non-matching callback
		`class ThenBadCb { then(cb: number) {} } async function testThenBadCb() { await new ThenBadCb(); }`,
		// unknown primitive type node name
		`type MyUnknownPrim = foobar;`,
		// thenable method with empty params
		`class ThenEmpty { then(): void {} } async function testThenEmpty() { await new ThenEmpty(); }`,
		// thenable method with non-function onfulfilled
		`class ThenNonFn { then(onfulfilled: number): void {} } async function testThenNonFn() { await new ThenNonFn(); }`,
		// thenable onfulfilled with 0 params
		`class ThenZeroParam { then(onfulfilled: () => void): void {} } async function testThenZeroParam() { await new ThenZeroParam(); }`,
		// duplicate import alias on same module with exported member present
		`function a() {} function b() {} import { a as dupLocal, b as dupLocal } from "./mod";`,
		// base class with method whose owner is empty
		`class TopBase { mTop() {} } class MidBase extends TopBase {} class LowBase extends MidBase {}`,
		// class without constructor extending base with constructor
		`class CtorBase { constructor(x: number) {} } class SubNoCtor extends CtorBase {} function testSubNoCtor() { new SubNoCtor(1); }`,
		// index expr with unknown object
		`const anyObj: any = {}; anyObj["k"];`,
		// Arrow function with declared return type and expression body assignable
		`const arrowMatchRet = (x: number): number => x;`,
		// Arrow function with declared return type mismatch on expr body
		`const arrowMismatchRet = (x: number): string => x;`,
		// Union of null and undefined reduced to never
		`function testUnionNullUndef(x: null | undefined) { if (x == null) return; }`,
		// Import alias duplicate definition error
		`import { a as dupAlias } from "./mod1"; import { b as dupAlias } from "./mod2";`,
		// Base class without owner
		`class RootBase { rootM() {} } class DerivedChild extends RootBase {}`,
		// Enum member with identifier value
		`const notNum = "str"; enum BadEnumInit { Val = notNum }`,
		// Arrow function with non-expr body and no return statement
		`const arrowVoidRet: () => void = () => {};`,
	}
	for i, dSrc := range diagSources {
		f := fs.AddFile(fmt.Sprintf("diag_%d.ts", i), []byte(dSrc))
		p := parser.New(f)
		prog, _ := p.Parse()
		_ = sema.Check(prog)
	}

	// 3. Types methods
	tNum := types.TypeNumber
	tStr := types.TypeString
	tAny := types.TypeAny
	tVoid := types.TypeVoid

	_ = tNum.Equals(tNum)
	_ = tNum.Equals(tStr)
	_ = tNum.Equals(nil)
	_ = tNum.AssignableTo(tAny)
	_ = tNum.AssignableTo(nil)

	tv := types.NewTypeVar("T", nil)
	_ = tv.Kind()
	_ = tv.String()
	_ = tv.Equals(tv)
	_ = tv.AssignableTo(tAny)

	tup := types.NewTuple(tNum, tStr)
	_ = tup.Kind()
	_ = tup.String()
	_ = tup.Equals(tup)
	_ = tup.AssignableTo(tup)
	_ = tup.AssignableTo(tAny)

	arr := types.NewArray(tNum)
	_ = arr.Kind()
	_ = arr.String()
	_ = arr.Equals(arr)
	_ = arr.AssignableTo(arr)
	_ = arr.AssignableTo(tAny)

	obj := types.NewObject("Shape")
	obj.AddField("val", tNum, false)
	_ = obj.Kind()
	_ = obj.String()
	_ = obj.Equals(obj)
	_ = obj.AssignableTo(obj)
	_ = obj.AssignableTo(tAny)

	fn := types.NewFunction([]types.Param{{Name: "a", Type: tNum}}, tVoid)
	_ = fn.Kind()
	_ = fn.String()
	_ = fn.Equals(fn)
	_ = fn.AssignableTo(fn)
	_ = fn.AssignableTo(tAny)

	u := types.NewUnion(tNum, tStr)
	_ = u.Kind()
	_ = u.String()
	_ = u.Equals(u)
	_ = u.AssignableTo(u)
	_ = u.AssignableTo(tAny)
}

func TestParserRemainingEdgeCases(t *testing.T) {
	fs := source.NewFileSet()
	// 1. Bitwise binary expressions
	f1 := fs.AddFile("bitwise.ts", []byte("const a = 1 | 2 ^ 3 & 4;"))
	p1 := parser.New(f1)
	_, _ = p1.Parse()

	// 2. Destructuring comma skips in array pattern
	f2 := fs.AddFile("destruct.ts", []byte("const [, b, , d] = [1, 2, 3, 4];"))
	p2 := parser.New(f2)
	_, _ = p2.Parse()

	// 3. Class method with static, override, public/private/protected, readonly
	f3 := fs.AddFile("class_mods.ts", []byte(`
class ModTest {
	static staticMethod(): void {}
	private privateField: number;
	protected protectedField: string;
	readonly roField: boolean = true;
	public override score(): number { return 1; }
}
`))
	p3 := parser.New(f3)
	_, _ = p3.Parse()

	// 4. Async without function keyword error
	f4 := fs.AddFile("bad_async.ts", []byte("async let x = 1;"))
	p4 := parser.New(f4)
	_, _ = p4.Parse()

	// 5. Try without catch or finally error
	f5 := fs.AddFile("bad_try.ts", []byte("try { let x = 1; }"))
	p5 := parser.New(f5)
	_, _ = p5.Parse()

	// 6. Switch with bad clause
	f6 := fs.AddFile("bad_switch.ts", []byte("switch (x) { foo: break; }"))
	p6 := parser.New(f6)
	_, _ = p6.Parse()

	// 7. Unterminated regex
	f7 := fs.AddFile("bad_regex.ts", []byte("const r = /unterminated;"))
	p7 := parser.New(f7)
	_, _ = p7.Parse()

	// 8. Regex with character class
	f8 := fs.AddFile("good_regex.ts", []byte("const r = /[a-z]/g;"))
	p8 := parser.New(f8)
	_, _ = p8.Parse()

	// 9. Arrow function tryParse failures
	f9 := fs.AddFile("not_arrow.ts", []byte("const x = (a: number); const y = ((1));"))
	p9 := parser.New(f9)
	_, _ = p9.Parse()

	// 10. IRGen capture and closure branches
	f10 := fs.AddFile("captures.ts", []byte(`
function testCap(a: number, b: number) {
	const arrow = () => a ? b : 0;
	function inner() {
		let x = 1;
		if (a > 0) {
			x = arrow();
		} else {
			x = 2;
		}
		while (x < 10) {
			x++;
		}
		do {
			x += 1;
		} while (x < 12);
		for (let i = 0; i < 5; i++) {
			x += i;
		}
		for (const v of [a, b]) {
			x += v;
		}
		switch (x) {
			case 1:
				x = 10;
				break;
			default:
				x = 20;
				break;
		}
		const obj = { p: a, q: [b] };
		const arr = [a, ...obj.q];
		x += arr[0] + obj.p;
		return x;
	}
	return inner();
}
`))
	p10 := parser.New(f10)
	prog10, _ := p10.Parse()
	res10 := sema.Check(prog10)
	ir10, _ := irgen.Generate(prog10, res10)
	if ir10 != nil {
		_, _ = lower.Lower(ir10, lower.ArchAMD64)
	}

	// 11. Readonly constructor param, grouped non-arrow type, named function expr, numeric object key
	f11 := fs.AddFile("parser_corners.ts", []byte(`
class ReadonlyParamTest {
	constructor(public readonly prop: number) {}
}
type GroupedType = (number);
const namedFn = function myFunc() {};
const objNum = { 123: "val" };
`))
	p11 := parser.New(f11)
	_, _ = p11.Parse()

	// 12. Unterminated regex with newline
	f12 := fs.AddFile("regex_newline.ts", []byte("const r = /abc\nlet x = 1;"))
	p12 := parser.New(f12)
	_, _ = p12.Parse()

	// 13. Template literal interpolation complex expressions, escapes, errors
	f13 := fs.AddFile("template_corners.ts", []byte("const t1 = `nested ${ { a: 1 } } done`; const t2 = `quotes ${ \"s\\\\\"tr\" + 'c\\\\'h' + `nest` } done`; const t3 = `unterminated ${ 1 + 2 done`; const t4 = `invalid ${ let x = 1; } done`;"))
	p13 := parser.New(f13)
	p13.EnsureProgress(0, "test")
	_, _ = p13.Parse()
}

