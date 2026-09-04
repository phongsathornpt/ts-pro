package macho

import (
	"encoding/binary"
	"testing"
)

func TestCreateMachOExecutable(t *testing.T) {
	code := []byte{0xC3} // ret
	machoData, err := CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("CreateExecutable failed: %v", err)
	}

	if len(machoData) < 32 {
		t.Fatalf("Mach-O binary too small: %d bytes", len(machoData))
	}

	magic := binary.LittleEndian.Uint32(machoData[0:4])
	if magic != MH_MAGIC_64 {
		t.Errorf("expected Mach-O 64 magic 0xfeedfacf, got 0x%x", magic)
	}

	// Test ARM64
	armData, err := CreateExecutable(code, true)
	if err != nil {
		t.Fatalf("CreateExecutable ARM64 failed: %v", err)
	}
	cpuType := binary.LittleEndian.Uint32(armData[4:8])
	if cpuType != CPU_TYPE_ARM64 {
		t.Errorf("expected CPU_TYPE_ARM64, got 0x%x", cpuType)
	}
}
