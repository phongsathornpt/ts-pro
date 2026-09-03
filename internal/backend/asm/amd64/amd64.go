package amd64

import (
	"encoding/binary"
)

// Register represents a 64-bit AMD64 integer register.
type Register uint8

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

// rex returns a REX prefix byte. W=1 (64-bit), R=reg_high, X=index_high, B=rm_high.
func rex(w, r, x, b bool) byte {
	var res byte = 0x40
	if w {
		res |= 0x08
	}
	if r {
		res |= 0x04
	}
	if x {
		res |= 0x02
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

// CmpRegReg: CMP reg1, reg2 (64-bit)
func (e *Emitter) CmpRegReg(r1, r2 Register) {
	e.emitByte(rex(true, r2 >= 8, false, r1 >= 8))
	e.emitByte(0x39)
	e.emitByte(modRM(0b11, r2, r1))
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

// Syscall: SYSCALL
func (e *Emitter) Syscall() {
	e.emitBytes(0x0F, 0x05)
}
