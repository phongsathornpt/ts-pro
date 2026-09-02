package escape

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/mir"
)

func analyzeSingle(fn mir.Function) FunctionResult {
	return Analyze(mir.Module{Functions: []mir.Function{fn}})[fn.ID]
}

func objectAlloc(result mir.ValueID) mir.Instruction {
	return mir.Instruction{Result: result, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0}}
}

func TestLocalObjectIsStackEligible(t *testing.T) {
	ret := mir.ValueID(3)
	fn := mir.Function{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
		objectAlloc(0),
		{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
		{Result: 2, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 0, Shape: 0, Field: 0, Value: 1}},
		{Result: 3, Repr: mir.ReprF64, Op: mir.FieldGet{Object: 0, Shape: 0, Field: 0}},
	}, Terminator: mir.Return{Value: &ret}}}}
	result := analyzeSingle(fn)
	if !result.CanStackAllocate(0) {
		t.Fatalf("local object escaped: %+v", result[0])
	}
}
func TestReturnedPhiObjectEscapes(t *testing.T) {
	ret := mir.ValueID(1)
	fn := mir.Function{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
		objectAlloc(0),
		{Result: 1, Repr: mir.ReprObjectRef, Op: mir.Phi{Incoming: []mir.PhiIncoming{{Block: 0, Value: 0}}}},
	}, Terminator: mir.Return{Value: &ret}}}}
	info := analyzeSingle(fn)[0]
	if !info.Escapes || info.Reasons&ReasonReturn == 0 {
		t.Fatalf("returned object escape = %+v", info)
	}
}

func TestEscapingContainerPropagatesToContainedObject(t *testing.T) {
	ret := mir.ValueID(1)
	fn := mir.Function{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
		objectAlloc(0),
		{Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectNew{Shape: 1, Fields: []mir.ValueID{0}}},
	}, Terminator: mir.Return{Value: &ret}}}}
	result := analyzeSingle(fn)
	if !result[1].Escapes || result[1].Reasons&ReasonReturn == 0 {
		t.Fatalf("outer object escape = %+v", result[1])
	}
	if !result[0].Escapes || result[0].Reasons&ReasonContained == 0 {
		t.Fatalf("contained object escape = %+v", result[0])
	}
}
func TestStoreIntoUnknownHeapEscapesValue(t *testing.T) {
	fn := mir.Function{ID: 0, Params: []mir.Param{{Value: 0, Repr: mir.ReprObjectRef}}, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
		objectAlloc(1),
		{Result: 2, Repr: mir.ReprObjectRef, Op: mir.FieldSet{Object: 0, Shape: 0, Field: 0, Value: 1}},
	}, Terminator: mir.Return{}}}}
	info := analyzeSingle(fn)[1]
	if !info.Escapes || info.Reasons&ReasonHeapStore == 0 {
		t.Fatalf("heap-stored object escape = %+v", info)
	}
}

func TestCallTaskChannelAndBoxEscapeValues(t *testing.T) {
	tests := []struct {
		name   string
		op     mir.Operation
		reason Reason
	}{
		{name: "call", op: mir.Call{Callee: 1, Args: []mir.ValueID{0}}, reason: ReasonCall},
		{name: "task", op: mir.TaskSpawn{Callee: 1, Captures: []mir.ValueID{0}}, reason: ReasonTask},
		{name: "channel", op: mir.ChannelSendRef{Channel: 9, Value: 0}, reason: ReasonChannel},
		{name: "box", op: mir.BoxJSValue{Kind: mir.BoxJSObject, Value: 0}, reason: ReasonBox},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := mir.Function{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				objectAlloc(0), {Result: 1, Repr: mir.ReprVoid, Op: test.op},
			}, Terminator: mir.Return{}}}}
			info := analyzeSingle(fn)[0]
			if !info.Escapes || info.Reasons&test.reason == 0 {
				t.Fatalf("escape = %+v, want reason %#x", info, test.reason)
			}
		})
	}
}
func TestLocalClosureAndCapturedObjectStayStackEligible(t *testing.T) {
	fn := mir.Function{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
		objectAlloc(0),
		{Result: 1, Repr: mir.ReprFunctionRef, Op: mir.ClosureNew{Callee: 1, Captures: []mir.ValueID{0}}},
		{Result: 2, Repr: mir.ReprVoid, Op: mir.ClosureCall{Closure: 1}},
	}, Terminator: mir.Return{}}}}
	result := analyzeSingle(fn)
	if !result.CanStackAllocate(1) || !result.CanStackAllocate(0) {
		t.Fatalf("local closure graph escaped: closure=%+v capture=%+v", result[1], result[0])
	}
}

func TestReturnedClosureEscapesCapturedObject(t *testing.T) {
	ret := mir.ValueID(1)
	fn := mir.Function{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
		objectAlloc(0),
		{Result: 1, Repr: mir.ReprFunctionRef, Op: mir.ClosureNew{Callee: 1, Captures: []mir.ValueID{0}}},
	}, Terminator: mir.Return{Value: &ret}}}}
	result := analyzeSingle(fn)
	if !result[1].Escapes || result[1].Reasons&ReasonReturn == 0 {
		t.Fatalf("returned closure escape = %+v", result[1])
	}
	if !result[0].Escapes || result[0].Reasons&ReasonContained == 0 {
		t.Fatalf("captured object escape = %+v", result[0])
	}
}
func TestSuspendingFunctionDisablesStackEligibility(t *testing.T) {
	fn := mir.Function{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
		objectAlloc(0),
		{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
		{Result: 2, Repr: mir.ReprVoid, Op: mir.Sleep{Duration: 1}},
	}, Terminator: mir.Return{}}}}
	info := analyzeSingle(fn)[0]
	if !info.Escapes || info.Reasons&ReasonSuspension == 0 {
		t.Fatalf("suspending allocation escape = %+v", info)
	}
}

func TestThrownObjectEscapes(t *testing.T) {
	fn := mir.Function{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
		objectAlloc(0),
	}, Terminator: mir.Throw{Value: 0}}}}
	info := analyzeSingle(fn)[0]
	if !info.Escapes || info.Reasons&ReasonThrow == 0 {
		t.Fatalf("thrown object escape = %+v", info)
	}
}

func TestReturnedFieldGetEscapesContainedObject(t *testing.T) {
	ret := mir.ValueID(2)
	fn := mir.Function{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
		objectAlloc(0),
		{Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectNew{Shape: 1, Fields: []mir.ValueID{0}}},
		{Result: 2, Repr: mir.ReprObjectRef, Op: mir.FieldGet{Object: 1, Shape: 1, Field: 0}},
	}, Terminator: mir.Return{Value: &ret}}}}
	result := analyzeSingle(fn)
	if !result[0].Escapes || result[0].Reasons&ReasonReturn == 0 {
		t.Fatalf("returned contained object escape = %+v", result[0])
	}
	if result[1].Escapes {
		t.Fatalf("container itself unexpectedly escaped: %+v", result[1])
	}
}
