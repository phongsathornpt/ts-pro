package hir

import "testing"

func TestDumpTextIsDeterministic(t *testing.T) {
	v0, v1 := NewValueID(0), NewValueID(1)
	module := Module{
		ID: NewModuleID(0), Name: "math",
		Types: []SemanticType{{Kind: TypeNumber}},
		Functions: []Function{
			{
				ID: NewFunctionID(1), Name: "later", ReturnType: NewTypeID(0),
				ReturnRepr: Repr{Kind: ReprF64}, Entry: NewBlockID(0),
				Blocks: []Block{{ID: NewBlockID(0), Terminator: ReturnTerm{}}},
			},
			{
				ID: NewFunctionID(0), Name: "add", ReturnType: NewTypeID(0),
				ReturnRepr: Repr{Kind: ReprF64}, Entry: NewBlockID(0),
				Params: []Param{{Value: v0, Name: "x", SemanticType: NewTypeID(0), Repr: Repr{Kind: ReprF64}}},
				Blocks: []Block{{ID: NewBlockID(0), Instructions: []Instruction{{Result: v1, SemanticType: NewTypeID(0), Repr: Repr{Kind: ReprF64}, Op: BinaryExpr{Operator: BinaryAdd, Left: v0, Right: v0}}}, Terminator: ReturnTerm{Value: &v1}}},
			},
		},
	}

	got := DumpText(module)
	want := "module @\"math\" m0 {\n" +
		"  type t0 = number\n" +
		"  func f0 @\"add\"(v0 \"x\":t0[f64]) -> t0[f64] entry b0 {\n" +
		"    b0:\n" +
		"      v1:t0[f64] = add v0, v0\n" +
		"      return v1\n" +
		"  }\n" +
		"  func f1 @\"later\"() -> t0[f64] entry b0 {\n" +
		"    b0:\n" +
		"      return\n" +
		"  }\n" +
		"}\n"
	if got != want {
		t.Fatalf("HIR dump mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}

	module.Functions[0], module.Functions[1] = module.Functions[1], module.Functions[0]
	if again := DumpText(module); again != want {
		t.Fatalf("HIR dump changed after function reorder:\n%s", again)
	}
}
