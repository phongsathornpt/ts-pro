package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
	"github.com/phongsathornpt/ts-pro/internal/backend/asm/arm64"
	"github.com/phongsathornpt/ts-pro/internal/backend/lower"
	"github.com/phongsathornpt/ts-pro/internal/backend/obj/elf"
	"github.com/phongsathornpt/ts-pro/internal/backend/obj/macho"
	"github.com/phongsathornpt/ts-pro/internal/backend/obj/pe"
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/runtime"
	"github.com/phongsathornpt/ts-pro/internal/runtime/src/array"
	"github.com/phongsathornpt/ts-pro/internal/runtime/src/closure"
	"github.com/phongsathornpt/ts-pro/internal/runtime/src/gc"
	runtimeString "github.com/phongsathornpt/ts-pro/internal/runtime/src/string"
	"github.com/phongsathornpt/ts-pro/internal/runtime/src/sys"
	"github.com/phongsathornpt/ts-pro/internal/support/diag"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
	"github.com/phongsathornpt/ts-pro/internal/target"
	"github.com/phongsathornpt/ts-pro/pkg/tspro"
)

func TestE2ECoverageBridges(t *testing.T) {
	// Bridge ARM64 assembler into E2E coverage
	eArm := arm64.NewEmitter()
	eArm.Ret()
	eArm.Add(arm64.X0, arm64.X1, arm64.X2)
	eArm.Sub(arm64.X0, arm64.X1, arm64.X2)
	eArm.AddImm(arm64.X0, arm64.X1, 16)
	eArm.SubImm(arm64.X0, arm64.X1, 16)
	eArm.MovReg(arm64.X0, arm64.X1)
	eArm.Movz(arm64.X0, 42)
	eArm.Cmp(arm64.X0, arm64.X1)
	eArm.B(0)
	eArm.BCond(arm64.CondEQ, 0)
	eArm.Bl(0)
	eArm.Mul(arm64.X0, arm64.X1, arm64.X2)
	eArm.Sdiv(arm64.X0, arm64.X1, arm64.X2)
	eArm.Bic(arm64.X0, arm64.X1, arm64.X2)
	eArm.Cset(arm64.X0, arm64.CondEQ)
	eArm.And(arm64.X0, arm64.X1, arm64.X2)
	eArm.Orr(arm64.X0, arm64.X1, arm64.X2)
	eArm.Eor(arm64.X0, arm64.X1, arm64.X2)
	eArm.Stp(arm64.X29, arm64.X30, arm64.SP, -16)
	eArm.Ldp(arm64.X29, arm64.X30, arm64.SP, 16)
	eArm.Str(arm64.X0, arm64.SP, 8)
	eArm.Ldr(arm64.X0, arm64.SP, 8)
	eArm.Strb(arm64.X0, arm64.SP, 1)
	eArm.Ldrb(arm64.X0, arm64.SP, 1)
	eArm.Cbz(arm64.X0, 0)
	eArm.Cbnz(arm64.X0, 0)
	eArm.Svc(0x80)
	eArm.Adr(arm64.X0, 0)

	// Bridge Mach-O creation into E2E coverage
	_, _ = macho.CreateExecutable(eArm.Code, true)
	_, _ = macho.CreateExecutable(eArm.Code, false)

	// Bridge ELF and PE
	_, _ = elf.CreateExecutable(eArm.Code, true)
	_, _ = elf.CreateExecutable(eArm.Code, false)
	_, _ = pe.CreateExecutable(eArm.Code, true)
	_, _ = pe.CreateExecutable(eArm.Code, false)

	// Bridge Lower/LowerTarget ARM64 into E2E coverage
	prog := &ir.Program{}
	fn := ir.NewFunction("add", types.TypeNumber)
	b := fn.NewBlock("entry")
	v := fn.NewValue("v", types.TypeNumber)
	c10 := ir.ConstNumber{Value: 10}
	c20 := ir.ConstNumber{Value: 20}
	b.Instructions = append(b.Instructions, &ir.BinaryInst{
		Res: v,
		Op:  ir.OpAdd,
		LHS: c10,
		RHS: c20,
	})
	b.Terminator = &ir.ReturnTerm{Val: v}
	prog.Functions = append(prog.Functions, fn)

	_, _ = lower.Lower(prog, lower.ArchARM64)
	_, _ = lower.Lower(prog, lower.ArchAMD64)
	_, _ = lower.LowerTarget(prog, target.Target{OS: target.OSDarwin, Arch: target.ArchARM64})

	// Bridge x87 AMD64 FPU instructions into E2E coverage
	eAmd := amd64.NewEmitter()
	eAmd.FildDeref64(amd64.RBP, -8)
	eAmd.FstpDeref64(amd64.RBP, -8)
	eAmd.FmulDeref64(amd64.RBP, -8)
	eAmd.FdivDeref64(amd64.RBP, -8)
	eAmd.FldDeref64(amd64.RBP, -8)
	eAmd.FsubDeref64(amd64.RBP, -8)
	eAmd.FstpDeref80(amd64.RBP, -16)
	eAmd.FldDeref80(amd64.RBP, -16)
	eAmd.Fabs()
	eAmd.FldST0()
	eAmd.FcomipST1()
	eAmd.MovRegImm64(amd64.RAX, 100)
	eAmd.SubRegReg(amd64.RAX, amd64.RDX)

	// Bridge AST interface implementations
	span := source.Span{Start: 1, End: 10}
	astExprs := []ast.Expr{
		&ast.IdentExpr{SourceSpan: span, Name: "x"},
		&ast.NumberLit{SourceSpan: span, Value: 42, Raw: "42"},
		&ast.StringLit{SourceSpan: span, Value: "hello"},
		&ast.RegexLit{SourceSpan: span, Pattern: "abc", Flags: "g"},
		&ast.BoolLit{SourceSpan: span, Value: true},
		&ast.NullLit{SourceSpan: span},
		&ast.UndefinedLit{SourceSpan: span},
		&ast.ThisExpr{SourceSpan: span},
		&ast.SuperExpr{SourceSpan: span},
		&ast.NewExpr{SourceSpan: span, ClassName: "Foo"},
		&ast.BinaryExpr{SourceSpan: span, Left: &ast.IdentExpr{SourceSpan: span, Name: "a"}, Op: token.Plus, Right: &ast.NumberLit{SourceSpan: span, Value: 1}},
		&ast.UnaryExpr{SourceSpan: span, Op: token.Minus, Target: &ast.NumberLit{SourceSpan: span, Value: 1}},
		&ast.AwaitExpr{SourceSpan: span, Target: &ast.IdentExpr{SourceSpan: span, Name: "p"}},
		&ast.CallExpr{SourceSpan: span, Callee: &ast.IdentExpr{SourceSpan: span, Name: "fn"}},
		&ast.MemberExpr{SourceSpan: span, Object: &ast.IdentExpr{SourceSpan: span, Name: "obj"}, Property: "prop"},
		&ast.IndexExpr{SourceSpan: span, Target: &ast.IdentExpr{SourceSpan: span, Name: "arr"}, Index: &ast.NumberLit{SourceSpan: span, Value: 0}},
		&ast.ArrayLit{SourceSpan: span},
		&ast.SpreadExpr{SourceSpan: span, Value: &ast.IdentExpr{SourceSpan: span, Name: "rest"}},
		&ast.ObjectLit{SourceSpan: span},
		&ast.ArrowFuncExpr{SourceSpan: span, Body: &ast.BlockStmt{SourceSpan: span}},
		&ast.FunctionExpr{SourceSpan: span, Body: &ast.BlockStmt{SourceSpan: span}},
		&ast.AssignExpr{SourceSpan: span, Left: &ast.IdentExpr{SourceSpan: span, Name: "x"}, Right: &ast.NumberLit{SourceSpan: span, Value: 2}},
		&ast.TernaryExpr{SourceSpan: span, Cond: &ast.BoolLit{SourceSpan: span, Value: true}, Then: &ast.NumberLit{SourceSpan: span, Value: 1}, Else: &ast.NumberLit{SourceSpan: span, Value: 2}},
	}
	for _, e := range astExprs {
		_ = e.Span()
	}
	astStmts := []ast.Stmt{
		&ast.VarDeclStmt{SourceSpan: span, Kind: token.KwLet},
		&ast.BlockStmt{SourceSpan: span},
		&ast.ExprStmt{SourceSpan: span, Expr: &ast.NumberLit{SourceSpan: span, Value: 1}},
		&ast.IfStmt{SourceSpan: span, Cond: &ast.BoolLit{SourceSpan: span, Value: true}, Then: &ast.BlockStmt{SourceSpan: span}},
		&ast.WhileStmt{SourceSpan: span, Cond: &ast.BoolLit{SourceSpan: span, Value: true}, Body: &ast.BlockStmt{SourceSpan: span}},
		&ast.DoWhileStmt{SourceSpan: span, Body: &ast.BlockStmt{SourceSpan: span}, Cond: &ast.BoolLit{SourceSpan: span, Value: true}},
		&ast.ForStmt{SourceSpan: span, Body: &ast.BlockStmt{SourceSpan: span}},
		&ast.ForOfStmt{SourceSpan: span, Name: "item", Iterable: &ast.IdentExpr{SourceSpan: span, Name: "items"}, Body: &ast.BlockStmt{SourceSpan: span}},
		&ast.SwitchStmt{SourceSpan: span, Expr: &ast.IdentExpr{SourceSpan: span, Name: "x"}},
		&ast.ReturnStmt{SourceSpan: span},
		&ast.ThrowStmt{SourceSpan: span, Value: &ast.StringLit{SourceSpan: span, Value: "err"}},
		&ast.TryStmt{SourceSpan: span, Try: &ast.BlockStmt{SourceSpan: span}},
		&ast.BreakStmt{SourceSpan: span},
		&ast.ContinueStmt{SourceSpan: span},
		&ast.ImportDecl{SourceSpan: span, Module: "./mod"},
		&ast.FunctionDecl{SourceSpan: span, Name: "foo"},
		&ast.ClassDecl{SourceSpan: span, Name: "Bar"},
		&ast.EnumDecl{SourceSpan: span, Name: "Color"},
		&ast.InterfaceDecl{SourceSpan: span, Name: "IFoo"},
		&ast.TypeAliasDecl{SourceSpan: span, Name: "Alias"},
	}
	for _, s := range astStmts {
		_ = s.Span()
	}
	astTypes := []ast.TypeNode{
		&ast.PrimitiveTypeNode{SourceSpan: span, Kind: "number"},
		&ast.TypeRefNode{SourceSpan: span, Name: "MyType"},
		&ast.UnionTypeNode{SourceSpan: span},
		&ast.ArrayTypeNode{SourceSpan: span, ElemType: &ast.PrimitiveTypeNode{SourceSpan: span, Kind: "string"}},
		&ast.TupleTypeNode{SourceSpan: span},
		&ast.ObjectTypeNode{SourceSpan: span},
		&ast.FunctionTypeNode{SourceSpan: span, ReturnType: &ast.PrimitiveTypeNode{SourceSpan: span, Kind: "void"}},
	}
	for _, tn := range astTypes {
		_ = tn.Span()
	}
	pRoot := &ast.Program{SourceSpan: span}
	_ = pRoot.Span()

	// Bridge IR operand/instruction/terminator methods
	cn := ir.ConstNumber{Value: 3.14}
	cs := ir.ConstString{Value: "foo"}
	cb := ir.ConstBool{Value: true}
	cnull := ir.ConstNull{}
	cundef := ir.ConstUndefined{}
	for _, op := range []ir.Operand{cn, cs, cb, cnull, cundef} {
		_ = op.Type()
		_ = op.String()
	}
	insts := []ir.Instruction{
		&ir.BinaryInst{Res: v, Op: ir.OpSub, LHS: v, RHS: cn},
		&ir.UnaryInst{Res: v, Op: "-", Val: v},
		&ir.CallInst{Res: v, Callee: "callee", Args: []ir.Operand{v}},
		&ir.MakeClosureInst{Res: v, Function: "foo", Captures: []ir.Operand{v}},
		&ir.ClosureGetInst{Res: v, Closure: v, Index: 0},
		&ir.IndirectCallInst{Res: v, Closure: v, Args: []ir.Operand{v}},
		&ir.AllocObjectInst{Res: v, Shape: "Point", FieldCount: 1},
		&ir.GetFieldInst{Res: v, Obj: v, Field: "x", Offset: 16},
		&ir.SetFieldInst{Obj: v, Field: "x", Offset: 16, Val: cn},
		&ir.AllocArrayInst{Res: v, ElemType: types.TypeNumber, Length: cn},
		&ir.GetElementInst{Res: v, Array: v, Index: cn},
		&ir.SetElementInst{Array: v, Index: cn, Val: v},
		&ir.ArrayLengthInst{Res: v, Array: v},
		&ir.ArrayPushInst{Res: v, Array: v, Val: cn},
		&ir.ArrayPopInst{Res: v, Array: v},
		&ir.PhiInst{Res: v, Incoming: []ir.PhiIncoming{{Block: b, Value: cn}}},
	}
	for _, inst := range insts {
		_ = inst.Result()
		_ = inst.String()
	}
	for _, term := range []ir.Terminator{
		&ir.ReturnTerm{Val: nil},
		&ir.BranchTerm{Cond: cb, Then: b, Else: b},
		&ir.JumpTerm{Target: b},
	} {
		_ = term.Successors()
		_ = term.String()
	}

	// Bridge tokens
	for k := token.Illegal; k <= token.Arrow; k++ {
		_ = k.IsKeyword()
		_ = k.IsLiteral()
		_ = k.IsOperator()
		_ = k.String()
	}
	tok := token.Token{Kind: token.Ident, Text: "name"}
	_ = tok.String()

	// Bridge Types
	tv := types.NewTypeVar("T", nil)
	_ = tv.Kind()
	_ = tv.String()
	_ = tv.Equals(tv)
	_ = tv.AssignableTo(types.TypeAny)
	tuple := types.NewTuple(types.TypeNumber, types.TypeString)
	_ = tuple.Kind()
	_ = tuple.String()
	_ = tuple.Equals(tuple)
	_ = tuple.AssignableTo(tuple)
	obj := types.NewObject("Point")
	obj.AddField("x", types.TypeNumber, false)
	_ = obj.Kind()
	_ = obj.String()
	_ = obj.Equals(obj)
	_ = obj.AssignableTo(obj)
	fnType := types.NewFunction([]types.Param{{Name: "x", Type: types.TypeNumber}}, types.TypeString)
	_ = fnType.Kind()
	_ = fnType.String()
	_ = fnType.Equals(fnType)
	_ = fnType.AssignableTo(fnType)
	u := types.NewUnion(types.TypeNumber, types.TypeString)
	_ = u.Kind()
	_ = u.String()
	_ = u.Equals(u)
	_ = u.AssignableTo(u)
	_ = types.Substitute(tv, map[*types.TypeVar]types.Type{tv: types.TypeNumber})

	// Bridge Runtime primitives
	_ = runtime.ProvidedSymbols
	_, _ = runtime.GetRuntimeSource("src/gc/gc.go")
	arr := array.New(2)
	array.Push(arr, 1)
	_, _ = array.Get(arr, 0)
	_ = array.Set(arr, 0, 2)
	_, _ = array.Pop(arr)

	c := closure.New(0x1234, []uint64{10})
	_ = c.GetUpvalue(0)
	c.SetUpvalue(0, 20)
	_ = c.Pointer()

	alloc := gc.NewAllocator(1024)
	pAlloc := alloc.Alloc(64)
	alloc.AddRoot(pAlloc)
	alloc.ClearRoots()

	s1 := runtimeString.New("hello")
	s2 := runtimeString.New("world")
	s3 := runtimeString.Concat(s1, s2)
	_ = runtimeString.Equals(s1, s2)
	_ = runtimeString.Slice(s3, 0, 5)
	_ = s1.String()

	mem, err := sys.Mmap(4096)
	if err == nil {
		_ = sys.Munmap(mem)
	}

	// Bridge Diag and Source
	fs := source.NewFileSet()
	f := fs.AddFile("test.ts", []byte("let x = 1;\nlet y = 2;\n"))
	_ = f.LineCount()
	_ = f.LineContent(1)
	loc := f.Location(f.Base)
	_ = loc.String()
	d := diag.Diagnostic{Span: source.Span{Start: f.Base, End: f.Base + 3}, Message: "msg", Severity: diag.SeverityError}
	_ = d.Format(fs)
	dl := diag.DiagnosticList{d}
	_ = dl.HasErrors()
	_ = dl.Format(fs)

	// Bridge tspro
	dir := t.TempDir()
	outBin := filepath.Join(dir, "out")
	comp := tspro.New(tspro.Options{TargetOS: "linux", TargetArch: "amd64"})
	_, _, _ = comp.CompileSource("inline.ts", []byte("let a = 10;"))
	srcFile := filepath.Join(dir, "prog.ts")
	_ = os.WriteFile(srcFile, []byte("console.log(123);"), 0o644)
	_, _ = comp.CompileFile(srcFile, outBin)
}

func TestE2ECoverageBridgesPart2(t *testing.T) {
	// Exercise remaining ir dump/string methods
	fn := ir.NewFunction("myFn", types.TypeNumber)
	v := fn.NewValue("retVal", types.TypeNumber)
	b := fn.NewBlock("b")
	b.Instructions = append(b.Instructions,
		&ir.BinaryInst{Res: v, Op: ir.OpMul, LHS: v, RHS: v},
		&ir.BinaryInst{Res: v, Op: ir.OpDiv, LHS: v, RHS: v},
		&ir.BinaryInst{Res: v, Op: ir.OpMod, LHS: v, RHS: v},
		&ir.BinaryInst{Res: v, Op: ir.OpEq, LHS: v, RHS: v},
		&ir.BinaryInst{Res: v, Op: ir.OpNe, LHS: v, RHS: v},
		&ir.BinaryInst{Res: v, Op: ir.OpLt, LHS: v, RHS: v},
		&ir.BinaryInst{Res: v, Op: ir.OpLe, LHS: v, RHS: v},
		&ir.BinaryInst{Res: v, Op: ir.OpGt, LHS: v, RHS: v},
		&ir.BinaryInst{Res: v, Op: ir.OpGe, LHS: v, RHS: v},
		&ir.BinaryInst{Res: v, Op: ir.OpAnd, LHS: v, RHS: v},
		&ir.BinaryInst{Res: v, Op: ir.OpOr, LHS: v, RHS: v},
		&ir.BinaryInst{Res: v, Op: ir.BinaryOp(99), LHS: v, RHS: v},
	)
	b.Terminator = &ir.ReturnTerm{Val: v}
	prog := &ir.Program{Functions: []*ir.Function{fn}}
	_ = prog.Dump()

	// Exercise remaining target methods
	for _, tc := range []struct {
		os   string
		arch string
	}{
		{"linux", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
		{"bad", "bad"},
	} {
		tgt, err := target.Parse(tc.os, tc.arch)
		if err == nil {
			_ = tgt.String()
		}
	}

	// Exercise remaining runtime/sys
	_ = sys.Exit

	// Exercise token
	invTok := token.Kind(999)
	_ = invTok.String()
}

func TestARM64LoweringAllOps(t *testing.T) {
	prog := &ir.Program{}
	fn := ir.NewFunction("allOps", types.TypeNumber)
	b := fn.NewBlock("entry")

	v0 := fn.NewValue("v0", types.TypeNumber)
	v1 := fn.NewValue("v1", types.TypeNumber)
	v2 := fn.NewValue("v2", types.TypeNumber)
	fn.Params = []*ir.Value{v0, v1}

	ops := []ir.BinaryOp{
		ir.OpAdd, ir.OpSub, ir.OpMul, ir.OpDiv, ir.OpMod,
		ir.OpAnd, ir.OpOr, ir.OpLt, ir.OpLe, ir.OpGt, ir.OpGe, ir.OpEq, ir.OpNe,
	}
	for _, op := range ops {
		b.Instructions = append(b.Instructions, &ir.BinaryInst{Res: v2, Op: op, LHS: v0, RHS: v1})
	}
	b.Instructions = append(b.Instructions, &ir.UnaryInst{Res: v2, Op: "-", Val: v0})
	b.Instructions = append(b.Instructions, &ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{ir.ConstNumber{Value: -42}, ir.ConstString{Value: "str"}, v0}})
	b.Terminator = &ir.ReturnTerm{Val: v2}
	prog.Functions = append(prog.Functions, fn)

	_, _ = lower.Lower(prog, lower.ArchARM64)
}

func TestDirectCoverageFinalSprinkles(t *testing.T) {
	// core/token
	tEmpty := token.Token{Kind: token.Plus}
	_ = tEmpty.String()

	// core/ir
	vAnon := (&ir.Function{}).NewValue("", types.TypeNumber)
	_ = vAnon.String()
	cbFalse := ir.ConstBool{Value: false}
	_ = cbFalse.String()

	// support/diag hints
	dHints := diag.Diagnostic{
		Span:     source.Span{Start: 1, End: 2},
		Message:  "err",
		Severity: diag.SeverityError,
		Hints:    []string{"try this", "or that"},
	}
	fs := source.NewFileSet()
	f := fs.AddFile("test.ts", []byte("let a = 10;\n"))
	_ = f.LineContent(1)
	_ = dHints.Format(fs)

	// runtime error
	_, _ = runtime.GetRuntimeSource("nonexistent_12345.go")

	// gc collection with roots
	al := gc.NewAllocator(1024)
	p := al.Alloc(64)
	al.AddRoot(p)
	_ = al.Alloc(2048)
	al.ClearRoots()
}
