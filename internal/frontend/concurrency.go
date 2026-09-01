package frontend

import (
	"fmt"

	"github.com/projectthorn/tsv7-bin/internal/tsast"
)

func (e *extractor) extractConcurrencyCall(node tsast.Node, expr *Expr, name string) (*Expr, error) {
	switch name {
	case "spawn":
		if len(expr.Args) != 1 {
			return nil, fmt.Errorf("spawn at %d requires exactly one function", node.Pos())
		}
		closure := expr.Args[0]
		if closure.Kind != ExprClosure || closure.CallTarget == nil {
			return nil, fmt.Errorf("spawn at %d requires a statically known closure", node.Pos())
		}
		if int(*closure.CallTarget) >= len(e.result.Functions) {
			return nil, fmt.Errorf("spawn at %d references invalid function", node.Pos())
		}
		target := e.result.Functions[*closure.CallTarget]
		if len(target.Params) != len(closure.Captures) || int(target.ReturnType) >= len(e.result.Types) {
			return nil, fmt.Errorf("spawn at %d requires a zero-argument closure with a supported result", node.Pos())
		}
		returnKind := e.result.Types[target.ReturnType].Kind
		if returnKind != TypeVoid && returnKind != TypeNumber && returnKind != TypeString && returnKind != TypeBoolean && returnKind != TypeObject && returnKind != TypeArray && returnKind != TypeFunction && returnKind != TypeAny {
			return nil, fmt.Errorf("spawn at %d does not support task result type %q yet", node.Pos(), e.result.Types[target.ReturnType].Name)
		}
		if int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeTask || !e.compatibleTaskResult(e.result.Types[expr.Type].ReturnType, target.ReturnType) {
			return nil, fmt.Errorf("spawn at %d has inconsistent task result type", node.Pos())
		}
		targetCopy := *closure.CallTarget
		expr.Kind = ExprTaskSpawn
		expr.CallTarget = &targetCopy
		expr.Captures = closure.Captures
		expr.Callee = nil
		expr.Args = nil
		return expr, nil
	case "join":
		if len(expr.Args) != 1 || int(expr.Args[0].Type) >= len(e.result.Types) || e.result.Types[expr.Args[0].Type].Kind != TypeTask {
			return nil, fmt.Errorf("join at %d requires exactly one TsnativeTask", node.Pos())
		}
		taskType := e.result.Types[expr.Args[0].Type]
		if !e.compatibleTaskResult(taskType.ReturnType, expr.Type) {
			return nil, fmt.Errorf("join at %d has inconsistent task result type", node.Pos())
		}
		expr.Kind = ExprTaskJoin
		expr.Callee = nil
		return expr, nil
	case "channel":
		if len(expr.Args) != 1 || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeChannel {
			return nil, fmt.Errorf("channel at %d requires one capacity and a concrete native channel type", node.Pos())
		}
		channelType := e.result.Types[expr.Type]
		if int(channelType.Element) >= len(e.result.Types) || e.result.Types[channelType.Element].Kind != TypeNumber {
			return nil, fmt.Errorf("channel at %d currently supports only channel<number>", node.Pos())
		}
		if int(expr.Args[0].Type) >= len(e.result.Types) || e.result.Types[expr.Args[0].Type].Kind != TypeNumber {
			return nil, fmt.Errorf("channel capacity at %d must be number", node.Pos())
		}
		expr.Kind = ExprChannelNew
		expr.Callee = nil
		return expr, nil
	case "channelTrySend":
		if len(expr.Args) != 2 || !e.isNumberChannel(expr.Args[0].Type) || e.result.Types[expr.Args[1].Type].Kind != TypeNumber || e.result.Types[expr.Type].Kind != TypeBoolean {
			return nil, fmt.Errorf("channelTrySend at %d requires (channel<number>, number) and returns boolean", node.Pos())
		}
		expr.Kind = ExprChannelTrySend
		expr.Callee = nil
		return expr, nil
	case "channelTryRecvOr":
		if len(expr.Args) != 2 || !e.isNumberChannel(expr.Args[0].Type) || e.result.Types[expr.Args[1].Type].Kind != TypeNumber || e.result.Types[expr.Type].Kind != TypeNumber {
			return nil, fmt.Errorf("channelTryRecvOr at %d requires (channel<number>, number) and returns number", node.Pos())
		}
		expr.Kind = ExprChannelTryRecvOr
		expr.Callee = nil
		return expr, nil
	case "channelSend":
		if len(expr.Args) != 2 || !e.isNumberChannel(expr.Args[0].Type) || e.result.Types[expr.Args[1].Type].Kind != TypeNumber || e.result.Types[expr.Type].Kind != TypeVoid {
			return nil, fmt.Errorf("channelSend at %d requires (channel<number>, number) and returns void", node.Pos())
		}
		expr.Kind = ExprChannelSend
		expr.Callee = nil
		return expr, nil
	case "channelRecv":
		if len(expr.Args) != 1 || !e.isNumberChannel(expr.Args[0].Type) || e.result.Types[expr.Type].Kind != TypeNumber {
			return nil, fmt.Errorf("channelRecv at %d requires channel<number> and returns number", node.Pos())
		}
		expr.Kind = ExprChannelRecv
		expr.Callee = nil
		return expr, nil
	case "sleep":
		if len(expr.Args) != 1 || int(expr.Args[0].Type) >= len(e.result.Types) || e.result.Types[expr.Args[0].Type].Kind != TypeNumber || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeVoid {
			return nil, fmt.Errorf("sleep at %d requires one number duration and returns void", node.Pos())
		}
		expr.Kind = ExprSleep
		expr.Callee = nil
		return expr, nil
	case "yieldNow":
		if len(expr.Args) != 0 {
			return nil, fmt.Errorf("yieldNow at %d takes no arguments", node.Pos())
		}
		expr.Kind = ExprTaskYield
		expr.Callee = nil
		return expr, nil
	default:
		return nil, fmt.Errorf("unknown concurrency intrinsic %q", name)
	}
}

func (e *extractor) compatibleTaskResult(left, right TypeID) bool {
	if left == right {
		return true
	}
	if int(left) >= len(e.result.Types) || int(right) >= len(e.result.Types) {
		return false
	}
	l, r := e.result.Types[left], e.result.Types[right]
	if l.Kind != r.Kind {
		return false
	}
	switch l.Kind {
	case TypeVoid, TypeNumber, TypeString, TypeBoolean, TypeNull, TypeUndefined:
		return true
	default:
		return false
	}
}

func (e *extractor) isNumberChannel(typeID TypeID) bool {
	if int(typeID) >= len(e.result.Types) {
		return false
	}
	typ := e.result.Types[typeID]
	return typ.Kind == TypeChannel && int(typ.Element) < len(e.result.Types) && e.result.Types[typ.Element].Kind == TypeNumber
}
