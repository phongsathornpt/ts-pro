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
