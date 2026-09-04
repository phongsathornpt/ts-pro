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

	// MUL X0, X1, X2 -> 20 7c 02 9b
	e = NewEmitter()
	e.Mul(X0, X1, X2)
	if !bytes.Equal(e.Code, []byte{0x20, 0x7C, 0x02, 0x9B}) {
		t.Errorf("Mul X0, X1, X2: got %x, want 207c029b", e.Code)
	}

	// SDIV X0, X1, X2 -> 20 0c c2 9a
	e = NewEmitter()
	e.Sdiv(X0, X1, X2)
	if !bytes.Equal(e.Code, []byte{0x20, 0x0C, 0xC2, 0x9A}) {
		t.Errorf("Sdiv X0, X1, X2: got %x, want 200cc29a", e.Code)
	}

	// ADR X0, #0 -> 00 00 00 10
	e = NewEmitter()
	e.Adr(X0, 0)
	if !bytes.Equal(e.Code, []byte{0x00, 0x00, 0x00, 0x10}) {
		t.Errorf("Adr X0, 0: got %x, want 00000010", e.Code)
	}

	// Additional instructions for coverage
	e = NewEmitter()
	e.Sub(X0, X1, X2)
	e.AddImm(X0, X1, 16)
	e.SubImm(X0, X1, 16)
	e.Cmp(X0, X1)
	e.Bic(X0, X1, X2)
	e.Cset(X0, CondEQ)
	e.And(X0, X1, X2)
	e.Orr(X0, X1, X2)
	e.Eor(X0, X1, X2)
	e.Stp(X29, X30, SP, -16)
	e.Ldp(X29, X30, SP, 16)
	e.Str(X0, SP, 8)
	e.Ldr(X0, SP, 8)
	e.Strb(X0, SP, 1)
	e.Ldrb(X0, SP, 1)
	e.Movz(X0, 42)
	e.B(0)
	e.Bl(0)
	e.BCond(CondNE, 0)
	e.Cbz(X0, 0)
	e.Cbnz(X0, 0)
	e.Svc(0x80)

	if len(e.Code) != 22*4 {
		t.Errorf("expected 22 instructions (88 bytes), got %d bytes", len(e.Code))
	}
}
