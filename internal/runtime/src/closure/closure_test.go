package closure

import (
	"testing"
)

func TestClosure(t *testing.T) {
	c := New(0xdeadbeef, []uint64{10, 20, 30})
	if c.FnPtr != 0xdeadbeef {
		t.Errorf("FnPtr = %x, want 0xdeadbeef", c.FnPtr)
	}
	if c.GetUpvalue(1) != 20 {
		t.Errorf("GetUpvalue(1) = %d, want 20", c.GetUpvalue(1))
	}
	c.SetUpvalue(1, 99)
	if c.GetUpvalue(1) != 99 {
		t.Errorf("GetUpvalue(1) after update = %d, want 99", c.GetUpvalue(1))
	}
}
