package lowering

import (
	"fmt"

	"github.com/projectthorn/tsv7-bin/internal/frontend"
	"github.com/projectthorn/tsv7-bin/internal/hir"
)

func (f *functionLowerer) channelElementKind(channelType frontend.TypeID) (frontend.TypeID, hir.ChannelElementKind, error) {
	if int(channelType) >= len(f.module.source.Types) {
		return 0, hir.ChannelElementInvalid, fmt.Errorf("channel type t%d is invalid", channelType)
	}
	typ := f.module.source.Types[channelType]
	if typ.Kind != frontend.TypeChannel || int(typ.Element) >= len(f.module.source.Types) {
		return 0, hir.ChannelElementInvalid, fmt.Errorf("semantic type %q is not a concrete channel", typ.Name)
	}
	kind := f.module.source.Types[typ.Element].Kind
	switch kind {
	case frontend.TypeNumber:
		return typ.Element, hir.ChannelElementF64, nil
	case frontend.TypeBoolean:
		return typ.Element, hir.ChannelElementBool, nil
	case frontend.TypeString, frontend.TypeObject, frontend.TypeArray, frontend.TypeFunction, frontend.TypeAny, frontend.TypeUnion, frontend.TypeNull, frontend.TypeUndefined:
		return typ.Element, hir.ChannelElementRef, nil
	default:
		return typ.Element, hir.ChannelElementInvalid, fmt.Errorf("channel element type %q has no native channel representation", f.module.source.Types[typ.Element].Name)
	}
}

func (f *functionLowerer) promiseResultKind(promiseType frontend.TypeID) (frontend.TypeID, hir.TaskResultKind, error) {
	if int(promiseType) >= len(f.module.source.Types) {
		return 0, hir.TaskResultInvalid, fmt.Errorf("promise type t%d is invalid", promiseType)
	}
	typ := f.module.source.Types[promiseType]
	if typ.Kind != frontend.TypePromise || int(typ.ReturnType) >= len(f.module.source.Types) {
		return 0, hir.TaskResultInvalid, fmt.Errorf("semantic type %q is not a concrete Promise", typ.Name)
	}
	result := f.module.source.Types[typ.ReturnType]
	switch result.Kind {
	case frontend.TypeNumber:
		return typ.ReturnType, hir.TaskResultF64, nil
	case frontend.TypeBoolean:
		return typ.ReturnType, hir.TaskResultBool, nil
	case frontend.TypeString, frontend.TypeObject, frontend.TypeArray, frontend.TypeFunction, frontend.TypeAny, frontend.TypeUnion, frontend.TypeNull, frontend.TypeUndefined:
		return typ.ReturnType, hir.TaskResultRef, nil
	default:
		return typ.ReturnType, hir.TaskResultInvalid, fmt.Errorf("Promise result type %q has no immediate native task representation", result.Name)
	}
}

func (f *functionLowerer) lowerExpr(expr *frontend.Expr) (hir.ValueID, error) {
	if expr == nil {
		return 0, fmt.Errorf("nil semantic expression")
	}
	switch expr.Kind {
	case frontend.ExprIdentifier:
		value, ok := f.locals[expr.Symbol]
		if !ok {
			return 0, fmt.Errorf("identifier %q is not a local value", expr.Name)
		}
		return value, nil
	case frontend.ExprBoolean:
		return f.emit(expr.Type, hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralBoolean, Bool: expr.Boolean}}), nil
	case frontend.ExprNumber:
		return f.emit(expr.Type, hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralNumber, Number: expr.Number}}), nil
	case frontend.ExprString:
		return f.emit(expr.Type, hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralString, String: expr.String}}), nil
	case frontend.ExprNull:
		return f.emit(expr.Type, hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralNull}}), nil
	case frontend.ExprUndefined:
		return f.emit(expr.Type, hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralUndefined}}), nil
	case frontend.ExprBinary:
		return f.lowerBinary(expr)
	case frontend.ExprCall:
		return f.lowerCall(expr)
	case frontend.ExprDynamicCall:
		return f.lowerDynamicCall(expr)
	case frontend.ExprArray:
		elements := make([]hir.ValueID, 0, len(expr.Elements))
		for _, element := range expr.Elements {
			value, err := f.lowerExpr(element)
			if err != nil {
				return 0, err
			}
			elements = append(elements, value)
		}
		return f.emit(expr.Type, hir.ArrayNewOp{Elements: elements}), nil
	case frontend.ExprArrayLength:
		array, err := f.lowerExpr(expr.Object)
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.ArrayLengthOp{Array: array}), nil
	case frontend.ExprIndex:
		array, err := f.lowerExpr(expr.Object)
		if err != nil {
			return 0, err
		}
		index, err := f.lowerExpr(expr.Index)
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.ArrayGetOp{Array: array, Index: index}), nil
	case frontend.ExprObject:
		if int(expr.Type) >= len(f.module.source.Types) {
			return 0, fmt.Errorf("object expression has invalid type t%d", expr.Type)
		}
		shape := f.module.source.Types[expr.Type].Shape
		fields := make([]hir.ValueID, 0, len(expr.Fields))
		for _, field := range expr.Fields {
			value, err := f.lowerExpr(field.Value)
			if err != nil {
				return 0, err
			}
			fields = append(fields, value)
		}
		return f.emit(expr.Type, hir.ObjectNewOp{Shape: hir.NewShapeID(uint32(shape)), Fields: fields}), nil
	case frontend.ExprNewClass:
		if expr.Constructor == nil {
			return 0, fmt.Errorf("native class construction has no constructor target")
		}
		if int(expr.Type) >= len(f.module.source.Types) {
			return 0, fmt.Errorf("native class construction has invalid type")
		}
		shape := f.module.source.Types[expr.Type].Shape
		object := f.emit(expr.Type, hir.ObjectAllocOp{Shape: hir.NewShapeID(uint32(shape))})
		args := []hir.ValueID{object}
		for _, arg := range expr.Args {
			value, err := f.lowerExpr(arg)
			if err != nil {
				return 0, err
			}
			args = append(args, value)
		}
		voidType, ok := findFrontendType(f.module.source, frontend.TypeVoid)
		if !ok {
			return 0, fmt.Errorf("native constructor call requires void type")
		}
		f.emit(voidType, hir.CallOp{Callee: hir.NewFunctionID(uint32(*expr.Constructor)), Args: args})
		return object, nil
	case frontend.ExprFieldGet:
		object, err := f.lowerExpr(expr.Object)
		if err != nil {
			return 0, err
		}
		if int(expr.Object.Type) >= len(f.module.source.Types) {
			return 0, fmt.Errorf("field access has invalid object type")
		}
		shape := f.module.source.Types[expr.Object.Type].Shape
		return f.emit(expr.Type, hir.FieldGetOp{Object: object, Shape: hir.NewShapeID(uint32(shape)), Field: expr.FieldIndex}), nil
	case frontend.ExprDynamicFieldGet:
		object, err := f.lowerExpr(expr.Object)
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.DynamicFieldGetOp{Object: object, Field: expr.Field}), nil
	case frontend.ExprPromiseResolve:
		resultType, resultKind, err := f.promiseResultKind(expr.Type)
		if err != nil {
			return 0, err
		}
		value, err := f.lowerExprAs(expr.Args[0], resultType)
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.PromiseResolveOp{Value: value, Result: resultKind}), nil
	case frontend.ExprPromiseReject:
		_, resultKind, err := f.promiseResultKind(expr.Type)
		if err != nil {
			return 0, err
		}
		anyType, ok := findFrontendType(f.module.source, frontend.TypeAny)
		if !ok {
			return 0, fmt.Errorf("Promise.reject requires any semantic type")
		}
		reason, err := f.lowerExprAs(expr.Args[0], anyType)
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.PromiseRejectOp{Reason: reason, Result: resultKind}), nil
	case frontend.ExprTaskSpawn:
		if expr.CallTarget == nil {
			return 0, fmt.Errorf("task spawn has no native target")
		}
		targetID := *expr.CallTarget
		if int(targetID) >= len(f.module.source.Functions) {
			return 0, fmt.Errorf("task target f%d is outside semantic function table", targetID)
		}
		target := f.module.source.Functions[targetID]
		if len(target.Params) != len(expr.Captures) {
			return 0, fmt.Errorf("task target %s expects %d captures; got %d", target.Name, len(target.Params), len(expr.Captures))
		}
		var group *hir.ValueID
		if expr.Object != nil {
			groupValue, err := f.lowerExpr(expr.Object)
			if err != nil {
				return 0, err
			}
			group = &groupValue
		}
		captures := make([]hir.ValueID, 0, len(expr.Captures))
		for i, capture := range expr.Captures {
			value, err := f.lowerExprAs(capture, target.Params[i].Type)
			if err != nil {
				return 0, err
			}
			captures = append(captures, value)
		}
		return f.emit(expr.Type, hir.TaskSpawnOp{Callee: hir.NewFunctionID(uint32(targetID)), Captures: captures, Group: group}), nil
	case frontend.ExprTaskJoin:
		if len(expr.Args) != 1 {
			return 0, fmt.Errorf("task join requires one handle")
		}
		if len(f.handlers) != 0 {
			return f.lowerCaughtTaskJoin(expr)
		}
		task, err := f.lowerExpr(expr.Args[0])
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.TaskJoinOp{Task: task}), nil
	case frontend.ExprTaskYield:
		return f.emit(expr.Type, hir.TaskYieldOp{}), nil
	case frontend.ExprTaskCancel:
		if len(expr.Args) != 1 {
			return 0, fmt.Errorf("task cancel requires one handle")
		}
		task, err := f.lowerExpr(expr.Args[0])
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.TaskCancelOp{Task: task}), nil
	case frontend.ExprTaskCancelled:
		return f.emit(expr.Type, hir.TaskCancelledOp{}), nil
	case frontend.ExprTaskGroupNew:
		return f.emit(expr.Type, hir.TaskGroupNewOp{}), nil
	case frontend.ExprTaskGroupJoin:
		group, err := f.lowerExpr(expr.Args[0])
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.TaskGroupJoinOp{Group: group}), nil
	case frontend.ExprTaskGroupCancel:
		group, err := f.lowerExpr(expr.Args[0])
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.TaskGroupCancelOp{Group: group}), nil
	case frontend.ExprTaskContextSet:
		anyType, ok := findFrontendType(f.module.source, frontend.TypeAny)
		if !ok {
			return 0, fmt.Errorf("task context requires any semantic type")
		}
		value, err := f.lowerExprAs(expr.Args[0], anyType)
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.TaskContextSetOp{Value: value}), nil
	case frontend.ExprTaskContextGet:
		return f.emit(expr.Type, hir.TaskContextGetOp{}), nil
	case frontend.ExprChannelNew:
		_, elementKind, err := f.channelElementKind(expr.Type)
		if err != nil {
			return 0, err
		}
		capacity, err := f.lowerExpr(expr.Args[0])
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.ChannelNewOp{Capacity: capacity, Element: elementKind}), nil
	case frontend.ExprChannelTrySend:
		elementType, elementKind, err := f.channelElementKind(expr.Args[0].Type)
		if err != nil {
			return 0, err
		}
		channel, err := f.lowerExpr(expr.Args[0])
		if err != nil {
			return 0, err
		}
		value, err := f.lowerExprAs(expr.Args[1], elementType)
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.ChannelTrySendOp{Channel: channel, Value: value, Element: elementKind}), nil
	case frontend.ExprChannelTryRecvOr:
		elementType, elementKind, err := f.channelElementKind(expr.Args[0].Type)
		if err != nil {
			return 0, err
		}
		channel, err := f.lowerExpr(expr.Args[0])
		if err != nil {
			return 0, err
		}
		fallback, err := f.lowerExprAs(expr.Args[1], elementType)
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.ChannelTryRecvOrOp{Channel: channel, Fallback: fallback, Element: elementKind}), nil
	case frontend.ExprChannelSend:
		elementType, elementKind, err := f.channelElementKind(expr.Args[0].Type)
		if err != nil {
			return 0, err
		}
		channel, err := f.lowerExpr(expr.Args[0])
		if err != nil {
			return 0, err
		}
		value, err := f.lowerExprAs(expr.Args[1], elementType)
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.ChannelSendOp{Channel: channel, Value: value, Element: elementKind}), nil
	case frontend.ExprChannelRecv:
		_, elementKind, err := f.channelElementKind(expr.Args[0].Type)
		if err != nil {
			return 0, err
		}
		channel, err := f.lowerExpr(expr.Args[0])
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.ChannelRecvOp{Channel: channel, Element: elementKind}), nil
	case frontend.ExprSleep:
		duration, err := f.lowerExpr(expr.Args[0])
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.SleepOp{Duration: duration}), nil
	case frontend.ExprClosure:
		if expr.CallTarget == nil {
			return 0, fmt.Errorf("closure value has no native target")
		}
		closureType := expr.Type
		if int(closureType) < len(f.module.source.Types) {
			kind := f.module.source.Types[closureType].Kind
			if kind == frontend.TypeAny || kind == frontend.TypeUnion {
				if nativeFunctionType, ok := findFrontendType(f.module.source, frontend.TypeFunction); ok {
					closureType = nativeFunctionType
				} else {
					return 0, fmt.Errorf("dynamic closure value requires a native function semantic type")
				}
			}
		}
		captures := make([]hir.ValueID, 0, len(expr.Captures))
		for _, capture := range expr.Captures {
			value, err := f.lowerExpr(capture)
			if err != nil {
				return 0, err
			}
			captures = append(captures, value)
		}
		return f.emit(closureType, hir.ClosureNewOp{Callee: hir.NewFunctionID(uint32(*expr.CallTarget)), Captures: captures}), nil
	default:
		return 0, fmt.Errorf("unsupported semantic expression kind %d", expr.Kind)
	}
}

func (f *functionLowerer) lowerBinary(expr *frontend.Expr) (hir.ValueID, error) {
	op := hir.BinaryOperator(0)
	switch expr.Operator {
	case frontend.BinaryAdd:
		op = hir.BinaryAdd
	case frontend.BinarySub:
		op = hir.BinarySub
	case frontend.BinaryMul:
		op = hir.BinaryMul
	case frontend.BinaryDiv:
		op = hir.BinaryDiv
	case frontend.BinaryLessThan:
		op = hir.BinaryLessThan
	case frontend.BinaryLessEqual:
		op = hir.BinaryLessEqual
	case frontend.BinaryGreaterThan:
		op = hir.BinaryGreaterThan
	case frontend.BinaryGreaterEqual:
		op = hir.BinaryGreaterEqual
	case frontend.BinaryEqual:
		op = hir.BinaryEqual
	case frontend.BinaryNotEqual:
		op = hir.BinaryNotEqual
	case frontend.BinaryStrictEqual:
		op = hir.BinaryStrictEqual
	case frontend.BinaryStrictNotEqual:
		op = hir.BinaryStrictNotEqual
	default:
		return 0, fmt.Errorf("unsupported semantic binary operator %d", expr.Operator)
	}
	dynamicType := frontend.TypeID(0)
	dynamic := false
	for _, operand := range []*frontend.Expr{expr.Left, expr.Right} {
		if operand == nil || int(operand.Type) >= len(f.module.source.Types) {
			continue
		}
		kind := f.module.source.Types[operand.Type].Kind
		if kind == frontend.TypeAny || kind == frontend.TypeUnion {
			dynamicType = operand.Type
			dynamic = true
			break
		}
	}
	if dynamic {
		left, err := f.lowerExprAs(expr.Left, dynamicType)
		if err != nil {
			return 0, err
		}
		right, err := f.lowerExprAs(expr.Right, dynamicType)
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.DynamicBinaryOp{Operator: op, Left: left, Right: right}), nil
	}
	left, err := f.lowerExpr(expr.Left)
	if err != nil {
		return 0, err
	}
	right, err := f.lowerExpr(expr.Right)
	if err != nil {
		return 0, err
	}
	return f.emit(expr.Type, hir.BinaryExpr{Operator: op, Left: left, Right: right}), nil
}

func (f *functionLowerer) lowerCall(expr *frontend.Expr) (hir.ValueID, error) {
	lowerArgs := func() ([]hir.ValueID, error) {
		args := make([]hir.ValueID, 0, len(expr.Args))
		for _, arg := range expr.Args {
			value, err := f.lowerExpr(arg)
			if err != nil {
				return nil, err
			}
			args = append(args, value)
		}
		return args, nil
	}
	if expr.Intrinsic != frontend.IntrinsicNone {
		args, err := lowerArgs()
		if err != nil {
			return 0, err
		}
		intrinsic := hir.IntrinsicInvalid
		switch expr.Intrinsic {
		case frontend.IntrinsicConsoleLogF64:
			intrinsic = hir.IntrinsicConsoleLogF64
		case frontend.IntrinsicConsoleLogString:
			intrinsic = hir.IntrinsicConsoleLogString
		case frontend.IntrinsicConsoleLogJSValue:
			intrinsic = hir.IntrinsicConsoleLogJSValue
		default:
			return 0, fmt.Errorf("unsupported semantic intrinsic %d", expr.Intrinsic)
		}
		return f.emit(expr.Type, hir.IntrinsicCallOp{Intrinsic: intrinsic, Args: args}), nil
	}
	if len(expr.Dispatch) != 0 {
		args, err := lowerArgs()
		if err != nil {
			return 0, err
		}
		cases := make([]hir.DispatchCase, 0, len(expr.Dispatch))
		for _, target := range expr.Dispatch {
			cases = append(cases, hir.DispatchCase{ClassTag: target.ClassTag, Callee: hir.NewFunctionID(uint32(target.Function))})
		}
		return f.emit(expr.Type, hir.DispatchCallOp{Args: args, Cases: cases}), nil
	}
	if expr.CallTarget != nil {
		targetID := *expr.CallTarget
		if int(targetID) >= len(f.module.source.Functions) {
			return 0, fmt.Errorf("call target f%d is outside semantic function table", targetID)
		}
		target := f.module.source.Functions[targetID]
		if len(target.Params) != len(expr.Args) {
			return 0, fmt.Errorf("call target %s expects %d args; got %d", target.Name, len(target.Params), len(expr.Args))
		}
		args := make([]hir.ValueID, 0, len(expr.Args))
		for i, arg := range expr.Args {
			value, err := f.lowerExprAs(arg, target.Params[i].Type)
			if err != nil {
				return 0, err
			}
			args = append(args, value)
		}
		return f.emit(expr.Type, hir.CallOp{Callee: hir.NewFunctionID(uint32(targetID)), Args: args}), nil
	}
	if expr.Callee == nil {
		return 0, fmt.Errorf("indirect call has no closure value")
	}
	closure, err := f.lowerExpr(expr.Callee)
	if err != nil {
		return 0, err
	}
	args, err := lowerArgs()
	if err != nil {
		return 0, err
	}
	return f.emit(expr.Type, hir.ClosureCallOp{Closure: closure, Args: args}), nil
}

func (f *functionLowerer) lowerDynamicCall(expr *frontend.Expr) (hir.ValueID, error) {
	if expr.Callee == nil {
		return 0, fmt.Errorf("dynamic call has no callee")
	}
	anyType, ok := findFrontendType(f.module.source, frontend.TypeAny)
	if !ok {
		return 0, fmt.Errorf("dynamic call requires any semantic type")
	}
	callee, err := f.lowerExprAs(expr.Callee, anyType)
	if err != nil {
		return 0, err
	}
	args := make([]hir.ValueID, 0, len(expr.Args))
	for _, arg := range expr.Args {
		value, err := f.lowerExprAs(arg, anyType)
		if err != nil {
			return 0, err
		}
		args = append(args, value)
	}
	return f.emit(expr.Type, hir.DynamicCallOp{Callee: callee, Args: args}), nil
}

func (f *functionLowerer) lowerCaughtTaskJoin(expr *frontend.Expr) (hir.ValueID, error) {
	task, err := f.lowerExpr(expr.Args[0])
	if err != nil {
		return 0, err
	}
	boolType, ok := findFrontendType(f.module.source, frontend.TypeBoolean)
	if !ok {
		return 0, fmt.Errorf("caught await requires boolean semantic type")
	}
	anyType, ok := findFrontendType(f.module.source, frontend.TypeAny)
	if !ok {
		return 0, fmt.Errorf("caught await requires any semantic type")
	}
	voidType, ok := findFrontendType(f.module.source, frontend.TypeVoid)
	if !ok {
		return 0, fmt.Errorf("caught await requires void semantic type")
	}
	status := f.emit(boolType, hir.TaskWaitOp{Task: task})
	successID := f.newBlockID()
	failureID := f.newBlockID()
	if err := f.terminate(hir.BranchTerm{Condition: status, Then: successID, Else: failureID}); err != nil {
		return 0, err
	}
	f.startBlock(failureID)
	failure := f.emit(anyType, hir.TaskFailureOp{Task: task})
	f.emit(voidType, hir.TaskReleaseOp{Task: task})
	index := len(f.handlers) - 1
	pred := f.block().ID
	f.handlers[index].incoming = append(f.handlers[index].incoming, hir.PhiIncoming{Block: pred, Value: failure})
	if err := f.terminate(hir.JumpTerm{Target: f.handlers[index].block}); err != nil {
		return 0, err
	}
	f.startBlock(successID)
	return f.emit(expr.Type, hir.TaskJoinOp{Task: task}), nil
}
