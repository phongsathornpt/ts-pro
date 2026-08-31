package llvm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

type emitter struct {
	module        mir.Module
	functions     map[mir.FunctionID]mir.Function
	shapes        map[mir.ShapeID]mir.Shape
	stringGlobals map[string]string
	closures      map[mir.FunctionID]closureDescriptor
}

func Emit(module mir.Module) (string, error) {
	if err := module.Verify(); err != nil {
		return "", fmt.Errorf("verify MIR before LLVM emission: %w", err)
	}
	closures, err := collectClosureDescriptors(module)
	if err != nil {
		return "", err
	}
	e := &emitter{module: module, functions: map[mir.FunctionID]mir.Function{}, shapes: map[mir.ShapeID]mir.Shape{}, stringGlobals: map[string]string{}, closures: closures}
	for _, fn := range module.Functions {
		e.functions[fn.ID] = fn
	}
	for _, shape := range module.Shapes {
		e.shapes[shape.ID] = shape
	}
	var b strings.Builder
	fmt.Fprintf(&b, "; tsnative module %s\n", strconv.Quote(module.Name))
	b.WriteString("target triple = \"x86_64-unknown-linux-gnu\"\n\n")
	b.WriteString("declare void @tsnative_console_log_f64(double)\n")
	b.WriteString("declare void @tsnative_console_log_string(ptr)\n")
	b.WriteString("declare ptr @tsnative_string_new(ptr, i64)\n")
	b.WriteString("declare ptr @tsnative_string_concat(ptr, ptr)\n")
	b.WriteString("declare ptr @tsnative_array_f64_new(i64)\n")
	b.WriteString("declare void @tsnative_array_f64_set(ptr, i64, double)\n")
	b.WriteString("declare double @tsnative_array_f64_len(ptr)\n")
	b.WriteString("declare double @tsnative_array_f64_get(ptr, double)\n")
	b.WriteString("declare ptr @tsnative_object_alloc(i64)\n\n")
	if err := e.emitClosureTypes(&b); err != nil {
		return "", err
	}
	shapes := append([]mir.Shape(nil), module.Shapes...)
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].ID < shapes[j].ID })
	for _, shape := range shapes {
		b.WriteString(shapeTypeName(shape.ID) + " = type { ")
		for i, field := range shape.Fields {
			if i != 0 {
				b.WriteString(", ")
			}
			typ, err := llvmType(field.Repr)
			if err != nil {
				return "", fmt.Errorf("shape s%d field %s: %w", shape.ID, field.Name, err)
			}
			b.WriteString(typ)
		}
		b.WriteString(" }\n")
	}
	if len(shapes) != 0 {
		b.WriteString("\n")
	}
	for _, global := range e.collectStringGlobals() {
		fmt.Fprintf(&b, "%s = private unnamed_addr constant [%d x i8] c\"%s\", align 1\n", global.name, len(global.value), escapeLLVMBytes(global.value))
	}
	if len(e.stringGlobals) != 0 {
		b.WriteString("\n")
	}
	functions := append([]mir.Function(nil), module.Functions...)
	sort.Slice(functions, func(i, j int) bool { return functions[i].ID < functions[j].ID })
	for _, fn := range functions {
		if err := e.emitFunction(&b, fn); err != nil {
			return "", err
		}
	}
	if err := e.emitClosureWrappers(&b); err != nil {
		return "", err
	}
	if module.Entry != nil {
		fmt.Fprintf(&b, "define i32 @main() {\nentry:\n  call void @%s()\n  ret i32 0\n}\n", functionName(*module.Entry))
	}
	return b.String(), nil
}

func (e *emitter) emitFunction(b *strings.Builder, fn mir.Function) error {
	returnType, err := llvmType(fn.ReturnRepr)
	if err != nil {
		return fmt.Errorf("function %s return: %w", fn.Name, err)
	}
	fmt.Fprintf(b, "; function %s\n", fn.Name)
	fmt.Fprintf(b, "define %s @%s(", returnType, functionName(fn.ID))
	values := buildValueOperands(fn)
	for i, param := range fn.Params {
		if i != 0 {
			b.WriteString(", ")
		}
		typ, err := llvmType(param.Repr)
		if err != nil {
			return fmt.Errorf("function %s parameter %s: %w", fn.Name, param.Name, err)
		}
		paramOperand := valueName(param.Value)
		fmt.Fprintf(b, "%s %s", typ, paramOperand)
	}
	b.WriteString(") {\n")
	blocks := append([]mir.Block(nil), fn.Blocks...)
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].ID < blocks[j].ID })
	for _, block := range blocks {
		fmt.Fprintf(b, "b%d:\n", block.ID)
		for _, inst := range block.Instructions {
			if err := e.emitInstruction(b, fn, inst, values); err != nil {
				return fmt.Errorf("function %s block b%d: %w", fn.Name, block.ID, err)
			}
		}
		if err := e.emitTerminator(b, fn, block.Terminator, values); err != nil {
			return fmt.Errorf("function %s block b%d: %w", fn.Name, block.ID, err)
		}
	}
	b.WriteString("}\n\n")
	return nil
}

func (e *emitter) emitInstruction(b *strings.Builder, fn mir.Function, inst mir.Instruction, values map[mir.ValueID]string) error {
	switch op := inst.Op.(type) {
	case mir.ConstF64:
		return nil
	case mir.ConstString:
		global, ok := e.stringGlobals[stringValueKey(fn.ID, inst.Result)]
		if !ok {
			return fmt.Errorf("missing LLVM string global for v%d", inst.Result)
		}
		name := valueName(inst.Result)
		length := len([]byte(op.Value))
		if length == 0 {
			fmt.Fprintf(b, "  %s = call ptr @tsnative_string_new(ptr null, i64 0)\n", name)
		} else {
			fmt.Fprintf(b, "  %s = call ptr @tsnative_string_new(ptr getelementptr inbounds ([%d x i8], ptr %s, i64 0, i64 0), i64 %d)\n", name, length, global, length)
		}
		return nil
	case mir.StringConcat:
		left, err := operand(values, op.Left)
		if err != nil {
			return err
		}
		right, err := operand(values, op.Right)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = call ptr @tsnative_string_concat(ptr %s, ptr %s)\n", valueName(inst.Result), left, right)
		return nil
	case mir.ArrayNewF64:
		if inst.Repr != mir.ReprArrayRef {
			return fmt.Errorf("array.new.f64 requires arrayref result")
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_array_f64_new(i64 %d)\n", name, len(op.Elements))
		for i, element := range op.Elements {
			value, err := operand(values, element)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  call void @tsnative_array_f64_set(ptr %s, i64 %d, double %s)\n", name, i, value)
		}
		return nil
	case mir.ArrayLengthF64:
		array, err := operand(values, op.Array)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call double @tsnative_array_f64_len(ptr %s)\n", name, array)
		return nil
	case mir.ArrayGetF64:
		array, err := operand(values, op.Array)
		if err != nil {
			return err
		}
		index, err := operand(values, op.Index)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call double @tsnative_array_f64_get(ptr %s, double %s)\n", name, array, index)
		return nil
	case mir.ObjectNew:
		shape, ok := e.shapes[op.Shape]
		if !ok {
			return fmt.Errorf("unknown object shape s%d", op.Shape)
		}
		name := valueName(inst.Result)
		typeName := shapeTypeName(op.Shape)
		fmt.Fprintf(b, "  %s.sizeptr = getelementptr %s, ptr null, i32 1\n", name, typeName)
		fmt.Fprintf(b, "  %s.size = ptrtoint ptr %s.sizeptr to i64\n", name, name)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_object_alloc(i64 %s.size)\n", name, name)
		for i, fieldValue := range op.Fields {
			value, err := operand(values, fieldValue)
			if err != nil {
				return err
			}
			fieldType, err := llvmType(shape.Fields[i].Repr)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  %s.f%d = getelementptr %s, ptr %s, i32 0, i32 %d\n", name, i, typeName, name, i)
			fmt.Fprintf(b, "  store %s %s, ptr %s.f%d\n", fieldType, value, name, i)
		}
		return nil
	case mir.ClosureNew:
		return e.emitClosureNew(b, inst, op, values)
	case mir.ClosureCall:
		return e.emitClosureCall(b, fn, inst, op, values)
	case mir.FieldGet:
		shape, ok := e.shapes[op.Shape]
		if !ok {
			return fmt.Errorf("unknown field shape s%d", op.Shape)
		}
		if int(op.Field) >= len(shape.Fields) {
			return fmt.Errorf("invalid field %d for shape s%d", op.Field, op.Shape)
		}
		object, err := operand(values, op.Object)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		typeName := shapeTypeName(op.Shape)
		fieldType, err := llvmType(shape.Fields[op.Field].Repr)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s.ptr = getelementptr %s, ptr %s, i32 0, i32 %d\n", name, typeName, object, op.Field)
		fmt.Fprintf(b, "  %s = load %s, ptr %s.ptr\n", name, fieldType, name)
		return nil
	case mir.Phi:
		typ, err := llvmType(inst.Repr)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = phi %s ", name, typ)
		for i, incoming := range op.Incoming {
			if i != 0 {
				b.WriteString(", ")
			}
			value, err := operand(values, incoming.Value)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "[ %s, %%b%d ]", value, incoming.Block)
		}
		b.WriteString("\n")
		return nil
	case mir.FloatBinary:
		left, err := operand(values, op.Left)
		if err != nil {
			return err
		}
		right, err := operand(values, op.Right)
		if err != nil {
			return err
		}
		opcode := map[mir.FloatBinaryOp]string{mir.FloatAdd: "fadd", mir.FloatSub: "fsub", mir.FloatMul: "fmul", mir.FloatDiv: "fdiv"}[op.Operator]
		if opcode == "" {
			return fmt.Errorf("unsupported float binary operator %d", op.Operator)
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = %s double %s, %s\n", name, opcode, left, right)
		values[inst.Result] = name
		return nil
	case mir.FloatCompare:
		left, err := operand(values, op.Left)
		if err != nil {
			return err
		}
		right, err := operand(values, op.Right)
		if err != nil {
			return err
		}
		predicate := map[mir.FloatCompareOp]string{
			mir.FloatLessThan: "olt", mir.FloatLessEqual: "ole",
			mir.FloatGreaterThan: "ogt", mir.FloatGreaterEqual: "oge",
			mir.FloatEqual: "oeq", mir.FloatNotEqual: "une",
		}[op.Operator]
		if predicate == "" {
			return fmt.Errorf("unsupported float compare operator %d", op.Operator)
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = fcmp %s double %s, %s\n", name, predicate, left, right)
		values[inst.Result] = name
		return nil
	case mir.Call:
		return e.emitCall(b, inst, op, values)
	case mir.IntrinsicCall:
		return e.emitIntrinsicCall(b, inst, op, values)
	default:
		return fmt.Errorf("unsupported MIR operation %T", inst.Op)
	}
}

func (e *emitter) emitCall(b *strings.Builder, inst mir.Instruction, call mir.Call, values map[mir.ValueID]string) error {
	callee, ok := e.functions[call.Callee]
	if !ok {
		return fmt.Errorf("unknown callee f%d", call.Callee)
	}
	if len(call.Args) != len(callee.Params) {
		return fmt.Errorf("call f%d has %d args; expected %d", call.Callee, len(call.Args), len(callee.Params))
	}
	retType, err := llvmType(callee.ReturnRepr)
	if err != nil {
		return err
	}
	name := valueName(inst.Result)
	fmt.Fprintf(b, "  %s = call %s @%s(", name, retType, functionName(callee.ID))
	for i, arg := range call.Args {
		if i != 0 {
			b.WriteString(", ")
		}
		typ, err := llvmType(callee.Params[i].Repr)
		if err != nil {
			return err
		}
		op, err := operand(values, arg)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "%s %s", typ, op)
	}
	b.WriteString(")\n")
	values[inst.Result] = name
	return nil
}

func (e *emitter) emitIntrinsicCall(b *strings.Builder, inst mir.Instruction, call mir.IntrinsicCall, values map[mir.ValueID]string) error {
	if inst.Repr != mir.ReprVoid || len(call.Args) != 1 {
		return fmt.Errorf("console.log requires void result and one argument")
	}
	arg, err := operand(values, call.Args[0])
	if err != nil {
		return err
	}
	switch call.Intrinsic {
	case mir.IntrinsicConsoleLogF64:
		fmt.Fprintf(b, "  call void @tsnative_console_log_f64(double %s)\n", arg)
	case mir.IntrinsicConsoleLogString:
		fmt.Fprintf(b, "  call void @tsnative_console_log_string(ptr %s)\n", arg)
	default:
		return fmt.Errorf("unsupported intrinsic %d", call.Intrinsic)
	}
	return nil
}

func (e *emitter) emitTerminator(b *strings.Builder, fn mir.Function, term mir.Terminator, values map[mir.ValueID]string) error {
	switch term := term.(type) {
	case mir.Return:
		if term.Value == nil {
			b.WriteString("  ret void\n")
			return nil
		}
		typ, err := llvmType(fn.ReturnRepr)
		if err != nil {
			return err
		}
		op, err := operand(values, *term.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  ret %s %s\n", typ, op)
		return nil
	case mir.Jump:
		fmt.Fprintf(b, "  br label %%b%d\n", term.Target)
		return nil
	case mir.Branch:
		condition, err := operand(values, term.Condition)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  br i1 %s, label %%b%d, label %%b%d\n", condition, term.Then, term.Else)
		return nil
	default:
		return fmt.Errorf("unsupported MIR terminator %T", term)
	}
}

func llvmType(repr mir.Repr) (string, error) {
	switch repr {
	case mir.ReprVoid:
		return "void", nil
	case mir.ReprBool:
		return "i1", nil
	case mir.ReprI32:
		return "i32", nil
	case mir.ReprI64:
		return "i64", nil
	case mir.ReprF64:
		return "double", nil
	case mir.ReprStringRef, mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef:
		return "ptr", nil
	case mir.ReprTagged, mir.ReprJSValue:
		return "i64", nil
	default:
		return "", fmt.Errorf("unsupported MIR representation %d", repr)
	}
}

type llvmStringGlobal struct {
	name  string
	value []byte
}

func (e *emitter) collectStringGlobals() []llvmStringGlobal {
	functions := append([]mir.Function(nil), e.module.Functions...)
	sort.Slice(functions, func(i, j int) bool { return functions[i].ID < functions[j].ID })
	var globals []llvmStringGlobal
	for _, fn := range functions {
		blocks := append([]mir.Block(nil), fn.Blocks...)
		sort.Slice(blocks, func(i, j int) bool { return blocks[i].ID < blocks[j].ID })
		for _, block := range blocks {
			for _, inst := range block.Instructions {
				constant, ok := inst.Op.(mir.ConstString)
				if !ok {
					continue
				}
				name := fmt.Sprintf("@.tsnative.str.%d", len(globals))
				e.stringGlobals[stringValueKey(fn.ID, inst.Result)] = name
				globals = append(globals, llvmStringGlobal{name: name, value: []byte(constant.Value)})
			}
		}
	}
	return globals
}

func stringValueKey(fn mir.FunctionID, value mir.ValueID) string {
	return fmt.Sprintf("%d:%d", fn, value)
}

func escapeLLVMBytes(value []byte) string {
	var b strings.Builder
	for _, ch := range value {
		if ch >= 0x20 && ch <= 0x7e && ch != '\\' && ch != '"' {
			b.WriteByte(ch)
		} else {
			fmt.Fprintf(&b, "\\%02X", ch)
		}
	}
	return b.String()
}

func buildValueOperands(fn mir.Function) map[mir.ValueID]string {
	values := make(map[mir.ValueID]string)
	for _, param := range fn.Params {
		values[param.Value] = valueName(param.Value)
	}
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			if constant, ok := inst.Op.(mir.ConstF64); ok {
				values[inst.Result] = formatF64(constant.Value)
			} else if inst.Repr != mir.ReprVoid {
				values[inst.Result] = valueName(inst.Result)
			}
		}
	}
	return values
}

func operand(values map[mir.ValueID]string, value mir.ValueID) (string, error) {
	operand, ok := values[value]
	if !ok {
		return "", fmt.Errorf("LLVM operand for v%d is unavailable", value)
	}
	return operand, nil
}

func valueName(value mir.ValueID) string    { return fmt.Sprintf("%%v%d", value) }
func functionName(id mir.FunctionID) string { return fmt.Sprintf("tsnative_f%d", id) }
func shapeTypeName(id mir.ShapeID) string   { return fmt.Sprintf("%%tsnative_shape_s%d", id) }
func formatF64(value float64) string        { return strconv.FormatFloat(value, 'e', 6, 64) }

func buildValueReprs(fn mir.Function) map[mir.ValueID]mir.Repr {
	result := make(map[mir.ValueID]mir.Repr)
	for _, param := range fn.Params {
		result[param.Value] = param.Repr
	}
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			result[inst.Result] = inst.Repr
		}
	}
	return result
}
