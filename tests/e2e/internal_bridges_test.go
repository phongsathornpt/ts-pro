package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
	"github.com/phongsathornpt/ts-pro/internal/backend/regalloc"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/frontend/lexer"
	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
	"github.com/phongsathornpt/ts-pro/internal/midend/irgen"
	"github.com/phongsathornpt/ts-pro/internal/midend/opt"
	"github.com/phongsathornpt/ts-pro/internal/runtime"
	"github.com/phongsathornpt/ts-pro/internal/runtime/src/array"
	"github.com/phongsathornpt/ts-pro/internal/runtime/src/closure"
	"github.com/phongsathornpt/ts-pro/internal/runtime/src/gc"
	runtimeString "github.com/phongsathornpt/ts-pro/internal/runtime/src/string"
	"github.com/phongsathornpt/ts-pro/internal/runtime/src/sys"
	"github.com/phongsathornpt/ts-pro/internal/support/diag"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
	"github.com/phongsathornpt/ts-pro/pkg/tspro"
)

func TestInternalBridges(t *testing.T) {
	// 1. AMD64 all encoders
	eAmd := amd64.NewEmitter()
	eAmd.Ret()
	eAmd.Push(amd64.RBP)
	eAmd.Push(amd64.R12)
	eAmd.Pop(amd64.RBP)
	eAmd.Pop(amd64.R12)
	eAmd.MovRegReg(amd64.RAX, amd64.RDI)
	eAmd.MovRegReg(amd64.R12, amd64.R13)
	eAmd.AddRegReg(amd64.RAX, amd64.RDX)
	eAmd.AddRegReg(amd64.R12, amd64.R13)
	eAmd.SubRegReg(amd64.RAX, amd64.RDX)
	eAmd.SubRegReg(amd64.R12, amd64.R13)
	eAmd.ImulRegReg(amd64.RAX, amd64.RDX)
	eAmd.XorRegReg(amd64.RAX, amd64.RAX)
	eAmd.AndRegReg(amd64.RAX, amd64.RDX)
	eAmd.AndRegReg(amd64.R12, amd64.R13)
	eAmd.OrRegReg(amd64.RAX, amd64.RDX)
	eAmd.OrRegReg(amd64.R12, amd64.R13)
	eAmd.LeaRipRel32(amd64.RAX, 0)
	eAmd.MovDerefReg(amd64.RSP, 16, amd64.RAX)
	eAmd.MovRegDeref(amd64.RAX, amd64.RSP, 16)
	eAmd.MovDerefReg(amd64.R12, 16, amd64.RAX)
	eAmd.MovRegDeref(amd64.RAX, amd64.R12, 16)
	eAmd.TestRegReg(amd64.R12, amd64.R12)
	eAmd.TestRegReg(amd64.RAX, amd64.RAX)
	eAmd.Setcc(amd64.CondNE, amd64.R12)
	eAmd.Setcc(amd64.CondE, amd64.RAX)
	eAmd.Cqo()
	eAmd.IdivReg(amd64.R12)
	eAmd.MovDerefReg8(amd64.RSP, 16, amd64.RDX)
	eAmd.MovDerefReg8(amd64.R12, 1000, amd64.RDX)
	eAmd.MovzxRegDeref8(amd64.R12, amd64.RSP, 16)
	eAmd.NegReg(amd64.R12)
	eAmd.MovQXMMReg(amd64.XMM0, amd64.RAX)
	eAmd.MovQRegXMM(amd64.R12, amd64.XMM3)
	eAmd.AddSD(amd64.XMM0, amd64.XMM1)
	eAmd.SubSD(amd64.XMM2, amd64.XMM3)
	eAmd.MulSD(amd64.XMM0, amd64.XMM1)
	eAmd.DivSD(amd64.XMM0, amd64.XMM1)
	eAmd.Ucomisd(amd64.XMM0, amd64.XMM1)
	eAmd.Cvttsd2si(amd64.RAX, amd64.XMM2)
	eAmd.Cvtsi2sd(amd64.XMM3, amd64.R12)
	eAmd.FildDeref64(amd64.RBP, -120)
	eAmd.FildDeref64(amd64.R12, -8)
	eAmd.FstpDeref64(amd64.RBP, -128)
	eAmd.FstpDeref64(amd64.R12, -8)
	eAmd.FmulDeref64(amd64.RBP, -128)
	eAmd.FmulDeref64(amd64.R12, -8)
	eAmd.FdivDeref64(amd64.RBP, -128)
	eAmd.FdivDeref64(amd64.R12, -8)
	eAmd.FldDeref64(amd64.RBP, -128)
	eAmd.FldDeref64(amd64.R12, -8)
	eAmd.FsubDeref64(amd64.RBP, -128)
	eAmd.FsubDeref64(amd64.R12, -8)
	eAmd.FstpDeref80(amd64.RBP, -128)
	eAmd.FstpDeref80(amd64.R12, -16)
	eAmd.FldDeref80(amd64.RBP, -128)
	eAmd.FldDeref80(amd64.R12, -16)
	eAmd.Fabs()
	eAmd.FldST0()
	eAmd.FcomipST1()
	eAmd.CallReg(amd64.RAX)
	eAmd.CallReg(amd64.R11)
	eAmd.MovRegImm64(amd64.RAX, 12345)
	eAmd.MovRegImm64(amd64.R12, 12345)
	eAmd.AddRegImm32(amd64.RAX, 10)
	eAmd.AddRegImm32(amd64.R12, 10)
	eAmd.SubRegImm32(amd64.RAX, 10)
	eAmd.SubRegImm32(amd64.R12, 10)
	eAmd.CmpRegImm32(amd64.RAX, 10)
	eAmd.CmpRegImm32(amd64.R12, 10)
	eAmd.CmpRegReg(amd64.RAX, amd64.RDX)
	eAmd.CmpRegReg(amd64.R12, amd64.R13)
	eAmd.ShrRegImm8(amd64.RAX, 2)
	eAmd.ShrRegImm8(amd64.R12, 2)
	eAmd.Syscall()
	eAmd.MovSDRegReg(amd64.XMM0, amd64.XMM1)
	eAmd.MovSDRegReg(amd64.XMM8, amd64.XMM9)
	eAmd.XorPD(amd64.XMM0, amd64.XMM0)
	eAmd.JmpRel32(0)
	eAmd.JccRel32(amd64.CondE, 0)
	eAmd.CallRel32(0)

	// 2. Types comprehensive coverage
	tv := types.NewTypeVar("T", nil)
	_ = tv.Kind()
	_ = tv.String()
	_ = tv.Equals(tv)
	_ = tv.Equals(nil)
	_ = tv.Equals(types.TypeNumber)
	_ = tv.AssignableTo(types.TypeNumber)
	_ = tv.AssignableTo(types.TypeAny)
	_ = tv.AssignableTo(nil)
	tvC := types.NewTypeVar("C", types.TypeNumber)
	_ = tvC.AssignableTo(types.TypeNumber)
	_ = tvC.AssignableTo(types.TypeString)

	tup := types.NewTuple(types.TypeNumber, types.TypeString)
	_ = tup.Kind()
	_ = tup.String()
	_ = tup.Equals(tup)
	_ = tup.Equals(types.NewTuple(types.TypeNumber))
	_ = tup.Equals(nil)
	_ = tup.AssignableTo(nil)
	_ = tup.AssignableTo(types.TypeAny)
	_ = tup.AssignableTo(tup)
	_ = tup.AssignableTo(types.NewTuple(types.TypeString, types.TypeNumber))
	_ = tup.AssignableTo(types.NewArray(types.TypeNumber))

	arrT := types.NewArray(types.TypeNumber)
	_ = arrT.Kind()
	_ = arrT.String()
	_ = arrT.Equals(arrT)
	_ = arrT.Equals(types.NewArray(types.TypeString))
	_ = arrT.Equals(nil)
	_ = arrT.AssignableTo(nil)
	_ = arrT.AssignableTo(types.TypeAny)
	_ = arrT.AssignableTo(arrT)
	_ = arrT.AssignableTo(types.NewArray(types.TypeString))

	objT := types.NewObject("Obj")
	objT.AddField("x", types.TypeNumber, false)
	objT.AddField("y", types.TypeString, true)
	_ = objT.String()
	_ = objT.Equals(objT)
	_ = objT.Equals(nil)
	_ = objT.AssignableTo(nil)
	_ = objT.AssignableTo(types.TypeAny)
	_ = objT.AssignableTo(objT)
	objT2 := types.NewObject("Obj2")
	objT2.AddField("x", types.TypeNumber, false)
	_ = objT2.AssignableTo(objT)

	fnT := types.NewFunction([]types.Param{{Name: "x", Type: types.TypeNumber, Optional: true, Rest: false}}, types.TypeString)
	_ = fnT.Kind()
	_ = fnT.String()
	_ = fnT.Equals(fnT)
	_ = fnT.Equals(nil)
	_ = fnT.AssignableTo(nil)
	_ = fnT.AssignableTo(types.TypeAny)
	_ = fnT.AssignableTo(fnT)
	fnTThis := &types.FunctionType{This: types.TypeString, Params: []types.Param{{Name: "a", Type: types.TypeNumber, Rest: true}}, Return: types.TypeVoid}
	_ = fnTThis.String()
	_ = fnTThis.Equals(fnT)
	_ = fnT.AssignableTo(fnTThis)

	uT := types.NewUnion(types.TypeNumber, types.TypeString)
	_ = uT.String()
	_ = uT.Equals(uT)
	_ = uT.Equals(nil)
	_ = uT.AssignableTo(nil)
	_ = uT.AssignableTo(types.TypeAny)
	_ = uT.AssignableTo(uT)
	_ = types.NewUnion()
	_ = types.NewUnion(types.TypeNumber)
	_ = types.NewUnion(uT, types.TypeBoolean, types.TypeNever)

	_ = types.Substitute(tv, map[*types.TypeVar]types.Type{tv: types.TypeNumber})
	_ = types.Substitute(tup, map[*types.TypeVar]types.Type{tv: types.TypeNumber})
	_ = types.Substitute(arrT, map[*types.TypeVar]types.Type{tv: types.TypeNumber})
	_ = types.Substitute(objT, map[*types.TypeVar]types.Type{tv: types.TypeNumber})
	_ = types.Substitute(fnT, map[*types.TypeVar]types.Type{tv: types.TypeNumber})
	_ = types.Substitute(uT, map[*types.TypeVar]types.Type{tv: types.TypeNumber})

	genFn := types.NewGenericFunction([]*types.TypeVar{tv}, []types.Param{{Name: "v", Type: tv}}, tv)
	_, _ = types.InstantiateFunction(genFn, []types.Type{types.TypeNumber})
	_, _ = types.InstantiateFunction(nil, nil)
	_, _ = types.InstantiateFunction(genFn, []types.Type{})
	_, _ = types.InferFunction(genFn, []types.Type{types.TypeNumber})
	_, _ = types.InferFunction(nil, nil)
	_, _ = types.InferFunction(types.NewFunction(nil, types.TypeVoid), nil)
	_, _ = types.InferFunction(genFn, []types.Type{})
	_, _ = types.FunctionBindings(genFn, genFn)
	_, _ = types.FunctionBindings(nil, nil)
	_, _ = types.FunctionBindings(types.NewFunction(nil, types.TypeVoid), types.NewFunction(nil, types.TypeVoid))
	// Exercise inferTypeBindings all pattern kinds via InferFunction
	tvObj := types.NewTypeVar("O", nil)
	objPat := types.NewObject("Pat")
	objPat.AddField("item", tvObj, false)
	objArg := types.NewObject("Arg")
	objArg.AddField("item", types.TypeNumber, false)
	objGenFn := types.NewGenericFunction([]*types.TypeVar{tvObj}, []types.Param{{Name: "o", Type: objPat}}, tvObj)
	_, _ = types.InferFunction(objGenFn, []types.Type{objArg})
	tvB := types.NewTypeVar("B", nil)
	tupPatFn := types.NewGenericFunction([]*types.TypeVar{tvB}, []types.Param{{Name: "t", Type: types.NewTuple(tvB, types.TypeNumber)}}, tvB)
	_, _ = types.InferFunction(tupPatFn, []types.Type{types.NewTuple(types.TypeString, types.TypeNumber)})
	arrPatFn := types.NewGenericFunction([]*types.TypeVar{tvB}, []types.Param{{Name: "a", Type: types.NewArray(tvB)}}, tvB)
	_, _ = types.InferFunction(arrPatFn, []types.Type{types.NewArray(types.TypeString)})
	fnPatFn := types.NewGenericFunction([]*types.TypeVar{tvB}, []types.Param{{Name: "f", Type: types.NewFunction([]types.Param{{Name: "x", Type: tvB}}, types.TypeVoid)}}, tvB)
	_, _ = types.InferFunction(fnPatFn, []types.Type{types.NewFunction([]types.Param{{Name: "x", Type: types.TypeNumber}}, types.TypeVoid)})
	// nil pattern/actual and conflicting bindings
	_, _ = types.InferFunction(genFn, nil)
	conflictFn := types.NewGenericFunction([]*types.TypeVar{tv}, []types.Param{{Name: "a", Type: tv}, {Name: "b", Type: tv}}, tv)
	_, _ = types.InferFunction(conflictFn, []types.Type{types.TypeNumber, types.TypeString})
	// FunctionBindings mismatch paths
	_, _ = types.FunctionBindings(genFn, types.NewFunction([]types.Param{{Name: "v", Type: types.TypeString}}, types.TypeString))
	// InstantiateFunction constraint violation
	tvCon := types.NewTypeVar("C", types.TypeNumber)
	conFn := types.NewGenericFunction([]*types.TypeVar{tvCon}, []types.Param{{Name: "v", Type: tvCon}}, tvCon)
	_, _ = types.InstantiateFunction(conFn, []types.Type{types.TypeString})
	// Substitute object/union/this paths
	fnThis := &types.FunctionType{This: tv, Params: []types.Param{{Name: "v", Type: tv}}, Return: tv}
	_ = types.Substitute(fnThis, map[*types.TypeVar]types.Type{tv: types.TypeNumber})
	anonObj := types.NewObject("")
	anonObj.AddField("f", tv, true)
	_ = anonObj.String()
	_ = types.Substitute(anonObj, map[*types.TypeVar]types.Type{tv: types.TypeString})

	// 3. Regalloc all paths
	ra := regalloc.New(1)
	fnAlloc := ir.NewFunction("allocFn", types.TypeNumber)
	bAlloc := fnAlloc.NewBlock("entry")
	vA := fnAlloc.NewValue("vA", types.TypeNumber)
	vB := fnAlloc.NewValue("vB", types.TypeNumber)
	vC := fnAlloc.NewValue("vC", types.TypeNumber)
	bAlloc.Instructions = append(bAlloc.Instructions,
		&ir.BinaryInst{Res: vA, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 1}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: vB, Op: ir.OpAdd, LHS: vA, RHS: ir.ConstNumber{Value: 3}},
		&ir.BinaryInst{Res: vC, Op: ir.OpAdd, LHS: vB, RHS: vA},
	)
	bAlloc.Terminator = &ir.ReturnTerm{Val: vC}
	_ = ra.Allocate(fnAlloc)
	_ = ra.StackFrameSlots()

	// 4. Opt all paths
	optProg := &ir.Program{}
	optFn := ir.NewFunction("optFn", types.TypeNumber)
	optParam := optFn.NewValue("param", types.TypeNumber)
	optFn.Params = []*ir.Value{optParam}
	optB := optFn.NewBlock("entry")
	optV := optFn.NewValue("res", types.TypeNumber)
	deadV := optFn.NewValue("dead", types.TypeNumber)
	targetB := optFn.NewBlock("target")
	targetB.Terminator = &ir.ReturnTerm{Val: optV}
	optB.Instructions = append(optB.Instructions,
		&ir.BinaryInst{Res: optV, Op: ir.OpSub, LHS: ir.ConstNumber{Value: 10}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: optV, Op: ir.OpMul, LHS: ir.ConstNumber{Value: 10}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: optV, Op: ir.OpDiv, LHS: ir.ConstNumber{Value: 10}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: optV, Op: ir.OpMod, LHS: ir.ConstNumber{Value: 10}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: optV, Op: ir.OpAnd, LHS: ir.ConstNumber{Value: 10}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: optV, Op: ir.OpOr, LHS: ir.ConstNumber{Value: 10}, RHS: ir.ConstNumber{Value: 2}},
		&ir.UnaryInst{Res: optV, Op: "-", Val: ir.ConstNumber{Value: 10}},
		&ir.BinaryInst{Res: deadV, Op: ir.OpAdd, LHS: optParam, RHS: ir.ConstNumber{Value: 1}},
	)
	optB.Terminator = &ir.BranchTerm{Cond: ir.ConstBool{Value: true}, Then: targetB, Else: targetB}
	optProg.Functions = append(optProg.Functions, optFn)
	opt.Optimize(optProg, opt.Options{Level: 0})
	opt.Optimize(optProg, opt.Options{Level: 2})

	// 5. Lexer all paths
	fsLex := source.NewFileSet()
	fLex := fsLex.AddFile("lex.ts", []byte(`
/* block */
// line
let s = "hello\n\r\t\\\'\"";
let s2 = 'single';
let num = 0x1F + 0b1010 + 3.14e-2 + .5;
let ops = + ++ += - -- -= * ** *= / /= % %= ! ~ & && &= | || |= ^ ^= << >> >>> = == === != !== < <= > >= ? ?? ?. : => ; , . ... ( ) { } [ ];
`))
	_, _ = lexer.TokenizeAll(fLex)
	fErr1 := fsLex.AddFile("err1.ts", []byte(`/* unterminated`))
	_, _ = lexer.TokenizeAll(fErr1)
	fErr2 := fsLex.AddFile("err2.ts", []byte(`"unterminated`))
	_, _ = lexer.TokenizeAll(fErr2)
	fErr3 := fsLex.AddFile("err3.ts", []byte("`unterminated template"))
	_, _ = lexer.TokenizeAll(fErr3)
	fUtf8 := fsLex.AddFile("utf8.ts", []byte("let \u4e16\u754c = 1; let \u00fc = \"\\z\"; let t = `\\`hello`; obj.\u4e16; obj."))
	_, _ = lexer.TokenizeAll(fUtf8)
	// Exercise ParseFloat helper and Diagnostics accessor
	_, _ = lexer.ParseFloat("3.14")
	_, _ = lexer.ParseFloat("not-a-number")
	lx := lexer.New(fLex)
	_ = lx.Diagnostics()
	pTmp := parser.New(fLex)
	_ = pTmp.Diagnostics()

	// 6. Parser all paths
	fsParse := source.NewFileSet()
	fParse := fsParse.AddFile("parse.ts", []byte(`
import { a, b as c } from "./mod";
let x: (number | string)[] = [1, "two"];
type P = { x: number; y?: string };
function rest(p: string, ...rest: number[]): [string, number] { return [p, 0]; }
const arrow = (x: number): number => x + 1;
const fnExpr = function(this: P, delta: number): number { return this.x + delta; };
switch (1) { case 1: break; default: break; }
for (const v of [1, 2]) {}
while (false) {}
do {} while (false);
try { throw "err"; } catch (e: any) {} finally {}
const r = /abc/i;
const tpl = `+"`"+`num ${1 + 2} text`+"`"+`;
`))
	pInst := parser.New(fParse)
	progParsed, _ := pInst.Parse()

	// 7. Sema all paths & diagnostics
	resSema := sema.Check(progParsed)
	_ = resSema

	// Check sema diagnostics
	for _, errSrc := range []string{
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
	} {
		fDiag := fsParse.AddFile("diag.ts", []byte(errSrc))
		pDiag := parser.New(fDiag)
		pProg, _ := pDiag.Parse()
		_ = sema.Check(pProg)
	}

	// 8. Runtime tests
	_ = runtime.ProvidedSymbols
	_, _ = runtime.GetRuntimeSource("src/gc/gc.go")
	arr := array.New(2)
	array.Push(arr, 1)
	array.Push(arr, 2)
	array.Push(arr, 3)
	_, _ = array.Get(arr, 0)
	_, _ = array.Get(arr, -1)
	_, _ = array.Get(arr, 10)
	_ = array.Set(arr, 1, 10)
	_ = array.Set(arr, -1, 10)
	_ = array.Set(arr, 10, 10)
	_, _ = array.Pop(arr)
	_, _ = array.Pop(arr)
	_, _ = array.Pop(arr)
	_, _ = array.Pop(arr)

	cClosure := closure.New(0x1234, []uint64{10})
	_ = cClosure.GetUpvalue(0)
	_ = cClosure.GetUpvalue(-1)
	_ = cClosure.GetUpvalue(10)
	cClosure.SetUpvalue(0, 20)
	cClosure.SetUpvalue(-1, 20)
	cClosure.SetUpvalue(10, 20)
	_ = cClosure.Pointer()

	alloc := gc.NewAllocator(1024)
	p1 := alloc.Alloc(64)
	p2 := alloc.Alloc(128)
	_ = p1
	_ = p2
	alloc.AddRoot(p1)
	_ = alloc.Alloc(2048)
	alloc.ClearRoots()

	s1 := runtimeString.New("hello ")
	s2 := runtimeString.New("world")
	s3 := runtimeString.Concat(s1, s2)
	_ = runtimeString.Concat(nil, nil)
	_ = runtimeString.Concat(s1, nil)
	_ = runtimeString.Concat(nil, s2)
	_ = runtimeString.Equals(s1, s2)
	_ = runtimeString.Equals(nil, nil)
	_ = runtimeString.Equals(s1, nil)
	_ = runtimeString.Equals(nil, s2)
	_ = runtimeString.Slice(s3, 6, 11)
	_ = runtimeString.Slice(nil, 0, 5)
	_ = runtimeString.Slice(s1, -1, 100)
	_ = runtimeString.Slice(s1, 4, 2)
	_ = s1.String()
	var nilS *runtimeString.TSString
	_ = nilS.String()

	mem, err := sys.Mmap(4096)
	if err == nil {
		_ = sys.Munmap(mem)
	}
	rPipe, wPipe, err := os.Pipe()
	if err == nil {
		_, _ = sys.Write(int(wPipe.Fd()), []byte("test"))
		rPipe.Close()
		wPipe.Close()
	}

	// 9. Diag & Source
	dList := diag.DiagnosticList{
		diag.Diagnostic{Severity: diag.SeverityHint, Message: "hint"},
		diag.Diagnostic{Severity: diag.SeverityInfo, Message: "info"},
		diag.Diagnostic{Severity: diag.SeverityWarning, Message: "warn"},
		diag.Diagnostic{Severity: diag.SeverityError, Message: "err"},
		diag.Diagnostic{Severity: diag.Severity(99), Message: "other"},
	}
	_ = dList.HasErrors()
	_ = dList.Format(fsParse)
	dNoSpan := diag.Diagnostic{Message: "msg"}
	_ = dNoSpan.Format(nil)

	// 10. Tspro error paths
	dir := t.TempDir()
	cTspro := tspro.New(tspro.Options{})
	_ = cTspro.Check("test.ts", []byte("let x: number = 'mismatch';"))
	_ = cTspro.Check("test2.ts", []byte("let x: = 1;"))
	_, _, _ = cTspro.CompileSource("bad.ts", []byte("let x: = ;"))
	_, _, _ = cTspro.CompileSource("imp.ts", []byte("import { x } from './m';"))
	cBadOS := tspro.New(tspro.Options{TargetOS: "bad", TargetArch: "bad"})
	_, _, _ = cBadOS.CompileSource("t.ts", []byte("let a = 1;"))

	badImpFile := filepath.Join(dir, "bad_imp.ts")
	_ = os.WriteFile(badImpFile, []byte("import { x } from 'pkg';"), 0o644)
	_, _ = cTspro.CompileFile(badImpFile, filepath.Join(dir, "out1"))

	missingImpFile := filepath.Join(dir, "missing_imp.ts")
	_ = os.WriteFile(missingImpFile, []byte("import { x } from './nonexistent';"), 0o644)
	_, _ = cTspro.CompileFile(missingImpFile, filepath.Join(dir, "out2"))

	// 11. IRGen direct tests
	fIrgen := fsParse.AddFile("irgen_test.ts", []byte(`
function f(a: number, b: number): number { return a + b; }
class Point { x: number; constructor(x: number) { this.x = x; } }
const pt = new Point(1);
`))
	pIrgen := parser.New(fIrgen)
	progIrgen, _ := pIrgen.Parse()
	semaIrgen := sema.Check(progIrgen)
	irGenerated, _ := irgen.Generate(progIrgen, semaIrgen)
	if irGenerated != nil {
		_ = irGenerated.Dump()
	}
}
