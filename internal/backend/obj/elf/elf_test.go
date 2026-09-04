package elf

import (
	"bytes"
	"testing"
)

func TestCreateExecutable(t *testing.T) {
	code := []byte{0x48, 0x31, 0xC0, 0xC3} // xor rax, rax; ret
	elfData, err := CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("CreateExecutable failed: %v", err)
	}

	if len(elfData) < 64 {
		t.Fatalf("ELF binary too small: %d bytes", len(elfData))
	}

	// Verify magic bytes
	if !bytes.Equal(elfData[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		t.Errorf("expected ELF magic 0x7fELF, got %x", elfData[:4])
	}

	// Test ARM64
	elfArm, err := CreateExecutable(code, true)
	if err != nil {
		t.Fatalf("CreateExecutable(ARM64) failed: %v", err)
	}
	if len(elfArm) < 64 {
		t.Fatalf("ELF ARM64 binary too small: %d bytes", len(elfArm))
	}
}
