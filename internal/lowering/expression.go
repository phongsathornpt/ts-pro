package lowering

import (
	"fmt"

	"github.com/projectthorn/tsv7-bin/internal/frontend"
	"github.com/projectthorn/tsv7-bin/internal/hir"
)

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
	case frontend.ExprNumber:
		return f.emit(expr.Type, hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralNumber, Number: expr.Number}}), nil
	case frontend.ExprString:
		return f.emit(expr.Type, hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralString, String: expr.String}}), nil
	case frontend.ExprBinary:
		return f.lowerBinary(expr)
	case frontend.ExprCall:
		return f.lowerCall(expr)
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
	case frontend.ExprClosure:
		if expr.CallTarget == nil {
			return 0, fmt.Errorf("closure value has no native target")
		}
		captures := make([]hir.ValueID, 0, len(expr.Captures))
		for _, capture := range expr.Captures {
			value, err := f.lowerExpr(capture)
			if err != nil {
				return 0, err
			}
			captures = append(captures, value)
		}
		return f.emit(expr.Type, hir.ClosureNewOp{Callee: hir.NewFunctionID(uint32(*expr.CallTarget)), Captures: captures}), nil
	default:
		return 0, fmt.Errorf("unsupported semantic expression kind %d", expr.Kind)
	}
}

func (f *functionLowerer) lowerBinary(expr *frontend.Expr) (hir.ValueID, error) {
	left, err := f.lowerExpr(expr.Left)
	if err != nil {
		return 0, err
	}
	right, err := f.lowerExpr(expr.Right)
	if err != nil {
		return 0, err
	}
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
	default:
		return 0, fmt.Errorf("unsupported semantic binary operator %d", expr.Operator)
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
		default:
			return 0, fmt.Errorf("unsupported semantic intrinsic %d", expr.Intrinsic)
		}
		return f.emit(expr.Type, hir.IntrinsicCallOp{Intrinsic: intrinsic, Args: args}), nil
	}
	if expr.CallTarget != nil {
		args, err := lowerArgs()
		if err != nil {
			return 0, err
		}
		return f.emit(expr.Type, hir.CallOp{Callee: hir.NewFunctionID(uint32(*expr.CallTarget)), Args: args}), nil
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
