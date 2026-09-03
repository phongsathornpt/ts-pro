package amd64

import (
	"bytes"
	"testing"
)

func TestAMD64Encodings(t *testing.T) {
	e := NewEmitter()
	// RET -> 0xC3
	e.Ret()
	if !bytes.Equal(e.Code, []byte{0xC3}) {
		t.Errorf("Ret encoding: got %x, want c3", e.Code)
	}

	// PUSH RBP -> 0x55
	e = NewEmitter()
	e.Push(RBP)
	if !bytes.Equal(e.Code, []byte{0x55}) {
		t.Errorf("Push RBP encoding: got %x, want 55", e.Code)
	}

	// MOV RAX, RDI -> 48 89 f8
	e = NewEmitter()
	e.MovRegReg(RAX, RDI)
	if !bytes.Equal(e.Code, []byte{0x48, 0x89, 0xF8}) {
		t.Errorf("Mov RAX, RDI: got %x, want 4889f8", e.Code)
	}

	// ADD RAX, RDX -> 48 01 d0
	e = NewEmitter()
	e.AddRegReg(RAX, RDX)
	if !bytes.Equal(e.Code, []byte{0x48, 0x01, 0xD0}) {
		t.Errorf("Add RAX, RDX: got %x, want 4801d0", e.Code)
	}
}
