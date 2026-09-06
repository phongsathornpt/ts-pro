package lower

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestAMD64RootSlotsTrackStringSSAValues(t *testing.T) {
	fn := ir.NewFunction("roots", types.TypeString)
	param := fn.NewValue("param", types.TypeString)
	number := fn.NewValue("number", types.TypeNumber)
	fn.Params = []*ir.Value{param, number}
	entry := fn.NewBlock("entry")
	strResult := fn.NewValue("str", types.TypeString)
	entry.Instructions = append(entry.Instructions, &ir.CallInst{Res: strResult, Callee: "make_string"})
	boolResult := fn.NewValue("bool", types.TypeBoolean)
	entry.Instructions = append(entry.Instructions, &ir.CallInst{Res: boolResult, Callee: "make_bool"})
	join := fn.NewBlock("join")
	phiResult := fn.NewValue("phi", types.TypeString)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: phiResult})

	slots := amd64RootSlots(fn)
	if len(slots) != 3 {
		t.Fatalf("root slot count: got %d, want 3", len(slots))
	}
	for _, v := range []*ir.Value{param, strResult, phiResult} {
		if _, ok := slots[v.ID]; !ok {
			t.Fatalf("missing root slot for %s", v)
		}
	}
	if _, ok := slots[number.ID]; ok {
		t.Fatal("number value must not receive a heap root slot")
	}
	if _, ok := slots[boolResult.ID]; ok {
		t.Fatal("boolean value must not receive a heap root slot")
	}
}

func TestAMD64HeapRefTypeRecognizesReferenceUnion(t *testing.T) {
	if !isAMD64HeapRefType(types.NewUnion(types.TypeNumber, types.TypeString)) {
		t.Fatal("number|string union must be reference-capable for conservative heap tracing")
	}
	if !isAMD64HeapRefType(types.NewUnion(types.TypeNumber, types.TypeBoolean)) {
		t.Fatal("number|boolean union requires a JSValue root slot")
	}
	if got := amd64ArrayElementClass(types.NewUnion(types.TypeNumber, types.TypeString)); got != 2 {
		t.Fatalf("number|string array element class = %d, want JSValue class 2", got)
	}
}

func TestAMD64RootLiveOutTracksPhiEdges(t *testing.T) {
	fn := ir.NewFunction("root_phi_edges", types.TypeString)
	entry := fn.NewBlock("entry")
	left := fn.NewBlock("left")
	right := fn.NewBlock("right")
	join := fn.NewBlock("join")

	leftVal := fn.NewValue("left_ref", types.TypeString)
	rightVal := fn.NewValue("right_ref", types.TypeString)
	left.Instructions = append(left.Instructions, &ir.CallInst{Res: leftVal, Callee: "make_left"})
	right.Instructions = append(right.Instructions, &ir.CallInst{Res: rightVal, Callee: "make_right"})
	entry.Terminator = &ir.BranchTerm{Cond: ir.ConstBool{Value: true}, Then: left, Else: right}
	left.Terminator = &ir.JumpTerm{Target: join}
	right.Terminator = &ir.JumpTerm{Target: join}
	phi := fn.NewValue("joined", types.TypeString)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: phi, Incoming: []ir.PhiIncoming{{Block: left, Value: leftVal}, {Block: right, Value: rightVal}}})
	join.Terminator = &ir.ReturnTerm{Val: phi}

	liveOut := amd64RootLiveOut(fn)
	if _, ok := liveOut[left][leftVal.ID]; !ok {
		t.Fatal("left phi incoming must be live on left->join edge")
	}
	if _, ok := liveOut[right][rightVal.ID]; !ok {
		t.Fatal("right phi incoming must be live on right->join edge")
	}
	if _, ok := liveOut[left][rightVal.ID]; ok {
		t.Fatal("right-only value must not be live on left edge")
	}
}

func TestAMD64RootLiveOutDropsDeadBlockLocal(t *testing.T) {
	fn := ir.NewFunction("root_dead_local", types.TypeVoid)
	entry := fn.NewBlock("entry")
	dead := fn.NewValue("dead", types.TypeString)
	entry.Instructions = append(entry.Instructions, &ir.CallInst{Res: dead, Callee: "make_string"})
	entry.Instructions = append(entry.Instructions, &ir.CallInst{Callee: "consume", Args: []ir.Operand{dead}})
	entry.Terminator = &ir.ReturnTerm{}
	if _, ok := amd64RootLiveOut(fn)[entry][dead.ID]; ok {
		t.Fatal("dead local must not remain live out of return block")
	}
}

func TestAMD64RootSlotsDoNotShareWithinBlock(t *testing.T) {
	fn := ir.NewFunction("root_same_block", types.TypeVoid)
	entry := fn.NewBlock("entry")
	first := fn.NewValue("first", types.TypeString)
	second := fn.NewValue("second", types.TypeString)
	entry.Instructions = append(entry.Instructions,
		&ir.CallInst{Res: first, Callee: "make_first"},
		&ir.CallInst{Res: second, Callee: "make_second"},
		&ir.CallInst{Callee: "consume", Args: []ir.Operand{first, second}},
	)
	entry.Terminator = &ir.ReturnTerm{}
	slots := amd64RootSlots(fn)
	if slots[first.ID] == slots[second.ID] {
		t.Fatalf("same-block roots shared slot %d", slots[first.ID])
	}
}

func TestAMD64RootSlotsReuseAcrossDeadSequentialBlocks(t *testing.T) {
	fn := ir.NewFunction("root_sequential_blocks", types.TypeVoid)
	firstBlock := fn.NewBlock("first")
	secondBlock := fn.NewBlock("second")
	first := fn.NewValue("first", types.TypeString)
	second := fn.NewValue("second", types.TypeString)
	firstBlock.Instructions = append(firstBlock.Instructions, &ir.CallInst{Res: first, Callee: "make_first"})
	firstBlock.Terminator = &ir.JumpTerm{Target: secondBlock}
	secondBlock.Instructions = append(secondBlock.Instructions, &ir.CallInst{Res: second, Callee: "make_second"})
	secondBlock.Terminator = &ir.ReturnTerm{}
	slots := amd64RootSlots(fn)
	if got := amd64RootSlotCount(slots); got != 1 {
		t.Fatalf("root slot count = %d, want 1 for non-overlapping block-local roots", got)
	}
	if slots[first.ID] != slots[second.ID] {
		t.Fatalf("sequential dead roots use slots %d and %d, want reuse", slots[first.ID], slots[second.ID])
	}
}

func TestAMD64GCLayoutDescriptorsCoverEveryObjectType(t *testing.T) {
	for typ := int64(0); typ <= amd64ObjectTypeCollection; typ++ {
		desc, ok := amd64GCLayoutForType(typ)
		if !ok {
			t.Fatalf("missing GC layout descriptor for object type %d", typ)
		}
		if desc.ObjectType != typ {
			t.Fatalf("descriptor type = %d, want %d", desc.ObjectType, typ)
		}
	}
	if desc, ok := amd64GCLayoutForType(amd64ObjectTypeAtomic); !ok || desc.TraceKind != amd64GCTraceAtomic {
		t.Fatalf("atomic descriptor = %#v, ok=%v", desc, ok)
	}
	if desc, ok := amd64GCLayoutForType(amd64ObjectTypeArrayData); !ok || desc.TraceKind != amd64GCTraceAtomic {
		t.Fatalf("number array data must be atomic: %#v, ok=%v", desc, ok)
	}
	if _, ok := amd64GCLayoutForType(amd64ObjectTypeCollection + 1); ok {
		t.Fatal("unknown object type must not resolve to a GC layout")
	}
}

func TestAMD64ChunkBitmapLayoutCoversDefaultChunk(t *testing.T) {
	if amd64ChunkSize%16 != 0 {
		t.Fatalf("chunk metadata size %d must preserve 16-byte object alignment", amd64ChunkSize)
	}
	if amd64ChunkMarkBitmap != amd64ChunkAllocBitmap+amd64ChunkBitmapBytes {
		t.Fatalf("mark bitmap offset = %d, want %d", amd64ChunkMarkBitmap, amd64ChunkAllocBitmap+amd64ChunkBitmapBytes)
	}
	objectSlots := ((1 << 20) - int(amd64ChunkSize)) / 16
	bitmapSlots := int(amd64ChunkBitmapBytes) * 8
	if objectSlots > bitmapSlots {
		t.Fatalf("default chunk has %d object slots but bitmap covers %d", objectSlots, bitmapSlots)
	}
}
