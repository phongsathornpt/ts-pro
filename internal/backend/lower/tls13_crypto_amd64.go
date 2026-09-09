package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64TLS13HKDFExpandLabel emits RFC 8446 HKDF-Expand-Label for SHA-256.
// ABI: RDI=secret, RSI=label (without "tls13 "), RDX=context,
// XMM0=output length -> RAX=derived ByteBuffer.
func emitAMD64TLS13HKDFExpandLabel(e *amd64.Emitter, hkdfExpandOffset, newOffset, copyOffset int) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 96)
	e.MovRegReg(amd64.RBX, amd64.RDI) // secret
	e.MovRegReg(amd64.R12, amd64.RSI) // label
	e.MovRegReg(amd64.R13, amd64.RDX) // context
	e.Cvttsd2si(amd64.R14, amd64.XMM0)

	e.MovRegDeref(amd64.R8, amd64.R12, amd64ByteBufferLength)
	e.CmpRegImm32(amd64.R8, 249) // 6-byte "tls13 " prefix + label <= 255
	invalidLabel := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	e.MovRegDeref(amd64.R9, amd64.R13, amd64ByteBufferLength)
	e.CmpRegImm32(amd64.R9, 255)
	invalidContext := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	e.TestRegReg(amd64.R14, amd64.R14)
	invalidNegative := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	e.CmpRegImm32(amd64.R14, 255*32)
	invalidLength := len(e.Code)
	e.JccRel32(amd64.CondA, 0)

	// Root secret, label, context, info and result across allocations.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 5)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.RSP, 32, amd64.R13)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RSP, 40, amd64.R10)
	e.MovDerefReg(amd64.RSP, 48, amd64.R10)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegDeref(amd64.R8, amd64.R12, amd64ByteBufferLength)
	e.MovRegDeref(amd64.R9, amd64.R13, amd64ByteBufferLength)
	e.MovRegReg(amd64.R10, amd64.R8)
	e.AddRegReg(amd64.R10, amd64.R9)
	e.AddRegImm32(amd64.R10, 10)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callInfo := len(e.Code)
	e.CallRel32(int32(newOffset - (callInfo + 5)))
	e.MovDerefReg(amd64.RSP, 40, amd64.RAX)

	// HkdfLabel.length is uint16 network byte order.
	e.MovRegDeref(amd64.R10, amd64.RSP, 40)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64ByteBufferData)
	e.MovRegReg(amd64.RAX, amd64.R14)
	e.ShrRegImm8(amd64.RAX, 8)
	e.MovDerefReg8(amd64.R11, 0, amd64.RAX)
	e.MovDerefReg8(amd64.R11, 1, amd64.R14)
	e.MovRegDeref(amd64.R8, amd64.R12, amd64ByteBufferLength)
	e.MovRegReg(amd64.RAX, amd64.R8)
	e.AddRegImm32(amd64.RAX, 6)
	e.MovDerefReg8(amd64.R11, 2, amd64.RAX)
	for i, b := range []byte("tls13 ") {
		e.MovRegImm64(amd64.RAX, int64(b))
		e.MovDerefReg8(amd64.R11, int32(3+i), amd64.RAX)
	}

	// Copy caller label after the mandatory prefix.
	e.MovRegDeref(amd64.RDI, amd64.RSP, 40)
	e.MovRegReg(amd64.RSI, amd64.R12)
	e.MovRegImm64(amd64.R10, 9)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.Cvtsi2sd(amd64.XMM2, amd64.R8)
	copyLabel := len(e.Code)
	e.CallRel32(int32(copyOffset - (copyLabel + 5)))

	// context_length follows the variable-size label.
	e.MovRegDeref(amd64.R10, amd64.RSP, 40)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64ByteBufferData)
	e.MovRegDeref(amd64.R8, amd64.R12, amd64ByteBufferLength)
	e.MovRegDeref(amd64.R9, amd64.R13, amd64ByteBufferLength)
	e.MovRegReg(amd64.R10, amd64.R11)
	e.AddRegImm32(amd64.R10, 9)
	e.AddRegReg(amd64.R10, amd64.R8)
	e.MovDerefReg8(amd64.R10, 0, amd64.R9)

	// Copy transcript/context bytes after the context length byte.
	e.MovRegDeref(amd64.RDI, amd64.RSP, 40)
	e.MovRegReg(amd64.RSI, amd64.R13)
	e.MovRegReg(amd64.R10, amd64.R8)
	e.AddRegImm32(amd64.R10, 10)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.Cvtsi2sd(amd64.XMM2, amd64.R9)
	copyContext := len(e.Code)
	e.CallRel32(int32(copyOffset - (copyContext + 5)))

	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegDeref(amd64.RSI, amd64.RSP, 40)
	e.Cvtsi2sd(amd64.XMM0, amd64.R14)
	callExpand := len(e.Code)
	e.CallRel32(int32(hkdfExpandOffset - (callExpand + 5)))
	e.MovDerefReg(amd64.RSP, 48, amd64.RAX)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	successDone := len(e.Code)
	e.JmpRel32(0)
	invalid := len(e.Code)
	for _, at := range []int{invalidLabel, invalidContext, invalidNegative, invalidLength} {
		patchJcc(at, invalid)
	}
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callEmpty := len(e.Code)
	e.CallRel32(int32(newOffset - (callEmpty + 5)))

	epilogue := len(e.Code)
	patchJmp(successDone, epilogue)
	e.AddRegImm32(amd64.RSP, 96)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

// emitAMD64TLS13Nonce computes RFC 8446 record nonce = static_iv XOR seq_num.
// ABI: RDI=12-byte IV, XMM0=sequence number -> RAX=12-byte nonce.
func emitAMD64TLS13Nonce(e *amd64.Emitter, newOffset, copyOffset int) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.SubRegImm32(amd64.RSP, 32)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.Cvttsd2si(amd64.R12, amd64.XMM0)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferLength)
	e.CmpRegImm32(amd64.R10, 12)
	invalidIV := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 2)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RSP, 24, amd64.R10)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegImm64(amd64.R10, 12)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callNew := len(e.Code)
	e.CallRel32(int32(newOffset - (callNew + 5)))
	e.MovDerefReg(amd64.RSP, 24, amd64.RAX)
	e.MovRegReg(amd64.RDI, amd64.RAX)
	e.MovRegReg(amd64.RSI, amd64.RBX)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.MovRegImm64(amd64.R10, 12)
	e.Cvtsi2sd(amd64.XMM2, amd64.R10)
	copyIV := len(e.Code)
	e.CallRel32(int32(copyOffset - (copyIV + 5)))

	e.MovRegDeref(amd64.R10, amd64.RSP, 24)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64ByteBufferData)
	for i := 0; i < 8; i++ {
		e.MovRegReg(amd64.RDX, amd64.R12)
		if i != 0 {
			e.ShrRegImm8(amd64.RDX, byte(8*i))
		}
		e.MovRegImm64(amd64.RAX, 0xff)
		e.AndRegReg(amd64.RDX, amd64.RAX)
		e.MovzxRegDeref8(amd64.RAX, amd64.R11, int32(11-i))
		e.XorRegReg(amd64.RAX, amd64.RDX)
		e.MovDerefReg8(amd64.R11, int32(11-i), amd64.RAX)
	}
	e.MovRegDeref(amd64.RAX, amd64.RSP, 24)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	successDone := len(e.Code)
	e.JmpRel32(0)

	invalid := len(e.Code)
	patchJcc(invalidIV, invalid)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callEmpty := len(e.Code)
	e.CallRel32(int32(newOffset - (callEmpty + 5)))

	epilogue := len(e.Code)
	patchJmp(successDone, epilogue)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

// emitAMD64TLS13EncryptRecord builds one TLS 1.3 TLSCiphertext record.
// ABI: RDI=key, RSI=iv, RDX=content, XMM0=inner content type, XMM1=seq.
func emitAMD64TLS13EncryptRecord(e *amd64.Emitter, nonceOffset, aeadOffset, newOffset, copyOffset int) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 128)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegReg(amd64.R13, amd64.RDX)
	e.Cvttsd2si(amd64.R10, amd64.XMM0)
	e.MovDerefReg(amd64.RSP, 96, amd64.R10)
	e.Cvttsd2si(amd64.R10, amd64.XMM1)
	e.MovDerefReg(amd64.RSP, 104, amd64.R10)
	// Root key, IV, content and all temporary buffers across allocations.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 7)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.RSP, 32, amd64.R13)
	e.MovRegImm64(amd64.R10, 0)
	for _, off := range []int32{40, 48, 56, 64} {
		e.MovDerefReg(amd64.RSP, off, amd64.R10)
	}
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	// inner = content || content_type (no padding for now).
	e.MovRegDeref(amd64.R10, amd64.R13, amd64ByteBufferLength)
	e.AddRegImm32(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 112, amd64.R10)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callInnerNew := len(e.Code)
	e.CallRel32(int32(newOffset - (callInnerNew + 5)))
	e.MovDerefReg(amd64.RSP, 40, amd64.RAX)
	e.MovRegReg(amd64.RDI, amd64.RAX)
	e.MovRegReg(amd64.RSI, amd64.R13)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.R13, amd64ByteBufferLength)
	e.Cvtsi2sd(amd64.XMM2, amd64.R10)
	callCopyContent := len(e.Code)
	e.CallRel32(int32(copyOffset - (callCopyContent + 5)))
	e.MovRegDeref(amd64.R10, amd64.RSP, 40)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64ByteBufferData)
	e.MovRegDeref(amd64.RDX, amd64.R13, amd64ByteBufferLength)
	e.AddRegReg(amd64.R11, amd64.RDX)
	e.MovRegDeref(amd64.RAX, amd64.RSP, 96)
	e.MovDerefReg8(amd64.R11, 0, amd64.RAX)

	// nonce = iv XOR padded sequence number.
	e.MovRegReg(amd64.RDI, amd64.R12)
	e.MovRegDeref(amd64.R10, amd64.RSP, 104)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callNonce := len(e.Code)
	e.CallRel32(int32(nonceOffset - (callNonce + 5)))
	e.MovDerefReg(amd64.RSP, 48, amd64.RAX)
	// AAD is the five-byte TLSCiphertext header: application_data, 0x0303, len.
	e.MovRegImm64(amd64.R10, 5)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callAADNew := len(e.Code)
	e.CallRel32(int32(newOffset - (callAADNew + 5)))
	e.MovDerefReg(amd64.RSP, 56, amd64.RAX)
	e.MovRegDeref(amd64.R11, amd64.RAX, amd64ByteBufferData)
	for i, b := range []int64{23, 3, 3} {
		e.MovRegImm64(amd64.R10, b)
		e.MovDerefReg8(amd64.R11, int32(i), amd64.R10)
	}
	e.MovRegDeref(amd64.R10, amd64.RSP, 112)
	e.AddRegImm32(amd64.R10, 16)
	e.MovRegReg(amd64.RAX, amd64.R10)
	e.ShrRegImm8(amd64.RAX, 8)
	e.MovDerefReg8(amd64.R11, 3, amd64.RAX)
	e.MovDerefReg8(amd64.R11, 4, amd64.R10)

	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegDeref(amd64.RSI, amd64.RSP, 48)
	e.MovRegDeref(amd64.RDX, amd64.RSP, 56)
	e.MovRegDeref(amd64.RCX, amd64.RSP, 40)
	callAEAD := len(e.Code)
	e.CallRel32(int32(aeadOffset - (callAEAD + 5)))
	e.MovDerefReg(amd64.RSP, 64, amd64.RAX)
	// Reject AF_ALG failure before constructing a record.
	e.MovRegDeref(amd64.R10, amd64.RAX, amd64ByteBufferLength)
	e.TestRegReg(amd64.R10, amd64.R10)
	aeadFailed := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.AddRegImm32(amd64.R10, 5)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callRecordNew := len(e.Code)
	e.CallRel32(int32(newOffset - (callRecordNew + 5)))
	e.MovDerefReg(amd64.RSP, 72, amd64.RAX)

	// Copy header then ciphertext||tag.
	e.MovRegReg(amd64.RDI, amd64.RAX)
	e.MovRegDeref(amd64.RSI, amd64.RSP, 56)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.MovRegImm64(amd64.R10, 5)
	e.Cvtsi2sd(amd64.XMM2, amd64.R10)
	copyHeader := len(e.Code)
	e.CallRel32(int32(copyOffset - (copyHeader + 5)))
	e.MovRegDeref(amd64.RDI, amd64.RSP, 72)
	e.MovRegDeref(amd64.RSI, amd64.RSP, 64)
	e.MovRegImm64(amd64.R10, 5)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.RSI, amd64ByteBufferLength)
	e.Cvtsi2sd(amd64.XMM2, amd64.R10)
	copyCiphertext := len(e.Code)
	e.CallRel32(int32(copyOffset - (copyCiphertext + 5)))
	e.MovRegDeref(amd64.RAX, amd64.RSP, 72)
	success := len(e.Code)
	e.JmpRel32(0)

	failed := len(e.Code)
	patchJcc(aeadFailed, failed)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callEmpty := len(e.Code)
	e.CallRel32(int32(newOffset - (callEmpty + 5)))

	done := len(e.Code)
	patchJmp(success, done)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.AddRegImm32(amd64.RSP, 128)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

// emitAMD64TLS13DecryptRecord authenticates one TLSCiphertext record and
// returns TLSInnerPlaintext (content || content_type || optional padding).
// ABI: RDI=key, RSI=iv, RDX=record, XMM0=seq -> RAX=inner plaintext.
func emitAMD64TLS13DecryptRecord(e *amd64.Emitter, nonceOffset, aeadOffset, newOffset, copyOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 112)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegReg(amd64.R13, amd64.RDX)
	e.Cvttsd2si(amd64.R10, amd64.XMM0)
	e.MovDerefReg(amd64.RSP, 88, amd64.R10)
	// Validate outer header and encoded length before invoking AEAD.
	e.MovRegDeref(amd64.R10, amd64.R13, amd64ByteBufferLength)
	e.CmpRegImm32(amd64.R10, 21) // 5-byte header + at least 16-byte tag.
	invalidShort := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovRegDeref(amd64.R11, amd64.R13, amd64ByteBufferData)
	e.MovzxRegDeref8(amd64.R10, amd64.R11, 0)
	e.CmpRegImm32(amd64.R10, 23)
	invalidType := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovzxRegDeref8(amd64.R10, amd64.R11, 1)
	e.CmpRegImm32(amd64.R10, 3)
	invalidMajor := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovzxRegDeref8(amd64.R10, amd64.R11, 2)
	e.CmpRegImm32(amd64.R10, 3)
	invalidMinor := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	e.MovzxRegDeref8(amd64.R10, amd64.R11, 3)
	e.ShlRegImm8(amd64.R10, 8)
	e.MovzxRegDeref8(amd64.RAX, amd64.R11, 4)
	e.AddRegReg(amd64.R10, amd64.RAX)
	e.MovRegDeref(amd64.RAX, amd64.R13, amd64ByteBufferLength)
	e.SubRegImm32(amd64.RAX, 5)
	e.CmpRegReg(amd64.R10, amd64.RAX)
	invalidLength := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	// Root inputs plus AAD, ciphertext and nonce slices.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 6)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.RSP, 32, amd64.R13)
	e.MovRegImm64(amd64.R10, 0)
	for _, off := range []int32{40, 48, 56} {
		e.MovDerefReg(amd64.RSP, off, amd64.R10)
	}
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	// Slice AAD header [0:5].
	e.MovRegImm64(amd64.R10, 5)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callAADNew := len(e.Code)
	e.CallRel32(int32(newOffset - (callAADNew + 5)))
	e.MovDerefReg(amd64.RSP, 40, amd64.RAX)
	e.MovRegReg(amd64.RDI, amd64.RAX)
	e.MovRegReg(amd64.RSI, amd64.R13)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.MovRegImm64(amd64.R10, 5)
	e.Cvtsi2sd(amd64.XMM2, amd64.R10)
	copyAAD := len(e.Code)
	e.CallRel32(int32(copyOffset - (copyAAD + 5)))
	// Slice ciphertext||tag [5:].
	e.MovRegDeref(amd64.R10, amd64.R13, amd64ByteBufferLength)
	e.SubRegImm32(amd64.R10, 5)
	e.MovDerefReg(amd64.RSP, 96, amd64.R10)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callCipherNew := len(e.Code)
	e.CallRel32(int32(newOffset - (callCipherNew + 5)))
	e.MovDerefReg(amd64.RSP, 48, amd64.RAX)
	e.MovRegReg(amd64.RDI, amd64.RAX)
	e.MovRegReg(amd64.RSI, amd64.R13)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.MovRegImm64(amd64.R10, 5)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.RSP, 96)
	e.Cvtsi2sd(amd64.XMM2, amd64.R10)
	copyCipher := len(e.Code)
	e.CallRel32(int32(copyOffset - (copyCipher + 5)))

	// Derive per-record nonce and authenticate/decrypt.
	e.MovRegReg(amd64.RDI, amd64.R12)
	e.MovRegDeref(amd64.R10, amd64.RSP, 88)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callNonce := len(e.Code)
	e.CallRel32(int32(nonceOffset - (callNonce + 5)))
	e.MovDerefReg(amd64.RSP, 56, amd64.RAX)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegReg(amd64.RSI, amd64.RAX)
	e.MovRegDeref(amd64.RDX, amd64.RSP, 40)
	e.MovRegDeref(amd64.RCX, amd64.RSP, 48)
	callAEAD := len(e.Code)
	e.CallRel32(int32(aeadOffset - (callAEAD + 5)))
	// RAX is plaintext on success; empty buffer also represents authentication failure.
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	success := len(e.Code)
	e.JmpRel32(0)
	invalid := len(e.Code)
	for _, at := range []int{invalidShort, invalidType, invalidMajor, invalidMinor, invalidLength} {
		patchJcc(at, invalid)
	}
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callEmpty := len(e.Code)
	e.CallRel32(int32(newOffset - (callEmpty + 5)))
	invalidDone := len(e.Code)
	e.JmpRel32(0)

	done := len(e.Code)
	patchJmp(success, done)
	patchJmp(invalidDone, done)
	e.AddRegImm32(amd64.RSP, 112)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
