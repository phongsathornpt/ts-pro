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

func TestAMD64MemoryAndConditionEncodings(t *testing.T) {
	tests := []struct {
		name string
		emit func(*Emitter)
		want []byte
	}{
		{
			name: "mov [rsp+16], rax uses sib",
			emit: func(e *Emitter) { e.MovDerefReg(RSP, 16, RAX) },
			want: []byte{0x48, 0x89, 0x44, 0x24, 0x10},
		},
		{
			name: "mov rax, [rsp+16] uses sib",
			emit: func(e *Emitter) { e.MovRegDeref(RAX, RSP, 16) },
			want: []byte{0x48, 0x8B, 0x44, 0x24, 0x10},
		},
		{
			name: "mov [r12+16], rax uses sib and rex-b",
			emit: func(e *Emitter) { e.MovDerefReg(R12, 16, RAX) },
			want: []byte{0x49, 0x89, 0x44, 0x24, 0x10},
		},
		{
			name: "test r12 r12",
			emit: func(e *Emitter) { e.TestRegReg(R12, R12) },
			want: []byte{0x4D, 0x85, 0xE4},
		},
		{
			name: "setne r12 canonicalizes upper bits",
			emit: func(e *Emitter) { e.Setcc(CondNE, R12) },
			want: []byte{0x41, 0x0F, 0x95, 0xC4, 0x4D, 0x0F, 0xB6, 0xE4},
		},
		{
			name: "cqo idiv r12",
			emit: func(e *Emitter) { e.Cqo(); e.IdivReg(R12) },
			want: []byte{0x48, 0x99, 0x49, 0xF7, 0xFC},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEmitter()
			tc.emit(e)
			if !bytes.Equal(e.Code, tc.want) {
				t.Fatalf("encoding: got %x, want %x", e.Code, tc.want)
			}
		})
	}
}

func TestAMD64ByteMemoryEncodings(t *testing.T) {
	e := NewEmitter()
	e.MovDerefReg8(RSP, 16, RDX)
	if want := []byte{0x88, 0x54, 0x24, 0x10}; !bytes.Equal(e.Code, want) {
		t.Fatalf("mov byte [rsp+16], dl: got %x, want %x", e.Code, want)
	}

	e = NewEmitter()
	e.MovzxRegDeref8(R12, RSP, 16)
	if want := []byte{0x4C, 0x0F, 0xB6, 0x64, 0x24, 0x10}; !bytes.Equal(e.Code, want) {
		t.Fatalf("movzx r12, byte [rsp+16]: got %x, want %x", e.Code, want)
	}

	e = NewEmitter()
	e.NegReg(R12)
	if want := []byte{0x49, 0xF7, 0xDC}; !bytes.Equal(e.Code, want) {
		t.Fatalf("neg r12: got %x, want %x", e.Code, want)
	}
}
