package e2e_test

import (
	"encoding/hex"
	"runtime"
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestLinuxAMD64InternalSHA384AndSHA512KnownAnswers(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	type vector struct {
		message string
		sha384  string
		sha512  string
	}
	vectors := []vector{
		{
			message: "",
			sha384:  "38b060a751ac96384cd9327eb1b1e36a21fdb71114be07434c0cc7bf63f6e1da274edebfe76f65fbd51ad2f14898b95b",
			sha512:  "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
		},
		{
			message: "abc",
			sha384:  "cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7",
			sha512:  "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
		},
		{
			message: strings.Repeat("a", 112),
			sha384:  "187d4e07cb306103c69967bf544d0dfbe9042577599c73c330abc0cb64c61236d5ed565ee19119d8c31779a38f791fcd",
			sha512:  "c01d080efd492776a1c43bd23dd99d0a2e626d481e16782e75d54c2503b5dc32bd05f0f1ba33e568b88fd2d970929b719ecbb152f58f130a407c8830604b70ca",
		},
		{
			message: strings.Repeat("a", 1000),
			sha384:  "f54480689c6b0b11d0303285d9a81b21a93bca6ba5a1b4472765dca4da45ee328082d469c650cd3b61b16d3266ab8ced",
			sha512:  "67ba5535a46e3f86dbfbed8cbbaf0125c76ed549ff8b0b9e03e0c88cf90fa634fa7b12b47d77b694de488ace8d9a65967dc96df599727d3292a8d9d447709c97",
		},
	}

	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	var want strings.Builder
	for vi, v := range vectors {
		input := fn.NewValue("sha2_input", bufType)
		bb.Instructions = append(bb.Instructions, &ir.CallInst{
			Res: input, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{ir.ConstString{Value: v.message}}, ParamTypes: []types.Type{types.TypeString},
		})
		for _, digestCase := range []struct {
			callee string
			hex    string
		}{
			{callee: "ts_crypto_sha384", hex: v.sha384},
			{callee: "ts_crypto_sha512", hex: v.sha512},
		} {
			digest := fn.NewValue("sha2_digest", bufType)
			bb.Instructions = append(bb.Instructions, &ir.CallInst{
				Res: digest, Callee: digestCase.callee, Args: []ir.Operand{input}, ParamTypes: []types.Type{bufType},
			})
			decoded, err := hex.DecodeString(digestCase.hex)
			if err != nil {
				t.Fatalf("decode vector %d for %s: %v", vi, digestCase.callee, err)
			}
			for i, b := range decoded {
				value := fn.NewValue("sha2_byte", types.TypeNumber)
				bb.Instructions = append(bb.Instructions,
					&ir.CallInst{Res: value, Callee: "ts_byte_buffer_get", Args: []ir.Operand{digest, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
					&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeNumber}},
				)
				want.WriteString(formatByte(b))
			}
		}
	}
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	runInternalAMD64IR(t, prog, "sha384-sha512-kat", want.String())
}
