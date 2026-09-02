package hir

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// DumpText returns a deterministic, human-readable representation of HIR.
// Functions and blocks are ordered by ID; instruction and parameter order is preserved.
func DumpText(module Module) string {
	var b strings.Builder
	fmt.Fprintf(&b, "module @%s m%d {\n", quoteName(module.Name), module.ID)
	for i, typ := range module.Types {
		fmt.Fprintf(&b, "  type t%d = %s\n", i, formatType(typ))
	}
	for _, shape := range module.Shapes {
		fmt.Fprintf(&b, "  shape s%d %s {", shape.ID, quoteName(shape.Name))
		for i, field := range shape.Fields {
			if i != 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s:t%d", quoteName(field.Name), field.SemanticType)
		}
		b.WriteString("}\n")
	}
	if module.Entry != nil {
		fmt.Fprintf(&b, "  entry f%d\n", *module.Entry)
	}
	functions := append([]Function(nil), module.Functions...)
	sort.Slice(functions, func(i, j int) bool { return functions[i].ID < functions[j].ID })
	for _, function := range functions {
		writeFunction(&b, function)
	}
	b.WriteString("}\n")
	return b.String()
}

func writeFunction(b *strings.Builder, fn Function) {
	fmt.Fprintf(b, "  func f%d @%s(", fn.ID, quoteName(fn.Name))
	for i, param := range fn.Params {
		if i != 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "v%d %s:t%d", param.Value, quoteName(param.Name), param.SemanticType)
		writeRepr(b, param.Repr)
	}
	fmt.Fprintf(b, ") -> t%d", fn.ReturnType)
	writeRepr(b, fn.ReturnRepr)
	fmt.Fprintf(b, " entry b%d {\n", fn.Entry)
	blocks := append([]Block(nil), fn.Blocks...)
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].ID < blocks[j].ID })
	for _, block := range blocks {
		fmt.Fprintf(b, "    b%d:\n", block.ID)
		for _, instruction := range block.Instructions {
			fmt.Fprintf(b, "      v%d:t%d", instruction.Result, instruction.SemanticType)
			writeRepr(b, instruction.Repr)
			fmt.Fprintf(b, " = %s\n", formatOperation(instruction.Op))
		}
		fmt.Fprintf(b, "      %s\n", formatTerminator(block.Terminator))
	}
	b.WriteString("  }\n")
}

func writeRepr(b *strings.Builder, repr Repr) {
	if repr.Kind == ReprUnproven {
		return
	}
	fmt.Fprintf(b, "[%s]", formatRepr(repr))
}

func formatOperation(op Operation) string {
	switch op := op.(type) {
	case ConstOp:
		return "const " + formatLiteral(op.Literal)
	case UnaryExpr:
		return fmt.Sprintf("%s v%d", formatUnary(op.Operator), op.Operand)
	case BinaryExpr:
		return fmt.Sprintf("%s v%d, v%d", formatBinary(op.Operator), op.Left, op.Right)
	case BoxOp:
		return fmt.Sprintf("box.%d v%d", op.Kind, op.Value)
	case DynamicBinaryOp:
		return fmt.Sprintf("dynamic.%s v%d, v%d", formatBinary(op.Operator), op.Left, op.Right)
	case DynamicCallOp:
		args := make([]string, len(op.Args))
		for i, arg := range op.Args {
			args[i] = fmt.Sprintf("v%d", arg)
		}
		if op.HasReceiver {
			return fmt.Sprintf("dynamic.call.this v%d recv=v%d(%s)", op.Callee, op.Receiver, strings.Join(args, ", "))
		}
		return fmt.Sprintf("dynamic.call v%d(%s)", op.Callee, strings.Join(args, ", "))
	case DynamicMethodCallOp:
		args := make([]string, len(op.Args))
		for i, arg := range op.Args {
			args[i] = fmt.Sprintf("v%d", arg)
		}
		return fmt.Sprintf("dynamic.method v%d(%s)", op.Receiver, strings.Join(args, ", "))
	case CallOp:
		args := make([]string, len(op.Args))
		for i, arg := range op.Args {
			args[i] = fmt.Sprintf("v%d", arg)
		}
		return fmt.Sprintf("call f%d(%s)", op.Callee, strings.Join(args, ", "))
	case DispatchCallOp:
		args := make([]string, len(op.Args))
		for i, arg := range op.Args {
			args[i] = fmt.Sprintf("v%d", arg)
		}
		cases := make([]string, len(op.Cases))
		for i, target := range op.Cases {
			cases[i] = fmt.Sprintf("%d:f%d", target.ClassTag, target.Callee)
		}
		return fmt.Sprintf("dispatch [%s](%s)", strings.Join(cases, ","), strings.Join(args, ", "))
	case ObjectAllocOp:
		return fmt.Sprintf("object.alloc s%d", op.Shape)
	case FieldSetOp:
		return fmt.Sprintf("field.set v%d, s%d.%d, v%d", op.Object, op.Shape, op.Field, op.Value)
	case DynamicFieldGetOp:
		return fmt.Sprintf("dynamic.field.get v%d, %q", op.Object, op.Field)
	case DynamicFieldSetOp:
		return fmt.Sprintf("dynamic.field.set v%d, %q, v%d", op.Object, op.Field, op.Value)
	case ClosureNewOp:
		captures := make([]string, len(op.Captures))
		for i, capture := range op.Captures {
			captures[i] = fmt.Sprintf("v%d", capture)
		}
		return fmt.Sprintf("closure.new f%d(%s)", op.Callee, strings.Join(captures, ", "))
	case PromiseResolveOp:
		return fmt.Sprintf("promise.resolve v%d, kind=%d", op.Value, op.Result)
	case PromiseAdoptOp:
		return fmt.Sprintf("promise.adopt v%d", op.Promise)
	case PromiseThenableOp:
		return fmt.Sprintf("promise.thenable v%d arity=%d cases=%d resolveJS=%t rejectJS=%t", op.Thenable, op.Arity, len(op.Cases), op.ResolveReturnsJS, op.RejectReturnsJS)
	case PromiseRejectOp:
		return fmt.Sprintf("promise.reject v%d, kind=%d", op.Reason, op.Result)
	case TaskSpawnOp:
		captures := make([]string, len(op.Captures))
		for i, capture := range op.Captures {
			captures[i] = fmt.Sprintf("v%d", capture)
		}
		if op.Group != nil {
			return fmt.Sprintf("task.group_spawn v%d, f%d(%s)", *op.Group, op.Callee, strings.Join(captures, ", "))
		}
		return fmt.Sprintf("task.spawn f%d(%s)", op.Callee, strings.Join(captures, ", "))
	case TaskJoinOp:
		if !op.Shared {
			return fmt.Sprintf("task.join.consume v%d", op.Task)
		}
		return fmt.Sprintf("task.join v%d", op.Task)
	case TaskWaitOp:
		return fmt.Sprintf("task.wait v%d", op.Task)
	case TaskFailureOp:
		return fmt.Sprintf("task.failure v%d", op.Task)
	case TaskRetainOp:
		return fmt.Sprintf("task.retain v%d", op.Task)
	case TaskReleaseOp:
		return fmt.Sprintf("task.release v%d", op.Task)
	case TaskYieldOp:
		return "task.yield"
	case TaskGroupNewOp:
		return "task.group_new"
	case TaskGroupJoinOp:
		return fmt.Sprintf("task.group_join v%d", op.Group)
	case TaskGroupCancelOp:
		return fmt.Sprintf("task.group_cancel v%d", op.Group)
	case TaskContextSetOp:
		return fmt.Sprintf("task.context_set v%d", op.Value)
	case TaskContextGetOp:
		return "task.context_get"
	case ChannelNewOp:
		return fmt.Sprintf("channel.new v%d", op.Capacity)
	case ChannelTrySendOp:
		return fmt.Sprintf("channel.try_send v%d, v%d", op.Channel, op.Value)
	case ChannelTryRecvOrOp:
		return fmt.Sprintf("channel.try_recv_or v%d, v%d", op.Channel, op.Fallback)
	case ChannelSendOp:
		return fmt.Sprintf("channel.send v%d, v%d", op.Channel, op.Value)
	case ChannelRecvOp:
		return fmt.Sprintf("channel.recv v%d", op.Channel)
	case ClosureCallOp:
		args := make([]string, len(op.Args))
		for i, arg := range op.Args {
			args[i] = fmt.Sprintf("v%d", arg)
		}
		return fmt.Sprintf("closure.call v%d(%s)", op.Closure, strings.Join(args, ", "))
	case IntrinsicCallOp:
		args := make([]string, len(op.Args))
		for i, arg := range op.Args {
			args[i] = fmt.Sprintf("v%d", arg)
		}
		name := "invalid"
		if op.Intrinsic == IntrinsicConsoleLogF64 {
			name = "console.log.f64"
		}
		return fmt.Sprintf("intrinsic %s(%s)", name, strings.Join(args, ", "))
	default:
		return "<invalid-op>"
	}
}
func formatTerminator(term Terminator) string {
	switch term := term.(type) {
	case ReturnTerm:
		if term.Value == nil {
			return "return"
		}
		return fmt.Sprintf("return v%d", *term.Value)
	case ThrowTerm:
		return fmt.Sprintf("throw v%d", term.Value)
	case JumpTerm:
		return fmt.Sprintf("jump b%d", term.Target)
	case BranchTerm:
		return fmt.Sprintf("branch v%d, b%d, b%d", term.Condition, term.Then, term.Else)
	default:
		return "<invalid-term>"
	}
}

func formatLiteral(literal Literal) string {
	switch literal.Kind {
	case LiteralBoolean:
		return strconv.FormatBool(literal.Bool)
	case LiteralNumber:
		return strconv.FormatFloat(literal.Number, 'g', -1, 64)
	case LiteralString:
		return strconv.Quote(literal.String)
	case LiteralNull:
		return "null"
	case LiteralUndefined:
		return "undefined"
	default:
		return "<invalid-literal>"
	}
}

func formatUnary(op UnaryOperator) string {
	switch op {
	case UnaryNegate:
		return "neg"
	case UnaryNot:
		return "not"
	default:
		return "<invalid-unary>"
	}
}

func formatBinary(op BinaryOperator) string {
	names := map[BinaryOperator]string{BinaryAdd: "add", BinarySub: "sub", BinaryMul: "mul", BinaryDiv: "div", BinaryLessThan: "lt", BinaryLessEqual: "le", BinaryGreaterThan: "gt", BinaryGreaterEqual: "ge", BinaryEqual: "eq", BinaryNotEqual: "ne", BinaryStrictEqual: "seq", BinaryStrictNotEqual: "sne"}
	if name, ok := names[op]; ok {
		return name
	}
	return "<invalid-binary>"
}
func formatType(typ SemanticType) string {
	switch typ.Kind {
	case TypeAny:
		return "any"
	case TypeUnknown:
		return "unknown"
	case TypeNever:
		return "never"
	case TypeVoid:
		return "void"
	case TypeUndefined:
		return "undefined"
	case TypeNull:
		return "null"
	case TypeBoolean:
		return "boolean"
	case TypeNumber:
		return "number"
	case TypeString:
		return "string"
	case TypeObject:
		return fmt.Sprintf("object<s%d>", typ.Shape)
	case TypeArray:
		return fmt.Sprintf("array<t%d>", typ.Element)
	case TypeUnion:
		parts := make([]string, len(typ.Members))
		for i, member := range typ.Members {
			parts[i] = fmt.Sprintf("t%d", member)
		}
		return "union<" + strings.Join(parts, ",") + ">"
	case TypeFunction:
		parts := make([]string, len(typ.Params))
		for i, param := range typ.Params {
			parts[i] = fmt.Sprintf("t%d", param)
		}
		return fmt.Sprintf("fn(%s)->t%d", strings.Join(parts, ","), typ.ReturnType)
	default:
		return "<invalid-type>"
	}
}

func formatRepr(repr Repr) string {
	switch repr.Kind {
	case ReprVoid:
		return "void"
	case ReprBool:
		return "bool"
	case ReprI32:
		return "i32"
	case ReprI64:
		return "i64"
	case ReprF64:
		return "f64"
	case ReprStringRef:
		return "stringref"
	case ReprArrayRef:
		return "arrayref"
	case ReprObjectRef:
		return fmt.Sprintf("objectref<s%d>", repr.Shape)
	case ReprFunctionRef:
		return "funcref"
	case ReprTaggedUnion:
		return "tagged"
	case ReprJSValue:
		return "jsvalue"
	default:
		return "unproven"
	}
}

func quoteName(name string) string { return strconv.Quote(name) }
