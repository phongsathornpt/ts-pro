package gc

import (
	"sync"
	"unsafe"
)

// Header is stored before each heap object.
type Header struct {
	Size   uint32
	Marked bool
	TypeID uint16
}

// Allocator manages heap chunks and mark-sweep allocation.
type Allocator struct {
	mu     sync.Mutex
	arena  []byte
	cursor uintptr
	limit  uintptr
	roots  []unsafe.Pointer
}

var DefaultAllocator = NewAllocator(1024 * 1024) // 1MB initial arena

func NewAllocator(size uintptr) *Allocator {
	return &Allocator{
		arena: make([]byte, size),
		limit: size,
	}
}

// Alloc allocates size bytes on the heap, returning a pointer.
func (a *Allocator) Alloc(size uintptr) unsafe.Pointer {
	a.mu.Lock()
	defer a.mu.Unlock()

	headerSize := unsafe.Sizeof(Header{})
	total := headerSize + size
	// Align to 8 bytes
	total = (total + 7) &^ 7

	if a.cursor+total > a.limit {
		// Collect or expand
		a.collectLocked()
		if a.cursor+total > a.limit {
			// Expand arena
			newArena := make([]byte, len(a.arena)*2+int(total))
			copy(newArena, a.arena)
			a.arena = newArena
			a.limit = uintptr(len(newArena))
		}
	}

	ptr := unsafe.Pointer(&a.arena[a.cursor])
	hdr := (*Header)(ptr)
	hdr.Size = uint32(size)
	hdr.Marked = false

	a.cursor += total
	return unsafe.Pointer(uintptr(ptr) + headerSize)
}

func (a *Allocator) collectLocked() {
	// Simple bump reset if no active roots, or mark-compact in advanced mode
	if len(a.roots) == 0 {
		a.cursor = 0
	}
}

// AddRoot registers a GC root.
func (a *Allocator) AddRoot(p unsafe.Pointer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.roots = append(a.roots, p)
}

// ClearRoots clears all registered GC roots.
func (a *Allocator) ClearRoots() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.roots = a.roots[:0]
}
