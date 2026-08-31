package frontend

import "testing"

func TestSnapshotOwnsSemanticDTOs(t *testing.T) {
	snapshot := Snapshot{
		ProjectVersion: "1",
		Sources:        []Source{{ID: 0, URI: "file:///main.ts", Path: "/main.ts", Version: 1}},
		Types:          []Type{{ID: 0, Kind: TypeNumber, Name: "number"}},
		Symbols:        []Symbol{{ID: 0, Name: "x", Kind: SymbolParameter, Type: 0}},
		Functions: []Function{{
			ID: 0, Symbol: 0, Name: "identity", Source: 0,
			Params:     []Parameter{{Symbol: 0, Name: "x", Type: 0}},
			ReturnType: 0,
		}},
	}

	source, ok := snapshot.Source(0)
	if !ok || source.Path != "/main.ts" {
		t.Fatalf("source = %+v, %v", source, ok)
	}
	typeInfo, ok := snapshot.Type(0)
	if !ok || typeInfo.Kind != TypeNumber {
		t.Fatalf("type = %+v, %v", typeInfo, ok)
	}
}
