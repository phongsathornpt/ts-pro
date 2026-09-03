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
	SP  Register = 31 // SP
	XZR Register = 31 // XZR
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

// AddImm: ADD Xd, Xn, #imm12
func (e *Emitter) AddImm(xd, xn Register, imm12 uint16) {
	e.emitU32(0x91000000 | (uint32(imm12&0xFFF) << 10) | (uint32(xn) << 5) | uint32(xd))
}

// SubImm: SUB Xd, Xn, #imm12
func (e *Emitter) SubImm(xd, xn Register, imm12 uint16) {
	e.emitU32(0xD1000000 | (uint32(imm12&0xFFF) << 10) | (uint32(xn) << 5) | uint32(xd))
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

// Mul: MUL Xd, Xn, Xm (64-bit multiply)
func (e *Emitter) Mul(xd, xn, xm Register) {
	e.emitU32(0x9B007C00 | (uint32(xm) << 16) | (uint32(xn) << 5) | uint32(xd))
}

// Sdiv: SDIV Xd, Xn, Xm (64-bit signed divide)
func (e *Emitter) Sdiv(xd, xn, xm Register) {
	e.emitU32(0x9AC00C00 | (uint32(xm) << 16) | (uint32(xn) << 5) | uint32(xd))
}

// Bic: BIC Xd, Xn, Xm (64-bit bit clear: Xd = Xn & ~Xm)
func (e *Emitter) Bic(xd, xn, xm Register) {
	e.emitU32(0x8A200000 | (uint32(xm) << 16) | (uint32(xn) << 5) | uint32(xd))
}

// Cset: CSET Xd, cond (CSINC Xd, XZR, XZR, invert(cond))
func (e *Emitter) Cset(xd Register, cond Cond) {
	invCond := uint32(cond ^ 1)
	e.emitU32(0x9A9F07E0 | (invCond << 12) | uint32(xd))
}

// And: AND Xd, Xn, Xm
func (e *Emitter) And(xd, xn, xm Register) {
	e.emitU32(0x8A000000 | (uint32(xm) << 16) | (uint32(xn) << 5) | uint32(xd))
}

// Orr: ORR Xd, Xn, Xm
func (e *Emitter) Orr(xd, xn, xm Register) {
	e.emitU32(0xAA000000 | (uint32(xm) << 16) | (uint32(xn) << 5) | uint32(xd))
}

// Eor: EOR Xd, Xn, Xm
func (e *Emitter) Eor(xd, xn, xm Register) {
	e.emitU32(0xCA000000 | (uint32(xm) << 16) | (uint32(xn) << 5) | uint32(xd))
}

// Stp: STP Xt1, Xt2, [Xn, #imm7*8] (Store Pair)
func (e *Emitter) Stp(xt1, xt2, xn Register, immBytes int32) {
	imm7 := (uint32(immBytes/8) & 0x7F)
	e.emitU32(0xA9000000 | (imm7 << 15) | (uint32(xt2) << 10) | (uint32(xn) << 5) | uint32(xt1))
}

// Ldp: LDP Xt1, Xt2, [Xn, #imm7*8] (Load Pair)
func (e *Emitter) Ldp(xt1, xt2, xn Register, immBytes int32) {
	imm7 := (uint32(immBytes/8) & 0x7F)
	e.emitU32(0xA9400000 | (imm7 << 15) | (uint32(xt2) << 10) | (uint32(xn) << 5) | uint32(xt1))
}

// Str: STR Xt, [Xn, #imm12*8] (Store 64-bit register)
func (e *Emitter) Str(xt, xn Register, immBytes int32) {
	imm12 := (uint32(immBytes/8) & 0xFFF)
	e.emitU32(0xF9000000 | (imm12 << 10) | (uint32(xn) << 5) | uint32(xt))
}

// Ldr: LDR Xt, [Xn, #imm12*8] (Load 64-bit register)
func (e *Emitter) Ldr(xt, xn Register, immBytes int32) {
	imm12 := (uint32(immBytes/8) & 0xFFF)
	e.emitU32(0xF9400000 | (imm12 << 10) | (uint32(xn) << 5) | uint32(xt))
}

// Strb: STRB Wt, [Xn, #imm12] (Store byte)
func (e *Emitter) Strb(wt, xn Register, immBytes int32) {
	imm12 := (uint32(immBytes) & 0xFFF)
	e.emitU32(0x39000000 | (imm12 << 10) | (uint32(xn) << 5) | uint32(wt))
}

// Ldrb: LDRB Wt, [Xn, #imm12] (Load byte)
func (e *Emitter) Ldrb(wt, xn Register, immBytes int32) {
	imm12 := (uint32(immBytes) & 0xFFF)
	e.emitU32(0x39400000 | (imm12 << 10) | (uint32(xn) << 5) | uint32(wt))
}

// Cbz: CBZ Xt, label (19-bit signed word offset)
func (e *Emitter) Cbz(xt Register, wordOffset int32) {
	e.emitU32(0xB4000000 | ((uint32(wordOffset) & 0x7FFFF) << 5) | uint32(xt))
}

// Cbnz: CBNZ Xt, label (19-bit signed word offset)
func (e *Emitter) Cbnz(xt Register, wordOffset int32) {
	e.emitU32(0xB5000000 | ((uint32(wordOffset) & 0x7FFFF) << 5) | uint32(xt))
}

// Svc: SVC #0 (Syscall)
func (e *Emitter) Svc(imm16 uint16) {
	e.emitU32(0xD4000001 | (uint32(imm16) << 5))
}

// Adr: ADR Xd, byteOffset (+-1MB signed byte offset)
func (e *Emitter) Adr(xd Register, byteOffset int32) {
	imm21 := uint32(byteOffset) & 0x1FFFFF
	immlo := imm21 & 0x3
	immhi := (imm21 >> 2) & 0x7FFFF
	e.emitU32(0x10000000 | (immlo << 29) | (immhi << 5) | uint32(xd))
}
