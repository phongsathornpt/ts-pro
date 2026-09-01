package llvm

import (
	"fmt"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

type gcRootLayout struct {
	slots map[mir.ValueID]int
	count int
}

func buildGCRootLayout(fn mir.Function) gcRootLayout {
	layout := gcRootLayout{slots: map[mir.ValueID]int{}}
	add := func(value mir.ValueID, repr mir.Repr) {
		if !isGCReference(repr) {
			return
		}
		if _, exists := layout.slots[value]; exists {
			return
		}
		layout.slots[value] = layout.count
		layout.count++
	}
	for _, param := range fn.Params {
		add(param.Value, param.Repr)
	}
	for _, block := range fn.Blocks {
		for _, inst := range block.Instructions {
			add(inst.Result, inst.Repr)
		}
	}
	return layout
}

func isGCReference(repr mir.Repr) bool {
	switch repr {
	case mir.ReprStringRef, mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef, mir.ReprJSValue:
		return true
	default:
		return false
	}
}

func emitsGCAllocation(op mir.Operation) bool {
	switch op.(type) {
	case mir.ConstString, mir.StringConcat, mir.ArrayNewF64, mir.ObjectNew, mir.ObjectAlloc, mir.ClosureNew, mir.BoxJSValue, mir.DynamicAddJSValue:
		return true
	default:
		return false
	}
}

func gcSlotName(index int) string {
	return fmt.Sprintf("%%gc.slot.%d", index)
}

func (layout gcRootLayout) emitPrologue(b *strings.Builder, fn mir.Function) {
	if layout.count == 0 {
		return
	}
	b.WriteString("entry:\n")
	fmt.Fprintf(b, "  %%gc.roots = alloca [%d x ptr]\n", layout.count)
	for i := 0; i < layout.count; i++ {
		fmt.Fprintf(b, "  %s = getelementptr [%d x ptr], ptr %%gc.roots, i32 0, i32 %d\n", gcSlotName(i), layout.count, i)
		fmt.Fprintf(b, "  store ptr null, ptr %s\n", gcSlotName(i))
	}
	for _, param := range fn.Params {
		if slot, ok := layout.slots[param.Value]; ok {
			fmt.Fprintf(b, "  store ptr %s, ptr %s\n", valueName(param.Value), gcSlotName(slot))
		}
	}
	fmt.Fprintf(b, "  %%gc.frame = call ptr @tsnative_gc_enter(ptr %%gc.roots, i64 %d)\n", layout.count)
	fmt.Fprintf(b, "  br label %%b%d\n", fn.Entry)
}

func (layout gcRootLayout) emitStore(b *strings.Builder, inst mir.Instruction, values map[mir.ValueID]string) error {
	slot, ok := layout.slots[inst.Result]
	if !ok {
		return nil
	}
	value, err := operand(values, inst.Result)
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "  store ptr %s, ptr %s\n", value, gcSlotName(slot))
	return nil
}
