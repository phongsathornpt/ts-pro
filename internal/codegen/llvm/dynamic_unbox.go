package llvm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func objectUnboxHelperName(shape mir.ShapeID) string {
	return fmt.Sprintf("tsnative_unbox_object_s%d", shape)
}

func (e *emitter) objectUnboxShapes() []mir.ShapeID {
	set := map[mir.ShapeID]struct{}{}
	for _, fn := range e.module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				op, ok := inst.Op.(mir.UnboxJSValue)
				if ok && op.Kind == mir.UnboxJSObject {
					set[op.Shape] = struct{}{}
				}
			}
		}
	}
	result := make([]mir.ShapeID, 0, len(set))
	for shape := range set {
		result = append(result, shape)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func (e *emitter) emitObjectUnboxHelpers(b *strings.Builder) error {
	for _, expected := range e.objectUnboxShapes() {
		if err := e.emitObjectUnboxHelper(b, expected); err != nil {
			return err
		}
	}
	return nil
}

func (e *emitter) emitObjectUnboxHelper(b *strings.Builder, expected mir.ShapeID) error {
	if _, ok := e.shapes[expected]; !ok {
		return fmt.Errorf("object unbox helper references unknown shape s%d", expected)
	}
	compatible := make([]mir.ShapeID, 0)
	for _, candidate := range e.module.Shapes {
		if e.structurallyCompatibleShapes(candidate.ID, expected, map[[2]mir.ShapeID]bool{}) {
			compatible = append(compatible, candidate.ID)
		}
	}
	sort.Slice(compatible, func(i, j int) bool { return compatible[i] < compatible[j] })
	fmt.Fprintf(b, "define ptr @%s(ptr %%boxed) {\nentry:\n", objectUnboxHelperName(expected))
	b.WriteString("  %shape = call i32 @tsnative_jsvalue_object_shape(ptr %boxed)\n")
	b.WriteString("  switch i32 %shape, label %invalid [")
	for _, shape := range compatible {
		fmt.Fprintf(b, " i32 %d, label %%valid", shape)
	}
	b.WriteString(" ]\nvalid:\n  %object = call ptr @tsnative_jsvalue_unbox_object(ptr %boxed)\n  ret ptr %object\n")
	fmt.Fprintf(b, "invalid:\n  %%abort = call ptr @tsnative_jsvalue_unbox_object_shape(ptr %%boxed, i32 %d)\n  unreachable\n}\n\n", expected)
	return nil
}

func (e *emitter) structurallyCompatibleShapes(actual, expected mir.ShapeID, visiting map[[2]mir.ShapeID]bool) bool {
	if actual == expected {
		return true
	}
	pair := [2]mir.ShapeID{actual, expected}
	if visiting[pair] {
		return true
	}
	left, lok := e.shapes[actual]
	right, rok := e.shapes[expected]
	if !lok || !rok || len(left.Fields) != len(right.Fields) {
		return false
	}
	visiting[pair] = true
	defer delete(visiting, pair)
	for i := range left.Fields {
		a, b := left.Fields[i], right.Fields[i]
		if a.Name != b.Name || a.Repr != b.Repr || a.HasObjectShape != b.HasObjectShape {
			return false
		}
		if a.HasObjectShape && !e.structurallyCompatibleShapes(a.ObjectShape, b.ObjectShape, visiting) {
			return false
		}
	}
	return true
}
