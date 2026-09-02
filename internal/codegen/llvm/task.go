package llvm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/mir"
)

type taskDescriptor struct {
	Callee       mir.FunctionID
	CaptureCount int
	Continuation *taskContinuation
}

func taskTargetHasSuspension(fn mir.Function) bool {
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			switch inst.Op.(type) {
			case mir.TaskJoin, mir.ChannelSendF64, mir.ChannelRecvF64, mir.ChannelSendBool, mir.ChannelRecvBool, mir.ChannelSendRef, mir.ChannelRecvRef, mir.Sleep:
				return true
			}
		}
	}
	return false
}

func taskContinuationFor(fn mir.Function) (*taskContinuation, error) {
	continuation := analyzeTaskContinuation(fn)
	if taskTargetHasSuspension(fn) && continuation == nil {
		return nil, fmt.Errorf("task target %s contains a blocking operation that could not be lowered to a resumable continuation", fn.Name)
	}
	return continuation, nil
}

func (e *emitter) taskDescriptors() ([]taskDescriptor, error) {
	byCallee := map[mir.FunctionID]taskDescriptor{}
	for _, fn := range e.module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				spawn, ok := inst.Op.(mir.TaskSpawn)
				if !ok {
					continue
				}
				target, ok := e.functions[spawn.Callee]
				if !ok {
					return nil, fmt.Errorf("task target f%d is missing", spawn.Callee)
				}
				if len(spawn.Captures) > len(target.Params) {
					return nil, fmt.Errorf("task f%d captures %d values but target has %d params", spawn.Callee, len(spawn.Captures), len(target.Params))
				}
				continuation, err := taskContinuationFor(target)
				if err != nil {
					return nil, err
				}
				descriptor := taskDescriptor{Callee: spawn.Callee, CaptureCount: len(spawn.Captures), Continuation: continuation}
				if existing, ok := byCallee[spawn.Callee]; ok && existing.CaptureCount != descriptor.CaptureCount {
					return nil, fmt.Errorf("task f%d has inconsistent capture counts", spawn.Callee)
				}
				byCallee[spawn.Callee] = descriptor
			}
		}
	}
	result := make([]taskDescriptor, 0, len(byCallee))
	for _, descriptor := range byCallee {
		result = append(result, descriptor)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Callee < result[j].Callee })
	return result, nil
}

func taskWrapperName(id mir.FunctionID) string { return fmt.Sprintf("tsnative_task_entry_f%d", id) }
func taskEnvTypeName(id mir.FunctionID) string { return fmt.Sprintf("%%tsnative_task_env_f%d", id) }

func (e *emitter) emitTaskTypes(b *strings.Builder) error {
	descriptors, err := e.taskDescriptors()
	if err != nil {
		return err
	}
	for _, descriptor := range descriptors {
		if descriptor.CaptureCount == 0 && descriptor.Continuation == nil {
			continue
		}
		fn := e.functions[descriptor.Callee]
		fmt.Fprintf(b, "%s = type { ", taskEnvTypeName(descriptor.Callee))
		written := false
		for i := 0; i < descriptor.CaptureCount; i++ {
			if written {
				b.WriteString(", ")
			}
			typ, err := llvmType(fn.Params[i].Repr)
			if err != nil {
				return err
			}
			b.WriteString(typ)
			written = true
		}
		if descriptor.Continuation != nil {
			if written {
				b.WriteString(", ")
			}
			b.WriteString("i32")
			written = true
			for _, value := range descriptor.Continuation.sortedSpillValues() {
				slot := descriptor.Continuation.SpillSlots[value]
				typ, err := llvmType(slot.Repr)
				if err != nil {
					return err
				}
				b.WriteString(", ")
				b.WriteString(typ)
			}
		}
		b.WriteString(" }\n")
	}
	if len(descriptors) != 0 {
		b.WriteString("\n")
	}
	return nil
}

func (e *emitter) emitTaskWrappers(b *strings.Builder) error {
	descriptors, err := e.taskDescriptors()
	if err != nil {
		return err
	}
	for _, descriptor := range descriptors {
		fn := e.functions[descriptor.Callee]
		if (fn.ReturnRepr != mir.ReprVoid && fn.ReturnRepr != mir.ReprBool && fn.ReturnRepr != mir.ReprF64 && !isTaskReferenceResult(fn.ReturnRepr)) || len(fn.Params) != descriptor.CaptureCount {
			return fmt.Errorf("task target f%d must be a zero-argument source closure with a supported native result", descriptor.Callee)
		}
		fmt.Fprintf(b, "define void @%s(ptr %%state, ptr %%result_slot) {\nentry:\n", taskWrapperName(descriptor.Callee))
		if descriptor.Continuation != nil {
			if err := e.emitContinuationTaskWrapper(b, descriptor, fn); err != nil {
				return err
			}
			continue
		}
		for i := 0; i < descriptor.CaptureCount; i++ {
			typ, err := llvmType(fn.Params[i].Repr)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  %%capture%d.ptr = getelementptr %s, ptr %%state, i32 0, i32 %d\n", i, taskEnvTypeName(descriptor.Callee), i)
			fmt.Fprintf(b, "  %%capture%d = load %s, ptr %%capture%d.ptr\n", i, typ, i)
		}
		prefix := "  "
		if fn.ReturnRepr == mir.ReprBool || fn.ReturnRepr == mir.ReprF64 || isTaskReferenceResult(fn.ReturnRepr) {
			prefix = "  %result = "
		}
		if isTaskReferenceResult(fn.ReturnRepr) {
			b.WriteString("  call void @tsnative_gc_handoff_begin()\n")
		}
		retType, err := llvmType(fn.ReturnRepr)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "%scall %s @%s(", prefix, retType, functionName(descriptor.Callee))
		for i := 0; i < descriptor.CaptureCount; i++ {
			if i != 0 {
				b.WriteString(", ")
			}
			typ, _ := llvmType(fn.Params[i].Repr)
			fmt.Fprintf(b, "%s %%capture%d", typ, i)
		}
		b.WriteString(")\n")
		if fn.ReturnRepr == mir.ReprBool {
			b.WriteString("  store i1 %result, ptr %result_slot\n")
		} else if fn.ReturnRepr == mir.ReprF64 {
			b.WriteString("  store double %result, ptr %result_slot\n")
		} else if isTaskReferenceResult(fn.ReturnRepr) {
			b.WriteString("  store ptr %result, ptr %result_slot\n")
			b.WriteString("  call void @tsnative_gc_handoff_end()\n")
		}
		b.WriteString("  ret void\n}\n\n")
	}
	return nil
}

func (e *emitter) emitTaskSpawn(b *strings.Builder, inst mir.Instruction, op mir.TaskSpawn, values map[mir.ValueID]string) error {
	fn, ok := e.functions[op.Callee]
	if !ok || len(op.Captures) > len(fn.Params) {
		return fmt.Errorf("invalid task spawn target f%d", op.Callee)
	}
	name := valueName(inst.Result)
	state := "null"
	continuation, err := taskContinuationFor(fn)
	if err != nil {
		return err
	}
	descriptor := taskDescriptor{Callee: op.Callee, CaptureCount: len(op.Captures), Continuation: continuation}
	if len(op.Captures) != 0 || descriptor.Continuation != nil {
		state = name + ".state"
		fmt.Fprintf(b, "  %s.sizeptr = getelementptr %s, ptr null, i32 1\n", state, taskEnvTypeName(op.Callee))
		fmt.Fprintf(b, "  %s.size = ptrtoint ptr %s.sizeptr to i64\n", state, state)
		stateRefs := taskEnvRefFields(fn, descriptor)
		emitHeapObjectAlloc(b, state, state+".size", taskEnvRefDescriptorName(op.Callee), len(stateRefs))
		for i, capture := range op.Captures {
			value, err := operand(values, capture)
			if err != nil {
				return err
			}
			typ, err := llvmType(fn.Params[i].Repr)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  %s.c%d = getelementptr %s, ptr %s, i32 0, i32 %d\n", state, i, taskEnvTypeName(op.Callee), state, i)
			fmt.Fprintf(b, "  store %s %s, ptr %s.c%d\n", typ, value, state, i)
		}
		if descriptor.Continuation != nil {
			pcIndex := descriptor.CaptureCount
			fmt.Fprintf(b, "  %s.pc = getelementptr %s, ptr %s, i32 0, i32 %d\n", state, taskEnvTypeName(op.Callee), state, pcIndex)
			fmt.Fprintf(b, "  store i32 0, ptr %s.pc\n", state)
			for _, value := range descriptor.Continuation.sortedSpillValues() {
				slot := descriptor.Continuation.SpillSlots[value]
				fmt.Fprintf(b, "  %s.spill%d = getelementptr %s, ptr %s, i32 0, i32 %d\n", state, slot.Index, taskEnvTypeName(op.Callee), state, pcIndex+1+slot.Index)
				switch slot.Repr {
				case mir.ReprF64:
					fmt.Fprintf(b, "  store double 0.000000e+00, ptr %s.spill%d\n", state, slot.Index)
				case mir.ReprBool:
					fmt.Fprintf(b, "  store i1 false, ptr %s.spill%d\n", state, slot.Index)
				case mir.ReprStringRef, mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef, mir.ReprTaskRef, mir.ReprTaskGroupRef, mir.ReprJSValue:
					fmt.Fprintf(b, "  store ptr null, ptr %s.spill%d\n", state, slot.Index)
				default:
					return fmt.Errorf("unsupported task continuation spill representation %d", slot.Repr)
				}
			}
		}
	}
	spawnName := "tsnative_task_spawn_or_abort"
	if op.Group != nil {
		spawnName = "tsnative_task_group_spawn_or_abort"
	}
	if fn.ReturnRepr == mir.ReprBool {
		if op.Group != nil {
			spawnName = "tsnative_task_group_spawn_bool_or_abort"
		} else {
			spawnName = "tsnative_task_spawn_bool_or_abort"
		}
	} else if fn.ReturnRepr == mir.ReprF64 {
		if op.Group != nil {
			spawnName = "tsnative_task_group_spawn_f64_or_abort"
		} else {
			spawnName = "tsnative_task_spawn_f64_or_abort"
		}
	} else if isTaskReferenceResult(fn.ReturnRepr) {
		if op.Group != nil {
			spawnName = "tsnative_task_group_spawn_ref_or_abort"
		} else {
			spawnName = "tsnative_task_spawn_ref_or_abort"
		}
	} else if fn.ReturnRepr != mir.ReprVoid {
		return fmt.Errorf("unsupported task result representation %d", fn.ReturnRepr)
	}
	if op.Group != nil {
		group, err := operand(values, *op.Group)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %s = call ptr @%s(ptr %s, ptr @%s, ptr %s)\n", name, spawnName, group, taskWrapperName(op.Callee), state)
	} else {
		fmt.Fprintf(b, "  %s = call ptr @%s(ptr @%s, ptr %s)\n", name, spawnName, taskWrapperName(op.Callee), state)
	}
	values[inst.Result] = name
	return nil
}

func isTaskReferenceResult(repr mir.Repr) bool {
	switch repr {
	case mir.ReprStringRef, mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef, mir.ReprTaskRef, mir.ReprTaskGroupRef, mir.ReprJSValue:
		return true
	default:
		return false
	}
}
