package gc

import (
	"testing"
)

func TestAllocatorAlloc(t *testing.T) {
	alloc := NewAllocator(1024)
	p1 := alloc.Alloc(64)
	if p1 == nil {
		t.Fatalf("expected non-nil pointer")
	}

	p2 := alloc.Alloc(128)
	if p2 == nil {
		t.Fatalf("expected non-nil pointer")
	}

	if p1 == p2 {
		t.Errorf("expected distinct allocation pointers")
	}
}
