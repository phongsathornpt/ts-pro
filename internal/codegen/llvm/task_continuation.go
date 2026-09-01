package llvm

import (
	"fmt"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

type taskSuspendKind uint8

const (
	taskSuspendSendF64 taskSuspendKind = iota + 1
	taskSuspendRecvF64
)

type taskContinuation struct {
	Kind    taskSuspendKind
	Channel mir.ValueID
	Value   mir.ValueID
	Result  mir.ValueID
	Consts  map[mir.ValueID]float64
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
	cont := &taskContinuation{Consts: map[mir.ValueID]float64{}}
	seenSuspend := false
	for _, inst := range block.Instructions {
		switch op := inst.Op.(type) {
		case mir.ConstF64:
			if seenSuspend {
				return nil
			}
			cont.Consts[inst.Result] = op.Value
		case mir.ChannelSendF64:
			if seenSuspend || fn.ReturnRepr != mir.ReprVoid || ret.Value != nil {
				return nil
			}
			seenSuspend = true
			cont.Kind, cont.Channel, cont.Value = taskSuspendSendF64, op.Channel, op.Value
		case mir.ChannelRecvF64:
			if seenSuspend || fn.ReturnRepr != mir.ReprF64 || ret.Value == nil || *ret.Value != inst.Result {
				return nil
			}
			seenSuspend = true
			cont.Kind, cont.Channel, cont.Result = taskSuspendRecvF64, op.Channel, inst.Result
		default:
			return nil
		}
	}
	if !seenSuspend || !taskContinuationOperandSupported(fn, cont.Channel, cont.Consts) {
		return nil
	}
	if cont.Kind == taskSuspendSendF64 && !taskContinuationOperandSupported(fn, cont.Value, cont.Consts) {
		return nil
	}
	return cont
}

func taskContinuationOperandSupported(fn mir.Function, value mir.ValueID, constants map[mir.ValueID]float64) bool {
	if _, ok := constants[value]; ok {
		return true
	}
	for _, param := range fn.Params {
		if param.Value == value {
			return true
		}
	}
	return false
}

func continuationOperand(fn mir.Function, value mir.ValueID, constants map[mir.ValueID]float64) (string, mir.Repr, error) {
	if constant, ok := constants[value]; ok {
		return formatF64(constant), mir.ReprF64, nil
	}
	for i, param := range fn.Params {
		if param.Value == value {
			return fmt.Sprintf("%%capture%d", i), param.Repr, nil
		}
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
	if cont.Kind == taskSuspendRecvF64 {
		fmt.Fprintf(b, "  %%recv.ptr = getelementptr %s, ptr %%state, i32 0, i32 %d\n", taskEnvTypeName(descriptor.Callee), pcIndex+1)
	}
	b.WriteString("  %pc = load i32, ptr %pc.ptr\n")
	b.WriteString("  switch i32 %pc, label %invalid [ i32 0, label %start i32 1, label %resume ]\n")
	b.WriteString("start:\n")
	channel, repr, err := continuationOperand(fn, cont.Channel, cont.Consts)
	if err != nil {
		return err
	}
	if repr != mir.ReprChannelRef {
		return fmt.Errorf("task continuation channel must be ChannelRef")
	}
	if cont.Kind == taskSuspendSendF64 {
		value, valueRepr, err := continuationOperand(fn, cont.Value, cont.Consts)
		if err != nil {
			return err
		}
		if valueRepr != mir.ReprF64 {
			return fmt.Errorf("task continuation send value must be F64")
		}
		fmt.Fprintf(b, "  %%status = call i32 @tsnative_channel_f64_send_task(ptr %s, double %s)\n", channel, value)
		b.WriteString("  %parked = icmp eq i32 %status, 0\n  br i1 %parked, label %park, label %complete\n")
		b.WriteString("park:\n  store i32 1, ptr %pc.ptr\n  ret void\nresume:\n  br label %complete\ncomplete:\n  ret void\n")
	} else {
		fmt.Fprintf(b, "  %%status = call i32 @tsnative_channel_f64_recv_task(ptr %s, ptr %%recv.ptr)\n", channel)
		b.WriteString("  %parked = icmp eq i32 %status, 0\n  br i1 %parked, label %park, label %complete.now\n")
		b.WriteString("park:\n  store i32 1, ptr %pc.ptr\n  ret void\nresume:\n  br label %complete.now\ncomplete.now:\n  %recv = load double, ptr %recv.ptr\n  store double %recv, ptr %result_slot\n  ret void\n")
	}
	b.WriteString("invalid:\n  unreachable\n}\n\n")
	return nil
}
