package irgen

import (
	"fmt"
	"sort"
	"strings"

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
	catchStack                 []*catchContext
	finallyStack               []*finallyContext
	activeStreamControllerKind string
}

func typeNodeIsAny(node ast.TypeNode) bool {
	primitive, ok := node.(*ast.PrimitiveTypeNode)
	return ok && primitive.Kind == "any"
}

func cloneOperandMap(src map[string]ir.Operand) map[string]ir.Operand {
	out := make(map[string]ir.Operand, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
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

func (g *generator) nullRefAt(bb *ir.BasicBlock, t types.Type) ir.Operand {
	res := g.currentFn.NewValue("null_ref", t)
	bb.Instructions = append(bb.Instructions, &ir.CallInst{Res: res, Callee: "ts_null_ref"})
	return res
}
