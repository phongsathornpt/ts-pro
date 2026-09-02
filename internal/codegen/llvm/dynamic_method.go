package llvm

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func dynamicMethodCallHelperName(op mir.DynamicMethodCall) string {
	var key strings.Builder
	fmt.Fprintf(&key, "%d", len(op.Args))
	for _, target := range op.Cases {
		fmt.Fprintf(&key, ":%d/%d", target.ClassTag, target.Callee)
	}
	sum := sha256.Sum256([]byte(key.String()))
	return fmt.Sprintf("tsnative_dynamic_method_%x", sum[:8])
}

func (e *emitter) dynamicMethodCalls() []mir.DynamicMethodCall {
	unique := map[string]mir.DynamicMethodCall{}
	for _, fn := range e.module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				if op, ok := inst.Op.(mir.DynamicMethodCall); ok {
					unique[dynamicMethodCallHelperName(op)] = op
				}
			}
		}
	}
	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]mir.DynamicMethodCall, 0, len(names))
	for _, name := range names {
		result = append(result, unique[name])
	}
	return result
}

func (e *emitter) emitDynamicMethodCallHelpers(b *strings.Builder) error {
	for _, op := range e.dynamicMethodCalls() {
		if err := e.emitDynamicMethodCallHelper(b, op); err != nil {
			return err
		}
	}
	return nil
}
func (e *emitter) emitDynamicMethodCallHelper(b *strings.Builder, op mir.DynamicMethodCall) error {
	name := dynamicMethodCallHelperName(op)
	fmt.Fprintf(b, "define ptr @%s(ptr %%receiver", name)
	for i := range op.Args {
		fmt.Fprintf(b, ", ptr %%arg%d", i)
	}
	b.WriteString(") {\nentry:\n")
	b.WriteString("  %object = call ptr @tsnative_jsvalue_unbox_object(ptr %receiver)\n")
	b.WriteString("  %tag = load i32, ptr %object\n")
	b.WriteString("  switch i32 %tag, label %invalid [")
	for _, target := range op.Cases {
		fmt.Fprintf(b, " i32 %d, label %%f%d", target.ClassTag, target.Callee)
	}
	b.WriteString(" ]\n")
	for _, target := range op.Cases {
		if err := e.emitDynamicMethodCase(b, op, target); err != nil {
			return err
		}
	}
	b.WriteString("invalid:\n  call void @tsnative_jsvalue_dynamic_call_invalid()\n  unreachable\n}\n\n")
	return nil
}
func (e *emitter) emitDynamicMethodCase(b *strings.Builder, op mir.DynamicMethodCall, target mir.DispatchCase) error {
	fn, ok := e.functions[target.Callee]
	if !ok {
		return fmt.Errorf("dynamic method references unknown target f%d", target.Callee)
	}
	if len(fn.Params) != len(op.Args)+1 || fn.Params[0].Repr != mir.ReprObjectRef {
		return fmt.Errorf("dynamic method f%d has incompatible hidden-this signature", target.Callee)
	}
	fmt.Fprintf(b, "f%d:\n", target.Callee)
	args := make([]string, 0, len(op.Args))
	for i := range op.Args {
		value, err := emitDynamicCallUnbox(b, target.Callee, i, fn.Params[i+1])
		if err != nil {
			return err
		}
		args = append(args, value)
	}
	retType, err := llvmType(fn.ReturnRepr)
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("  %%result.f%d = ", target.Callee)
	if fn.ReturnRepr == mir.ReprVoid {
		prefix = "  "
	}
	fmt.Fprintf(b, "%scall %s @%s(ptr %%object", prefix, retType, functionName(target.Callee))
	for i, arg := range args {
		typ, err := llvmType(fn.Params[i+1].Repr)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, ", %s %s", typ, arg)
	}
	b.WriteString(")\n")
	return emitDynamicCallBoxResult(b, fn, target.Callee)
}
