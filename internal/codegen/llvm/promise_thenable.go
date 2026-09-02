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
	b.WriteString("entry:\n  %settled.ptr = getelementptr { i1, ptr }, ptr %env, i32 0, i32 0\n")
	b.WriteString("  %settled = load i1, ptr %settled.ptr\n  br i1 %settled, label %done, label %settle\nsettle:\n")
	switch repr {
	case mir.ReprF64:
		b.WriteString("  %task = call ptr @tsnative_promise_resolve_f64(double %value)\n")
	case mir.ReprBool:
		b.WriteString("  %value.i8 = zext i1 %value to i8\n  %task = call ptr @tsnative_promise_resolve_bool(i8 %value.i8)\n")
	case mir.ReprJSValue:
		b.WriteString("  %task = call ptr @tsnative_promise_resolve_ref(ptr %value)\n")
	}
	b.WriteString("  %task.ptr = getelementptr { i1, ptr }, ptr %env, i32 0, i32 1\n")
	b.WriteString("  store ptr %task, ptr %task.ptr\n  store i1 true, ptr %settled.ptr\n  br label %done\ndone:\n  ret void\n}\n\n")
	return nil
}

func emitPromiseThenableRejectHelper(b *strings.Builder, repr mir.Repr) error {
	name := promiseThenableResultName(repr)
	if name == "invalid" {
		return fmt.Errorf("unsupported thenable result representation %d", repr)
	}
	kind := 0
	switch repr {
	case mir.ReprF64:
		kind = 1
	case mir.ReprBool:
		kind = 2
	case mir.ReprJSValue:
		kind = 3
	}
	fmt.Fprintf(b, "define void @tsnative_thenable_reject_%s(ptr %%env, ptr %%reason) {\n", name)
	b.WriteString("entry:\n  %settled.ptr = getelementptr { i1, ptr }, ptr %env, i32 0, i32 0\n")
	b.WriteString("  %settled = load i1, ptr %settled.ptr\n  br i1 %settled, label %done, label %settle\nsettle:\n")
	fmt.Fprintf(b, "  %%task = call ptr @tsnative_promise_reject(ptr %%reason, i32 %d)\n", kind)
	b.WriteString("  %task.ptr = getelementptr { i1, ptr }, ptr %env, i32 0, i32 1\n")
	b.WriteString("  store ptr %task, ptr %task.ptr\n  store i1 true, ptr %settled.ptr\n  br label %done\ndone:\n  ret void\n}\n\n")
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
	fmt.Fprintf(b, "  %s.ctx = alloca { i1, ptr }\n", name)
	fmt.Fprintf(b, "  %s.settled = getelementptr { i1, ptr }, ptr %s.ctx, i32 0, i32 0\n", name, name)
	fmt.Fprintf(b, "  %s.task = getelementptr { i1, ptr }, ptr %s.ctx, i32 0, i32 1\n", name, name)
	fmt.Fprintf(b, "  store i1 false, ptr %s.settled\n  store ptr null, ptr %s.task\n", name, name)
	if err := emitPromiseThenableCallbackClosure(b, name+".resolve", name+".ctx", "tsnative_thenable_resolve_"+resultName); err != nil {
		return err
	}
	emitPromiseThenableFunctionBox(b, name+".resolve.box", name+".resolve")
	if op.Arity == 2 {
		if err := emitPromiseThenableCallbackClosure(b, name+".reject", name+".ctx", "tsnative_thenable_reject_"+resultName); err != nil {
			return err
		}
		emitPromiseThenableFunctionBox(b, name+".reject.box", name+".reject")
	}
	fmt.Fprintf(b, "  %s.then = call ptr @%s(ptr %s)\n", name, dynamicFieldGetHelperName("then"), thenable)
	fmt.Fprintf(b, "  %s.call = call ptr @%s(ptr %s.then, ptr %s", name, dynamicCallHelperName(int(op.Arity), true), name, thenable)
	fmt.Fprintf(b, ", ptr %s.resolve.box", name)
	if op.Arity == 2 {
		fmt.Fprintf(b, ", ptr %s.reject.box", name)
	}
	b.WriteString(")\n")
	fmt.Fprintf(b, "  %s.raw = load ptr, ptr %s.task\n", name, name)
	fmt.Fprintf(b, "  %s = call ptr @tsnative_promise_thenable_require_settled(ptr %s.raw)\n", name, name)
	values[inst.Result] = name
	return nil
}

func emitPromiseThenableCallbackClosure(b *strings.Builder, name, env, code string) error {
	fmt.Fprintf(b, "  %s = alloca %%tsnative_closure\n", name)
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
