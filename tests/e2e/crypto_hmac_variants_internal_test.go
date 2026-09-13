package e2e_test

import (
	"encoding/hex"
	"runtime"
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestLinuxAMD64InternalHMACHashVariantsKnownAnswers(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	type vector struct {
		name    string
		callee  string
		key     []byte
		message string
		digest  string
	}
	vectors := []vector{
		{
			name:    "sha1-long-key",
			callee:  "ts_crypto_hmac_sha1",
			key:     bytesRepeat(0xaa, 80),
			message: "Test Using Larger Than Block-Size Key - Hash Key First",
			digest:  "aa4ae5e15272d00e95705637ce8a3b55ed402112",
		},
		{
			name:    "sha384-long-key",
			callee:  "ts_crypto_hmac_sha384",
			key:     bytesRepeat(0xaa, 131),
			message: "Test Using Larger Than Block-Size Key - Hash Key First",
			digest:  "4ece084485813e9088d2c63a041bc5b44f9ef1012a2b588f3cd11f05033ac4c60c2ef6ab4030fe8296248df163f44952",
		},
		{
			name:    "sha512-long-key",
			callee:  "ts_crypto_hmac_sha512",
			key:     bytesRepeat(0xaa, 131),
			message: "Test Using Larger Than Block-Size Key - Hash Key First",
			digest:  "80b24263c7c1a3ebb71493c1dd7be8b49b46d1f41b4aeec1121b013783f8f3526b56d037e05f2598bd0fd2215d6a1e5295e64f73f63f0aec8b915a985d786598",
		},
	}

	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	var want strings.Builder

	for _, vector := range vectors {
		key := fn.NewValue("hmac_key_"+vector.name, bufType)
		message := fn.NewValue("hmac_message_"+vector.name, bufType)
		digest := fn.NewValue("hmac_digest_"+vector.name, bufType)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: key, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: float64(len(vector.key))}}, ParamTypes: []types.Type{types.TypeNumber}},
		)
		for i, b := range vector.key {
			bb.Instructions = append(bb.Instructions, &ir.CallInst{
				Callee:     "ts_byte_buffer_set",
				Args:       []ir.Operand{key, ir.ConstNumber{Value: float64(i)}, ir.ConstNumber{Value: float64(b)}},
				ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber},
			})
		}
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: message, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{ir.ConstString{Value: vector.message}}, ParamTypes: []types.Type{types.TypeString}},
			&ir.CallInst{Res: digest, Callee: vector.callee, Args: []ir.Operand{key, message}, ParamTypes: []types.Type{bufType, bufType}},
		)

		decoded, err := hex.DecodeString(vector.digest)
		if err != nil {
			t.Fatalf("decode %s vector: %v", vector.name, err)
		}
		for i, b := range decoded {
			value := fn.NewValue("hmac_byte_"+vector.name, types.TypeNumber)
			bb.Instructions = append(bb.Instructions,
				&ir.CallInst{Res: value, Callee: "ts_byte_buffer_get", Args: []ir.Operand{digest, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
				&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeNumber}},
			)
			want.WriteString(formatByte(b))
		}
	}

	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)
	runInternalAMD64IR(t, prog, "hmac-hash-variants-kat", want.String())
}
