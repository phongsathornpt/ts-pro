//go:build unix

package sys

import (
	"testing"
)

func TestMmapMunmap(t *testing.T) {
	mem, err := Mmap(4096)
	if err != nil {
		t.Fatalf("Mmap failed: %v", err)
	}
	if len(mem) != 4096 {
		t.Errorf("expected 4096 bytes, got %d", len(mem))
	}
	mem[0] = 0xAA
	mem[4095] = 0xBB

	if err := Munmap(mem); err != nil {
		t.Errorf("Munmap failed: %v", err)
	}
}
