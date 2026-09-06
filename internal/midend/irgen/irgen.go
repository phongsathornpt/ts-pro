package irgen

import (
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
