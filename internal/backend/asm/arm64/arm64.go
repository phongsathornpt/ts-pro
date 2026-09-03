package arm64

import (
	"encoding/binary"
)

// Register represents an ARM64 64-bit general-purpose register.
type Register uint8

const (
	X0  Register = 0
	X1  Register = 1
	X2  Register = 2
	X3  Register = 3
	X4  Register = 4
	X5  Register = 5
	X6  Register = 6
	X7  Register = 7
	X8  Register = 8
	X9  Register = 9
	X10 Register = 10
	X11 Register = 11
	X12 Register = 12
	X13 Register = 13
	X14 Register = 14
	X15 Register = 15
	X16 Register = 16
	X17 Register = 17
	X18 Register = 18
	X19 Register = 19
	X20 Register = 20
	X21 Register = 21
	X22 Register = 22
	X23 Register = 23
	X24 Register = 24
	X25 Register = 25
	X26 Register = 26
	X27 Register = 27
	X28 Register = 28
	X29 Register = 29 // FP
	X30 Register = 30 // LR
	SP  Register = 31 // SP / XZR
)

// Cond represents condition codes for B.cond.
type Cond uint8

const (
	CondEQ Cond = 0x0 // Equal
	CondNE Cond = 0x1 // Not Equal
	CondGE Cond = 0xA // Signed Greater or Equal
	CondLT Cond = 0xB // Signed Less
	CondGT Cond = 0xC // Signed Greater
	CondLE Cond = 0xD // Signed Less or Equal
)

type Emitter struct {
	Code []byte
}

func NewEmitter() *Emitter {
	return &Emitter{Code: make([]byte, 0, 256)}
}

func (e *Emitter) emitU32(word uint32) {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], word)
	e.Code = append(e.Code, buf[:]...)
}

// Add: ADD Xd, Xn, Xm (64-bit)
func (e *Emitter) Add(xd, xn, xm Register) {
	e.emitU32(0x8B000000 | (uint32(xm) << 16) | (uint32(xn) << 5) | uint32(xd))
}

// Sub: SUB Xd, Xn, Xm (64-bit)
func (e *Emitter) Sub(xd, xn, xm Register) {
	e.emitU32(0xCB000000 | (uint32(xm) << 16) | (uint32(xn) << 5) | uint32(xd))
}

// MovReg: MOV Xd, Xm (ORR Xd, XZR, Xm)
func (e *Emitter) MovReg(xd, xm Register) {
	e.emitU32(0xAA0003E0 | (uint32(xm) << 16) | uint32(xd))
}

// Movz: MOVZ Xd, #imm16 (LSL #0)
func (e *Emitter) Movz(xd Register, imm16 uint16) {
	e.emitU32(0xD2800000 | (uint32(imm16) << 5) | uint32(xd))
}

// Cmp: CMP Xn, Xm (SUBS XZR, Xn, Xm)
func (e *Emitter) Cmp(xn, xm Register) {
	e.emitU32(0xEB00001F | (uint32(xm) << 16) | (uint32(xn) << 5))
}

// B: B label (26-bit signed offset in words)
func (e *Emitter) B(wordOffset int32) {
	e.emitU32(0x14000000 | (uint32(wordOffset) & 0x03FFFFFF))
}

// BCond: B.cond label (19-bit signed offset in words)
func (e *Emitter) BCond(cond Cond, wordOffset int32) {
	e.emitU32(0x54000000 | ((uint32(wordOffset) & 0x7FFFF) << 5) | uint32(cond))
}

// Bl: BL label (26-bit signed offset in words)
func (e *Emitter) Bl(wordOffset int32) {
	e.emitU32(0x94000000 | (uint32(wordOffset) & 0x03FFFFFF))
}

// Ret: RET X30
func (e *Emitter) Ret() {
	e.emitU32(0xD65F03C0)
}

// Svc: SVC #0 (Syscall)
func (e *Emitter) Svc(imm16 uint16) {
	e.emitU32(0xD4000001 | (uint32(imm16) << 5))
}
