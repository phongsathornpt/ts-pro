package repr

import (
	"fmt"

	"github.com/projectthorn/tsv7-bin/internal/hir"
)

type Diagnostic struct {
	Function hir.FunctionID
	Value    *hir.ValueID
	Message  string
}

// Analyze assigns native runtime representations from semantic HIR types.
// It intentionally does not infer integer representations yet; TypeScript number
// is represented as IEEE-754 f64 until range analysis proves a narrower integer.
func Analyze(module *hir.Module) []Diagnostic {
	var diagnostics []Diagnostic
	for si := range module.Shapes {
		for fi := range module.Shapes[si].Fields {
			field := &module.Shapes[si].Fields[fi]
			field.Repr, diagnostics = assignType(module, 0, nil, field.SemanticType, diagnostics)
		}
	}
	for fi := range module.Functions {
		fn := &module.Functions[fi]
		fn.ReturnRepr, diagnostics = assignType(module, fn.ID, nil, fn.ReturnType, diagnostics)
		for pi := range fn.Params {
			param := &fn.Params[pi]
			value := param.Value
			param.Repr, diagnostics = assignType(module, fn.ID, &value, param.SemanticType, diagnostics)
		}
		for bi := range fn.Blocks {
			for ii := range fn.Blocks[bi].Instructions {
				instruction := &fn.Blocks[bi].Instructions[ii]
				value := instruction.Result
				instruction.Repr, diagnostics = assignType(module, fn.ID, &value, instruction.SemanticType, diagnostics)
			}
		}
	}
	return diagnostics
}

func assignType(module *hir.Module, fn hir.FunctionID, value *hir.ValueID, typeID hir.TypeID, diagnostics []Diagnostic) (hir.Repr, []Diagnostic) {
	if int(typeID) >= len(module.Types) {
		diagnostics = append(diagnostics, Diagnostic{Function: fn, Value: value, Message: fmt.Sprintf("invalid semantic type t%d", typeID)})
		return hir.Repr{}, diagnostics
	}
	typ := module.Types[typeID]
	switch typ.Kind {
	case hir.TypeVoid:
		return hir.Repr{Kind: hir.ReprVoid}, diagnostics
	case hir.TypeBoolean:
		return hir.Repr{Kind: hir.ReprBool}, diagnostics
	case hir.TypeNumber:
		return hir.Repr{Kind: hir.ReprF64}, diagnostics
	case hir.TypeString:
		return hir.Repr{Kind: hir.ReprStringRef}, diagnostics
	case hir.TypeArray:
		return hir.Repr{Kind: hir.ReprArrayRef}, diagnostics
	case hir.TypeObject:
		return hir.Repr{Kind: hir.ReprObjectRef, Shape: typ.Shape}, diagnostics
	case hir.TypeFunction:
		return hir.Repr{Kind: hir.ReprFunctionRef}, diagnostics
	case hir.TypeUnion:
		return hir.Repr{Kind: hir.ReprTaggedUnion}, diagnostics
	case hir.TypeAny, hir.TypeUnknown, hir.TypeUndefined, hir.TypeNull:
		diagnostics = append(diagnostics, Diagnostic{
			Function: fn, Value: value,
			Message: fmt.Sprintf("t%d requires dynamic JSValue representation", typeID),
		})
		return hir.Repr{Kind: hir.ReprJSValue}, diagnostics
	case hir.TypeNever:
		return hir.Repr{Kind: hir.ReprVoid}, diagnostics
	default:
		diagnostics = append(diagnostics, Diagnostic{Function: fn, Value: value, Message: fmt.Sprintf("t%d has no proven native representation", typeID)})
		return hir.Repr{}, diagnostics
	}
}
