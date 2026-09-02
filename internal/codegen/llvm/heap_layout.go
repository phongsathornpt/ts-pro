package llvm

import (
	"fmt"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func shapeRefDescriptorName(id mir.ShapeID) string {
	return fmt.Sprintf("@tsnative_refs_shape_s%d", id)
}

func closureEnvRefDescriptorName(id mir.FunctionID) string {
	return fmt.Sprintf("@tsnative_refs_env_f%d", id)
}

func taskEnvRefDescriptorName(id mir.FunctionID) string {
	return fmt.Sprintf("@tsnative_refs_task_f%d", id)
}

const closureRefDescriptorName = "@tsnative_refs_closure"

func emitRefOffsetDescriptor(b *strings.Builder, name, typeName string, fields []int) {
	if len(fields) == 0 {
		return
	}
	fmt.Fprintf(b, "%s = private constant [%d x i64] [", name, len(fields))
	for i, field := range fields {
		if i != 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "i64 ptrtoint (ptr getelementptr (%s, ptr null, i32 0, i32 %d) to i64)", typeName, field)
	}
	b.WriteString("]\n")
}

func emitHeapObjectAlloc(b *strings.Builder, result, size, descriptor string, refs int) {
	if refs == 0 {
		fmt.Fprintf(b, "  %s = call ptr @tsnative_object_alloc_atomic(i64 %s)\n", result, size)
		return
	}
	fmt.Fprintf(b, "  %s = call ptr @tsnative_object_alloc_refs(i64 %s, ptr %s, i64 %d)\n", result, size, descriptor, refs)
}
func shapeRefFields(shape mir.Shape) []int {
	fields := make([]int, 0, len(shape.Fields))
	for i, field := range shape.Fields {
		if isGCHeapReferenceRepr(field.Repr) {
			fields = append(fields, int(shapeFieldIndex(shape, uint32(i))))
		}
	}
	return fields
}

func closureEnvRefFields(fn mir.Function, captures int) []int {
	fields := make([]int, 0, captures)
	for i := 0; i < captures; i++ {
		if isGCHeapReferenceRepr(fn.Params[i].Repr) {
			fields = append(fields, i)
		}
	}
	return fields
}
func taskEnvRefFields(fn mir.Function, descriptor taskDescriptor) []int {
	fields := make([]int, 0)
	for i := 0; i < descriptor.CaptureCount; i++ {
		if isGCHeapReferenceRepr(fn.Params[i].Repr) {
			fields = append(fields, i)
		}
	}
	if descriptor.Continuation == nil {
		return fields
	}
	base := descriptor.CaptureCount + 1
	for _, value := range descriptor.Continuation.sortedSpillValues() {
		slot := descriptor.Continuation.SpillSlots[value]
		if isGCHeapReferenceRepr(slot.Repr) {
			fields = append(fields, base+slot.Index)
		}
	}
	return fields
}
func (e *emitter) emitHeapTraceDescriptors(b *strings.Builder) error {
	for _, shape := range e.module.Shapes {
		fields := shapeRefFields(shape)
		emitRefOffsetDescriptor(b, shapeRefDescriptorName(shape.ID), shapeTypeName(shape.ID), fields)
	}
	for _, descriptor := range sortedClosureDescriptors(e.closures) {
		if descriptor.CaptureCount == 0 {
			continue
		}
		fn := e.functions[descriptor.Callee]
		fields := closureEnvRefFields(fn, descriptor.CaptureCount)
		emitRefOffsetDescriptor(b, closureEnvRefDescriptorName(descriptor.Callee), closureEnvTypeName(descriptor.Callee), fields)
	}
	descriptors, err := e.taskDescriptors()
	if err != nil {
		return err
	}
	for _, descriptor := range descriptors {
		if descriptor.CaptureCount == 0 && descriptor.Continuation == nil {
			continue
		}
		fn := e.functions[descriptor.Callee]
		fields := taskEnvRefFields(fn, descriptor)
		emitRefOffsetDescriptor(b, taskEnvRefDescriptorName(descriptor.Callee), taskEnvTypeName(descriptor.Callee), fields)
	}
	if len(e.closures) != 0 || len(e.promiseThenableResults()) != 0 {
		emitRefOffsetDescriptor(b, closureRefDescriptorName, "%tsnative_closure", []int{1})
	}
	b.WriteString("\n")
	return nil
}
