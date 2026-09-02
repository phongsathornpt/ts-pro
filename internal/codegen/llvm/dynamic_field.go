package llvm

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func dynamicFieldGetHelperName(field string) string {
	sum := sha256.Sum256([]byte(field))
	return fmt.Sprintf("tsnative_dynamic_get_%x", sum[:8])
}

func (e *emitter) dynamicFieldNames() []string {
	set := map[string]struct{}{}
	for _, fn := range e.module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				if op, ok := inst.Op.(mir.DynamicFieldGet); ok {
					set[op.Field] = struct{}{}
				}
			}
		}
	}
	fields := make([]string, 0, len(set))
	for field := range set {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func (e *emitter) emitDynamicFieldGetHelpers(b *strings.Builder) error {
	for _, field := range e.dynamicFieldNames() {
		if err := e.emitDynamicFieldGetHelper(b, field); err != nil {
			return err
		}
	}
	return nil
}

func (e *emitter) emitDynamicFieldGetHelper(b *strings.Builder, fieldName string) error {
	type fieldCase struct {
		shape mir.Shape
		field int
	}
	cases := make([]fieldCase, 0)
	for _, shape := range e.module.Shapes {
		for i, field := range shape.Fields {
			if field.Name == fieldName {
				cases = append(cases, fieldCase{shape: shape, field: i})
				break
			}
		}
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].shape.ID < cases[j].shape.ID })

	name := dynamicFieldGetHelperName(fieldName)
	fmt.Fprintf(b, "define ptr @%s(ptr %%boxed) {\nentry:\n", name)
	b.WriteString("  %shape = call i32 @tsnative_jsvalue_object_shape(ptr %boxed)\n")
	b.WriteString("  switch i32 %shape, label %missing [")
	for _, c := range cases {
		fmt.Fprintf(b, " i32 %d, label %%shape%d", c.shape.ID, c.shape.ID)
	}
	b.WriteString(" ]\n")
	for _, c := range cases {
		field := c.shape.Fields[c.field]
		if err := validateDynamicFieldRepr(field.Repr); err != nil {
			return fmt.Errorf("dynamic property %q shape %s: %w", fieldName, c.shape.Name, err)
		}
		fmt.Fprintf(b, "shape%d:\n", c.shape.ID)
		b.WriteString("  %object")
		fmt.Fprintf(b, ".s%d = call ptr @tsnative_jsvalue_unbox_object(ptr %%boxed)\n", c.shape.ID)
		fmt.Fprintf(b, "  %%slot.s%d = getelementptr %s, ptr %%object.s%d, i32 0, i32 %d\n", c.shape.ID, shapeTypeName(c.shape.ID), c.shape.ID, shapeFieldIndex(c.shape, uint32(c.field)))
		if err := emitDynamicFieldBox(b, c.shape.ID, field); err != nil {
			return fmt.Errorf("dynamic property %q shape %s: %w", fieldName, c.shape.Name, err)
		}
	}
	b.WriteString("missing:\n  %undefined = call ptr @tsnative_jsvalue_undefined()\n  ret ptr %undefined\n}\n\n")
	return nil
}

func validateDynamicFieldRepr(repr mir.Repr) error {
	switch repr {
	case mir.ReprF64, mir.ReprBool, mir.ReprStringRef, mir.ReprArrayRef, mir.ReprObjectRef, mir.ReprFunctionRef, mir.ReprJSValue:
		return nil
	default:
		return fmt.Errorf("representation %d cannot be boxed for dynamic property access", repr)
	}
}

func emitDynamicFieldBox(b *strings.Builder, shapeID mir.ShapeID, field mir.ShapeField) error {
	prefix := fmt.Sprintf("%%field.s%d", shapeID)
	slot := fmt.Sprintf("%%slot.s%d", shapeID)
	switch field.Repr {
	case mir.ReprF64:
		fmt.Fprintf(b, "  %s = load double, ptr %s\n  %s.box = call ptr @tsnative_jsvalue_box_f64(double %s)\n  ret ptr %s.box\n", prefix, slot, prefix, prefix, prefix)
	case mir.ReprBool:
		fmt.Fprintf(b, "  %s = load i1, ptr %s\n  %s.i8 = zext i1 %s to i8\n  %s.box = call ptr @tsnative_jsvalue_box_bool(i8 %s.i8)\n  ret ptr %s.box\n", prefix, slot, prefix, prefix, prefix, prefix, prefix)
	case mir.ReprStringRef:
		fmt.Fprintf(b, "  %s = load ptr, ptr %s\n  %s.box = call ptr @tsnative_jsvalue_box_string(ptr %s)\n  ret ptr %s.box\n", prefix, slot, prefix, prefix, prefix)
	case mir.ReprArrayRef:
		fmt.Fprintf(b, "  %s = load ptr, ptr %s\n  %s.box = call ptr @tsnative_jsvalue_box_array(ptr %s)\n  ret ptr %s.box\n", prefix, slot, prefix, prefix, prefix)
	case mir.ReprObjectRef:
		fmt.Fprintf(b, "  %s = load ptr, ptr %s\n", prefix, slot)
		if field.HasObjectShape {
			fmt.Fprintf(b, "  %s.box = call ptr @tsnative_jsvalue_box_object_shape(ptr %s, i32 %d)\n", prefix, prefix, field.ObjectShape)
		} else {
			fmt.Fprintf(b, "  %s.box = call ptr @tsnative_jsvalue_box_object(ptr %s)\n", prefix, prefix)
		}
		fmt.Fprintf(b, "  ret ptr %s.box\n", prefix)
	case mir.ReprFunctionRef:
		fmt.Fprintf(b, "  %s = load ptr, ptr %s\n  %s.box = call ptr @tsnative_jsvalue_box_function(ptr %s)\n  ret ptr %s.box\n", prefix, slot, prefix, prefix, prefix)
	case mir.ReprJSValue:
		fmt.Fprintf(b, "  %s = load ptr, ptr %s\n  ret ptr %s\n", prefix, slot, prefix)
	default:
		return fmt.Errorf("representation %d cannot be boxed", field.Repr)
	}
	return nil
}

func dynamicFieldSetHelperName(field string) string {
	sum := sha256.Sum256([]byte("set:" + field))
	return fmt.Sprintf("tsnative_dynamic_set_%x", sum[:8])
}

func (e *emitter) dynamicFieldSetNames() []string {
	set := map[string]struct{}{}
	for _, fn := range e.module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				if op, ok := inst.Op.(mir.DynamicFieldSet); ok {
					set[op.Field] = struct{}{}
				}
			}
		}
	}
	fields := make([]string, 0, len(set))
	for field := range set {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func (e *emitter) emitDynamicFieldSetHelpers(b *strings.Builder) error {
	for _, field := range e.dynamicFieldSetNames() {
		if err := e.emitDynamicFieldSetHelper(b, field); err != nil {
			return err
		}
	}
	return nil
}

func (e *emitter) emitDynamicFieldSetHelper(b *strings.Builder, fieldName string) error {
	type fieldCase struct {
		shape mir.Shape
		field int
	}
	cases := make([]fieldCase, 0)
	for _, shape := range e.module.Shapes {
		for i, field := range shape.Fields {
			if field.Name == fieldName {
				cases = append(cases, fieldCase{shape: shape, field: i})
				break
			}
		}
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].shape.ID < cases[j].shape.ID })

	name := dynamicFieldSetHelperName(fieldName)
	fmt.Fprintf(b, "define ptr @%s(ptr %%boxed, ptr %%value) {\nentry:\n", name)
	b.WriteString("  %shape = call i32 @tsnative_jsvalue_object_shape(ptr %boxed)\n")
	b.WriteString("  switch i32 %shape, label %missing [")
	for _, c := range cases {
		fmt.Fprintf(b, " i32 %d, label %%shape%d", c.shape.ID, c.shape.ID)
	}
	b.WriteString(" ]\n")
	for _, c := range cases {
		field := c.shape.Fields[c.field]
		if err := validateDynamicFieldRepr(field.Repr); err != nil {
			return fmt.Errorf("dynamic property write %q shape %s: %w", fieldName, c.shape.Name, err)
		}
		fmt.Fprintf(b, "shape%d:\n", c.shape.ID)
		fmt.Fprintf(b, "  %%object.s%d = call ptr @tsnative_jsvalue_unbox_object(ptr %%boxed)\n", c.shape.ID)
		fmt.Fprintf(b, "  %%slot.s%d = getelementptr %s, ptr %%object.s%d, i32 0, i32 %d\n", c.shape.ID, shapeTypeName(c.shape.ID), c.shape.ID, shapeFieldIndex(c.shape, uint32(c.field)))
		if err := emitDynamicFieldStore(b, c.shape.ID, field); err != nil {
			return fmt.Errorf("dynamic property write %q shape %s: %w", fieldName, c.shape.Name, err)
		}
	}
	b.WriteString("missing:\n  call void @tsnative_jsvalue_dynamic_set_missing()\n  unreachable\n}\n\n")
	return nil
}

func emitDynamicFieldStore(b *strings.Builder, shapeID mir.ShapeID, field mir.ShapeField) error {
	prefix := fmt.Sprintf("%%set.s%d", shapeID)
	slot := fmt.Sprintf("%%slot.s%d", shapeID)
	object := fmt.Sprintf("%%object.s%d", shapeID)
	switch field.Repr {
	case mir.ReprF64:
		fmt.Fprintf(b, "  %s = call double @tsnative_jsvalue_unbox_f64(ptr %%value)\n  store double %s, ptr %s\n  ret ptr %%value\n", prefix, prefix, slot)
	case mir.ReprBool:
		fmt.Fprintf(b, "  %s.i8 = call i8 @tsnative_jsvalue_unbox_bool(ptr %%value)\n  %s = trunc i8 %s.i8 to i1\n  store i1 %s, ptr %s\n  ret ptr %%value\n", prefix, prefix, prefix, prefix, slot)
	case mir.ReprStringRef:
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_string(ptr %%value)\n  call void @tsnative_gc_store_ref(ptr %s, ptr %s, ptr %s)\n  ret ptr %%value\n", prefix, object, slot, prefix)
	case mir.ReprArrayRef:
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_array(ptr %%value)\n  call void @tsnative_gc_store_ref(ptr %s, ptr %s, ptr %s)\n  ret ptr %%value\n", prefix, object, slot, prefix)
	case mir.ReprObjectRef:
		if field.HasObjectShape {
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_object_shape(ptr %%value, i32 %d)\n", prefix, field.ObjectShape)
		} else {
			fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_object(ptr %%value)\n", prefix)
		}
		fmt.Fprintf(b, "  call void @tsnative_gc_store_ref(ptr %s, ptr %s, ptr %s)\n  ret ptr %%value\n", object, slot, prefix)
	case mir.ReprFunctionRef:
		fmt.Fprintf(b, "  %s = call ptr @tsnative_jsvalue_unbox_function(ptr %%value)\n  call void @tsnative_gc_store_ref(ptr %s, ptr %s, ptr %s)\n  ret ptr %%value\n", prefix, object, slot, prefix)
	case mir.ReprJSValue:
		fmt.Fprintf(b, "  call void @tsnative_gc_store_ref(ptr %s, ptr %s, ptr %%value)\n  ret ptr %%value\n", object, slot)
	default:
		return fmt.Errorf("representation %d cannot be assigned from JSValue", field.Repr)
	}
	return nil
}
