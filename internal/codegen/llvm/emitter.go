package llvm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	escapeanalysis "github.com/projectthorn/tsv7-bin/internal/analysis/escape"
	"github.com/projectthorn/tsv7-bin/internal/mir"
)

type emitter struct {
	module        mir.Module
	functions     map[mir.FunctionID]mir.Function
	shapes        map[mir.ShapeID]mir.Shape
	stringGlobals map[string]string
	closures      map[mir.FunctionID]closureDescriptor
	escapes       escapeanalysis.Result
	stackObjects  escapeanalysis.StackObjectResult
	stackAliases  map[mir.FunctionID]map[mir.ValueID]mir.ValueID
	scalarObjects escapeanalysis.ScalarObjectResult
}

func Emit(module mir.Module) (string, error) {
	escapes := escapeanalysis.Analyze(module)
	return EmitWithEscapeAnalysis(module, escapes)
}

func EmitWithEscapeAnalysis(module mir.Module, escapes escapeanalysis.Result) (string, error) {
	if err := module.Verify(); err != nil {
		return "", fmt.Errorf("verify MIR before LLVM emission: %w", err)
	}
	closures, err := collectClosureDescriptors(module)
	if err != nil {
		return "", err
	}
	stackObjects := escapeanalysis.StackObjects(module, escapes)
	e := &emitter{
		module: module, functions: map[mir.FunctionID]mir.Function{}, shapes: map[mir.ShapeID]mir.Shape{},
		stringGlobals: map[string]string{}, closures: closures, escapes: escapes, stackObjects: stackObjects,
		stackAliases:  escapeanalysis.StackObjectAliases(module, stackObjects),
		scalarObjects: escapeanalysis.ScalarObjectsWithEscapeAnalysis(module, stackObjects, escapes),
	}
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
	b.WriteString("declare void @tsnative_console_log_jsvalue(ptr)\n")
	b.WriteString("declare ptr @tsnative_jsvalue_box_f64(double)\n")
	b.WriteString("declare ptr @tsnative_jsvalue_box_string(ptr)\ndeclare ptr @tsnative_jsvalue_box_bool(i8)\ndeclare ptr @tsnative_jsvalue_box_object(ptr)\ndeclare ptr @tsnative_jsvalue_box_object_shape(ptr, i32)\ndeclare i32 @tsnative_jsvalue_object_shape(ptr)\ndeclare ptr @tsnative_jsvalue_box_array(ptr)\ndeclare ptr @tsnative_jsvalue_box_function(ptr)\ndeclare ptr @tsnative_jsvalue_box_function_target(ptr, i32)\ndeclare i32 @tsnative_jsvalue_function_target(ptr)\ndeclare ptr @tsnative_jsvalue_null()\ndeclare ptr @tsnative_jsvalue_undefined()\n")
	b.WriteString("declare double @tsnative_jsvalue_unbox_f64(ptr)\ndeclare ptr @tsnative_jsvalue_unbox_object(ptr)\ndeclare ptr @tsnative_jsvalue_unbox_object_shape(ptr, i32)\ndeclare ptr @tsnative_jsvalue_unbox_function(ptr)\ndeclare void @tsnative_jsvalue_dynamic_set_missing()\ndeclare void @tsnative_jsvalue_dynamic_call_invalid()\ndeclare ptr @tsnative_jsvalue_unbox_string(ptr)\ndeclare i8 @tsnative_jsvalue_unbox_bool(ptr)\ndeclare ptr @tsnative_jsvalue_unbox_array(ptr)\n")
	b.WriteString("declare ptr @tsnative_jsvalue_add(ptr, ptr)\n")
	b.WriteString("declare double @tsnative_jsvalue_sub(ptr, ptr)\ndeclare double @tsnative_jsvalue_mul(ptr, ptr)\ndeclare double @tsnative_jsvalue_div(ptr, ptr)\n")
	b.WriteString("declare i8 @tsnative_jsvalue_lt(ptr, ptr)\ndeclare i8 @tsnative_jsvalue_le(ptr, ptr)\ndeclare i8 @tsnative_jsvalue_gt(ptr, ptr)\ndeclare i8 @tsnative_jsvalue_ge(ptr, ptr)\n")
	b.WriteString("declare i8 @tsnative_jsvalue_eq(ptr, ptr)\ndeclare i8 @tsnative_jsvalue_ne(ptr, ptr)\ndeclare i8 @tsnative_jsvalue_strict_eq(ptr, ptr)\ndeclare i8 @tsnative_jsvalue_strict_ne(ptr, ptr)\n")
	b.WriteString("declare ptr @tsnative_string_new(ptr, i64)\n")
	b.WriteString("declare ptr @tsnative_string_concat(ptr, ptr)\n")
	b.WriteString("declare ptr @tsnative_array_f64_new(i64)\n")
	b.WriteString("declare ptr @tsnative_array_ref_new(i64)\n")
	b.WriteString("declare ptr @tsnative_channel_f64_new_checked(double)\n")
	b.WriteString("declare i32 @tsnative_channel_f64_try_send(ptr, double)\n")
	b.WriteString("declare double @tsnative_channel_f64_try_recv_or(ptr, double)\n")
	b.WriteString("declare void @tsnative_channel_f64_send_cooperative(ptr, double)\n")
	b.WriteString("declare i32 @tsnative_channel_f64_send_task(ptr, double)\n")
	b.WriteString("declare double @tsnative_channel_f64_recv_cooperative(ptr)\n")
	b.WriteString("declare i32 @tsnative_channel_f64_recv_task(ptr, ptr)\n")
	b.WriteString("declare ptr @tsnative_channel_bool_new_checked(double)\n")
	b.WriteString("declare i32 @tsnative_channel_bool_try_send(ptr, i8)\n")
	b.WriteString("declare i8 @tsnative_channel_bool_try_recv_or(ptr, i8)\n")
	b.WriteString("declare void @tsnative_channel_bool_send_cooperative(ptr, i8)\n")
	b.WriteString("declare i32 @tsnative_channel_bool_send_task(ptr, i8)\n")
	b.WriteString("declare i8 @tsnative_channel_bool_recv_cooperative(ptr)\n")
	b.WriteString("declare i32 @tsnative_channel_bool_recv_task(ptr, ptr)\n")
	b.WriteString("declare ptr @tsnative_channel_ref_new_checked(double)\n")
	b.WriteString("declare i32 @tsnative_channel_ref_try_send(ptr, ptr)\n")
	b.WriteString("declare ptr @tsnative_channel_ref_try_recv_or(ptr, ptr)\n")
	b.WriteString("declare void @tsnative_channel_ref_send_cooperative(ptr, ptr)\n")
	b.WriteString("declare i32 @tsnative_channel_ref_send_task(ptr, ptr)\n")
	b.WriteString("declare ptr @tsnative_channel_ref_recv_cooperative(ptr)\n")
	b.WriteString("declare i32 @tsnative_channel_ref_recv_task(ptr, ptr)\n")
	b.WriteString("declare void @tsnative_sleep_cooperative(double)\n")
	b.WriteString("declare i32 @tsnative_sleep_task(double)\n")
	b.WriteString("declare void @tsnative_timer_shutdown()\ndeclare void @tsnative_blocking_pool_shutdown()\n")
	b.WriteString("declare void @tsnative_array_f64_set(ptr, i64, double)\n")
	b.WriteString("declare void @tsnative_array_f64_set_checked(ptr, double, double)\n")
	b.WriteString("declare double @tsnative_array_f64_len(ptr)\n")
	b.WriteString("declare double @tsnative_array_f64_get(ptr, double)\n")
	b.WriteString("declare void @tsnative_array_ref_set(ptr, i64, ptr)\n")
	b.WriteString("declare void @tsnative_array_ref_set_checked(ptr, double, ptr)\n")
	b.WriteString("declare double @tsnative_array_ref_len(ptr)\n")
	b.WriteString("declare ptr @tsnative_array_ref_get(ptr, double)\n")
	b.WriteString("declare ptr @tsnative_object_alloc(i64)\ndeclare ptr @tsnative_object_alloc_atomic(i64)\ndeclare ptr @tsnative_object_alloc_refs(i64, ptr, i64)\n")
	b.WriteString("declare void @tsnative_heap_shutdown()\n")
	b.WriteString("declare void @tsnative_scheduler_shutdown()\n")
	b.WriteString("declare ptr @tsnative_task_spawn_or_abort(ptr, ptr)\n")
	b.WriteString("declare ptr @tsnative_promise_resolve_f64(double)\ndeclare ptr @tsnative_promise_resolve_bool(i8)\ndeclare ptr @tsnative_promise_resolve_ref(ptr)\ndeclare ptr @tsnative_promise_reject(ptr, i32)\n")
	b.WriteString("declare ptr @tsnative_promise_all_f64(ptr, i64)\ndeclare ptr @tsnative_promise_race_f64(ptr, i64)\n")
	b.WriteString("declare ptr @tsnative_task_spawn_f64_or_abort(ptr, ptr)\n")
	b.WriteString("declare ptr @tsnative_task_spawn_bool_or_abort(ptr, ptr)\n")
	b.WriteString("declare ptr @tsnative_task_spawn_ref_or_abort(ptr, ptr)\ndeclare ptr @tsnative_task_group_new()\ndeclare ptr @tsnative_task_group_spawn_or_abort(ptr, ptr, ptr)\ndeclare ptr @tsnative_task_group_spawn_f64_or_abort(ptr, ptr, ptr)\ndeclare ptr @tsnative_task_group_spawn_bool_or_abort(ptr, ptr, ptr)\ndeclare ptr @tsnative_task_group_spawn_ref_or_abort(ptr, ptr, ptr)\ndeclare i32 @tsnative_task_group_cancel(ptr)\ndeclare i32 @tsnative_task_group_join_release(ptr)\n")
	b.WriteString("declare void @tsnative_task_join_release(ptr)\ndeclare i32 @tsnative_task_await_task(ptr)\ndeclare i32 @tsnative_task_await_task_consume(ptr)\ndeclare i8 @tsnative_task_wait_status(ptr)\ndeclare ptr @tsnative_task_failure_ref(ptr)\ndeclare i32 @tsnative_task_await_status_task(ptr, ptr)\ndeclare i32 @tsnative_task_retain(ptr)\ndeclare void @tsnative_task_release(ptr)\ndeclare i32 @tsnative_task_await_f64_task(ptr, ptr)\ndeclare i32 @tsnative_task_await_f64_shared(ptr, ptr)\ndeclare i32 @tsnative_task_await_bool_task(ptr, ptr)\ndeclare i32 @tsnative_task_await_bool_shared(ptr, ptr)\ndeclare i32 @tsnative_task_await_ref_task(ptr, ptr)\ndeclare i32 @tsnative_task_await_ref_shared(ptr, ptr)\n")
	b.WriteString("declare double @tsnative_task_join_f64(ptr)\ndeclare double @tsnative_task_join_f64_release(ptr)\n")
	b.WriteString("declare i8 @tsnative_task_join_bool(ptr)\ndeclare i8 @tsnative_task_join_bool_release(ptr)\n")
	b.WriteString("declare ptr @tsnative_task_join_ref(ptr)\ndeclare ptr @tsnative_task_join_ref_release(ptr)\n")
	b.WriteString("declare i32 @tsnative_task_cancel(ptr)\ndeclare i32 @tsnative_task_is_cancelled()\ndeclare void @tsnative_task_set_context(ptr)\ndeclare ptr @tsnative_task_get_context()\ndeclare void @tsnative_task_fail_current(ptr)\ndeclare i32 @tsnative_task_budget_poll_task()\ndeclare i32 @tsnative_task_yield_task()\ndeclare void @tsnative_task_yield()\n")
	b.WriteString("declare ptr @tsnative_gc_enter(ptr, i64)\n")
	b.WriteString("declare void @tsnative_gc_leave(ptr)\n")
	b.WriteString("declare void @tsnative_gc_handoff_begin()\n")
	b.WriteString("declare void @tsnative_gc_handoff_end()\n")
	b.WriteString("declare void @tsnative_gc_safepoint()\n")
	b.WriteString("declare void @tsnative_gc_store_ref(ptr, ptr, ptr)\n\n")
	if err := e.emitClosureTypes(&b); err != nil {
		return "", err
	}
	if err := e.emitTaskTypes(&b); err != nil {
		return "", err
	}
	shapes := append([]mir.Shape(nil), module.Shapes...)
	sort.Slice(shapes, func(i, j int) bool { return shapes[i].ID < shapes[j].ID })
	for _, shape := range shapes {
		b.WriteString(shapeTypeName(shape.ID) + " = type { ")
		written := false
		if shape.ClassTag != 0 {
			b.WriteString("i32")
			written = true
		}
		for _, field := range shape.Fields {
			if written {
				b.WriteString(", ")
			}
			typ, err := llvmType(field.Repr)
			if err != nil {
				return "", fmt.Errorf("shape s%d field %s: %w", shape.ID, field.Name, err)
			}
			b.WriteString(typ)
			written = true
		}
		b.WriteString(" }\n")
	}
	if len(shapes) != 0 {
		b.WriteString("\n")
	}
	if err := e.emitHeapTraceDescriptors(&b); err != nil {
		return "", err
	}
	if err := e.emitDynamicFieldGetHelpers(&b); err != nil {
		return "", err
	}
	if err := e.emitDynamicFieldSetHelpers(&b); err != nil {
		return "", err
	}
	if err := e.emitDynamicCallHelpers(&b); err != nil {
		return "", err
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
	if err := e.emitTaskWrappers(&b); err != nil {
		return "", err
	}
	if module.Entry != nil {
		fmt.Fprintf(&b, "define i32 @main() {\nentry:\n  call void @%s()\n  call void @tsnative_blocking_pool_shutdown()\n  call void @tsnative_timer_shutdown()\n  call void @tsnative_scheduler_shutdown()\n  call void @tsnative_heap_shutdown()\n  ret i32 0\n}\n", functionName(*module.Entry))
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
	gc := buildGCRootLayout(fn, e.shapes, e.stackObjects[fn.ID], e.stackAliases[fn.ID], e.scalarObjects[fn.ID])
	gc.emitPrologue(b, fn)
	blocks := append([]mir.Block(nil), fn.Blocks...)
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].ID < blocks[j].ID })
	for _, block := range blocks {
		fmt.Fprintf(b, "b%d:\n", block.ID)
		index := 0
		var phiRoots []mir.Instruction
		for index < len(block.Instructions) {
			inst := block.Instructions[index]
			if _, ok := inst.Op.(mir.Phi); !ok {
				break
			}
			if err := e.emitInstruction(b, fn, inst, values); err != nil {
				return fmt.Errorf("function %s block b%d: %w", fn.Name, block.ID, err)
			}
			phiRoots = append(phiRoots, inst)
			index++
		}
		if err := e.emitScalarPhis(b, fn, block, values); err != nil {
			return fmt.Errorf("function %s block b%d scalar phi: %w", fn.Name, block.ID, err)
		}
		for _, inst := range phiRoots {
			if err := gc.emitStore(b, inst, values); err != nil {
				return fmt.Errorf("function %s root v%d: %w", fn.Name, inst.Result, err)
			}
		}
		for ; index < len(block.Instructions); index++ {
			inst := block.Instructions[index]
			if emitsGCAllocation(inst.Op) && !e.isHeapAllocationElided(fn.ID, inst.Result) {
				b.WriteString("  call void @tsnative_gc_safepoint()\n")
			}
			if err := e.emitInstruction(b, fn, inst, values); err != nil {
				return fmt.Errorf("function %s block b%d: %w", fn.Name, block.ID, err)
			}
			if err := gc.emitStore(b, inst, values); err != nil {
				return fmt.Errorf("function %s root v%d: %w", fn.Name, inst.Result, err)
			}
			if err := gc.emitStackFieldSync(b, inst, values); err != nil {
				return fmt.Errorf("function %s stack-field roots v%d: %w", fn.Name, inst.Result, err)
			}
		}
		if err := e.emitTerminator(b, fn, block.Terminator, values, gc); err != nil {
			return fmt.Errorf("function %s block b%d: %w", fn.Name, block.ID, err)
		}
	}
	b.WriteString("}\n\n")
	return nil
}

func (e *emitter) emitScalarPhis(b *strings.Builder, fn mir.Function, block mir.Block, values map[mir.ValueID]string) error {
	objects := e.scalarObjects[fn.ID]
	origins := make([]mir.ValueID, 0, len(objects))
	for origin := range objects {
		origins = append(origins, origin)
	}
	sort.Slice(origins, func(i, j int) bool { return origins[i] < origins[j] })
	for _, origin := range origins {
		scalar := objects[origin]
		if !scalar.Mutable || len(scalar.Phis) == 0 {
			continue
		}
		shape, ok := e.shapes[scalar.Shape]
		if !ok {
			return fmt.Errorf("unknown scalar phi shape s%d", scalar.Shape)
		}
		phis := make([]escapeanalysis.ScalarPhi, 0)
		for _, phi := range scalar.Phis {
			if phi.Block == block.ID {
				phis = append(phis, phi)
			}
		}
		sort.Slice(phis, func(i, j int) bool { return phis[i].Name < phis[j].Name })
		for _, phi := range phis {
			if int(phi.Field) >= len(shape.Fields) {
				return fmt.Errorf("invalid scalar phi field s%d.%d", scalar.Shape, phi.Field)
			}
			repr := shape.Fields[phi.Field].Repr
			typ, err := llvmType(repr)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  %%%s = phi %s ", phi.Name, typ)
			for i, incoming := range phi.Incoming {
				if i != 0 {
					b.WriteString(", ")
				}
				value, err := scalarFieldOperand(values, incoming.Source, repr)
				if err != nil {
					return err
				}
				fmt.Fprintf(b, "[ %s, %%b%d ]", value, incoming.Block)
			}
			b.WriteString("\n")
		}
	}
	return nil
}

func scalarFieldOperand(values map[mir.ValueID]string, source escapeanalysis.ScalarFieldValue, repr mir.Repr) (string, error) {
	if source.Zero {
		return llvmZero(repr)
	}
	if source.Phi != "" {
		return "%" + source.Phi, nil
	}
	return operand(values, source.Value)
}

func (e *emitter) emitInstruction(b *strings.Builder, fn mir.Function, inst mir.Instruction, values map[mir.ValueID]string) error {
	switch op := inst.Op.(type) {
	case mir.ConstBool:
		return nil
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
	case mir.ArrayNewRef:
		if inst.Repr != mir.ReprArrayRef {
			return fmt.Errorf("array.new.ref requires arrayref result")
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_array_ref_new(i64 %d)\n", name, len(op.Elements))
		for i, element := range op.Elements {
			value, err := operand(values, element)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  call void @tsnative_array_ref_set(ptr %s, i64 %d, ptr %s)\n", name, i, value)
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
	case mir.ArraySetF64:
		array, err := operand(values, op.Array)
		if err != nil {
			return err
		}
		index, err := operand(values, op.Index)
		if err != nil {
			return err
		}
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call void @tsnative_array_f64_set_checked(ptr %s, double %s, double %s)\n", array, index, value)
		values[inst.Result] = value
		return nil
	case mir.ArrayLengthRef:
		array, err := operand(values, op.Array)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = call double @tsnative_array_ref_len(ptr %s)\n", valueName(inst.Result), array)
		return nil
	case mir.ArrayGetRef:
		array, err := operand(values, op.Array)
		if err != nil {
			return err
		}
		index, err := operand(values, op.Index)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = call ptr @tsnative_array_ref_get(ptr %s, double %s)\n", valueName(inst.Result), array, index)
		return nil
	case mir.ArraySetRef:
		array, err := operand(values, op.Array)
		if err != nil {
			return err
		}
		index, err := operand(values, op.Index)
		if err != nil {
			return err
		}
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call void @tsnative_array_ref_set_checked(ptr %s, double %s, ptr %s)\n", array, index, value)
		values[inst.Result] = value
		return nil
	case mir.ObjectNew:
		shape, ok := e.shapes[op.Shape]
		if !ok {
			return fmt.Errorf("unknown object shape s%d", op.Shape)
		}
		name := valueName(inst.Result)
		typeName := shapeTypeName(op.Shape)
		if _, ok := e.scalarObjects.Get(fn.ID, inst.Result); ok {
			return nil
		}
		if e.isStackObject(fn.ID, inst.Result) {
			fmt.Fprintf(b, "  %s = alloca %s\n", name, typeName)
		} else {
			fmt.Fprintf(b, "  %s.sizeptr = getelementptr %s, ptr null, i32 1\n", name, typeName)
			fmt.Fprintf(b, "  %s.size = ptrtoint ptr %s.sizeptr to i64\n", name, name)
			shapeRefs := shapeRefFields(shape)
			emitHeapObjectAlloc(b, name, name+".size", shapeRefDescriptorName(op.Shape), len(shapeRefs))
		}
		if shape.ClassTag != 0 {
			fmt.Fprintf(b, "  %s.tag = getelementptr %s, ptr %s, i32 0, i32 0\n", name, typeName, name)
			fmt.Fprintf(b, "  store i32 %d, ptr %s.tag\n", shape.ClassTag, name)
		}
		for i, fieldValue := range op.Fields {
			value, err := operand(values, fieldValue)
			if err != nil {
				return err
			}
			fieldType, err := llvmType(shape.Fields[i].Repr)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  %s.f%d = getelementptr %s, ptr %s, i32 0, i32 %d\n", name, i, typeName, name, shapeFieldIndex(shape, uint32(i)))
			fmt.Fprintf(b, "  store %s %s, ptr %s.f%d\n", fieldType, value, name, i)
		}
		return nil
	case mir.ObjectAlloc:
		shape, ok := e.shapes[op.Shape]
		if !ok {
			return fmt.Errorf("unknown object shape s%d", op.Shape)
		}
		name := valueName(inst.Result)
		typeName := shapeTypeName(op.Shape)
		if _, ok := e.scalarObjects.Get(fn.ID, inst.Result); ok {
			return nil
		}
		if e.isStackObject(fn.ID, inst.Result) {
			fmt.Fprintf(b, "  %s = alloca %s\n", name, typeName)
		} else {
			fmt.Fprintf(b, "  %s.sizeptr = getelementptr %s, ptr null, i32 1\n", name, typeName)
			fmt.Fprintf(b, "  %s.size = ptrtoint ptr %s.sizeptr to i64\n", name, name)
			shapeRefs := shapeRefFields(shape)
			emitHeapObjectAlloc(b, name, name+".size", shapeRefDescriptorName(op.Shape), len(shapeRefs))
		}
		if shape.ClassTag != 0 {
			fmt.Fprintf(b, "  %s.tag = getelementptr %s, ptr %s, i32 0, i32 0\n", name, typeName, name)
			fmt.Fprintf(b, "  store i32 %d, ptr %s.tag\n", shape.ClassTag, name)
		}
		for i, field := range shape.Fields {
			fieldType, err := llvmType(field.Repr)
			if err != nil {
				return err
			}
			zero, err := llvmZero(field.Repr)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  %s.f%d = getelementptr %s, ptr %s, i32 0, i32 %d\n", name, i, typeName, name, shapeFieldIndex(shape, uint32(i)))
			fmt.Fprintf(b, "  store %s %s, ptr %s.f%d\n", fieldType, zero, name, i)
		}
		return nil
	case mir.DynamicFieldGet:
		object, err := operand(values, op.Object)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @%s(ptr %s)\n", name, dynamicFieldGetHelperName(op.Field), object)
		values[inst.Result] = name
		return nil
	case mir.DynamicFieldSet:
		object, err := operand(values, op.Object)
		if err != nil {
			return err
		}
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @%s(ptr %s, ptr %s)\n", name, dynamicFieldSetHelperName(op.Field), object, value)
		values[inst.Result] = name
		return nil
	case mir.FieldSet:
		shape, ok := e.shapes[op.Shape]
		if !ok || int(op.Field) >= len(shape.Fields) {
			return fmt.Errorf("invalid field store s%d.%d", op.Shape, op.Field)
		}
		if scalar, ok := e.scalarObjects.Get(fn.ID, op.Object); ok {
			if !scalar.Mutable {
				return fmt.Errorf("immutable scalar object v%d reached field mutation", op.Object)
			}
			value, err := operand(values, op.Value)
			if err != nil {
				return err
			}
			values[inst.Result] = value
			return nil
		}
		object, err := operand(values, op.Object)
		if err != nil {
			return err
		}
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		typeName := shapeTypeName(op.Shape)
		fieldType, err := llvmType(shape.Fields[op.Field].Repr)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s.ptr = getelementptr %s, ptr %s, i32 0, i32 %d\n", name, typeName, object, shapeFieldIndex(shape, op.Field))
		if isGCHeapReferenceRepr(shape.Fields[op.Field].Repr) && !e.isStackObject(fn.ID, op.Object) {
			fmt.Fprintf(b, "  call void @tsnative_gc_store_ref(ptr %s, ptr %s.ptr, ptr %s)\n", object, name, value)
		} else {
			fmt.Fprintf(b, "  store %s %s, ptr %s.ptr\n", fieldType, value, name)
		}
		values[inst.Result] = value
		return nil
	case mir.PromiseResolve:
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		switch op.Result {
		case mir.ReprF64:
			fmt.Fprintf(b, "  %s = call ptr @tsnative_promise_resolve_f64(double %s)\n", name, value)
		case mir.ReprBool:
			fmt.Fprintf(b, "  %s.bool = zext i1 %s to i8\n", name, value)
			fmt.Fprintf(b, "  %s = call ptr @tsnative_promise_resolve_bool(i8 %s.bool)\n", name, name)
		case mir.ReprJSValue:
			fmt.Fprintf(b, "  %s = call ptr @tsnative_promise_resolve_ref(ptr %s)\n", name, value)
		default:
			return fmt.Errorf("Promise.resolve has unsupported result representation %d", op.Result)
		}
		values[inst.Result] = name
		return nil
	case mir.PromiseAdopt:
		promise, err := operand(values, op.Promise)
		if err != nil {
			return err
		}
		values[inst.Result] = promise
		return nil
	case mir.PromiseAllF64:
		name := valueName(inst.Result)
		if len(op.Promises) == 0 {
			fmt.Fprintf(b, "  %s = call ptr @tsnative_promise_all_f64(ptr null, i64 0)\n", name)
		} else {
			fmt.Fprintf(b, "  %s.args = alloca [%d x ptr]\n", name, len(op.Promises))
			for i, promiseID := range op.Promises {
				promise, err := operand(values, promiseID)
				if err != nil {
					return err
				}
				fmt.Fprintf(b, "  %s.arg%d = getelementptr [%d x ptr], ptr %s.args, i32 0, i32 %d\n", name, i, len(op.Promises), name, i)
				fmt.Fprintf(b, "  store ptr %s, ptr %s.arg%d\n", promise, name, i)
			}
			fmt.Fprintf(b, "  %s.base = getelementptr [%d x ptr], ptr %s.args, i32 0, i32 0\n", name, len(op.Promises), name)
			fmt.Fprintf(b, "  %s = call ptr @tsnative_promise_all_f64(ptr %s.base, i64 %d)\n", name, name, len(op.Promises))
		}
		values[inst.Result] = name
		return nil
	case mir.PromiseRaceF64:
		name := valueName(inst.Result)
		if len(op.Promises) == 0 {
			fmt.Fprintf(b, "  %s = call ptr @tsnative_promise_race_f64(ptr null, i64 0)\n", name)
		} else {
			fmt.Fprintf(b, "  %s.args = alloca [%d x ptr]\n", name, len(op.Promises))
			for i, promiseID := range op.Promises {
				promise, err := operand(values, promiseID)
				if err != nil {
					return err
				}
				fmt.Fprintf(b, "  %s.arg%d = getelementptr [%d x ptr], ptr %s.args, i32 0, i32 %d\n", name, i, len(op.Promises), name, i)
				fmt.Fprintf(b, "  store ptr %s, ptr %s.arg%d\n", promise, name, i)
			}
			fmt.Fprintf(b, "  %s.base = getelementptr [%d x ptr], ptr %s.args, i32 0, i32 0\n", name, len(op.Promises), name)
			fmt.Fprintf(b, "  %s = call ptr @tsnative_promise_race_f64(ptr %s.base, i64 %d)\n", name, name, len(op.Promises))
		}
		values[inst.Result] = name
		return nil
	case mir.PromiseReject:
		reason, err := operand(values, op.Reason)
		if err != nil {
			return err
		}
		kind := 0
		switch op.Result {
		case mir.ReprF64:
			kind = 1
		case mir.ReprBool:
			kind = 2
		case mir.ReprJSValue:
			kind = 3
		default:
			return fmt.Errorf("Promise.reject has unsupported result representation %d", op.Result)
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_promise_reject(ptr %s, i32 %d)\n", name, reason, kind)
		values[inst.Result] = name
		return nil
	case mir.TaskSpawn:
		return e.emitTaskSpawn(b, inst, op, values)
	case mir.TaskWait:
		task, err := operand(values, op.Task)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s.raw = call i8 @tsnative_task_wait_status(ptr %s)\n", name, task)
		fmt.Fprintf(b, "  %s = trunc i8 %s.raw to i1\n", name, name)
		values[inst.Result] = name
		return nil
	case mir.TaskFailure:
		task, err := operand(values, op.Task)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_task_failure_ref(ptr %s)\n", name, task)
		values[inst.Result] = name
		return nil
	case mir.TaskRetain:
		task, err := operand(values, op.Task)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call i32 @tsnative_task_retain(ptr %s)\n", task)
		return nil
	case mir.TaskRelease:
		task, err := operand(values, op.Task)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call void @tsnative_task_release(ptr %s)\n", task)
		return nil
	case mir.TaskJoin:
		task, err := operand(values, op.Task)
		if err != nil {
			return err
		}
		switch inst.Repr {
		case mir.ReprVoid:
			if !op.Shared {
				fmt.Fprintf(b, "  call void @tsnative_task_join_release(ptr %s)\n", task)
			} else {
				fmt.Fprintf(b, "  call i32 @tsnative_task_join(ptr %s)\n", task)
			}
		case mir.ReprBool:
			name := valueName(inst.Result)
			join := "tsnative_task_join_bool"
			if !op.Shared {
				join = "tsnative_task_join_bool_release"
			}
			fmt.Fprintf(b, "  %s.raw = call i8 @%s(ptr %s)\n", name, join, task)
			fmt.Fprintf(b, "  %s = trunc i8 %s.raw to i1\n", name, name)
			values[inst.Result] = name
		case mir.ReprF64:
			name := valueName(inst.Result)
			join := "tsnative_task_join_f64"
			if !op.Shared {
				join = "tsnative_task_join_f64_release"
			}
			fmt.Fprintf(b, "  %s = call double @%s(ptr %s)\n", name, join, task)
			values[inst.Result] = name
		case mir.ReprStringRef, mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef, mir.ReprJSValue:
			name := valueName(inst.Result)
			join := "tsnative_task_join_ref"
			if !op.Shared {
				join = "tsnative_task_join_ref_release"
			}
			fmt.Fprintf(b, "  %s = call ptr @%s(ptr %s)\n", name, join, task)
			values[inst.Result] = name
		default:
			return fmt.Errorf("unsupported task join result representation %d", inst.Repr)
		}
		return nil
	case mir.TaskYield:
		b.WriteString("  call void @tsnative_task_yield()\n")
		return nil
	case mir.TaskCancel:
		task, err := operand(values, op.Task)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call i32 @tsnative_task_cancel(ptr %s)\n", task)
		return nil
	case mir.TaskCancelled:
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s.raw = call i32 @tsnative_task_is_cancelled()\n", name)
		fmt.Fprintf(b, "  %s = icmp ne i32 %s.raw, 0\n", name, name)
		values[inst.Result] = name
		return nil
	case mir.TaskGroupNew:
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_task_group_new()\n", name)
		values[inst.Result] = name
		return nil
	case mir.TaskGroupJoin:
		group, err := operand(values, op.Group)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call i32 @tsnative_task_group_join_release(ptr %s)\n", group)
		return nil
	case mir.TaskGroupCancel:
		group, err := operand(values, op.Group)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call i32 @tsnative_task_group_cancel(ptr %s)\n", group)
		return nil
	case mir.TaskContextSet:
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call void @tsnative_task_set_context(ptr %s)\n", value)
		return nil
	case mir.TaskContextGet:
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_task_get_context()\n", name)
		values[inst.Result] = name
		return nil
	case mir.ChannelNewF64:
		capacity, err := operand(values, op.Capacity)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_channel_f64_new_checked(double %s)\n", name, capacity)
		values[inst.Result] = name
		return nil
	case mir.ChannelTrySendF64:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s.raw = call i32 @tsnative_channel_f64_try_send(ptr %s, double %s)\n", name, channel, value)
		fmt.Fprintf(b, "  %s = icmp eq i32 %s.raw, 1\n", name, name)
		values[inst.Result] = name
		return nil
	case mir.ChannelTryRecvOrF64:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		fallback, err := operand(values, op.Fallback)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call double @tsnative_channel_f64_try_recv_or(ptr %s, double %s)\n", name, channel, fallback)
		values[inst.Result] = name
		return nil
	case mir.ChannelSendF64:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call void @tsnative_channel_f64_send_cooperative(ptr %s, double %s)\n", channel, value)
		return nil
	case mir.ChannelRecvF64:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call double @tsnative_channel_f64_recv_cooperative(ptr %s)\n", name, channel)
		values[inst.Result] = name
		return nil
	case mir.ChannelNewBool:
		capacity, err := operand(values, op.Capacity)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_channel_bool_new_checked(double %s)\n", name, capacity)
		values[inst.Result] = name
		return nil
	case mir.ChannelTrySendBool:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s.value = zext i1 %s to i8\n", name, value)
		fmt.Fprintf(b, "  %s.raw = call i32 @tsnative_channel_bool_try_send(ptr %s, i8 %s.value)\n", name, channel, name)
		fmt.Fprintf(b, "  %s = icmp eq i32 %s.raw, 1\n", name, name)
		values[inst.Result] = name
		return nil
	case mir.ChannelTryRecvOrBool:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		fallback, err := operand(values, op.Fallback)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s.fallback = zext i1 %s to i8\n", name, fallback)
		fmt.Fprintf(b, "  %s.raw = call i8 @tsnative_channel_bool_try_recv_or(ptr %s, i8 %s.fallback)\n", name, channel, name)
		fmt.Fprintf(b, "  %s = trunc i8 %s.raw to i1\n", name, name)
		values[inst.Result] = name
		return nil
	case mir.ChannelSendBool:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		tmp := valueName(inst.Result) + ".value"
		fmt.Fprintf(b, "  %s = zext i1 %s to i8\n", tmp, value)
		fmt.Fprintf(b, "  call void @tsnative_channel_bool_send_cooperative(ptr %s, i8 %s)\n", channel, tmp)
		return nil
	case mir.ChannelRecvBool:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s.raw = call i8 @tsnative_channel_bool_recv_cooperative(ptr %s)\n", name, channel)
		fmt.Fprintf(b, "  %s = trunc i8 %s.raw to i1\n", name, name)
		values[inst.Result] = name
		return nil
	case mir.ChannelNewRef:
		capacity, err := operand(values, op.Capacity)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_channel_ref_new_checked(double %s)\n", name, capacity)
		values[inst.Result] = name
		return nil
	case mir.ChannelTrySendRef:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s.raw = call i32 @tsnative_channel_ref_try_send(ptr %s, ptr %s)\n", name, channel, value)
		fmt.Fprintf(b, "  %s = icmp eq i32 %s.raw, 1\n", name, name)
		values[inst.Result] = name
		return nil
	case mir.ChannelTryRecvOrRef:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		fallback, err := operand(values, op.Fallback)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_channel_ref_try_recv_or(ptr %s, ptr %s)\n", name, channel, fallback)
		values[inst.Result] = name
		return nil
	case mir.ChannelSendRef:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call void @tsnative_channel_ref_send_cooperative(ptr %s, ptr %s)\n", channel, value)
		return nil
	case mir.ChannelRecvRef:
		channel, err := operand(values, op.Channel)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_channel_ref_recv_cooperative(ptr %s)\n", name, channel)
		values[inst.Result] = name
		return nil
	case mir.Sleep:
		duration, err := operand(values, op.Duration)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call void @tsnative_sleep_cooperative(double %s)\n", duration)
		return nil
	case mir.ClosureNew:
		return e.emitClosureNew(b, fn.ID, inst, op, values)
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
		if scalar, ok := e.scalarObjects.Get(fn.ID, op.Object); ok {
			var value string
			if scalar.Mutable {
				read, ok := scalar.Reads[inst.Result]
				if !ok {
					return fmt.Errorf("mutable scalar object v%d missing read plan for v%d", op.Object, inst.Result)
				}
				if read.Phi != "" {
					values[inst.Result] = "%" + read.Phi
					return nil
				}
				if read.Zero {
					zero, err := llvmZero(shape.Fields[op.Field].Repr)
					if err != nil {
						return err
					}
					value = zero
				} else {
					var err error
					value, err = operand(values, read.Value)
					if err != nil {
						return err
					}
				}
			} else if scalar.ZeroInitialized {
				zero, err := llvmZero(shape.Fields[op.Field].Repr)
				if err != nil {
					return err
				}
				value = zero
			} else {
				if int(op.Field) >= len(scalar.Fields) {
					return fmt.Errorf("scalar object v%d missing field %d", op.Object, op.Field)
				}
				var err error
				value, err = operand(values, scalar.Fields[op.Field])
				if err != nil {
					return err
				}
			}
			values[inst.Result] = value
			return nil
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
		fmt.Fprintf(b, "  %s.ptr = getelementptr %s, ptr %s, i32 0, i32 %d\n", name, typeName, object, shapeFieldIndex(shape, op.Field))
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
	case mir.ConstJSValue:
		name := valueName(inst.Result)
		switch op.Kind {
		case mir.ConstJSNull:
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_null()\n", name)
		case mir.ConstJSUndefined:
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_undefined()\n", name)
		default:
			return fmt.Errorf("unsupported JSValue const kind %d", op.Kind)
		}
		values[inst.Result] = name
		return nil
	case mir.BoxJSValue:
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		switch op.Kind {
		case mir.BoxJSNumber:
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_f64(double %s)\n", name, value)
		case mir.BoxJSString:
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_string(ptr %s)\n", name, value)
		case mir.BoxJSBoolean:
			fmt.Fprintf(b, "  %s.bool = zext i1 %s to i8\n", name, value)
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_bool(i8 %s.bool)\n", name, name)
		case mir.BoxJSObject:
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_object_shape(ptr %s, i32 %d)\n", name, value, uint32(op.Shape))
		case mir.BoxJSArray:
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_array(ptr %s)\n", name, value)
		case mir.BoxJSFunction:
			if !op.HasFunction {
				return fmt.Errorf("function JSValue box has no native target metadata")
			}
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_box_function_target(ptr %s, i32 %d)\n", name, value, uint32(op.Function))
		default:
			return fmt.Errorf("unsupported JSValue box kind %d", op.Kind)
		}
		values[inst.Result] = name
		return nil
	case mir.UnboxJSValue:
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		switch op.Kind {
		case mir.UnboxJSNumber:
			fmt.Fprintf(b, "  %s = call double @tsnative_jsvalue_unbox_f64(ptr %s)\n", name, value)
		case mir.UnboxJSString:
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_string(ptr %s)\n", name, value)
		case mir.UnboxJSBoolean:
			fmt.Fprintf(b, "  %s.raw = call i8 @tsnative_jsvalue_unbox_bool(ptr %s)\n", name, value)
			fmt.Fprintf(b, "  %s = trunc i8 %s.raw to i1\n", name, name)
		case mir.UnboxJSArray:
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_array(ptr %s)\n", name, value)
		default:
			return fmt.Errorf("unsupported JSValue unbox kind %d", op.Kind)
		}
		values[inst.Result] = name
		return nil
	case mir.DynamicAddJSValue:
		left, err := operand(values, op.Left)
		if err != nil {
			return err
		}
		right, err := operand(values, op.Right)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_add(ptr %s, ptr %s)\n", name, left, right)
		values[inst.Result] = name
		return nil
	case mir.DynamicBinaryJSValue:
		left, err := operand(values, op.Left)
		if err != nil {
			return err
		}
		right, err := operand(values, op.Right)
		if err != nil {
			return err
		}
		name := valueName(inst.Result)
		floatFn := map[mir.DynamicJSBinaryOp]string{mir.DynamicJSSub: "tsnative_jsvalue_sub", mir.DynamicJSMul: "tsnative_jsvalue_mul", mir.DynamicJSDiv: "tsnative_jsvalue_div"}[op.Operator]
		if floatFn != "" {
			fmt.Fprintf(b, "  %s = call double @%s(ptr %s, ptr %s)\n", name, floatFn, left, right)
			values[inst.Result] = name
			return nil
		}
		boolFn := map[mir.DynamicJSBinaryOp]string{
			mir.DynamicJSLessThan: "tsnative_jsvalue_lt", mir.DynamicJSLessEqual: "tsnative_jsvalue_le",
			mir.DynamicJSGreaterThan: "tsnative_jsvalue_gt", mir.DynamicJSGreaterEqual: "tsnative_jsvalue_ge",
			mir.DynamicJSEqual: "tsnative_jsvalue_eq", mir.DynamicJSNotEqual: "tsnative_jsvalue_ne",
			mir.DynamicJSStrictEqual: "tsnative_jsvalue_strict_eq", mir.DynamicJSStrictNotEqual: "tsnative_jsvalue_strict_ne",
		}[op.Operator]
		if boolFn == "" {
			return fmt.Errorf("unsupported dynamic JS binary operator %d", op.Operator)
		}
		fmt.Fprintf(b, "  %s.raw = call i8 @%s(ptr %s, ptr %s)\n", name, boolFn, left, right)
		fmt.Fprintf(b, "  %s = trunc i8 %s.raw to i1\n", name, name)
		values[inst.Result] = name
		return nil
	case mir.ProvenIntBinary:
		return e.emitProvenIntBinary(b, inst, op, values)
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
	case mir.DynamicCall:
		callee, err := operand(values, op.Callee)
		if err != nil {
			return err
		}
		args := make([]string, len(op.Args))
		for i, arg := range op.Args {
			value, err := operand(values, arg)
			if err != nil {
				return err
			}
			args[i] = value
		}
		name := valueName(inst.Result)
		fmt.Fprintf(b, "  %s = call ptr @%s(ptr %s", name, dynamicCallHelperName(len(args)), callee)
		for _, arg := range args {
			fmt.Fprintf(b, ", ptr %s", arg)
		}
		b.WriteString(")\n")
		values[inst.Result] = name
		return nil
	case mir.Call:
		return e.emitCall(b, inst, op, values)
	case mir.DispatchCall:
		return e.emitDispatchCall(b, inst, op, values)
	case mir.IntrinsicCall:
		return e.emitIntrinsicCall(b, inst, op, values)
	default:
		return fmt.Errorf("unsupported MIR operation %T", inst.Op)
	}
}

func (e *emitter) stackObjectOrigin(fn mir.FunctionID, value mir.ValueID) (mir.ValueID, bool) {
	if e.stackObjects.Contains(fn, value) {
		return value, true
	}
	origin, ok := e.stackAliases[fn][value]
	return origin, ok
}

func (e *emitter) isStackObject(fn mir.FunctionID, value mir.ValueID) bool {
	_, ok := e.stackObjectOrigin(fn, value)
	return ok
}

func (e *emitter) isHeapAllocationElided(fn mir.FunctionID, value mir.ValueID) bool {
	if e.isStackObject(fn, value) {
		return true
	}
	_, ok := e.scalarObjects.Get(fn, value)
	return ok
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
	if callee.ReturnRepr == mir.ReprVoid {
		fmt.Fprintf(b, "  call %s @%s(", retType, functionName(callee.ID))
	} else {
		fmt.Fprintf(b, "  %s = call %s @%s(", name, retType, functionName(callee.ID))
	}
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
	if callee.ReturnRepr != mir.ReprVoid {
		values[inst.Result] = name
	}
	return nil
}

func (e *emitter) emitDispatchCall(b *strings.Builder, inst mir.Instruction, call mir.DispatchCall, values map[mir.ValueID]string) error {
	if len(call.Args) == 0 || len(call.Cases) == 0 {
		return fmt.Errorf("dispatch requires receiver and targets")
	}
	receiver, err := operand(values, call.Args[0])
	if err != nil {
		return err
	}
	base, ok := e.functions[call.Cases[0].Callee]
	if !ok {
		return fmt.Errorf("dispatch references unknown callee f%d", call.Cases[0].Callee)
	}
	if len(base.Params) != len(call.Args) {
		return fmt.Errorf("dispatch f%d has %d args; expected %d", base.ID, len(call.Args), len(base.Params))
	}
	for _, target := range call.Cases[1:] {
		fn, ok := e.functions[target.Callee]
		if !ok {
			return fmt.Errorf("dispatch references unknown callee f%d", target.Callee)
		}
		if !sameDispatchSignature(base, fn) {
			return fmt.Errorf("dispatch targets f%d and f%d have incompatible native signatures", base.ID, fn.ID)
		}
	}
	name := valueName(inst.Result)
	fmt.Fprintf(b, "  %s.tag = load i32, ptr %s\n", name, receiver)
	selected := "@" + functionName(base.ID)
	for i, target := range call.Cases[1:] {
		fmt.Fprintf(b, "  %s.tagcmp.%d = icmp eq i32 %s.tag, %d\n", name, i, name, target.ClassTag)
		choice := fmt.Sprintf("%s.fn.%d", name, i)
		fmt.Fprintf(b, "  %s = select i1 %s.tagcmp.%d, ptr @%s, ptr %s\n", choice, name, i, functionName(target.Callee), selected)
		selected = choice
	}
	retType, err := llvmType(base.ReturnRepr)
	if err != nil {
		return err
	}
	if base.ReturnRepr == mir.ReprVoid {
		fmt.Fprintf(b, "  call %s %s(", retType, selected)
	} else {
		fmt.Fprintf(b, "  %s = call %s %s(", name, retType, selected)
	}
	for i, arg := range call.Args {
		if i != 0 {
			b.WriteString(", ")
		}
		typ, err := llvmType(base.Params[i].Repr)
		if err != nil {
			return err
		}
		value, err := operand(values, arg)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "%s %s", typ, value)
	}
	b.WriteString(")\n")
	if base.ReturnRepr != mir.ReprVoid {
		values[inst.Result] = name
	}
	return nil
}

func sameDispatchSignature(left, right mir.Function) bool {
	if left.ReturnRepr != right.ReturnRepr || len(left.Params) != len(right.Params) {
		return false
	}
	for i := range left.Params {
		if left.Params[i].Repr != right.Params[i].Repr {
			return false
		}
	}
	return true
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
	case mir.IntrinsicConsoleLogJSValue:
		fmt.Fprintf(b, "  call void @tsnative_console_log_jsvalue(ptr %s)\n", arg)
	default:
		return fmt.Errorf("unsupported intrinsic %d", call.Intrinsic)
	}
	return nil
}

func (e *emitter) emitTerminator(b *strings.Builder, fn mir.Function, term mir.Terminator, values map[mir.ValueID]string, gc gcRootLayout) error {
	switch term := term.(type) {
	case mir.Return:
		if term.Value == nil {
			if gc.count != 0 {
				b.WriteString("  call void @tsnative_gc_leave(ptr %gc.frame)\n")
			}
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
		if gc.count != 0 {
			b.WriteString("  call void @tsnative_gc_leave(ptr %gc.frame)\n")
		}
		fmt.Fprintf(b, "  ret %s %s\n", typ, op)
		return nil
	case mir.Throw:
		value, err := operand(values, term.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  call void @tsnative_task_fail_current(ptr %s)\n", value)
		if gc.count != 0 {
			b.WriteString("  call void @tsnative_gc_leave(ptr %gc.frame)\n")
		}
		if fn.ReturnRepr == mir.ReprVoid {
			b.WriteString("  ret void\n")
			return nil
		}
		typ, err := llvmType(fn.ReturnRepr)
		if err != nil {
			return err
		}
		zero, err := llvmZero(fn.ReturnRepr)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  ret %s %s\n", typ, zero)
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

func llvmZero(repr mir.Repr) (string, error) {
	switch repr {
	case mir.ReprBool, mir.ReprI32, mir.ReprI64, mir.ReprTagged:
		return "0", nil
	case mir.ReprF64:
		return "0.000000e+00", nil
	case mir.ReprJSValue:
		return "null", nil
	case mir.ReprStringRef, mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef, mir.ReprTaskRef, mir.ReprChannelRef, mir.ReprTaskGroupRef:
		return "null", nil
	default:
		return "", fmt.Errorf("no zero initializer for representation %d", repr)
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
	case mir.ReprStringRef, mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef, mir.ReprTaskRef, mir.ReprChannelRef, mir.ReprTaskGroupRef, mir.ReprJSValue:
		return "ptr", nil
	case mir.ReprTagged:
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
			if constant, ok := inst.Op.(mir.ConstBool); ok {
				if constant.Value {
					values[inst.Result] = "true"
				} else {
					values[inst.Result] = "false"
				}
			} else if constant, ok := inst.Op.(mir.ConstF64); ok {
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

func isGCHeapReferenceRepr(repr mir.Repr) bool {
	switch repr {
	case mir.ReprStringRef, mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef, mir.ReprChannelRef, mir.ReprJSValue:
		return true
	default:
		return false
	}
}

func shapeFieldIndex(shape mir.Shape, field uint32) uint32 {
	if shape.ClassTag != 0 {
		return field + 1
	}
	return field
}

func (e *emitter) emitProvenIntBinary(b *strings.Builder, inst mir.Instruction, op mir.ProvenIntBinary, values map[mir.ValueID]string) error {
	if inst.Repr != mir.ReprF64 {
		return fmt.Errorf("proven integer v%d must preserve f64 result", inst.Result)
	}
	left, err := operand(values, op.Left)
	if err != nil {
		return err
	}
	right, err := operand(values, op.Right)
	if err != nil {
		return err
	}
	intType := "i32"
	if op.Width == mir.IntWidth64 {
		intType = "i64"
	} else if op.Width != mir.IntWidth32 {
		return fmt.Errorf("invalid proven integer width %d", op.Width)
	}
	opcode := map[mir.FloatBinaryOp]string{mir.FloatAdd: "add", mir.FloatSub: "sub", mir.FloatMul: "mul", mir.FloatDiv: "sdiv"}[op.Operator]
	if opcode == "" {
		return fmt.Errorf("unsupported proven integer operator %d", op.Operator)
	}
	name := valueName(inst.Result)
	fmt.Fprintf(b, "  %s.lhs.int = fptosi double %s to %s\n", name, left, intType)
	fmt.Fprintf(b, "  %s.rhs.int = fptosi double %s to %s\n", name, right, intType)
	fmt.Fprintf(b, "  %s.int = %s %s %s.lhs.int, %s.rhs.int\n", name, opcode, intType, name, name)
	fmt.Fprintf(b, "  %s = sitofp %s %s.int to double\n", name, intType, name)
	values[inst.Result] = name
	return nil
}
