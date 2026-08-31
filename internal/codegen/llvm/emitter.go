package llvm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

type emitter struct {
	module    mir.Module
	functions map[mir.FunctionID]mir.Function
}

func Emit(module mir.Module) (string, error) {
	if err := module.Verify(); err != nil {
		return "", fmt.Errorf("verify MIR before LLVM emission: %w", err)
	}
	e := &emitter{module: module, functions: map[mir.FunctionID]mir.Function{}}
	for _, fn := range module.Functions {
		e.functions[fn.ID] = fn
	}
	var b strings.Builder
	fmt.Fprintf(&b, "; tsnative module %s\n", strconv.Quote(module.Name))
	b.WriteString("target triple = \"x86_64-unknown-linux-gnu\"\n\n")
	b.WriteString("declare void @tsnative_console_log_f64(double)\n\n")
	functions := append([]mir.Function(nil), module.Functions...)
	sort.Slice(functions, func(i, j int) bool { return functions[i].ID < functions[j].ID })
	for _, fn := range functions {
		if err := e.emitFunction(&b, fn); err != nil {
			return "", err
		}
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
	values := map[mir.ValueID]string{}
	for i, param := range fn.Params {
		if i != 0 {
			b.WriteString(", ")
		}
		typ, err := llvmType(param.Repr)
		if err != nil {
			return fmt.Errorf("function %s parameter %s: %w", fn.Name, param.Name, err)
		}
		operand := valueName(param.Value)
		values[param.Value] = operand
		fmt.Fprintf(b, "%s %s", typ, operand)
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
		values[inst.Result] = formatF64(op.Value)
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
		if op.Operator != mir.FloatLessEqual {
			return fmt.Errorf("unsupported float compare operator %d", op.Operator)
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = fcmp ole double %s, %s\n", name, left, right)
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
	if call.Intrinsic != mir.IntrinsicConsoleLogF64 {
		return fmt.Errorf("unsupported intrinsic %d", call.Intrinsic)
	}
	if inst.Repr != mir.ReprVoid || len(call.Args) != 1 {
		return fmt.Errorf("console.log.f64 requires void result and one argument")
	}
	arg, err := operand(values, call.Args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "  call void @tsnative_console_log_f64(double %s)\n", arg)
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

func operand(values map[mir.ValueID]string, value mir.ValueID) (string, error) {
	operand, ok := values[value]
	if !ok {
		return "", fmt.Errorf("LLVM operand for v%d is unavailable", value)
	}
	return operand, nil
}

func valueName(value mir.ValueID) string    { return fmt.Sprintf("%%v%d", value) }
func functionName(id mir.FunctionID) string { return fmt.Sprintf("tsnative_f%d", id) }
func formatF64(value float64) string        { return strconv.FormatFloat(value, 'e', 6, 64) }
