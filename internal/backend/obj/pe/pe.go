package pe

import (
	"bytes"
	"encoding/binary"
)

const (
	IMAGE_DOS_SIGNATURE = 0x5A4D     // MZ
	IMAGE_NT_SIGNATURE  = 0x00004550 // PE\0\0

	IMAGE_FILE_MACHINE_AMD64 = 0x8664
	IMAGE_FILE_MACHINE_ARM64 = 0xAA64

	IMAGE_NT_OPTIONAL_HDR64_MAGIC = 0x20B

	IMAGE_SUBSYSTEM_WINDOWS_CUI = 3 // Console Application

	IMAGE_SCN_CNT_CODE    = 0x00000020
	IMAGE_SCN_MEM_EXECUTE = 0x20000000
	IMAGE_SCN_MEM_READ    = 0x40000000
)

type DosHeader struct {
	Magic    uint16
	Cblp     uint16
	Cp       uint16
	Crlc     uint16
	Cparhdr  uint16
	Minalloc uint16
	Maxalloc uint16
	Ss       uint16
	Sp       uint16
	Csum     uint16
	Ip       uint16
	Cs       uint16
	Lfarlc   uint16
	Ovno     uint16
	Res      [4]uint16
	Oemid    uint16
	Oeminfo  uint16
	Res2     [10]uint16
	Lfanew   uint32 // Offset to PE header
}

type FileHeader struct {
	Machine              uint16
	NumberOfSections     uint16
	TimeDateStamp        uint32
	PointerToSymbolTable uint32
	NumberOfSymbols      uint32
	SizeOfOptionalHeader uint16
	Characteristics      uint16
}

type OptionalHeader64 struct {
	Magic                       uint16
	MajorLinkerVersion          uint8
	MinorLinkerVersion          uint8
	SizeOfCode                  uint32
	SizeOfInitializedData       uint32
	SizeOfUninitializedData     uint32
	AddressOfEntryPoint         uint32
	BaseOfCode                  uint32
	ImageBase                   uint64
	SectionAlignment            uint32
	FileAlignment               uint32
	MajorOperatingSystemVersion uint16
	MinorOperatingSystemVersion uint16
	MajorImageVersion           uint16
	MinorImageVersion           uint16
	MajorSubsystemVersion       uint16
	MinorSubsystemVersion       uint16
	Win32VersionValue           uint32
	SizeOfImage                 uint32
	SizeOfHeaders               uint32
	CheckSum                    uint32
	Subsystem                   uint16
	DllCharacteristics          uint16
	SizeOfStackReserve          uint64
	SizeOfStackCommit           uint64
	SizeOfHeapReserve           uint64
	SizeOfHeapCommit            uint64
	LoaderFlags                 uint32
	NumberOfRvaAndSizes         uint32
	DataDirectory               [16][2]uint32
}

type SectionHeader struct {
	Name                 [8]byte
	VirtualSize          uint32
	VirtualAddress       uint32
	SizeOfRawData        uint32
	PointerToRawData     uint32
	PointerToRelocations uint32
	PointerToLinenumbers uint32
	NumberOfRelocations  uint16
	NumberOfLinenumbers  uint16
	Characteristics      uint32
}

// CreateExecutable generates a PE/COFF 64-bit executable from machine code.
func CreateExecutable(code []byte, isARM64 bool) ([]byte, error) {
	buf := new(bytes.Buffer)

	machine := uint16(IMAGE_FILE_MACHINE_AMD64)
	if isARM64 {
		machine = IMAGE_FILE_MACHINE_ARM64
	}

	dosHdr := DosHeader{
		Magic:  IMAGE_DOS_SIGNATURE,
		Lfanew: 0x80, // Offset to PE signature
	}

	fileHdr := FileHeader{
		Machine:              machine,
		NumberOfSections:     1,
		SizeOfOptionalHeader: uint16(binary.Size(OptionalHeader64{})),
		Characteristics:      0x0022, // EXECUTABLE_IMAGE | LARGE_ADDRESS_AWARE
	}

	optHdr := OptionalHeader64{
		Magic:                       IMAGE_NT_OPTIONAL_HDR64_MAGIC,
		SizeOfCode:                  uint32(len(code)),
		AddressOfEntryPoint:         0x1000,
		BaseOfCode:                  0x1000,
		ImageBase:                   0x140000000,
		SectionAlignment:            0x1000,
		FileAlignment:               0x200,
		MajorOperatingSystemVersion: 6,
		MajorSubsystemVersion:       6,
		SizeOfImage:                 0x2000,
		SizeOfHeaders:               0x200,
		Subsystem:                   IMAGE_SUBSYSTEM_WINDOWS_CUI,
		DllCharacteristics:          0x8160,
		SizeOfStackReserve:          0x100000,
		SizeOfStackCommit:           0x1000,
		SizeOfHeapReserve:           0x100000,
		SizeOfHeapCommit:            0x1000,
		NumberOfRvaAndSizes:         16,
	}

	var sectHdr SectionHeader
	copy(sectHdr.Name[:], ".text")
	sectHdr.VirtualSize = uint32(len(code))
	sectHdr.VirtualAddress = 0x1000
	sectHdr.SizeOfRawData = uint32((len(code) + 0x1FF) &^ 0x1FF)
	sectHdr.PointerToRawData = 0x200
	sectHdr.Characteristics = IMAGE_SCN_CNT_CODE | IMAGE_SCN_MEM_EXECUTE | IMAGE_SCN_MEM_READ

	// Write DOS Header
	_ = binary.Write(buf, binary.LittleEndian, dosHdr)

	// Pad to Lfanew (0x80)
	if buf.Len() < int(dosHdr.Lfanew) {
		buf.Write(make([]byte, int(dosHdr.Lfanew)-buf.Len()))
	}

	// Write PE Signature
	_ = binary.Write(buf, binary.LittleEndian, uint32(IMAGE_NT_SIGNATURE))
	// Write COFF Header
	_ = binary.Write(buf, binary.LittleEndian, fileHdr)
	// Write Optional Header
	_ = binary.Write(buf, binary.LittleEndian, optHdr)
	// Write Section Header
	_ = binary.Write(buf, binary.LittleEndian, sectHdr)

	// Pad to PointerToRawData (0x200)
	if buf.Len() < int(sectHdr.PointerToRawData) {
		buf.Write(make([]byte, int(sectHdr.PointerToRawData)-buf.Len()))
	}

	// Write .text code
	buf.Write(code)

	// Pad to raw data size
	if buf.Len() < int(sectHdr.PointerToRawData+sectHdr.SizeOfRawData) {
		buf.Write(make([]byte, int(sectHdr.PointerToRawData+sectHdr.SizeOfRawData)-buf.Len()))
	}

	return buf.Bytes(), nil
}
