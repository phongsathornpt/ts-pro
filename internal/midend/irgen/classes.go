package irgen

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
)

func (g *generator) classTag(name string) int {
	return g.classTags[name]
}

func (g *generator) isClassDescendant(name, base string) bool {
	for name != "" {
		if name == base {
			return true
		}
		info := g.semaResult.Classes[name]
		name = info.BaseName
	}
	return false
}

func (g *generator) emitClassMethodCall(obj ir.Operand, staticInfo *sema.ClassInfo, method string, args []ir.Operand) ir.Operand {
	methodType := staticInfo.Methods[method]
	callOwner := func(owner string, bb *ir.BasicBlock) ir.Operand {
		g.currentBB = bb
		callArgs := make([]ir.Operand, 0, len(args)+1)
		callArgs = append(callArgs, obj)
		callArgs = append(callArgs, args...)
		if methodType.Return == types.TypeVoid {
			bb.Instructions = append(bb.Instructions, &ir.CallInst{Callee: classMethodName(owner, method), Args: callArgs})
			return nil
		}
		res := g.currentFn.NewValue("method_ret", methodType.Return)
		bb.Instructions = append(bb.Instructions, &ir.CallInst{Res: res, Callee: classMethodName(owner, method), Args: callArgs})
		return res
	}

	// A more concrete SSA type proves the runtime class, so devirtualize.
	if concrete, ok := obj.Type().(*types.ObjectType); ok && concrete.Name != "" && concrete.Name != staticInfo.Name {
		if info := g.semaResult.Classes[concrete.Name]; info != nil && g.isClassDescendant(info.Name, staticInfo.Name) {
			return callOwner(info.MethodOwners[method], g.currentBB)
		}
	}

	staticOwner := staticInfo.MethodOwners[method]
	type candidate struct {
		name, owner string
		tag         int
	}
	var candidates []candidate
	for name, info := range g.semaResult.Classes {
		if name == staticInfo.Name || !g.isClassDescendant(name, staticInfo.Name) {
			continue
		}
		owner := info.MethodOwners[method]
		if owner == "" || owner == staticOwner {
			continue
		}
		candidates = append(candidates, candidate{name: name, owner: owner, tag: g.classTag(name)})
	}
	if len(candidates) == 0 {
		return callOwner(staticOwner, g.currentBB)
	}

	tagVal := g.currentFn.NewValue("class_tag", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: tagVal, Obj: obj, Field: "$class", Offset: 16})
	join := g.currentFn.NewBlock("vcall_join")
	incoming := make([]ir.PhiIncoming, 0, len(candidates)+1)
	check := g.currentBB
	for i, cand := range candidates {
		g.currentBB = check
		cond := g.currentFn.NewValue("class_match", types.TypeBoolean)
		check.Instructions = append(check.Instructions, &ir.BinaryInst{Res: cond, Op: ir.OpEq, LHS: tagVal, RHS: ir.ConstNumber{Value: float64(cand.tag)}})
		callBB := g.currentFn.NewBlock("vcall_" + cand.name)
		nextBB := g.currentFn.NewBlock("vcall_next")
		check.Terminator = &ir.BranchTerm{Cond: cond, Then: callBB, Else: nextBB}
		res := callOwner(cand.owner, callBB)
		if callBB.Terminator == nil {
			callBB.Terminator = &ir.JumpTerm{Target: join}
		}
		if res != nil {
			incoming = append(incoming, ir.PhiIncoming{Block: callBB, Value: res})
		}
		check = nextBB
		_ = i
	}
	fallback := check
	res := callOwner(staticOwner, fallback)
	if fallback.Terminator == nil {
		fallback.Terminator = &ir.JumpTerm{Target: join}
	}
	if res != nil {
		incoming = append(incoming, ir.PhiIncoming{Block: fallback, Value: res})
	}
	g.currentBB = join
	if methodType.Return == types.TypeVoid {
		return nil
	}
	out := g.currentFn.NewValue("vcall_ret", methodType.Return)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: out, Incoming: incoming})
	return out
}

func classConstructorName(className string) string {
	return className + "$constructor"
}

func classMethodName(className, methodName string) string {
	return className + "$" + methodName
}

func classConstructorDecl(cls *ast.ClassDecl) *ast.ClassMethod {
	for i := range cls.Methods {
		if cls.Methods[i].Name == "constructor" {
			return &cls.Methods[i]
		}
	}
	return nil
}

func (g *generator) lowerClassFunction(cls *ast.ClassDecl, info *sema.ClassInfo, method *ast.ClassMethod, fnType *types.FunctionType, name string, constructor bool) (*ir.Function, error) {
	retType := types.TypeVoid
	if fnType != nil {
		retType = fnType.Return
	}
	if constructor {
		retType = types.TypeVoid
	}

	irFn := ir.NewFunction(name, retType)
	g.currentFn = irFn
	g.currentBB = irFn.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	previousClass := g.currentClass
	g.currentClass = info
	defer func() { g.currentClass = previousClass }()

	thisVal := irFn.NewValue("$this", info.Instance)
	irFn.Params = append(irFn.Params, thisVal)
	g.locals["$this"] = thisVal

	if method != nil {
		for i, p := range method.Params {
			pt := types.TypeAny
			if fnType != nil && i < len(fnType.Params) {
				pt = fnType.Params[i].Type
			}
			v := irFn.NewValue(p.Name, pt)
			irFn.Params = append(irFn.Params, v)
			g.locals[p.Name] = v
		}
	}

	offsets, _, _ := g.objectLayout(info.Instance)
	emitOwnInitializers := func() {
		for _, field := range cls.Fields {
			if field.IsStatic || field.Init == nil {
				continue
			}
			offset := offsets[field.Name]
			value := g.lowerExpr(field.Init)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: thisVal, Field: field.Name, Offset: offset, Val: value})
		}
		if method != nil {
			for _, p := range method.Params {
				if !p.IsParameterProperty {
					continue
				}
				offset := offsets[p.Name]
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: thisVal, Field: p.Name, Offset: offset, Val: g.locals[p.Name]})
			}
		}
	}

	bodyStart := 0
	if constructor && info.BaseName != "" {
		// JavaScript derived constructors initialize the base portion first. The
		// current native subset requires an explicit leading super(...) when a
		// derived constructor is declared; a synthesized constructor calls the
		// parameterless base constructor.
		if method != nil && method.Body != nil && len(method.Body.Statements) > 0 {
			if exprStmt, ok := method.Body.Statements[0].(*ast.ExprStmt); ok {
				if call, ok := exprStmt.Expr.(*ast.CallExpr); ok {
					if _, ok := call.Callee.(*ast.SuperExpr); ok {
						g.lowerExpr(call)
						bodyStart = 1
					}
				}
			}
			if bodyStart == 0 {
				return nil, fmt.Errorf("derived class %s constructor must begin with super(...) in native lowering", cls.Name)
			}
		} else {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: classConstructorName(info.BaseName), Args: []ir.Operand{thisVal}})
		}
		emitOwnInitializers()
	} else if constructor {
		emitOwnInitializers()
	}

	if method != nil && method.Body != nil {
		for _, stmt := range method.Body.Statements[bodyStart:] {
			g.lowerStatement(stmt)
		}
	}
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}
	return irFn, g.err
}

func (g *generator) lowerClassDecl(cls *ast.ClassDecl) error {
	info := g.semaResult.Classes[cls.Name]
	if len(cls.TypeParams) > 0 {
		// Generic classes are emitted only after concrete class specialization is
		// implemented. Their declarations have no standalone native ABI.
		return nil
	}

	outerFn, outerBB, outerLocals, outerProvenance, outerBindings := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.typeBindings
	defer func() {
		g.currentFn, g.currentBB, g.locals, g.localProvenance, g.typeBindings = outerFn, outerBB, outerLocals, outerProvenance, outerBindings
	}()

	ctorDecl := classConstructorDecl(cls)
	ctor, err := g.lowerClassFunction(cls, info, ctorDecl, info.Constructor, classConstructorName(info.Name), true)
	if err != nil {
		return err
	}
	g.prog.Functions = append(g.prog.Functions, ctor)

	for i := range cls.Methods {
		method := &cls.Methods[i]
		if method.Name == "constructor" || method.IsStatic {
			continue
		}
		fnType := info.Methods[method.Name]
		fn, _ := g.lowerClassFunction(cls, info, method, fnType, classMethodName(info.Name, method.Name), false)
		g.prog.Functions = append(g.prog.Functions, fn)
	}
	return nil
}

func (g *generator) ensureClassSpecialization(info *sema.ClassInfo) {
	if info == nil || info.GenericBase == "" {
		return
	}
	if g.emittedClassSpecs[info.Name] {
		return
	}
	g.emittedClassSpecs[info.Name] = true
	outerFn, outerBB, outerLocals, outerProvenance, outerBindings, outerClass := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.typeBindings, g.currentClass
	defer func() {
		g.currentFn, g.currentBB, g.locals, g.localProvenance, g.typeBindings, g.currentClass = outerFn, outerBB, outerLocals, outerProvenance, outerBindings, outerClass
	}()
	g.typeBindings = info.TypeBindings
	cls := info.Decl
	ctorDecl := classConstructorDecl(cls)
	ctor, _ := g.lowerClassFunction(cls, info, ctorDecl, info.Constructor, classConstructorName(info.Name), true)
	g.prog.Functions = append(g.prog.Functions, ctor)
	for i := range cls.Methods {
		method := &cls.Methods[i]
		if method.Name == "constructor" || method.IsStatic {
			continue
		}
		fnType := info.Methods[method.Name]
		fn, _ := g.lowerClassFunction(cls, info, method, fnType, classMethodName(info.Name, method.Name), false)
		g.prog.Functions = append(g.prog.Functions, fn)
	}
}
