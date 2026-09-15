package sema

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) checkCallExpr(e *ast.CallExpr) types.Type {
	if member, ok := e.Callee.(*ast.MemberExpr); ok {
		if result, handled := c.checkWebCryptoCall(e, member); handled {
			return result
		}
		if result, handled := c.checkPromiseStaticCall(e, member); handled {
			return result
		}
	}
	if ident, ok := e.Callee.(*ast.IdentExpr); ok {
		switch ident.Name {
		case "setTimeout", "setInterval":
			if len(e.Args) < 1 || len(e.Args) > 2 {
				c.error(e.Span(), "TS2554", ident.Name+" expects a callback and optional delay.")
			} else {
				if fn, ok := c.checkExpr(e.Args[0]).(*types.FunctionType); !ok || len(fn.Params) != 0 {
					c.error(e.Args[0].Span(), "TS2345", ident.Name+" expects a zero-argument function.")
				}
				if len(e.Args) == 2 && c.checkExpr(e.Args[1]) != types.TypeNumber {
					c.error(e.Args[1].Span(), "TS2345", ident.Name+" delay must be a number.")
				}
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeNumber
			return types.TypeNumber
		case "clearTimeout", "clearInterval":
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", ident.Name+" expects one timer id.")
			} else if c.checkExpr(e.Args[0]) != types.TypeNumber {
				c.error(e.Args[0].Span(), "TS2345", ident.Name+" expects a numeric timer id.")
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeVoid
			return types.TypeVoid
		case "structuredClone":
			if len(e.Args) < 1 || len(e.Args) > 2 {
				c.error(e.Span(), "TS2554", "structuredClone expects a value and optional options.")
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			resultType := c.checkExpr(e.Args[0])
			if len(e.Args) == 2 {
				c.checkExpr(e.Args[1])
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = resultType
			return resultType
		case "queueMicrotask":
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", "queueMicrotask expects exactly one callback.")
			} else if fn, ok := c.checkExpr(e.Args[0]).(*types.FunctionType); !ok || len(fn.Params) != 0 {
				c.error(e.Args[0].Span(), "TS2345", "queueMicrotask expects a zero-argument function.")
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeVoid
			return types.TypeVoid
		case "atob", "btoa":
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", ident.Name+" expects exactly one string argument.")
			} else if argType := c.checkExpr(e.Args[0]); argType != types.TypeString {
				c.error(e.Args[0].Span(), "TS2345", ident.Name+" expects a string.")
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeString
			return types.TypeString
		case "taskGroup":
			if len(e.Args) != 0 {
				c.error(e.Span(), "TS2554", "taskGroup expects no arguments.")
			}
			if c.result.TaskGroupType == nil {
				c.result.TaskGroupType = types.NewObject("$TaskGroup")
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = c.result.TaskGroupType
			return c.result.TaskGroupType
		case "groupSpawn":
			if len(e.Args) != 2 {
				c.error(e.Span(), "TS2554", "groupSpawn expects group and zero-argument function.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			groupType := c.checkExpr(e.Args[0])
			if c.result.TaskGroupType == nil || !groupType.Equals(c.result.TaskGroupType) {
				c.error(e.Args[0].Span(), "TS2345", "groupSpawn expects a task group.")
			}
			fnType, ok := c.checkExpr(e.Args[1]).(*types.FunctionType)
			if !ok || len(fnType.Params) != 0 {
				c.error(e.Args[1].Span(), "TS2345", "groupSpawn expects a zero-argument function.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			name := fmt.Sprintf("$Task$%d", c.taskTypeCount)
			c.taskTypeCount++
			taskType := types.NewObject(name)
			c.result.TaskResults[name] = fnType.Return
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = taskType
			return taskType
		case "groupJoin", "groupCancel":
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", ident.Name+" expects one task group.")
			} else {
				groupType := c.checkExpr(e.Args[0])
				if c.result.TaskGroupType == nil || !groupType.Equals(c.result.TaskGroupType) {
					c.error(e.Args[0].Span(), "TS2345", ident.Name+" expects a task group.")
				}
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeVoid
			return types.TypeVoid
		case "channel":
			if len(e.TypeArgs) != 1 {
				c.error(e.Span(), "TS2558", "channel expects exactly one type argument.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", "channel expects exactly one capacity argument.")
			} else if capType := c.checkExpr(e.Args[0]); !capType.AssignableTo(types.TypeNumber) {
				c.error(e.Args[0].Span(), "TS2345", "channel capacity must be a number.")
			}
			elem := c.resolveTypeNode(e.TypeArgs[0])
			name := fmt.Sprintf("$Channel$%d", c.channelTypeCount)
			c.channelTypeCount++
			channelType := types.NewObject(name)
			c.result.ChannelElements[name] = elem
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = channelType
			return channelType
		case "channelSend":
			if len(e.Args) != 2 {
				c.error(e.Span(), "TS2554", "channelSend expects channel and value.")
				c.result.Types[e] = types.TypeVoid
				return types.TypeVoid
			}
			chType := c.checkExpr(e.Args[0])
			obj, ok := chType.(*types.ObjectType)
			elem, known := types.Type(nil), false
			if ok {
				elem, known = c.result.ChannelElements[obj.Name]
			}
			valueType := c.checkExpr(e.Args[1])
			if !known {
				c.error(e.Args[0].Span(), "TS2345", "channelSend expects a channel handle.")
			} else if !valueType.AssignableTo(elem) {
				c.error(e.Args[1].Span(), "TS2345", fmt.Sprintf("Type '%s' is not assignable to channel element type '%s'.", valueType, elem))
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeVoid
			return types.TypeVoid
		case "channelRecv":
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", "channelRecv expects one channel.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			chType := c.checkExpr(e.Args[0])
			obj, ok := chType.(*types.ObjectType)
			elem, known := types.Type(nil), false
			if ok {
				elem, known = c.result.ChannelElements[obj.Name]
			}
			if !known {
				c.error(e.Args[0].Span(), "TS2345", "channelRecv expects a channel handle.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = elem
			return elem
		case "channelTrySend":
			if len(e.Args) != 2 {
				c.error(e.Span(), "TS2554", "channelTrySend expects channel and value.")
				c.result.Types[e] = types.TypeBoolean
				return types.TypeBoolean
			}
			chType := c.checkExpr(e.Args[0])
			obj, ok := chType.(*types.ObjectType)
			elem, known := types.Type(nil), false
			if ok {
				elem, known = c.result.ChannelElements[obj.Name]
			}
			valueType := c.checkExpr(e.Args[1])
			if !known {
				c.error(e.Args[0].Span(), "TS2345", "channelTrySend expects a channel handle.")
			} else if !valueType.AssignableTo(elem) {
				c.error(e.Args[1].Span(), "TS2345", fmt.Sprintf("Type '%s' is not assignable to channel element type '%s'.", valueType, elem))
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeBoolean
			return types.TypeBoolean
		case "channelTryRecvOr":
			if len(e.Args) != 2 {
				c.error(e.Span(), "TS2554", "channelTryRecvOr expects channel and fallback.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			chType := c.checkExpr(e.Args[0])
			obj, ok := chType.(*types.ObjectType)
			elem, known := types.Type(nil), false
			if ok {
				elem, known = c.result.ChannelElements[obj.Name]
			}
			fallbackType := c.checkExpr(e.Args[1])
			if !known {
				c.error(e.Args[0].Span(), "TS2345", "channelTryRecvOr expects a channel handle.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			if !fallbackType.AssignableTo(elem) {
				c.error(e.Args[1].Span(), "TS2345", fmt.Sprintf("Fallback type '%s' is not assignable to channel element type '%s'.", fallbackType, elem))
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = elem
			return elem
		case "spawn":
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", "spawn expects exactly one zero-argument function.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			fnType, ok := c.checkExpr(e.Args[0]).(*types.FunctionType)
			if !ok {
				c.error(e.Args[0].Span(), "TS2345", "spawn expects a function value.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			if len(fnType.Params) != 0 {
				c.error(e.Args[0].Span(), "TS2345", "spawn currently requires a zero-argument function.")
			}
			name := fmt.Sprintf("$Task$%d", c.taskTypeCount)
			c.taskTypeCount++
			taskType := types.NewObject(name)
			c.result.TaskResults[name] = fnType.Return
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = taskType
			return taskType
		case "yieldNow":
			if len(e.Args) != 0 {
				c.error(e.Span(), "TS2554", "yieldNow expects no arguments.")
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeVoid
			return types.TypeVoid
		case "sleep":
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", "sleep expects exactly one millisecond argument.")
			} else if argType := c.checkExpr(e.Args[0]); !argType.AssignableTo(types.TypeNumber) {
				c.error(e.Args[0].Span(), "TS2345", "sleep expects a number of milliseconds.")
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeVoid
			return types.TypeVoid
		case "setTaskContext":
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", "setTaskContext expects one string argument.")
			} else if argType := c.checkExpr(e.Args[0]); !argType.AssignableTo(types.TypeString) {
				c.error(e.Args[0].Span(), "TS2345", "setTaskContext expects a string.")
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeVoid
			return types.TypeVoid
		case "taskContext":
			if len(e.Args) != 0 {
				c.error(e.Span(), "TS2554", "taskContext expects no arguments.")
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeString
			return types.TypeString
		case "cancelTask":
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", "cancelTask expects exactly one task.")
			} else {
				taskType := c.checkExpr(e.Args[0])
				obj, ok := taskType.(*types.ObjectType)
				if !ok {
					c.error(e.Args[0].Span(), "TS2345", "cancelTask expects a task handle.")
				} else if _, known := c.result.TaskResults[obj.Name]; !known {
					c.error(e.Args[0].Span(), "TS2345", "cancelTask received an unknown task handle type.")
				}
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeVoid
			return types.TypeVoid
		case "taskCancelled":
			if len(e.Args) != 0 {
				c.error(e.Span(), "TS2554", "taskCancelled expects no arguments.")
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = types.TypeBoolean
			return types.TypeBoolean
		case "join":
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", "join expects exactly one task.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			taskType := c.checkExpr(e.Args[0])
			obj, ok := taskType.(*types.ObjectType)
			if !ok {
				c.error(e.Args[0].Span(), "TS2345", "join expects a task handle.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			resultType, ok := c.result.TaskResults[obj.Name]
			if !ok {
				c.error(e.Args[0].Span(), "TS2345", "join received an unknown task handle type.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			c.result.Types[e.Callee] = types.TypeAny
			c.result.Types[e] = resultType
			return resultType
		}
	}
	calleeType := c.checkExpr(e.Callee)
	argTypes := make([]types.Type, len(e.Args))
	for i, arg := range e.Args {
		argTypes[i] = c.checkExpr(arg)
	}

	fnType, ok := calleeType.(*types.FunctionType)
	if !ok {
		if calleeType.Kind() != types.KindAny {
			c.error(e.Callee.Span(), "TS2349", "This expression is not callable.")
		}
		c.result.Types[e] = types.TypeAny
		return types.TypeAny
	}

	effective := fnType
	if len(e.TypeArgs) > 0 {
		if len(fnType.TypeParams) == 0 {
			c.error(e.Span(), "TS2558", fmt.Sprintf("Expected 0 type arguments, but got %d.", len(e.TypeArgs)))
		} else {
			args := make([]types.Type, len(e.TypeArgs))
			for i, node := range e.TypeArgs {
				args[i] = c.resolveTypeNode(node)
			}
			instantiated, err := types.InstantiateFunction(fnType, args)
			if err != nil {
				c.error(e.Span(), "TS2558", err.Error())
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			effective = instantiated
		}
	} else if len(fnType.TypeParams) > 0 {
		instantiated, err := types.InferFunction(fnType, argTypes)
		if err != nil {
			c.error(e.Span(), "TS2684", err.Error())
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		effective = instantiated
	}

	for i, argType := range argTypes {
		var expected types.Type
		if i < len(effective.Params) {
			param := effective.Params[i]
			if param.Rest {
				if arr, ok := param.Type.(*types.ArrayType); ok {
					expected = arr.Elem
				}
			} else {
				expected = param.Type
			}
		} else if len(effective.Params) > 0 && effective.Params[len(effective.Params)-1].Rest {
			if arr, ok := effective.Params[len(effective.Params)-1].Type.(*types.ArrayType); ok {
				expected = arr.Elem
			}
		}
		if expected != nil && !argType.AssignableTo(expected) {
			c.error(e.Args[i].Span(), "TS2345", fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.", argType, expected))
		}
	}
	if len(fnType.TypeParams) > 0 {
		c.result.GenericCalls[e] = effective
	}
	c.result.Types[e] = effective.Return
	return effective.Return
}
