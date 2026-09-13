package amd64

// RorRegImm8 rotates a 64-bit register right by an immediate count.
func (e *Emitter) RorRegImm8(reg Register, imm byte) {
	e.emitByte(rex(true, false, false, reg >= 8))
	e.emitByte(0xC1)
	e.emitByte(modRM(0b11, 1, reg))
	e.emitByte(imm)
}

// NotReg inverts all bits in a 64-bit register.
func (e *Emitter) NotReg(reg Register) {
	e.emitByte(rex(true, false, false, reg >= 8))
	e.emitByte(0xF7)
	e.emitByte(modRM(0b11, 2, reg))
}

// BswapReg reverses the byte order of a 64-bit register.
func (e *Emitter) BswapReg(reg Register) {
	e.emitByte(rex(true, false, false, reg >= 8))
	e.emitBytes(0x0F, 0xC8+(byte(reg)&0x07))
}
