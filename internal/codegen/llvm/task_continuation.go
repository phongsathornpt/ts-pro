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
	taskSuspendAwaitBool
	taskSuspendAwaitRef
	taskSuspendJoinVoid
	taskStepTaskRelease
	taskStepFloatBinary
	taskStepProvenIntBinary
	taskStepFloatCompare
	taskStepConstString
	taskStepStringConcat
	taskStepBoxJSValue
	taskStepDynamicAddJSValue
	taskStepDynamicBinaryJSValue
	taskStepArrayLengthF64
	taskStepArrayGetF64
	taskStepNativeOp
	taskStepBranch
	taskStepJump
	taskStepReturn
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
	Inst     mir.Instruction
	Target   mir.BlockID
	Then     mir.BlockID
	Else     mir.BlockID
	HasValue bool
	Block    mir.BlockID
}

type taskSpillSlot struct {
	Index int
	Repr  mir.Repr
}

type taskPhi struct {
	Result   mir.ValueID
	Repr     mir.Repr
	Incoming map[mir.BlockID]mir.ValueID
}

type taskContinuation struct {
	Steps      []taskSuspendStep
	Consts     map[mir.ValueID]float64
	BoolConsts map[mir.ValueID]bool
	SpillSlots map[mir.ValueID]taskSpillSlot
	BlockPC    map[mir.BlockID]int
	Phis       map[mir.BlockID][]taskPhi
}

func continuationNativeOperands(op mir.Operation) ([]mir.ValueID, bool) {
	switch op := op.(type) {
	case mir.ArrayNewF64:
		return append([]mir.ValueID(nil), op.Elements...), true
	case mir.ArraySetF64:
		return []mir.ValueID{op.Array, op.Index, op.Value}, true
	case mir.ObjectNew:
		return append([]mir.ValueID(nil), op.Fields...), true
	case mir.ObjectAlloc:
		return nil, true
	case mir.FieldSet:
		return []mir.ValueID{op.Object, op.Value}, true
	case mir.FieldGet:
		return []mir.ValueID{op.Object}, true
	case mir.ClosureNew:
		return append([]mir.ValueID(nil), op.Captures...), true
	case mir.ClosureCall:
		result := []mir.ValueID{op.Closure}
		return append(result, op.Args...), true
	case mir.Call:
		return append([]mir.ValueID(nil), op.Args...), true
	case mir.DispatchCall:
		return append([]mir.ValueID(nil), op.Args...), true
	case mir.IntrinsicCall:
		return append([]mir.ValueID(nil), op.Args...), true
	case mir.ChannelNewF64:
		return []mir.ValueID{op.Capacity}, true
	case mir.ChannelTrySendF64:
		return []mir.ValueID{op.Channel, op.Value}, true
	case mir.ChannelTryRecvOrF64:
		return []mir.ValueID{op.Channel, op.Fallback}, true
	case mir.ConstJSValue:
		return nil, true
	case mir.UnboxJSValue:
		return []mir.ValueID{op.Value}, true
	case mir.TaskSpawn:
		return append([]mir.ValueID(nil), op.Captures...), true
	case mir.TaskYield:
		return nil, true
	default:
		return nil, false
	}
}

func analyzeTaskContinuation(fn mir.Function) *taskContinuation {
	if len(fn.Blocks) == 0 {
		return nil
	}
	cont := &taskContinuation{
		Consts:     map[mir.ValueID]float64{},
		BoolConsts: map[mir.ValueID]bool{},
		SpillSlots: map[mir.ValueID]taskSpillSlot{},
		BlockPC:    map[mir.BlockID]int{},
		Phis:       map[mir.BlockID][]taskPhi{},
	}
	available := map[mir.ValueID]bool{}
	for _, param := range fn.Params {
		available[param.Value] = true
	}
	blocks := append([]mir.Block(nil), fn.Blocks...)
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].ID < blocks[j].ID })
	hasSuspend := false
	for _, block := range blocks {
		cont.BlockPC[block.ID] = len(cont.Steps)
		for _, inst := range block.Instructions {
			switch op := inst.Op.(type) {
			case mir.ConstF64:
				cont.Consts[inst.Result] = op.Value
				available[inst.Result] = true
			case mir.ConstBool:
				cont.BoolConsts[inst.Result] = op.Value
				available[inst.Result] = true
			case mir.FloatBinary:
				if !available[op.Left] || !available[op.Right] || inst.Repr != mir.ReprF64 {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: mir.ReprF64}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepFloatBinary, Result: inst.Result, Inst: inst})
			case mir.ProvenIntBinary:
				if !available[op.Left] || !available[op.Right] || inst.Repr != mir.ReprF64 {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: mir.ReprF64}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepProvenIntBinary, Result: inst.Result, Inst: inst})
			case mir.FloatCompare:
				if !available[op.Left] || !available[op.Right] || inst.Repr != mir.ReprBool {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: mir.ReprBool}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepFloatCompare, Result: inst.Result, Inst: inst})
			case mir.ConstString:
				if inst.Repr != mir.ReprStringRef {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: inst.Repr}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepConstString, Result: inst.Result, Inst: inst})
			case mir.StringConcat:
				if !available[op.Left] || !available[op.Right] || inst.Repr != mir.ReprStringRef {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: inst.Repr}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepStringConcat, Result: inst.Result, Inst: inst})
			case mir.BoxJSValue:
				if !available[op.Value] || inst.Repr != mir.ReprJSValue {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: inst.Repr}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepBoxJSValue, Result: inst.Result, Inst: inst})
			case mir.DynamicAddJSValue:
				if !available[op.Left] || !available[op.Right] || inst.Repr != mir.ReprJSValue {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: inst.Repr}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepDynamicAddJSValue, Result: inst.Result, Inst: inst})
			case mir.DynamicBinaryJSValue:
				if !available[op.Left] || !available[op.Right] || (inst.Repr != mir.ReprF64 && inst.Repr != mir.ReprBool) {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: inst.Repr}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepDynamicBinaryJSValue, Result: inst.Result, Inst: inst})
			case mir.ArrayLengthF64:
				if !available[op.Array] || inst.Repr != mir.ReprF64 {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: inst.Repr}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepArrayLengthF64, Result: inst.Result, Inst: inst})
			case mir.ArrayGetF64:
				if !available[op.Array] || !available[op.Index] || inst.Repr != mir.ReprF64 {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: inst.Repr}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepArrayGetF64, Result: inst.Result, Inst: inst})
			case mir.TaskSpawn:
				for _, capture := range op.Captures {
					if !available[capture] {
						return nil
					}
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: mir.ReprTaskRef}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepNativeOp, Result: inst.Result, Inst: inst})
			case mir.TaskJoin:
				if !available[op.Task] {
					return nil
				}
				hasSuspend = true
				if inst.Repr == mir.ReprVoid {
					cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskSuspendJoinVoid, Task: op.Task})
					cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepTaskRelease, Task: op.Task})
					continue
				}
				kind := taskSuspendKind(0)
				switch inst.Repr {
				case mir.ReprF64:
					kind = taskSuspendAwaitF64
				case mir.ReprBool:
					kind = taskSuspendAwaitBool
				case mir.ReprStringRef, mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef, mir.ReprJSValue:
					kind = taskSuspendAwaitRef
				default:
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: inst.Repr}
				available[inst.Result] = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: kind, Task: op.Task, Result: inst.Result})
			case mir.ChannelSendF64:
				if !available[op.Channel] || !available[op.Value] {
					return nil
				}
				hasSuspend = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskSuspendSendF64, Channel: op.Channel, Value: op.Value})
			case mir.ChannelRecvF64:
				if !available[op.Channel] {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: mir.ReprF64}
				available[inst.Result] = true
				hasSuspend = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskSuspendRecvF64, Channel: op.Channel, Result: inst.Result})
			case mir.Sleep:
				if !available[op.Duration] {
					return nil
				}
				hasSuspend = true
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskSuspendSleep, Duration: op.Duration})
			case mir.Phi:
				if inst.Repr == mir.ReprVoid || inst.Repr == mir.ReprInvalid {
					return nil
				}
				if _, exists := cont.SpillSlots[inst.Result]; exists {
					return nil
				}
				cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: inst.Repr}
				available[inst.Result] = true
				phi := taskPhi{Result: inst.Result, Repr: inst.Repr, Incoming: map[mir.BlockID]mir.ValueID{}}
				for _, incoming := range op.Incoming {
					phi.Incoming[incoming.Block] = incoming.Value
				}
				cont.Phis[block.ID] = append(cont.Phis[block.ID], phi)
			default:
				operands, ok := continuationNativeOperands(inst.Op)
				if !ok {
					return nil
				}
				for _, value := range operands {
					if !available[value] {
						return nil
					}
				}
				if inst.Repr != mir.ReprVoid {
					cont.SpillSlots[inst.Result] = taskSpillSlot{Index: len(cont.SpillSlots), Repr: inst.Repr}
					available[inst.Result] = true
				}
				cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepNativeOp, Result: inst.Result, Inst: inst})
			}
		}
		switch term := block.Terminator.(type) {
		case mir.Return:
			step := taskSuspendStep{Kind: taskStepReturn, Block: block.ID}
			if term.Value != nil {
				if !available[*term.Value] {
					return nil
				}
				step.Value = *term.Value
				step.HasValue = true
			} else if fn.ReturnRepr != mir.ReprVoid {
				return nil
			}
			cont.Steps = append(cont.Steps, step)
		case mir.Jump:
			cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepJump, Target: term.Target, Block: block.ID})
		case mir.Branch:
			if !available[term.Condition] {
				return nil
			}
			cont.Steps = append(cont.Steps, taskSuspendStep{Kind: taskStepBranch, Value: term.Condition, Then: term.Then, Else: term.Else, Block: block.ID})
		default:
			return nil
		}
	}
	if !hasSuspend {
		return nil
	}
	return cont
}

func (c *taskContinuation) spillSlotCount() int {
	if c == nil {
		return 0
	}
	return len(c.SpillSlots)
}

func (c *taskContinuation) sortedSpillValues() []mir.ValueID {
	values := make([]mir.ValueID, len(c.SpillSlots))
	for value, slot := range c.SpillSlots {
		values[slot.Index] = value
	}
	return values
}

func continuationOperand(b *strings.Builder, fn mir.Function, descriptor taskDescriptor, value mir.ValueID, suffix string) (string, mir.Repr, error) {
	cont := descriptor.Continuation
	if constant, ok := cont.Consts[value]; ok {
		return formatF64(constant), mir.ReprF64, nil
	}
	if constant, ok := cont.BoolConsts[value]; ok {
		if constant {
			return "true", mir.ReprBool, nil
		}
		return "false", mir.ReprBool, nil
	}
	for i, param := range fn.Params {
		if param.Value == value {
			return fmt.Sprintf("%%capture%d", i), param.Repr, nil
		}
	}
	if slot, ok := cont.SpillSlots[value]; ok {
		name := fmt.Sprintf("%%spill.%d.%s", slot.Index, suffix)
		typ, err := llvmType(slot.Repr)
		if err != nil {
			return "", mir.ReprInvalid, err
		}
		fmt.Fprintf(b, "  %s = load %s, ptr %%spill%d.ptr\n", name, typ, slot.Index)
		return name, slot.Repr, nil
	}
	return "", mir.ReprInvalid, fmt.Errorf("task continuation operand v%d is unavailable", value)
}

func (e *emitter) emitPureContinuationStep(b *strings.Builder, descriptor taskDescriptor, fn mir.Function, step taskSuspendStep, suffix string) error {
	values := map[mir.ValueID]string{}
	switch op := step.Inst.Op.(type) {
	case mir.ConstString:
		// no operands
	case mir.FloatBinary:
		for _, valueID := range []mir.ValueID{op.Left, op.Right} {
			value, _, err := continuationOperand(b, fn, descriptor, valueID, suffix)
			if err != nil {
				return err
			}
			values[valueID] = value
		}
	case mir.ProvenIntBinary:
		for _, valueID := range []mir.ValueID{op.Left, op.Right} {
			value, _, err := continuationOperand(b, fn, descriptor, valueID, suffix)
			if err != nil {
				return err
			}
			values[valueID] = value
		}
	case mir.FloatCompare:
		for _, valueID := range []mir.ValueID{op.Left, op.Right} {
			value, _, err := continuationOperand(b, fn, descriptor, valueID, suffix)
			if err != nil {
				return err
			}
			values[valueID] = value
		}
	case mir.StringConcat:
		for _, valueID := range []mir.ValueID{op.Left, op.Right} {
			value, _, err := continuationOperand(b, fn, descriptor, valueID, suffix)
			if err != nil {
				return err
			}
			values[valueID] = value
		}
	case mir.BoxJSValue:
		value, _, err := continuationOperand(b, fn, descriptor, op.Value, suffix)
		if err != nil {
			return err
		}
		values[op.Value] = value
	case mir.DynamicAddJSValue:
		for _, valueID := range []mir.ValueID{op.Left, op.Right} {
			value, _, err := continuationOperand(b, fn, descriptor, valueID, suffix)
			if err != nil {
				return err
			}
			values[valueID] = value
		}
	case mir.DynamicBinaryJSValue:
		for _, valueID := range []mir.ValueID{op.Left, op.Right} {
			value, _, err := continuationOperand(b, fn, descriptor, valueID, suffix)
			if err != nil {
				return err
			}
			values[valueID] = value
		}
	case mir.ArrayLengthF64:
		value, _, err := continuationOperand(b, fn, descriptor, op.Array, suffix)
		if err != nil {
			return err
		}
		values[op.Array] = value
	case mir.ArrayGetF64:
		for _, valueID := range []mir.ValueID{op.Array, op.Index} {
			value, _, err := continuationOperand(b, fn, descriptor, valueID, suffix)
			if err != nil {
				return err
			}
			values[valueID] = value
		}
	default:
		operands, ok := continuationNativeOperands(step.Inst.Op)
		if !ok {
			return fmt.Errorf("unsupported pure continuation op %T", step.Inst.Op)
		}
		for _, valueID := range operands {
			value, _, err := continuationOperand(b, fn, descriptor, valueID, suffix)
			if err != nil {
				return err
			}
			values[valueID] = value
		}
	}
	if emitsGCAllocation(step.Inst.Op) {
		b.WriteString("  call void @tsnative_gc_safepoint()\n")
	}
	if err := e.emitInstruction(b, fn, step.Inst, values); err != nil {
		return err
	}
	if step.Inst.Repr == mir.ReprVoid {
		return nil
	}
	if _, ok := values[step.Inst.Result]; !ok {
		values[step.Inst.Result] = valueName(step.Inst.Result)
	}
	result, ok := values[step.Inst.Result]
	if !ok {
		return fmt.Errorf("pure continuation op did not produce v%d", step.Inst.Result)
	}
	slot, ok := descriptor.Continuation.SpillSlots[step.Inst.Result]
	if !ok {
		return fmt.Errorf("pure continuation op v%d has no spill slot", step.Inst.Result)
	}
	typ, err := llvmType(slot.Repr)
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "  store %s %s, ptr %%spill%d.ptr\n", typ, result, slot.Index)
	return nil
}

func (e *emitter) emitContinuationPhiEdge(b *strings.Builder, descriptor taskDescriptor, fn mir.Function, from, target mir.BlockID, suffix string) error {
	phis := descriptor.Continuation.Phis[target]
	if len(phis) == 0 {
		return nil
	}
	type pendingStore struct {
		typ, value string
		slot       int
	}
	stores := make([]pendingStore, 0, len(phis))
	for i, phi := range phis {
		incoming, ok := phi.Incoming[from]
		if !ok {
			return fmt.Errorf("task continuation phi v%d has no incoming edge b%d -> b%d", phi.Result, from, target)
		}
		value, repr, err := continuationOperand(b, fn, descriptor, incoming, fmt.Sprintf("%s.phi%d", suffix, i))
		if err != nil {
			return err
		}
		if repr != phi.Repr {
			return fmt.Errorf("task continuation phi v%d repr mismatch", phi.Result)
		}
		typ, err := llvmType(repr)
		if err != nil {
			return err
		}
		slot := descriptor.Continuation.SpillSlots[phi.Result]
		stores = append(stores, pendingStore{typ: typ, value: value, slot: slot.Index})
	}
	for _, store := range stores {
		fmt.Fprintf(b, "  store %s %s, ptr %%spill%d.ptr\n", store.typ, store.value, store.slot)
	}
	return nil
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
	for _, value := range cont.sortedSpillValues() {
		slot := cont.SpillSlots[value]
		fmt.Fprintf(b, "  %%spill%d.ptr = getelementptr %s, ptr %%state, i32 0, i32 %d\n", slot.Index, taskEnvTypeName(descriptor.Callee), pcIndex+1+slot.Index)
	}
	b.WriteString("  %pc = load i32, ptr %pc.ptr\n")
	fmt.Fprintf(b, "  switch i32 %%pc, label %%invalid [")
	for i := 0; i < len(cont.Steps); i++ {
		fmt.Fprintf(b, " i32 %d, label %%step%d", i, i)
	}
	b.WriteString(" ]\n")
	for i, step := range cont.Steps {
		fmt.Fprintf(b, "step%d:\n", i)
		next := fmt.Sprintf("%%step%d", i+1)
		park := fmt.Sprintf("%%park%d", i)
		switch step.Kind {
		case taskStepBranch:
			condition, repr, err := continuationOperand(b, fn, descriptor, step.Value, fmt.Sprintf("br%d", i))
			if err != nil {
				return err
			}
			if repr != mir.ReprBool {
				return fmt.Errorf("task continuation branch condition must be Bool")
			}
			thenPC, thenOK := cont.BlockPC[step.Then]
			elsePC, elseOK := cont.BlockPC[step.Else]
			if !thenOK || !elseOK {
				return fmt.Errorf("task continuation branch targets are unavailable")
			}
			thenPhis, elsePhis := len(cont.Phis[step.Then]) != 0, len(cont.Phis[step.Else]) != 0
			thenLabel, elseLabel := fmt.Sprintf("%%step%d", thenPC), fmt.Sprintf("%%step%d", elsePC)
			if thenPhis {
				thenLabel = fmt.Sprintf("%%edge%d_then", i)
			}
			if elsePhis {
				elseLabel = fmt.Sprintf("%%edge%d_else", i)
			}
			fmt.Fprintf(b, "  br i1 %s, label %s, label %s\n", condition, thenLabel, elseLabel)
			if thenPhis {
				fmt.Fprintf(b, "edge%d_then:\n", i)
				if err := e.emitContinuationPhiEdge(b, descriptor, fn, step.Block, step.Then, fmt.Sprintf("e%d.then", i)); err != nil {
					return err
				}
				fmt.Fprintf(b, "  br label %%step%d\n", thenPC)
			}
			if elsePhis {
				fmt.Fprintf(b, "edge%d_else:\n", i)
				if err := e.emitContinuationPhiEdge(b, descriptor, fn, step.Block, step.Else, fmt.Sprintf("e%d.else", i)); err != nil {
					return err
				}
				fmt.Fprintf(b, "  br label %%step%d\n", elsePC)
			}
			continue
		case taskStepJump:
			targetPC, ok := cont.BlockPC[step.Target]
			if !ok {
				return fmt.Errorf("task continuation jump target b%d is unavailable", step.Target)
			}
			if err := e.emitContinuationPhiEdge(b, descriptor, fn, step.Block, step.Target, fmt.Sprintf("j%d", i)); err != nil {
				return err
			}
			fmt.Fprintf(b, "  br label %%step%d\n", targetPC)
			continue
		case taskStepReturn:
			if step.HasValue {
				value, repr, err := continuationOperand(b, fn, descriptor, step.Value, fmt.Sprintf("ret%d", i))
				if err != nil {
					return err
				}
				if repr != fn.ReturnRepr {
					return fmt.Errorf("task continuation result repr %d does not match function repr %d", repr, fn.ReturnRepr)
				}
				typ, err := llvmType(repr)
				if err != nil {
					return err
				}
				fmt.Fprintf(b, "  store %s %s, ptr %%result_slot\n", typ, value)
			} else if fn.ReturnRepr != mir.ReprVoid {
				return fmt.Errorf("task continuation non-void return has no value")
			}
			b.WriteString("  ret void\n")
			continue
		case taskStepFloatBinary, taskStepProvenIntBinary, taskStepFloatCompare,
			taskStepConstString, taskStepStringConcat, taskStepBoxJSValue, taskStepDynamicAddJSValue, taskStepDynamicBinaryJSValue,
			taskStepArrayLengthF64, taskStepArrayGetF64, taskStepNativeOp:
			if err := e.emitPureContinuationStep(b, descriptor, fn, step, fmt.Sprintf("p%d", i)); err != nil {
				return err
			}
			fmt.Fprintf(b, "  br label %s\n", next)
			continue
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
			slot := cont.SpillSlots[step.Result]
			fmt.Fprintf(b, "  %%status%d = call i32 @tsnative_channel_f64_recv_task(ptr %s, ptr %%spill%d.ptr)\n", i, channel, slot.Index)
		case taskStepTaskRelease:
			task, repr, err := continuationOperand(b, fn, descriptor, step.Task, fmt.Sprintf("release%d", i))
			if err != nil {
				return err
			}
			if repr != mir.ReprTaskRef {
				return fmt.Errorf("task continuation release requires TaskRef")
			}
			fmt.Fprintf(b, "  call void @tsnative_task_release(ptr %s)\n", task)
			fmt.Fprintf(b, "  br label %s\n", next)
			continue
		case taskSuspendJoinVoid:
			task, repr, err := continuationOperand(b, fn, descriptor, step.Task, fmt.Sprintf("join%d", i))
			if err != nil {
				return err
			}
			if repr != mir.ReprTaskRef {
				return fmt.Errorf("task continuation join requires TaskRef")
			}
			fmt.Fprintf(b, "  %%status%d = call i32 @tsnative_task_await_task(ptr %s)\n", i, task)
		case taskSuspendAwaitF64, taskSuspendAwaitBool, taskSuspendAwaitRef:
			task, repr, err := continuationOperand(b, fn, descriptor, step.Task, fmt.Sprintf("await%d", i))
			if err != nil {
				return err
			}
			if repr != mir.ReprTaskRef {
				return fmt.Errorf("task continuation await requires TaskRef")
			}
			slot := cont.SpillSlots[step.Result]
			awaitName := "tsnative_task_await_f64_task"
			if step.Kind == taskSuspendAwaitBool {
				awaitName = "tsnative_task_await_bool_task"
			} else if step.Kind == taskSuspendAwaitRef {
				awaitName = "tsnative_task_await_ref_task"
			}
			fmt.Fprintf(b, "  %%status%d = call i32 @%s(ptr %s, ptr %%spill%d.ptr)\n", i, awaitName, task, slot.Index)
		default:
			return fmt.Errorf("unsupported task suspension kind %d", step.Kind)
		}
		fmt.Fprintf(b, "  switch i32 %%status%d, label %%invalid [ i32 0, label %s i32 1, label %s ]\n", i, park, next)
		fmt.Fprintf(b, "park%d:\n  store i32 %d, ptr %%pc.ptr\n  ret void\n", i, i+1)
	}
	b.WriteString("invalid:\n  unreachable\n}\n\n")
	return nil
}
