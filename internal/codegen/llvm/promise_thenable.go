package llvm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func promiseThenableResultName(repr mir.Repr) string {
	switch repr {
	case mir.ReprF64:
		return "f64"
	case mir.ReprBool:
		return "bool"
	case mir.ReprJSValue:
		return "ref"
	default:
		return "invalid"
	}
}

func (e *emitter) promiseThenableResults() []mir.Repr {
	set := map[mir.Repr]struct{}{}
	for _, fn := range e.module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				if op, ok := inst.Op.(mir.PromiseThenable); ok {
					set[op.Result] = struct{}{}
				}
			}
		}
	}
	result := make([]mir.Repr, 0, len(set))
	for repr := range set {
		result = append(result, repr)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func (e *emitter) emitPromiseThenableHelpers(b *strings.Builder) error {
	for _, repr := range e.promiseThenableResults() {
		if err := emitPromiseThenableResolveHelper(b, repr); err != nil {
			return err
		}
		if err := emitPromiseThenableRejectHelper(b, repr); err != nil {
			return err
		}
	}
	return nil
}

func emitPromiseThenableResolveHelper(b *strings.Builder, repr mir.Repr) error {
	name := promiseThenableResultName(repr)
	if name == "invalid" {
		return fmt.Errorf("unsupported thenable result representation %d", repr)
	}
	valueType, err := llvmType(repr)
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "define void @tsnative_thenable_resolve_%s(ptr %%env, %s %%value) {\n", name, valueType)
	b.WriteString("entry:\n  %task = load ptr, ptr %env\n")
	switch repr {
	case mir.ReprF64:
		b.WriteString("  call void @tsnative_promise_thenable_resolve_f64(ptr %task, double %value)\n")
	case mir.ReprBool:
		b.WriteString("  %value.i8 = zext i1 %value to i8\n  call void @tsnative_promise_thenable_resolve_bool(ptr %task, i8 %value.i8)\n")
	case mir.ReprJSValue:
		b.WriteString("  call void @tsnative_promise_thenable_resolve_ref(ptr %task, ptr %value)\n")
	}
	b.WriteString("  ret void\n}\n\n")
	return nil
}

func emitPromiseThenableRejectHelper(b *strings.Builder, repr mir.Repr) error {
	name := promiseThenableResultName(repr)
	if name == "invalid" {
		return fmt.Errorf("unsupported thenable result representation %d", repr)
	}
	fmt.Fprintf(b, "define void @tsnative_thenable_reject_%s(ptr %%env, ptr %%reason) {\n", name)
	b.WriteString("entry:\n  %task = load ptr, ptr %env\n  call void @tsnative_promise_thenable_reject(ptr %task, ptr %reason)\n  ret void\n}\n\n")
	return nil
}

func (e *emitter) emitPromiseThenable(b *strings.Builder, inst mir.Instruction, op mir.PromiseThenable, values map[mir.ValueID]string) error {
	thenable, err := operand(values, op.Thenable)
	if err != nil {
		return err
	}
	name := valueName(inst.Result)
	resultName := promiseThenableResultName(op.Result)
	if resultName == "invalid" || (op.Arity != 1 && op.Arity != 2) {
		return fmt.Errorf("invalid Promise thenable result or callback arity")
	}
	kind := map[mir.Repr]int{mir.ReprF64: 1, mir.ReprBool: 2, mir.ReprJSValue: 3}[op.Result]
	fmt.Fprintf(b, "  %s = call ptr @tsnative_promise_thenable_new(i32 %d)\n", name, kind)
	fmt.Fprintf(b, "  %s.env = call ptr @tsnative_object_alloc_atomic(i64 8)\n", name)
	fmt.Fprintf(b, "  store ptr %s, ptr %s.env\n", name, name)
	emitPromiseThenableTemporaryRoot(b, name+".env", name+".env")
	if err := emitPromiseThenableCallbackClosure(b, name+".resolve", name+".env", "tsnative_thenable_resolve_"+resultName); err != nil {
		return err
	}
	emitPromiseThenableTemporaryRoot(b, name+".resolve", name+".resolve")
	emitPromiseThenableFunctionBox(b, name+".resolve.box", name+".resolve")
	if op.Arity == 2 {
		if err := emitPromiseThenableCallbackClosure(b, name+".reject", name+".env", "tsnative_thenable_reject_"+resultName); err != nil {
			return err
		}
		emitPromiseThenableTemporaryRoot(b, name+".reject", name+".reject")
		emitPromiseThenableFunctionBox(b, name+".reject.box", name+".reject")
	}
	fmt.Fprintf(b, "  %s.then = call ptr @%s(ptr %s)\n", name, dynamicFieldGetHelperName("then"), thenable)
	fmt.Fprintf(b, "  %s.call = call ptr @%s(ptr %s.then, ptr %s", name, dynamicCallHelperName(int(op.Arity), true), name, thenable)
	fmt.Fprintf(b, ", ptr %s.resolve.box", name)
	if op.Arity == 2 {
		fmt.Fprintf(b, ", ptr %s.reject.box", name)
	}
	b.WriteString(")\n")
	if op.Arity == 2 {
		fmt.Fprintf(b, "  call void @tsnative_gc_root_unregister(ptr %s.reject.root)\n", name)
	}
	fmt.Fprintf(b, "  call void @tsnative_gc_root_unregister(ptr %s.resolve.root)\n", name)
	fmt.Fprintf(b, "  call void @tsnative_gc_root_unregister(ptr %s.env.root)\n", name)
	values[inst.Result] = name
	return nil
}

func emitPromiseThenableTemporaryRoot(b *strings.Builder, name, value string) {
	fmt.Fprintf(b, "  %s.slot = alloca ptr\n", name)
	fmt.Fprintf(b, "  store ptr %s, ptr %s.slot\n", value, name)
	fmt.Fprintf(b, "  %s.root = call ptr @tsnative_gc_root_register(ptr %s.slot)\n", name, name)
}

func emitPromiseThenableCallbackClosure(b *strings.Builder, name, env, code string) error {
	fmt.Fprintf(b, "  %s.sizeptr = getelementptr %%tsnative_closure, ptr null, i32 1\n", name)
	fmt.Fprintf(b, "  %s.size = ptrtoint ptr %s.sizeptr to i64\n", name, name)
	emitHeapObjectAlloc(b, name, name+".size", closureRefDescriptorName, 1)
	fmt.Fprintf(b, "  %s.code = getelementptr %%tsnative_closure, ptr %s, i32 0, i32 0\n", name, name)
	fmt.Fprintf(b, "  store ptr @%s, ptr %s.code\n", code, name)
	fmt.Fprintf(b, "  %s.env = getelementptr %%tsnative_closure, ptr %s, i32 0, i32 1\n", name, name)
	fmt.Fprintf(b, "  store ptr %s, ptr %s.env\n", env, name)
	fmt.Fprintf(b, "  %s.target = getelementptr %%tsnative_closure, ptr %s, i32 0, i32 2\n", name, name)
	fmt.Fprintf(b, "  store i32 -1, ptr %s.target\n", name)
	return nil
}

func emitPromiseThenableFunctionBox(b *strings.Builder, name, closure string) {
	fmt.Fprintf(b, "  %s = alloca { i32, i32, ptr }\n", name)
	fmt.Fprintf(b, "  %s.tag = getelementptr { i32, i32, ptr }, ptr %s, i32 0, i32 0\n", name, name)
	fmt.Fprintf(b, "  store i32 7, ptr %s.tag\n", name)
	fmt.Fprintf(b, "  %s.reserved = getelementptr { i32, i32, ptr }, ptr %s, i32 0, i32 1\n", name, name)
	fmt.Fprintf(b, "  store i32 0, ptr %s.reserved\n", name)
	fmt.Fprintf(b, "  %s.payload = getelementptr { i32, i32, ptr }, ptr %s, i32 0, i32 2\n", name, name)
	fmt.Fprintf(b, "  store ptr %s, ptr %s.payload\n", closure, name)
}
