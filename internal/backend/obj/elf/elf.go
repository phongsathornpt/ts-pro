package elf

import (
	"bytes"
	"encoding/binary"
)

// Linux ELF64 constants
const (
	EI_CLASS   = 4
	EI_DATA    = 5
	EI_VERSION = 6
	EI_OSABI   = 7

	ELFCLASS64    = 2
	ELFDATA2LSB   = 1
	EV_CURRENT    = 1
	ELFOSABI_NONE = 0

	ET_EXEC = 2

	EM_X86_64  = 62
	EM_AARCH64 = 183

	PT_LOAD = 1
	PF_X    = 1
	PF_R    = 4
)

// Header64 represents the ELF64 file header.
type Header64 struct {
	Ident     [16]byte
	Type      uint16
	Machine   uint16
	Version   uint32
	Entry     uint64
	Phoff     uint64
	Shoff     uint64
	Flags     uint32
	Ehsize    uint16
	Phentsize uint16
	Phnum     uint16
	Shentsize uint16
	Shnum     uint16
	Shstrndx  uint16
}

// ProgHeader64 represents an ELF64 program header (segment).
type ProgHeader64 struct {
	Type   uint32
	Flags  uint32
	Off    uint64
	Vaddr  uint64
	Paddr  uint64
	Filesz uint64
	Memsz  uint64
	Align  uint64
}

// CreateExecutable generates a minimal standalone Linux ELF64 executable from raw machine code.
func CreateExecutable(code []byte, isARM64 bool) ([]byte, error) {
	buf := new(bytes.Buffer)

	machine := uint16(EM_X86_64)
	if isARM64 {
		machine = EM_AARCH64
	}

	baseAddr := uint64(0x400000)
	headerSize := uint64(64)
	phSize := uint64(56)
	entryOffset := headerSize + phSize
	entryAddr := baseAddr + entryOffset

	hdr := Header64{
		Type:      ET_EXEC,
		Machine:   machine,
		Version:   EV_CURRENT,
		Entry:     entryAddr,
		Phoff:     headerSize,
		Ehsize:    uint16(headerSize),
		Phentsize: uint16(phSize),
		Phnum:     1,
	}
	copy(hdr.Ident[:4], []byte{0x7f, 'E', 'L', 'F'})
	hdr.Ident[EI_CLASS] = ELFCLASS64
	hdr.Ident[EI_DATA] = ELFDATA2LSB
	hdr.Ident[EI_VERSION] = EV_CURRENT
	hdr.Ident[EI_OSABI] = ELFOSABI_NONE

	fileSize := entryOffset + uint64(len(code))

	ph := ProgHeader64{
		Type:   PT_LOAD,
		Flags:  PF_R | PF_X,
		Off:    0,
		Vaddr:  baseAddr,
		Paddr:  baseAddr,
		Filesz: fileSize,
		Memsz:  fileSize,
		Align:  0x1000,
	}

	_ = binary.Write(buf, binary.LittleEndian, hdr)
	_ = binary.Write(buf, binary.LittleEndian, ph)
	buf.Write(code)

	return buf.Bytes(), nil
}
