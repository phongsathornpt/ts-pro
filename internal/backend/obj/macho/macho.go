package macho

import (
	"bytes"
	"encoding/binary"
)

const (
	MH_MAGIC_64     = 0xfeedfacf
	CPU_TYPE_X86_64 = 0x01000007
	CPU_TYPE_ARM64  = 0x0100000c
	CPU_SUBTYPE_ALL = 0x00000003

	MH_EXECUTE = 0x2

	LC_SEGMENT_64 = 0x19
	LC_MAIN       = 0x80000028

	VM_PROT_NONE    = 0x0
	VM_PROT_READ    = 0x1
	VM_PROT_EXECUTE = 0x4
)

// Header64 represents the Mach-O 64-bit header.
type Header64 struct {
	Magic      uint32
	CpuType    uint32
	CpuSubtype uint32
	FileType   uint32
	NCmds      uint32
	SizeOfCmds uint32
	Flags      uint32
	Reserved   uint32
}

// SegmentCmd64 represents LC_SEGMENT_64.
type SegmentCmd64 struct {
	Cmd      uint32
	CmdSize  uint32
	SegName  [16]byte
	VmAddr   uint64
	VmSize   uint64
	FileOff  uint64
	FileSize uint64
	MaxProt  uint32
	InitProt uint32
	NSects   uint32
	Flags    uint32
}

// Section64 represents a Mach-O 64-bit section header.
type Section64 struct {
	SectName  [16]byte
	SegName   [16]byte
	Addr      uint64
	Size      uint64
	Offset    uint32
	Align     uint32
	RelOff    uint32
	NReloc    uint32
	Flags     uint32
	Reserved1 uint32
	Reserved2 uint32
	Reserved3 uint32
}

// EntryPointCmd represents LC_MAIN.
type EntryPointCmd struct {
	Cmd       uint32
	CmdSize   uint32
	EntryOff  uint64
	StackSize uint64
}

// CreateExecutable generates a standalone macOS Mach-O 64 executable from machine code.
func CreateExecutable(code []byte, isARM64 bool) ([]byte, error) {
	buf := new(bytes.Buffer)

	cpuType := uint32(CPU_TYPE_X86_64)
	if isARM64 {
		cpuType = CPU_TYPE_ARM64
	}

	headerSize := uint32(32)
	segPageZeroSize := uint32(72)
	segTextSize := uint32(72 + 80) // 1 section (__text)
	entryCmdSize := uint32(24)

	sizeOfCmds := segPageZeroSize + segTextSize + entryCmdSize
	codeOffset := uint64(headerSize + sizeOfCmds)
	// Align codeOffset to 16 bytes
	padding := int((16 - (codeOffset % 16)) % 16)
	codeOffset += uint64(padding)

	hdr := Header64{
		Magic:      MH_MAGIC_64,
		CpuType:    cpuType,
		CpuSubtype: CPU_SUBTYPE_ALL,
		FileType:   MH_EXECUTE,
		NCmds:      3,
		SizeOfCmds: sizeOfCmds,
		Flags:      0x00200085, // MH_NOUNDEFS | MH_DYLDLINK | MH_TWOLEVEL | MH_PIE
	}

	// __PAGEZERO segment
	var pageZero SegmentCmd64
	pageZero.Cmd = LC_SEGMENT_64
	pageZero.CmdSize = segPageZeroSize
	copy(pageZero.SegName[:], "__PAGEZERO")
	pageZero.VmSize = 0x100000000 // 4GB

	// __TEXT segment
	baseAddr := uint64(0x100000000)
	var textSeg SegmentCmd64
	textSeg.Cmd = LC_SEGMENT_64
	textSeg.CmdSize = segTextSize
	copy(textSeg.SegName[:], "__TEXT")
	textSeg.VmAddr = baseAddr
	textSeg.VmSize = 0x4000
	textSeg.FileOff = 0
	textSeg.FileSize = codeOffset + uint64(len(code))
	textSeg.MaxProt = VM_PROT_READ | VM_PROT_EXECUTE
	textSeg.InitProt = VM_PROT_READ | VM_PROT_EXECUTE
	textSeg.NSects = 1

	// __text section
	var textSect Section64
	copy(textSect.SectName[:], "__text")
	copy(textSect.SegName[:], "__TEXT")
	textSect.Addr = baseAddr + codeOffset
	textSect.Size = uint64(len(code))
	textSect.Offset = uint32(codeOffset)
	textSect.Align = 4
	textSect.Flags = 0x80000400 // S_ATTR_PURE_INSTRUCTIONS | S_ATTR_SOME_INSTRUCTIONS

	// LC_MAIN
	var entry EntryPointCmd
	entry.Cmd = LC_MAIN
	entry.CmdSize = entryCmdSize
	entry.EntryOff = codeOffset

	_ = binary.Write(buf, binary.LittleEndian, hdr)
	_ = binary.Write(buf, binary.LittleEndian, pageZero)
	_ = binary.Write(buf, binary.LittleEndian, textSeg)
	_ = binary.Write(buf, binary.LittleEndian, textSect)
	_ = binary.Write(buf, binary.LittleEndian, entry)

	if padding > 0 {
		buf.Write(make([]byte, padding))
	}
	buf.Write(code)

	return buf.Bytes(), nil
}
