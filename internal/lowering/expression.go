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
		return f.emit(expr.Type, hir.ConstOp{Literal: hir.Literal{
			Kind: hir.LiteralNumber, Number: expr.Number,
		}}), nil
	case frontend.ExprBinary:
		return f.lowerBinary(expr)
	case frontend.ExprCall:
		return f.lowerCall(expr)
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
	case frontend.BinaryLessEqual:
		op = hir.BinaryLessEqual
	default:
		return 0, fmt.Errorf("unsupported semantic binary operator %d", expr.Operator)
	}
	return f.emit(expr.Type, hir.BinaryExpr{Operator: op, Left: left, Right: right}), nil
}

func (f *functionLowerer) lowerCall(expr *frontend.Expr) (hir.ValueID, error) {
	args := make([]hir.ValueID, 0, len(expr.Args))
	for _, arg := range expr.Args {
		value, err := f.lowerExpr(arg)
		if err != nil {
			return 0, err
		}
		args = append(args, value)
	}
	if expr.Intrinsic != frontend.IntrinsicNone {
		intrinsic := hir.IntrinsicInvalid
		switch expr.Intrinsic {
		case frontend.IntrinsicConsoleLogF64:
			intrinsic = hir.IntrinsicConsoleLogF64
		default:
			return 0, fmt.Errorf("unsupported semantic intrinsic %d", expr.Intrinsic)
		}
		return f.emit(expr.Type, hir.IntrinsicCallOp{Intrinsic: intrinsic, Args: args}), nil
	}
	if expr.CallTarget == nil {
		return 0, fmt.Errorf("dynamic call is not supported by HIR MVP")
	}
	return f.emit(expr.Type, hir.CallOp{
		Callee: hir.NewFunctionID(uint32(*expr.CallTarget)),
		Args:   args,
	}), nil
}
