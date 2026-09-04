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

	// Add root, test expansion
	alloc.AddRoot(p1)
	pBig := alloc.Alloc(2048)
	if pBig == nil {
		t.Fatalf("expected non-nil pointer after expansion")
	}

	alloc.ClearRoots()
	alloc.collectLocked()
	if alloc.cursor != 0 {
		t.Errorf("expected cursor reset after ClearRoots, got %d", alloc.cursor)
	}
}
