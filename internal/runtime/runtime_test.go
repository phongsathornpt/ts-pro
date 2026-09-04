package runtime

import (
	"testing"
)

func TestEmbeddedRuntime(t *testing.T) {
	if len(ProvidedSymbols) == 0 {
		t.Fatalf("expected provided symbols")
	}

	data, err := GetRuntimeSource("src/gc/gc.go")
	if err != nil {
		t.Fatalf("GetRuntimeSource(src/gc/gc.go) failed: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("expected non-empty embedded file")
	}

	if _, err := GetRuntimeSource("nonexistent.go"); err == nil {
		t.Errorf("expected error for nonexistent file")
	}
}
