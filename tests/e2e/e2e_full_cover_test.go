package e2e_test

import (
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
	}

	fs := source.NewFileSet()
	for i, src := range edgeSources {
		f := fs.AddFile(string(rune('a'+i))+".ts", []byte(src))
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

	// 2. Types methods
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
