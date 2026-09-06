package irgen

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
)

type catchContext struct {
	block        *ir.BasicBlock
	phi          *ir.PhiInst
	finallyDepth int
}

type finallyContext struct {
	block    *ir.BasicBlock
	kindPhi  *ir.PhiInst
	valuePhi *ir.PhiInst
}

type generator struct {
	semaResult        *sema.Result
	prog              *ir.Program
	currentFn         *ir.Function
	currentBB         *ir.BasicBlock
	locals            map[string]ir.Operand
	localProvenance   map[string]types.Type
	localDirectCallee map[string]string
	captureCells      map[*ir.Function]map[string]ir.Operand
	captureCellTypes  map[*ir.Function]map[string]types.Type
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
	catchStack        []*catchContext
	finallyStack      []*finallyContext
}

func typeNodeIsAny(node ast.TypeNode) bool {
	primitive, ok := node.(*ast.PrimitiveTypeNode)
	return ok && primitive.Kind == "any"
}

func irJSValueType(t types.Type) bool {
	if t.Kind() == types.KindAny || t.Kind() == types.KindUnknown {
		return true
	}
	u, ok := t.(*types.UnionType)
	if !ok {
		return false
	}
	classes := map[int]bool{}
	for _, member := range u.Members {
		switch member.Kind() {
		case types.KindNull, types.KindUndefined, types.KindNever:
			continue
		case types.KindNumber:
			classes[1] = true
		case types.KindBoolean:
			classes[2] = true
		case types.KindString, types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
			classes[3] = true
		case types.KindAny, types.KindUnknown:
			return true
		}
	}
	return len(classes) > 1
}

func irHeapRefType(t types.Type) bool {
	if irJSValueType(t) {
		return true
	}
	switch t.Kind() {
	case types.KindString, types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
		return true
	default:
		return true
	}
}

func captureCellRuntimeType() *types.ObjectType {
	return types.NewObject("$JSValueCell")
}

func (g *generator) captureCell(name string) (ir.Operand, types.Type, bool) {
	if g.currentFn == nil {
		return nil, nil, false
	}
	cells := g.captureCells[g.currentFn]
	if cells == nil {
		return nil, nil, false
	}
	cell, ok := cells[name]
	if !ok {
		return nil, nil, false
	}
	return cell, g.captureCellTypes[g.currentFn][name], true
}

func (g *generator) bindCaptureCell(fn *ir.Function, name string, cell ir.Operand, valueType types.Type) {
	if g.captureCells[fn] == nil {
		g.captureCells[fn] = make(map[string]ir.Operand)
		g.captureCellTypes[fn] = make(map[string]types.Type)
	}
	g.captureCells[fn][name] = cell
	g.captureCellTypes[fn][name] = valueType
}

func (g *generator) ensureCaptureCell(name string) (ir.Operand, types.Type) {
	if cell, valueType, ok := g.captureCell(name); ok {
		return cell, valueType
	}
	value := g.locals[name]
	valueType := value.Type()
	cellType := captureCellRuntimeType()
	cell := g.currentFn.NewValue(name+"_cell", cellType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: cell, Callee: "ts_jsvalue_cell_new"})
	boxed := value
	if !irJSValueType(valueType) {
		boxed = g.boxJSValue(value, valueType)
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_jsvalue_cell_set", Args: []ir.Operand{cell, boxed}, ParamTypes: []types.Type{cellType, types.TypeAny}})
	g.bindCaptureCell(g.currentFn, name, cell, valueType)
	return cell, valueType
}

func (g *generator) readLocal(name string) ir.Operand {
	cell, valueType, ok := g.captureCell(name)
	if !ok {
		return g.locals[name]
	}
	boxed := g.currentFn.NewValue(name+"_cell_value", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: boxed, Callee: "ts_jsvalue_cell_get", Args: []ir.Operand{cell}})
	return g.coerceJSValueBoundary(boxed, types.TypeAny, valueType)
}

func (g *generator) writeCapturedLocal(name string, value ir.Operand) bool {
	cell, _, ok := g.captureCell(name)
	if !ok {
		return false
	}
	boxed := value
	if !irJSValueType(value.Type()) {
		boxed = g.boxJSValue(value, value.Type())
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_jsvalue_cell_set", Args: []ir.Operand{cell, boxed}, ParamTypes: []types.Type{cell.Type(), types.TypeAny}})
	g.locals[name] = value
	return true
}

func cloneOperandMap(src map[string]ir.Operand) map[string]ir.Operand {
	out := make(map[string]ir.Operand, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (g *generator) currentCatch() *catchContext {
	if len(g.catchStack) == 0 {
		return nil
	}
	return g.catchStack[len(g.catchStack)-1]
}

func (g *generator) currentFinally() *finallyContext {
	if len(g.finallyStack) == 0 {
		return nil
	}
	return g.finallyStack[len(g.finallyStack)-1]
}

func (g *generator) routeFinallyCompletion(ctx *finallyContext, kind float64, value ir.Operand) {
	from := g.currentBB
	ctx.kindPhi.Incoming = append(ctx.kindPhi.Incoming, ir.PhiIncoming{Block: from, Value: ir.ConstNumber{Value: kind}})
	ctx.valuePhi.Incoming = append(ctx.valuePhi.Incoming, ir.PhiIncoming{Block: from, Value: value})
	from.Terminator = &ir.JumpTerm{Target: ctx.block}
	g.currentBB = g.currentFn.NewBlock("after_finally_route_dead")
	g.currentBB.Terminator = &ir.ReturnTerm{}
}

func (g *generator) routeThrownValue(value ir.Operand) {
	fctx := g.currentFinally()
	cctx := g.currentCatch()
	// A catch belonging to the innermost active try sees the throw before that
	// try's finally. If the top catch belongs to an outer try, the inner finally
	// must run first and forward the saved throw afterwards.
	if fctx != nil && (cctx == nil || cctx.finallyDepth < len(g.finallyStack)) {
		g.routeFinallyCompletion(fctx, 2, value)
		return
	}
	if cctx != nil {
		from := g.currentBB
		cctx.phi.Incoming = append(cctx.phi.Incoming, ir.PhiIncoming{Block: from, Value: value})
		from.Terminator = &ir.JumpTerm{Target: cctx.block}
		g.currentBB = g.currentFn.NewBlock("after_throw_dead")
		g.currentBB.Terminator = &ir.ReturnTerm{}
		return
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeAny}})
	g.currentBB.Terminator = &ir.ReturnTerm{}
	g.currentBB = g.currentFn.NewBlock("after_reject_dead")
	g.currentBB.Terminator = &ir.ReturnTerm{}
}

func (g *generator) lowerTry(s *ast.TryStmt) {
	if s.Finally != nil {
		g.lowerTryWithFinally(s)
		return
	}
	outerLocals := cloneOperandMap(g.locals)
	catchBB := g.currentFn.NewBlock("catch")
	exitBB := g.currentFn.NewBlock("try_exit")
	errorVal := g.currentFn.NewValue("caught_error", types.TypeAny)
	phi := &ir.PhiInst{Res: errorVal}
	catchBB.Phis = append(catchBB.Phis, phi)
	ctx := &catchContext{block: catchBB, phi: phi, finallyDepth: len(g.finallyStack)}
	g.catchStack = append(g.catchStack, ctx)
	g.lowerStatement(s.Try)
	g.catchStack = g.catchStack[:len(g.catchStack)-1]
	hasExit := false
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: exitBB}
		hasExit = true
	}
	g.currentBB = catchBB
	g.locals = cloneOperandMap(outerLocals)
	if s.CatchName != "" {
		g.locals[s.CatchName] = errorVal
	}
	g.lowerStatement(s.Catch)
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: exitBB}
		hasExit = true
	}
	g.currentBB = exitBB
	if !hasExit {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}
	g.locals = outerLocals
}

func (g *generator) lowerTryWithFinally(s *ast.TryStmt) {
	outerLocals := cloneOperandMap(g.locals)
	finallyBB := g.currentFn.NewBlock("finally")
	afterBB := g.currentFn.NewBlock("finally_after")
	kindVal := g.currentFn.NewValue("completion_kind", types.TypeNumber)
	valueVal := g.currentFn.NewValue("completion_value", types.TypeAny)
	kindPhi := &ir.PhiInst{Res: kindVal}
	valuePhi := &ir.PhiInst{Res: valueVal}
	finallyBB.Phis = append(finallyBB.Phis, kindPhi, valuePhi)
	fctx := &finallyContext{block: finallyBB, kindPhi: kindPhi, valuePhi: valuePhi}
	g.finallyStack = append(g.finallyStack, fctx)
	hasNormalCompletion := false

	if s.Catch != nil {
		catchBB := g.currentFn.NewBlock("catch")
		errorVal := g.currentFn.NewValue("caught_error", types.TypeAny)
		catchPhi := &ir.PhiInst{Res: errorVal}
		catchBB.Phis = append(catchBB.Phis, catchPhi)
		cctx := &catchContext{block: catchBB, phi: catchPhi, finallyDepth: len(g.finallyStack)}
		g.catchStack = append(g.catchStack, cctx)
		g.lowerStatement(s.Try)
		g.catchStack = g.catchStack[:len(g.catchStack)-1]
		if g.currentBB.Terminator == nil {
			hasNormalCompletion = true
			g.routeFinallyCompletion(fctx, 0, ir.ConstUndefined{})
		}

		g.currentBB = catchBB
		g.locals = cloneOperandMap(outerLocals)
		if s.CatchName != "" {
			g.locals[s.CatchName] = errorVal
		}
		g.lowerStatement(s.Catch)
		if g.currentBB.Terminator == nil {
			hasNormalCompletion = true
			g.routeFinallyCompletion(fctx, 0, ir.ConstUndefined{})
		}
	} else {
		g.lowerStatement(s.Try)
		if g.currentBB.Terminator == nil {
			hasNormalCompletion = true
			g.routeFinallyCompletion(fctx, 0, ir.ConstUndefined{})
		}
	}

	g.finallyStack = g.finallyStack[:len(g.finallyStack)-1]
	g.currentBB = finallyBB
	g.locals = cloneOperandMap(outerLocals)
	g.lowerStatement(s.Finally)
	finalLocals := cloneOperandMap(g.locals)
	if g.currentBB.Terminator != nil {
		g.locals = outerLocals
		return
	}

	normalBB := g.currentFn.NewBlock("finally_normal")
	abruptBB := g.currentFn.NewBlock("finally_abrupt")
	isNormal := g.currentFn.NewValue("completion_normal", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: isNormal, Op: ir.OpEq, LHS: kindVal, RHS: ir.ConstNumber{Value: 0}})
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isNormal, Then: normalBB, Else: abruptBB}
	normalBB.Terminator = &ir.JumpTerm{Target: afterBB}

	g.currentBB = abruptBB
	returnBB := g.currentFn.NewBlock("finally_return")
	throwBB := g.currentFn.NewBlock("finally_throw")
	isReturn := g.currentFn.NewValue("completion_return", types.TypeBoolean)
	abruptBB.Instructions = append(abruptBB.Instructions, &ir.BinaryInst{Res: isReturn, Op: ir.OpEq, LHS: kindVal, RHS: ir.ConstNumber{Value: 1}})
	abruptBB.Terminator = &ir.BranchTerm{Cond: isReturn, Then: returnBB, Else: throwBB}

	g.currentBB = returnBB
	ret := g.coerceJSValueBoundary(valueVal, types.TypeAny, g.currentFn.ReturnType)
	returnBB.Terminator = &ir.ReturnTerm{Val: ret}

	g.currentBB = throwBB
	g.routeThrownValue(valueVal)

	g.currentBB = afterBB
	g.locals = finalLocals
	if !hasNormalCompletion {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}
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

func (g *generator) tupleRefMask(t *types.TupleType) uint64 {
	var mask uint64
	for i, elem := range t.Elements {
		if irHeapRefType(elem) {
			mask |= uint64(1) << i
		}
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

func (g *generator) ensureGenericSpecialization(decl *ast.FunctionDecl, concrete *types.FunctionType) string {
	generic := g.semaResult.Types[decl].(*types.FunctionType)
	bindings, _ := types.FunctionBindings(generic, concrete)
	key := genericSpecializationKey(decl.Name, concrete)
	if name, ok := g.genericSpecs[key]; ok {
		return name
	}
	name := fmt.Sprintf("%s$spec%d", decl.Name, g.genericSpecCount)
	g.genericSpecCount++
	// Register before lowering so recursive calls reuse this specialization.
	g.genericSpecs[key] = name

	outerFn, outerBB, outerLocals, outerProvenance, outerBindings := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.typeBindings
	g.typeBindings = bindings
	fn, _ := g.lowerFunctionAs(decl, concrete, name)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.typeBindings = outerFn, outerBB, outerLocals, outerProvenance, outerBindings
	g.prog.Functions = append(g.prog.Functions, fn)
	return name
}

func (g *generator) lowerAssignmentValue(e *ast.AssignExpr, current, rhs ir.Operand) ir.Operand {
	resultType := current.Type()
	if t := g.semanticType(e); t != nil {
		resultType = t
	}
	if e.Op == token.PlusEq && resultType == types.TypeString {
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
	default:
		op = ir.OpDiv
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
	return ir.ConstString{Value: "undefined"}
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
			concrete = member
		}
	}
	if concrete == nil || (concrete != types.TypeString && concrete != types.TypeBoolean && !isNumberSemanticType(concrete)) {
		concrete = types.TypeString
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
	if union, ok := t.(*types.UnionType); ok {
		return g.coerceNullableUnionString(union, op)
	}
	return g.coerceStringType(t, op)
}

func (g *generator) canFuseStringConcat(expr ast.Expr) bool {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != token.Plus || g.semanticType(bin) != types.TypeString {
		return false
	}
	// Dynamic JSValue addition owns ToPrimitive/addition semantics and must stay
	// on ts_js_add rather than being flattened into native string concatenation.
	return !irJSValueType(g.semanticType(bin.Left)) && !irJSValueType(g.semanticType(bin.Right))
}

func (g *generator) collectStringConcatParts(expr ast.Expr, parts *[]ast.Expr) {
	if g.canFuseStringConcat(expr) {
		bin := expr.(*ast.BinaryExpr)
		g.collectStringConcatParts(bin.Left, parts)
		g.collectStringConcatParts(bin.Right, parts)
		return
	}
	*parts = append(*parts, expr)
}

func (g *generator) lowerStringConcatChain(expr *ast.BinaryExpr) (ir.Operand, bool) {
	var parts []ast.Expr
	g.collectStringConcatParts(expr, &parts)
	if len(parts) < 3 {
		return nil, false
	}
	ops := make([]ir.Operand, 0, len(parts))
	for _, part := range parts {
		op := g.lowerExpr(part)
		ops = append(ops, g.coerceStringOperand(part, op))
	}
	emit := func(args []ir.Operand) ir.Operand {
		callee := "ts_string_concat"
		switch len(args) {
		case 3:
			callee = "ts_string_concat3"
		case 4:
			callee = "ts_string_concat4"
		}
		res := g.currentFn.NewValue("str", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: callee, Args: args})
		return res
	}
	if len(ops) <= 4 {
		return emit(ops), true
	}
	current := emit(ops[:4])
	for i := 4; i < len(ops); {
		n := len(ops) - i
		if n > 3 {
			n = 3
		}
		args := make([]ir.Operand, 1, n+1)
		args[0] = current
		args = append(args, ops[i:i+n]...)
		current = emit(args)
		i += n
	}
	return current, true
}

func nativeTaskResultKind(t types.Type) float64 {
	if t == nil || t.Kind() == types.KindVoid {
		return float64(amd64TaskResultVoidIR)
	}
	if t.Kind() == types.KindNumber {
		return float64(amd64TaskResultNumberIR)
	}
	if irJSValueType(t) {
		return float64(amd64TaskResultJSIR)
	}
	switch t.Kind() {
	case types.KindString, types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
		return float64(amd64TaskResultRefIR)
	default:
		return float64(amd64TaskResultScalarIR)
	}
}

const (
	amd64TaskResultVoidIR = iota
	amd64TaskResultNumberIR
	amd64TaskResultScalarIR
	amd64TaskResultRefIR
	amd64TaskResultJSIR
)

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
	arrType := fnType.Params[restIndex].Type.(*types.ArrayType)
	restCount := len(args) - restIndex
	rest := g.currentFn.NewValue("rest", arrType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: rest, ElemType: arrType.Elem, Length: ir.ConstNumber{Value: float64(restCount)},
	})
	for i, value := range args[restIndex:] {
		stored := value
		if irJSValueType(arrType.Elem) {
			stored = g.boxJSValue(value, value.Type())
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{
			Array: rest, Index: ir.ConstNumber{Value: float64(i)}, Val: stored,
		})
	}
	packed := append([]ir.Operand(nil), args[:restIndex]...)
	packed = append(packed, rest)
	return packed
}

func (g *generator) coerceJSValueBoundary(value ir.Operand, sourceType, targetType types.Type) ir.Operand {
	if sourceType == nil {
		sourceType = value.Type()
	}
	actualType := value.Type()

	// Primitive values entering any/unknown must use the boxed JSValue ABI.
	if irJSValueType(targetType) {
		if actualType != nil && irJSValueType(actualType) {
			return value
		}
		switch sourceType.Kind() {
		case types.KindNumber, types.KindString, types.KindBoolean,
			types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
			return g.boxJSValue(value, sourceType)
		case types.KindNull, types.KindUndefined:
			return value
		default:
			// Reference boxing is handled separately so existing closed-shape
			// provenance remains available until the dynamic-reference milestone.
			return value
		}
	}

	// Primitive typed consumers decode values that crossed an any boundary.
	if irJSValueType(sourceType) || (actualType != nil && irJSValueType(actualType)) {
		var callee string
		switch targetType.Kind() {
		case types.KindNumber:
			callee = "ts_js_unbox_number"
		case types.KindString:
			callee = "ts_js_unbox_string"
		case types.KindBoolean:
			callee = "ts_js_unbox_bool"
		case types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
			callee = "ts_js_unbox_ref"
		default:
			return value
		}
		res := g.currentFn.NewValue("js_unbox", targetType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: res, Callee: callee, Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeAny},
		})
		return res
	}
	return value
}

func (g *generator) coerceCallOperands(args []ir.Operand, sourceTypes []types.Type, fnType *types.FunctionType) []ir.Operand {
	if fnType == nil {
		return args
	}
	restIndex := restParamIndex(fnType.Params)
	for i := range args {
		var target types.Type
		if restIndex >= 0 && i >= restIndex {
			arr, ok := fnType.Params[restIndex].Type.(*types.ArrayType)
			if ok {
				target = arr.Elem
			}
		} else if i < len(fnType.Params) {
			target = fnType.Params[i].Type
		}
		source := sourceTypes[i]
		args[i] = g.coerceJSValueBoundary(args[i], source, target)
	}
	return args
}

func (g *generator) boxJSValue(value ir.Operand, sourceType types.Type) ir.Operand {
	if irJSValueType(sourceType) {
		return value
	}
	switch sourceType.Kind() {
	case types.KindNull, types.KindUndefined:
		return value
	case types.KindNumber:
		res := g.currentFn.NewValue("js_num", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_js_box_number", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeNumber}})
		return res
	case types.KindBoolean:
		res := g.currentFn.NewValue("js_bool", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_js_box_bool", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeBoolean}})
		return res
	case types.KindString:
		res := g.currentFn.NewValue("js_str", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_js_box_string", Args: []ir.Operand{value}, ParamTypes: []types.Type{sourceType}})
		return res
	default:
		res := g.currentFn.NewValue("js_ref", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_js_box_ref", Args: []ir.Operand{value}, ParamTypes: []types.Type{sourceType}})
		return res
	}
}

func (g *generator) pushArrayOperand(array ir.Operand, value ir.Operand) {
	length := g.currentFn.NewValue("push_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayPushInst{Res: length, Array: array, Val: value})
}

func (g *generator) appendSpreadArray(dst ir.Operand, src ir.Operand, elemType, dstElemType types.Type) {
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
	stored := ir.Operand(elem)
	if irJSValueType(dstElemType) && !irJSValueType(elemType) {
		stored = g.boxJSValue(elem, elemType)
	}
	g.pushArrayOperand(dst, stored)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	bodyBB.Terminator = &ir.JumpTerm{Target: condBB}
	g.currentBB = doneBB
}

func isNumberSemanticType(t types.Type) bool {
	return t == types.TypeNumber
}

func (g *generator) collectArrowCaptures(expr ast.Expr, params map[string]struct{}) []string {
	found := make(map[string]struct{})
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
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
			nestedParams := make(map[string]struct{}, len(n.Params))
			for _, p := range n.Params {
				nestedParams[p.Name] = struct{}{}
			}
			var nested []string
			if n.IsExprBody {
				nested = g.collectArrowCaptures(n.Body.(ast.Expr), nestedParams)
			} else {
				nested = g.collectBlockClosureCaptures(n.Body.(*ast.BlockStmt), nestedParams)
			}
			for _, name := range nested {
				if _, isParam := params[name]; !isParam {
					found[name] = struct{}{}
				}
			}
		case *ast.FunctionExpr:
			nestedParams := make(map[string]struct{}, len(n.Params))
			for _, p := range n.Params {
				if !p.IsThis {
					nestedParams[p.Name] = struct{}{}
				}
			}
			for _, name := range g.collectBlockClosureCaptures(n.Body, nestedParams) {
				if _, isParam := params[name]; !isParam {
					found[name] = struct{}{}
				}
			}
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

func (g *generator) collectBlockClosureCaptures(block *ast.BlockStmt, params map[string]struct{}) []string {
	locals := make(map[string]struct{}, len(params))
	for name := range params {
		locals[name] = struct{}{}
	}
	var collectDecls func(ast.Stmt)
	collectDecls = func(stmt ast.Stmt) {
		if stmt == nil {
			return
		}
		switch n := stmt.(type) {
		case *ast.VarDeclStmt:
			for _, d := range n.Declarations {
				locals[d.Name] = struct{}{}
			}
		case *ast.ForOfStmt:
			locals[n.Name] = struct{}{}
			collectDecls(n.Body)
		case *ast.ForStmt:
			collectDecls(n.Init)
			collectDecls(n.Body)
		case *ast.BlockStmt:
			for _, child := range n.Statements {
				collectDecls(child)
			}
		case *ast.IfStmt:
			collectDecls(n.Then)
			collectDecls(n.Else)
		case *ast.WhileStmt:
			collectDecls(n.Body)
		case *ast.DoWhileStmt:
			collectDecls(n.Body)
		case *ast.SwitchStmt:
			for _, c := range n.Cases {
				for _, child := range c.Statements {
					collectDecls(child)
				}
			}
		case *ast.FunctionDecl:
			locals[n.Name] = struct{}{}
		}
	}
	collectDecls(block)

	found := make(map[string]struct{})
	var walkExpr func(ast.Expr)
	var walkStmt func(ast.Stmt)
	walkExpr = func(e ast.Expr) {
		if e == nil {
			return
		}
		switch n := e.(type) {
		case *ast.IdentExpr:
			if _, local := locals[n.Name]; local {
				return
			}
			if _, outer := g.locals[n.Name]; outer {
				found[n.Name] = struct{}{}
			}
		case *ast.BinaryExpr:
			walkExpr(n.Left)
			walkExpr(n.Right)
		case *ast.UnaryExpr:
			walkExpr(n.Target)
		case *ast.CallExpr:
			walkExpr(n.Callee)
			for _, a := range n.Args {
				walkExpr(a)
			}
		case *ast.MemberExpr:
			walkExpr(n.Object)
		case *ast.IndexExpr:
			walkExpr(n.Target)
			walkExpr(n.Index)
		case *ast.ArrayLit:
			for _, el := range n.Elements {
				walkExpr(el)
			}
		case *ast.SpreadExpr:
			walkExpr(n.Value)
		case *ast.ObjectLit:
			for _, prop := range n.Properties {
				walkExpr(prop.Value)
			}
		case *ast.AssignExpr:
			walkExpr(n.Left)
			walkExpr(n.Right)
		case *ast.TernaryExpr:
			walkExpr(n.Cond)
			walkExpr(n.Then)
			walkExpr(n.Else)
		case *ast.ArrowFuncExpr:
			nestedParams := make(map[string]struct{}, len(n.Params))
			for _, p := range n.Params {
				nestedParams[p.Name] = struct{}{}
			}
			var nested []string
			if n.IsExprBody {
				nested = g.collectArrowCaptures(n.Body.(ast.Expr), nestedParams)
			} else {
				nested = g.collectBlockClosureCaptures(n.Body.(*ast.BlockStmt), nestedParams)
			}
			for _, name := range nested {
				if _, shadowed := locals[name]; !shadowed {
					found[name] = struct{}{}
				}
			}
		case *ast.FunctionExpr:
			nestedParams := make(map[string]struct{}, len(n.Params))
			for _, p := range n.Params {
				if !p.IsThis {
					nestedParams[p.Name] = struct{}{}
				}
			}
			for _, name := range g.collectBlockClosureCaptures(n.Body, nestedParams) {
				if _, shadowed := locals[name]; !shadowed {
					found[name] = struct{}{}
				}
			}
		}
	}
	walkStmt = func(stmt ast.Stmt) {
		if stmt == nil {
			return
		}
		switch n := stmt.(type) {
		case *ast.BlockStmt:
			for _, child := range n.Statements {
				walkStmt(child)
			}
		case *ast.VarDeclStmt:
			for _, d := range n.Declarations {
				walkExpr(d.Init)
			}
		case *ast.ExprStmt:
			walkExpr(n.Expr)
		case *ast.ReturnStmt:
			walkExpr(n.Value)
		case *ast.IfStmt:
			walkExpr(n.Cond)
			walkStmt(n.Then)
			walkStmt(n.Else)
		case *ast.WhileStmt:
			walkExpr(n.Cond)
			walkStmt(n.Body)
		case *ast.DoWhileStmt:
			walkStmt(n.Body)
			walkExpr(n.Cond)
		case *ast.ForStmt:
			walkStmt(n.Init)
			walkExpr(n.Cond)
			walkExpr(n.Post)
			walkStmt(n.Body)
		case *ast.ForOfStmt:
			walkExpr(n.Iterable)
			walkStmt(n.Body)
		case *ast.SwitchStmt:
			walkExpr(n.Expr)
			for _, c := range n.Cases {
				walkExpr(c.Test)
				for _, child := range c.Statements {
					walkStmt(child)
				}
			}
		case *ast.FunctionDecl:
			// Nested declarations own their body.
		}
	}
	walkStmt(block)
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (g *generator) lowerArrowExpr(e *ast.ArrowFuncExpr) ir.Operand {
	fnType := g.semanticType(e).(*types.FunctionType)
	paramSet := make(map[string]struct{}, len(e.Params))
	for _, p := range e.Params {
		paramSet[p.Name] = struct{}{}
	}
	var captureNames []string
	if e.IsExprBody {
		body := e.Body.(ast.Expr)
		captureNames = g.collectArrowCaptures(body, paramSet)
	} else {
		body := e.Body.(*ast.BlockStmt)
		captureNames = g.collectBlockClosureCaptures(body, paramSet)
	}
	captureOps := make([]ir.Operand, 0, len(captureNames))
	captureTypes := make([]types.Type, 0, len(captureNames))
	var refMask uint64
	for i, name := range captureNames {
		cell, valueType := g.ensureCaptureCell(name)
		captureOps = append(captureOps, cell)
		captureTypes = append(captureTypes, valueType)
		// Capture cells are always GC-managed references.
		refMask |= uint64(1) << i
	}

	outerFn, outerBB, outerLocals, outerProvenance, outerDirectCallees := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	name := fmt.Sprintf("$arrow%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, fnType.Return)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)

	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	for i, captureName := range captureNames {
		cellType := captureOps[i].Type()
		cell := lifted.NewValue(captureName+"_cell_capture", cellType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: cell, Closure: env, Index: i})
		// Keep the name visible to nested capture analysis while all reads/writes
		// route through the shared cell.
		g.locals[captureName] = cell
		g.bindCaptureCell(lifted, captureName, cell, captureTypes[i])
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
	if e.IsExprBody {
		body := e.Body.(ast.Expr)
		ret := g.lowerExpr(body)
		if fnType.Return == types.TypeVoid {
			if g.currentBB.Terminator == nil {
				g.currentBB.Terminator = &ir.ReturnTerm{}
			}
		} else {
			ret = g.coerceJSValueBoundary(ret, g.semanticType(body), fnType.Return)
			if g.currentBB.Terminator == nil {
				g.currentBB.Terminator = &ir.ReturnTerm{Val: ret}
			}
		}
	} else {
		body := e.Body.(*ast.BlockStmt)
		for _, stmt := range body.Statements {
			g.lowerStatement(stmt)
		}
		if g.currentBB.Terminator == nil {
			g.currentBB.Terminator = &ir.ReturnTerm{}
		}
	}
	g.prog.Functions = append(g.prog.Functions, lifted)

	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProvenance, outerDirectCallees
	res := g.currentFn.NewValue("closure", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{
		Res: res, Function: name, Captures: captureOps, RefMask: refMask,
	})
	return res
}

func irFunctionMemberType(t types.Type) *types.FunctionType {
	if fn, ok := t.(*types.FunctionType); ok {
		return fn
	}
	u := t.(*types.UnionType)
	return u.Members[0].(*types.FunctionType)
}

func (g *generator) thenableMethodType(t types.Type) (*types.ObjectType, *types.FunctionType, bool) {
	obj := t.(*types.ObjectType)
	if info := g.semaResult.Classes[obj.Name]; info != nil {
		if fn := info.Methods["then"]; fn != nil {
			return obj, fn, true
		}
	}
	field := obj.Fields["then"]
	return obj, irFunctionMemberType(field.Type), true
}

func (g *generator) emitThenableMethodCall(receiver ir.Operand, obj *types.ObjectType, fn *types.FunctionType, args []ir.Operand) {
	if info := g.semaResult.Classes[obj.Name]; info != nil && info.Methods["then"] != nil {
		_ = g.emitClassMethodCall(receiver, info, "then", args)
		return
	}
	field := obj.Fields["then"]
	offsets, _, _ := g.objectLayout(obj)
	rawType := field.Type
	raw := g.currentFn.NewValue("then_method_raw", rawType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: raw, Obj: receiver, Field: "then", Offset: offsets["then"]})
	closure := ir.Operand(raw)
	if rawType != fn {
		unboxed := g.currentFn.NewValue("then_method", fn)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: unboxed, Callee: "ts_js_unbox_ref", Args: []ir.Operand{raw}, ParamTypes: []types.Type{types.TypeAny}})
		closure = unboxed
	}
	paramTypes := make([]types.Type, len(fn.Params))
	for i := range fn.Params {
		paramTypes[i] = fn.Params[i].Type
	}
	var thisArg ir.Operand
	if fn.This != nil {
		thisArg = receiver
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{Closure: closure, ThisArg: thisArg, Args: args, ParamTypes: paramTypes})
}

func (g *generator) makeThenableSettlementCallback(statusCh, valueCh ir.Operand, valueType types.Type, fulfilled bool) ir.Operand {
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	name := fmt.Sprintf("$promise_settle%d", g.arrowCounter)
	g.arrowCounter++
	fnType := types.NewFunction([]types.Param{{Name: "value", Type: valueType}}, types.TypeVoid)
	lifted := ir.NewFunction(name, types.TypeVoid)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	status := lifted.NewValue("status_ch", statusCh.Type())
	payload := lifted.NewValue("value_ch", valueCh.Type())
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.ClosureGetInst{Res: status, Closure: env, Index: 0},
		&ir.ClosureGetInst{Res: payload, Closure: env, Index: 1})
	arg := lifted.NewValue("value", valueType)
	lifted.Params = append(lifted.Params, arg)
	boxedStatus := g.boxJSValue(ir.ConstBool{Value: fulfilled}, types.TypeBoolean)
	sent := lifted.NewValue("settled", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: sent, Callee: "ts_channel_try_send", Args: []ir.Operand{status, boxedStatus}, ParamTypes: []types.Type{status.Type(), types.TypeAny}})
	sendBB := lifted.NewBlock("settle_send")
	doneBB := lifted.NewBlock("settle_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: sent, Then: sendBB, Else: doneBB}
	g.currentBB = sendBB
	boxedArg := g.boxJSValue(arg, valueType)
	sendBB.Instructions = append(sendBB.Instructions, &ir.CallInst{Callee: "ts_channel_send", Args: []ir.Operand{payload, boxedArg}, ParamTypes: []types.Type{payload.Type(), types.TypeAny}})
	sendBB.Terminator = &ir.JumpTerm{Target: doneBB}
	doneBB.Terminator = &ir.ReturnTerm{}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("settler", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: name, Captures: []ir.Operand{statusCh, valueCh}, RefMask: 3})
	return closure
}

func (g *generator) lowerThenablePromise(e *ast.CallExpr, taskType *types.ObjectType, inner types.Type, thenableType *types.ObjectType, thenFn *types.FunctionType) ir.Operand {
	thenable := g.lowerExpr(e.Args[0])
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	driverType := types.NewFunction(nil, inner)
	driverName := fmt.Sprintf("$thenable%d", g.arrowCounter)
	g.arrowCounter++
	driver := ir.NewFunction(driverName, inner)
	g.currentFn = driver
	g.currentBB = driver.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := driver.NewValue("$env", driverType)
	driver.Params = append(driver.Params, env)
	receiver := driver.NewValue("thenable", thenableType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: receiver, Closure: env, Index: 0})
	channelType := types.NewObject(fmt.Sprintf("$ThenableChannel$%d", g.arrowCounter))
	statusCh := driver.NewValue("settle_status", channelType)
	valueCh := driver.NewValue("settle_value", channelType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.CallInst{Res: statusCh, Callee: "ts_channel_new", Args: []ir.Operand{ir.ConstNumber{Value: 1}}, ParamTypes: []types.Type{types.TypeNumber}},
		&ir.CallInst{Res: valueCh, Callee: "ts_channel_new", Args: []ir.Operand{ir.ConstNumber{Value: 1}}, ParamTypes: []types.Type{types.TypeNumber}})
	resolve := g.makeThenableSettlementCallback(statusCh, valueCh, inner, true)
	reject := g.makeThenableSettlementCallback(statusCh, valueCh, types.TypeAny, false)
	g.emitThenableMethodCall(receiver, thenableType, thenFn, []ir.Operand{resolve, reject})
	statusBox := driver.NewValue("settle_status_box", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: statusBox, Callee: "ts_channel_recv", Args: []ir.Operand{statusCh}, ParamTypes: []types.Type{channelType}})
	status := g.coerceJSValueBoundary(statusBox, types.TypeAny, types.TypeBoolean)
	payload := driver.NewValue("settle_payload", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: payload, Callee: "ts_channel_recv", Args: []ir.Operand{valueCh}, ParamTypes: []types.Type{channelType}})
	okBB := driver.NewBlock("thenable_fulfilled")
	rejectBB := driver.NewBlock("thenable_rejected")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: status, Then: okBB, Else: rejectBB}
	g.currentBB = okBB
	resolved := g.coerceJSValueBoundary(payload, types.TypeAny, inner)
	okBB.Terminator = &ir.ReturnTerm{Val: resolved}
	g.currentBB = rejectBB
	rejectBB.Instructions = append(rejectBB.Instructions, &ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{payload}, ParamTypes: []types.Type{types.TypeAny}})
	rejectBB.Terminator = &ir.ReturnTerm{}
	g.prog.Functions = append(g.prog.Functions, driver)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("thenable_driver", driverType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: driverName, Captures: []ir.Operand{thenable}, RefMask: 1})
	task := g.currentFn.NewValue("thenable_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: task, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(inner)}}})
	return task
}

func (g *generator) promiseSettledIRType(t types.Type) types.Type {
	if obj, ok := t.(*types.ObjectType); ok {
		if inner := g.semaResult.TaskResults[obj.Name]; inner != nil {
			return inner
		}
		if _, fn, ok := g.thenableMethodType(obj); ok && len(fn.Params) > 0 {
			if resolve := irFunctionMemberType(fn.Params[0].Type); resolve != nil && len(resolve.Params) > 0 {
				return resolve.Params[0].Type
			}
		}
	}
	return t
}

func (g *generator) makeImmediatePromiseTask(value ir.Operand, sourceType, resultType types.Type) ir.Operand {
	value = g.coerceJSValueBoundary(value, sourceType, resultType)
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	name := fmt.Sprintf("$promise_immediate%d", g.arrowCounter)
	g.arrowCounter++
	closureType := types.NewFunction(nil, resultType)
	lifted := ir.NewFunction(name, resultType)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", closureType)
	lifted.Params = append(lifted.Params, env)
	captured := lifted.NewValue("promise_value", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: captured, Closure: env, Index: 0})
	g.currentBB.Terminator = &ir.ReturnTerm{Val: captured}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("promise_immediate_closure", closureType)
	var refMask uint64
	if irHeapRefType(resultType) {
		refMask = 1
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: name, Captures: []ir.Operand{value}, RefMask: refMask})
	taskType := types.NewObject(fmt.Sprintf("$PromiseImmediate$%d", g.arrowCounter))
	task := g.currentFn.NewValue("promise_immediate_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: task, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(resultType)}}})
	return task
}

func (g *generator) lowerPromiseAggregateInput(expr ast.Expr) (ir.Operand, types.Type) {
	sourceType := g.semanticType(expr)
	settledType := g.promiseSettledIRType(sourceType)
	if obj, ok := sourceType.(*types.ObjectType); ok {
		if inner := g.semaResult.TaskResults[obj.Name]; inner != nil {
			return g.lowerExpr(expr), inner
		}
		if thenObj, thenFn, isThenable := g.thenableMethodType(obj); isThenable {
			taskType := types.NewObject(fmt.Sprintf("$PromiseAggregateThenable$%d", g.arrowCounter))
			fake := &ast.CallExpr{Args: []ast.Expr{expr}}
			return g.lowerThenablePromise(fake, taskType, settledType, thenObj, thenFn), settledType
		}
	}
	value := g.lowerExpr(expr)
	return g.makeImmediatePromiseTask(value, sourceType, settledType), settledType
}

func (g *generator) lowerPromiseLiteralAggregate(member *ast.MemberExpr, taskType *types.ObjectType, inner types.Type, literal *ast.ArrayLit) ir.Operand {

	tasks := make([]ir.Operand, 0, len(literal.Elements))
	resultTypes := make([]types.Type, 0, len(literal.Elements))
	for _, element := range literal.Elements {
		task, resultType := g.lowerPromiseAggregateInput(element)
		tasks = append(tasks, task)
		resultTypes = append(resultTypes, resultType)
	}

	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	driverType := types.NewFunction(nil, inner)
	driverName := fmt.Sprintf("$promise_%s%d", member.Property, g.arrowCounter)
	g.arrowCounter++
	driver := ir.NewFunction(driverName, inner)
	g.currentFn = driver
	g.currentBB = driver.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := driver.NewValue("$env", driverType)
	driver.Params = append(driver.Params, env)
	captured := make([]ir.Operand, len(tasks))
	for i, task := range tasks {
		v := driver.NewValue(fmt.Sprintf("aggregate_task_%d", i), task.Type())
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: v, Closure: env, Index: i})
		captured[i] = v
	}

	if member.Property == "all" {
		g.lowerPromiseAllDriver(captured, resultTypes, inner)
	} else {
		g.lowerPromiseRaceDriver(captured, resultTypes, inner)
	}
	g.prog.Functions = append(g.prog.Functions, driver)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect

	closure := g.currentFn.NewValue("promise_aggregate_driver", driverType)
	var refMask uint64
	if len(tasks) > 0 {
		refMask = (uint64(1) << len(tasks)) - 1
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: driverName, Captures: tasks, RefMask: refMask})
	result := g.currentFn.NewValue("promise_aggregate_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: result, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(inner)}}})
	return result
}

func (g *generator) lowerPromiseAllDriver(tasks []ir.Operand, resultTypes []types.Type, inner types.Type) {
	poll := g.currentFn.NewBlock("promise_all_poll")
	pending := g.currentFn.NewBlock("promise_all_pending")
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
	g.currentBB = poll

	for i, task := range tasks {
		rejected := g.currentFn.NewValue(fmt.Sprintf("all_rejected_%d", i), types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: rejected, Callee: "ts_task_rejected", Args: []ir.Operand{task}})
		rejectBB := g.currentFn.NewBlock(fmt.Sprintf("promise_all_reject_%d", i))
		nextBB := g.currentFn.NewBlock(fmt.Sprintf("promise_all_reject_next_%d", i))
		g.currentBB.Terminator = &ir.BranchTerm{Cond: rejected, Then: rejectBB, Else: nextBB}
		g.currentBB = rejectBB
		errVal := g.currentFn.NewValue("aggregate_error", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.CallInst{Res: errVal, Callee: "ts_task_error", Args: []ir.Operand{task}},
			&ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{errVal}, ParamTypes: []types.Type{types.TypeAny}},
		)
		g.currentBB.Terminator = &ir.ReturnTerm{}
		g.currentBB = nextBB
	}

	for i, task := range tasks {
		done := g.currentFn.NewValue(fmt.Sprintf("all_done_%d", i), types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: done, Callee: "ts_task_done", Args: []ir.Operand{task}})
		nextBB := g.currentFn.NewBlock(fmt.Sprintf("promise_all_done_next_%d", i))
		g.currentBB.Terminator = &ir.BranchTerm{Cond: done, Then: nextBB, Else: pending}
		g.currentBB = nextBB
	}

	values := make([]ir.Operand, len(tasks))
	for i, task := range tasks {
		resultType := resultTypes[i]
		if resultType == nil || resultType.Kind() == types.KindVoid {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_join", Args: []ir.Operand{task}})
			values[i] = ir.ConstUndefined{}
			continue
		}
		value := g.currentFn.NewValue(fmt.Sprintf("all_value_%d", i), resultType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: value, Callee: "ts_task_join", Args: []ir.Operand{task}})
		values[i] = value
	}
	g.finishPromiseAllResult(values, resultTypes, inner)

	g.currentBB = pending
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_yield"})
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
}

func (g *generator) finishPromiseAllResult(values []ir.Operand, resultTypes []types.Type, inner types.Type) {
	switch out := inner.(type) {
	case *types.TupleType:
		res := g.currentFn.NewValue("promise_all_tuple", out)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: out.String(), FieldCount: len(out.Elements), RefMask: g.tupleRefMask(out)})
		for i, value := range values {
			coerced := g.coerceJSValueBoundary(value, resultTypes[i], out.Elements[i])
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: strconv.Itoa(i), Offset: 16 + i*8, Val: coerced})
		}
		g.currentBB.Terminator = &ir.ReturnTerm{Val: res}
	case *types.ArrayType:
		res := g.currentFn.NewValue("promise_all_array", out)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{Res: res, ElemType: out.Elem, Length: ir.ConstNumber{Value: float64(len(values))}})
		for i, value := range values {
			coerced := g.coerceJSValueBoundary(value, resultTypes[i], out.Elem)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{Array: res, Index: ir.ConstNumber{Value: float64(i)}, Val: coerced})
		}
		g.currentBB.Terminator = &ir.ReturnTerm{Val: res}
	}
}

func (g *generator) lowerPromiseRaceDriver(tasks []ir.Operand, resultTypes []types.Type, inner types.Type) {
	poll := g.currentFn.NewBlock("promise_race_poll")
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
	g.currentBB = poll
	for i, task := range tasks {
		done := g.currentFn.NewValue(fmt.Sprintf("race_done_%d", i), types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: done, Callee: "ts_task_done", Args: []ir.Operand{task}})
		settledBB := g.currentFn.NewBlock(fmt.Sprintf("promise_race_settled_%d", i))
		nextBB := g.currentFn.NewBlock(fmt.Sprintf("promise_race_next_%d", i))
		g.currentBB.Terminator = &ir.BranchTerm{Cond: done, Then: settledBB, Else: nextBB}
		g.currentBB = settledBB
		rejected := g.currentFn.NewValue(fmt.Sprintf("race_rejected_%d", i), types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: rejected, Callee: "ts_task_rejected", Args: []ir.Operand{task}})
		rejectBB := g.currentFn.NewBlock(fmt.Sprintf("promise_race_reject_%d", i))
		fulfillBB := g.currentFn.NewBlock(fmt.Sprintf("promise_race_fulfill_%d", i))
		g.currentBB.Terminator = &ir.BranchTerm{Cond: rejected, Then: rejectBB, Else: fulfillBB}
		g.currentBB = rejectBB
		errVal := g.currentFn.NewValue("race_error", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.CallInst{Res: errVal, Callee: "ts_task_error", Args: []ir.Operand{task}},
			&ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{errVal}, ParamTypes: []types.Type{types.TypeAny}},
		)
		g.currentBB.Terminator = &ir.ReturnTerm{}
		g.currentBB = fulfillBB
		resultType := resultTypes[i]
		if resultType == nil || resultType.Kind() == types.KindVoid {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_join", Args: []ir.Operand{task}})
			g.currentBB.Terminator = &ir.ReturnTerm{Val: ir.ConstUndefined{}}
		} else {
			value := g.currentFn.NewValue(fmt.Sprintf("race_value_%d", i), resultType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: value, Callee: "ts_task_join", Args: []ir.Operand{task}})
			coerced := g.coerceJSValueBoundary(value, resultType, inner)
			g.currentBB.Terminator = &ir.ReturnTerm{Val: coerced}
		}
		g.currentBB = nextBB
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_yield"})
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
}

func (g *generator) promiseArrayTaskResultType(t types.Type) (types.Type, bool) {
	if obj, ok := t.(*types.ObjectType); ok {
		inner := g.semaResult.TaskResults[obj.Name]
		return inner, inner != nil
	}
	if union, ok := t.(*types.UnionType); ok {
		members := make([]types.Type, 0, len(union.Members))
		for _, member := range union.Members {
			inner, _ := g.promiseArrayTaskResultType(member)
			members = append(members, inner)
		}
		return types.NewUnion(members...), true
	}
	return t, true
}

func (g *generator) lowerPromiseArrayAggregate(e *ast.CallExpr, member *ast.MemberExpr, taskType *types.ObjectType, inner types.Type) ir.Operand {
	arrType := g.semanticType(e.Args[0]).(*types.ArrayType)
	resultType, _ := g.promiseArrayTaskResultType(arrType.Elem)
	source := g.lowerExpr(e.Args[0])
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	driverType := types.NewFunction(nil, inner)
	driverName := fmt.Sprintf("$promise_%s_array%d", member.Property, g.arrowCounter)
	g.arrowCounter++
	driver := ir.NewFunction(driverName, inner)
	g.currentFn = driver
	g.currentBB = driver.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := driver.NewValue("$env", driverType)
	driver.Params = append(driver.Params, env)
	array := driver.NewValue("aggregate_array", arrType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: array, Closure: env, Index: 0})
	length := driver.NewValue("aggregate_length", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: array})
	if member.Property == "all" {
		g.lowerPromiseAllArrayDriver(array, length, arrType.Elem, resultType, inner)
	} else {
		g.lowerPromiseRaceArrayDriver(array, length, arrType.Elem, resultType, inner)
	}
	g.prog.Functions = append(g.prog.Functions, driver)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("promise_array_driver", driverType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: driverName, Captures: []ir.Operand{source}, RefMask: 1})
	result := g.currentFn.NewValue("promise_array_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: result, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(inner)}}})
	return result
}

func (g *generator) lowerPromiseAllArrayDriver(array, length ir.Operand, taskElemType, resultType, inner types.Type) {
	out := inner.(*types.ArrayType)
	poll := g.currentFn.NewBlock("promise_all_array_poll")
	rejectCond := g.currentFn.NewBlock("promise_all_array_reject_cond")
	rejectBody := g.currentFn.NewBlock("promise_all_array_reject_body")
	rejectNext := g.currentFn.NewBlock("promise_all_array_reject_next")
	doneCond := g.currentFn.NewBlock("promise_all_array_done_cond")
	doneBody := g.currentFn.NewBlock("promise_all_array_done_body")
	doneNext := g.currentFn.NewBlock("promise_all_array_done_next")
	pending := g.currentFn.NewBlock("promise_all_array_pending")
	allocBB := g.currentFn.NewBlock("promise_all_array_alloc")
	fillCond := g.currentFn.NewBlock("promise_all_array_fill_cond")
	fillBody := g.currentFn.NewBlock("promise_all_array_fill_body")
	fillDone := g.currentFn.NewBlock("promise_all_array_fill_done")
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
	poll.Terminator = &ir.JumpTerm{Target: rejectCond}

	rejectIndex := g.currentFn.NewValue("all_reject_i", types.TypeNumber)
	rejectNextIndex := g.currentFn.NewValue("all_reject_next", types.TypeNumber)
	rejectCond.Phis = append(rejectCond.Phis, &ir.PhiInst{Res: rejectIndex, Incoming: []ir.PhiIncoming{{Block: poll, Value: ir.ConstNumber{Value: 0}}, {Block: rejectNext, Value: rejectNextIndex}}})
	rejectMore := g.currentFn.NewValue("all_reject_more", types.TypeBoolean)
	rejectCond.Instructions = append(rejectCond.Instructions, &ir.BinaryInst{Res: rejectMore, Op: ir.OpLt, LHS: rejectIndex, RHS: length})
	rejectCond.Terminator = &ir.BranchTerm{Cond: rejectMore, Then: rejectBody, Else: doneCond}
	task := g.currentFn.NewValue("all_reject_task", taskElemType)
	rejected := g.currentFn.NewValue("all_array_rejected", types.TypeBoolean)
	rejectBody.Instructions = append(rejectBody.Instructions,
		&ir.GetElementInst{Res: task, Array: array, Index: rejectIndex},
		&ir.CallInst{Res: rejected, Callee: "ts_task_rejected", Args: []ir.Operand{task}},
	)
	rejectFail := g.currentFn.NewBlock("promise_all_array_reject")
	rejectBody.Terminator = &ir.BranchTerm{Cond: rejected, Then: rejectFail, Else: rejectNext}
	errVal := g.currentFn.NewValue("all_array_error", types.TypeAny)
	rejectFail.Instructions = append(rejectFail.Instructions,
		&ir.CallInst{Res: errVal, Callee: "ts_task_error", Args: []ir.Operand{task}},
		&ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{errVal}, ParamTypes: []types.Type{types.TypeAny}},
	)
	rejectFail.Terminator = &ir.ReturnTerm{}
	rejectNext.Instructions = append(rejectNext.Instructions, &ir.BinaryInst{Res: rejectNextIndex, Op: ir.OpAdd, LHS: rejectIndex, RHS: ir.ConstNumber{Value: 1}})
	rejectNext.Terminator = &ir.JumpTerm{Target: rejectCond}

	doneIndex := g.currentFn.NewValue("all_done_i", types.TypeNumber)
	doneNextIndex := g.currentFn.NewValue("all_done_next", types.TypeNumber)
	doneCond.Phis = append(doneCond.Phis, &ir.PhiInst{Res: doneIndex, Incoming: []ir.PhiIncoming{{Block: rejectCond, Value: ir.ConstNumber{Value: 0}}, {Block: doneNext, Value: doneNextIndex}}})
	doneMore := g.currentFn.NewValue("all_done_more", types.TypeBoolean)
	doneCond.Instructions = append(doneCond.Instructions, &ir.BinaryInst{Res: doneMore, Op: ir.OpLt, LHS: doneIndex, RHS: length})
	doneCond.Terminator = &ir.BranchTerm{Cond: doneMore, Then: doneBody, Else: allocBB}
	doneTask := g.currentFn.NewValue("all_done_task", taskElemType)
	done := g.currentFn.NewValue("all_array_done", types.TypeBoolean)
	doneBody.Instructions = append(doneBody.Instructions,
		&ir.GetElementInst{Res: doneTask, Array: array, Index: doneIndex},
		&ir.CallInst{Res: done, Callee: "ts_task_done", Args: []ir.Operand{doneTask}},
	)
	doneBody.Terminator = &ir.BranchTerm{Cond: done, Then: doneNext, Else: pending}
	doneNext.Instructions = append(doneNext.Instructions, &ir.BinaryInst{Res: doneNextIndex, Op: ir.OpAdd, LHS: doneIndex, RHS: ir.ConstNumber{Value: 1}})
	doneNext.Terminator = &ir.JumpTerm{Target: doneCond}
	pending.Instructions = append(pending.Instructions, &ir.CallInst{Callee: "ts_task_yield"})
	pending.Terminator = &ir.JumpTerm{Target: poll}

	result := g.currentFn.NewValue("promise_all_array", out)
	allocBB.Instructions = append(allocBB.Instructions, &ir.AllocArrayInst{Res: result, ElemType: out.Elem, Length: length})
	allocBB.Terminator = &ir.JumpTerm{Target: fillCond}
	fillIndex := g.currentFn.NewValue("all_fill_i", types.TypeNumber)
	fillNextIndex := g.currentFn.NewValue("all_fill_next", types.TypeNumber)
	fillCond.Phis = append(fillCond.Phis, &ir.PhiInst{Res: fillIndex, Incoming: []ir.PhiIncoming{{Block: allocBB, Value: ir.ConstNumber{Value: 0}}, {Block: fillBody, Value: fillNextIndex}}})
	fillMore := g.currentFn.NewValue("all_fill_more", types.TypeBoolean)
	fillCond.Instructions = append(fillCond.Instructions, &ir.BinaryInst{Res: fillMore, Op: ir.OpLt, LHS: fillIndex, RHS: length})
	fillCond.Terminator = &ir.BranchTerm{Cond: fillMore, Then: fillBody, Else: fillDone}
	fillTask := g.currentFn.NewValue("all_fill_task", taskElemType)
	fillValue := g.currentFn.NewValue("all_fill_value", resultType)
	fillBody.Instructions = append(fillBody.Instructions,
		&ir.GetElementInst{Res: fillTask, Array: array, Index: fillIndex},
		&ir.CallInst{Res: fillValue, Callee: "ts_task_join", Args: []ir.Operand{fillTask}},
	)
	g.currentBB = fillBody
	stored := g.coerceJSValueBoundary(fillValue, resultType, out.Elem)
	fillBody.Instructions = append(fillBody.Instructions,
		&ir.SetElementInst{Array: result, Index: fillIndex, Val: stored},
		&ir.BinaryInst{Res: fillNextIndex, Op: ir.OpAdd, LHS: fillIndex, RHS: ir.ConstNumber{Value: 1}},
	)
	fillBody.Terminator = &ir.JumpTerm{Target: fillCond}
	fillDone.Terminator = &ir.ReturnTerm{Val: result}
	g.currentBB = fillDone
}

func (g *generator) lowerPromiseRaceArrayDriver(array, length ir.Operand, taskElemType, resultType, inner types.Type) {
	poll := g.currentFn.NewBlock("promise_race_array_poll")
	cond := g.currentFn.NewBlock("promise_race_array_cond")
	body := g.currentFn.NewBlock("promise_race_array_body")
	next := g.currentFn.NewBlock("promise_race_array_next")
	pending := g.currentFn.NewBlock("promise_race_array_pending")
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
	poll.Terminator = &ir.JumpTerm{Target: cond}

	index := g.currentFn.NewValue("race_array_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("race_array_next_i", types.TypeNumber)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: poll, Value: ir.ConstNumber{Value: 0}}, {Block: next, Value: nextIndex}}})
	more := g.currentFn.NewValue("race_array_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: pending}
	task := g.currentFn.NewValue("race_array_task", taskElemType)
	done := g.currentFn.NewValue("race_array_done", types.TypeBoolean)
	body.Instructions = append(body.Instructions,
		&ir.GetElementInst{Res: task, Array: array, Index: index},
		&ir.CallInst{Res: done, Callee: "ts_task_done", Args: []ir.Operand{task}},
	)
	settled := g.currentFn.NewBlock("promise_race_array_settled")
	body.Terminator = &ir.BranchTerm{Cond: done, Then: settled, Else: next}
	next.Instructions = append(next.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	next.Terminator = &ir.JumpTerm{Target: cond}
	pending.Instructions = append(pending.Instructions, &ir.CallInst{Callee: "ts_task_yield"})
	pending.Terminator = &ir.JumpTerm{Target: poll}

	rejected := g.currentFn.NewValue("race_array_rejected", types.TypeBoolean)
	settled.Instructions = append(settled.Instructions, &ir.CallInst{Res: rejected, Callee: "ts_task_rejected", Args: []ir.Operand{task}})
	rejectBB := g.currentFn.NewBlock("promise_race_array_reject")
	fulfillBB := g.currentFn.NewBlock("promise_race_array_fulfill")
	settled.Terminator = &ir.BranchTerm{Cond: rejected, Then: rejectBB, Else: fulfillBB}
	errVal := g.currentFn.NewValue("race_array_error", types.TypeAny)
	rejectBB.Instructions = append(rejectBB.Instructions,
		&ir.CallInst{Res: errVal, Callee: "ts_task_error", Args: []ir.Operand{task}},
		&ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{errVal}, ParamTypes: []types.Type{types.TypeAny}},
	)
	rejectBB.Terminator = &ir.ReturnTerm{}
	value := g.currentFn.NewValue("race_array_value", resultType)
	fulfillBB.Instructions = append(fulfillBB.Instructions, &ir.CallInst{Res: value, Callee: "ts_task_join", Args: []ir.Operand{task}})
	g.currentBB = fulfillBB
	coerced := g.coerceJSValueBoundary(value, resultType, inner)
	fulfillBB.Terminator = &ir.ReturnTerm{Val: coerced}
	g.currentBB = fulfillBB
}

func (g *generator) lowerPromiseStaticCall(e *ast.CallExpr, member *ast.MemberExpr) (ir.Operand, bool) {
	ident, ok := member.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "Promise" {
		return nil, false
	}
	if member.Property == "all" || member.Property == "race" {
		taskType := g.semanticType(e).(*types.ObjectType)
		inner := g.semaResult.TaskResults[taskType.Name]
		if literal, ok := e.Args[0].(*ast.ArrayLit); ok {
			return g.lowerPromiseLiteralAggregate(member, taskType, inner, literal), true
		}
		return g.lowerPromiseArrayAggregate(e, member, taskType, inner), true
	}
	taskType := g.semanticType(e).(*types.ObjectType)
	inner := g.semaResult.TaskResults[taskType.Name]
	if member.Property == "resolve" {
		if argObj, ok := g.semanticType(e.Args[0]).(*types.ObjectType); ok {
			if _, isTask := g.semaResult.TaskResults[argObj.Name]; isTask {
				return g.lowerExpr(e.Args[0]), true
			}
			if thenObj, thenFn, isThenable := g.thenableMethodType(argObj); isThenable {
				return g.lowerThenablePromise(e, taskType, inner, thenObj, thenFn), true
			}
		}
	}

	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	closureType := types.NewFunction(nil, inner)
	liftedName := fmt.Sprintf("$promise%d", g.arrowCounter)
	g.arrowCounter++
	var capture ir.Operand
	var captureType types.Type
	if member.Property == "resolve" {
		capture = g.lowerExpr(e.Args[0])
		captureType = g.semanticType(e.Args[0])
		capture = g.coerceJSValueBoundary(capture, captureType, inner)
		captureType = inner
	} else {
		capture = g.lowerExpr(e.Args[0])
		capture = g.boxJSValue(capture, g.semanticType(e.Args[0]))
		captureType = types.TypeAny
	}

	lifted := ir.NewFunction(liftedName, inner)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", closureType)
	lifted.Params = append(lifted.Params, env)
	captured := lifted.NewValue("promise_value", captureType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: captured, Closure: env, Index: 0})
	if member.Property == "resolve" {
		g.currentBB.Terminator = &ir.ReturnTerm{Val: captured}
	} else {
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{captured}, ParamTypes: []types.Type{types.TypeAny}})
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect

	closure := g.currentFn.NewValue("promise_closure", closureType)
	var refMask uint64
	if irHeapRefType(captureType) {
		refMask = 1
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: liftedName, Captures: []ir.Operand{capture}, RefMask: refMask})
	task := g.currentFn.NewValue("promise_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: task, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(inner)}}})
	return task, true
}

func (g *generator) lowerFunctionExpr(e *ast.FunctionExpr) ir.Operand {
	fnType := g.semanticType(e).(*types.FunctionType)
	paramSet := make(map[string]struct{}, len(e.Params))
	for _, p := range e.Params {
		if !p.IsThis {
			paramSet[p.Name] = struct{}{}
		}
	}
	captureNames := g.collectBlockClosureCaptures(e.Body, paramSet)
	captureOps := make([]ir.Operand, 0, len(captureNames))
	captureTypes := make([]types.Type, 0, len(captureNames))
	var refMask uint64
	for i, captureName := range captureNames {
		cell, valueType := g.ensureCaptureCell(captureName)
		captureOps = append(captureOps, cell)
		captureTypes = append(captureTypes, valueType)
		refMask |= uint64(1) << i
	}
	outerFn, outerBB, outerLocals, outerProvenance, outerDirectCallees := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	name := fmt.Sprintf("$function%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, fnType.Return)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)

	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	for i, captureName := range captureNames {
		cellType := captureOps[i].Type()
		cell := lifted.NewValue(captureName+"_cell_capture", cellType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: cell, Closure: env, Index: i})
		g.locals[captureName] = cell
		g.bindCaptureCell(lifted, captureName, cell, captureTypes[i])
	}
	if fnType.This != nil {
		thisVal := lifted.NewValue("$this", fnType.This)
		lifted.Params = append(lifted.Params, thisVal)
		g.locals["$this"] = thisVal
	}
	runtimeIndex := 0
	for _, p := range e.Params {
		if p.IsThis {
			continue
		}
		pt := types.TypeAny
		if runtimeIndex < len(fnType.Params) {
			pt = fnType.Params[runtimeIndex].Type
		}
		v := lifted.NewValue(p.Name, pt)
		lifted.Params = append(lifted.Params, v)
		g.locals[p.Name] = v
		runtimeIndex++
	}
	for _, stmt := range e.Body.Statements {
		g.lowerStatement(stmt)
	}
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProvenance, outerDirectCallees
	res := g.currentFn.NewValue("closure", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: res, Function: name, Captures: captureOps, RefMask: refMask})
	return res
}

func consolePrinterForType(t types.Type) (string, bool) {
	switch t {
	case types.TypeNumber:
		return "ts_print_val", true
	case types.TypeString:
		return "ts_print_str", true
	case types.TypeBoolean:
		return "ts_print_bool", true
	case types.TypeUndefined:
		return "ts_print_undefined", true
	case types.TypeNull:
		return "ts_print_null", true
	default:
		return "", false
	}
}

func (g *generator) staticStringKey(expr ast.Expr) (string, bool) {
	switch key := expr.(type) {
	case *ast.StringLit:
		return key.Value, true
	case *ast.IdentExpr:
		if op, ok := g.locals[key.Name]; ok {
			if value, ok := op.(ir.ConstString); ok {
				return value.Value, true
			}
		}
	}
	return "", false
}

func (g *generator) concatNativeStrings(a, b ir.Operand) ir.Operand {
	res := g.currentFn.NewValue("json_str", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_string_concat", Args: []ir.Operand{a, b}})
	return res
}

func jsonConstantType(v any) types.Type {
	switch x := v.(type) {
	case nil:
		return types.TypeNull
	case bool:
		return types.TypeBoolean
	case float64:
		return types.TypeNumber
	case string:
		return types.TypeString
	case []any:
		var elem types.Type = types.TypeAny
		if len(x) > 0 {
			elem = jsonConstantType(x[0])
		}
		return types.NewArray(elem)
	default:
		m := v.(map[string]any)
		obj := types.NewObject("")
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			obj.AddField(k, jsonConstantType(m[k]), false)
		}
		return obj
	}
}

func (g *generator) lowerJSONConstant(v any) ir.Operand {
	switch x := v.(type) {
	case nil:
		return ir.ConstNull{}
	case bool:
		return ir.ConstBool{Value: x}
	case float64:
		return ir.ConstNumber{Value: x}
	case string:
		return ir.ConstString{Value: x}
	case []any:
		arrType := jsonConstantType(x).(*types.ArrayType)
		res := g.currentFn.NewValue("json_array", arrType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{Res: res, ElemType: arrType.Elem, Length: ir.ConstNumber{Value: float64(len(x))}})
		for i, item := range x {
			value := g.lowerJSONConstant(item)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{Array: res, Index: ir.ConstNumber{Value: float64(i)}, Val: value})
		}
		return res
	default:
		m := v.(map[string]any)
		objType := jsonConstantType(m).(*types.ObjectType)
		offsets, refMask, shape := g.objectLayout(objType)
		res := g.currentFn.NewValue("json_object", objType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
		for _, name := range objType.FieldOrder {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: name, Offset: offsets[name], Val: g.lowerJSONConstant(m[name])})
		}
		return res
	}
}

func (g *generator) lowerJSONStringifyValue(value ir.Operand, t types.Type) ir.Operand {
	switch t {
	case types.TypeNumber:
		res := g.currentFn.NewValue("json_number", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_number_to_string", Args: []ir.Operand{value}})
		return res
	case types.TypeString:
		return g.concatNativeStrings(g.concatNativeStrings(ir.ConstString{Value: "\""}, value), ir.ConstString{Value: "\""})
	case types.TypeBoolean:
		res := g.currentFn.NewValue("json_bool", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_bool_to_string", Args: []ir.Operand{value}})
		return res
	case types.TypeNull:
		return ir.ConstString{Value: "null"}
	}
	if arr, ok := t.(*types.ArrayType); ok {
		return g.lowerJSONStringifyArray(value, arr)
	}
	if obj, ok := t.(*types.ObjectType); ok {
		offsets, _, _ := g.objectLayout(obj)
		acc := ir.Operand(ir.ConstString{Value: "{"})
		for i, name := range obj.FieldOrder {
			prefix := "\"" + name + "\":"
			if i > 0 {
				prefix = "," + prefix
			}
			acc = g.concatNativeStrings(acc, ir.ConstString{Value: prefix})
			field := obj.Fields[name]
			fv := g.currentFn.NewValue("json_field", field.Type)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: fv, Obj: value, Field: name, Offset: offsets[name]})
			acc = g.concatNativeStrings(acc, g.lowerJSONStringifyValue(fv, field.Type))
		}
		return g.concatNativeStrings(acc, ir.ConstString{Value: "}"})
	}
	return ir.ConstString{Value: "{}"}
}

func (g *generator) lowerJSONStringifyArray(array ir.Operand, arr *types.ArrayType) ir.Operand {
	start := g.currentBB
	length := g.currentFn.NewValue("json_len", types.TypeNumber)
	start.Instructions = append(start.Instructions, &ir.ArrayLengthInst{Res: length, Array: array})
	hasAny := g.currentFn.NewValue("json_has", types.TypeBoolean)
	start.Instructions = append(start.Instructions, &ir.BinaryInst{Res: hasAny, Op: ir.OpGt, LHS: length, RHS: ir.ConstNumber{Value: 0}})
	nonEmpty := g.currentFn.NewBlock("json_array_nonempty")
	empty := g.currentFn.NewBlock("json_array_empty")
	cond := g.currentFn.NewBlock("json_array_cond")
	body := g.currentFn.NewBlock("json_array_body")
	done := g.currentFn.NewBlock("json_array_done")
	join := g.currentFn.NewBlock("json_array_join")
	start.Terminator = &ir.BranchTerm{Cond: hasAny, Then: nonEmpty, Else: empty}
	empty.Terminator = &ir.JumpTerm{Target: join}

	g.currentBB = nonEmpty
	first := g.currentFn.NewValue("json_elem0", arr.Elem)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: first, Array: array, Index: ir.ConstNumber{Value: 0}})
	firstText := g.lowerJSONStringifyValue(first, arr.Elem)
	acc0 := g.concatNativeStrings(ir.ConstString{Value: "["}, firstText)
	nonEmptyEnd := g.currentBB
	nonEmptyEnd.Terminator = &ir.JumpTerm{Target: cond}

	index := g.currentFn.NewValue("json_i", types.TypeNumber)
	acc := g.currentFn.NewValue("json_acc", types.TypeString)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: nonEmptyEnd, Value: ir.ConstNumber{Value: 1}}}}, &ir.PhiInst{Res: acc, Incoming: []ir.PhiIncoming{{Block: nonEmptyEnd, Value: acc0}}})
	more := g.currentFn.NewValue("json_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: done}

	g.currentBB = body
	elem := g.currentFn.NewValue("json_elem", arr.Elem)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: elem, Array: array, Index: index})
	text := g.lowerJSONStringifyValue(elem, arr.Elem)
	commaText := g.concatNativeStrings(ir.ConstString{Value: ","}, text)
	nextAcc := g.concatNativeStrings(acc, commaText)
	next := g.currentFn.NewValue("json_next", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	bodyEnd := g.currentBB
	bodyEnd.Terminator = &ir.JumpTerm{Target: cond}
	cond.Phis[0].Incoming = append(cond.Phis[0].Incoming, ir.PhiIncoming{Block: bodyEnd, Value: next})
	cond.Phis[1].Incoming = append(cond.Phis[1].Incoming, ir.PhiIncoming{Block: bodyEnd, Value: nextAcc})

	g.currentBB = done
	final := g.concatNativeStrings(acc, ir.ConstString{Value: "]"})
	doneEnd := g.currentBB
	doneEnd.Terminator = &ir.JumpTerm{Target: join}
	g.currentBB = join
	result := g.currentFn.NewValue("json_result", types.TypeString)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{{Block: empty, Value: ir.ConstString{Value: "[]"}}, {Block: doneEnd, Value: final}}})
	return result
}

func (g *generator) lowerJSONCall(call *ast.CallExpr, mem *ast.MemberExpr) ir.Operand {
	switch mem.Property {
	case "parse":
		if lit, ok := call.Args[0].(*ast.StringLit); ok {
			var decoded any
			_ = json.Unmarshal([]byte(lit.Value), &decoded)
			value := g.lowerJSONConstant(decoded)
			switch decoded.(type) {
			case []any, map[string]any:
				return value
			default:
				return g.boxJSValue(value, value.Type())
			}
		}
		text := g.lowerExpr(call.Args[0])
		res := g.currentFn.NewValue("json_parsed", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_json_parse_scalar", Args: []ir.Operand{text}})
		return res
	default:
		value := g.lowerExpr(call.Args[0])
		return g.lowerJSONStringifyValue(value, value.Type())
	}
}

func isBuiltinRegExpType(t types.Type) bool {
	obj, ok := t.(*types.ObjectType)
	return ok && obj.Name == "$RegExp"
}

func classifyNativeRegExp(pattern, flags string) (kind float64, needle string, flagBits float64, err error) {
	for _, flag := range flags {
		if flag == 'i' {
			flagBits = 1
			continue
		}
		return 0, "", 0, fmt.Errorf("unsupported native RegExp flag %q", flag)
	}
	if strings.HasPrefix(pattern, "^") {
		needle = pattern[1:]
		if strings.ContainsAny(needle, `.*+?[](){}|^$\\`) {
			return 0, "", 0, fmt.Errorf("unsupported anchored RegExp pattern %q", pattern)
		}
		return 1, needle, flagBits, nil
	}
	if strings.HasSuffix(pattern, `\d+`) {
		needle = strings.TrimSuffix(pattern, `\d+`)
		if needle == "" || strings.ContainsAny(needle, `.*+?[](){}|^$\\`) {
			return 0, "", 0, fmt.Errorf("unsupported digit RegExp pattern %q", pattern)
		}
		return 2, needle, flagBits, nil
	}
	if strings.ContainsAny(pattern, `.*+?[](){}|^$\\`) {
		return 0, "", 0, fmt.Errorf("unsupported native RegExp pattern %q", pattern)
	}
	return 0, pattern, flagBits, nil
}

func (g *generator) lowerNativeRegExp(pattern, flags string, resultType types.Type) ir.Operand {
	kind, needle, flagBits, err := classifyNativeRegExp(pattern, flags)
	if err != nil {
		return g.failExpr("%v", err)
	}
	res := g.currentFn.NewValue("regexp", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: "$RegExp", FieldCount: 4, RefMask: 0b0011})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: res, Field: "source", Offset: 16, Val: ir.ConstString{Value: pattern}},
		&ir.SetFieldInst{Obj: res, Field: "needle", Offset: 24, Val: ir.ConstString{Value: needle}},
		&ir.SetFieldInst{Obj: res, Field: "kind", Offset: 32, Val: ir.ConstNumber{Value: kind}},
		&ir.SetFieldInst{Obj: res, Field: "flags", Offset: 40, Val: ir.ConstNumber{Value: flagBits}},
	)
	return res
}

func (g *generator) emitRegExpTest(call *ast.CallExpr, mem *ast.MemberExpr) ir.Operand {
	obj := g.lowerExpr(mem.Object)
	text := g.lowerExpr(call.Args[0])
	res := g.currentFn.NewValue("regexp_test", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_regexp_test", Args: []ir.Operand{obj, text}})
	return res
}

func isBuiltinDateType(t types.Type) bool {
	obj, ok := t.(*types.ObjectType)
	return ok && obj.Name == "$Date"
}

func (g *generator) emitDateMethodCall(call *ast.CallExpr, mem *ast.MemberExpr) ir.Operand {
	date := g.lowerExpr(mem.Object)
	callee := map[string]string{
		"toISOString":    "ts_date_to_iso",
		"getUTCFullYear": "ts_date_get_year",
		"getUTCMonth":    "ts_date_get_month",
		"getUTCDate":     "ts_date_get_date",
		"getUTCHours":    "ts_date_get_hours",
		"getUTCMinutes":  "ts_date_get_minutes",
		"getUTCSeconds":  "ts_date_get_seconds",
	}[mem.Property]
	resultType := g.semanticType(call)
	res := g.currentFn.NewValue("date_result", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: callee, Args: []ir.Operand{date}})
	return res
}

func (g *generator) builtinCollectionInfo(t types.Type) *sema.BuiltinCollectionInfo {
	obj, ok := t.(*types.ObjectType)
	if !ok || g.semaResult == nil {
		return nil
	}
	return g.semaResult.BuiltinCollections[obj.Name]
}

func (g *generator) emitBuiltinCollectionCall(call *ast.CallExpr, mem *ast.MemberExpr, info *sema.BuiltinCollectionInfo) ir.Operand {
	obj := g.lowerExpr(mem.Object)
	boxArg := func(i int) ir.Operand {
		v := g.lowerExpr(call.Args[i])
		return g.boxJSValue(v, g.semanticType(call.Args[i]))
	}
	resultType := g.semanticType(call)
	switch info.Kind + "." + mem.Property {
	case "Map.set":
		res := g.currentFn.NewValue("map", info.Instance)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_set", Args: []ir.Operand{obj, boxArg(0), boxArg(1)}})
		return res
	case "Set.add":
		res := g.currentFn.NewValue("set", info.Instance)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_set", Args: []ir.Operand{obj, boxArg(0), ir.ConstUndefined{}}})
		return res
	case "Map.get":
		res := g.currentFn.NewValue("map_value", resultType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_get", Args: []ir.Operand{obj, boxArg(0)}})
		return res
	case "Map.has", "Set.has":
		res := g.currentFn.NewValue("has", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_has", Args: []ir.Operand{obj, boxArg(0)}})
		return res
	case "Map.delete", "Set.delete":
		res := g.currentFn.NewValue("deleted", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_delete", Args: []ir.Operand{obj, boxArg(0)}})
		return res
	default:
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_collection_clear", Args: []ir.Operand{obj}})
		return nil
	}
}

func (g *generator) lowerConsoleLog(expr ast.Expr) ir.Operand {
	t := g.semanticType(expr)
	if t == types.TypeAny {
		value := g.lowerExpr(expr)
		if actual := value.Type(); actual != nil && actual != types.TypeAny {
			if callee, ok := consolePrinterForType(actual); ok {
				if actual == types.TypeUndefined || actual == types.TypeNull {
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: callee})
					return nil
				}
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: callee, Args: []ir.Operand{value}, ParamTypes: []types.Type{actual}})
				return nil
			}
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_js_print", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeAny}})
		return nil
	}
	if irJSValueType(t) {
		value := g.lowerExpr(expr)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_js_print", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeAny}})
		return nil
	}
	if callee, ok := consolePrinterForType(t); ok {
		if t == types.TypeUndefined || t == types.TypeNull {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: callee})
			return nil
		}
		value := g.lowerExpr(expr)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: callee, Args: []ir.Operand{value}, ParamTypes: []types.Type{t}})
		return nil
	}
	union := t.(*types.UnionType)
	var concrete types.Type
	hasNull, hasUndefined := false, false
	for _, member := range union.Members {
		switch member {
		case types.TypeNull:
			hasNull = true
		case types.TypeUndefined:
			hasUndefined = true
		default:
			concrete = member
		}
	}
	printer, _ := consolePrinterForType(concrete)
	value := g.lowerExpr(expr)
	join := g.currentFn.NewBlock("print_join")
	emitMissing := func(name string, sentinel ir.Operand, callee string) {
		cond := g.currentFn.NewValue(name+"_match", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: cond, Op: ir.OpEq, LHS: value, RHS: sentinel})
		printBB := g.currentFn.NewBlock(name)
		nextBB := g.currentFn.NewBlock(name + "_next")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: cond, Then: printBB, Else: nextBB}
		printBB.Instructions = append(printBB.Instructions, &ir.CallInst{Callee: callee})
		printBB.Terminator = &ir.JumpTerm{Target: join}
		g.currentBB = nextBB
	}
	if hasUndefined {
		emitMissing("print_undefined", ir.ConstUndefined{}, "ts_print_undefined")
	}
	if hasNull {
		emitMissing("print_null", ir.ConstNull{}, "ts_print_null")
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: printer, Args: []ir.Operand{value}, ParamTypes: []types.Type{concrete}})
	g.currentBB.Terminator = &ir.JumpTerm{Target: join}
	g.currentBB = join
	return nil
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
		captureCells:      make(map[*ir.Function]map[string]ir.Operand),
		captureCellTypes:  make(map[*ir.Function]map[string]types.Type),
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
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)

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
	if fnDecl.IsAsync {
		return g.lowerAsyncFunction(fnDecl, fnType, fnDecl.Name), nil
	}
	return g.lowerFunctionAs(fnDecl, fnType, fnDecl.Name)
}

func (g *generator) lowerAsyncFunction(fnDecl *ast.FunctionDecl, fnType *types.FunctionType, name string) *ir.Function {
	taskType := fnType.Return.(*types.ObjectType)
	innerType := g.semaResult.AsyncResults[fnDecl]

	wrapper := ir.NewFunction(name, taskType)
	g.currentFn = wrapper
	g.currentBB = wrapper.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)

	captures := make([]ir.Operand, 0, len(fnDecl.Params))
	var refMask uint64
	for i, p := range fnDecl.Params {
		pt := types.TypeAny
		if i < len(fnType.Params) {
			pt = fnType.Params[i].Type
		}
		v := wrapper.NewValue(p.Name, pt)
		wrapper.Params = append(wrapper.Params, v)
		g.locals[p.Name] = v
		captures = append(captures, v)
		if irHeapRefType(pt) {
			refMask |= uint64(1) << i
		}
	}

	closureType := types.NewFunction(nil, innerType)
	liftedName := fmt.Sprintf("$async%d", g.arrowCounter)
	g.arrowCounter++
	outerFn, outerBB, outerLocals, outerProvenance, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee

	lifted := ir.NewFunction(liftedName, innerType)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", closureType)
	lifted.Params = append(lifted.Params, env)
	for i, p := range fnDecl.Params {
		pt := captures[i].Type()
		v := lifted.NewValue(p.Name+"_capture", pt)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: v, Closure: env, Index: i})
		g.locals[p.Name] = v
	}
	if fnDecl.Body != nil {
		for _, stmt := range fnDecl.Body.Statements {
			g.lowerStatement(stmt)
		}
	}
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}

	g.prog.Functions = append(g.prog.Functions, lifted)

	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProvenance, outerDirect
	closure := wrapper.NewValue("async_closure", closureType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: liftedName, Captures: captures, RefMask: refMask})
	task := wrapper.NewValue("async_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: task, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(innerType)}}})
	g.currentBB.Terminator = &ir.ReturnTerm{Val: task}
	return wrapper
}

func (g *generator) lowerFunctionAs(fnDecl *ast.FunctionDecl, fnType *types.FunctionType, name string) (*ir.Function, error) {
	var retType types.Type = types.TypeVoid
	if fnType != nil {
		retType = fnType.Return
	}

	irFn := ir.NewFunction(name, retType)
	g.currentFn = irFn
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)

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
	return irFn, g.err
}

func (g *generator) provenLocalType(expr ast.Expr) (types.Type, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || g.localProvenance == nil {
		return nil, false
	}
	t, ok := g.localProvenance[ident.Name]
	return t, ok && t != nil
}

func dynamicBackedObjectType(t *types.ObjectType) bool {
	if t == nil {
		return false
	}
	switch t.Name {
	case "$GlobalScope", "$DOMException":
		return true
	default:
		return false
	}
}

func (g *generator) provenObjectType(expr ast.Expr) (*types.ObjectType, bool) {
	t, ok := g.provenLocalType(expr)
	if !ok {
		return nil, false
	}
	objectType, ok := t.(*types.ObjectType)
	return objectType, ok && objectType != nil
}

func (g *generator) provenFunctionType(expr ast.Expr) (*types.FunctionType, bool) {
	t, ok := g.provenLocalType(expr)
	if !ok {
		return nil, false
	}
	fnType, ok := t.(*types.FunctionType)
	return fnType, ok && fnType != nil
}

func (g *generator) directCalleeForExpr(expr ast.Expr) (string, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return "", false
	}
	if target := g.localDirectCallee[ident.Name]; target != "" {
		return target, true
	}
	if _, local := g.locals[ident.Name]; local {
		return "", false
	}
	name := ident.Name
	if imported := g.semaResult.ImportAliases[name]; imported != "" {
		return imported, true
	}
	return name, true
}

func (g *generator) unboxKnownObject(value ir.Operand, objectType *types.ObjectType) ir.Operand {
	return g.coerceJSValueBoundary(value, types.TypeAny, objectType)
}

func (g *generator) materializeDynamicObject(value ir.Operand, objectType *types.ObjectType) ir.Operand {
	if info := g.semaResult.Classes[objectType.Name]; info != nil {
		return g.failExpr("dynamic structural conversion to class %q is not implemented", objectType.Name)
	}
	offsets, refMask, shape := g.objectLayout(objectType)
	res := g.currentFn.NewValue("dynamic_struct", objectType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	for _, name := range objectType.FieldOrder {
		field := objectType.Fields[name]
		boxed := g.lowerDynamicGet(value, name)
		converted := g.coerceJSValueBoundary(boxed, types.TypeAny, field.Type)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
			Obj: res, Field: name, Offset: offsets[name], Val: converted,
		})
	}
	return res
}

func (g *generator) newDOMException(message, name ir.Operand) ir.Operand {
	t := g.semaResult.DOMExceptionType
	dyn := g.currentFn.NewValue("dom_exception_dynamic", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: dyn, Callee: "ts_dynamic_object_new"})
	for key, value := range map[string]ir.Operand{
		"code": ir.ConstNumber{Value: 0}, "message": message, "name": name,
	} {
		boxed := value
		if !irJSValueType(value.Type()) {
			boxed = g.boxJSValue(value, value.Type())
		}
		set := g.currentFn.NewValue("dom_exception_set", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: set, Callee: "ts_dynamic_set", Args: []ir.Operand{dyn, ir.ConstString{Value: key}, boxed},
			ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny},
		})
	}
	return g.coerceJSValueBoundary(dyn, types.TypeAny, t)
}

func (g *generator) lowerDynamicObjectLiteral(lit *ast.ObjectLit) ir.Operand {
	obj := g.currentFn.NewValue("dynamic_object", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: obj, Callee: "ts_dynamic_object_new"})
	for _, prop := range lit.Properties {
		if prop.Spread {
			return g.failExpr("dynamic object spread is not implemented yet")
		}
		value := g.lowerExpr(prop.Value)
		boxed := g.boxJSValue(value, g.semanticType(prop.Value))
		res := g.currentFn.NewValue("dynamic_init", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: res, Callee: "ts_dynamic_set",
			Args:       []ir.Operand{obj, ir.ConstString{Value: prop.Key}, boxed},
			ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny},
		})
	}
	return obj
}

func (g *generator) lowerDynamicGet(obj ir.Operand, key string) ir.Operand {
	res := g.currentFn.NewValue("dynamic_get", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_dynamic_get",
		Args:       []ir.Operand{obj, ir.ConstString{Value: key}},
		ParamTypes: []types.Type{types.TypeAny, types.TypeString},
	})
	return res
}

func (g *generator) lowerDynamicSet(obj ir.Operand, key string, rhs ast.Expr) ir.Operand {
	value := g.lowerExpr(rhs)
	boxed := g.boxJSValue(value, g.semanticType(rhs))
	res := g.currentFn.NewValue("dynamic_set", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_dynamic_set",
		Args:       []ir.Operand{obj, ir.ConstString{Value: key}, boxed},
		ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny},
	})
	return res
}

func (g *generator) lowerStatement(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		for _, child := range s.Statements {
			g.lowerStatement(child)
		}
	case *ast.VarDeclStmt:
		resolved := g.semaResult.VarTypes[s]
		for i, d := range s.Declarations {
			var initOp ir.Operand
			if d.Init != nil {
				if typeNodeIsAny(d.Type) {
					if lit, ok := d.Init.(*ast.ObjectLit); ok {
						initOp = g.lowerDynamicObjectLiteral(lit)
					}
				}
				if initOp == nil {
					initOp = g.lowerExpr(d.Init)
				}
				if i < len(resolved) {
					sourceType := g.semanticType(d.Init)
					targetType := resolved[i]
					if objectType, ok := targetType.(*types.ObjectType); ok && irJSValueType(sourceType) {
						if provenance, known := g.provenObjectType(d.Init); known {
							initOp = g.unboxKnownObject(initOp, provenance)
						} else {
							initOp = g.materializeDynamicObject(initOp, objectType)
						}
					} else {
						initOp = g.coerceJSValueBoundary(initOp, sourceType, targetType)
					}
					if irJSValueType(targetType) {
						switch concrete := sourceType.(type) {
						case *types.ObjectType:
							if _, dynamicLiteral := d.Init.(*ast.ObjectLit); !dynamicLiteral && !dynamicBackedObjectType(concrete) {
								g.localProvenance[d.Name] = concrete
							}
						case *types.FunctionType:
							g.localProvenance[d.Name] = concrete
							if target, ok := g.directCalleeForExpr(d.Init); ok {
								g.localDirectCallee[d.Name] = target
							}
						}
					}
				}
			}
			if initOp == nil {
				initOp = ir.ConstNumber{Value: 0}
			}
			g.locals[d.Name] = initOp
		}
	case *ast.ThrowStmt:
		val := g.lowerExpr(s.Value)
		val = g.boxJSValue(val, g.semanticType(s.Value))
		g.routeThrownValue(val)
	case *ast.TryStmt:
		g.lowerTry(s)
	case *ast.ReturnStmt:
		if fctx := g.currentFinally(); fctx != nil {
			var val ir.Operand = ir.ConstUndefined{}
			if s.Value != nil {
				val = g.lowerExpr(s.Value)
				val = g.boxJSValue(val, g.semanticType(s.Value))
			}
			g.routeFinallyCompletion(fctx, 1, val)
			break
		}
		var val ir.Operand
		if s.Value != nil {
			val = g.lowerExpr(s.Value)
			val = g.coerceJSValueBoundary(val, g.semanticType(s.Value), g.currentFn.ReturnType)
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
		case *ast.ForStmt:
			walk(node.Init)
			walk(node.Cond)
			walk(node.Post)
			walk(node.Body)
		}
	}
	walk(stmt)
	return res
}

func (g *generator) lowerForOf(s *ast.ForOfStmt) {
	iterable := g.lowerExpr(s.Iterable)
	arrType := g.semaResult.Types[s.Iterable].(*types.ArrayType)

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

func (g *generator) ownedStringAppendCandidate(s *ast.ForStmt) (string, ast.Expr, bool) {
	body, ok := s.Body.(*ast.BlockStmt)
	if !ok || len(body.Statements) != 1 {
		return "", nil, false
	}
	exprStmt, ok := body.Statements[0].(*ast.ExprStmt)
	if !ok {
		return "", nil, false
	}
	assign, ok := exprStmt.Expr.(*ast.AssignExpr)
	if !ok || assign.Op != token.Eq {
		return "", nil, false
	}
	left, ok := assign.Left.(*ast.IdentExpr)
	if !ok {
		return "", nil, false
	}
	bin, ok := assign.Right.(*ast.BinaryExpr)
	if !ok || bin.Op != token.Plus || g.semanticType(bin) != types.TypeString {
		return "", nil, false
	}
	base, ok := bin.Left.(*ast.IdentExpr)
	if !ok || base.Name != left.Name {
		return "", nil, false
	}
	if _, ok := g.locals[left.Name].(ir.ConstString); !ok {
		return "", nil, false
	}
	// Keep the first ownership proof intentionally narrow. Pure literals and a
	// different local cannot observe or alias the accumulator during append.
	switch suffix := bin.Right.(type) {
	case *ast.StringLit, *ast.NumberLit, *ast.BoolLit:
		return left.Name, suffix, true
	case *ast.IdentExpr:
		if suffix.Name != left.Name {
			return left.Name, suffix, true
		}
	}
	return "", nil, false
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
	ownedStringName, ownedStringSuffix, ownedStringAppend := g.ownedStringAppendCandidate(s)
	if ownedStringAppend {
		seed := g.currentFn.NewValue("str_owned", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: seed, Callee: "ts_string_builder_seed", Args: []ir.Operand{g.locals[ownedStringName]},
		})
		g.locals[ownedStringName] = seed
	}
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
	if ownedStringAppend {
		suffix := g.lowerExpr(ownedStringSuffix)
		suffix = g.coerceStringOperand(ownedStringSuffix, suffix)
		res := g.currentFn.NewValue("str_append", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: res, Callee: "ts_string_append_owned", Args: []ir.Operand{g.locals[ownedStringName], suffix},
		})
		g.locals[ownedStringName] = res
	} else {
		g.lowerStatement(s.Body)
	}
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
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
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

func removeNullishIRType(t types.Type) types.Type {
	union, ok := t.(*types.UnionType)
	if !ok {
		return t
	}
	members := make([]types.Type, 0, len(union.Members))
	for _, member := range union.Members {
		if member != types.TypeNull && member != types.TypeUndefined {
			members = append(members, member)
		}
	}
	return members[0]
}

func (g *generator) lowerOptionalMember(e *ast.MemberExpr) ir.Operand {
	baseType := removeNullishIRType(g.semanticType(e.Object))
	objType, ok := baseType.(*types.ObjectType)
	if !ok {
		return g.failExpr("native optional chaining currently requires a closed object receiver, got %s", baseType)
	}
	offsets, _, _ := g.objectLayout(objType)
	offset := offsets[e.Property]

	obj := g.lowerExpr(e.Object)
	start := g.currentBB
	checkNull := g.currentFn.NewBlock("optional_check_null")
	missing := g.currentFn.NewBlock("optional_missing")
	load := g.currentFn.NewBlock("optional_load")
	join := g.currentFn.NewBlock("optional_join")

	isUndefined := g.currentFn.NewValue("optional_undefined", types.TypeBoolean)
	start.Instructions = append(start.Instructions, &ir.BinaryInst{Res: isUndefined, Op: ir.OpEq, LHS: obj, RHS: ir.ConstUndefined{}})
	start.Terminator = &ir.BranchTerm{Cond: isUndefined, Then: missing, Else: checkNull}

	g.currentBB = checkNull
	isNull := g.currentFn.NewValue("optional_null", types.TypeBoolean)
	checkNull.Instructions = append(checkNull.Instructions, &ir.BinaryInst{Res: isNull, Op: ir.OpEq, LHS: obj, RHS: ir.ConstNull{}})
	checkNull.Terminator = &ir.BranchTerm{Cond: isNull, Then: missing, Else: load}

	missing.Terminator = &ir.JumpTerm{Target: join}

	g.currentBB = load
	resultType := types.TypeAny
	if t := g.semanticType(e); t != nil {
		resultType = t
	}
	loaded := g.currentFn.NewValue("optional_field", resultType)
	load.Instructions = append(load.Instructions, &ir.GetFieldInst{Res: loaded, Obj: obj, Field: e.Property, Offset: offset})
	load.Terminator = &ir.JumpTerm{Target: join}

	g.currentBB = join
	res := g.currentFn.NewValue("optional", resultType)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{
		{Block: missing, Value: ir.ConstUndefined{}},
		{Block: load, Value: loaded},
	}})
	return res
}

func (g *generator) lowerNullishExpr(e *ast.BinaryExpr) ir.Operand {
	lhs := g.lowerExpr(e.Left)
	lhsBB := g.currentBB
	checkNullBB := g.currentFn.NewBlock("nullish_check_null")
	rhsBB := g.currentFn.NewBlock("nullish_rhs")
	shortBB := g.currentFn.NewBlock("nullish_value")
	joinBB := g.currentFn.NewBlock("nullish_join")

	undef := g.currentFn.NewValue("is_undefined", types.TypeBoolean)
	lhsBB.Instructions = append(lhsBB.Instructions, &ir.BinaryInst{Res: undef, Op: ir.OpEq, LHS: lhs, RHS: ir.ConstUndefined{}})
	lhsBB.Terminator = &ir.BranchTerm{Cond: undef, Then: rhsBB, Else: checkNullBB}

	g.currentBB = checkNullBB
	isNull := g.currentFn.NewValue("is_null", types.TypeBoolean)
	checkNullBB.Instructions = append(checkNullBB.Instructions, &ir.BinaryInst{Res: isNull, Op: ir.OpEq, LHS: lhs, RHS: ir.ConstNull{}})
	checkNullBB.Terminator = &ir.BranchTerm{Cond: isNull, Then: rhsBB, Else: shortBB}

	g.currentBB = shortBB
	shortBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = rhsBB
	rhs := g.lowerExpr(e.Right)
	rhsEnd := g.currentBB
	if rhsEnd.Terminator == nil {
		rhsEnd.Terminator = &ir.JumpTerm{Target: joinBB}
	}

	g.currentBB = joinBB
	resultType := rhs.Type()
	if t := g.semanticType(e); t != nil {
		resultType = t
	}
	res := g.currentFn.NewValue("nullish", resultType)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{
		{Block: shortBB, Value: lhs},
		{Block: rhsEnd, Value: rhs},
	}})
	return res
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

func (g *generator) eventField(obj ir.Operand, name string, resultType types.Type) ir.Operand {
	offsets, _, _ := g.objectLayout(g.semaResult.EventType)
	res := g.currentFn.NewValue("event_"+strings.TrimPrefix(name, "$"), resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: name, Offset: offsets[name]})
	return res
}

func (g *generator) setEventField(obj ir.Operand, name string, value ir.Operand) {
	offsets, _, _ := g.objectLayout(g.semaResult.EventType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: obj, Field: name, Offset: offsets[name], Val: value})
}

func (g *generator) lowerWebIDLBoolean(value ir.Operand, sourceType types.Type) ir.Operand {
	if sourceType == types.TypeBoolean && value.Type() == types.TypeBoolean {
		return value
	}
	boxed := value
	if !irJSValueType(value.Type()) {
		boxed = g.boxJSValue(value, sourceType)
	}
	res := g.currentFn.NewValue("webidl_bool", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_js_to_bool", Args: []ir.Operand{boxed}, ParamTypes: []types.Type{types.TypeAny},
	})
	return res
}

func (g *generator) lowerWebIDLDictionaryMember(expr ast.Expr, value ir.Operand, name string, targetType types.Type, fallback ir.Operand) (ir.Operand, bool) {
	if expr == nil || value == nil {
		return fallback, false
	}
	convert := func(raw ir.Operand, sourceType types.Type) ir.Operand {
		if targetType == types.TypeBoolean {
			return g.lowerWebIDLBoolean(raw, sourceType)
		}
		if targetType == types.TypeAny {
			if irJSValueType(raw.Type()) {
				return raw
			}
			return g.boxJSValue(raw, sourceType)
		}
		return g.coerceJSValueBoundary(raw, sourceType, targetType)
	}
	if objType, ok := g.semanticType(expr).(*types.ObjectType); ok {
		field, exists := objType.Fields[name]
		if !exists {
			return fallback, false
		}
		offsets, _, _ := g.objectLayout(objType)
		raw := g.currentFn.NewValue("webidl_dict_"+name, field.Type)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: raw, Obj: value, Field: name, Offset: offsets[name]})
		return convert(raw, field.Type), true
	}
	if irJSValueType(value.Type()) {
		raw := g.lowerDynamicGet(value, name)
		return convert(raw, types.TypeAny), true
	}
	return fallback, false
}

func (g *generator) lowerWebIDLDictionaryBool(expr ast.Expr, value ir.Operand, name string, fallback bool) ir.Operand {
	fallbackValue := ir.Operand(ir.ConstBool{Value: fallback})
	result, _ := g.lowerWebIDLDictionaryMember(expr, value, name, types.TypeBoolean, fallbackValue)
	return result
}

func (g *generator) lowerEventInitBool(init ast.Expr, initValue ir.Operand, name string) ir.Operand {
	return g.lowerWebIDLDictionaryBool(init, initValue, name, false)
}

func (g *generator) lowerEventInitValue(init ast.Expr, initValue ir.Operand, name string, targetType types.Type, fallback ir.Operand) ir.Operand {
	result, _ := g.lowerWebIDLDictionaryMember(init, initValue, name, targetType, fallback)
	return result
}

func (g *generator) initEventVariantFields(obj ir.Operand, className string, init ast.Expr, initValue ir.Operand) {
	g.setEventField(obj, "$detail", ir.ConstNull{})
	g.setEventField(obj, "$data", ir.ConstNull{})
	g.setEventField(obj, "$origin", ir.ConstString{Value: ""})
	g.setEventField(obj, "$lastEventId", ir.ConstString{Value: ""})
	g.setEventField(obj, "$source", ir.ConstNull{})
	g.setEventField(obj, "$ports", ir.ConstNull{})
	g.setEventField(obj, "$message", ir.ConstString{Value: ""})
	g.setEventField(obj, "$filename", ir.ConstString{Value: ""})
	g.setEventField(obj, "$lineno", ir.ConstNumber{Value: 0})
	g.setEventField(obj, "$colno", ir.ConstNumber{Value: 0})
	g.setEventField(obj, "$error", ir.ConstNull{})
	switch className {
	case "CustomEvent":
		g.setEventField(obj, "$detail", g.lowerEventInitValue(init, initValue, "detail", types.TypeAny, ir.ConstNull{}))
	case "MessageEvent":
		g.setEventField(obj, "$data", g.lowerEventInitValue(init, initValue, "data", types.TypeAny, ir.ConstNull{}))
		g.setEventField(obj, "$origin", g.lowerEventInitValue(init, initValue, "origin", types.TypeString, ir.ConstString{Value: ""}))
		g.setEventField(obj, "$lastEventId", g.lowerEventInitValue(init, initValue, "lastEventId", types.TypeString, ir.ConstString{Value: ""}))
		g.setEventField(obj, "$source", g.lowerEventInitValue(init, initValue, "source", types.TypeAny, ir.ConstNull{}))
		g.setEventField(obj, "$ports", g.lowerEventInitValue(init, initValue, "ports", types.TypeAny, ir.ConstNull{}))
	case "ErrorEvent":
		g.setEventField(obj, "$message", g.lowerEventInitValue(init, initValue, "message", types.TypeString, ir.ConstString{Value: ""}))
		g.setEventField(obj, "$filename", g.lowerEventInitValue(init, initValue, "filename", types.TypeString, ir.ConstString{Value: ""}))
		g.setEventField(obj, "$lineno", g.lowerEventInitValue(init, initValue, "lineno", types.TypeNumber, ir.ConstNumber{Value: 0}))
		g.setEventField(obj, "$colno", g.lowerEventInitValue(init, initValue, "colno", types.TypeNumber, ir.ConstNumber{Value: 0}))
		g.setEventField(obj, "$error", g.lowerEventInitValue(init, initValue, "error", types.TypeAny, ir.ConstNull{}))
	}
}

func (g *generator) lowerEventConstructor(e *ast.NewExpr, resultType *types.ObjectType) ir.Operand {
	eventType := g.semaResult.EventType
	typeValue := g.lowerExpr(e.Args[0])
	var initExpr ast.Expr
	var initValue ir.Operand
	if len(e.Args) > 1 {
		initExpr = e.Args[1]
		initValue = g.lowerExpr(initExpr)
	}
	bubbles := g.lowerEventInitBool(initExpr, initValue, "bubbles")
	cancelable := g.lowerEventInitBool(initExpr, initValue, "cancelable")
	composed := g.lowerEventInitBool(initExpr, initValue, "composed")
	offsets, refMask, shape := g.objectLayout(eventType)
	obj := g.currentFn.NewValue("event", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: obj, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
	g.setEventField(obj, "type", typeValue)
	g.setEventField(obj, "bubbles", bubbles)
	g.setEventField(obj, "cancelable", cancelable)
	g.setEventField(obj, "composed", composed)
	g.setEventField(obj, "currentTarget", ir.ConstNull{})
	g.setEventField(obj, "target", ir.ConstNull{})
	g.setEventField(obj, "defaultPrevented", ir.ConstBool{Value: false})
	g.setEventField(obj, "eventPhase", ir.ConstNumber{Value: 0})
	g.setEventField(obj, "isTrusted", ir.ConstBool{Value: false})
	timestamp := g.currentFn.NewValue("event_timestamp", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: timestamp, Callee: "ts_performance_now"})
	g.setEventField(obj, "timeStamp", timestamp)
	g.setEventField(obj, "$dispatching", ir.ConstBool{Value: false})
	g.setEventField(obj, "$inPassiveListener", ir.ConstBool{Value: false})
	g.setEventField(obj, "$stopImmediate", ir.ConstBool{Value: false})
	g.setEventField(obj, "$stopPropagation", ir.ConstBool{Value: false})
	g.initEventVariantFields(obj, e.ClassName, initExpr, initValue)
	return obj
}

func (g *generator) nullRef(t types.Type) ir.Operand {
	res := g.currentFn.NewValue("null_ref", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_null_ref"})
	return res
}

func eventPhysicalProperty(objType *types.ObjectType, property string) (string, bool) {
	switch property {
	case "type", "target", "currentTarget", "bubbles", "cancelable", "defaultPrevented", "composed", "isTrusted", "eventPhase", "timeStamp":
		return property, true
	}
	switch objType.Name {
	case "$CustomEvent":
		if property == "detail" {
			return "$detail", true
		}
	case "$MessageEvent":
		switch property {
		case "data":
			return "$data", true
		case "origin":
			return "$origin", true
		case "lastEventId":
			return "$lastEventId", true
		case "source":
			return "$source", true
		case "ports":
			return "$ports", true
		}
	case "$ErrorEvent":
		switch property {
		case "message":
			return "$message", true
		case "filename":
			return "$filename", true
		case "lineno":
			return "$lineno", true
		case "colno":
			return "$colno", true
		case "error":
			return "$error", true
		}
	}
	return "", false
}

func isEventObjectType(t *types.ObjectType) bool {
	if t == nil {
		return false
	}
	switch t.Name {
	case "$Event", "$CustomEvent", "$MessageEvent", "$ErrorEvent":
		return true
	default:
		return false
	}
}

func (g *generator) lowerEventMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || (objType.Name != "$Event" && objType.Name != "$CustomEvent" && objType.Name != "$MessageEvent" && objType.Name != "$ErrorEvent") {
		return nil, false
	}
	event := g.lowerExpr(mem.Object)
	switch mem.Property {
	case "preventDefault":
		cancelable := g.eventField(event, "cancelable", types.TypeBoolean)
		passive := g.eventField(event, "$inPassiveListener", types.TypeBoolean)
		notPassive := g.currentFn.NewValue("event_not_passive", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: notPassive, Op: ir.OpEq, LHS: passive, RHS: ir.ConstBool{Value: false}})
		canPrevent := g.currentFn.NewValue("event_can_prevent", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: canPrevent, Op: ir.OpAnd, LHS: cancelable, RHS: notPassive})
		setBB := g.currentFn.NewBlock("event_prevent_default")
		doneBB := g.currentFn.NewBlock("event_prevent_default_done")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: canPrevent, Then: setBB, Else: doneBB}
		g.currentBB = setBB
		g.setEventField(event, "defaultPrevented", ir.ConstBool{Value: true})
		setBB.Terminator = &ir.JumpTerm{Target: doneBB}
		g.currentBB = doneBB
		return nil, true
	case "stopPropagation":
		g.setEventField(event, "$stopPropagation", ir.ConstBool{Value: true})
		return nil, true
	case "stopImmediatePropagation":
		g.setEventField(event, "$stopPropagation", ir.ConstBool{Value: true})
		g.setEventField(event, "$stopImmediate", ir.ConstBool{Value: true})
		return nil, true
	case "composedPath":
		arrType := types.NewArray(types.TypeAny)
		arr := g.currentFn.NewValue("event_path", arrType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{Res: arr, Length: ir.ConstNumber{Value: 0}, ElemType: types.TypeAny})
		target := g.eventField(event, "target", types.TypeAny)
		length := g.currentFn.NewValue("event_path_len", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayPushInst{Res: length, Array: arr, Val: target})
		return arr, true
	}
	return nil, false
}

func (g *generator) lowerEventListenerOptionBool(expr ast.Expr, value ir.Operand, name string) ir.Operand {
	if expr == nil {
		return ir.ConstBool{Value: false}
	}
	if g.semanticType(expr) == types.TypeBoolean {
		if name == "capture" {
			return value
		}
		return ir.ConstBool{Value: false}
	}
	return g.lowerWebIDLDictionaryBool(expr, value, name, false)
}

func (g *generator) lowerEventListenerSignal(expr ast.Expr, value ir.Operand) (ir.Operand, bool) {
	if expr == nil {
		return nil, false
	}
	objType, ok := g.semanticType(expr).(*types.ObjectType)
	if !ok {
		return nil, false
	}
	if _, exists := objType.Fields["signal"]; !exists {
		return nil, false
	}
	return g.lowerWebIDLDictionaryMember(expr, value, "signal", g.semaResult.AbortSignalType, ir.ConstNull{})
}

func (g *generator) makeEventListenerAbortRemovalCallback(target, eventType, callback, capture ir.Operand) ir.Operand {
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	fnType := types.NewFunction([]types.Param{{Name: "event", Type: g.semaResult.EventType}}, types.TypeVoid)
	name := fmt.Sprintf("$event_listener_abort%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, types.TypeVoid)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	capturedTarget := lifted.NewValue("target", target.Type())
	capturedType := lifted.NewValue("type", types.TypeString)
	capturedCallback := lifted.NewValue("callback", callback.Type())
	capturedCapture := lifted.NewValue("capture", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.ClosureGetInst{Res: capturedTarget, Closure: env, Index: 0},
		&ir.ClosureGetInst{Res: capturedType, Closure: env, Index: 1},
		&ir.ClosureGetInst{Res: capturedCallback, Closure: env, Index: 2},
		&ir.ClosureGetInst{Res: capturedCapture, Closure: env, Index: 3})
	ignoredEvent := lifted.NewValue("event", g.semaResult.EventType)
	lifted.Params = append(lifted.Params, ignoredEvent)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_event_target_remove", Args: []ir.Operand{capturedTarget, capturedType, capturedCallback, capturedCapture}})
	g.currentBB.Terminator = &ir.ReturnTerm{}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("event_listener_abort_callback", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: name, Captures: []ir.Operand{target, eventType, callback, capture}, RefMask: 0b111})
	return closure
}

func (g *generator) lowerEventTargetMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || (objType.Name != "$EventTarget" && objType.Name != "$AbortSignal") {
		return nil, false
	}
	target := g.lowerExpr(mem.Object)
	switch mem.Property {
	case "addEventListener":
		typeArg := g.lowerExpr(e.Args[0])
		callback := g.lowerExpr(e.Args[1])
		capture := ir.Operand(ir.ConstBool{Value: false})
		once := ir.Operand(ir.ConstBool{Value: false})
		passive := ir.Operand(ir.ConstBool{Value: false})
		var signal ir.Operand
		hasSignal := false
		if len(e.Args) > 2 {
			options := g.lowerExpr(e.Args[2])
			capture = g.lowerEventListenerOptionBool(e.Args[2], options, "capture")
			once = g.lowerEventListenerOptionBool(e.Args[2], options, "once")
			passive = g.lowerEventListenerOptionBool(e.Args[2], options, "passive")
			signal, hasSignal = g.lowerEventListenerSignal(e.Args[2], options)
		}
		if !hasSignal {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Callee: "ts_event_target_add", Args: []ir.Operand{target, typeArg, callback, once, capture, passive},
			})
			return nil, true
		}
		aborted := g.currentFn.NewValue("event_listener_signal_aborted", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: aborted, Callee: "ts_abort_signal_aborted", Args: []ir.Operand{signal}})
		addBB := g.currentFn.NewBlock("event_listener_signal_add")
		doneBB := g.currentFn.NewBlock("event_listener_signal_done")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: aborted, Then: doneBB, Else: addBB}
		g.currentBB = addBB
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Callee: "ts_event_target_add", Args: []ir.Operand{target, typeArg, callback, once, capture, passive},
		})
		removal := g.makeEventListenerAbortRemovalCallback(target, typeArg, callback, capture)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Callee: "ts_event_target_add", Args: []ir.Operand{signal, ir.ConstString{Value: "abort"}, removal, ir.ConstBool{Value: true}, ir.ConstBool{Value: false}, ir.ConstBool{Value: false}},
		})
		g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}
		g.currentBB = doneBB
		return nil, true
	case "removeEventListener":
		typeArg := g.lowerExpr(e.Args[0])
		callback := g.lowerExpr(e.Args[1])
		capture := ir.Operand(ir.ConstBool{Value: false})
		if len(e.Args) > 2 {
			options := g.lowerExpr(e.Args[2])
			capture = g.lowerEventListenerOptionBool(e.Args[2], options, "capture")
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Callee: "ts_event_target_remove", Args: []ir.Operand{target, typeArg, callback, capture},
		})
		return nil, true
	case "dispatchEvent":
		return g.lowerEventDispatch(target, g.lowerExpr(e.Args[0])), true
	}
	return nil, false
}

func (g *generator) lowerEventDispatch(target, event ir.Operand) ir.Operand {
	eventType := g.semaResult.EventType
	listenerType := types.NewObject("$EventListener")
	listenerFn := types.NewFunction([]types.Param{{Name: "event", Type: eventType}}, types.TypeVoid)
	g.setEventField(event, "target", g.boxJSValue(target, g.semaResult.EventTargetType))
	g.setEventField(event, "currentTarget", g.boxJSValue(target, g.semaResult.EventTargetType))
	g.setEventField(event, "eventPhase", ir.ConstNumber{Value: 2})
	g.setEventField(event, "$dispatching", ir.ConstBool{Value: true})
	g.setEventField(event, "$stopImmediate", ir.ConstBool{Value: false})
	g.setEventField(event, "$stopPropagation", ir.ConstBool{Value: false})
	boundary := g.currentFn.NewValue("event_listener_boundary", listenerType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: boundary, Callee: "ts_event_target_tail", Args: []ir.Operand{target}})

	entry := g.currentBB
	loopBB := g.currentFn.NewBlock("event_listener_loop")
	bodyBB := g.currentFn.NewBlock("event_listener_body")
	onceBB := g.currentFn.NewBlock("event_listener_once")
	invokeBB := g.currentFn.NewBlock("event_listener_invoke")
	doneBB := g.currentFn.NewBlock("event_dispatch_done")
	entry.Terminator = &ir.JumpTerm{Target: loopBB}

	prev := g.currentFn.NewValue("event_prev_listener", listenerType)
	loopBB.Phis = append(loopBB.Phis, &ir.PhiInst{Res: prev, Incoming: []ir.PhiIncoming{{Block: entry, Value: g.nullRefAt(entry, listenerType)}}})
	eventName := g.currentFn.NewValue("event_type_for_listener", types.TypeString)
	eventOffsets, _, _ := g.objectLayout(eventType)
	loopBB.Instructions = append(loopBB.Instructions, &ir.GetFieldInst{Res: eventName, Obj: event, Field: "type", Offset: eventOffsets["type"]})
	next := g.currentFn.NewValue("event_listener", listenerType)
	loopBB.Instructions = append(loopBB.Instructions, &ir.CallInst{Res: next, Callee: "ts_event_target_next", Args: []ir.Operand{target, eventName, prev}})
	loopBB.Terminator = &ir.BranchTerm{Cond: next, Then: bodyBB, Else: doneBB}

	once := g.currentFn.NewValue("event_listener_once", types.TypeBoolean)
	passive := g.currentFn.NewValue("event_listener_passive", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.CallInst{Res: once, Callee: "ts_event_listener_once", Args: []ir.Operand{next}},
		&ir.CallInst{Res: passive, Callee: "ts_event_listener_passive", Args: []ir.Operand{next}})
	bodyBB.Terminator = &ir.BranchTerm{Cond: once, Then: onceBB, Else: invokeBB}
	onceBB.Instructions = append(onceBB.Instructions, &ir.CallInst{Callee: "ts_event_listener_remove", Args: []ir.Operand{next}})
	onceBB.Terminator = &ir.JumpTerm{Target: invokeBB}

	callback := g.currentFn.NewValue("event_callback", listenerFn)
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.CallInst{Res: callback, Callee: "ts_event_listener_callback", Args: []ir.Operand{next}})
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.SetFieldInst{Obj: event, Field: "$inPassiveListener", Offset: eventOffsets["$inPassiveListener"], Val: passive})
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.IndirectCallInst{Closure: callback, Args: []ir.Operand{event}, ParamTypes: []types.Type{eventType}})
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.SetFieldInst{Obj: event, Field: "$inPassiveListener", Offset: eventOffsets["$inPassiveListener"], Val: ir.ConstBool{Value: false}})
	stop := g.currentFn.NewValue("event_stop_immediate", types.TypeBoolean)
	offsets, _, _ := g.objectLayout(eventType)
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.GetFieldInst{Res: stop, Obj: event, Field: "$stopImmediate", Offset: offsets["$stopImmediate"]})
	atBoundary := g.currentFn.NewValue("event_at_boundary", types.TypeBoolean)
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.BinaryInst{Res: atBoundary, Op: ir.OpEq, LHS: next, RHS: boundary})
	finish := g.currentFn.NewValue("event_finish_dispatch", types.TypeBoolean)
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.BinaryInst{Res: finish, Op: ir.OpOr, LHS: stop, RHS: atBoundary})
	invokeBB.Terminator = &ir.BranchTerm{Cond: finish, Then: doneBB, Else: loopBB}
	loopBB.Phis[0].Incoming = append(loopBB.Phis[0].Incoming, ir.PhiIncoming{Block: invokeBB, Value: next})

	g.currentBB = doneBB
	g.setEventField(event, "currentTarget", ir.ConstNull{})
	g.setEventField(event, "eventPhase", ir.ConstNumber{Value: 0})
	g.setEventField(event, "$dispatching", ir.ConstBool{Value: false})
	g.setEventField(event, "$inPassiveListener", ir.ConstBool{Value: false})
	defaultPrevented := g.eventField(event, "defaultPrevented", types.TypeBoolean)
	result := g.currentFn.NewValue("event_dispatch_result", types.TypeBoolean)
	doneBB.Instructions = append(doneBB.Instructions, &ir.BinaryInst{Res: result, Op: ir.OpEq, LHS: defaultPrevented, RHS: ir.ConstBool{Value: false}})
	return result
}

func (g *generator) newAbortReason(name, message string) ir.Operand {
	err := g.newDOMException(ir.ConstString{Value: message}, ir.ConstString{Value: name})
	return g.boxJSValue(err, g.semaResult.DOMExceptionType)
}

func (g *generator) makeAbortTimeoutCallback(signal ir.Operand) ir.Operand {
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	fnType := types.NewFunction(nil, types.TypeVoid)
	name := fmt.Sprintf("$abort_timeout%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, types.TypeVoid)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	captured := lifted.NewValue("signal", g.semaResult.AbortSignalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: captured, Closure: env, Index: 0})
	reason := g.newAbortReason("TimeoutError", "The operation timed out")
	first := lifted.NewValue("timeout_abort_first", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: first, Callee: "ts_abort_signal_set_reason", Args: []ir.Operand{captured, reason}, ParamTypes: []types.Type{g.semaResult.AbortSignalType, types.TypeAny}})
	fireBB := lifted.NewBlock("timeout_abort_fire")
	doneBB := lifted.NewBlock("timeout_abort_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: first, Then: fireBB, Else: doneBB}
	g.currentBB = fireBB
	event := g.lowerSimpleEvent("abort")
	g.lowerEventDispatch(captured, event)
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}
	}
	g.currentBB = doneBB
	g.currentBB.Terminator = &ir.ReturnTerm{}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("abort_timeout_callback", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: name, Captures: []ir.Operand{signal}, RefMask: 1})
	return closure
}

func (g *generator) lowerArrayBufferMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$ArrayBuffer" || mem.Property != "slice" {
		return nil, false
	}
	obj := g.lowerExpr(mem.Object)
	data := g.arrayBufferData(obj)
	begin := g.lowerExpr(e.Args[0])
	end := ir.Operand(nil)
	if len(e.Args) > 1 {
		end = g.lowerExpr(e.Args[1])
	} else {
		length := g.currentFn.NewValue("array_buffer_len", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
		end = length
	}
	sliced := g.currentFn.NewValue("array_buffer_slice_data", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: sliced, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{data, begin, end}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
	return g.newArrayBufferFromData(sliced), true
}

func (g *generator) lowerUint8ArrayMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$Uint8Array" || (mem.Property != "slice" && mem.Property != "subarray") {
		return nil, false
	}
	obj := g.lowerExpr(mem.Object)
	data := g.uint8ArrayField(obj, "$data", g.semaResult.ByteBufferType)
	buffer := g.uint8ArrayField(obj, "buffer", g.semaResult.ArrayBufferType)
	base := g.uint8ArrayField(obj, "byteOffset", types.TypeNumber)
	length := g.uint8ArrayField(obj, "length", types.TypeNumber)
	begin := g.lowerExpr(e.Args[0])
	end := ir.Operand(length)
	if len(e.Args) > 1 {
		end = g.lowerExpr(e.Args[1])
	}
	startAbs := g.currentFn.NewValue("uint8_slice_start", types.TypeNumber)
	endAbs := g.currentFn.NewValue("uint8_slice_end", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: startAbs, Op: ir.OpAdd, LHS: base, RHS: begin},
		&ir.BinaryInst{Res: endAbs, Op: ir.OpAdd, LHS: base, RHS: end},
	)
	newLen := g.currentFn.NewValue("uint8_slice_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: newLen, Op: ir.OpSub, LHS: end, RHS: begin})
	if mem.Property == "subarray" {
		return g.newUint8ArrayView(data, buffer, startAbs, newLen), true
	}
	sliced := g.currentFn.NewValue("uint8_slice_data", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: sliced, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{data, startAbs, endAbs}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
	newBuffer := g.newArrayBufferFromData(sliced)
	return g.newUint8ArrayView(sliced, newBuffer, ir.ConstNumber{Value: 0}, newLen), true
}

func (g *generator) lowerAbortSignalStaticCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	ident, ok := mem.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "AbortSignal" {
		return nil, false
	}
	signal := g.currentFn.NewValue("abort_signal", g.semaResult.AbortSignalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: signal, Callee: "ts_abort_signal_new"})
	switch mem.Property {
	case "abort":
		var reason ir.Operand
		if len(e.Args) > 0 {
			reason = g.lowerExpr(e.Args[0])
			if !irJSValueType(reason.Type()) {
				reason = g.boxJSValue(reason, g.semanticType(e.Args[0]))
			}
		} else {
			reason = g.newAbortReason("AbortError", "This operation was aborted")
		}
		set := g.currentFn.NewValue("abort_signal_static_set", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: set, Callee: "ts_abort_signal_set_reason", Args: []ir.Operand{signal, reason}, ParamTypes: []types.Type{g.semaResult.AbortSignalType, types.TypeAny}})
		return signal, true
	case "timeout":
		delay := g.lowerExpr(e.Args[0])
		callback := g.makeAbortTimeoutCallback(signal)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_set_timeout", Args: []ir.Operand{callback, delay}, ParamTypes: []types.Type{callback.Type(), types.TypeNumber}})
		return signal, true
	case "any":
		return g.lowerAbortSignalAny(e, signal), true
	}
	return nil, false
}

func (g *generator) makeAbortDependencyCallback(source, result ir.Operand) ir.Operand {
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	fnType := types.NewFunction([]types.Param{{Name: "event", Type: g.semaResult.EventType}}, types.TypeVoid)
	name := fmt.Sprintf("$abort_dependency%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, types.TypeVoid)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	capturedSource := lifted.NewValue("source", g.semaResult.AbortSignalType)
	capturedResult := lifted.NewValue("result", g.semaResult.AbortSignalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.ClosureGetInst{Res: capturedSource, Closure: env, Index: 0},
		&ir.ClosureGetInst{Res: capturedResult, Closure: env, Index: 1})
	eventParam := lifted.NewValue("event", g.semaResult.EventType)
	lifted.Params = append(lifted.Params, eventParam)
	already := lifted.NewValue("dependent_already_aborted", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: already, Callee: "ts_abort_signal_aborted", Args: []ir.Operand{capturedResult}})
	workBB := lifted.NewBlock("dependent_abort_work")
	doneBB := lifted.NewBlock("dependent_abort_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: already, Then: doneBB, Else: workBB}
	g.currentBB = workBB
	reason := lifted.NewValue("dependent_reason", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: reason, Callee: "ts_abort_signal_reason", Args: []ir.Operand{capturedSource}})
	first := lifted.NewValue("dependent_first", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: first, Callee: "ts_abort_signal_set_reason", Args: []ir.Operand{capturedResult, reason}, ParamTypes: []types.Type{g.semaResult.AbortSignalType, types.TypeAny}})
	fireBB := lifted.NewBlock("dependent_fire")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: first, Then: fireBB, Else: doneBB}
	g.currentBB = fireBB
	event := g.lowerSimpleEvent("abort")
	g.lowerEventDispatch(capturedResult, event)
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}
	}
	g.currentBB = doneBB
	g.currentBB.Terminator = &ir.ReturnTerm{}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("abort_dependency_callback", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: name, Captures: []ir.Operand{source, result}, RefMask: 3})
	return closure
}

func (g *generator) lowerAbortSignalAny(e *ast.CallExpr, result ir.Operand) ir.Operand {
	signals := g.lowerExpr(e.Args[0])
	arrType, ok := g.semanticType(e.Args[0]).(*types.ArrayType)
	if !ok {
		return g.failExpr("AbortSignal.any expects an AbortSignal array")
	}
	pre := g.currentBB
	condBB := g.currentFn.NewBlock("abort_any_cond")
	bodyBB := g.currentFn.NewBlock("abort_any_body")
	alreadyBB := g.currentFn.NewBlock("abort_any_already")
	listenBB := g.currentFn.NewBlock("abort_any_listen")
	postBB := g.currentFn.NewBlock("abort_any_post")
	doneBB := g.currentFn.NewBlock("abort_any_done")
	pre.Terminator = &ir.JumpTerm{Target: condBB}
	index := g.currentFn.NewValue("abort_any_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("abort_any_next", types.TypeNumber)
	phi := &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: pre, Value: ir.ConstNumber{Value: 0}}, {Block: postBB, Value: nextIndex}}}
	condBB.Phis = append(condBB.Phis, phi)
	length := g.currentFn.NewValue("abort_any_len", types.TypeNumber)
	condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: signals})
	more := g.currentFn.NewValue("abort_any_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	source := g.currentFn.NewValue("abort_any_source", arrType.Elem)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: source, Array: signals, Index: index})
	aborted := g.currentFn.NewValue("abort_any_source_aborted", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{Res: aborted, Callee: "ts_abort_signal_aborted", Args: []ir.Operand{source}})
	bodyBB.Terminator = &ir.BranchTerm{Cond: aborted, Then: alreadyBB, Else: listenBB}

	g.currentBB = alreadyBB
	reason := g.currentFn.NewValue("abort_any_reason", types.TypeAny)
	alreadyBB.Instructions = append(alreadyBB.Instructions, &ir.CallInst{Res: reason, Callee: "ts_abort_signal_reason", Args: []ir.Operand{source}})
	set := g.currentFn.NewValue("abort_any_set", types.TypeBoolean)
	alreadyBB.Instructions = append(alreadyBB.Instructions, &ir.CallInst{Res: set, Callee: "ts_abort_signal_set_reason", Args: []ir.Operand{result, reason}, ParamTypes: []types.Type{g.semaResult.AbortSignalType, types.TypeAny}})
	alreadyBB.Terminator = &ir.JumpTerm{Target: doneBB}

	g.currentBB = listenBB
	callback := g.makeAbortDependencyCallback(source, result)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_event_target_add", Args: []ir.Operand{source, ir.ConstString{Value: "abort"}, callback, ir.ConstBool{Value: false}, ir.ConstBool{Value: false}, ir.ConstBool{Value: false}}})
	listenBB.Terminator = &ir.JumpTerm{Target: postBB}

	postBB.Instructions = append(postBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	postBB.Terminator = &ir.JumpTerm{Target: condBB}
	g.currentBB = doneBB
	return result
}

func (g *generator) lowerAbortControllerNew() ir.Operand {
	signalType := g.semaResult.AbortSignalType
	signal := g.currentFn.NewValue("abort_signal", signalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: signal, Callee: "ts_abort_signal_new"})
	controllerType := g.semaResult.AbortControllerType
	offsets, refMask, shape := g.objectLayout(controllerType)
	controller := g.currentFn.NewValue("abort_controller", controllerType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: controller, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: controller, Field: "signal", Offset: offsets["signal"], Val: signal})
	return controller
}

func (g *generator) lowerSimpleEvent(typeName string) ir.Operand {
	eventType := g.semaResult.EventType
	offsets, refMask, shape := g.objectLayout(eventType)
	event := g.currentFn.NewValue("event", eventType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: event, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
	g.setEventField(event, "type", ir.ConstString{Value: typeName})
	g.setEventField(event, "bubbles", ir.ConstBool{Value: false})
	g.setEventField(event, "cancelable", ir.ConstBool{Value: false})
	g.setEventField(event, "composed", ir.ConstBool{Value: false})
	g.setEventField(event, "currentTarget", ir.ConstNull{})
	g.setEventField(event, "target", ir.ConstNull{})
	g.setEventField(event, "defaultPrevented", ir.ConstBool{Value: false})
	g.setEventField(event, "eventPhase", ir.ConstNumber{Value: 0})
	g.setEventField(event, "isTrusted", ir.ConstBool{Value: false})
	timestamp := g.currentFn.NewValue("event_timestamp", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: timestamp, Callee: "ts_performance_now"})
	g.setEventField(event, "timeStamp", timestamp)
	g.setEventField(event, "$dispatching", ir.ConstBool{Value: false})
	g.setEventField(event, "$stopImmediate", ir.ConstBool{Value: false})
	g.setEventField(event, "$stopPropagation", ir.ConstBool{Value: false})
	g.initEventVariantFields(event, "Event", nil, nil)
	return event
}

func (g *generator) lowerAbortControllerMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$AbortController" || mem.Property != "abort" {
		return nil, false
	}
	controller := g.lowerExpr(mem.Object)
	offsets, _, _ := g.objectLayout(g.semaResult.AbortControllerType)
	signal := g.currentFn.NewValue("abort_signal", g.semaResult.AbortSignalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: signal, Obj: controller, Field: "signal", Offset: offsets["signal"]})
	var reason ir.Operand
	if len(e.Args) > 0 {
		reason = g.lowerExpr(e.Args[0])
		if !irJSValueType(reason.Type()) {
			reason = g.boxJSValue(reason, g.semanticType(e.Args[0]))
		}
	} else {
		err := g.newDOMException(ir.ConstString{Value: "This operation was aborted"}, ir.ConstString{Value: "AbortError"})
		reason = g.boxJSValue(err, g.semaResult.DOMExceptionType)
	}
	first := g.currentFn.NewValue("abort_first", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: first, Callee: "ts_abort_signal_set_reason", Args: []ir.Operand{signal, reason}, ParamTypes: []types.Type{g.semaResult.AbortSignalType, types.TypeAny}})
	fireBB := g.currentFn.NewBlock("abort_fire")
	doneBB := g.currentFn.NewBlock("abort_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: first, Then: fireBB, Else: doneBB}
	g.currentBB = fireBB
	event := g.lowerSimpleEvent("abort")
	g.lowerEventDispatch(signal, event)
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}
	}
	g.currentBB = doneBB
	return nil, true
}

func (g *generator) lowerAbortSignalMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$AbortSignal" || mem.Property != "throwIfAborted" {
		return nil, false
	}
	signal := g.lowerExpr(mem.Object)
	aborted := g.currentFn.NewValue("abort_signal_aborted", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: aborted, Callee: "ts_abort_signal_aborted", Args: []ir.Operand{signal}})
	throwBB := g.currentFn.NewBlock("abort_throw")
	doneBB := g.currentFn.NewBlock("abort_throw_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: aborted, Then: throwBB, Else: doneBB}
	g.currentBB = throwBB
	reason := g.currentFn.NewValue("abort_reason", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: reason, Callee: "ts_abort_signal_reason", Args: []ir.Operand{signal}})
	g.routeThrownValue(reason)
	g.currentBB = doneBB
	return nil, true
}

func (g *generator) nullRefAt(bb *ir.BasicBlock, t types.Type) ir.Operand {
	res := g.currentFn.NewValue("null_ref", t)
	bb.Instructions = append(bb.Instructions, &ir.CallInst{Res: res, Callee: "ts_null_ref"})
	return res
}

func (g *generator) newArrayBufferFromData(data ir.Operand) ir.Operand {
	t := g.semaResult.ArrayBufferType
	obj := g.currentFn.NewValue("array_buffer", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: obj, Callee: "ts_array_buffer_wrap", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
	return obj
}

func (g *generator) arrayBufferData(obj ir.Operand) ir.Operand {
	t := g.semaResult.ArrayBufferType
	offsets, _, _ := g.objectLayout(t)
	data := g.currentFn.NewValue("array_buffer_data", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: data, Obj: obj, Field: "$data", Offset: offsets["$data"]})
	return data
}

func (g *generator) newUint8ArrayView(data, buffer, offset, length ir.Operand) ir.Operand {
	t := g.semaResult.Uint8ArrayType
	obj := g.currentFn.NewValue("uint8_array", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: obj, Callee: "ts_uint8_array_wrap", Args: []ir.Operand{data, buffer, offset, length}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, g.semaResult.ArrayBufferType, types.TypeNumber, types.TypeNumber}})
	return obj
}

func (g *generator) uint8ArrayField(obj ir.Operand, name string, typ types.Type) ir.Operand {
	offsets, _, _ := g.objectLayout(g.semaResult.Uint8ArrayType)
	res := g.currentFn.NewValue("uint8_"+strings.TrimPrefix(name, "$"), typ)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: name, Offset: offsets[name]})
	return res
}

func (g *generator) lowerExpr(expr ast.Expr) ir.Operand {

	switch e := expr.(type) {
	case *ast.NumberLit:
		return ir.ConstNumber{Value: e.Value}
	case *ast.StringLit:
		return ir.ConstString{Value: e.Value}
	case *ast.RegexLit:
		return g.lowerNativeRegExp(e.Pattern, e.Flags, g.semanticType(e))
	case *ast.BoolLit:
		return ir.ConstBool{Value: e.Value}
	case *ast.NullLit:
		return ir.ConstNull{}
	case *ast.UndefinedLit:
		return ir.ConstUndefined{}
	case *ast.ThisExpr:
		return g.locals["$this"]

	case *ast.NewExpr:
		if e.ClassName == "ArrayBuffer" {
			length := g.lowerExpr(e.Args[0])
			data := g.currentFn.NewValue("array_buffer_data", g.semaResult.ByteBufferType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: data, Callee: "ts_byte_buffer_new", Args: []ir.Operand{length}, ParamTypes: []types.Type{types.TypeNumber}})
			return g.newArrayBufferFromData(data)
		}
		if e.ClassName == "Uint8Array" {
			argType := g.semanticType(e.Args[0])
			if obj, ok := argType.(*types.ObjectType); ok && obj.Name == "$ArrayBuffer" {
				buffer := g.lowerExpr(e.Args[0])
				data := g.arrayBufferData(buffer)
				length := g.currentFn.NewValue("uint8_len", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
				return g.newUint8ArrayView(data, buffer, ir.ConstNumber{Value: 0}, length)
			}
			length := g.lowerExpr(e.Args[0])
			data := g.currentFn.NewValue("uint8_data", g.semaResult.ByteBufferType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: data, Callee: "ts_byte_buffer_new", Args: []ir.Operand{length}, ParamTypes: []types.Type{types.TypeNumber}})
			buffer := g.newArrayBufferFromData(data)
			return g.newUint8ArrayView(data, buffer, ir.ConstNumber{Value: 0}, length)
		}
		if e.ClassName == "AbortController" {
			return g.lowerAbortControllerNew()
		}
		if e.ClassName == "EventTarget" {
			t := g.semaResult.EventTargetType
			res := g.currentFn.NewValue("event_target", t)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_event_target_new"})
			return res
		}
		if e.ClassName == "Event" {
			return g.lowerEventConstructor(e, g.semaResult.EventType)
		}
		if e.ClassName == "CustomEvent" {
			return g.lowerEventConstructor(e, g.semaResult.CustomEventType)
		}
		if e.ClassName == "MessageEvent" {
			return g.lowerEventConstructor(e, g.semaResult.MessageEventType)
		}
		if e.ClassName == "ErrorEvent" {
			return g.lowerEventConstructor(e, g.semaResult.ErrorEventType)
		}
		if e.ClassName == "DOMException" {
			message := ir.Operand(ir.ConstString{Value: ""})
			name := ir.Operand(ir.ConstString{Value: "Error"})
			if len(e.Args) > 0 {
				message = g.lowerExpr(e.Args[0])
			}
			if len(e.Args) > 1 {
				name = g.lowerExpr(e.Args[1])
			}
			return g.newDOMException(message, name)
		}
		if e.ClassName == "RegExp" {
			pattern := e.Args[0].(*ast.StringLit)
			flags := ""
			if len(e.Args) == 2 {
				flags = e.Args[1].(*ast.StringLit).Value
			}
			return g.lowerNativeRegExp(pattern.Value, flags, g.semanticType(e))
		}
		if e.ClassName == "Date" {
			arg := e.Args[0]
			value := g.lowerExpr(arg)
			if lit, ok := arg.(*ast.StringLit); ok {
				parsed, _ := time.Parse(time.RFC3339Nano, lit.Value)
				value = ir.ConstNumber{Value: float64(parsed.UnixMilli())}
			}
			res := g.currentFn.NewValue("date", g.semanticType(e))
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_date_from_number", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeNumber}})
			return res
		}
		if collection := g.builtinCollectionInfo(g.semanticType(e)); collection != nil {
			res := g.currentFn.NewValue(strings.ToLower(collection.Kind), collection.Instance)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_new"})
			return res
		}
		info := g.semaResult.GenericClasses[e]
		if info == nil {
			info = g.semaResult.Classes[e.ClassName]
		}
		g.ensureClassSpecialization(info)
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

	case *ast.ArrayLit:
		arrType := types.NewArray(types.TypeAny)
		if semantic := g.semanticType(e); semantic != nil {
			if tuple, ok := semantic.(*types.TupleType); ok {
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
				sourceType := g.semanticType(spread.Value).(*types.ArrayType)
				source := g.lowerExpr(spread.Value)
				g.appendSpreadArray(res, source, sourceType.Elem, arrType.Elem)
				continue
			}
			val := g.lowerExpr(el)
			stored := val
			if irJSValueType(arrType.Elem) {
				stored = g.boxJSValue(val, g.semanticType(el))
			}
			if hasSpread {
				g.pushArrayOperand(res, stored)
			} else {
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{
					Array: res, Index: ir.ConstNumber{Value: float64(i)}, Val: stored,
				})
			}
		}
		return res
	case *ast.ObjectLit:
		objType := g.semanticType(e).(*types.ObjectType)
		offsets, refMask, shape := g.objectLayout(objType)
		res := g.currentFn.NewValue("obj", objType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
		written := make(map[string]bool, len(objType.Fields))
		for _, prop := range e.Properties {
			if prop.Spread {
				sourceType := g.semanticType(prop.Value).(*types.ObjectType)
				source := g.lowerExpr(prop.Value)
				sourceOffsets, _, _ := g.objectLayout(sourceType)
				for _, name := range sourceType.FieldOrder {
					field := sourceType.Fields[name]
					value := g.currentFn.NewValue("spread_field", field.Type)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: value, Obj: source, Field: name, Offset: sourceOffsets[name]})
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: name, Offset: offsets[name], Val: value})
					written[name] = true
				}
				continue
			}
			val := g.lowerExpr(prop.Value)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: prop.Key, Offset: offsets[prop.Key], Val: val})
			written[prop.Key] = true
		}
		for _, name := range objType.FieldOrder {
			field := objType.Fields[name]
			if written[name] || !field.Optional {
				continue
			}
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: name, Offset: offsets[name], Val: ir.ConstUndefined{}})
		}
		return res
	case *ast.IdentExpr:
		if e.Name == "globalThis" || e.Name == "self" {
			t := g.semanticType(e)
			res := g.currentFn.NewValue("global_scope", t)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_global_object"})
			return res
		}
		if _, exists := g.locals[e.Name]; exists {
			return g.readLocal(e.Name)
		}
		sym := g.semaResult.Symbols[e]
		fnType := sym.Type.(*types.FunctionType)
		res := g.currentFn.NewValue("closure", fnType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: res, Function: e.Name})
		return res
	case *ast.BinaryExpr:
		if e.Op == token.AmpAmp || e.Op == token.PipePipe {
			return g.lowerLogicalExpr(e)
		}
		if e.Op == token.QuestionQuestion {
			return g.lowerNullishExpr(e)
		}
		if g.canFuseStringConcat(e) {
			if fused, ok := g.lowerStringConcatChain(e); ok {
				return fused
			}
		}

		lhs := g.lowerExpr(e.Left)
		rhs := g.lowerExpr(e.Right)

		if e.Op == token.Plus && (irJSValueType(g.semanticType(e.Left)) || irJSValueType(g.semanticType(e.Right))) {
			if !irJSValueType(lhs.Type()) {
				lhs = g.boxJSValue(lhs, lhs.Type())
			}
			if !irJSValueType(rhs.Type()) {
				rhs = g.boxJSValue(rhs, rhs.Type())
			}
			resVal := g.currentFn.NewValue("js_add", types.TypeAny)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: resVal, Callee: "ts_js_add", Args: []ir.Operand{lhs, rhs}, ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
			})
			return resVal
		}

		if (e.Op == token.Minus || e.Op == token.Star || e.Op == token.Slash || e.Op == token.Percent) &&
			(irJSValueType(g.semanticType(e.Left)) || irJSValueType(g.semanticType(e.Right))) {
			if !irJSValueType(lhs.Type()) {
				lhs = g.boxJSValue(lhs, lhs.Type())
			}
			if !irJSValueType(rhs.Type()) {
				rhs = g.boxJSValue(rhs, rhs.Type())
			}
			callee := "ts_js_sub"
			switch e.Op {
			case token.Star:
				callee = "ts_js_mul"
			case token.Slash:
				callee = "ts_js_div"
			case token.Percent:
				callee = "ts_js_mod"
			}
			resVal := g.currentFn.NewValue("js_num_op", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: resVal, Callee: callee, Args: []ir.Operand{lhs, rhs}, ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
			})
			return resVal
		}

		if (e.Op == token.EqEq || e.Op == token.EqEqEq || e.Op == token.BangEq || e.Op == token.BangEqEq) &&
			(irJSValueType(g.semanticType(e.Left)) || irJSValueType(g.semanticType(e.Right))) {
			if !irJSValueType(lhs.Type()) {
				lhs = g.boxJSValue(lhs, lhs.Type())
			}
			if !irJSValueType(rhs.Type()) {
				rhs = g.boxJSValue(rhs, rhs.Type())
			}
			callee := "ts_js_loose_eq"
			if e.Op == token.EqEqEq || e.Op == token.BangEqEq {
				callee = "ts_js_strict_eq"
			}
			eq := g.currentFn.NewValue("js_eq", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: eq, Callee: callee, Args: []ir.Operand{lhs, rhs}, ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
			})
			if e.Op == token.BangEq || e.Op == token.BangEqEq {
				resVal := g.currentFn.NewValue("js_ne", types.TypeBoolean)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: resVal, Op: ir.OpEq, LHS: eq, RHS: ir.ConstBool{Value: false}})
				return resVal
			}
			return eq
		}

		if (e.Op == token.Lt || e.Op == token.LtEq || e.Op == token.Gt || e.Op == token.GtEq) &&
			(irJSValueType(g.semanticType(e.Left)) || irJSValueType(g.semanticType(e.Right))) {
			if !irJSValueType(lhs.Type()) {
				lhs = g.boxJSValue(lhs, lhs.Type())
			}
			if !irJSValueType(rhs.Type()) {
				rhs = g.boxJSValue(rhs, rhs.Type())
			}
			callee := "ts_js_lt"
			switch e.Op {
			case token.LtEq:
				callee = "ts_js_le"
			case token.Gt:
				callee = "ts_js_gt"
			case token.GtEq:
				callee = "ts_js_ge"
			}
			resVal := g.currentFn.NewValue("js_rel", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: resVal, Callee: callee, Args: []ir.Operand{lhs, rhs}, ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
			})
			return resVal
		}

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
		case token.Pipe:
			op = ir.OpOr
		case token.Amp:
			op = ir.OpAnd
		default:
			op = ir.OpAnd
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
	case *ast.FunctionExpr:
		return g.lowerFunctionExpr(e)
	case *ast.AwaitExpr:
		task := g.lowerExpr(e.Target)
		resultType := g.semanticType(e)
		var res ir.Operand
		if resultType == nil || resultType.Kind() == types.KindVoid {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_join", Args: []ir.Operand{task}})
		} else {
			v := g.currentFn.NewValue("await_result", resultType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: v, Callee: "ts_task_join", Args: []ir.Operand{task}})
			res = v
		}
		rejected := g.currentFn.NewValue("await_rejected", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: rejected, Callee: "ts_task_rejected", Args: []ir.Operand{task}})
		okBB := g.currentFn.NewBlock("await_ok")
		rejectBB := g.currentFn.NewBlock("await_reject")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: rejected, Then: rejectBB, Else: okBB}
		g.currentBB = rejectBB
		errVal := g.currentFn.NewValue("await_error", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: errVal, Callee: "ts_task_error", Args: []ir.Operand{task}})
		g.routeThrownValue(errVal)
		g.currentBB = okBB
		return res
	case *ast.UnaryExpr:
		if e.Op == token.PlusPlus || e.Op == token.MinusMinus {
			if ident, ok := e.Target.(*ast.IdentExpr); ok {
				currVal := g.readLocal(ident.Name)
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
				if !g.writeCapturedLocal(ident.Name, nextVal) {
					g.locals[ident.Name] = nextVal
				}
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
		}
		return g.lowerExpr(e.Target)
	case *ast.IndexExpr:
		if obj, ok := g.semanticType(e.Target).(*types.ObjectType); ok && obj.Name == "$Uint8Array" {
			target := g.lowerExpr(e.Target)
			index := g.lowerExpr(e.Index)
			data := g.uint8ArrayField(target, "$data", g.semaResult.ByteBufferType)
			offset := g.uint8ArrayField(target, "byteOffset", types.TypeNumber)
			actual := g.currentFn.NewValue("uint8_index", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: actual, Op: ir.OpAdd, LHS: offset, RHS: index})
			res := g.currentFn.NewValue("uint8_value", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_byte_buffer_get", Args: []ir.Operand{data, actual}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber}})
			return res
		}
		if key, ok := g.staticStringKey(e.Index); ok {
			target := g.lowerExpr(e.Target)
			if object, ok := target.Type().(*types.ObjectType); ok {
				offsets, _, _ := g.objectLayout(object)
				offset := offsets[key]
				resultType := object.Fields[key].Type
				if semantic := g.semanticType(e); semantic != nil && semantic != types.TypeAny {
					resultType = semantic
				}
				res := g.currentFn.NewValue("computed_field", resultType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: target, Field: key, Offset: offset})
				return res
			}
			if target.Type() == types.TypeAny {
				if concrete, ok := g.provenObjectType(e.Target); ok {
					offsets, _, _ := g.objectLayout(concrete)
					if offset, exists := offsets[key]; exists {
						field := concrete.Fields[key]
						raw := g.unboxKnownObject(target, concrete)
						res := g.currentFn.NewValue("computed_any_field", field.Type)
						g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: raw, Field: key, Offset: offset})
						return res
					}
					return ir.ConstUndefined{}
				}
				return g.lowerDynamicGet(target, key)
			}
		}
		if tuple, ok := g.semanticType(e.Target).(*types.TupleType); ok {
			lit := e.Index.(*ast.NumberLit)
			idx := int(lit.Value)
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
		if e.Optional {
			return g.lowerOptionalMember(e)
		}
		if ident, ok := e.Object.(*ast.IdentExpr); ok {
			if ident.Name == "performance" && e.Property == "timeOrigin" {
				res := g.currentFn.NewValue("performance_time_origin", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_performance_time_origin"})
				return res
			}
			if ident.Name == "navigator" && e.Property == "userAgent" {
				return ir.ConstString{Value: "ts-pro"}
			}
			if members := g.semaResult.Enums[ident.Name]; members != nil {
				return ir.ConstNumber{Value: members[e.Property]}
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
			if objType.Name == "$ArrayBuffer" && e.Property == "byteLength" {
				obj := g.lowerExpr(e.Object)
				data := g.arrayBufferData(obj)
				res := g.currentFn.NewValue("array_buffer_len", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
				return res
			}
			if objType.Name == "$AbortSignal" {
				signal := g.lowerExpr(e.Object)
				switch e.Property {
				case "aborted":
					res := g.currentFn.NewValue("abort_signal_aborted", types.TypeBoolean)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_abort_signal_aborted", Args: []ir.Operand{signal}})
					return res
				case "reason":
					res := g.currentFn.NewValue("abort_signal_reason", types.TypeAny)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_abort_signal_reason", Args: []ir.Operand{signal}})
					return res
				case "onabort":
					fnType := types.NewFunction([]types.Param{{Name: "event", Type: g.semaResult.EventType}}, types.TypeVoid)
					raw := g.currentFn.NewValue("abort_signal_onabort", fnType)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: raw, Callee: "ts_abort_signal_onabort", Args: []ir.Operand{signal}})
					return g.boxJSValue(raw, fnType)
				}
			}
			if isEventObjectType(objType) {
				if physical, ok := eventPhysicalProperty(objType, e.Property); ok {
					obj := g.lowerExpr(e.Object)
					return g.eventField(obj, physical, g.semanticType(e))
				}
			}
			if objType.Name == "$DOMException" {
				raw := g.lowerExpr(e.Object)
				boxed := g.boxJSValue(raw, objType)
				value := g.lowerDynamicGet(boxed, e.Property)
				if field, ok := objType.Fields[e.Property]; ok {
					return g.coerceJSValueBoundary(value, types.TypeAny, field.Type)
				}
				return value
			}
			if objType.Name == "$RegExp" && e.Property == "source" {
				obj := g.lowerExpr(e.Object)
				res := g.currentFn.NewValue("regexp_source", types.TypeString)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: "source", Offset: 16})
				return res
			}
			if collection := g.semaResult.BuiltinCollections[objType.Name]; collection != nil && e.Property == "size" {
				obj := g.lowerExpr(e.Object)
				res := g.currentFn.NewValue("collection_size", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_size", Args: []ir.Operand{obj}})
				return res
			}
			offsets, _, _ := g.objectLayout(objType)
			offset := offsets[e.Property]
			obj := g.lowerExpr(e.Object)
			resultType := types.TypeAny
			if t := g.semanticType(e); t != nil {
				resultType = t
			}
			res := g.currentFn.NewValue("field", resultType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: e.Property, Offset: offset})
			return res
		}
		if _, ok := g.semanticType(e.Object).(*types.UnionType); ok {
			obj := g.lowerExpr(e.Object)
			if concrete, ok := obj.Type().(*types.ObjectType); ok {
				offsets, _, _ := g.objectLayout(concrete)
				if offset, exists := offsets[e.Property]; exists {
					resultType := types.TypeAny
					if t := g.semanticType(e); t != nil {
						resultType = t
					}
					res := g.currentFn.NewValue("union_field", resultType)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: e.Property, Offset: offset})
					return res
				}
			}
		}
		obj := g.lowerExpr(e.Object)
		if concrete, ok := obj.Type().(*types.ObjectType); ok {
			offsets, _, _ := g.objectLayout(concrete)
			field := concrete.Fields[e.Property]
			res := g.currentFn.NewValue("any_typed_field", field.Type)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: e.Property, Offset: offsets[e.Property]})
			return res
		}
		if concrete, ok := g.provenObjectType(e.Object); ok {
			offsets, _, _ := g.objectLayout(concrete)
			if offset, exists := offsets[e.Property]; exists {
				field := concrete.Fields[e.Property]
				raw := g.unboxKnownObject(obj, concrete)
				res := g.currentFn.NewValue("any_field", field.Type)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: raw, Field: e.Property, Offset: offset})
				return res
			}
			return ir.ConstUndefined{}
		}
		return g.lowerDynamicGet(obj, e.Property)
	case *ast.CallExpr:
		if member, ok := e.Callee.(*ast.MemberExpr); ok {
			if ident, ok := member.Object.(*ast.IdentExpr); ok && ident.Name == "performance" {
				switch member.Property {
				case "now":
					res := g.currentFn.NewValue("performance_now", types.TypeNumber)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_performance_now"})
					return res
				case "toJSON":
					t := g.semanticType(e).(*types.ObjectType)
					offsets, refMask, shape := g.objectLayout(t)
					obj := g.currentFn.NewValue("performance_json", t)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: obj, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
					origin := g.currentFn.NewValue("performance_json_origin", types.TypeNumber)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: origin, Callee: "ts_performance_time_origin"})
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: obj, Field: "timeOrigin", Offset: offsets["timeOrigin"], Val: origin})
					return obj
				}
			}
			if promise, handled := g.lowerPromiseStaticCall(e, member); handled {
				return promise
			}
		}
		if ident, ok := e.Callee.(*ast.IdentExpr); ok {
			switch ident.Name {
			case "setTimeout":
				closure := g.lowerExpr(e.Args[0])
				delay := ir.Operand(ir.ConstNumber{Value: 0})
				if len(e.Args) == 2 {
					delay = g.lowerExpr(e.Args[1])
				}
				res := g.currentFn.NewValue("timer_id", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_set_timeout", Args: []ir.Operand{closure, delay}})
				return res
			case "clearTimeout":
				id := g.lowerExpr(e.Args[0])
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_clear_timeout", Args: []ir.Operand{id}})
				return nil
			case "queueMicrotask":
				closure := g.lowerExpr(e.Args[0])
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
					Callee: "ts_microtask_spawn",
					Args:   []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(types.TypeVoid)}},
				})
				return nil
			case "atob":
				input := g.lowerExpr(e.Args[0])
				valid := g.currentFn.NewValue("atob_valid", types.TypeBoolean)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: valid, Callee: "ts_atob_valid", Args: []ir.Operand{input}, ParamTypes: []types.Type{types.TypeString}})
				okBB := g.currentFn.NewBlock("atob_decode")
				errBB := g.currentFn.NewBlock("atob_invalid")
				g.currentBB.Terminator = &ir.BranchTerm{Cond: valid, Then: okBB, Else: errBB}
				g.currentBB = errBB
				errObj := g.newDOMException(ir.ConstString{Value: "The string to be decoded is not correctly encoded."}, ir.ConstString{Value: "InvalidCharacterError"})
				g.routeThrownValue(g.boxJSValue(errObj, g.semaResult.DOMExceptionType))
				g.currentBB = okBB
				res := g.currentFn.NewValue("atob_result", types.TypeString)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_atob", Args: []ir.Operand{input}, ParamTypes: []types.Type{types.TypeString}})
				return res
			case "btoa":
				input := g.lowerExpr(e.Args[0])
				valid := g.currentFn.NewValue("btoa_valid", types.TypeBoolean)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: valid, Callee: "ts_btoa_valid", Args: []ir.Operand{input}, ParamTypes: []types.Type{types.TypeString}})
				okBB := g.currentFn.NewBlock("btoa_encode")
				errBB := g.currentFn.NewBlock("btoa_invalid")
				g.currentBB.Terminator = &ir.BranchTerm{Cond: valid, Then: okBB, Else: errBB}
				g.currentBB = errBB
				errObj := g.newDOMException(ir.ConstString{Value: "The string to be encoded contains characters outside of the Latin1 range."}, ir.ConstString{Value: "InvalidCharacterError"})
				g.routeThrownValue(g.boxJSValue(errObj, g.semaResult.DOMExceptionType))
				g.currentBB = okBB
				res := g.currentFn.NewValue("btoa_result", types.TypeString)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_btoa", Args: []ir.Operand{input}, ParamTypes: []types.Type{types.TypeString}})
				return res
			case "taskGroup":
				groupType := g.semanticType(e).(*types.ObjectType)
				res := g.currentFn.NewValue("task_group", groupType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_task_group_new"})
				return res
			case "groupSpawn":
				group := g.lowerExpr(e.Args[0])
				closure := g.lowerExpr(e.Args[1])
				taskType := g.semanticType(e).(*types.ObjectType)
				resultType := g.semaResult.TaskResults[taskType.Name]
				res := g.currentFn.NewValue("group_task", taskType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_task_group_spawn", Args: []ir.Operand{group, closure, ir.ConstNumber{Value: nativeTaskResultKind(resultType)}}})
				return res
			case "groupJoin":
				group := g.lowerExpr(e.Args[0])
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_group_join", Args: []ir.Operand{group}})
				return nil
			case "groupCancel":
				group := g.lowerExpr(e.Args[0])
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_group_cancel", Args: []ir.Operand{group}})
				return nil
			case "channel":
				capacity := g.lowerExpr(e.Args[0])
				channelType := g.semanticType(e).(*types.ObjectType)
				res := g.currentFn.NewValue("channel", channelType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_channel_new", Args: []ir.Operand{capacity}, ParamTypes: []types.Type{types.TypeNumber}})
				return res
			case "channelSend":
				ch := g.lowerExpr(e.Args[0])
				chType := g.semanticType(e.Args[0]).(*types.ObjectType)
				value := g.lowerExpr(e.Args[1])
				value = g.boxJSValue(value, g.semanticType(e.Args[1]))
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_channel_send", Args: []ir.Operand{ch, value}, ParamTypes: []types.Type{chType, types.TypeAny}})
				return nil
			case "channelRecv":
				ch := g.lowerExpr(e.Args[0])
				chType := g.semanticType(e.Args[0]).(*types.ObjectType)
				elem := g.semaResult.ChannelElements[chType.Name]
				boxed := g.currentFn.NewValue("channel_recv", types.TypeAny)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: boxed, Callee: "ts_channel_recv", Args: []ir.Operand{ch}, ParamTypes: []types.Type{chType}})
				return g.coerceJSValueBoundary(boxed, types.TypeAny, elem)
			case "channelTrySend":
				ch := g.lowerExpr(e.Args[0])
				chType := g.semanticType(e.Args[0]).(*types.ObjectType)
				value := g.lowerExpr(e.Args[1])
				value = g.boxJSValue(value, g.semanticType(e.Args[1]))
				res := g.currentFn.NewValue("channel_sent", types.TypeBoolean)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_channel_try_send", Args: []ir.Operand{ch, value}, ParamTypes: []types.Type{chType, types.TypeAny}})
				return res
			case "channelTryRecvOr":
				ch := g.lowerExpr(e.Args[0])
				chType := g.semanticType(e.Args[0]).(*types.ObjectType)
				elem := g.semaResult.ChannelElements[chType.Name]
				fallback := g.lowerExpr(e.Args[1])
				fallback = g.boxJSValue(fallback, g.semanticType(e.Args[1]))
				boxed := g.currentFn.NewValue("channel_boxed", types.TypeAny)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: boxed, Callee: "ts_channel_try_recv_or", Args: []ir.Operand{ch, fallback}, ParamTypes: []types.Type{chType, types.TypeAny}})
				return g.coerceJSValueBoundary(boxed, types.TypeAny, elem)
			case "spawn":
				taskType := g.semanticType(e).(*types.ObjectType)
				resultType := g.semaResult.TaskResults[taskType.Name]
				closure := g.lowerExpr(e.Args[0])
				res := g.currentFn.NewValue("task", taskType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
					Res: res, Callee: "ts_task_spawn",
					Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(resultType)}},
				})
				return res
			case "yieldNow":
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_yield"})
				return nil
			case "sleep":
				ms := g.lowerExpr(e.Args[0])
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_sleep", Args: []ir.Operand{ms}, ParamTypes: []types.Type{types.TypeNumber}})
				return nil
			case "setTaskContext":
				value := g.lowerExpr(e.Args[0])
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_set_context", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeString}})
				return nil
			case "taskContext":
				res := g.currentFn.NewValue("task_context", types.TypeString)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_task_context"})
				return res
			case "cancelTask":
				task := g.lowerExpr(e.Args[0])
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_cancel", Args: []ir.Operand{task}})
				return nil
			case "taskCancelled":
				res := g.currentFn.NewValue("task_cancelled", types.TypeBoolean)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_task_cancelled"})
				return res
			case "join":
				task := g.lowerExpr(e.Args[0])
				resultType := g.semanticType(e)
				if resultType == nil || resultType.Kind() == types.KindVoid {
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_join", Args: []ir.Operand{task}})
					return nil
				}
				res := g.currentFn.NewValue("task_result", resultType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_task_join", Args: []ir.Operand{task}})
				return res
			}
		}
		if isConsoleLogCall(e.Callee) {
			return g.lowerConsoleLog(e.Args[0])
		}
		if _, ok := e.Callee.(*ast.SuperExpr); ok {
			thisVal := g.locals["$this"]
			args := make([]ir.Operand, 0, len(e.Args)+1)
			args = append(args, thisVal)
			for _, arg := range e.Args {
				args = append(args, g.lowerExpr(arg))
			}
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: classConstructorName(g.currentClass.BaseName), Args: args})
			return nil
		}
		if mem, ok := e.Callee.(*ast.MemberExpr); ok {
			if res, handled := g.lowerArrayBufferMethodCall(e, mem); handled {
				return res
			}
			if res, handled := g.lowerUint8ArrayMethodCall(e, mem); handled {
				return res
			}
			if res, handled := g.lowerAbortSignalStaticCall(e, mem); handled {
				return res
			}
			if res, handled := g.lowerEventMethodCall(e, mem); handled {
				return res
			}
			if res, handled := g.lowerEventTargetMethodCall(e, mem); handled {
				return res
			}
			if res, handled := g.lowerAbortControllerMethodCall(e, mem); handled {
				return res
			}
			if res, handled := g.lowerAbortSignalMethodCall(e, mem); handled {
				return res
			}
			if proven, ok := g.provenObjectType(mem.Object); ok {
				if field, exists := proven.Fields[mem.Property]; exists {
					if fnType, ok := field.Type.(*types.FunctionType); ok {
						boxedReceiver := g.lowerExpr(mem.Object)
						receiver := g.unboxKnownObject(boxedReceiver, proven)
						offsets, _, _ := g.objectLayout(proven)
						closure := g.currentFn.NewValue("method_closure", fnType)
						g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: closure, Obj: receiver, Field: mem.Property, Offset: offsets[mem.Property]})
						args := make([]ir.Operand, 0, len(e.Args))
						sourceTypes := make([]types.Type, 0, len(e.Args))
						for _, arg := range e.Args {
							args = append(args, g.lowerExpr(arg))
							sourceTypes = append(sourceTypes, g.semanticType(arg))
						}
						args = g.coerceCallOperands(args, sourceTypes, fnType)
						args = g.packRestOperands(args, fnType)
						res := g.currentFn.NewValue("structural_method", fnType.Return)
						paramTypes := make([]types.Type, len(fnType.Params))
						for i := range fnType.Params {
							paramTypes[i] = fnType.Params[i].Type
						}
						var thisArg ir.Operand
						if fnType.This != nil {
							thisArg = receiver
						}
						g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{Res: res, Closure: closure, ThisArg: thisArg, Args: args, ParamTypes: paramTypes})
						return res
					}
				}
				if info := g.semaResult.Classes[proven.Name]; info != nil && info.Methods[mem.Property] != nil {
					boxed := g.lowerExpr(mem.Object)
					receiver := g.unboxKnownObject(boxed, proven)
					callArgs := make([]ir.Operand, 0, len(e.Args))
					for _, arg := range e.Args {
						callArgs = append(callArgs, g.lowerExpr(arg))
					}
					return g.emitClassMethodCall(receiver, info, mem.Property, callArgs)
				}
			}
			if isBuiltinRegExpType(g.semanticType(mem.Object)) {
				return g.emitRegExpTest(e, mem)
			}
			if ident, ok := mem.Object.(*ast.IdentExpr); ok && ident.Name == "JSON" {
				return g.lowerJSONCall(e, mem)
			}
			if ident, ok := mem.Object.(*ast.IdentExpr); ok && ident.Name == "Date" && mem.Property == "now" {
				res := g.currentFn.NewValue("date_now", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_date_now"})
				return res
			}
			if isBuiltinDateType(g.semanticType(mem.Object)) {
				return g.emitDateMethodCall(e, mem)
			}
			if collection := g.builtinCollectionInfo(g.semanticType(mem.Object)); collection != nil {
				return g.emitBuiltinCollectionCall(e, mem, collection)
			}
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
					val := g.lowerExpr(e.Args[0])
					if irJSValueType(arrType.Elem) {
						val = g.boxJSValue(val, g.semanticType(e.Args[0]))
					}
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
		if fnType, ok := g.provenFunctionType(e.Callee); ok && !isConsoleLogCall(e.Callee) {
			args := make([]ir.Operand, 0, len(e.Args))
			sourceTypes := make([]types.Type, 0, len(e.Args))
			for _, arg := range e.Args {
				args = append(args, g.lowerExpr(arg))
				sourceTypes = append(sourceTypes, g.semanticType(arg))
			}
			args = g.coerceCallOperands(args, sourceTypes, fnType)
			args = g.packRestOperands(args, fnType)
			res := g.currentFn.NewValue("dynamic_call", fnType.Return)
			paramTypes := make([]types.Type, len(fnType.Params))
			for i := range fnType.Params {
				paramTypes[i] = fnType.Params[i].Type
			}
			if target, direct := g.directCalleeForExpr(e.Callee); direct {
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: target, Args: args, ParamTypes: paramTypes})
				return res
			}
			boxedClosure := g.lowerExpr(e.Callee)
			closure := g.coerceJSValueBoundary(boxedClosure, types.TypeAny, fnType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{Res: res, Closure: closure, Args: args, ParamTypes: paramTypes})
			return res
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
				sourceTypes := make([]types.Type, 0, len(e.Args))
				for _, arg := range e.Args {
					args = append(args, g.lowerExpr(arg))
					sourceTypes = append(sourceTypes, g.semanticType(arg))
				}
				args = g.coerceCallOperands(args, sourceTypes, fnType)
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
			if imported := g.semaResult.ImportAliases[ident.Name]; imported != "" {
				calleeName = imported
			}
			if decl := g.genericDecls[calleeName]; decl != nil {
				concrete := g.semaResult.GenericCalls[e]
				if len(g.typeBindings) > 0 {
					concrete = types.Substitute(concrete, g.typeBindings).(*types.FunctionType)
				}
				specialized := g.ensureGenericSpecialization(decl, concrete)
				calleeName = specialized
			}
		}
		var args []ir.Operand
		var sourceTypes []types.Type
		for _, arg := range e.Args {
			args = append(args, g.lowerExpr(arg))
			sourceTypes = append(sourceTypes, g.semanticType(arg))
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
						sourceTypes = append(sourceTypes, g.semanticType(p.Default))
						continue
					}
					args = append(args, ir.ConstUndefined{})
					sourceTypes = append(sourceTypes, types.TypeUndefined)
					continue
				}
			}
		}
		args = g.coerceCallOperands(args, sourceTypes, callType)
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
			if objType, ok := g.semanticType(mem.Object).(*types.ObjectType); ok && objType.Name == "$AbortSignal" && mem.Property == "onabort" {
				if e.Op != token.Eq {
					return g.failExpr("compound assignment to AbortSignal.onabort is not supported")
				}
				signal := g.lowerExpr(mem.Object)
				rhs := g.lowerExpr(e.Right)
				fnType := types.NewFunction([]types.Param{{Name: "event", Type: g.semaResult.EventType}}, types.TypeVoid)
				oldHandler := g.currentFn.NewValue("abort_old_handler", fnType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: oldHandler, Callee: "ts_abort_signal_onabort", Args: []ir.Operand{signal}})
				removeBB := g.currentFn.NewBlock("abort_onabort_remove")
				setBB := g.currentFn.NewBlock("abort_onabort_set")
				g.currentBB.Terminator = &ir.BranchTerm{Cond: oldHandler, Then: removeBB, Else: setBB}
				removeBB.Instructions = append(removeBB.Instructions, &ir.CallInst{Callee: "ts_event_target_remove", Args: []ir.Operand{signal, ir.ConstString{Value: "abort"}, oldHandler, ir.ConstBool{Value: false}}})
				removeBB.Terminator = &ir.JumpTerm{Target: setBB}
				g.currentBB = setBB
				handler := rhs
				isNull := false
				if _, ok := e.Right.(*ast.NullLit); ok {
					isNull = true
					handler = g.nullRef(fnType)
				} else if irJSValueType(rhs.Type()) {
					handler = g.coerceJSValueBoundary(rhs, rhs.Type(), fnType)
				}
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_abort_signal_set_onabort", Args: []ir.Operand{signal, handler}})
				if !isNull {
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_event_target_add", Args: []ir.Operand{signal, ir.ConstString{Value: "abort"}, handler, ir.ConstBool{Value: false}, ir.ConstBool{Value: false}, ir.ConstBool{Value: false}}})
				}
				return rhs
			}

			if objType, ok := g.semanticType(mem.Object).(*types.ObjectType); ok {
				offsets, _, _ := g.objectLayout(objType)
				offset := offsets[mem.Property]
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
			obj := g.lowerExpr(mem.Object)
			if concrete, ok := g.provenObjectType(mem.Object); ok {
				offsets, _, _ := g.objectLayout(concrete)
				if offset, exists := offsets[mem.Property]; exists {
					if e.Op != token.Eq {
						return g.failExpr("compound assignment through any alias is not implemented yet")
					}
					rhs := g.lowerExpr(e.Right)
					raw := g.unboxKnownObject(obj, concrete)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: raw, Field: mem.Property, Offset: offset, Val: rhs})
					return rhs
				}
				return g.failExpr("cannot add property %q to a proven closed shape through any", mem.Property)
			}
			if e.Op != token.Eq {
				return g.failExpr("dynamic compound property assignment is not implemented yet")
			}
			return g.lowerDynamicSet(obj, mem.Property, e.Right)
		}

		if idx, ok := e.Left.(*ast.IndexExpr); ok {
			if obj, ok := g.semanticType(idx.Target).(*types.ObjectType); ok && obj.Name == "$Uint8Array" {
				target := g.lowerExpr(idx.Target)
				index := g.lowerExpr(idx.Index)
				value := g.lowerExpr(e.Right)
				data := g.uint8ArrayField(target, "$data", g.semaResult.ByteBufferType)
				offset := g.uint8ArrayField(target, "byteOffset", types.TypeNumber)
				actual := g.currentFn.NewValue("uint8_index", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: actual, Op: ir.OpAdd, LHS: offset, RHS: index})
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_byte_buffer_set", Args: []ir.Operand{data, actual, value}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
				return value
			}
			if key, ok := g.staticStringKey(idx.Index); ok {
				target := g.lowerExpr(idx.Target)
				if object, ok := target.Type().(*types.ObjectType); ok {
					offsets, _, _ := g.objectLayout(object)
					offset := offsets[key]
					fieldType := object.Fields[key].Type
					if e.Op == token.Eq {
						rhs := g.lowerExpr(e.Right)
						g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: target, Field: key, Offset: offset, Val: rhs})
						return rhs
					}
					current := g.currentFn.NewValue("computed_old", fieldType)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: current, Obj: target, Field: key, Offset: offset})
					rhs := g.lowerExpr(e.Right)
					value := g.lowerAssignmentValue(e, current, rhs)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: target, Field: key, Offset: offset, Val: value})
					return value
				}
				if concrete, ok := g.provenObjectType(idx.Target); ok {
					offsets, _, _ := g.objectLayout(concrete)
					if offset, exists := offsets[key]; exists {
						if e.Op != token.Eq {
							return g.failExpr("compound computed assignment through any alias is not implemented yet")
						}
						rhs := g.lowerExpr(e.Right)
						raw := g.unboxKnownObject(target, concrete)
						g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: raw, Field: key, Offset: offset, Val: rhs})
						return rhs
					}
					return g.failExpr("cannot add computed property %q to a proven closed shape through any", key)
				}
				if e.Op != token.Eq {
					return g.failExpr("dynamic computed compound assignment is not implemented yet")
				}
				return g.lowerDynamicSet(target, key, e.Right)
			}
			if tuple, isTuple := g.semanticType(idx.Target).(*types.TupleType); isTuple {
				lit := idx.Index.(*ast.NumberLit)
				i := int(lit.Value)
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
				stored := rhs
				if arrType, ok := array.Type().(*types.ArrayType); ok && irJSValueType(arrType.Elem) {
					stored = g.boxJSValue(rhs, g.semanticType(e.Right))
				}
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{Array: array, Index: index, Val: stored})
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
		ident := e.Left.(*ast.IdentExpr)
		current := g.readLocal(ident.Name)
		rhs := g.lowerExpr(e.Right)
		if e.Op == token.Eq {
			targetType := g.semanticType(e.Left)
			sourceType := g.semanticType(e.Right)
			rhs = g.coerceJSValueBoundary(rhs, sourceType, targetType)
			if !g.writeCapturedLocal(ident.Name, rhs) {
				g.locals[ident.Name] = rhs
			}
			delete(g.localProvenance, ident.Name)
			delete(g.localDirectCallee, ident.Name)
			if irJSValueType(targetType) {
				switch concrete := sourceType.(type) {
				case *types.ObjectType:
					g.localProvenance[ident.Name] = concrete
				case *types.FunctionType:
					g.localProvenance[ident.Name] = concrete
					if target, ok := g.directCalleeForExpr(e.Right); ok {
						g.localDirectCallee[ident.Name] = target
					}
				}
			}
			return rhs
		}
		value := g.lowerAssignmentValue(e, current, rhs)
		if !g.writeCapturedLocal(ident.Name, value) {
			g.locals[ident.Name] = value
		}
		return value

	default:
		te := expr.(*ast.TernaryExpr)
		cond := g.lowerExpr(te.Cond)
		thenBB := g.currentFn.NewBlock("tern_then")
		elseBB := g.currentFn.NewBlock("tern_else")
		joinBB := g.currentFn.NewBlock("tern_join")

		g.currentBB.Terminator = &ir.BranchTerm{Cond: cond, Then: thenBB, Else: elseBB}

		g.currentBB = thenBB
		thenVal := g.lowerExpr(te.Then)
		thenEndBB := g.currentBB
		if thenEndBB.Terminator == nil {
			thenEndBB.Terminator = &ir.JumpTerm{Target: joinBB}
		}

		g.currentBB = elseBB
		elseVal := g.lowerExpr(te.Else)
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
	}
}
