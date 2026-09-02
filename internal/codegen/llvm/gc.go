package llvm

import (
	"fmt"
	"strings"

	escapeanalysis "github.com/projectthorn/tsv7-bin/internal/analysis/escape"
	"github.com/projectthorn/tsv7-bin/internal/mir"
)

type stackFieldRoot struct {
	object mir.ValueID
	field  uint32
}

type gcRootLayout struct {
	slots        map[mir.ValueID]int
	stackFields  map[stackFieldRoot]int
	stackAliases map[mir.ValueID]mir.ValueID
	count        int
}

func buildGCRootLayout(fn mir.Function, shapes map[mir.ShapeID]mir.Shape, stackObjects map[mir.ValueID]bool, stackAliases map[mir.ValueID]mir.ValueID, scalarObjects map[mir.ValueID]escapeanalysis.ScalarObject) gcRootLayout {
	layout := gcRootLayout{slots: map[mir.ValueID]int{}, stackFields: map[stackFieldRoot]int{}, stackAliases: stackAliases}
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
			if stackObjects[inst.Result] {
				if shapeID, ok := stackObjectShapeForGC(inst.Op); ok {
					shape := shapes[shapeID]
					for field, info := range shape.Fields {
						if !isGCHeapReferenceRepr(info.Repr) {
							continue
						}
						key := stackFieldRoot{object: inst.Result, field: uint32(field)}
						layout.stackFields[key] = layout.count
						layout.count++
					}
				}
				continue
			}
			if _, ok := scalarObjects[inst.Result]; ok {
				continue
			}
			add(inst.Result, inst.Repr)
		}
	}
	return layout
}

func stackObjectShapeForGC(op mir.Operation) (mir.ShapeID, bool) {
	switch op := op.(type) {
	case mir.ObjectNew:
		return op.Shape, true
	case mir.ObjectAlloc:
		return op.Shape, true
	default:
		return 0, false
	}
}

func isGCReference(repr mir.Repr) bool {
	switch repr {
	case mir.ReprStringRef, mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef, mir.ReprChannelRef, mir.ReprJSValue:
		return true
	default:
		return false
	}
}

func emitsGCAllocation(op mir.Operation) bool {
	switch op.(type) {
	case mir.ConstString, mir.ConstJSValue, mir.StringConcat, mir.ArrayNewF64, mir.ArrayNewBool, mir.ArrayNewRef, mir.ObjectNew, mir.ObjectAlloc, mir.ClosureNew, mir.BoxJSValue, mir.DynamicAddJSValue, mir.DynamicFieldGet, mir.DynamicCall, mir.DynamicMethodCall, mir.PromiseResolve, mir.PromiseReject, mir.TaskSpawn, mir.ChannelNewF64, mir.ChannelNewBool, mir.ChannelNewRef:
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

func (layout gcRootLayout) emitStackFieldSync(b *strings.Builder, inst mir.Instruction, values map[mir.ValueID]string) error {
	switch op := inst.Op.(type) {
	case mir.ObjectNew:
		for field, valueID := range op.Fields {
			slot, ok := layout.stackFields[stackFieldRoot{object: inst.Result, field: uint32(field)}]
			if !ok {
				continue
			}
			value, err := operand(values, valueID)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "  store ptr %s, ptr %s\n", value, gcSlotName(slot))
		}
	case mir.FieldSet:
		object := op.Object
		if origin, ok := layout.stackAliases[object]; ok {
			object = origin
		}
		slot, ok := layout.stackFields[stackFieldRoot{object: object, field: op.Field}]
		if !ok {
			return nil
		}
		value, err := operand(values, op.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  store ptr %s, ptr %s\n", value, gcSlotName(slot))
	}
	return nil
}
