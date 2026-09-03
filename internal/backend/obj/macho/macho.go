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

	LC_SEGMENT_64    = 0x19
	LC_BUILD_VERSION = 0x32
	LC_MAIN          = 0x80000028
	LC_LOAD_DYLINKER = 0xE
	LC_LOAD_DYLIB    = 0xC

	VM_PROT_NONE    = 0x0
	VM_PROT_READ    = 0x1
	VM_PROT_EXECUTE = 0x4
)

// DylibCmd represents LC_LOAD_DYLIB.
type DylibCmd struct {
	Cmd                  uint32
	CmdSize              uint32
	NameOffset           uint32
	Timestamp            uint32
	CurrentVersion       uint32
	CompatibilityVersion uint32
}

// BuildVersionCmd represents LC_BUILD_VERSION.
type BuildVersionCmd struct {
	Cmd      uint32
	CmdSize  uint32
	Platform uint32 // 1 = macOS
	MinOS    uint32 // 11.0.0 (0x000B0000)
	SDK      uint32 // 11.0.0 (0x000B0000)
	NTools   uint32 // 0
}

// DylinkerCmd represents LC_LOAD_DYLINKER.
type DylinkerCmd struct {
	Cmd        uint32
	CmdSize    uint32
	NameOffset uint32
}

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
	cpuSubtype := uint32(3) // CPU_SUBTYPE_X86_64_ALL
	if isARM64 {
		cpuType = CPU_TYPE_ARM64
		cpuSubtype = 0 // CPU_SUBTYPE_ARM64_ALL
	}

	headerSize := uint32(32)
	segPageZeroSize := uint32(72)
	segTextSize := uint32(72 + 80) // 1 section (__text)
	segLinkeditSize := uint32(72)
	buildVerSize := uint32(24)
	entryCmdSize := uint32(24)
	dylinkerPath := []byte("/usr/lib/dyld\x00\x00\x00\x00\x00\x00\x00") // 20 bytes -> cmdsize 32
	dylinkerCmdSize := uint32(12 + len(dylinkerPath))
	dylibPath := []byte("/usr/lib/libSystem.B.dylib\x00\x00\x00\x00\x00\x00") // 32 bytes -> cmdsize 56
	dylibCmdSize := uint32(24 + len(dylibPath))

	sizeOfCmds := segPageZeroSize + segTextSize + segLinkeditSize + buildVerSize + entryCmdSize + dylinkerCmdSize + dylibCmdSize
	codeOffset := uint64(1024)
	padding := int(codeOffset - uint64(headerSize+sizeOfCmds))

	hdr := Header64{
		Magic:      MH_MAGIC_64,
		CpuType:    cpuType,
		CpuSubtype: cpuSubtype,
		FileType:   MH_EXECUTE,
		NCmds:      7,
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
	pageSize := uint64(0x4000) // 16KB page on Apple Silicon
	var textSeg SegmentCmd64
	textSeg.Cmd = LC_SEGMENT_64
	textSeg.CmdSize = segTextSize
	copy(textSeg.SegName[:], "__TEXT")
	textSeg.VmAddr = baseAddr
	textSeg.VmSize = pageSize
	textSeg.FileOff = 0
	textSeg.FileSize = pageSize
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

	// __LINKEDIT segment
	var linkeditSeg SegmentCmd64
	linkeditSeg.Cmd = LC_SEGMENT_64
	linkeditSeg.CmdSize = segLinkeditSize
	copy(linkeditSeg.SegName[:], "__LINKEDIT")
	linkeditSeg.VmAddr = baseAddr + pageSize
	linkeditSeg.VmSize = pageSize
	linkeditSeg.FileOff = pageSize
	linkeditSeg.FileSize = 0
	linkeditSeg.MaxProt = VM_PROT_READ
	linkeditSeg.InitProt = VM_PROT_READ
	linkeditSeg.NSects = 0

	// LC_BUILD_VERSION
	var buildVer BuildVersionCmd
	buildVer.Cmd = LC_BUILD_VERSION
	buildVer.CmdSize = buildVerSize
	buildVer.Platform = 1       // PLATFORM_MACOS
	buildVer.MinOS = 0x000B0000 // macOS 11.0
	buildVer.SDK = 0x000B0000   // macOS 11.0
	buildVer.NTools = 0

	// LC_MAIN
	var entry EntryPointCmd
	entry.Cmd = LC_MAIN
	entry.CmdSize = entryCmdSize
	entry.EntryOff = codeOffset

	// LC_LOAD_DYLINKER
	var dylinker DylinkerCmd
	dylinker.Cmd = LC_LOAD_DYLINKER
	dylinker.CmdSize = dylinkerCmdSize
	dylinker.NameOffset = 12

	// LC_LOAD_DYLIB
	var dylib DylibCmd
	dylib.Cmd = LC_LOAD_DYLIB
	dylib.CmdSize = dylibCmdSize
	dylib.NameOffset = 24
	dylib.Timestamp = 2
	dylib.CurrentVersion = 0x054C0000       // 1356.0.0
	dylib.CompatibilityVersion = 0x00010000 // 1.0.0

	_ = binary.Write(buf, binary.LittleEndian, hdr)
	_ = binary.Write(buf, binary.LittleEndian, pageZero)
	_ = binary.Write(buf, binary.LittleEndian, textSeg)
	_ = binary.Write(buf, binary.LittleEndian, textSect)
	_ = binary.Write(buf, binary.LittleEndian, linkeditSeg)
	_ = binary.Write(buf, binary.LittleEndian, buildVer)
	_ = binary.Write(buf, binary.LittleEndian, entry)
	_ = binary.Write(buf, binary.LittleEndian, dylinker)
	buf.Write(dylinkerPath)
	_ = binary.Write(buf, binary.LittleEndian, dylib)
	buf.Write(dylibPath)

	if padding > 0 {
		buf.Write(make([]byte, padding))
	}
	buf.Write(code)

	// Pad remaining bytes of __TEXT up to pageSize
	if uint64(buf.Len()) < pageSize {
		buf.Write(make([]byte, pageSize-uint64(buf.Len())))
	}

	return buf.Bytes(), nil
}
