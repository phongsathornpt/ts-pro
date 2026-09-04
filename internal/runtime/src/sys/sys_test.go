//go:build unix

package sys

import (
	"io"
	"os"
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

func TestSysWrite(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	n, err := Write(int(w.Fd()), []byte("sys write test"))
	if err != nil || n != 14 {
		t.Fatalf("Write failed: n=%d err=%v", n, err)
	}

	buf := make([]byte, 14)
	if _, err := io.ReadFull(r, buf); err != nil || string(buf) != "sys write test" {
		t.Fatalf("read pipe failed: %v, got %q", err, string(buf))
	}
}
