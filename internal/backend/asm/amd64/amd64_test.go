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

	// IMUL RAX, RDX -> 48 0f af c2
	e = NewEmitter()
	e.ImulRegReg(RAX, RDX)
	if !bytes.Equal(e.Code, []byte{0x48, 0x0F, 0xAF, 0xC2}) {
		t.Errorf("Imul RAX, RDX: got %x, want 480fafc2", e.Code)
	}

	// XOR RAX, RAX -> 48 31 c0
	e = NewEmitter()
	e.XorRegReg(RAX, RAX)
	if !bytes.Equal(e.Code, []byte{0x48, 0x31, 0xC0}) {
		t.Errorf("Xor RAX, RAX: got %x, want 4831c0", e.Code)
	}

	// LEA RAX, [RIP + 0] -> 48 8d 05 00 00 00 00
	e = NewEmitter()
	e.LeaRipRel32(RAX, 0)
	if !bytes.Equal(e.Code, []byte{0x48, 0x8D, 0x05, 0x00, 0x00, 0x00, 0x00}) {
		t.Errorf("LeaRipRel32 RAX, 0: got %x, want 488d0500000000", e.Code)
	}
}
