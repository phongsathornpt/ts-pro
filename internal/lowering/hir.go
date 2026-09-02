package lowering

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/frontend"
	"github.com/phongsathornpt/ts-pro/internal/hir"
)

type moduleLowerer struct {
	source frontend.Snapshot
	types  map[frontend.TypeID]hir.TypeID
	module hir.Module
}

func LowerHIR(source frontend.Snapshot, name string) (hir.Module, error) {
	lowerer := &moduleLowerer{
		source: source,
		types:  map[frontend.TypeID]hir.TypeID{},
		module: hir.Module{ID: hir.NewModuleID(0), Name: name},
	}
	if err := lowerer.lowerTypes(); err != nil {
		return hir.Module{}, err
	}
	if err := lowerer.lowerShapes(); err != nil {
		return hir.Module{}, err
	}
	for _, fn := range source.Functions {
		lowered, err := lowerer.lowerFunction(fn)
		if err != nil {
			return hir.Module{}, err
		}
		lowerer.module.Functions = append(lowerer.module.Functions, lowered)
	}
	if len(source.Entry) != 0 {
		voidType, ok := findFrontendType(source, frontend.TypeVoid)
		if !ok {
			return hir.Module{}, fmt.Errorf("top-level entry requires a void semantic type")
		}
		entrySource := frontend.Function{
			ID: frontend.FunctionID(len(source.Functions)), Name: "__entry",
			ReturnType: voidType, Body: source.Entry,
		}
		lowered, err := lowerer.lowerFunction(entrySource)
		if err != nil {
			return hir.Module{}, err
		}
		lowerer.module.Functions = append(lowerer.module.Functions, lowered)
		entryID := lowered.ID
		lowerer.module.Entry = &entryID
	}
	if err := lowerer.module.Verify(); err != nil {
		return hir.Module{}, fmt.Errorf("verify lowered HIR: %w", err)
	}
	return lowerer.module, nil
}

func (l *moduleLowerer) lowerTypes() error {
	canonical := map[frontend.TypeKind]hir.TypeID{}
	for _, typ := range l.source.Types {
		kind, err := lowerTypeKind(typ.Kind)
		if err != nil {
			return fmt.Errorf("lower type %q: %w", typ.Name, err)
		}
		if existing, ok := canonical[typ.Kind]; ok && isScalarType(typ.Kind) {
			l.types[typ.ID] = existing
			continue
		}
		id := hir.NewTypeID(uint32(len(l.module.Types)))
		l.module.Types = append(l.module.Types, hir.SemanticType{Kind: kind})
		l.types[typ.ID] = id
		if isScalarType(typ.Kind) {
			canonical[typ.Kind] = id
		}
	}

	for _, typ := range l.source.Types {
		id := l.types[typ.ID]
		semantic := &l.module.Types[id]
		switch typ.Kind {
		case frontend.TypeObject:
			semantic.Shape = hir.NewShapeID(uint32(typ.Shape))
		case frontend.TypeArray:
			element, ok := l.types[typ.Element]
			if !ok {
				return fmt.Errorf("array type %q references unavailable element type t%d", typ.Name, typ.Element)
			}
			semantic.Element = element
		case frontend.TypeUnion:
			for _, member := range typ.Members {
				mapped, ok := l.types[member]
				if !ok {
					return fmt.Errorf("union type %q references unavailable member t%d", typ.Name, member)
				}
				semantic.Members = append(semantic.Members, mapped)
			}
		case frontend.TypeTask, frontend.TypePromise:
			mapped, ok := l.types[typ.ReturnType]
			if !ok {
				return fmt.Errorf("task type %q references unavailable result t%d", typ.Name, typ.ReturnType)
			}
			semantic.ReturnType = mapped
		case frontend.TypeChannel:
			mapped, ok := l.types[typ.Element]
			if !ok {
				return fmt.Errorf("channel type %q references unavailable element t%d", typ.Name, typ.Element)
			}
			semantic.Element = mapped
		case frontend.TypeFunction:
			for _, param := range typ.Params {
				mapped, ok := l.types[param]
				if !ok {
					return fmt.Errorf("function type %q references unavailable parameter t%d", typ.Name, param)
				}
				semantic.Params = append(semantic.Params, mapped)
			}
			mapped, ok := l.types[typ.ReturnType]
			if !ok {
				return fmt.Errorf("function type %q references unavailable return t%d", typ.Name, typ.ReturnType)
			}
			semantic.ReturnType = mapped
		}
	}
	return nil
}

func (l *moduleLowerer) lowerShapes() error {
	for _, shape := range l.source.Shapes {
		lowered := hir.Shape{ID: hir.NewShapeID(uint32(shape.ID)), Name: shape.Name, ClassTag: shape.ClassTag}
		for _, field := range shape.Fields {
			typeID, ok := l.types[field.Type]
			if !ok {
				return fmt.Errorf("shape %s field %s references unavailable type t%d", shape.Name, field.Name, field.Type)
			}
			lowered.Fields = append(lowered.Fields, hir.ShapeField{Name: field.Name, SemanticType: typeID})
		}
		l.module.Shapes = append(l.module.Shapes, lowered)
	}
	return nil
}

func isScalarType(kind frontend.TypeKind) bool {
	switch kind {
	case frontend.TypeAny, frontend.TypeUnknown, frontend.TypeNever, frontend.TypeVoid,
		frontend.TypeUndefined, frontend.TypeNull, frontend.TypeBoolean, frontend.TypeNumber, frontend.TypeString:
		return true
	default:
		return false
	}
}

func lowerTypeKind(kind frontend.TypeKind) (hir.TypeKind, error) {
	switch kind {
	case frontend.TypeAny:
		return hir.TypeAny, nil
	case frontend.TypeUnknown:
		return hir.TypeUnknown, nil
	case frontend.TypeNever:
		return hir.TypeNever, nil
	case frontend.TypeVoid:
		return hir.TypeVoid, nil
	case frontend.TypeUndefined:
		return hir.TypeUndefined, nil
	case frontend.TypeNull:
		return hir.TypeNull, nil
	case frontend.TypeBoolean:
		return hir.TypeBoolean, nil
	case frontend.TypeNumber:
		return hir.TypeNumber, nil
	case frontend.TypeString:
		return hir.TypeString, nil
	case frontend.TypeArray:
		return hir.TypeArray, nil
	case frontend.TypeObject:
		return hir.TypeObject, nil
	case frontend.TypeUnion:
		return hir.TypeUnion, nil
	case frontend.TypeFunction:
		return hir.TypeFunction, nil
	case frontend.TypeTask:
		return hir.TypeTask, nil
	case frontend.TypePromise:
		return hir.TypePromise, nil
	case frontend.TypeChannel:
		return hir.TypeChannel, nil
	case frontend.TypeTaskGroup:
		return hir.TypeTaskGroup, nil
	case frontend.TypeParameter:
		return hir.TypeParameter, nil
	default:
		return hir.TypeInvalid, fmt.Errorf("semantic type kind %d is not supported by the HIR MVP", kind)
	}
}

func findFrontendType(source frontend.Snapshot, kind frontend.TypeKind) (frontend.TypeID, bool) {
	for _, typ := range source.Types {
		if typ.Kind == kind {
			return typ.ID, true
		}
	}
	return 0, false
}
