package closure

import (
	"unsafe"
)

// TSClosure represents a function pointer combined with captured upvalue environment.
type TSClosure struct {
	FnPtr    uintptr
	Upvalues []uint64
}

// New creates a new closure.
func New(fnPtr uintptr, upvalues []uint64) *TSClosure {
	ups := make([]uint64, len(upvalues))
	copy(ups, upvalues)
	return &TSClosure{
		FnPtr:    fnPtr,
		Upvalues: ups,
	}
}

// GetUpvalue returns the upvalue at index i.
func (c *TSClosure) GetUpvalue(i int) uint64 {
	if i < 0 || i >= len(c.Upvalues) {
		return 0
	}
	return c.Upvalues[i]
}

// SetUpvalue updates the upvalue at index i.
func (c *TSClosure) SetUpvalue(i int, val uint64) {
	if i >= 0 && i < len(c.Upvalues) {
		c.Upvalues[i] = val
	}
}

// Pointer returns the raw pointer to the closure for pass-through to native code.
func (c *TSClosure) Pointer() unsafe.Pointer {
	return unsafe.Pointer(c)
}
