package e2e_test

import (
	"encoding/hex"
	"runtime"
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestLinuxAMD64InternalSHA1KnownAnswers(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	vectors := []struct {
		message string
		digest  string
	}{
		{"", "da39a3ee5e6b4b0d3255bfef95601890afd80709"},
		{"abc", "a9993e364706816aba3e25717850c26c9cd0d89d"},
		{strings.Repeat("a", 56), "c2db330f6083854c99d4b5bfb6e8f29f201be699"},
		{strings.Repeat("a", 1000), "291e9a6c66994949b57ba5e650361e98fc36b1ba"},
	}

	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	var want strings.Builder
	for vi, vector := range vectors {
		input := fn.NewValue("sha1_input", bufType)
		digest := fn.NewValue("sha1_digest", bufType)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: input, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{ir.ConstString{Value: vector.message}}, ParamTypes: []types.Type{types.TypeString}},
			&ir.CallInst{Res: digest, Callee: "ts_crypto_sha1", Args: []ir.Operand{input}, ParamTypes: []types.Type{bufType}},
		)
		decoded, err := hex.DecodeString(vector.digest)
		if err != nil {
			t.Fatalf("decode SHA-1 vector %d: %v", vi, err)
		}
		for i, b := range decoded {
			value := fn.NewValue("sha1_byte", types.TypeNumber)
			bb.Instructions = append(bb.Instructions,
				&ir.CallInst{Res: value, Callee: "ts_byte_buffer_get", Args: []ir.Operand{digest, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
				&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeNumber}},
			)
			want.WriteString(formatByte(b))
		}
	}
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	runInternalAMD64IR(t, prog, "sha1-kat", want.String())
}
