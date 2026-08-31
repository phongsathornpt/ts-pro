package lowering

import (
	"fmt"

	"github.com/projectthorn/tsv7-bin/internal/hir"
	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func LowerMIR(source hir.Module) (mir.Module, error) {
	result := mir.Module{Name: source.Name}
	for _, fn := range source.Functions {
		lowered, err := lowerMIRFunction(fn)
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

func lowerMIRFunction(source hir.Function) (mir.Function, error) {
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
			inst, err := lowerMIRInstruction(instruction)
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

func lowerMIRInstruction(source hir.Instruction) (mir.Instruction, error) {
	repr, err := lowerRepr(source.Repr)
	if err != nil {
		return mir.Instruction{}, fmt.Errorf("value v%d: %w", source.Result, err)
	}
	result := mir.Instruction{Result: mir.ValueID(source.Result), Repr: repr}
	switch op := source.Op.(type) {
	case hir.ConstOp:
		if op.Literal.Kind != hir.LiteralNumber || repr != mir.ReprF64 {
			return mir.Instruction{}, fmt.Errorf("unsupported const representation %v", repr)
		}
		result.Op = mir.ConstF64{Value: op.Literal.Number}
	case hir.BinaryExpr:
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
	default:
		return mir.Instruction{}, fmt.Errorf("unsupported HIR operation %T", source.Op)
	}
	return result, nil
}

func lowerMIRBinary(op hir.BinaryExpr, repr mir.Repr) (mir.Operation, error) {
	left, right := mir.ValueID(op.Left), mir.ValueID(op.Right)
	if repr == mir.ReprBool {
		if op.Operator != hir.BinaryLessEqual {
			return nil, fmt.Errorf("unsupported boolean binary operator %d", op.Operator)
		}
		return mir.FloatCompare{Operator: mir.FloatLessEqual, Left: left, Right: right}, nil
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
