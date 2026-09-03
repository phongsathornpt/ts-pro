package arm64

import (
	"bytes"
	"testing"
)

func TestARM64Encodings(t *testing.T) {
	e := NewEmitter()
	// RET -> c0 03 5f d6
	e.Ret()
	if !bytes.Equal(e.Code, []byte{0xC0, 0x03, 0x5F, 0xD6}) {
		t.Errorf("Ret encoding: got %x, want c0035fd6", e.Code)
	}

	// ADD X0, X1, X2 -> 20 00 02 8b
	e = NewEmitter()
	e.Add(X0, X1, X2)
	if !bytes.Equal(e.Code, []byte{0x20, 0x00, 0x02, 0x8B}) {
		t.Errorf("Add X0, X1, X2: got %x, want 2000028b", e.Code)
	}

	// MOV X0, X1 (ORR X0, XZR, X1) -> e0 03 01 aa
	e = NewEmitter()
	e.MovReg(X0, X1)
	if !bytes.Equal(e.Code, []byte{0xE0, 0x03, 0x01, 0xAA}) {
		t.Errorf("Mov X0, X1: got %x, want e00301aa", e.Code)
	}
}
