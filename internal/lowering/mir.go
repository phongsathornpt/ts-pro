package lowering

import (
	"fmt"

	rangeanalysis "github.com/projectthorn/tsv7-bin/internal/analysis/range"
	"github.com/projectthorn/tsv7-bin/internal/hir"
	"github.com/projectthorn/tsv7-bin/internal/mir"
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
			lowered.Fields = append(lowered.Fields, mir.ShapeField{Name: field.Name, Repr: repr})
		}
		result.Shapes = append(result.Shapes, lowered)
	}
	if source.Entry != nil {
		entry := mir.FunctionID(*source.Entry)
		result.Entry = &entry
	}
	for _, fn := range source.Functions {
		lowered, err := lowerMIRFunction(fn, ranges[fn.ID])
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

func lowerMIRFunction(source hir.Function, ranges rangeanalysis.FunctionResult) (mir.Function, error) {
	returnRepr, err := lowerRepr(source.ReturnRepr)
	if err != nil {
		return mir.Function{}, fmt.Errorf("function %s return: %w", source.Name, err)
	}
	result := mir.Function{
		ID: mir.FunctionID(source.ID), Name: source.Name,
		ReturnRepr: returnRepr, Entry: mir.BlockID(source.Entry),
	}
	for _, param := range source.Params {
		repr, err := lowerRepr(param.Repr)
		if err != nil {
			return mir.Function{}, fmt.Errorf("function %s parameter %s: %w", source.Name, param.Name, err)
		}
		result.Params = append(result.Params, mir.Param{Value: mir.ValueID(param.Value), Name: param.Name, Repr: repr})
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

func lowerMIRInstruction(source hir.Instruction, ranges rangeanalysis.FunctionResult) (mir.Instruction, error) {
	repr, err := lowerRepr(source.Repr)
	if err != nil {
		return mir.Instruction{}, fmt.Errorf("value v%d: %w", source.Result, err)
	}
	result := mir.Instruction{Result: mir.ValueID(source.Result), Repr: repr}
	switch op := source.Op.(type) {
	case hir.ConstOp:
		switch {
		case op.Literal.Kind == hir.LiteralNumber && repr == mir.ReprF64:
			result.Op = mir.ConstF64{Value: op.Literal.Number}
		case op.Literal.Kind == hir.LiteralString && repr == mir.ReprStringRef:
			result.Op = mir.ConstString{Value: op.Literal.String}
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
		result.Op = mir.ArrayNewF64{Elements: elements}
	case hir.ArrayLengthOp:
		result.Op = mir.ArrayLengthF64{Array: mir.ValueID(op.Array)}
	case hir.ArrayGetOp:
		result.Op = mir.ArrayGetF64{Array: mir.ValueID(op.Array), Index: mir.ValueID(op.Index)}
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
