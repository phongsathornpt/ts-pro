package frontend

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/tsast"
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
		if len(expr.Args) != 1 || int(expr.Args[0].Type) >= len(e.result.Types) || (e.result.Types[expr.Args[0].Type].Kind != TypeTask && e.result.Types[expr.Args[0].Type].Kind != TypePromise) {
			return nil, fmt.Errorf("join at %d requires exactly one TsnativeTask", node.Pos())
		}
		taskType := e.result.Types[expr.Args[0].Type]
		if !e.compatibleTaskResult(taskType.ReturnType, expr.Type) {
			return nil, fmt.Errorf("join at %d has inconsistent task result type", node.Pos())
		}
		expr.Kind = ExprTaskJoin
		expr.Callee = nil
		return expr, nil
	case "cancelTask":
		if len(expr.Args) != 1 || int(expr.Args[0].Type) >= len(e.result.Types) || (e.result.Types[expr.Args[0].Type].Kind != TypeTask && e.result.Types[expr.Args[0].Type].Kind != TypePromise) || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeVoid {
			return nil, fmt.Errorf("cancelTask at %d requires one native task and returns void", node.Pos())
		}
		expr.Kind = ExprTaskCancel
		expr.Callee = nil
		return expr, nil
	case "taskCancelled":
		if len(expr.Args) != 0 || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeBoolean {
			return nil, fmt.Errorf("taskCancelled at %d takes no arguments and returns boolean", node.Pos())
		}
		expr.Kind = ExprTaskCancelled
		expr.Callee = nil
		return expr, nil
	case "taskGroup":
		if len(expr.Args) != 0 || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeTaskGroup {
			return nil, fmt.Errorf("taskGroup at %d takes no arguments and returns TsnativeTaskGroup", node.Pos())
		}
		expr.Kind = ExprTaskGroupNew
		expr.Callee = nil
		return expr, nil
	case "groupSpawn":
		if len(expr.Args) != 2 || int(expr.Args[0].Type) >= len(e.result.Types) || e.result.Types[expr.Args[0].Type].Kind != TypeTaskGroup {
			return nil, fmt.Errorf("groupSpawn at %d requires (TsnativeTaskGroup, closure)", node.Pos())
		}
		closure := expr.Args[1]
		if closure.Kind != ExprClosure || closure.CallTarget == nil || int(*closure.CallTarget) >= len(e.result.Functions) {
			return nil, fmt.Errorf("groupSpawn at %d requires a statically known closure", node.Pos())
		}
		target := e.result.Functions[*closure.CallTarget]
		if len(target.Params) != len(closure.Captures) || int(target.ReturnType) >= len(e.result.Types) {
			return nil, fmt.Errorf("groupSpawn at %d requires a supported zero-argument closure", node.Pos())
		}
		returnKind := e.result.Types[target.ReturnType].Kind
		if returnKind != TypeVoid && returnKind != TypeNumber && returnKind != TypeString && returnKind != TypeBoolean && returnKind != TypeObject && returnKind != TypeArray && returnKind != TypeFunction && returnKind != TypeAny {
			return nil, fmt.Errorf("groupSpawn at %d does not support task result type %q", node.Pos(), e.result.Types[target.ReturnType].Name)
		}
		if int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeTask || !e.compatibleTaskResult(e.result.Types[expr.Type].ReturnType, target.ReturnType) {
			return nil, fmt.Errorf("groupSpawn at %d has inconsistent task result type", node.Pos())
		}
		targetCopy := *closure.CallTarget
		expr.Kind = ExprTaskSpawn
		expr.CallTarget = &targetCopy
		expr.Captures = closure.Captures
		expr.Object = expr.Args[0]
		expr.Callee = nil
		expr.Args = nil
		return expr, nil
	case "groupJoin":
		if len(expr.Args) != 1 || int(expr.Args[0].Type) >= len(e.result.Types) || e.result.Types[expr.Args[0].Type].Kind != TypeTaskGroup || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeVoid {
			return nil, fmt.Errorf("groupJoin at %d requires one TsnativeTaskGroup and returns void", node.Pos())
		}
		expr.Kind = ExprTaskGroupJoin
		expr.Callee = nil
		return expr, nil
	case "groupCancel":
		if len(expr.Args) != 1 || int(expr.Args[0].Type) >= len(e.result.Types) || e.result.Types[expr.Args[0].Type].Kind != TypeTaskGroup || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeVoid {
			return nil, fmt.Errorf("groupCancel at %d requires one TsnativeTaskGroup and returns void", node.Pos())
		}
		expr.Kind = ExprTaskGroupCancel
		expr.Callee = nil
		return expr, nil
	case "setTaskContext":
		if len(expr.Args) != 1 || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeVoid {
			return nil, fmt.Errorf("setTaskContext at %d requires one value and returns void", node.Pos())
		}
		e.ensureSemanticType(TypeAny, "any")
		expr.Kind = ExprTaskContextSet
		expr.Callee = nil
		return expr, nil
	case "taskContext":
		if len(expr.Args) != 0 || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeAny {
			return nil, fmt.Errorf("taskContext at %d takes no arguments and returns any", node.Pos())
		}
		expr.Kind = ExprTaskContextGet
		expr.Callee = nil
		return expr, nil
	case "channel":
		if len(expr.Args) != 1 || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeChannel {
			return nil, fmt.Errorf("channel at %d requires one capacity and a concrete native channel type", node.Pos())
		}
		if _, _, ok := e.channelElement(expr.Type); !ok {
			return nil, fmt.Errorf("channel at %d uses an unsupported element type", node.Pos())
		}
		if int(expr.Args[0].Type) >= len(e.result.Types) || e.result.Types[expr.Args[0].Type].Kind != TypeNumber {
			return nil, fmt.Errorf("channel capacity at %d must be number", node.Pos())
		}
		expr.Kind = ExprChannelNew
		expr.Callee = nil
		return expr, nil
	case "channelTrySend":
		if len(expr.Args) != 2 || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeBoolean {
			return nil, fmt.Errorf("channelTrySend at %d requires (channel<T>, T) and returns boolean", node.Pos())
		}
		element, _, ok := e.channelElement(expr.Args[0].Type)
		if !ok || !e.channelValueCompatible(element, expr.Args[1].Type) {
			return nil, fmt.Errorf("channelTrySend at %d has an unsupported or incompatible channel element", node.Pos())
		}
		expr.Kind = ExprChannelTrySend
		expr.Callee = nil
		return expr, nil
	case "channelTryRecvOr":
		if len(expr.Args) != 2 {
			return nil, fmt.Errorf("channelTryRecvOr at %d requires (channel<T>, T)", node.Pos())
		}
		element, _, ok := e.channelElement(expr.Args[0].Type)
		if !ok || !e.channelValueCompatible(element, expr.Args[1].Type) || !e.channelValueCompatible(element, expr.Type) {
			return nil, fmt.Errorf("channelTryRecvOr at %d has an unsupported or incompatible channel element", node.Pos())
		}
		expr.Kind = ExprChannelTryRecvOr
		expr.Callee = nil
		return expr, nil
	case "channelSend":
		if len(expr.Args) != 2 || int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypeVoid {
			return nil, fmt.Errorf("channelSend at %d requires (channel<T>, T) and returns void", node.Pos())
		}
		element, _, ok := e.channelElement(expr.Args[0].Type)
		if !ok || !e.channelValueCompatible(element, expr.Args[1].Type) {
			return nil, fmt.Errorf("channelSend at %d has an unsupported or incompatible channel element", node.Pos())
		}
		expr.Kind = ExprChannelSend
		expr.Callee = nil
		return expr, nil
	case "channelRecv":
		if len(expr.Args) != 1 {
			return nil, fmt.Errorf("channelRecv at %d requires channel<T>", node.Pos())
		}
		element, _, ok := e.channelElement(expr.Args[0].Type)
		if !ok || !e.channelValueCompatible(element, expr.Type) {
			return nil, fmt.Errorf("channelRecv at %d has an unsupported or incompatible channel result", node.Pos())
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

func (e *extractor) channelElement(typeID TypeID) (TypeID, TypeKind, bool) {
	if int(typeID) >= len(e.result.Types) {
		return 0, TypeInvalid, false
	}
	typ := e.result.Types[typeID]
	if typ.Kind != TypeChannel || int(typ.Element) >= len(e.result.Types) {
		return 0, TypeInvalid, false
	}
	kind := e.result.Types[typ.Element].Kind
	switch kind {
	case TypeNumber, TypeBoolean, TypeString, TypeObject, TypeArray, TypeFunction, TypeAny, TypeUnion, TypeNull, TypeUndefined:
		return typ.Element, kind, true
	default:
		return typ.Element, kind, false
	}
}

func (e *extractor) channelValueCompatible(element, value TypeID) bool {
	if element == value {
		return true
	}
	if int(element) >= len(e.result.Types) || int(value) >= len(e.result.Types) {
		return false
	}
	elementKind, valueKind := e.result.Types[element].Kind, e.result.Types[value].Kind
	if elementKind == TypeAny || elementKind == TypeUnion {
		return true
	}
	return elementKind == valueKind
}

func isConcurrencyIntrinsic(name string) bool {
	switch name {
	case "spawn", "join", "yieldNow", "cancelTask", "taskCancelled", "taskGroup", "groupSpawn", "groupJoin", "groupCancel", "setTaskContext", "taskContext", "channel", "channelTrySend", "channelTryRecvOr", "channelSend", "channelRecv", "sleep":
		return true
	default:
		return false
	}
}
