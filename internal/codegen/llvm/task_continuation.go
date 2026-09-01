package llvm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

type taskSuspendKind uint8

const (
	taskSuspendSendF64 taskSuspendKind = iota + 1
	taskSuspendRecvF64
	taskSuspendSleep
	taskSuspendAwaitF64
)

type taskSuspendStep struct {
	Kind     taskSuspendKind
	Channel  mir.ValueID
	Value    mir.ValueID
	Result   mir.ValueID
	Duration mir.ValueID
	Callee   mir.FunctionID
	Captures []mir.ValueID
	Task     mir.ValueID
}

type taskContinuation struct {
	Steps     []taskSuspendStep
	Consts    map[mir.ValueID]float64
	RecvSlots map[mir.ValueID]int
}

func analyzeTaskContinuation(fn mir.Function) *taskContinuation {
	if len(fn.Blocks) != 1 {
		return nil
	}
	block := fn.Blocks[0]
	ret, ok := block.Terminator.(mir.Return)
	if !ok {
		return nil
	}
	cont := &taskContinuation{
		Consts:    map[mir.ValueID]float64{},
		RecvSlots: map[mir.ValueID]int{},
	}
	available := map[mir.ValueID]bool{}
	for _, param := range fn.Params {
		available[param.Value] = true
	}
	var pendingSpawn *mir.TaskSpawn
	var pendingTask mir.ValueID
	for _, inst := range block.Instructions {
		if pendingSpawn != nil {
			join, ok := inst.Op.(mir.TaskJoin)
			if !ok || join.Task != pendingTask || inst.Repr != mir.ReprF64 {
				return nil
			}
			cont.RecvSlots[inst.Result] = len(cont.RecvSlots)
			available[inst.Result] = true
			cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskSuspendAwaitF64, Callee: pendingSpawn.Callee, Captures: append([]mir.ValueID(nil), pendingSpawn.Captures...), Task: pendingTask, Result: inst.Result})
			pendingSpawn = nil
			continue
		}
		switch op := inst.Op.(type) {
		case mir.ConstF64:
			cont.Consts[inst.Result] = op.Value
			available[inst.Result] = true
		case mir.TaskSpawn:
			for _, capture := range op.Captures {
				if !available[capture] {
					return nil
				}
			}
			copy := op
			pendingSpawn = &copy
			pendingTask = inst.Result
		case mir.ChannelSendF64:
			if !available[op.Channel] || !available[op.Value] {
				return nil
			}
			cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskSuspendSendF64, Channel: op.Channel, Value: op.Value})
		case mir.ChannelRecvF64:
			if !available[op.Channel] {
				return nil
			}
			cont.RecvSlots[inst.Result] = len(cont.RecvSlots)
			available[inst.Result] = true
			cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskSuspendRecvF64, Channel: op.Channel, Result: inst.Result})
		case mir.Sleep:
			if !available[op.Duration] {
				return nil
			}
			cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskSuspendSleep, Duration: op.Duration})
		default:
			return nil
		}
	}
	if pendingSpawn != nil {
		return nil
	}
	if len(cont.Steps) == 0 {
		return nil
	}
	switch fn.ReturnRepr {
	case mir.ReprVoid:
		if ret.Value != nil {
			return nil
		}
	case mir.ReprF64:
		if ret.Value == nil || !available[*ret.Value] {
			return nil
		}
	default:
		return nil
	}
	return cont
}

func (c *taskContinuation) recvSlotCount() int {
	if c == nil {
		return 0
	}
	return len(c.RecvSlots)
}

func (c *taskContinuation) sortedRecvValues() []mir.ValueID {
	values := make([]mir.ValueID, len(c.RecvSlots))
	for value, index := range c.RecvSlots {
		values[index] = value
	}
	return values
}

func continuationOperand(b *strings.Builder, fn mir.Function, descriptor taskDescriptor, value mir.ValueID, suffix string) (string, mir.Repr, error) {
	cont := descriptor.Continuation
	if constant, ok := cont.Consts[value]; ok {
		return formatF64(constant), mir.ReprF64, nil
	}
	for i, param := range fn.Params {
		if param.Value == value {
			return fmt.Sprintf("%%capture%d", i), param.Repr, nil
		}
	}
	if slot, ok := cont.RecvSlots[value]; ok {
		name := fmt.Sprintf("%%spill.%d.%s", slot, suffix)
		fmt.Fprintf(b, "  %s = load double, ptr %%recv%d.ptr\n", name, slot)
		return name, mir.ReprF64, nil
	}
	return "", mir.ReprInvalid, fmt.Errorf("task continuation operand v%d is unavailable", value)
}

func (e *emitter) emitContinuationTaskWrapper(b *strings.Builder, descriptor taskDescriptor, fn mir.Function) error {
	cont := descriptor.Continuation
	for i := 0; i < descriptor.CaptureCount; i++ {
		typ, err := llvmType(fn.Params[i].Repr)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %%capture%d.ptr = getelementptr %s, ptr %%state, i32 0, i32 %d\n", i, taskEnvTypeName(descriptor.Callee), i)
		fmt.Fprintf(b, "  %%capture%d = load %s, ptr %%capture%d.ptr\n", i, typ, i)
	}
	pcIndex := descriptor.CaptureCount
	fmt.Fprintf(b, "  %%pc.ptr = getelementptr %s, ptr %%state, i32 0, i32 %d\n", taskEnvTypeName(descriptor.Callee), pcIndex)
	for slot := 0; slot < cont.recvSlotCount(); slot++ {
		fmt.Fprintf(b, "  %%recv%d.ptr = getelementptr %s, ptr %%state, i32 0, i32 %d\n", slot, taskEnvTypeName(descriptor.Callee), pcIndex+1+slot)
	}
	b.WriteString("  %pc = load i32, ptr %pc.ptr\n")
	fmt.Fprintf(b, "  switch i32 %%pc, label %%invalid [")
	for i := 0; i <= len(cont.Steps); i++ {
		fmt.Fprintf(b, " i32 %d, label %%step%d", i, i)
	}
	b.WriteString(" ]\n")
	for i, step := range cont.Steps {
		fmt.Fprintf(b, "step%d:\n", i)
		next := fmt.Sprintf("%%step%d", i+1)
		park := fmt.Sprintf("%%park%d", i)
		switch step.Kind {
		case taskSuspendSleep:
			duration, repr, err := continuationOperand(b, fn, descriptor, step.Duration, fmt.Sprintf("s%d", i))
			if err != nil {
				return err
			}
			if repr != mir.ReprF64 {
				return fmt.Errorf("task continuation sleep duration must be F64")
			}
			fmt.Fprintf(b, "  %%status%d = call i32 @tsnative_sleep_task(double %s)\n", i, duration)
		case taskSuspendSendF64:
			channel, repr, err := continuationOperand(b, fn, descriptor, step.Channel, fmt.Sprintf("c%d", i))
			if err != nil {
				return err
			}
			if repr != mir.ReprChannelRef {
				return fmt.Errorf("task continuation channel must be ChannelRef")
			}
			value, valueRepr, err := continuationOperand(b, fn, descriptor, step.Value, fmt.Sprintf("v%d", i))
			if err != nil {
				return err
			}
			if valueRepr != mir.ReprF64 {
				return fmt.Errorf("task continuation send value must be F64")
			}
			fmt.Fprintf(b, "  %%status%d = call i32 @tsnative_channel_f64_send_task(ptr %s, double %s)\n", i, channel, value)
		case taskSuspendRecvF64:
			channel, repr, err := continuationOperand(b, fn, descriptor, step.Channel, fmt.Sprintf("c%d", i))
			if err != nil {
				return err
			}
			if repr != mir.ReprChannelRef {
				return fmt.Errorf("task continuation channel must be ChannelRef")
			}
			slot := cont.RecvSlots[step.Result]
			fmt.Fprintf(b, "  %%status%d = call i32 @tsnative_channel_f64_recv_task(ptr %s, ptr %%recv%d.ptr)\n", i, channel, slot)
		case taskSuspendAwaitF64:
			values := map[mir.ValueID]string{}
			for _, capture := range step.Captures {
				value, _, err := continuationOperand(b, fn, descriptor, capture, fmt.Sprintf("a%d", i))
				if err != nil {
					return err
				}
				values[capture] = value
			}
			spawnInst := mir.Instruction{Result: step.Task, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: step.Callee, Captures: step.Captures}}
			if err := e.emitTaskSpawn(b, spawnInst, spawnInst.Op.(mir.TaskSpawn), values); err != nil {
				return err
			}
			child := values[step.Task]
			slot := cont.RecvSlots[step.Result]
			fmt.Fprintf(b, "  %%status%d = call i32 @tsnative_task_await_f64_task(ptr %s, ptr %%recv%d.ptr)\n", i, child, slot)
		default:
			return fmt.Errorf("unsupported task suspension kind %d", step.Kind)
		}
		fmt.Fprintf(b, "  switch i32 %%status%d, label %%invalid [ i32 0, label %s i32 1, label %s ]\n", i, park, next)
		fmt.Fprintf(b, "park%d:\n  store i32 %d, ptr %%pc.ptr\n  ret void\n", i, i+1)
	}
	fmt.Fprintf(b, "step%d:\n", len(cont.Steps))
	ret := fn.Blocks[0].Terminator.(mir.Return)
	if fn.ReturnRepr == mir.ReprF64 {
		value, repr, err := continuationOperand(b, fn, descriptor, *ret.Value, "ret")
		if err != nil {
			return err
		}
		if repr != mir.ReprF64 {
			return fmt.Errorf("task continuation result must be F64")
		}
		fmt.Fprintf(b, "  store double %s, ptr %%result_slot\n", value)
	}
	b.WriteString("  ret void\ninvalid:\n  unreachable\n}\n\n")
	return nil
}

func sortedRecvSlotValues(slots map[mir.ValueID]int) []mir.ValueID {
	values := make([]mir.ValueID, 0, len(slots))
	for value := range slots {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return slots[values[i]] < slots[values[j]] })
	return values
}
