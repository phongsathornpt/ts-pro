package amd64

import (
	"encoding/binary"
)

// Register represents a 64-bit AMD64 integer register.
type Register uint8

// XMMRegister represents an SSE register whose low lane carries binary64 values.
type XMMRegister uint8

const (
	XMM0 XMMRegister = iota
	XMM1
	XMM2
	XMM3
	XMM4
	XMM5
	XMM6
	XMM7
	XMM8
	XMM9
	XMM10
	XMM11
	XMM12
	XMM13
	XMM14
	XMM15
)

const (
	RAX Register = 0
	RCX Register = 1
	RDX Register = 2
	RBX Register = 3
	RSP Register = 4
	RBP Register = 5
	RSI Register = 6
	RDI Register = 7
	R8  Register = 8
	R9  Register = 9
	R10 Register = 10
	R11 Register = 11
	R12 Register = 12
	R13 Register = 13
	R14 Register = 14
	R15 Register = 15
)

// Cond represents condition codes for Jcc instructions.
type Cond uint8

const (
	CondE  Cond = 0x4 // Equal (ZF=1)
	CondNE Cond = 0x5 // Not Equal (ZF=0)
	CondL  Cond = 0xC // Less
	CondLE Cond = 0xE // Less or Equal
	CondG  Cond = 0xF // Greater
	CondGE Cond = 0xD // Greater or Equal
	CondB  Cond = 0x2
	CondAE Cond = 0x3
	CondBE Cond = 0x6
	CondA  Cond = 0x7
	CondP  Cond = 0xA
	CondNP Cond = 0xB
)

// Emitter emits x86-64 machine code bytes.
type Emitter struct {
	Code []byte
}

func NewEmitter() *Emitter {
	return &Emitter{Code: make([]byte, 0, 256)}
}

func (e *Emitter) emitByte(b byte) {
	e.Code = append(e.Code, b)
}

func (e *Emitter) emitBytes(b ...byte) {
	e.Code = append(e.Code, b...)
}

func (e *Emitter) emitInt32(v int32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], uint32(v))
	e.Code = append(e.Code, buf[:]...)
}

func (e *Emitter) emitInt64(v int64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(v))
	e.Code = append(e.Code, buf[:]...)
}

// rex returns a REX prefix byte. W=1 (64-bit), R=reg_high, B=rm_high.
func rex(w, r, _, b bool) byte {
	var res byte = 0x40
	if w {
		res |= 0x08
	}
	if r {
		res |= 0x04
	}
	if b {
		res |= 0x01
	}
	return res
}

// modRM builds ModR/M byte: mod(2) | reg(3) | rm(3)
func modRM(mod byte, reg Register, rm Register) byte {
	return ((mod & 0x03) << 6) | ((byte(reg) & 0x07) << 3) | (byte(rm) & 0x07)
}

func sib(scale byte, index Register, base Register) byte {
	return ((scale & 0x03) << 6) | ((byte(index) & 0x07) << 3) | (byte(base) & 0x07)
}

func (e *Emitter) emitBaseDisp(regField Register, base Register, disp int32) {
	mod := byte(0b10)
	if disp >= -128 && disp <= 127 {
		mod = 0b01
	}

	if base&7 == RSP {
		e.emitByte(modRM(mod, regField, RSP))
		e.emitByte(sib(0, RSP, base)) // no index, base=RSP/R12
	} else {
		e.emitByte(modRM(mod, regField, base))
	}

	if mod == 0b01 {
		e.emitByte(byte(disp))
	} else {
		e.emitInt32(disp)
	}
}

// MovRegReg: MOV dst, src (64-bit)
func (e *Emitter) MovRegReg(dst, src Register) {
	e.emitByte(rex(true, src >= 8, false, dst >= 8))
	e.emitByte(0x89)
	e.emitByte(modRM(0b11, src, dst))
}

// MovRegImm64: MOV dst, imm64
func (e *Emitter) MovRegImm64(dst Register, imm int64) {
	e.emitByte(rex(true, false, false, dst >= 8))
	e.emitByte(0xB8 + (byte(dst) & 0x07))
	e.emitInt64(imm)
}

// AddRegReg: ADD dst, src (64-bit)
func (e *Emitter) AddRegReg(dst, src Register) {
	e.emitByte(rex(true, src >= 8, false, dst >= 8))
	e.emitByte(0x01)
	e.emitByte(modRM(0b11, src, dst))
}

// SubRegReg: SUB dst, src (64-bit)
func (e *Emitter) SubRegReg(dst, src Register) {
	e.emitByte(rex(true, src >= 8, false, dst >= 8))
	e.emitByte(0x29)
	e.emitByte(modRM(0b11, src, dst))
}

// SubRegImm32: SUB dst, imm32 (64-bit)
func (e *Emitter) SubRegImm32(dst Register, imm int32) {
	e.emitByte(rex(true, false, false, dst >= 8))
	e.emitByte(0x81)
	e.emitByte(modRM(0b11, 5, dst))
	e.emitInt32(imm)
}

// AddRegImm32: ADD dst, imm32 (64-bit)
func (e *Emitter) AddRegImm32(dst Register, imm int32) {
	e.emitByte(rex(true, false, false, dst >= 8))
	e.emitByte(0x81)
	e.emitByte(modRM(0b11, 0, dst))
	e.emitInt32(imm)
}

// CmpRegReg: CMP reg1, reg2 (64-bit)
func (e *Emitter) CmpRegReg(r1, r2 Register) {
	e.emitByte(rex(true, r2 >= 8, false, r1 >= 8))
	e.emitByte(0x39)
	e.emitByte(modRM(0b11, r2, r1))
}

// CmpRegImm32: CMP reg, imm32 (64-bit, sign-extended immediate).
func (e *Emitter) CmpRegImm32(reg Register, imm int32) {
	e.emitByte(rex(true, false, false, reg >= 8))
	e.emitByte(0x81)
	e.emitByte(modRM(0b11, 7, reg))
	e.emitInt32(imm)
}

// TestRegReg: TEST lhs, rhs (64-bit).
func (e *Emitter) TestRegReg(lhs, rhs Register) {
	e.emitByte(rex(true, rhs >= 8, false, lhs >= 8))
	e.emitByte(0x85)
	e.emitByte(modRM(0b11, rhs, lhs))
}

// JmpRel32: JMP rel32
func (e *Emitter) JmpRel32(rel int32) {
	e.emitByte(0xE9)
	e.emitInt32(rel)
}

// JccRel32: Jcc rel32
func (e *Emitter) JccRel32(cond Cond, rel int32) {
	e.emitByte(0x0F)
	e.emitByte(0x80 | byte(cond))
	e.emitInt32(rel)
}

// CallRel32: CALL rel32
func (e *Emitter) CallRel32(rel int32) {
	e.emitByte(0xE8)
	e.emitInt32(rel)
}

// CallReg emits CALL r64.
func (e *Emitter) CallReg(reg Register) {
	if reg >= 8 {
		e.emitByte(0x41)
	}
	e.emitByte(0xFF)
	e.emitByte(modRM(0b11, 2, reg))
}

// Ret: RET
func (e *Emitter) Ret() {
	e.emitByte(0xC3)
}

// Push: PUSH reg
func (e *Emitter) Push(reg Register) {
	if reg >= 8 {
		e.emitByte(rex(false, false, false, true))
	}
	e.emitByte(0x50 + (byte(reg) & 0x07))
}

// Pop: POP reg
func (e *Emitter) Pop(reg Register) {
	if reg >= 8 {
		e.emitByte(rex(false, false, false, true))
	}
	e.emitByte(0x58 + (byte(reg) & 0x07))
}

// ImulRegReg: IMUL dst, src (64-bit signed multiply)
func (e *Emitter) ImulRegReg(dst, src Register) {
	e.emitByte(rex(true, dst >= 8, false, src >= 8))
	e.emitBytes(0x0F, 0xAF)
	e.emitByte(modRM(0b11, dst, src))
}

// XorRegReg: XOR dst, src (64-bit)
func (e *Emitter) XorRegReg(dst, src Register) {
	e.emitByte(rex(true, src >= 8, false, dst >= 8))
	e.emitByte(0x31)
	e.emitByte(modRM(0b11, src, dst))
}

// AndRegReg: AND dst, src (64-bit)
func (e *Emitter) AndRegReg(dst, src Register) {
	e.emitByte(rex(true, src >= 8, false, dst >= 8))
	e.emitByte(0x21)
	e.emitByte(modRM(0b11, src, dst))
}

// OrRegReg: OR dst, src (64-bit)
func (e *Emitter) OrRegReg(dst, src Register) {
	e.emitByte(rex(true, src >= 8, false, dst >= 8))
	e.emitByte(0x09)
	e.emitByte(modRM(0b11, src, dst))
}

func (e *Emitter) emitBitRegReg(opcode byte, base, bit Register) {
	e.emitByte(rex(true, bit >= 8, false, base >= 8))
	e.emitBytes(0x0F, opcode, modRM(0x3, bit, base))
}

// BtRegReg tests bit index in base and stores the old bit in CF.
func (e *Emitter) BtRegReg(base, bit Register) { e.emitBitRegReg(0xA3, base, bit) }

// BtsRegReg tests and sets bit index in base, storing the old bit in CF.
func (e *Emitter) BtsRegReg(base, bit Register) { e.emitBitRegReg(0xAB, base, bit) }

// BtrRegReg tests and resets bit index in base, storing the old bit in CF.
func (e *Emitter) BtrRegReg(base, bit Register) { e.emitBitRegReg(0xB3, base, bit) }

// Setcc sets dst to a canonical 0/1 value. SETcc writes only the low byte,
// so MOVZX is emitted immediately afterwards to clear the upper bits.
func (e *Emitter) Setcc(cond Cond, dst Register) {
	if dst >= 4 {
		// REX prefix required to access SIL/DIL/BPL/SPL or R8B-R15B.
		e.emitByte(rex(false, false, false, dst >= 8))
	}
	e.emitBytes(0x0F, 0x90|byte(cond))
	e.emitByte(modRM(0b11, 0, dst))

	e.emitByte(rex(true, dst >= 8, false, dst >= 8))
	e.emitBytes(0x0F, 0xB6)
	e.emitByte(modRM(0b11, dst, dst))
}

// NegReg: NEG reg (64-bit).
func (e *Emitter) NegReg(reg Register) {
	e.emitByte(rex(true, false, false, reg >= 8))
	e.emitByte(0xF7)
	e.emitByte(modRM(0b11, 3, reg))
}

// MovDerefReg8: MOV byte ptr [base + disp], src8.
func (e *Emitter) MovDerefReg8(base Register, disp int32, src Register) {
	if src >= 4 || base >= 8 {
		e.emitByte(rex(false, src >= 8, false, base >= 8))
	}
	e.emitByte(0x88)
	e.emitBaseDisp(src, base, disp)
}

// MovzxRegDeref8: MOVZX dst64, byte ptr [base + disp].
func (e *Emitter) MovzxRegDeref8(dst Register, base Register, disp int32) {
	e.emitByte(rex(true, dst >= 8, false, base >= 8))
	e.emitBytes(0x0F, 0xB6)
	e.emitBaseDisp(dst, base, disp)
}

// MovDerefReg: MOV [base + disp], src (64-bit).
func (e *Emitter) MovDerefReg(base Register, disp int32, src Register) {
	e.emitByte(rex(true, src >= 8, false, base >= 8))
	e.emitByte(0x89)
	e.emitBaseDisp(src, base, disp)
}

// MovRegDeref: MOV dst, [base + disp] (64-bit).
func (e *Emitter) MovRegDeref(dst Register, base Register, disp int32) {
	e.emitByte(rex(true, dst >= 8, false, base >= 8))
	e.emitByte(0x8B)
	e.emitBaseDisp(dst, base, disp)
}

// Cqo sign-extends RAX into RDX:RAX for signed division.
func (e *Emitter) Cqo() {
	e.emitBytes(0x48, 0x99)
}

// IdivReg divides signed RDX:RAX by src. Quotient is returned in RAX and remainder in RDX.
func (e *Emitter) IdivReg(src Register) {
	e.emitByte(rex(true, false, false, src >= 8))
	e.emitByte(0xF7)
	e.emitByte(modRM(0b11, 7, src))
}

// MovQXMMReg moves a raw 64-bit GPR payload into an XMM low lane.
func (e *Emitter) MovQXMMReg(dst XMMRegister, src Register) {
	e.emitByte(0x66)
	e.emitByte(rex(true, dst >= 8, false, src >= 8))
	e.emitBytes(0x0F, 0x6E)
	e.emitByte((0b11 << 6) | ((byte(dst) & 7) << 3) | (byte(src) & 7))
}

// MovQRegXMM moves an XMM low lane into a GPR without conversion.
func (e *Emitter) MovQRegXMM(dst Register, src XMMRegister) {
	e.emitByte(0x66)
	e.emitByte(rex(true, src >= 8, false, dst >= 8))
	e.emitBytes(0x0F, 0x7E)
	e.emitByte((0b11 << 6) | ((byte(src) & 7) << 3) | (byte(dst) & 7))
}

func (e *Emitter) emitSSE2Binary(prefix, opcode byte, dst, src XMMRegister) {
	e.emitByte(prefix)
	if dst >= 8 || src >= 8 {
		e.emitByte(rex(false, dst >= 8, false, src >= 8))
	}
	e.emitBytes(0x0F, opcode)
	e.emitByte((0b11 << 6) | ((byte(dst) & 7) << 3) | (byte(src) & 7))
}

func (e *Emitter) MovSDRegReg(dst, src XMMRegister) { e.emitSSE2Binary(0xF2, 0x10, dst, src) }
func (e *Emitter) AddSD(dst, src XMMRegister)       { e.emitSSE2Binary(0xF2, 0x58, dst, src) }
func (e *Emitter) SubSD(dst, src XMMRegister)       { e.emitSSE2Binary(0xF2, 0x5C, dst, src) }
func (e *Emitter) MulSD(dst, src XMMRegister)       { e.emitSSE2Binary(0xF2, 0x59, dst, src) }
func (e *Emitter) DivSD(dst, src XMMRegister)       { e.emitSSE2Binary(0xF2, 0x5E, dst, src) }
func (e *Emitter) XorPD(dst, src XMMRegister)       { e.emitSSE2Binary(0x66, 0x57, dst, src) }
func (e *Emitter) Ucomisd(lhs, rhs XMMRegister)     { e.emitSSE2Binary(0x66, 0x2E, lhs, rhs) }

func (e *Emitter) Cvttsd2si(dst Register, src XMMRegister) {
	e.emitByte(0xF2)
	e.emitByte(rex(true, dst >= 8, false, src >= 8))
	e.emitBytes(0x0F, 0x2C)
	e.emitByte((0b11 << 6) | ((byte(dst) & 7) << 3) | (byte(src) & 7))
}

func (e *Emitter) Cvtsi2sd(dst XMMRegister, src Register) {
	e.emitByte(0xF2)
	e.emitByte(rex(true, dst >= 8, false, src >= 8))
	e.emitBytes(0x0F, 0x2A)
	e.emitByte((0b11 << 6) | ((byte(dst) & 7) << 3) | (byte(src) & 7))
}

func (e *Emitter) ShrRegImm8(reg Register, imm byte) {
	e.emitByte(rex(true, false, false, reg >= 8))
	e.emitByte(0xC1)
	e.emitByte(modRM(0b11, 5, reg))
	e.emitByte(imm)
}

// Syscall: SYSCALL
func (e *Emitter) Syscall() {
	e.emitBytes(0x0F, 0x05)
}

// LeaRipRel32: LEA dst, [RIP + disp32] (64-bit)
func (e *Emitter) LeaRipRel32(dst Register, disp int32) {
	e.emitByte(rex(true, dst >= 8, false, false))
	e.emitByte(0x8D)
	e.emitByte(modRM(0b00, dst, 5))
	e.emitInt32(disp)
}

// FildDeref64 loads a signed 64-bit integer from memory into x87 ST(0).
func (e *Emitter) FildDeref64(base Register, disp int32) {
	if base >= 8 {
		e.emitByte(rex(false, false, false, true))
	}
	e.emitByte(0xDF)
	e.emitBaseDisp(5, base, disp)
}

// FstpDeref64 stores x87 ST(0) as binary64 and pops the x87 stack.
func (e *Emitter) FstpDeref64(base Register, disp int32) {
	if base >= 8 {
		e.emitByte(rex(false, false, false, true))
	}
	e.emitByte(0xDD)
	e.emitBaseDisp(3, base, disp)
}

// FmulDeref64 multiplies x87 ST(0) by a binary64 memory operand.
func (e *Emitter) FmulDeref64(base Register, disp int32) {
	if base >= 8 {
		e.emitByte(rex(false, false, false, true))
	}
	e.emitByte(0xDC)
	e.emitBaseDisp(1, base, disp)
}

// FdivDeref64 divides x87 ST(0) by a binary64 memory operand.
func (e *Emitter) FdivDeref64(base Register, disp int32) {
	if base >= 8 {
		e.emitByte(rex(false, false, false, true))
	}
	e.emitByte(0xDC)
	e.emitBaseDisp(6, base, disp)
}

func (e *Emitter) FldDeref64(base Register, disp int32) {
	if base >= 8 {
		e.emitByte(rex(false, false, false, true))
	}
	e.emitByte(0xDD)
	e.emitBaseDisp(0, base, disp)
}
func (e *Emitter) FsubDeref64(base Register, disp int32) {
	if base >= 8 {
		e.emitByte(rex(false, false, false, true))
	}
	e.emitByte(0xDC)
	e.emitBaseDisp(4, base, disp)
}
func (e *Emitter) Fabs()   { e.emitBytes(0xD9, 0xE1) }
func (e *Emitter) FldST0() { e.emitBytes(0xD9, 0xC0) }
func (e *Emitter) FstpDeref80(base Register, disp int32) {
	if base >= 8 {
		e.emitByte(rex(false, false, false, true))
	}
	e.emitByte(0xDB)
	e.emitBaseDisp(7, base, disp)
}
func (e *Emitter) FldDeref80(base Register, disp int32) {
	if base >= 8 {
		e.emitByte(rex(false, false, false, true))
	}
	e.emitByte(0xDB)
	e.emitBaseDisp(5, base, disp)
}
func (e *Emitter) FcomipST1() { e.emitBytes(0xDF, 0xF1) }
