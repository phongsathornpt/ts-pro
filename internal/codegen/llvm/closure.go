package llvm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

type closureDescriptor struct {
	Callee       mir.FunctionID
	CaptureCount int
}

func collectClosureDescriptors(module mir.Module) (map[mir.FunctionID]closureDescriptor, error) {
	functions := map[mir.FunctionID]mir.Function{}
	for _, fn := range module.Functions {
		functions[fn.ID] = fn
	}
	result := map[mir.FunctionID]closureDescriptor{}
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				op, ok := inst.Op.(mir.ClosureNew)
				if !ok {
					continue
				}
				target, ok := functions[op.Callee]
				if !ok {
					return nil, fmt.Errorf("closure target f%d is missing", op.Callee)
				}
				if len(op.Captures) > len(target.Params) {
					return nil, fmt.Errorf("closure f%d captures %d values but target has %d params", op.Callee, len(op.Captures), len(target.Params))
				}
				descriptor := closureDescriptor{Callee: op.Callee, CaptureCount: len(op.Captures)}
				if existing, ok := result[op.Callee]; ok && existing.CaptureCount != descriptor.CaptureCount {
					return nil, fmt.Errorf("closure f%d has inconsistent capture counts", op.Callee)
				}
				result[op.Callee] = descriptor
			}
		}
	}
	return result, nil
}

func (e *emitter) emitClosureTypes(b *strings.Builder) error {
	if len(e.closures) == 0 {
		return nil
	}
	b.WriteString("%tsnative_closure = type { ptr, ptr }\n")
	for _, descriptor := range sortedClosureDescriptors(e.closures) {
		if descriptor.CaptureCount == 0 {
			continue
		}
		fn := e.functions[descriptor.Callee]
		fmt.Fprintf(b, "%s = type { ", closureEnvTypeName(descriptor.Callee))
		for i := 0; i < descriptor.CaptureCount; i++ {
			if i != 0 {
				b.WriteString(", ")
			}
			typ, err := llvmType(fn.Params[i].Repr)
			if err != nil {
				return err
			}
			b.WriteString(typ)
		}
		b.WriteString(" }\n")
	}
	b.WriteString("\n")
	return nil
}

func (e *emitter) emitClosureWrappers(b *strings.Builder) error {
	for _, descriptor := range sortedClosureDescriptors(e.closures) {
		fn := e.functions[descriptor.Callee]
		retType, err := llvmType(fn.ReturnRepr)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "define %s @%s(ptr %%env", retType, closureWrapperName(descriptor.Callee))
		for i := descriptor.CaptureCount; i < len(fn.Params); i++ {
			typ, err := llvmType(fn.Params[i].Repr)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, ", %s %%arg%d", typ, i-descriptor.CaptureCount)
		}
		b.WriteString(") {\nentry:\n")
		for i := 0; i < descriptor.CaptureCount; i++ {
			typ, err := llvmType(fn.Params[i].Repr)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  %%capture%d.ptr = getelementptr %s, ptr %%env, i32 0, i32 %d\n", i, closureEnvTypeName(descriptor.Callee), i)
			fmt.Fprintf(b, "  %%capture%d = load %s, ptr %%capture%d.ptr\n", i, typ, i)
		}
		if err := emitClosureWrapperCall(b, fn, descriptor); err != nil {
			return err
		}
		b.WriteString("}\n\n")
	}
	return nil
}

func emitClosureWrapperCall(b *strings.Builder, fn mir.Function, descriptor closureDescriptor) error {
	retType, err := llvmType(fn.ReturnRepr)
	if err != nil {
		return err
	}
	prefix := "  %result = "
	if fn.ReturnRepr == mir.ReprVoid {
		prefix = "  "
	}
	fmt.Fprintf(b, "%scall %s @%s(", prefix, retType, functionName(fn.ID))
	for i, param := range fn.Params {
		if i != 0 {
			b.WriteString(", ")
		}
		typ, err := llvmType(param.Repr)
		if err != nil {
			return err
		}
		if i < descriptor.CaptureCount {
			fmt.Fprintf(b, "%s %%capture%d", typ, i)
		} else {
			fmt.Fprintf(b, "%s %%arg%d", typ, i-descriptor.CaptureCount)
		}
	}
	b.WriteString(")\n")
	if fn.ReturnRepr == mir.ReprVoid {
		b.WriteString("  ret void\n")
	} else {
		fmt.Fprintf(b, "  ret %s %%result\n", retType)
	}
	return nil
}

func (e *emitter) emitClosureNew(b *strings.Builder, parent mir.FunctionID, inst mir.Instruction, op mir.ClosureNew, values map[mir.ValueID]string) error {
	descriptor, ok := e.closures[op.Callee]
	if !ok {
		return fmt.Errorf("missing closure descriptor for f%d", op.Callee)
	}
	fn := e.functions[op.Callee]
	if len(op.Captures) != descriptor.CaptureCount {
		return fmt.Errorf("closure f%d capture mismatch", op.Callee)
	}
	name := valueName(inst.Result)
	stack := e.isStackObject(parent, inst.Result)
	env := "null"
	if descriptor.CaptureCount != 0 {
		env = name + ".env"
		if stack {
			fmt.Fprintf(b, "  %s = alloca %s\n", env, closureEnvTypeName(op.Callee))
		} else {
			fmt.Fprintf(b, "  %s.sizeptr = getelementptr %s, ptr null, i32 1\n", env, closureEnvTypeName(op.Callee))
			fmt.Fprintf(b, "  %s.size = ptrtoint ptr %s.sizeptr to i64\n", env, env)
			envRefs := closureEnvRefFields(fn, descriptor.CaptureCount)
			emitHeapObjectAlloc(b, env, env+".size", closureEnvRefDescriptorName(op.Callee), len(envRefs))
		}
		for i, capture := range op.Captures {
			value, err := operand(values, capture)
			if err != nil {
				return err
			}
			typ, err := llvmType(fn.Params[i].Repr)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  %s.c%d = getelementptr %s, ptr %s, i32 0, i32 %d\n", env, i, closureEnvTypeName(op.Callee), env, i)
			fmt.Fprintf(b, "  store %s %s, ptr %s.c%d\n", typ, value, env, i)
		}
	}
	if stack {
		fmt.Fprintf(b, "  %s = alloca %%tsnative_closure\n", name)
	} else {
		fmt.Fprintf(b, "  %s.sizeptr = getelementptr %%tsnative_closure, ptr null, i32 1\n", name)
		fmt.Fprintf(b, "  %s.size = ptrtoint ptr %s.sizeptr to i64\n", name, name)
		emitHeapObjectAlloc(b, name, name+".size", closureRefDescriptorName, 1)
	}
	fmt.Fprintf(b, "  %s.codeptr = getelementptr %%tsnative_closure, ptr %s, i32 0, i32 0\n", name, name)
	fmt.Fprintf(b, "  store ptr @%s, ptr %s.codeptr\n", closureWrapperName(op.Callee), name)
	fmt.Fprintf(b, "  %s.envptr = getelementptr %%tsnative_closure, ptr %s, i32 0, i32 1\n", name, name)
	fmt.Fprintf(b, "  store ptr %s, ptr %s.envptr\n", env, name)
	return nil
}

func (e *emitter) emitClosureCall(b *strings.Builder, fn mir.Function, inst mir.Instruction, op mir.ClosureCall, values map[mir.ValueID]string) error {
	closure, err := operand(values, op.Closure)
	if err != nil {
		return err
	}
	name := valueName(inst.Result)
	fmt.Fprintf(b, "  %s.codeptr = getelementptr %%tsnative_closure, ptr %s, i32 0, i32 0\n", name, closure)
	fmt.Fprintf(b, "  %s.code = load ptr, ptr %s.codeptr\n", name, name)
	fmt.Fprintf(b, "  %s.envptr = getelementptr %%tsnative_closure, ptr %s, i32 0, i32 1\n", name, closure)
	fmt.Fprintf(b, "  %s.env = load ptr, ptr %s.envptr\n", name, name)
	retType, err := llvmType(inst.Repr)
	if err != nil {
		return err
	}
	reprs := buildValueReprs(fn)
	prefix := fmt.Sprintf("  %s = ", name)
	if inst.Repr == mir.ReprVoid {
		prefix = "  "
	}
	fmt.Fprintf(b, "%scall %s %s.code(ptr %s.env", prefix, retType, name, name)
	for _, arg := range op.Args {
		typ, err := llvmType(reprs[arg])
		if err != nil {
			return err
		}
		value, err := operand(values, arg)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, ", %s %s", typ, value)
	}
	b.WriteString(")\n")
	return nil
}

func sortedClosureDescriptors(values map[mir.FunctionID]closureDescriptor) []closureDescriptor {
	result := make([]closureDescriptor, 0, len(values))
	for _, descriptor := range values {
		result = append(result, descriptor)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Callee < result[j].Callee })
	return result
}

func closureEnvTypeName(id mir.FunctionID) string { return fmt.Sprintf("%%tsnative_env_f%d", id) }
func closureWrapperName(id mir.FunctionID) string {
	return fmt.Sprintf("tsnative_closure_invoke_f%d", id)
}
