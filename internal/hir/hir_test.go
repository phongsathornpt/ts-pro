package hir

import "testing"

func TestSemanticTypeAndNativeRepresentationStaySeparate(t *testing.T) {
	semantic := SemanticType{Kind: TypeNumber}
	repr := Repr{Kind: ReprF64}

	if semantic.Kind != TypeNumber {
		t.Fatalf("semantic kind = %v", semantic.Kind)
	}
	if repr.Kind != ReprF64 || !repr.Proven() {
		t.Fatalf("repr = %+v", repr)
	}
}

func TestFunctionCarriesTypedHIRContracts(t *testing.T) {
	value := NewValueID(0)
	module := Module{
		ID:    NewModuleID(0),
		Name:  "math",
		Types: []SemanticType{{Kind: TypeNumber}},
		Functions: []Function{{
			ID:         NewFunctionID(0),
			Name:       "identity",
			Params:     []Param{{Value: value, Name: "x", SemanticType: NewTypeID(0), Repr: Repr{Kind: ReprF64}}},
			ReturnType: NewTypeID(0),
			ReturnRepr: Repr{Kind: ReprF64},
			Entry:      NewBlockID(0),
			Blocks:     []Block{{ID: NewBlockID(0), Terminator: ReturnTerm{Value: &value}}},
		}},
	}

	if module.Functions[0].ReturnRepr.Kind != ReprF64 {
		t.Fatalf("return repr = %+v", module.Functions[0].ReturnRepr)
	}
}
