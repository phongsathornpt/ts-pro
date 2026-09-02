package lowering

import (
	"fmt"

	rangeanalysis "github.com/phongsathornpt/ts-pro/internal/analysis/range"
	"github.com/phongsathornpt/ts-pro/internal/hir"
	"github.com/phongsathornpt/ts-pro/internal/mir"
)

func LowerMIR(source hir.Module) (mir.Module, error) {
	return LowerMIRWithRanges(source, nil)
}

func LowerMIRWithRanges(source hir.Module, ranges rangeanalysis.Result) (mir.Module, error) {
	result := mir.Module{Name: source.Name}
	for _, shape := range source.Shapes {
		lowered := mir.Shape{ID: mir.ShapeID(shape.ID), Name: shape.Name, ClassTag: shape.ClassTag}
		for _, field := range shape.Fields {
			repr, err := lowerRepr(field.Repr)
			if err != nil {
				return mir.Module{}, fmt.Errorf("shape %s field %s: %w", shape.Name, field.Name, err)
			}
			loweredField := mir.ShapeField{Name: field.Name, Repr: repr}
			if int(field.SemanticType) < len(source.Types) && source.Types[field.SemanticType].Kind == hir.TypeObject {
				loweredField.ObjectShape = mir.ShapeID(source.Types[field.SemanticType].Shape)
				loweredField.HasObjectShape = true
			}
			lowered.Fields = append(lowered.Fields, loweredField)
		}
		result.Shapes = append(result.Shapes, lowered)
	}
	if source.Entry != nil {
		entry := mir.FunctionID(*source.Entry)
		result.Entry = &entry
	}
	for _, fn := range source.Functions {
		lowered, err := lowerMIRFunction(fn, ranges[fn.ID], source.Types)
		if err != nil {
			return mir.Module{}, err
		}
		result.Functions = append(result.Functions, lowered)
	}
	if err := result.Verify(); err != nil {
		return mir.Module{}, fmt.Errorf("verify MIR: %w", err)
	}
	return result, nil
}

func lowerMIRFunction(source hir.Function, ranges rangeanalysis.FunctionResult, types []hir.SemanticType) (mir.Function, error) {
	returnRepr, err := lowerRepr(source.ReturnRepr)
	if err != nil {
		return mir.Function{}, fmt.Errorf("function %s return: %w", source.Name, err)
	}
	result := mir.Function{
		ID: mir.FunctionID(source.ID), Name: source.Name,
		ReturnRepr: returnRepr, HasExplicitThis: source.HasExplicitThis, Entry: mir.BlockID(source.Entry),
	}
	if int(source.ReturnType) < len(types) && types[source.ReturnType].Kind == hir.TypeObject {
		result.ReturnObjectShape = mir.ShapeID(types[source.ReturnType].Shape)
		result.HasReturnObjectShape = true
	}
	for _, param := range source.Params {
		repr, err := lowerRepr(param.Repr)
		if err != nil {
			return mir.Function{}, fmt.Errorf("function %s parameter %s: %w", source.Name, param.Name, err)
		}
		loweredParam := mir.Param{Value: mir.ValueID(param.Value), Name: param.Name, Repr: repr}
		if int(param.SemanticType) < len(types) && types[param.SemanticType].Kind == hir.TypeObject {
			loweredParam.ObjectShape = mir.ShapeID(types[param.SemanticType].Shape)
			loweredParam.HasObjectShape = true
		}
		result.Params = append(result.Params, loweredParam)
	}
	for _, block := range source.Blocks {
		lowered := mir.Block{ID: mir.BlockID(block.ID)}
		for _, instruction := range block.Instructions {
			inst, err := lowerMIRInstruction(instruction, ranges)
			if err != nil {
				return mir.Function{}, fmt.Errorf("function %s block b%d: %w", source.Name, block.ID, err)
			}
			lowered.Instructions = append(lowered.Instructions, inst)
		}
		term, err := lowerMIRTerminator(block.Terminator)
		if err != nil {
			return mir.Function{}, fmt.Errorf("function %s block b%d: %w", source.Name, block.ID, err)
		}
		lowered.Terminator = term
		result.Blocks = append(result.Blocks, lowered)
	}
	return result, nil
}

func lowerTaskResultRepr(kind hir.TaskResultKind) (mir.Repr, error) {
	switch kind {
	case hir.TaskResultF64:
		return mir.ReprF64, nil
	case hir.TaskResultBool:
		return mir.ReprBool, nil
	case hir.TaskResultRef:
		return mir.ReprJSValue, nil
	default:
		return mir.ReprVoid, fmt.Errorf("invalid task result kind %d", kind)
	}
}

func lowerMIRInstruction(source hir.Instruction, ranges rangeanalysis.FunctionResult) (mir.Instruction, error) {
	repr, err := lowerRepr(source.Repr)
	if err != nil {
		return mir.Instruction{}, fmt.Errorf("value v%d: %w", source.Result, err)
	}
	result := mir.Instruction{Result: mir.ValueID(source.Result), Repr: repr}
	switch op := source.Op.(type) {
	case hir.ConstOp:
		switch {
		case op.Literal.Kind == hir.LiteralBoolean && repr == mir.ReprBool:
			result.Op = mir.ConstBool{Value: op.Literal.Bool}
		case op.Literal.Kind == hir.LiteralNumber && repr == mir.ReprF64:
			result.Op = mir.ConstF64{Value: op.Literal.Number}
		case op.Literal.Kind == hir.LiteralString && repr == mir.ReprStringRef:
			result.Op = mir.ConstString{Value: op.Literal.String}
		case op.Literal.Kind == hir.LiteralNull && repr == mir.ReprJSValue:
			result.Op = mir.ConstJSValue{Kind: mir.ConstJSNull}
		case op.Literal.Kind == hir.LiteralUndefined && repr == mir.ReprJSValue:
			result.Op = mir.ConstJSValue{Kind: mir.ConstJSUndefined}
		default:
			return mir.Instruction{}, fmt.Errorf("unsupported const kind %d representation %v", op.Literal.Kind, repr)
		}
	case hir.BinaryExpr:
		if width, ok := provenIntegerWidth(source.Result, op, repr, ranges); ok {
			operator, err := lowerIntegerBinaryOperator(op.Operator)
			if err != nil {
				return mir.Instruction{}, err
			}
			result.Op = mir.ProvenIntBinary{Width: width, Operator: operator, Left: mir.ValueID(op.Left), Right: mir.ValueID(op.Right)}
			break
		}
		lowered, err := lowerMIRBinary(op, repr)
		if err != nil {
			return mir.Instruction{}, err
		}
		result.Op = lowered
	case hir.BoxOp:
		kind := mir.BoxJSInvalid
		switch op.Kind {
		case hir.BoxNumber:
			kind = mir.BoxJSNumber
		case hir.BoxString:
			kind = mir.BoxJSString
		case hir.BoxBoolean:
			kind = mir.BoxJSBoolean
		case hir.BoxObject:
			kind = mir.BoxJSObject
		case hir.BoxArray:
			kind = mir.BoxJSArray
		case hir.BoxFunction:
			kind = mir.BoxJSFunction
		}
		if kind == mir.BoxJSInvalid {
			return mir.Instruction{}, fmt.Errorf("invalid JSValue box kind %d", op.Kind)
		}
		result.Op = mir.BoxJSValue{Kind: kind, Value: mir.ValueID(op.Value), Shape: mir.ShapeID(op.Shape), Function: mir.FunctionID(op.Function), HasFunction: op.HasFunction}
	case hir.UnboxOp:
		kind := mir.UnboxJSInvalid
		switch op.Kind {
		case hir.UnboxNumber:
			kind = mir.UnboxJSNumber
		case hir.UnboxString:
			kind = mir.UnboxJSString
		case hir.UnboxBoolean:
			kind = mir.UnboxJSBoolean
		case hir.UnboxArray:
			kind = mir.UnboxJSArray
		case hir.UnboxObject:
			kind = mir.UnboxJSObject
		case hir.UnboxFunction:
			kind = mir.UnboxJSFunction
		}
		if kind == mir.UnboxJSInvalid {
			return mir.Instruction{}, fmt.Errorf("invalid JSValue unbox kind %d", op.Kind)
		}
		result.Op = mir.UnboxJSValue{Kind: kind, Value: mir.ValueID(op.Value), Shape: mir.ShapeID(op.Shape)}
	case hir.DynamicBinaryOp:
		if op.Operator == hir.BinaryAdd {
			result.Op = mir.DynamicAddJSValue{Left: mir.ValueID(op.Left), Right: mir.ValueID(op.Right)}
			break
		}
		operator := map[hir.BinaryOperator]mir.DynamicJSBinaryOp{
			hir.BinarySub: mir.DynamicJSSub, hir.BinaryMul: mir.DynamicJSMul, hir.BinaryDiv: mir.DynamicJSDiv,
			hir.BinaryLessThan: mir.DynamicJSLessThan, hir.BinaryLessEqual: mir.DynamicJSLessEqual,
			hir.BinaryGreaterThan: mir.DynamicJSGreaterThan, hir.BinaryGreaterEqual: mir.DynamicJSGreaterEqual,
			hir.BinaryEqual: mir.DynamicJSEqual, hir.BinaryNotEqual: mir.DynamicJSNotEqual,
			hir.BinaryStrictEqual: mir.DynamicJSStrictEqual, hir.BinaryStrictNotEqual: mir.DynamicJSStrictNotEqual,
		}[op.Operator]
		if operator == mir.DynamicJSInvalid {
			return mir.Instruction{}, fmt.Errorf("unsupported dynamic binary operator %d", op.Operator)
		}
		result.Op = mir.DynamicBinaryJSValue{Operator: operator, Left: mir.ValueID(op.Left), Right: mir.ValueID(op.Right)}
	case hir.DynamicCallOp:
		args := make([]mir.ValueID, len(op.Args))
		for i, arg := range op.Args {
			args[i] = mir.ValueID(arg)
		}
		result.Op = mir.DynamicCall{Callee: mir.ValueID(op.Callee), Receiver: mir.ValueID(op.Receiver), HasReceiver: op.HasReceiver, Args: args}
	case hir.DynamicMethodCallOp:
		args := make([]mir.ValueID, len(op.Args))
		for i, arg := range op.Args {
			args[i] = mir.ValueID(arg)
		}
		cases := make([]mir.DispatchCase, len(op.Cases))
		for i, c := range op.Cases {
			cases[i] = mir.DispatchCase{ClassTag: c.ClassTag, Callee: mir.FunctionID(c.Callee)}
		}
		result.Op = mir.DynamicMethodCall{Receiver: mir.ValueID(op.Receiver), Args: args, Cases: cases}
	case hir.CallOp:
		args := make([]mir.ValueID, len(op.Args))
		for i, arg := range op.Args {
			args[i] = mir.ValueID(arg)
		}
		result.Op = mir.Call{Callee: mir.FunctionID(op.Callee), Args: args}
	case hir.DispatchCallOp:
		args := make([]mir.ValueID, len(op.Args))
		for i, arg := range op.Args {
			args[i] = mir.ValueID(arg)
		}
		cases := make([]mir.DispatchCase, len(op.Cases))
		for i, target := range op.Cases {
			cases[i] = mir.DispatchCase{ClassTag: target.ClassTag, Callee: mir.FunctionID(target.Callee)}
		}
		result.Op = mir.DispatchCall{Args: args, Cases: cases}
	case hir.ArrayNewOp:
		elements := make([]mir.ValueID, len(op.Elements))
		for i, value := range op.Elements {
			elements[i] = mir.ValueID(value)
		}
		switch op.Element {
		case hir.ArrayElementF64:
			result.Op = mir.ArrayNewF64{Elements: elements}
		case hir.ArrayElementBool:
			result.Op = mir.ArrayNewBool{Elements: elements}
		case hir.ArrayElementRef:
			result.Op = mir.ArrayNewRef{Elements: elements}
		default:
			return mir.Instruction{}, fmt.Errorf("unsupported array element specialization %d", op.Element)
		}
	case hir.ArrayLengthOp:
		switch op.Element {
		case hir.ArrayElementF64:
			result.Op = mir.ArrayLengthF64{Array: mir.ValueID(op.Array)}
		case hir.ArrayElementBool:
			result.Op = mir.ArrayLengthBool{Array: mir.ValueID(op.Array)}
		case hir.ArrayElementRef:
			result.Op = mir.ArrayLengthRef{Array: mir.ValueID(op.Array)}
		default:
			return mir.Instruction{}, fmt.Errorf("unsupported array element specialization %d", op.Element)
		}
	case hir.ArrayGetOp:
		switch op.Element {
		case hir.ArrayElementF64:
			result.Op = mir.ArrayGetF64{Array: mir.ValueID(op.Array), Index: mir.ValueID(op.Index)}
		case hir.ArrayElementBool:
			result.Op = mir.ArrayGetBool{Array: mir.ValueID(op.Array), Index: mir.ValueID(op.Index)}
		case hir.ArrayElementRef:
			result.Op = mir.ArrayGetRef{Array: mir.ValueID(op.Array), Index: mir.ValueID(op.Index)}
		default:
			return mir.Instruction{}, fmt.Errorf("unsupported array element specialization %d", op.Element)
		}
	case hir.ArraySetOp:
		switch op.Element {
		case hir.ArrayElementF64:
			result.Op = mir.ArraySetF64{Array: mir.ValueID(op.Array), Index: mir.ValueID(op.Index), Value: mir.ValueID(op.Value)}
		case hir.ArrayElementBool:
			result.Op = mir.ArraySetBool{Array: mir.ValueID(op.Array), Index: mir.ValueID(op.Index), Value: mir.ValueID(op.Value)}
		case hir.ArrayElementRef:
			result.Op = mir.ArraySetRef{Array: mir.ValueID(op.Array), Index: mir.ValueID(op.Index), Value: mir.ValueID(op.Value)}
		default:
			return mir.Instruction{}, fmt.Errorf("unsupported array element specialization %d", op.Element)
		}
	case hir.ObjectNewOp:
		fields := make([]mir.ValueID, len(op.Fields))
		for i, value := range op.Fields {
			fields[i] = mir.ValueID(value)
		}
		result.Op = mir.ObjectNew{Shape: mir.ShapeID(op.Shape), Fields: fields}
	case hir.ObjectAllocOp:
		result.Op = mir.ObjectAlloc{Shape: mir.ShapeID(op.Shape)}
	case hir.FieldSetOp:
		result.Op = mir.FieldSet{Object: mir.ValueID(op.Object), Shape: mir.ShapeID(op.Shape), Field: op.Field, Value: mir.ValueID(op.Value)}
	case hir.FieldGetOp:
		result.Op = mir.FieldGet{Object: mir.ValueID(op.Object), Shape: mir.ShapeID(op.Shape), Field: op.Field}
	case hir.DynamicFieldGetOp:
		result.Op = mir.DynamicFieldGet{Object: mir.ValueID(op.Object), Field: op.Field}
	case hir.DynamicFieldSetOp:
		result.Op = mir.DynamicFieldSet{Object: mir.ValueID(op.Object), Field: op.Field, Value: mir.ValueID(op.Value)}
	case hir.ClosureNewOp:
		captures := make([]mir.ValueID, len(op.Captures))
		for i, capture := range op.Captures {
			captures[i] = mir.ValueID(capture)
		}
		result.Op = mir.ClosureNew{Callee: mir.FunctionID(op.Callee), Captures: captures}
	case hir.ClosureCallOp:
		args := make([]mir.ValueID, len(op.Args))
		for i, arg := range op.Args {
			args[i] = mir.ValueID(arg)
		}
		result.Op = mir.ClosureCall{Closure: mir.ValueID(op.Closure), Args: args}
	case hir.PromiseResolveOp:
		resultRepr, err := lowerTaskResultRepr(op.Result)
		if err != nil {
			return mir.Instruction{}, err
		}
		result.Op = mir.PromiseResolve{Value: mir.ValueID(op.Value), Result: resultRepr}
	case hir.PromiseAdoptOp:
		result.Op = mir.PromiseAdopt{Promise: mir.ValueID(op.Promise)}
	case hir.PromiseThenableOp:
		resultRepr, err := lowerTaskResultRepr(op.Result)
		if err != nil {
			return mir.Instruction{}, err
		}
		cases := make([]mir.DispatchCase, len(op.Cases))
		for i, target := range op.Cases {
			cases[i] = mir.DispatchCase{ClassTag: target.ClassTag, Callee: mir.FunctionID(target.Callee)}
		}
		result.Op = mir.PromiseThenable{Thenable: mir.ValueID(op.Thenable), Result: resultRepr, Arity: op.Arity, Cases: cases, ResolveReturnsJS: op.ResolveReturnsJS, RejectReturnsJS: op.RejectReturnsJS}
	case hir.PromiseAllF64Op:
		promises := make([]mir.ValueID, len(op.Promises))
		for i, value := range op.Promises {
			promises[i] = mir.ValueID(value)
		}
		result.Op = mir.PromiseAllF64{Promises: promises}
	case hir.PromiseRaceF64Op:
		promises := make([]mir.ValueID, len(op.Promises))
		for i, value := range op.Promises {
			promises[i] = mir.ValueID(value)
		}
		result.Op = mir.PromiseRaceF64{Promises: promises}
	case hir.PromiseAllBoolOp:
		promises := make([]mir.ValueID, len(op.Promises))
		for i, value := range op.Promises {
			promises[i] = mir.ValueID(value)
		}
		result.Op = mir.PromiseAllBool{Promises: promises}
	case hir.PromiseRaceBoolOp:
		promises := make([]mir.ValueID, len(op.Promises))
		for i, value := range op.Promises {
			promises[i] = mir.ValueID(value)
		}
		result.Op = mir.PromiseRaceBool{Promises: promises}
	case hir.PromiseAllRefOp:
		promises := make([]mir.ValueID, len(op.Promises))
		for i, value := range op.Promises {
			promises[i] = mir.ValueID(value)
		}
		result.Op = mir.PromiseAllRef{Promises: promises}
	case hir.PromiseRaceRefOp:
		promises := make([]mir.ValueID, len(op.Promises))
		for i, value := range op.Promises {
			promises[i] = mir.ValueID(value)
		}
		result.Op = mir.PromiseRaceRef{Promises: promises}
	case hir.PromiseRejectOp:
		resultRepr, err := lowerTaskResultRepr(op.Result)
		if err != nil {
			return mir.Instruction{}, err
		}
		result.Op = mir.PromiseReject{Reason: mir.ValueID(op.Reason), Result: resultRepr}
	case hir.TaskSpawnOp:
		captures := make([]mir.ValueID, len(op.Captures))
		for i, capture := range op.Captures {
			captures[i] = mir.ValueID(capture)
		}
		var group *mir.ValueID
		if op.Group != nil {
			g := mir.ValueID(*op.Group)
			group = &g
		}
		result.Op = mir.TaskSpawn{Callee: mir.FunctionID(op.Callee), Captures: captures, Group: group}
	case hir.TaskJoinOp:
		result.Op = mir.TaskJoin{Task: mir.ValueID(op.Task), Shared: op.Shared}
	case hir.TaskWaitOp:
		result.Op = mir.TaskWait{Task: mir.ValueID(op.Task)}
	case hir.TaskFailureOp:
		result.Op = mir.TaskFailure{Task: mir.ValueID(op.Task)}
	case hir.TaskRetainOp:
		result.Op = mir.TaskRetain{Task: mir.ValueID(op.Task)}
	case hir.TaskReleaseOp:
		result.Op = mir.TaskRelease{Task: mir.ValueID(op.Task)}
	case hir.TaskYieldOp:
		result.Op = mir.TaskYield{}
	case hir.TaskCancelOp:
		result.Op = mir.TaskCancel{Task: mir.ValueID(op.Task)}
	case hir.TaskCancelledOp:
		result.Op = mir.TaskCancelled{}
	case hir.TaskGroupNewOp:
		result.Op = mir.TaskGroupNew{}
	case hir.TaskGroupJoinOp:
		result.Op = mir.TaskGroupJoin{Group: mir.ValueID(op.Group)}
	case hir.TaskGroupCancelOp:
		result.Op = mir.TaskGroupCancel{Group: mir.ValueID(op.Group)}
	case hir.TaskContextSetOp:
		result.Op = mir.TaskContextSet{Value: mir.ValueID(op.Value)}
	case hir.TaskContextGetOp:
		result.Op = mir.TaskContextGet{}
	case hir.ChannelNewOp:
		switch op.Element {
		case hir.ChannelElementF64:
			result.Op = mir.ChannelNewF64{Capacity: mir.ValueID(op.Capacity)}
		case hir.ChannelElementBool:
			result.Op = mir.ChannelNewBool{Capacity: mir.ValueID(op.Capacity)}
		case hir.ChannelElementRef:
			result.Op = mir.ChannelNewRef{Capacity: mir.ValueID(op.Capacity)}
		default:
			return mir.Instruction{}, fmt.Errorf("invalid channel element kind %d", op.Element)
		}
	case hir.ChannelTrySendOp:
		switch op.Element {
		case hir.ChannelElementF64:
			result.Op = mir.ChannelTrySendF64{Channel: mir.ValueID(op.Channel), Value: mir.ValueID(op.Value)}
		case hir.ChannelElementBool:
			result.Op = mir.ChannelTrySendBool{Channel: mir.ValueID(op.Channel), Value: mir.ValueID(op.Value)}
		case hir.ChannelElementRef:
			result.Op = mir.ChannelTrySendRef{Channel: mir.ValueID(op.Channel), Value: mir.ValueID(op.Value)}
		default:
			return mir.Instruction{}, fmt.Errorf("invalid channel element kind %d", op.Element)
		}
	case hir.ChannelTryRecvOrOp:
		switch op.Element {
		case hir.ChannelElementF64:
			result.Op = mir.ChannelTryRecvOrF64{Channel: mir.ValueID(op.Channel), Fallback: mir.ValueID(op.Fallback)}
		case hir.ChannelElementBool:
			result.Op = mir.ChannelTryRecvOrBool{Channel: mir.ValueID(op.Channel), Fallback: mir.ValueID(op.Fallback)}
		case hir.ChannelElementRef:
			result.Op = mir.ChannelTryRecvOrRef{Channel: mir.ValueID(op.Channel), Fallback: mir.ValueID(op.Fallback)}
		default:
			return mir.Instruction{}, fmt.Errorf("invalid channel element kind %d", op.Element)
		}
	case hir.ChannelSendOp:
		switch op.Element {
		case hir.ChannelElementF64:
			result.Op = mir.ChannelSendF64{Channel: mir.ValueID(op.Channel), Value: mir.ValueID(op.Value)}
		case hir.ChannelElementBool:
			result.Op = mir.ChannelSendBool{Channel: mir.ValueID(op.Channel), Value: mir.ValueID(op.Value)}
		case hir.ChannelElementRef:
			result.Op = mir.ChannelSendRef{Channel: mir.ValueID(op.Channel), Value: mir.ValueID(op.Value)}
		default:
			return mir.Instruction{}, fmt.Errorf("invalid channel element kind %d", op.Element)
		}
	case hir.ChannelRecvOp:
		switch op.Element {
		case hir.ChannelElementF64:
			result.Op = mir.ChannelRecvF64{Channel: mir.ValueID(op.Channel)}
		case hir.ChannelElementBool:
			result.Op = mir.ChannelRecvBool{Channel: mir.ValueID(op.Channel)}
		case hir.ChannelElementRef:
			result.Op = mir.ChannelRecvRef{Channel: mir.ValueID(op.Channel)}
		default:
			return mir.Instruction{}, fmt.Errorf("invalid channel element kind %d", op.Element)
		}
	case hir.SleepOp:
		result.Op = mir.Sleep{Duration: mir.ValueID(op.Duration)}
	case hir.PhiOp:
		incoming := make([]mir.PhiIncoming, len(op.Incoming))
		for i, item := range op.Incoming {
			incoming[i] = mir.PhiIncoming{Block: mir.BlockID(item.Block), Value: mir.ValueID(item.Value)}
		}
		result.Op = mir.Phi{Incoming: incoming}
	case hir.IntrinsicCallOp:
		args := make([]mir.ValueID, len(op.Args))
		for i, arg := range op.Args {
			args[i] = mir.ValueID(arg)
		}
		intrinsic := mir.IntrinsicInvalid
		switch op.Intrinsic {
		case hir.IntrinsicConsoleLogF64:
			intrinsic = mir.IntrinsicConsoleLogF64
		case hir.IntrinsicConsoleLogString:
			intrinsic = mir.IntrinsicConsoleLogString
		case hir.IntrinsicConsoleLogJSValue:
			intrinsic = mir.IntrinsicConsoleLogJSValue
		}
		if intrinsic == mir.IntrinsicInvalid {
			return mir.Instruction{}, fmt.Errorf("unsupported HIR intrinsic %d", op.Intrinsic)
		}
		result.Op = mir.IntrinsicCall{Intrinsic: intrinsic, Args: args}
	default:
		return mir.Instruction{}, fmt.Errorf("unsupported HIR operation %T", source.Op)
	}
	return result, nil
}

func lowerMIRBinary(op hir.BinaryExpr, repr mir.Repr) (mir.Operation, error) {
	left, right := mir.ValueID(op.Left), mir.ValueID(op.Right)
	if repr == mir.ReprBool {
		operator := map[hir.BinaryOperator]mir.FloatCompareOp{
			hir.BinaryLessThan: mir.FloatLessThan, hir.BinaryLessEqual: mir.FloatLessEqual,
			hir.BinaryGreaterThan: mir.FloatGreaterThan, hir.BinaryGreaterEqual: mir.FloatGreaterEqual,
			hir.BinaryEqual: mir.FloatEqual, hir.BinaryNotEqual: mir.FloatNotEqual,
			hir.BinaryStrictEqual: mir.FloatEqual, hir.BinaryStrictNotEqual: mir.FloatNotEqual,
		}[op.Operator]
		if operator == 0 {
			return nil, fmt.Errorf("unsupported boolean binary operator %d", op.Operator)
		}
		return mir.FloatCompare{Operator: operator, Left: left, Right: right}, nil
	}
	if repr == mir.ReprStringRef {
		if op.Operator != hir.BinaryAdd {
			return nil, fmt.Errorf("string binary operator %d is not supported", op.Operator)
		}
		return mir.StringConcat{Left: left, Right: right}, nil
	}
	if repr != mir.ReprF64 {
		return nil, fmt.Errorf("binary result requires unsupported representation %d", repr)
	}
	operator := mir.FloatBinaryOp(0)
	switch op.Operator {
	case hir.BinaryAdd:
		operator = mir.FloatAdd
	case hir.BinarySub:
		operator = mir.FloatSub
	case hir.BinaryMul:
		operator = mir.FloatMul
	case hir.BinaryDiv:
		operator = mir.FloatDiv
	default:
		return nil, fmt.Errorf("unsupported f64 binary operator %d", op.Operator)
	}
	return mir.FloatBinary{Operator: operator, Left: left, Right: right}, nil
}

func lowerMIRTerminator(source hir.Terminator) (mir.Terminator, error) {
	switch term := source.(type) {
	case hir.ReturnTerm:
		if term.Value == nil {
			return mir.Return{}, nil
		}
		value := mir.ValueID(*term.Value)
		return mir.Return{Value: &value}, nil
	case hir.ThrowTerm:
		return mir.Throw{Value: mir.ValueID(term.Value)}, nil
	case hir.JumpTerm:
		return mir.Jump{Target: mir.BlockID(term.Target)}, nil
	case hir.BranchTerm:
		return mir.Branch{Condition: mir.ValueID(term.Condition), Then: mir.BlockID(term.Then), Else: mir.BlockID(term.Else)}, nil
	default:
		return nil, fmt.Errorf("unsupported HIR terminator %T", source)
	}
}

func lowerRepr(source hir.Repr) (mir.Repr, error) {
	switch source.Kind {
	case hir.ReprVoid:
		return mir.ReprVoid, nil
	case hir.ReprBool:
		return mir.ReprBool, nil
	case hir.ReprI32:
		return mir.ReprI32, nil
	case hir.ReprI64:
		return mir.ReprI64, nil
	case hir.ReprF64:
		return mir.ReprF64, nil
	case hir.ReprStringRef:
		return mir.ReprStringRef, nil
	case hir.ReprArrayRef:
		return mir.ReprArrayRef, nil
	case hir.ReprObjectRef:
		return mir.ReprObjectRef, nil
	case hir.ReprFunctionRef:
		return mir.ReprFunctionRef, nil
	case hir.ReprTaskRef:
		return mir.ReprTaskRef, nil
	case hir.ReprChannelRef:
		return mir.ReprChannelRef, nil
	case hir.ReprTaskGroupRef:
		return mir.ReprTaskGroupRef, nil
	case hir.ReprTaggedUnion:
		return mir.ReprTagged, nil
	case hir.ReprJSValue:
		return mir.ReprJSValue, nil
	case hir.ReprUnproven:
		return mir.ReprInvalid, fmt.Errorf("representation is not proven")
	default:
		return mir.ReprInvalid, fmt.Errorf("unknown HIR representation %d", source.Kind)
	}
}

func provenIntegerWidth(result hir.ValueID, op hir.BinaryExpr, repr mir.Repr, ranges rangeanalysis.FunctionResult) (mir.IntWidth, bool) {
	if repr != mir.ReprF64 || ranges == nil {
		return 0, false
	}
	switch op.Operator {
	case hir.BinaryAdd, hir.BinarySub, hir.BinaryMul, hir.BinaryDiv:
	default:
		return 0, false
	}
	out, ook := ranges[result]
	left, lok := ranges[op.Left]
	right, rok := ranges[op.Right]
	if !ook || !lok || !rok || !out.Known || !left.Known || !right.Known {
		return 0, false
	}
	if out.FitsI32() && left.FitsI32() && right.FitsI32() {
		return mir.IntWidth32, true
	}
	if out.FitsI64() && left.FitsI64() && right.FitsI64() {
		return mir.IntWidth64, true
	}
	return 0, false
}

func lowerIntegerBinaryOperator(op hir.BinaryOperator) (mir.FloatBinaryOp, error) {
	switch op {
	case hir.BinaryAdd:
		return mir.FloatAdd, nil
	case hir.BinarySub:
		return mir.FloatSub, nil
	case hir.BinaryMul:
		return mir.FloatMul, nil
	case hir.BinaryDiv:
		return mir.FloatDiv, nil
	default:
		return 0, fmt.Errorf("unsupported proven integer operator %d", op)
	}
}
