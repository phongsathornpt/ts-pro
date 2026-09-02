package llvm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func dynamicCallHelperName(arity int) string { return fmt.Sprintf("tsnative_dynamic_call_%d", arity) }

func (e *emitter) dynamicCallArities() []int {
	set := map[int]struct{}{}
	for _, fn := range e.module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				if op, ok := inst.Op.(mir.DynamicCall); ok {
					set[len(op.Args)] = struct{}{}
				}
			}
		}
	}
	result := make([]int, 0, len(set))
	for arity := range set {
		result = append(result, arity)
	}
	sort.Ints(result)
	return result
}

func (e *emitter) emitDynamicCallHelpers(b *strings.Builder) error {
	for _, arity := range e.dynamicCallArities() {
		if err := e.emitDynamicCallHelper(b, arity); err != nil {
			return err
		}
	}
	return nil
}

func (e *emitter) emitDynamicCallHelper(b *strings.Builder, arity int) error {
	name := dynamicCallHelperName(arity)
	fmt.Fprintf(b, "define ptr @%s(ptr %%boxed", name)
	for i := 0; i < arity; i++ {
		fmt.Fprintf(b, ", ptr %%arg%d", i)
	}
	b.WriteString(") {\nentry:\n")
	b.WriteString("  %target = call i32 @tsnative_jsvalue_function_target(ptr %boxed)\n")
	b.WriteString("  switch i32 %target, label %invalid [")
	candidates := make([]closureDescriptor, 0)
	for _, descriptor := range sortedClosureDescriptors(e.closures) {
		fn := e.functions[descriptor.Callee]
		if len(fn.Params)-descriptor.CaptureCount == arity {
			candidates = append(candidates, descriptor)
			fmt.Fprintf(b, " i32 %d, label %%f%d", descriptor.Callee, descriptor.Callee)
		}
	}
	b.WriteString(" ]\n")
	for _, descriptor := range candidates {
		if err := e.emitDynamicCallCase(b, descriptor); err != nil {
			return err
		}
	}
	b.WriteString("invalid:\n  call void @tsnative_jsvalue_dynamic_call_invalid()\n  unreachable\n}\n\n")
	return nil
}

func (e *emitter) emitDynamicCallCase(b *strings.Builder, descriptor closureDescriptor) error {
	fn := e.functions[descriptor.Callee]
	fmt.Fprintf(b, "f%d:\n", descriptor.Callee)
	fmt.Fprintf(b, "  %%closure.f%d = call ptr @tsnative_jsvalue_unbox_function(ptr %%boxed)\n", descriptor.Callee)
	fmt.Fprintf(b, "  %%codeptr.f%d = getelementptr %%tsnative_closure, ptr %%closure.f%d, i32 0, i32 0\n", descriptor.Callee, descriptor.Callee)
	fmt.Fprintf(b, "  %%code.f%d = load ptr, ptr %%codeptr.f%d\n", descriptor.Callee, descriptor.Callee)
	fmt.Fprintf(b, "  %%envptr.f%d = getelementptr %%tsnative_closure, ptr %%closure.f%d, i32 0, i32 1\n", descriptor.Callee, descriptor.Callee)
	fmt.Fprintf(b, "  %%env.f%d = load ptr, ptr %%envptr.f%d\n", descriptor.Callee, descriptor.Callee)

	args := make([]string, 0, len(fn.Params)-descriptor.CaptureCount)
	for i := descriptor.CaptureCount; i < len(fn.Params); i++ {
		argIndex := i - descriptor.CaptureCount
		value, err := emitDynamicCallUnbox(b, descriptor.Callee, argIndex, fn.Params[i])
		if err != nil {
			return fmt.Errorf("dynamic call f%d arg %d: %w", descriptor.Callee, argIndex, err)
		}
		args = append(args, value)
	}
	retType, err := llvmType(fn.ReturnRepr)
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("  %%result.f%d = ", descriptor.Callee)
	if fn.ReturnRepr == mir.ReprVoid {
		prefix = "  "
	}
	fmt.Fprintf(b, "%scall %s %%code.f%d(ptr %%env.f%d", prefix, retType, descriptor.Callee, descriptor.Callee)
	for i, arg := range args {
		param := fn.Params[descriptor.CaptureCount+i]
		typ, err := llvmType(param.Repr)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, ", %s %s", typ, arg)
	}
	b.WriteString(")\n")
	return emitDynamicCallBoxResult(b, fn, descriptor.Callee)
}

func emitDynamicCallUnbox(b *strings.Builder, target mir.FunctionID, index int, param mir.Param) (string, error) {
	name := fmt.Sprintf("%%arg%d.f%d.native", index, target)
	switch param.Repr {
	case mir.ReprF64:
		fmt.Fprintf(b, "  %s = call double @tsnative_jsvalue_unbox_f64(ptr %%arg%d)\n", name, index)
	case mir.ReprBool:
		fmt.Fprintf(b, "  %s.i8 = call i8 @tsnative_jsvalue_unbox_bool(ptr %%arg%d)\n  %s = trunc i8 %s.i8 to i1\n", name, index, name, name)
	case mir.ReprStringRef:
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_string(ptr %%arg%d)\n", name, index)
	case mir.ReprArrayRef:
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_array(ptr %%arg%d)\n", name, index)
	case mir.ReprObjectRef:
		if param.HasObjectShape {
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_object_shape(ptr %%arg%d, i32 %d)\n", name, index, param.ObjectShape)
		} else {
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_object(ptr %%arg%d)\n", name, index)
		}
	case mir.ReprFunctionRef:
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_function(ptr %%arg%d)\n", name, index)
	case mir.ReprJSValue:
		return fmt.Sprintf("%%arg%d", index), nil
	default:
		return "", fmt.Errorf("unsupported parameter representation %d", param.Repr)
	}
	return name, nil
}

func emitDynamicCallBoxResult(b *strings.Builder, fn mir.Function, target mir.FunctionID) error {
	result := fmt.Sprintf("%%result.f%d", target)
	box := fmt.Sprintf("%%boxed.result.f%d", target)
	switch fn.ReturnRepr {
	case mir.ReprVoid:
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_undefined()\n", box)
	case mir.ReprF64:
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_f64(double %s)\n", box, result)
	case mir.ReprBool:
		fmt.Fprintf(b, "  %s.i8 = zext i1 %s to i8\n  %s = call ptr @tsnative_jsvalue_box_bool(i8 %s.i8)\n", box, result, box, box)
	case mir.ReprStringRef:
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_string(ptr %s)\n", box, result)
	case mir.ReprArrayRef:
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_array(ptr %s)\n", box, result)
	case mir.ReprObjectRef:
		if fn.HasReturnObjectShape {
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_object_shape(ptr %s, i32 %d)\n", box, result, fn.ReturnObjectShape)
		} else {
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_object(ptr %s)\n", box, result)
		}
	case mir.ReprFunctionRef:
		fmt.Fprintf(b, "  %s.targetptr = getelementptr %%tsnative_closure, ptr %s, i32 0, i32 2\n", box, result)
		fmt.Fprintf(b, "  %s.target = load i32, ptr %s.targetptr\n", box, box)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_function_target(ptr %s, i32 %s.target)\n", box, result, box)
	case mir.ReprJSValue:
		b.WriteString("  ret ptr " + result + "\n")
		return nil
	default:
		return fmt.Errorf("unsupported return representation %d", fn.ReturnRepr)
	}
	fmt.Fprintf(b, "  ret ptr %s\n", box)
	return nil
}
