package runtime

import (
	"embed"
	"fmt"
)

// Embed all runtime primitive sources
//
//go:embed src/*
var RuntimeFS embed.FS

// RuntimeSymbol defines an exported native symbol provided by the embedded runtime.
type RuntimeSymbol struct {
	Name      string
	Signature string
}

// ProvidedSymbols lists the ABI runtime functions linked into generated executables.
var ProvidedSymbols = []RuntimeSymbol{
	{Name: "ts_alloc", Signature: "func(size: uint64) -> ptr"},
	{Name: "ts_string_concat", Signature: "func(a: ptr, b: ptr) -> ptr"},
	{Name: "ts_string_equals", Signature: "func(a: ptr, b: ptr) -> bool"},
	{Name: "ts_string_slice", Signature: "func(s: ptr, start: int64, end: int64) -> ptr"},
	{Name: "ts_array_new", Signature: "func(cap: int64) -> ptr"},
	{Name: "ts_array_push", Signature: "func(arr: ptr, val: uint64)"},
	{Name: "ts_array_pop", Signature: "func(arr: ptr) -> uint64"},
	{Name: "ts_array_get", Signature: "func(arr: ptr, idx: int64) -> uint64"},
	{Name: "ts_array_set", Signature: "func(arr: ptr, idx: int64, val: uint64)"},
	{Name: "ts_closure_new", Signature: "func(fn: ptr, upvalues: ptr) -> ptr"},
	{Name: "ts_sys_write", Signature: "func(fd: int64, ptr: ptr, len: int64) -> int64"},
	{Name: "ts_sys_exit", Signature: "func(code: int64)"},
}

// GetRuntimeSource reads an embedded runtime source file.
func GetRuntimeSource(path string) ([]byte, error) {
	data, err := RuntimeFS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read embedded runtime source %q: %w", path, err)
	}
	return data, nil
}
