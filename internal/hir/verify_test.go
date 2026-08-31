package hir

import (
	"strings"
	"testing"
)

func TestVerifyAcceptsValidModule(t *testing.T) {
	module := testIdentityModule()
	if err := module.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRejectsMissingEntryAndUnknownValue(t *testing.T) {
	module := testIdentityModule()
	module.Functions[0].Entry = NewBlockID(99)
	module.Functions[0].Blocks[0].Terminator = ReturnTerm{Value: valuePtr(NewValueID(42))}

	err := module.Verify()
	if err == nil {
		t.Fatal("expected verification failure")
	}
	text := err.Error()
	if !strings.Contains(text, "missing entry block b99") || !strings.Contains(text, "unknown value v42") {
		t.Fatalf("unexpected verification error: %s", text)
	}
}
func testIdentityModule() Module {
	value := NewValueID(0)
	return Module{
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
}

func valuePtr(value ValueID) *ValueID {
	return &value
}
