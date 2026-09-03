package irgen

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
)

type generator struct {
	semaResult        *sema.Result
	prog              *ir.Program
	currentFn         *ir.Function
	currentBB         *ir.BasicBlock
	locals            map[string]ir.Operand
	err               error
	arrowCounter      int
	genericDecls      map[string]*ast.FunctionDecl
	functionDecls     map[string]*ast.FunctionDecl
	genericSpecs      map[string]string
	genericSpecCount  int
	typeBindings      map[*types.TypeVar]types.Type
	currentClass      *sema.ClassInfo
	classTags         map[string]int
	emittedClassSpecs map[string]bool
}

func irHeapRefType(t types.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case types.KindString, types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
		return true
	case types.KindUnion:
		if u, ok := t.(*types.UnionType); ok {
			for _, m := range u.Members {
				if irHeapRefType(m) {
					return true
				}
			}
		}
	}
	return false
}

func (g *generator) objectLayout(t *types.ObjectType) (map[string]int, uint64, string) {
	names := make([]string, 0, len(t.Fields))
	classLayout := g.semaResult != nil && g.semaResult.Classes[t.Name] != nil && len(t.FieldOrder) == len(t.Fields)
	if classLayout {
		names = append(names, t.FieldOrder...)
	} else {
		for name := range t.Fields {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	offsets := make(map[string]int, len(names))
	var refMask uint64
	baseSlot := 0
	baseOffset := 16
	shapeNames := names
	if classLayout {
		baseSlot = 1
		baseOffset = 24
		shapeNames = append([]string{"$class"}, names...)
	}
	for i, name := range names {
		offsets[name] = baseOffset + i*8
		if irHeapRefType(t.Fields[name].Type) {
			slot := i + baseSlot
			if slot >= 64 {
				if g.err == nil {
					g.err = fmt.Errorf("object reference field %q occupies slot %d beyond the 64-bit GC reference mask", name, slot)
				}
			} else {
				refMask |= uint64(1) << slot
			}
		}
	}
	return offsets, refMask, strings.Join(shapeNames, ",")
}

func (g *generator) classTag(name string) int {
	if tag := g.classTags[name]; tag != 0 {
		return tag
	}
	return 0
}

func (g *generator) isClassDescendant(name, base string) bool {
	for name != "" {
		if name == base {
			return true
		}
		info := g.semaResult.Classes[name]
		if info == nil {
			return false
		}
		name = info.BaseName
	}
	return false
}

func (g *generator) emitClassMethodCall(obj ir.Operand, staticInfo *sema.ClassInfo, method string, args []ir.Operand) ir.Operand {
	methodType := staticInfo.Methods[method]
	if methodType == nil {
		return g.failExpr("class %s has no method %q", staticInfo.Name, method)
	}
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
			owner := info.MethodOwners[method]
			if owner == "" {
				owner = info.Name
			}
			return callOwner(owner, g.currentBB)
		}
	}

	staticOwner := staticInfo.MethodOwners[method]
	if staticOwner == "" {
		staticOwner = staticInfo.Name
	}
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
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].tag < candidates[j].tag })

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

func (g *generator) tupleRefMask(t *types.TupleType) uint64 {
	var mask uint64
	for i, elem := range t.Elements {
		if !irHeapRefType(elem) {
			continue
		}
		if i >= 64 {
			if g.err == nil {
				g.err = fmt.Errorf("tuple reference element %d exceeds the 64-bit GC reference mask", i)
			}
			continue
		}
		mask |= uint64(1) << i
	}
	return mask
}

func (g *generator) failExpr(format string, args ...any) ir.Operand {
	if g.err == nil {
		g.err = fmt.Errorf(format, args...)
	}
	return ir.ConstNumber{Value: 0}
}

func (g *generator) semanticType(node ast.Node) types.Type {
	if node == nil || g.semaResult == nil {
		return nil
	}
	t := g.semaResult.Types[node]
	if t == nil || len(g.typeBindings) == 0 {
		return t
	}
	return types.Substitute(t, g.typeBindings)
}

func genericSpecializationKey(name string, fn *types.FunctionType) string {
	parts := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		parts[i] = p.Type.String()
	}
	return name + "(" + strings.Join(parts, ",") + ")->" + fn.Return.String()
}

func (g *generator) ensureGenericSpecialization(decl *ast.FunctionDecl, concrete *types.FunctionType) (string, error) {
	generic, ok := g.semaResult.Types[decl].(*types.FunctionType)
	if !ok || len(generic.TypeParams) == 0 {
		return "", fmt.Errorf("function %q is not a resolved generic declaration", decl.Name)
	}
	bindings, err := types.FunctionBindings(generic, concrete)
	if err != nil {
		return "", fmt.Errorf("bind generic %s: %w", decl.Name, err)
	}
	key := genericSpecializationKey(decl.Name, concrete)
	if name, ok := g.genericSpecs[key]; ok {
		return name, nil
	}
	name := fmt.Sprintf("%s$spec%d", decl.Name, g.genericSpecCount)
	g.genericSpecCount++
	// Register before lowering so recursive calls reuse this specialization.
	g.genericSpecs[key] = name

	outerFn, outerBB, outerLocals, outerBindings := g.currentFn, g.currentBB, g.locals, g.typeBindings
	g.typeBindings = bindings
	fn, err := g.lowerFunctionAs(decl, concrete, name)
	g.currentFn, g.currentBB, g.locals, g.typeBindings = outerFn, outerBB, outerLocals, outerBindings
	if err != nil {
		delete(g.genericSpecs, key)
		return "", err
	}
	g.prog.Functions = append(g.prog.Functions, fn)
	return name, nil
}

func (g *generator) lowerAssignmentValue(e *ast.AssignExpr, current, rhs ir.Operand) ir.Operand {
	if e.Op == token.Eq {
		return rhs
	}
	resultType := current.Type()
	if t := g.semanticType(e); t != nil {
		resultType = t
	}
	if e.Op == token.PlusEq && resultType == types.TypeString {
		if current.Type() != types.TypeString || rhs.Type() != types.TypeString {
			return g.failExpr("native string += currently requires string operands")
		}
		res := g.currentFn.NewValue("str_assign", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_string_concat", Args: []ir.Operand{current, rhs}})
		return res
	}
	var op ir.BinaryOp
	switch e.Op {
	case token.PlusEq:
		op = ir.OpAdd
	case token.MinusEq:
		op = ir.OpSub
	case token.StarEq:
		op = ir.OpMul
	case token.SlashEq:
		op = ir.OpDiv
	default:
		return g.failExpr("unsupported assignment operator %s", e.Op)
	}
	if current.Type() != types.TypeNumber || rhs.Type() != types.TypeNumber {
		return g.failExpr("native %s currently requires number operands", e.Op)
	}
	res := g.currentFn.NewValue("assign", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: res, Op: op, LHS: current, RHS: rhs})
	return res
}

func (g *generator) coerceStringType(t types.Type, op ir.Operand) ir.Operand {
	if t == types.TypeString {
		return op
	}
	if isNumberSemanticType(t) {
		res := g.currentFn.NewValue("num_str", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_number_to_string", Args: []ir.Operand{op}})
		return res
	}
	if t == types.TypeBoolean {
		res := g.currentFn.NewValue("bool_str", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_bool_to_string", Args: []ir.Operand{op}})
		return res
	}
	if t == types.TypeNull {
		return ir.ConstString{Value: "null"}
	}
	if t == types.TypeUndefined {
		return ir.ConstString{Value: "undefined"}
	}
	return g.failExpr("native string coercion is not implemented for %s", t)
}

func (g *generator) coerceNullableUnionString(t *types.UnionType, op ir.Operand) ir.Operand {
	var concrete types.Type
	hasNull, hasUndefined := false, false
	for _, member := range t.Members {
		switch member.Kind() {
		case types.KindNull:
			hasNull = true
		case types.KindUndefined:
			hasUndefined = true
		default:
			if concrete != nil {
				return g.failExpr("native string coercion is not implemented for multi-representation union %s", t)
			}
			concrete = member
		}
	}
	if concrete == nil {
		return g.failExpr("native string coercion requires a concrete member in %s", t)
	}
	if concrete != types.TypeString && concrete != types.TypeBoolean && !isNumberSemanticType(concrete) {
		return g.failExpr("native string coercion is not implemented for %s", t)
	}

	join := g.currentFn.NewBlock("str_coerce_join")
	incoming := make([]ir.PhiIncoming, 0, 3)
	emitNullish := func(name string, sentinel ir.Operand, literal string) {
		cond := g.currentFn.NewValue(name+"_match", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: cond, Op: ir.OpEq, LHS: op, RHS: sentinel})
		match := g.currentFn.NewBlock(name)
		next := g.currentFn.NewBlock(name + "_next")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: cond, Then: match, Else: next}
		match.Terminator = &ir.JumpTerm{Target: join}
		incoming = append(incoming, ir.PhiIncoming{Block: match, Value: ir.ConstString{Value: literal}})
		g.currentBB = next
	}
	if hasUndefined {
		emitNullish("str_undefined", ir.ConstUndefined{}, "undefined")
	}
	if hasNull {
		emitNullish("str_null", ir.ConstNull{}, "null")
	}
	fallback := g.currentBB
	converted := g.coerceStringType(concrete, op)
	if fallback.Terminator == nil {
		fallback.Terminator = &ir.JumpTerm{Target: join}
	}
	incoming = append(incoming, ir.PhiIncoming{Block: fallback, Value: converted})
	g.currentBB = join
	res := g.currentFn.NewValue("str_coerce", types.TypeString)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: res, Incoming: incoming})
	return res
}

func (g *generator) coerceStringOperand(expr ast.Expr, op ir.Operand) ir.Operand {
	t := g.semanticType(expr)
	if t == nil {
		t = op.Type()
	}
	if union, ok := t.(*types.UnionType); ok {
		return g.coerceNullableUnionString(union, op)
	}
	return g.coerceStringType(t, op)
}

func restParamIndex(params []types.Param) int {
	for i, param := range params {
		if param.Rest {
			return i
		}
	}
	return -1
}

func (g *generator) packRestOperands(args []ir.Operand, fnType *types.FunctionType) []ir.Operand {
	if fnType == nil {
		return args
	}
	restIndex := restParamIndex(fnType.Params)
	if restIndex < 0 {
		return args
	}
	if restIndex >= len(fnType.Params) {
		return args
	}
	arrType, ok := fnType.Params[restIndex].Type.(*types.ArrayType)
	if !ok {
		g.failExpr("rest parameter %q has non-array native type %s", fnType.Params[restIndex].Name, fnType.Params[restIndex].Type)
		return args
	}
	if len(args) < restIndex {
		g.failExpr("call is missing %d fixed arguments before rest parameter", restIndex-len(args))
		return args
	}
	restCount := len(args) - restIndex
	rest := g.currentFn.NewValue("rest", arrType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: rest, ElemType: arrType.Elem, Length: ir.ConstNumber{Value: float64(restCount)},
	})
	for i, value := range args[restIndex:] {
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{
			Array: rest, Index: ir.ConstNumber{Value: float64(i)}, Val: value,
		})
	}
	packed := append([]ir.Operand(nil), args[:restIndex]...)
	packed = append(packed, rest)
	return packed
}

func (g *generator) pushArrayOperand(array ir.Operand, value ir.Operand) {
	length := g.currentFn.NewValue("push_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayPushInst{Res: length, Array: array, Val: value})
}

func (g *generator) appendSpreadArray(dst ir.Operand, src ir.Operand, elemType types.Type) {
	preheader := g.currentBB
	condBB := g.currentFn.NewBlock("spread_cond")
	bodyBB := g.currentFn.NewBlock("spread_body")
	doneBB := g.currentFn.NewBlock("spread_done")
	preheader.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("spread_i", types.TypeNumber)
	next := g.currentFn.NewValue("spread_next", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
		{Block: preheader, Value: ir.ConstNumber{Value: 0}},
		{Block: bodyBB, Value: next},
	}})
	length := g.currentFn.NewValue("spread_len", types.TypeNumber)
	condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: src})
	cond := g.currentFn.NewValue("spread_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: cond, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: cond, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	elem := g.currentFn.NewValue("spread_elem", elemType)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: elem, Array: src, Index: index})
	g.pushArrayOperand(dst, elem)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	bodyBB.Terminator = &ir.JumpTerm{Target: condBB}
	g.currentBB = doneBB
}

func isNumberSemanticType(t types.Type) bool {
	if t == nil {
		return false
	}
	if t == types.TypeNumber {
		return true
	}
	if u, ok := t.(*types.UnionType); ok {
		hasNumber := false
		for _, m := range u.Members {
			switch m.Kind() {
			case types.KindNumber:
				hasNumber = true
			case types.KindNull, types.KindUndefined:
			default:
				return false
			}
		}
		return hasNumber
	}
	return false
}

func (g *generator) collectArrowCaptures(expr ast.Expr, params map[string]struct{}) []string {
	found := make(map[string]struct{})
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		if e == nil {
			return
		}
		switch n := e.(type) {
		case *ast.IdentExpr:
			if _, isParam := params[n.Name]; isParam {
				return
			}
			if _, ok := g.locals[n.Name]; ok {
				found[n.Name] = struct{}{}
			}
		case *ast.BinaryExpr:
			walk(n.Left)
			walk(n.Right)
		case *ast.UnaryExpr:
			walk(n.Target)
		case *ast.CallExpr:
			walk(n.Callee)
			for _, a := range n.Args {
				walk(a)
			}
		case *ast.MemberExpr:
			walk(n.Object)
		case *ast.IndexExpr:
			walk(n.Target)
			walk(n.Index)
		case *ast.ArrayLit:
			for _, el := range n.Elements {
				walk(el)
			}
		case *ast.SpreadExpr:
			walk(n.Value)
		case *ast.ObjectLit:
			for _, prop := range n.Properties {
				walk(prop.Value)
			}
		case *ast.AssignExpr:
			walk(n.Left)
			walk(n.Right)
		case *ast.TernaryExpr:
			walk(n.Cond)
			walk(n.Then)
			walk(n.Else)
		case *ast.ArrowFuncExpr:
			// Nested arrows own their capture analysis.
		}
	}
	walk(expr)
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (g *generator) lowerArrowExpr(e *ast.ArrowFuncExpr) ir.Operand {
	if !e.IsExprBody {
		return g.failExpr("block-body arrow functions are not yet supported by native closure lowering")
	}
	body, ok := e.Body.(ast.Expr)
	if !ok {
		return g.failExpr("arrow expression body has unsupported node %T", e.Body)
	}
	fnType, ok := g.semanticType(e).(*types.FunctionType)
	if !ok {
		return g.failExpr("arrow function is missing a resolved function type")
	}
	paramSet := make(map[string]struct{}, len(e.Params))
	for _, p := range e.Params {
		paramSet[p.Name] = struct{}{}
	}
	captureNames := g.collectArrowCaptures(body, paramSet)
	captureOps := make([]ir.Operand, 0, len(captureNames))
	var refMask uint64
	for i, name := range captureNames {
		op := g.locals[name]
		captureOps = append(captureOps, op)
		if irHeapRefType(op.Type()) {
			if i >= 64 {
				return g.failExpr("closure capture %q exceeds the 64-bit GC reference mask", name)
			}
			refMask |= uint64(1) << i
		}
	}

	outerFn, outerBB, outerLocals := g.currentFn, g.currentBB, g.locals
	name := fmt.Sprintf("$arrow%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, fnType.Return)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)

	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	for i, captureName := range captureNames {
		captureType := captureOps[i].Type()
		v := lifted.NewValue(captureName+"_capture", captureType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: v, Closure: env, Index: i})
		g.locals[captureName] = v
	}
	for i, p := range e.Params {
		pt := types.TypeAny
		if i < len(fnType.Params) {
			pt = fnType.Params[i].Type
		}
		v := lifted.NewValue(p.Name, pt)
		lifted.Params = append(lifted.Params, v)
		g.locals[p.Name] = v
	}
	ret := g.lowerExpr(body)
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.ReturnTerm{Val: ret}
	}
	g.prog.Functions = append(g.prog.Functions, lifted)

	g.currentFn, g.currentBB, g.locals = outerFn, outerBB, outerLocals
	res := g.currentFn.NewValue("closure", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{
		Res: res, Function: name, Captures: captureOps, RefMask: refMask,
	})
	return res
}

func isConsoleLogCall(expr ast.Expr) bool {
	mem, ok := expr.(*ast.MemberExpr)
	if !ok || mem.Property != "log" {
		return false
	}
	ident, ok := mem.Object.(*ast.IdentExpr)
	return ok && ident.Name == "console"
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
	if info == nil || info.Instance == nil {
		return nil, fmt.Errorf("class %q is missing semantic metadata", cls.Name)
	}
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
	emitOwnInitializers := func() error {
		for _, field := range cls.Fields {
			if field.IsStatic || field.Init == nil {
				continue
			}
			offset, ok := offsets[field.Name]
			if !ok {
				return fmt.Errorf("class %s field %q is missing from instance layout", cls.Name, field.Name)
			}
			value := g.lowerExpr(field.Init)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: thisVal, Field: field.Name, Offset: offset, Val: value})
		}
		if method != nil {
			for _, p := range method.Params {
				if !p.IsParameterProperty {
					continue
				}
				offset, ok := offsets[p.Name]
				if !ok {
					return fmt.Errorf("class %s parameter property %q is missing from instance layout", cls.Name, p.Name)
				}
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: thisVal, Field: p.Name, Offset: offset, Val: g.locals[p.Name]})
			}
		}
		return nil
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
		if err := emitOwnInitializers(); err != nil {
			return nil, err
		}
	} else if constructor {
		if err := emitOwnInitializers(); err != nil {
			return nil, err
		}
	}

	if method != nil && method.Body != nil {
		for _, stmt := range method.Body.Statements[bodyStart:] {
			g.lowerStatement(stmt)
		}
	}
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}
	if g.err != nil {
		return nil, g.err
	}
	return irFn, nil
}

func (g *generator) lowerClassDecl(cls *ast.ClassDecl) error {
	info := g.semaResult.Classes[cls.Name]
	if info == nil {
		return fmt.Errorf("class %q is missing semantic metadata", cls.Name)
	}
	if len(cls.TypeParams) > 0 {
		// Generic classes are emitted only after concrete class specialization is
		// implemented. Their declarations have no standalone native ABI.
		return nil
	}

	outerFn, outerBB, outerLocals, outerBindings := g.currentFn, g.currentBB, g.locals, g.typeBindings
	defer func() {
		g.currentFn, g.currentBB, g.locals, g.typeBindings = outerFn, outerBB, outerLocals, outerBindings
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
		fn, err := g.lowerClassFunction(cls, info, method, fnType, classMethodName(info.Name, method.Name), false)
		if err != nil {
			return err
		}
		g.prog.Functions = append(g.prog.Functions, fn)
	}
	return nil
}

func (g *generator) ensureClassSpecialization(info *sema.ClassInfo) error {
	if info == nil || info.GenericBase == "" {
		return nil
	}
	if g.emittedClassSpecs[info.Name] {
		return nil
	}
	g.emittedClassSpecs[info.Name] = true
	outerFn, outerBB, outerLocals, outerBindings, outerClass := g.currentFn, g.currentBB, g.locals, g.typeBindings, g.currentClass
	defer func() {
		g.currentFn, g.currentBB, g.locals, g.typeBindings, g.currentClass = outerFn, outerBB, outerLocals, outerBindings, outerClass
	}()
	g.typeBindings = info.TypeBindings
	cls := info.Decl
	ctorDecl := classConstructorDecl(cls)
	ctor, err := g.lowerClassFunction(cls, info, ctorDecl, info.Constructor, classConstructorName(info.Name), true)
	if err != nil {
		delete(g.emittedClassSpecs, info.Name)
		return err
	}
	g.prog.Functions = append(g.prog.Functions, ctor)
	for i := range cls.Methods {
		method := &cls.Methods[i]
		if method.Name == "constructor" || method.IsStatic {
			continue
		}
		fnType := info.Methods[method.Name]
		fn, err := g.lowerClassFunction(cls, info, method, fnType, classMethodName(info.Name, method.Name), false)
		if err != nil {
			delete(g.emittedClassSpecs, info.Name)
			return err
		}
		g.prog.Functions = append(g.prog.Functions, fn)
	}
	return nil
}

// Generate lowers an AST program and its semantic facts into SSA IR.
func Generate(astProg *ast.Program, semaResult *sema.Result) (*ir.Program, error) {
	g := &generator{
		semaResult:        semaResult,
		prog:              &ir.Program{},
		genericDecls:      make(map[string]*ast.FunctionDecl),
		functionDecls:     make(map[string]*ast.FunctionDecl),
		genericSpecs:      make(map[string]string),
		classTags:         make(map[string]int),
		emittedClassSpecs: make(map[string]bool),
	}
	classNames := make([]string, 0, len(semaResult.Classes))
	for name := range semaResult.Classes {
		classNames = append(classNames, name)
	}
	sort.Strings(classNames)
	for i, name := range classNames {
		g.classTags[name] = i + 1
	}

	for _, stmt := range astProg.Statements {
		if fnDecl, ok := stmt.(*ast.FunctionDecl); ok {
			g.functionDecls[fnDecl.Name] = fnDecl
			if len(fnDecl.TypeParams) > 0 {
				g.genericDecls[fnDecl.Name] = fnDecl
			}
		}
	}
	for _, stmt := range astProg.Statements {
		if cls, ok := stmt.(*ast.ClassDecl); ok {
			if err := g.lowerClassDecl(cls); err != nil {
				return nil, err
			}
		}
	}

	var topStmts []ast.Stmt
	for _, stmt := range astProg.Statements {
		if fnDecl, ok := stmt.(*ast.FunctionDecl); ok {
			if len(fnDecl.TypeParams) > 0 {
				continue
			}
			fn, err := g.lowerFunction(fnDecl)
			if err != nil {
				return nil, err
			}
			g.prog.Functions = append(g.prog.Functions, fn)
		} else if _, isClass := stmt.(*ast.ClassDecl); !isClass {
			topStmts = append(topStmts, stmt)
		}
	}

	if len(topStmts) > 0 {
		mainFn := g.lowerTopLevel(topStmts)
		// Put @main at the front of Functions so it's the entrypoint
		g.prog.Functions = append([]*ir.Function{mainFn}, g.prog.Functions...)
	}

	if g.err != nil {
		return nil, g.err
	}
	return g.prog, nil
}

func (g *generator) lowerTopLevel(stmts []ast.Stmt) *ir.Function {
	irFn := ir.NewFunction("@main", types.TypeVoid)
	g.currentFn = irFn
	g.locals = make(map[string]ir.Operand)

	entryBB := irFn.NewBlock("entry")
	g.currentBB = entryBB

	for _, s := range stmts {
		g.lowerStatement(s)
	}

	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}

	return irFn
}

func (g *generator) lowerFunction(fnDecl *ast.FunctionDecl) (*ir.Function, error) {
	fnType, _ := g.semaResult.Types[fnDecl].(*types.FunctionType)
	return g.lowerFunctionAs(fnDecl, fnType, fnDecl.Name)
}

func (g *generator) lowerFunctionAs(fnDecl *ast.FunctionDecl, fnType *types.FunctionType, name string) (*ir.Function, error) {
	var retType types.Type = types.TypeVoid
	if fnType != nil {
		retType = fnType.Return
	}

	irFn := ir.NewFunction(name, retType)
	g.currentFn = irFn
	g.locals = make(map[string]ir.Operand)

	entryBB := irFn.NewBlock("entry")
	g.currentBB = entryBB

	for i, p := range fnDecl.Params {
		pType := types.TypeNumber
		if fnType != nil && i < len(fnType.Params) {
			pType = fnType.Params[i].Type
		}
		val := irFn.NewValue(p.Name, pType)
		irFn.Params = append(irFn.Params, val)
		g.locals[p.Name] = val
	}

	if fnDecl.Body != nil {
		for _, stmt := range fnDecl.Body.Statements {
			g.lowerStatement(stmt)
		}
	}

	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}
	if g.err != nil {
		return nil, g.err
	}
	return irFn, nil
}

func (g *generator) lowerStatement(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		for _, child := range s.Statements {
			g.lowerStatement(child)
		}
	case *ast.VarDeclStmt:
		for _, d := range s.Declarations {
			var initOp ir.Operand
			if d.Init != nil {
				initOp = g.lowerExpr(d.Init)
			}
			if initOp == nil {
				initOp = ir.ConstNumber{Value: 0}
			}
			g.locals[d.Name] = initOp
		}
	case *ast.ReturnStmt:
		var val ir.Operand
		if s.Value != nil {
			val = g.lowerExpr(s.Value)
		}
		g.currentBB.Terminator = &ir.ReturnTerm{Val: val}
	case *ast.ExprStmt:
		g.lowerExpr(s.Expr)
	case *ast.IfStmt:
		g.lowerIf(s)
	case *ast.WhileStmt:
		g.lowerWhile(s)
	case *ast.DoWhileStmt:
		g.lowerDoWhile(s)
	case *ast.ForStmt:
		g.lowerFor(s)
	case *ast.ForOfStmt:
		g.lowerForOf(s)
	case *ast.SwitchStmt:
		g.lowerSwitch(s)
	case *ast.BreakStmt:
		if g.err == nil {
			g.err = fmt.Errorf("break outside supported switch lowering")
		}
	case *ast.ContinueStmt:
		if g.err == nil {
			g.err = fmt.Errorf("continue lowering is not implemented")
		}
	}
}

func findModifiedVars(stmt ast.Stmt) map[string]bool {
	res := make(map[string]bool)
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		if n == nil {
			return
		}
		switch node := n.(type) {
		case *ast.AssignExpr:
			if ident, ok := node.Left.(*ast.IdentExpr); ok {
				res[ident.Name] = true
			}
			walk(node.Right)
		case *ast.UnaryExpr:
			if node.Op == token.PlusPlus || node.Op == token.MinusMinus {
				if ident, ok := node.Target.(*ast.IdentExpr); ok {
					res[ident.Name] = true
				}
			}
			walk(node.Target)
		case *ast.BlockStmt:
			for _, s := range node.Statements {
				walk(s)
			}
		case *ast.ExprStmt:
			walk(node.Expr)
		case *ast.IfStmt:
			walk(node.Cond)
			walk(node.Then)
			walk(node.Else)
		case *ast.WhileStmt:
			walk(node.Cond)
			walk(node.Body)
		case *ast.DoWhileStmt:
			walk(node.Body)
			walk(node.Cond)
		case *ast.ForStmt:
			walk(node.Init)
			walk(node.Cond)
			walk(node.Post)
			walk(node.Body)
		case *ast.ForOfStmt:
			walk(node.Iterable)
			walk(node.Body)
		case *ast.SwitchStmt:
			walk(node.Expr)
			for _, clause := range node.Cases {
				walk(clause.Test)
				for _, stmt := range clause.Statements {
					walk(stmt)
				}
			}
		}
	}
	walk(stmt)
	return res
}

func (g *generator) lowerForOf(s *ast.ForOfStmt) {
	iterable := g.lowerExpr(s.Iterable)
	arrType, ok := g.semaResult.Types[s.Iterable].(*types.ArrayType)
	if !ok {
		if g.err == nil {
			g.err = fmt.Errorf("native for-of currently requires an array iterable")
		}
		return
	}

	preBB := g.currentBB
	condBB := g.currentFn.NewBlock("forof_cond")
	bodyBB := g.currentFn.NewBlock("forof_body")
	postBB := g.currentFn.NewBlock("forof_post")
	exitBB := g.currentFn.NewBlock("forof_exit")
	if preBB.Terminator == nil {
		preBB.Terminator = &ir.JumpTerm{Target: condBB}
	}

	modVars := findModifiedVars(s.Body)
	delete(modVars, s.Name)
	loopPhis := make(map[string]*ir.PhiInst)
	for name := range modVars {
		if val, exists := g.locals[name]; exists {
			phiVal := g.currentFn.NewValue(fmt.Sprintf("%s_forof", name), val.Type())
			phi := &ir.PhiInst{Res: phiVal, Incoming: []ir.PhiIncoming{{Block: preBB, Value: val}}}
			loopPhis[name] = phi
			condBB.Phis = append(condBB.Phis, phi)
			g.locals[name] = phiVal
		}
	}
	index := g.currentFn.NewValue("forof_i", types.TypeNumber)
	indexPhi := &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: preBB, Value: ir.ConstNumber{Value: 0}}}}
	condBB.Phis = append(condBB.Phis, indexPhi)

	g.currentBB = condBB
	length := g.currentFn.NewValue("forof_len", types.TypeNumber)
	condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: iterable})
	cond := g.currentFn.NewValue("forof_has", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: cond, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: cond, Then: bodyBB, Else: exitBB}

	previousLoopVar, hadPrevious := g.locals[s.Name]
	g.currentBB = bodyBB
	elem := g.currentFn.NewValue(s.Name, arrType.Elem)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: elem, Array: iterable, Index: index})
	g.locals[s.Name] = elem
	g.lowerStatement(s.Body)
	bodyEnd := g.currentBB
	if bodyEnd.Terminator == nil {
		bodyEnd.Terminator = &ir.JumpTerm{Target: postBB}
	}

	g.currentBB = postBB
	nextIndex := g.currentFn.NewValue("forof_next", types.TypeNumber)
	postBB.Instructions = append(postBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	postEnd := g.currentBB
	if postEnd.Terminator == nil {
		postEnd.Terminator = &ir.JumpTerm{Target: condBB}
	}
	indexPhi.Incoming = append(indexPhi.Incoming, ir.PhiIncoming{Block: postEnd, Value: nextIndex})
	for name, phi := range loopPhis {
		phi.Incoming = append(phi.Incoming, ir.PhiIncoming{Block: postEnd, Value: g.locals[name]})
		g.locals[name] = phi.Res
	}
	if hadPrevious {
		g.locals[s.Name] = previousLoopVar
	} else {
		delete(g.locals, s.Name)
	}
	g.currentBB = exitBB
}

func (g *generator) lowerFor(s *ast.ForStmt) {
	if s.Init != nil {
		g.lowerStatement(s.Init)
	}
	preBB := g.currentBB
	condBB := g.currentFn.NewBlock("for_cond")
	bodyBB := g.currentFn.NewBlock("for_body")
	postBB := g.currentFn.NewBlock("for_post")
	exitBB := g.currentFn.NewBlock("for_exit")

	if preBB.Terminator == nil {
		preBB.Terminator = &ir.JumpTerm{Target: condBB}
	}

	modVars := findModifiedVars(s.Body)
	if s.Post != nil {
		for k, v := range findModifiedVars(&ast.ExprStmt{Expr: s.Post}) {
			if v {
				modVars[k] = true
			}
		}
	}

	loopPhis := make(map[string]*ir.PhiInst)
	for name := range modVars {
		if val, exists := g.locals[name]; exists {
			phiVal := g.currentFn.NewValue(fmt.Sprintf("%s_loop", name), val.Type())
			phi := &ir.PhiInst{
				Res: phiVal,
				Incoming: []ir.PhiIncoming{
					{Block: preBB, Value: val},
				},
			}
			loopPhis[name] = phi
			condBB.Phis = append(condBB.Phis, phi)
			g.locals[name] = phiVal
		}
	}

	g.currentBB = condBB
	if s.Cond != nil {
		cond := g.lowerExpr(s.Cond)
		condBB.Terminator = &ir.BranchTerm{Cond: cond, Then: bodyBB, Else: exitBB}
	} else {
		condBB.Terminator = &ir.JumpTerm{Target: bodyBB}
	}

	g.currentBB = bodyBB
	g.lowerStatement(s.Body)
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: postBB}
	}

	g.currentBB = postBB
	if s.Post != nil {
		g.lowerExpr(s.Post)
	}
	postEndBB := g.currentBB
	if postEndBB.Terminator == nil {
		postEndBB.Terminator = &ir.JumpTerm{Target: condBB}
	}

	for name, phi := range loopPhis {
		updatedVal := g.locals[name]
		phi.Incoming = append(phi.Incoming, ir.PhiIncoming{
			Block: postEndBB,
			Value: updatedVal,
		})
		g.locals[name] = phi.Res
	}

	g.currentBB = exitBB
}

func (g *generator) lowerIf(s *ast.IfStmt) {
	cond := g.lowerExpr(s.Cond)
	thenBB := g.currentFn.NewBlock("then")
	elseBB := g.currentFn.NewBlock("else")
	joinBB := g.currentFn.NewBlock("join")

	g.currentBB.Terminator = &ir.BranchTerm{
		Cond: cond,
		Then: thenBB,
		Else: elseBB,
	}

	// Save locals snapshot
	origLocals := make(map[string]ir.Operand)
	for k, v := range g.locals {
		origLocals[k] = v
	}

	// Then block
	g.currentBB = thenBB
	g.lowerStatement(s.Then)
	thenEndBB := g.currentBB
	if thenEndBB.Terminator == nil {
		thenEndBB.Terminator = &ir.JumpTerm{Target: joinBB}
	}
	thenLocals := make(map[string]ir.Operand)
	for k, v := range g.locals {
		thenLocals[k] = v
	}

	// Restore locals for else block
	g.locals = make(map[string]ir.Operand)
	for k, v := range origLocals {
		g.locals[k] = v
	}

	// Else block
	g.currentBB = elseBB
	if s.Else != nil {
		g.lowerStatement(s.Else)
	}
	elseEndBB := g.currentBB
	if elseEndBB.Terminator == nil {
		elseEndBB.Terminator = &ir.JumpTerm{Target: joinBB}
	}
	elseLocals := make(map[string]ir.Operand)
	for k, v := range g.locals {
		elseLocals[k] = v
	}

	// Join block - insert SSA phi nodes for modified variables
	g.currentBB = joinBB
	for name, origVal := range origLocals {
		thenVal := thenLocals[name]
		elseVal := elseLocals[name]
		if thenVal != elseVal {
			phiVal := g.currentFn.NewValue(fmt.Sprintf("%s_phi", name), origVal.Type())
			phi := &ir.PhiInst{
				Res: phiVal,
				Incoming: []ir.PhiIncoming{
					{Block: thenEndBB, Value: thenVal},
					{Block: elseEndBB, Value: elseVal},
				},
			}
			joinBB.Phis = append(joinBB.Phis, phi)
			g.locals[name] = phiVal
		} else {
			g.locals[name] = origVal
		}
	}
}

func (g *generator) lowerWhile(s *ast.WhileStmt) {
	preBB := g.currentBB
	condBB := g.currentFn.NewBlock("while_cond")
	bodyBB := g.currentFn.NewBlock("while_body")
	exitBB := g.currentFn.NewBlock("while_exit")

	preBB.Terminator = &ir.JumpTerm{Target: condBB}

	modVars := findModifiedVars(s.Body)
	loopPhis := make(map[string]*ir.PhiInst)
	for name := range modVars {
		if val, exists := g.locals[name]; exists {
			phiVal := g.currentFn.NewValue(fmt.Sprintf("%s_loop", name), val.Type())
			phi := &ir.PhiInst{
				Res: phiVal,
				Incoming: []ir.PhiIncoming{
					{Block: preBB, Value: val},
				},
			}
			loopPhis[name] = phi
			condBB.Phis = append(condBB.Phis, phi)
			g.locals[name] = phiVal
		}
	}

	g.currentBB = condBB
	cond := g.lowerExpr(s.Cond)
	condBB.Terminator = &ir.BranchTerm{
		Cond: cond,
		Then: bodyBB,
		Else: exitBB,
	}

	g.currentBB = bodyBB
	g.lowerStatement(s.Body)
	bodyEndBB := g.currentBB
	if bodyEndBB.Terminator == nil {
		bodyEndBB.Terminator = &ir.JumpTerm{Target: condBB}
	}

	for name, phi := range loopPhis {
		updatedVal := g.locals[name]
		phi.Incoming = append(phi.Incoming, ir.PhiIncoming{
			Block: bodyEndBB,
			Value: updatedVal,
		})
		g.locals[name] = phi.Res
	}

	g.currentBB = exitBB
}

func (g *generator) lowerSwitch(s *ast.SwitchStmt) {
	discr := g.lowerExpr(s.Expr)
	tempName := fmt.Sprintf("$switch%d", g.arrowCounter)
	g.arrowCounter++
	g.locals[tempName] = discr

	var defaultClause *ast.SwitchCase
	for i, clause := range s.Cases {
		if clause.Test == nil && i != len(s.Cases)-1 {
			if g.err == nil {
				g.err = fmt.Errorf("native switch lowering currently requires default to be the final clause")
			}
			return
		}
	}
	var chain ast.Stmt
	for i := len(s.Cases) - 1; i >= 0; i-- {
		clause := s.Cases[i]
		stmts := append([]ast.Stmt(nil), clause.Statements...)
		terminated := false
		if len(stmts) > 0 {
			switch stmts[len(stmts)-1].(type) {
			case *ast.BreakStmt:
				stmts = stmts[:len(stmts)-1]
				terminated = true
			case *ast.ReturnStmt:
				terminated = true
			}
		}
		if !terminated && i != len(s.Cases)-1 {
			if g.err == nil {
				g.err = fmt.Errorf("switch fallthrough is not yet supported in native lowering")
			}
			return
		}
		block := &ast.BlockStmt{SourceSpan: clause.SourceSpan, Statements: stmts}
		if clause.Test == nil {
			if defaultClause != nil {
				if g.err == nil {
					g.err = fmt.Errorf("switch contains multiple default clauses")
				}
				return
			}
			copyClause := clause
			defaultClause = &copyClause
			chain = block
			continue
		}
		left := &ast.IdentExpr{SourceSpan: s.Expr.Span(), Name: tempName}
		cond := &ast.BinaryExpr{SourceSpan: clause.SourceSpan, Left: left, Op: token.EqEqEq, Right: clause.Test}
		g.semaResult.Types[left] = discr.Type()
		g.semaResult.Types[cond] = types.TypeBoolean
		chain = &ast.IfStmt{SourceSpan: clause.SourceSpan, Cond: cond, Then: block, Else: chain}
	}
	if chain != nil {
		g.lowerStatement(chain)
	}
	delete(g.locals, tempName)
}

func (g *generator) lowerDoWhile(s *ast.DoWhileStmt) {
	preBB := g.currentBB
	bodyBB := g.currentFn.NewBlock("do_body")
	condBB := g.currentFn.NewBlock("do_cond")
	exitBB := g.currentFn.NewBlock("do_exit")
	if preBB.Terminator == nil {
		preBB.Terminator = &ir.JumpTerm{Target: bodyBB}
	}

	modVars := findModifiedVars(s.Body)
	for name := range findModifiedVars(&ast.ExprStmt{Expr: s.Cond}) {
		modVars[name] = true
	}
	loopPhis := make(map[string]*ir.PhiInst)
	for name := range modVars {
		if val, exists := g.locals[name]; exists {
			phiVal := g.currentFn.NewValue(fmt.Sprintf("%s_do", name), val.Type())
			phi := &ir.PhiInst{Res: phiVal, Incoming: []ir.PhiIncoming{{Block: preBB, Value: val}}}
			loopPhis[name] = phi
			bodyBB.Phis = append(bodyBB.Phis, phi)
			g.locals[name] = phiVal
		}
	}

	g.currentBB = bodyBB
	g.lowerStatement(s.Body)
	bodyEnd := g.currentBB
	if bodyEnd.Terminator == nil {
		bodyEnd.Terminator = &ir.JumpTerm{Target: condBB}
	}

	g.currentBB = condBB
	cond := g.lowerExpr(s.Cond)
	condEnd := g.currentBB
	if condEnd.Terminator == nil {
		condEnd.Terminator = &ir.BranchTerm{Cond: cond, Then: bodyBB, Else: exitBB}
	}
	for name, phi := range loopPhis {
		updated := g.locals[name]
		phi.Incoming = append(phi.Incoming, ir.PhiIncoming{Block: condEnd, Value: updated})
	}

	g.currentBB = exitBB
}

func (g *generator) lowerLogicalExpr(e *ast.BinaryExpr) ir.Operand {
	lhs := g.lowerExpr(e.Left)
	lhsBB := g.currentBB
	rhsBB := g.currentFn.NewBlock("logical_rhs")
	shortBB := g.currentFn.NewBlock("logical_short")
	joinBB := g.currentFn.NewBlock("logical_join")

	if e.Op == token.AmpAmp {
		lhsBB.Terminator = &ir.BranchTerm{Cond: lhs, Then: rhsBB, Else: shortBB}
	} else {
		lhsBB.Terminator = &ir.BranchTerm{Cond: lhs, Then: shortBB, Else: rhsBB}
	}

	g.currentBB = shortBB
	shortVal := lhs
	shortEnd := g.currentBB
	shortEnd.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = rhsBB
	rhsVal := g.lowerExpr(e.Right)
	rhsEnd := g.currentBB
	if rhsEnd.Terminator == nil {
		rhsEnd.Terminator = &ir.JumpTerm{Target: joinBB}
	}

	g.currentBB = joinBB
	resultType := rhsVal.Type()
	if t := g.semanticType(e); t != nil {
		resultType = t
	}
	res := g.currentFn.NewValue("logical", resultType)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{
		Res: res,
		Incoming: []ir.PhiIncoming{
			{Block: shortEnd, Value: shortVal},
			{Block: rhsEnd, Value: rhsVal},
		},
	})
	return res
}

func (g *generator) lowerExpr(expr ast.Expr) ir.Operand {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *ast.NumberLit:
		return ir.ConstNumber{Value: e.Value}
	case *ast.StringLit:
		return ir.ConstString{Value: e.Value}
	case *ast.BoolLit:
		return ir.ConstBool{Value: e.Value}
	case *ast.NullLit:
		return ir.ConstNull{}
	case *ast.UndefinedLit:
		return ir.ConstUndefined{}
	case *ast.ThisExpr:
		if thisVal, ok := g.locals["$this"]; ok {
			return thisVal
		}
		return g.failExpr("cannot lower 'this' outside a native class function")
	case *ast.SuperExpr:
		return g.failExpr("super lowering is reserved for the inheritance phase")
	case *ast.NewExpr:
		info := g.semaResult.GenericClasses[e]
		if info == nil {
			info = g.semaResult.Classes[e.ClassName]
		}
		if info == nil || info.Instance == nil {
			return g.failExpr("cannot lower new %s without class metadata", e.ClassName)
		}
		if len(info.TypeParams) > 0 {
			return g.failExpr("generic class %s is missing a concrete semantic specialization", e.ClassName)
		}
		if err := g.ensureClassSpecialization(info); err != nil {
			return g.failExpr("emit class specialization %s: %v", info.Name, err)
		}
		offsets, refMask, shape := g.objectLayout(info.Instance)
		obj := g.currentFn.NewValue("instance", info.Instance)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
			Res: obj, Shape: shape, FieldCount: len(offsets) + 1, RefMask: refMask,
		})
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: obj, Field: "$class", Offset: 16, Val: ir.ConstNumber{Value: float64(g.classTag(info.Name))}})
		args := make([]ir.Operand, 0, len(e.Args)+1)
		args = append(args, obj)
		for _, arg := range e.Args {
			args = append(args, g.lowerExpr(arg))
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Callee: classConstructorName(info.Name), Args: args,
		})
		return obj
	case *ast.SpreadExpr:
		return g.lowerExpr(e.Value)
	case *ast.ArrayLit:
		arrType := types.NewArray(types.TypeAny)
		if semantic := g.semanticType(e); semantic != nil {
			if tuple, ok := semantic.(*types.TupleType); ok {
				if len(tuple.Elements) != len(e.Elements) {
					return g.failExpr("tuple literal has %d elements, expected %d", len(e.Elements), len(tuple.Elements))
				}
				for _, el := range e.Elements {
					if _, spread := el.(*ast.SpreadExpr); spread {
						return g.failExpr("native tuple literals do not support spread elements yet")
					}
				}
				res := g.currentFn.NewValue("tuple", tuple)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
					Res: res, Shape: tuple.String(), FieldCount: len(tuple.Elements), RefMask: g.tupleRefMask(tuple),
				})
				for i, el := range e.Elements {
					val := g.lowerExpr(el)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
						Obj: res, Field: strconv.Itoa(i), Offset: 16 + i*8, Val: val,
					})
				}
				return res
			}
			if t, ok := semantic.(*types.ArrayType); ok {
				arrType = t
			}
		}
		hasSpread := false
		for _, el := range e.Elements {
			if _, ok := el.(*ast.SpreadExpr); ok {
				hasSpread = true
				break
			}
		}
		res := g.currentFn.NewValue("arr", arrType)
		initialLength := float64(len(e.Elements))
		if hasSpread {
			initialLength = 0
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
			Res: res, ElemType: arrType.Elem, Length: ir.ConstNumber{Value: initialLength},
		})
		for i, el := range e.Elements {
			if spread, ok := el.(*ast.SpreadExpr); ok {
				sourceType, ok := g.semanticType(spread.Value).(*types.ArrayType)
				if !ok {
					return g.failExpr("native array spread requires an array source")
				}
				source := g.lowerExpr(spread.Value)
				g.appendSpreadArray(res, source, sourceType.Elem)
				continue
			}
			val := g.lowerExpr(el)
			if hasSpread {
				g.pushArrayOperand(res, val)
			} else {
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{
					Array: res, Index: ir.ConstNumber{Value: float64(i)}, Val: val,
				})
			}
		}
		return res
	case *ast.ObjectLit:
		objType, _ := g.semanticType(e).(*types.ObjectType)
		if objType == nil {
			return g.failExpr("cannot lower object literal without a closed object type")
		}
		offsets, refMask, shape := g.objectLayout(objType)
		res := g.currentFn.NewValue("obj", objType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
		for _, prop := range e.Properties {
			val := g.lowerExpr(prop.Value)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: prop.Key, Offset: offsets[prop.Key], Val: val})
		}
		return res
	case *ast.IdentExpr:
		if op, exists := g.locals[e.Name]; exists {
			return op
		}
		if sym := g.semaResult.Symbols[e]; sym != nil && sym.Kind == sema.SymFunc {
			fnType, ok := sym.Type.(*types.FunctionType)
			if !ok {
				return g.failExpr("function symbol %q has non-function type %T", e.Name, sym.Type)
			}
			if len(fnType.TypeParams) > 0 {
				return g.failExpr("generic function %q requires specialization before use as a value", e.Name)
			}
			res := g.currentFn.NewValue("closure", fnType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: res, Function: e.Name})
			return res
		}
		return g.failExpr("cannot lower unresolved or non-local identifier %q as a value", e.Name)
	case *ast.BinaryExpr:
		if e.Op == token.AmpAmp || e.Op == token.PipePipe {
			return g.lowerLogicalExpr(e)
		}

		lhs := g.lowerExpr(e.Left)
		rhs := g.lowerExpr(e.Right)

		if e.Op == token.Plus {
			isString := false
			if g.semanticType(e) == types.TypeString || g.semanticType(e.Left) == types.TypeString || g.semanticType(e.Right) == types.TypeString {
				isString = true
			}
			if isString {
				lhs = g.coerceStringOperand(e.Left, lhs)
				rhs = g.coerceStringOperand(e.Right, rhs)
				resVal := g.currentFn.NewValue("str", types.TypeString)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: resVal, Callee: "ts_string_concat", Args: []ir.Operand{lhs, rhs}})
				return resVal
			}
		}

		op := ir.OpAdd
		switch e.Op {
		case token.Plus:
			op = ir.OpAdd
		case token.Minus:
			op = ir.OpSub
		case token.Star:
			op = ir.OpMul
		case token.Slash:
			op = ir.OpDiv
		case token.Percent:
			op = ir.OpMod
		case token.EqEq, token.EqEqEq:
			op = ir.OpEq
		case token.BangEq, token.BangEqEq:
			op = ir.OpNe
		case token.Lt:
			op = ir.OpLt
		case token.LtEq:
			op = ir.OpLe
		case token.Gt:
			op = ir.OpGt
		case token.GtEq:
			op = ir.OpGe
		case token.AmpAmp:
			op = ir.OpAnd
		case token.PipePipe:
			op = ir.OpOr
		default:
			return g.failExpr("unsupported binary operator %s", e.Op)
		}
		resultType := types.TypeNumber
		if t := g.semanticType(e); t != nil {
			resultType = t
		}
		resVal := g.currentFn.NewValue("t", resultType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
			Res: resVal,
			Op:  op,
			LHS: lhs,
			RHS: rhs,
		})
		return resVal
	case *ast.ArrowFuncExpr:
		return g.lowerArrowExpr(e)
	case *ast.UnaryExpr:
		if e.Op == token.PlusPlus || e.Op == token.MinusMinus {
			if ident, ok := e.Target.(*ast.IdentExpr); ok {
				currVal, exists := g.locals[ident.Name]
				if !exists {
					return g.failExpr("cannot lower %s for unresolved local %q", e.Op, ident.Name)
				}
				op := ir.OpAdd
				if e.Op == token.MinusMinus {
					op = ir.OpSub
				}
				nextVal := g.currentFn.NewValue(ident.Name+"_inc", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
					Res: nextVal,
					Op:  op,
					LHS: currVal,
					RHS: ir.ConstNumber{Value: 1},
				})
				g.locals[ident.Name] = nextVal
				if e.Prefix {
					return nextVal
				}
				return currVal
			}
		} else if e.Op == token.Minus {
			target := g.lowerExpr(e.Target)
			if c, ok := target.(ir.ConstNumber); ok {
				return ir.ConstNumber{Value: -c.Value}
			}
			resVal := g.currentFn.NewValue("neg", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.UnaryInst{Res: resVal, Op: "-", Val: target})
			return resVal
		} else if e.Op == token.Bang {
			target := g.lowerExpr(e.Target)
			resVal := g.currentFn.NewValue("not", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
				Res: resVal,
				Op:  ir.OpEq,
				LHS: target,
				RHS: ir.ConstNumber{Value: 0},
			})
			return resVal
		} else if e.Op == token.Plus {
			return g.lowerExpr(e.Target)
		}
		return g.failExpr("unsupported unary operator %s", e.Op)
	case *ast.IndexExpr:
		if tuple, ok := g.semanticType(e.Target).(*types.TupleType); ok {
			lit, ok := e.Index.(*ast.NumberLit)
			if !ok {
				return g.failExpr("native tuple indexing currently requires a constant numeric index")
			}
			idx := int(lit.Value)
			if float64(idx) != lit.Value || idx < 0 || idx >= len(tuple.Elements) {
				return g.failExpr("tuple index %v is outside [0,%d)", lit.Value, len(tuple.Elements))
			}
			tupleVal := g.lowerExpr(e.Target)
			resultType := tuple.Elements[idx]
			if t := g.semanticType(e); t != nil {
				resultType = t
			}
			res := g.currentFn.NewValue("tuple_elem", resultType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
				Res: res, Obj: tupleVal, Field: strconv.Itoa(idx), Offset: 16 + idx*8,
			})
			return res
		}
		array := g.lowerExpr(e.Target)
		index := g.lowerExpr(e.Index)
		resultType := types.TypeAny
		if t := g.semanticType(e); t != nil {
			resultType = t
		}
		res := g.currentFn.NewValue("elem", resultType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: res, Array: array, Index: index})
		return res
	case *ast.MemberExpr:
		if ident, ok := e.Object.(*ast.IdentExpr); ok {
			if members := g.semaResult.Enums[ident.Name]; members != nil {
				if value, exists := members[e.Property]; exists {
					return ir.ConstNumber{Value: value}
				}
				return g.failExpr("enum %s has no member %s", ident.Name, e.Property)
			}
		}
		if tuple, ok := g.semanticType(e.Object).(*types.TupleType); ok && e.Property == "length" {
			return ir.ConstNumber{Value: float64(len(tuple.Elements))}
		}
		if _, ok := g.semanticType(e.Object).(*types.ArrayType); ok && e.Property == "length" {
			array := g.lowerExpr(e.Object)
			res := g.currentFn.NewValue("len", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: res, Array: array})
			return res
		}
		if objType, ok := g.semanticType(e.Object).(*types.ObjectType); ok {
			offsets, _, _ := g.objectLayout(objType)
			offset, exists := offsets[e.Property]
			if !exists {
				return g.failExpr("object shape has no field %q", e.Property)
			}
			obj := g.lowerExpr(e.Object)
			resultType := types.TypeAny
			if t := g.semanticType(e); t != nil {
				resultType = t
			}
			res := g.currentFn.NewValue("field", resultType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: e.Property, Offset: offset})
			return res
		}
		return g.failExpr("unsupported member access .%s", e.Property)
	case *ast.CallExpr:
		if _, ok := e.Callee.(*ast.SuperExpr); ok {
			if g.currentClass == nil || g.currentClass.BaseName == "" {
				return g.failExpr("cannot lower super(...) outside a derived class constructor")
			}
			thisVal, ok := g.locals["$this"]
			if !ok {
				return g.failExpr("derived constructor is missing native this value")
			}
			args := make([]ir.Operand, 0, len(e.Args)+1)
			args = append(args, thisVal)
			for _, arg := range e.Args {
				args = append(args, g.lowerExpr(arg))
			}
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: classConstructorName(g.currentClass.BaseName), Args: args})
			return nil
		}
		if mem, ok := e.Callee.(*ast.MemberExpr); ok {
			if staticType, ok := g.semanticType(mem.Object).(*types.ObjectType); ok {
				if staticInfo := g.semaResult.Classes[staticType.Name]; staticInfo != nil && staticInfo.Methods[mem.Property] != nil {
					obj := g.lowerExpr(mem.Object)
					callArgs := make([]ir.Operand, 0, len(e.Args))
					for _, arg := range e.Args {
						callArgs = append(callArgs, g.lowerExpr(arg))
					}
					return g.emitClassMethodCall(obj, staticInfo, mem.Property, callArgs)
				}
			}
			if arrType, ok := g.semanticType(mem.Object).(*types.ArrayType); ok {
				array := g.lowerExpr(mem.Object)
				switch mem.Property {
				case "push":
					if len(e.Args) != 1 {
						return g.failExpr("array.push expects exactly one argument in native lowering")
					}
					val := g.lowerExpr(e.Args[0])
					res := g.currentFn.NewValue("len", types.TypeNumber)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayPushInst{Res: res, Array: array, Val: val})
					return res
				case "pop":
					res := g.currentFn.NewValue("elem", arrType.Elem)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayPopInst{Res: res, Array: array})
					return res
				}
			}
		}
		if fnType, ok := g.semanticType(e.Callee).(*types.FunctionType); ok && !isConsoleLogCall(e.Callee) {
			directNamed := false
			if ident, isIdent := e.Callee.(*ast.IdentExpr); isIdent {
				_, isLocal := g.locals[ident.Name]
				if sym := g.semaResult.Symbols[ident]; !isLocal && sym != nil && sym.Kind == sema.SymFunc {
					directNamed = true
				}
			}
			if !directNamed {
				closure := g.lowerExpr(e.Callee)
				args := make([]ir.Operand, 0, len(e.Args))
				for _, arg := range e.Args {
					args = append(args, g.lowerExpr(arg))
				}
				args = g.packRestOperands(args, fnType)
				res := g.currentFn.NewValue("ret", fnType.Return)
				paramTypes := make([]types.Type, len(fnType.Params))
				for i := range fnType.Params {
					paramTypes[i] = fnType.Params[i].Type
				}
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{Res: res, Closure: closure, Args: args, ParamTypes: paramTypes})
				return res
			}
		}
		calleeName := "unknown"
		if ident, ok := e.Callee.(*ast.IdentExpr); ok {
			calleeName = ident.Name
			if decl := g.genericDecls[ident.Name]; decl != nil {
				concrete := g.semaResult.GenericCalls[e]
				if concrete == nil {
					return g.failExpr("generic call %q is missing a semantic instantiation", ident.Name)
				}
				if len(g.typeBindings) > 0 {
					substituted, ok := types.Substitute(concrete, g.typeBindings).(*types.FunctionType)
					if !ok {
						return g.failExpr("generic call %q substitution produced %T", ident.Name, substituted)
					}
					concrete = substituted
				}
				specialized, err := g.ensureGenericSpecialization(decl, concrete)
				if err != nil {
					return g.failExpr("specialize %q: %v", ident.Name, err)
				}
				calleeName = specialized
			}
		} else if mem, ok := e.Callee.(*ast.MemberExpr); ok {
			if objIdent, ok := mem.Object.(*ast.IdentExpr); ok && objIdent.Name == "console" && mem.Property == "log" {
				calleeName = "ts_print_val"
				if len(e.Args) > 0 && g.semaResult != nil {
					if t := g.semanticType(e.Args[0]); t != nil {
						switch t {
						case types.TypeString:
							calleeName = "ts_print_str"
						case types.TypeBoolean:
							calleeName = "ts_print_bool"
						case types.TypeUndefined:
							calleeName = "ts_print_undefined"
						}
					}
				}
			}
		}
		if calleeName == "unknown" {
			return g.failExpr("unsupported call target %T", e.Callee)
		}
		var args []ir.Operand
		for _, arg := range e.Args {
			args = append(args, g.lowerExpr(arg))
		}
		callType, _ := g.semanticType(e.Callee).(*types.FunctionType)
		if concrete := g.semaResult.GenericCalls[e]; concrete != nil {
			callType = concrete
		}
		if ident, ok := e.Callee.(*ast.IdentExpr); ok {
			if decl := g.functionDecls[ident.Name]; decl != nil {
				fixedLimit := len(decl.Params)
				for i, param := range decl.Params {
					if param.Rest {
						fixedLimit = i
						break
					}
				}
				for i := len(args); i < fixedLimit; i++ {
					p := decl.Params[i]
					if p.Default != nil {
						args = append(args, g.lowerExpr(p.Default))
						continue
					}
					if p.Optional {
						args = append(args, ir.ConstUndefined{})
						continue
					}
					break
				}
			}
		}
		args = g.packRestOperands(args, callType)
		var paramTypes []types.Type
		if callType != nil && !isConsoleLogCall(e.Callee) && !strings.HasPrefix(calleeName, "ts_") {
			paramTypes = make([]types.Type, len(callType.Params))
			for i := range callType.Params {
				paramTypes[i] = callType.Params[i].Type
			}
		}
		resultType := types.TypeNumber
		if t := g.semanticType(e); t != nil {
			resultType = t
		}
		resVal := g.currentFn.NewValue("ret", resultType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: resVal, Callee: calleeName, Args: args, ParamTypes: paramTypes,
		})
		return resVal
	case *ast.AssignExpr:
		if mem, ok := e.Left.(*ast.MemberExpr); ok {
			if objType, ok := g.semanticType(mem.Object).(*types.ObjectType); ok {
				offsets, _, _ := g.objectLayout(objType)
				offset, exists := offsets[mem.Property]
				if !exists {
					return g.failExpr("object shape has no writable field %q", mem.Property)
				}
				obj := g.lowerExpr(mem.Object)
				if e.Op == token.Eq {
					rhs := g.lowerExpr(e.Right)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: obj, Field: mem.Property, Offset: offset, Val: rhs})
					return rhs
				}
				fieldType := types.TypeAny
				if t := g.semanticType(mem); t != nil {
					fieldType = t
				}
				current := g.currentFn.NewValue("field_old", fieldType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: current, Obj: obj, Field: mem.Property, Offset: offset})
				rhs := g.lowerExpr(e.Right)
				value := g.lowerAssignmentValue(e, current, rhs)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: obj, Field: mem.Property, Offset: offset, Val: value})
				return value
			}
		}
		if idx, ok := e.Left.(*ast.IndexExpr); ok {
			if tuple, isTuple := g.semanticType(idx.Target).(*types.TupleType); isTuple {
				lit, ok := idx.Index.(*ast.NumberLit)
				if !ok {
					return g.failExpr("native tuple assignment currently requires a constant numeric index")
				}
				i := int(lit.Value)
				if float64(i) != lit.Value || i < 0 || i >= len(tuple.Elements) {
					return g.failExpr("tuple assignment index %v is outside [0,%d)", lit.Value, len(tuple.Elements))
				}
				tupleVal := g.lowerExpr(idx.Target)
				field := strconv.Itoa(i)
				offset := 16 + i*8
				if e.Op == token.Eq {
					rhs := g.lowerExpr(e.Right)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: tupleVal, Field: field, Offset: offset, Val: rhs})
					return rhs
				}
				current := g.currentFn.NewValue("tuple_old", tuple.Elements[i])
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: current, Obj: tupleVal, Field: field, Offset: offset})
				rhs := g.lowerExpr(e.Right)
				value := g.lowerAssignmentValue(e, current, rhs)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: tupleVal, Field: field, Offset: offset, Val: value})
				return value
			}
			array := g.lowerExpr(idx.Target)
			index := g.lowerExpr(idx.Index)
			if e.Op == token.Eq {
				rhs := g.lowerExpr(e.Right)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{Array: array, Index: index, Val: rhs})
				return rhs
			}
			currentType := types.TypeAny
			if t := g.semanticType(idx); t != nil {
				currentType = t
			}
			current := g.currentFn.NewValue("elem_old", currentType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: current, Array: array, Index: index})
			rhs := g.lowerExpr(e.Right)
			value := g.lowerAssignmentValue(e, current, rhs)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{Array: array, Index: index, Val: value})
			return value
		}
		if ident, ok := e.Left.(*ast.IdentExpr); ok {
			current, exists := g.locals[ident.Name]
			if !exists {
				return g.failExpr("cannot assign unresolved local %q", ident.Name)
			}
			rhs := g.lowerExpr(e.Right)
			value := g.lowerAssignmentValue(e, current, rhs)
			g.locals[ident.Name] = value
			return value
		}
		return g.failExpr("unsupported assignment target %T", e.Left)

	case *ast.TernaryExpr:
		cond := g.lowerExpr(e.Cond)
		thenBB := g.currentFn.NewBlock("tern_then")
		elseBB := g.currentFn.NewBlock("tern_else")
		joinBB := g.currentFn.NewBlock("tern_join")

		g.currentBB.Terminator = &ir.BranchTerm{Cond: cond, Then: thenBB, Else: elseBB}

		g.currentBB = thenBB
		thenVal := g.lowerExpr(e.Then)
		thenEndBB := g.currentBB
		if thenEndBB.Terminator == nil {
			thenEndBB.Terminator = &ir.JumpTerm{Target: joinBB}
		}

		g.currentBB = elseBB
		elseVal := g.lowerExpr(e.Else)
		elseEndBB := g.currentBB
		if elseEndBB.Terminator == nil {
			elseEndBB.Terminator = &ir.JumpTerm{Target: joinBB}
		}

		g.currentBB = joinBB
		resVal := g.currentFn.NewValue("tern", thenVal.Type())
		joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{
			Res: resVal,
			Incoming: []ir.PhiIncoming{
				{Block: thenEndBB, Value: thenVal},
				{Block: elseEndBB, Value: elseVal},
			},
		})
		return resVal
	default:
		return g.failExpr("unsupported expression node %T", expr)
	}
}
