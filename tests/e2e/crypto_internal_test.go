package e2e_test

import (
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/backend/lower"
	"github.com/phongsathornpt/ts-pro/internal/backend/obj/elf"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestLinuxAMD64InternalSHA256KnownAnswers(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	vectors := []struct {
		message string
		digest  string
	}{
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{strings.Repeat("a", 56), "b35439a4ac6f0948b6d6f9e3c6af0f5f590ce20f1bde7090ef7970686ec6738a"},
		{strings.Repeat("a", 1000), "41edece42d63e8d9bf515a9ba6932e1c20cbc9f5a5d134645adb5db1b9737ea3"},
	}

	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	var want strings.Builder
	for vi, vector := range vectors {
		input := fn.NewValue("sha_input", bufType)
		digest := fn.NewValue("sha_digest", bufType)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: input, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{ir.ConstString{Value: vector.message}}, ParamTypes: []types.Type{types.TypeString}},
			&ir.CallInst{Res: digest, Callee: "ts_crypto_sha256", Args: []ir.Operand{input}, ParamTypes: []types.Type{bufType}},
		)
		decoded, err := hex.DecodeString(vector.digest)
		if err != nil {
			t.Fatalf("decode vector %d: %v", vi, err)
		}
		for i, b := range decoded {
			v := fn.NewValue("sha_byte", types.TypeNumber)
			bb.Instructions = append(bb.Instructions,
				&ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{digest, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
				&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{v}, ParamTypes: []types.Type{types.TypeNumber}},
			)
			want.WriteString(formatByte(b))
		}
	}
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := lower.Lower(prog, lower.ArchAMD64)
	if err != nil {
		t.Fatalf("lower sha256 IR: %v", err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("create sha256 ELF: %v", err)
	}
	path := filepath.Join(t.TempDir(), "sha256-kat")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatalf("write sha256 ELF: %v", err)
	}
	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("execute sha256 ELF: %v\nOutput:\n%s", err, out)
	}
	if got := string(out); got != want.String() {
		t.Fatalf("sha256 output mismatch\ngot:\n%s\nwant:\n%s", got, want.String())
	}
}

func formatByte(b byte) string {
	if b == 0 {
		return "0\n"
	}
	var buf [3]byte
	i := len(buf)
	for b != 0 {
		i--
		buf[i] = '0' + b%10
		b /= 10
	}
	return string(buf[i:]) + "\n"
}

func TestLinuxAMD64InternalHMACSHA256KnownAnswers(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	type vector struct {
		key     []byte
		message string
		digest  string
	}
	vectors := []vector{
		{key: []byte("Jefe"), message: "what do ya want for nothing?", digest: "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843"},
		{key: bytesRepeat(0x0b, 20), message: "Hi There", digest: "b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7"},
		{key: bytesRepeat(0xaa, 131), message: "Test Using Larger Than Block-Size Key - Hash Key First", digest: "60e431591ee0b67f0d8a26aacbf5b77f8e0bc6213728c5140546040f0ee37f54"},
	}

	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	var want strings.Builder
	for _, vector := range vectors {
		key := fn.NewValue("hmac_key", bufType)
		message := fn.NewValue("hmac_message", bufType)
		digest := fn.NewValue("hmac_digest", bufType)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: key, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: float64(len(vector.key))}}, ParamTypes: []types.Type{types.TypeNumber}},
		)
		for i, b := range vector.key {
			bb.Instructions = append(bb.Instructions, &ir.CallInst{Callee: "ts_byte_buffer_set", Args: []ir.Operand{key, ir.ConstNumber{Value: float64(i)}, ir.ConstNumber{Value: float64(b)}}, ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber}})
		}
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: message, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{ir.ConstString{Value: vector.message}}, ParamTypes: []types.Type{types.TypeString}},
			&ir.CallInst{Res: digest, Callee: "ts_crypto_hmac_sha256", Args: []ir.Operand{key, message}, ParamTypes: []types.Type{bufType, bufType}},
		)
		decoded, err := hex.DecodeString(vector.digest)
		if err != nil {
			t.Fatal(err)
		}
		for i, b := range decoded {
			v := fn.NewValue("hmac_byte", types.TypeNumber)
			bb.Instructions = append(bb.Instructions,
				&ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{digest, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
				&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{v}, ParamTypes: []types.Type{types.TypeNumber}},
			)
			want.WriteString(formatByte(b))
		}
	}
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)
	runInternalAMD64IR(t, prog, "hmac-sha256-kat", want.String())
}

func TestLinuxAMD64InternalOSRandom(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	a := fn.NewValue("random_a", bufType)
	b := fn.NewValue("random_b", bufType)
	lenA := fn.NewValue("random_len", types.TypeNumber)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: a, Callee: "ts_os_random", Args: []ir.Operand{ir.ConstNumber{Value: 32}}, ParamTypes: []types.Type{types.TypeNumber}},
		&ir.CallInst{Res: b, Callee: "ts_os_random", Args: []ir.Operand{ir.ConstNumber{Value: 32}}, ParamTypes: []types.Type{types.TypeNumber}},
		&ir.CallInst{Res: lenA, Callee: "ts_byte_buffer_len", Args: []ir.Operand{a}, ParamTypes: []types.Type{bufType}},
		&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{lenA}, ParamTypes: []types.Type{types.TypeNumber}},
	)
	// Compare two independent 32-byte draws; equality would be cryptographically negligible.
	for i := 0; i < 32; i++ {
		av := fn.NewValue("random_a_byte", types.TypeNumber)
		bv := fn.NewValue("random_b_byte", types.TypeNumber)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: av, Callee: "ts_byte_buffer_get", Args: []ir.Operand{a, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
			&ir.CallInst{Res: bv, Callee: "ts_byte_buffer_get", Args: []ir.Operand{b, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
		)
		if i == 0 {
			// Printing both first bytes also ensures data is readable without exposing a pass/fail predicate to IR.
			bb.Instructions = append(bb.Instructions,
				&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{av}, ParamTypes: []types.Type{types.TypeNumber}},
				&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{bv}, ParamTypes: []types.Type{types.TypeNumber}},
			)
		}
	}
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := lower.Lower(prog, lower.ArchAMD64)
	if err != nil {
		t.Fatal(err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "os-random")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("execute random ELF: %v\n%s", err, out)
	}
	lines := strings.Fields(string(out))
	if len(lines) != 3 || lines[0] != "32" {
		t.Fatalf("unexpected random output %q", out)
	}
}

func bytesRepeat(value byte, count int) []byte {
	out := make([]byte, count)
	for i := range out {
		out[i] = value
	}
	return out
}

func runInternalAMD64IR(t *testing.T, prog *ir.Program, name, want string) {
	t.Helper()
	code, err := lower.Lower(prog, lower.ArchAMD64)
	if err != nil {
		t.Fatalf("lower %s IR: %v", name, err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("create %s ELF: %v", name, err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatalf("write %s ELF: %v", name, err)
	}
	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("execute %s ELF: %v\nOutput:\n%s", name, err, out)
	}
	if got := string(out); got != want {
		t.Fatalf("%s output mismatch\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

func TestLinuxAMD64InternalX25519RFC7748(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	const expectedHex = "422c8e7a6227d7bca1350b3e2bb7279f7897b87bb6854b783c60e80311ae3079"
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	scalar := fn.NewValue("x25519_scalar", bufType)
	u := fn.NewValue("x25519_u", bufType)
	result := fn.NewValue("x25519_result", bufType)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: scalar, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: 32}}, ParamTypes: []types.Type{types.TypeNumber}},
		&ir.CallInst{Res: u, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: 32}}, ParamTypes: []types.Type{types.TypeNumber}},
		&ir.CallInst{Callee: "ts_byte_buffer_set", Args: []ir.Operand{scalar, ir.ConstNumber{Value: 0}, ir.ConstNumber{Value: 9}}, ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber}},
		&ir.CallInst{Callee: "ts_byte_buffer_set", Args: []ir.Operand{u, ir.ConstNumber{Value: 0}, ir.ConstNumber{Value: 9}}, ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber}},
		&ir.CallInst{Res: result, Callee: "ts_crypto_x25519", Args: []ir.Operand{scalar, u}, ParamTypes: []types.Type{bufType, bufType}},
	)
	decoded, err := hex.DecodeString(expectedHex)
	if err != nil {
		t.Fatal(err)
	}
	var want strings.Builder
	for i, b := range decoded {
		v := fn.NewValue("x25519_byte", types.TypeNumber)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{result, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
			&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{v}, ParamTypes: []types.Type{types.TypeNumber}},
		)
		want.WriteString(formatByte(b))
	}
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)
	runInternalAMD64IR(t, prog, "x25519-rfc7748", want.String())
}

func TestLinuxAMD64InternalHKDFSHA256RFC5869(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	ikm := bytesRepeat(0x0b, 22)
	salt, _ := hex.DecodeString("000102030405060708090a0b0c")
	info, _ := hex.DecodeString("f0f1f2f3f4f5f6f7f8f9")
	wantPRK, _ := hex.DecodeString("077709362c2e32df0ddc3f0dc47bba63" +
		"90b6c73bb50f9c3122ec844ad7c2b3e5")
	wantOKM, _ := hex.DecodeString("3cb25f25faacd57a90434f64d0362f2a" +
		"2d2d0a90cf1a5a4c5db02d56ecc4c5bf" +
		"34007208d5b887185865")

	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	newLiteral := func(name string, data []byte) *ir.Value {
		buf := fn.NewValue(name, bufType)
		bb.Instructions = append(bb.Instructions, &ir.CallInst{
			Res: buf, Callee: "ts_byte_buffer_new",
			Args: []ir.Operand{ir.ConstNumber{Value: float64(len(data))}}, ParamTypes: []types.Type{types.TypeNumber},
		})
		for i, b := range data {
			bb.Instructions = append(bb.Instructions, &ir.CallInst{
				Callee:     "ts_byte_buffer_set",
				Args:       []ir.Operand{buf, ir.ConstNumber{Value: float64(i)}, ir.ConstNumber{Value: float64(b)}},
				ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber},
			})
		}
		return buf
	}
	saltBuf := newLiteral("hkdf_salt", salt)
	ikmBuf := newLiteral("hkdf_ikm", ikm)
	infoBuf := newLiteral("hkdf_info", info)
	prk := fn.NewValue("hkdf_prk", bufType)
	okm := fn.NewValue("hkdf_okm", bufType)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: prk, Callee: "ts_crypto_hkdf_extract_sha256", Args: []ir.Operand{saltBuf, ikmBuf}, ParamTypes: []types.Type{bufType, bufType}},
		&ir.CallInst{Res: okm, Callee: "ts_crypto_hkdf_expand_sha256", Args: []ir.Operand{prk, infoBuf, ir.ConstNumber{Value: 42}}, ParamTypes: []types.Type{bufType, bufType, types.TypeNumber}},
	)
	var want strings.Builder
	for _, item := range []struct {
		buf   *ir.Value
		bytes []byte
	}{{prk, wantPRK}, {okm, wantOKM}} {
		for i, b := range item.bytes {
			v := fn.NewValue("hkdf_byte", types.TypeNumber)
			bb.Instructions = append(bb.Instructions,
				&ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{item.buf, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
				&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{v}, ParamTypes: []types.Type{types.TypeNumber}},
			)
			want.WriteString(formatByte(b))
		}
	}
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)
	runInternalAMD64IR(t, prog, "hkdf-sha256-rfc5869", want.String())
}

func TestLinuxAMD64InternalAES128GCM(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	wantCipher, _ := hex.DecodeString("0388dace60b6a392f328c2b971b2fe78" +
		"c7acb19afdcc5aeaba8599b72b79a5f1")
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	newLiteral := func(name string, data []byte) *ir.Value {
		buf := fn.NewValue(name, bufType)
		bb.Instructions = append(bb.Instructions, &ir.CallInst{
			Res: buf, Callee: "ts_byte_buffer_new",
			Args: []ir.Operand{ir.ConstNumber{Value: float64(len(data))}}, ParamTypes: []types.Type{types.TypeNumber},
		})
		for i, b := range data {
			bb.Instructions = append(bb.Instructions, &ir.CallInst{
				Callee:     "ts_byte_buffer_set",
				Args:       []ir.Operand{buf, ir.ConstNumber{Value: float64(i)}, ir.ConstNumber{Value: float64(b)}},
				ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber},
			})
		}
		return buf
	}
	key := newLiteral("gcm_key", make([]byte, 16))
	iv := newLiteral("gcm_iv", make([]byte, 12))
	aad := newLiteral("gcm_aad", []byte("ABCDE"))
	plain := newLiteral("gcm_plain", make([]byte, 16))
	ciphertext := fn.NewValue("gcm_ciphertext", bufType)
	decrypted := fn.NewValue("gcm_decrypted", bufType)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: ciphertext, Callee: "ts_crypto_aes_128_gcm_encrypt", Args: []ir.Operand{key, iv, aad, plain}, ParamTypes: []types.Type{bufType, bufType, bufType, bufType}},
		&ir.CallInst{Res: decrypted, Callee: "ts_crypto_aes_128_gcm_decrypt", Args: []ir.Operand{key, iv, aad, ciphertext}, ParamTypes: []types.Type{bufType, bufType, bufType, bufType}},
	)
	var want strings.Builder
	for i, b := range wantCipher {
		v := fn.NewValue("gcm_cipher_byte", types.TypeNumber)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{ciphertext, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
			&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{v}, ParamTypes: []types.Type{types.TypeNumber}},
		)
		want.WriteString(formatByte(b))
	}
	for i := 0; i < 16; i++ {
		v := fn.NewValue("gcm_plain_byte", types.TypeNumber)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{decrypted, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
			&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{v}, ParamTypes: []types.Type{types.TypeNumber}},
		)
		want.WriteString("0\n")
	}
	// Flip one authentication-tag bit. Decrypt must fail closed with length 0.
	bad := fn.NewValue("gcm_bad", bufType)
	badLen := fn.NewValue("gcm_bad_len", types.TypeNumber)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Callee: "ts_byte_buffer_set", Args: []ir.Operand{ciphertext, ir.ConstNumber{Value: 31}, ir.ConstNumber{Value: 240}}, ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber}},
		&ir.CallInst{Res: bad, Callee: "ts_crypto_aes_128_gcm_decrypt", Args: []ir.Operand{key, iv, aad, ciphertext}, ParamTypes: []types.Type{bufType, bufType, bufType, bufType}},
		&ir.CallInst{Res: badLen, Callee: "ts_byte_buffer_len", Args: []ir.Operand{bad}, ParamTypes: []types.Type{bufType}},
		&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{badLen}, ParamTypes: []types.Type{types.TypeNumber}},
	)
	want.WriteString("0\n")
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)
	runInternalAMD64IR(t, prog, "aes-128-gcm-afalg", want.String())
}

func TestLinuxAMD64InternalTLS13KDFAndNonce(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	wantKey, _ := hex.DecodeString("41bad7ea872fee55da270034f38b9e87")
	wantNonce, _ := hex.DecodeString("00010203040507050b0d0f0d")
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	newLiteral := func(name string, data []byte) *ir.Value {
		buf := fn.NewValue(name, bufType)
		bb.Instructions = append(bb.Instructions, &ir.CallInst{
			Res: buf, Callee: "ts_byte_buffer_new",
			Args: []ir.Operand{ir.ConstNumber{Value: float64(len(data))}}, ParamTypes: []types.Type{types.TypeNumber},
		})
		for i, b := range data {
			bb.Instructions = append(bb.Instructions, &ir.CallInst{
				Callee:     "ts_byte_buffer_set",
				Args:       []ir.Operand{buf, ir.ConstNumber{Value: float64(i)}, ir.ConstNumber{Value: float64(b)}},
				ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber},
			})
		}
		return buf
	}
	secretBytes := make([]byte, 32)
	for i := range secretBytes {
		secretBytes[i] = byte(i)
	}
	ivBytes := make([]byte, 12)
	for i := range ivBytes {
		ivBytes[i] = byte(i)
	}
	secret := newLiteral("tls_secret", secretBytes)
	label := newLiteral("tls_label", []byte("key"))
	context := newLiteral("tls_context", make([]byte, 32))
	iv := newLiteral("tls_iv", ivBytes)
	derived := fn.NewValue("tls_derived", bufType)
	nonce := fn.NewValue("tls_nonce", bufType)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: derived, Callee: "ts_tls13_hkdf_expand_label", Args: []ir.Operand{secret, label, context, ir.ConstNumber{Value: 16}}, ParamTypes: []types.Type{bufType, bufType, bufType, types.TypeNumber}},
		&ir.CallInst{Res: nonce, Callee: "ts_tls13_nonce", Args: []ir.Operand{iv, ir.ConstNumber{Value: 1108152157446}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
	)
	var want strings.Builder
	for _, item := range []struct {
		buf   *ir.Value
		bytes []byte
	}{{derived, wantKey}, {nonce, wantNonce}} {
		for i, b := range item.bytes {
			v := fn.NewValue("tls_kdf_byte", types.TypeNumber)
			bb.Instructions = append(bb.Instructions,
				&ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{item.buf, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
				&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{v}, ParamTypes: []types.Type{types.TypeNumber}},
			)
			want.WriteString(formatByte(b))
		}
	}
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)
	runInternalAMD64IR(t, prog, "tls13-kdf-nonce", want.String())
}

func TestLinuxAMD64InternalTLS13EncryptRecord(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	const expected = "170303001a6277276bcaad992e148d1273b5aeeb64d37ff9781dfd3844a01c"
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	key := fn.NewValue("tls_key", bufType)
	iv := fn.NewValue("tls_iv", bufType)
	content := fn.NewValue("tls_content", bufType)
	record := fn.NewValue("tls_record", bufType)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: key, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: 16}}, ParamTypes: []types.Type{types.TypeNumber}},
		&ir.CallInst{Res: iv, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: 12}}, ParamTypes: []types.Type{types.TypeNumber}},
	)
	for i := 0; i < 16; i++ {
		bb.Instructions = append(bb.Instructions, &ir.CallInst{Callee: "ts_byte_buffer_set", Args: []ir.Operand{key, ir.ConstNumber{Value: float64(i)}, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber}})
	}
	for i := 0; i < 12; i++ {
		bb.Instructions = append(bb.Instructions, &ir.CallInst{Callee: "ts_byte_buffer_set", Args: []ir.Operand{iv, ir.ConstNumber{Value: float64(i)}, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber}})
	}
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: content, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{ir.ConstString{Value: "hello tls"}}, ParamTypes: []types.Type{types.TypeString}},
		&ir.CallInst{Res: record, Callee: "ts_tls13_encrypt_record", Args: []ir.Operand{key, iv, content, ir.ConstNumber{Value: 22}, ir.ConstNumber{Value: 7}}, ParamTypes: []types.Type{bufType, bufType, bufType, types.TypeNumber, types.TypeNumber}},
	)
	decoded, err := hex.DecodeString(expected)
	if err != nil {
		t.Fatal(err)
	}
	var want strings.Builder
	for i, b := range decoded {
		v := fn.NewValue("tls_record_byte", types.TypeNumber)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{record, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
			&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{v}, ParamTypes: []types.Type{types.TypeNumber}},
		)
		want.WriteString(formatByte(b))
	}
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)
	runInternalAMD64IR(t, prog, "tls13-encrypt-record", want.String())
}

func TestLinuxAMD64InternalTLS13DecryptRecord(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	const recordHex = "170303001a6277276bcaad992e148d1273b5aeeb64d37ff9781dfd3844a01c"
	recordBytes, err := hex.DecodeString(recordHex)
	if err != nil {
		t.Fatal(err)
	}
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	newBytes := func(name string, data []byte) *ir.Value {
		buf := fn.NewValue(name, bufType)
		bb.Instructions = append(bb.Instructions, &ir.CallInst{Res: buf, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: float64(len(data))}}, ParamTypes: []types.Type{types.TypeNumber}})
		for i, b := range data {
			bb.Instructions = append(bb.Instructions, &ir.CallInst{Callee: "ts_byte_buffer_set", Args: []ir.Operand{buf, ir.ConstNumber{Value: float64(i)}, ir.ConstNumber{Value: float64(b)}}, ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber}})
		}
		return buf
	}
	keyBytes := make([]byte, 16)
	ivBytes := make([]byte, 12)
	for i := range keyBytes {
		keyBytes[i] = byte(i)
	}
	for i := range ivBytes {
		ivBytes[i] = byte(i)
	}
	key := newBytes("tls_key", keyBytes)
	iv := newBytes("tls_iv", ivBytes)
	record := newBytes("tls_record", recordBytes)
	plain := fn.NewValue("tls_plain", bufType)
	bb.Instructions = append(bb.Instructions, &ir.CallInst{Res: plain, Callee: "ts_tls13_decrypt_record", Args: []ir.Operand{key, iv, record, ir.ConstNumber{Value: 7}}, ParamTypes: []types.Type{bufType, bufType, bufType, types.TypeNumber}})
	wantBytes := append([]byte("hello tls"), byte(22))
	var want strings.Builder
	for i, b := range wantBytes {
		v := fn.NewValue("tls_plain_byte", types.TypeNumber)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{plain, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
			&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{v}, ParamTypes: []types.Type{types.TypeNumber}},
		)
		want.WriteString(formatByte(b))
	}
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)
	runInternalAMD64IR(t, prog, "tls13-decrypt-record", want.String())
}
