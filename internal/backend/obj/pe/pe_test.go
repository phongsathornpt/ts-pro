package pe

import (
	"encoding/binary"
	"testing"
)

func TestCreatePEExecutable(t *testing.T) {
	code := []byte{0xC3} // ret
	peData, err := CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("CreateExecutable failed: %v", err)
	}

	if len(peData) < 512 {
		t.Fatalf("PE binary too small: %d bytes", len(peData))
	}

	dosSig := binary.LittleEndian.Uint16(peData[0:2])
	if dosSig != IMAGE_DOS_SIGNATURE {
		t.Errorf("expected DOS signature 'MZ' (0x5A4D), got 0x%x", dosSig)
	}

	peOffset := binary.LittleEndian.Uint32(peData[0x3C:0x40])
	peSig := binary.LittleEndian.Uint32(peData[peOffset : peOffset+4])
	if peSig != IMAGE_NT_SIGNATURE {
		t.Errorf("expected PE signature 'PE\\0\\0', got 0x%x", peSig)
	}
}
