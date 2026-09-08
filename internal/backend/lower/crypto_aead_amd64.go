package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// AES-128-GCM is provided by Linux AF_ALG so TLS can use the kernel's
// accelerated AEAD implementation while the generated ELF remains libc-free.
// ABI for both operations: RDI=key(16), RSI=iv(12), RDX=AAD, RCX=input.
func emitAMD64AES128GCMEncrypt(e *amd64.Emitter, newOffset, copyOffset int) {
	emitAMD64AES128GCM(e, true, newOffset, copyOffset)
}

func emitAMD64AES128GCMDecrypt(e *amd64.Emitter, newOffset, copyOffset int) {
	emitAMD64AES128GCM(e, false, newOffset, copyOffset)
}

func emitAMD64AES128GCM(e *amd64.Emitter, encrypt bool, newOffset, copyOffset int) {
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
	e.SubRegImm32(amd64.RSP, 384)
	e.MovRegReg(amd64.RBX, amd64.RDI) // key
	e.MovRegReg(amd64.R12, amd64.RSI) // IV
	e.MovRegReg(amd64.R13, amd64.RDX) // AAD
	e.MovRegReg(amd64.R14, amd64.RCX) // plaintext or ciphertext||tag

	// TLS_AES_128_GCM_SHA256 requires a 16-byte key and 12-byte nonce.
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferLength)
	e.CmpRegImm32(amd64.R10, 16)
	invalidKey := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegDeref(amd64.R10, amd64.R12, amd64ByteBufferLength)
	e.CmpRegImm32(amd64.R10, 12)
	invalidIV := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	invalidCiphertext := -1
	if !encrypt {
		e.MovRegDeref(amd64.R10, amd64.R14, amd64ByteBufferLength)
		e.CmpRegImm32(amd64.R10, 16)
		invalidCiphertext = len(e.Code)
		e.JccRel32(amd64.CondB, 0)
		e.MovDerefReg(amd64.RSP, 376, amd64.R10)
	}
	// Six precise roots survive ByteBuffer allocations around the syscall path.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 6)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.RSP, 32, amd64.R13)
	e.MovDerefReg(amd64.RSP, 40, amd64.R14)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RSP, 48, amd64.R10)
	e.MovDerefReg(amd64.RSP, 56, amd64.R10)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)
	e.MovRegImm64(amd64.R10, -1)
	e.MovDerefReg(amd64.RSP, 64, amd64.R10) // parent AF_ALG fd
	e.MovDerefReg(amd64.RSP, 72, amd64.R10) // operation fd

	e.MovRegDeref(amd64.R8, amd64.R13, amd64ByteBufferLength)
	e.MovRegDeref(amd64.R9, amd64.R14, amd64ByteBufferLength)
	if encrypt {
		e.AddRegImm32(amd64.R9, 16)
	} else {
		e.SubRegImm32(amd64.R9, 16)
	}
	e.MovDerefReg(amd64.RSP, 80, amd64.R9) // final output length
	e.AddRegReg(amd64.R8, amd64.R9)
	e.MovDerefReg(amd64.RSP, 88, amd64.R8) // raw AF_ALG output includes AAD prefix
	e.Cvtsi2sd(amd64.XMM0, amd64.R8)
	callRaw := len(e.Code)
	e.CallRel32(int32(newOffset - (callRaw + 5)))
	e.MovDerefReg(amd64.RSP, 48, amd64.RAX)

	// sockaddr_alg { AF_ALG, "aead", 0, 0, "gcm(aes)" } at rsp+96.
	e.MovRegImm64(amd64.R10, 0)
	for off := int32(96); off < 184; off += 8 {
		e.MovDerefReg(amd64.RSP, off, amd64.R10)
	}
	storeByte := func(off int32, value byte) {
		e.MovRegImm64(amd64.R11, int64(value))
		e.MovDerefReg8(amd64.RSP, off, amd64.R11)
	}
	storeByte(96, 38) // AF_ALG
	for i, b := range []byte("aead") {
		storeByte(98+int32(i), b)
	}
	for i, b := range []byte("gcm(aes)") {
		storeByte(120+int32(i), b)
	}

	// socket(AF_ALG, SOCK_SEQPACKET, 0)
	e.MovRegImm64(amd64.RAX, 41)
	e.MovRegImm64(amd64.RDI, 38)
	e.MovRegImm64(amd64.RSI, 5)
	e.MovRegImm64(amd64.RDX, 0)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	socketFailed := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	e.MovDerefReg(amd64.RSP, 64, amd64.RAX)

	// bind(parent, &sockaddr_alg, 88)
	e.MovRegImm64(amd64.RAX, 49)
	e.MovRegDeref(amd64.RDI, amd64.RSP, 64)
	e.MovRegReg(amd64.RSI, amd64.RSP)
	e.AddRegImm32(amd64.RSI, 96)
	e.MovRegImm64(amd64.RDX, 88)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	bindFailed := len(e.Code)
	e.JccRel32(amd64.CondL, 0)

	// setsockopt(parent, SOL_ALG, ALG_SET_KEY, key, 16)
	e.MovRegImm64(amd64.RAX, 54)
	e.MovRegDeref(amd64.RDI, amd64.RSP, 64)
	e.MovRegImm64(amd64.RSI, 279)
	e.MovRegImm64(amd64.RDX, 1)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferData)
	e.MovRegImm64(amd64.R8, 16)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	keyFailed := len(e.Code)
	e.JccRel32(amd64.CondL, 0)

	// For AF_ALG AEAD the authentication size is encoded in optlen.
	e.MovRegImm64(amd64.RAX, 54)
	e.MovRegDeref(amd64.RDI, amd64.RSP, 64)
	e.MovRegImm64(amd64.RSI, 279)
	e.MovRegImm64(amd64.RDX, 5)
	e.MovRegImm64(amd64.R10, 0)
	e.MovRegImm64(amd64.R8, 16)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	authSizeFailed := len(e.Code)
	e.JccRel32(amd64.CondL, 0)

	// accept(parent, NULL, NULL) creates the operation socket.
	e.MovRegImm64(amd64.RAX, 43)
	e.MovRegDeref(amd64.RDI, amd64.RSP, 64)
	e.MovRegImm64(amd64.RSI, 0)
	e.MovRegImm64(amd64.RDX, 0)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	acceptFailed := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	e.MovDerefReg(amd64.RSP, 72, amd64.RAX)
	// Two iovecs: AAD followed by plaintext or ciphertext||tag.
	e.MovRegDeref(amd64.R10, amd64.R13, amd64ByteBufferData)
	e.MovDerefReg(amd64.RSP, 192, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.R13, amd64ByteBufferLength)
	e.MovDerefReg(amd64.RSP, 200, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.R14, amd64ByteBufferData)
	e.MovDerefReg(amd64.RSP, 208, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.R14, amd64ByteBufferLength)
	e.MovDerefReg(amd64.RSP, 216, amd64.R10)

	// Zero msghdr and control storage before filling pointer fields.
	e.MovRegImm64(amd64.R10, 0)
	for off := int32(224); off < 280; off += 8 {
		e.MovDerefReg(amd64.RSP, off, amd64.R10)
	}
	for off := int32(288); off < 368; off += 8 {
		e.MovDerefReg(amd64.RSP, off, amd64.R10)
	}
	e.MovRegReg(amd64.R10, amd64.RSP)
	e.AddRegImm32(amd64.R10, 192)
	e.MovDerefReg(amd64.RSP, 240, amd64.R10)
	e.MovRegImm64(amd64.R10, 2)
	e.MovDerefReg(amd64.RSP, 248, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.RSP)
	e.AddRegImm32(amd64.R10, 288)
	e.MovDerefReg(amd64.RSP, 256, amd64.R10)
	e.MovRegImm64(amd64.R10, 80)
	e.MovDerefReg(amd64.RSP, 264, amd64.R10)

	// cmsg #1: ALG_SET_OP, payload u32 encrypt/decrypt.
	e.MovRegImm64(amd64.R10, 20)
	e.MovDerefReg(amd64.RSP, 288, amd64.R10)
	e.MovRegImm32(amd64.R10, 279)
	e.MovDerefReg32(amd64.RSP, 296, amd64.R10)
	e.MovRegImm32(amd64.R10, 3)
	e.MovDerefReg32(amd64.RSP, 300, amd64.R10)
	if encrypt {
		e.MovRegImm32(amd64.R10, 1)
	} else {
		e.MovRegImm32(amd64.R10, 0)
	}
	e.MovDerefReg32(amd64.RSP, 304, amd64.R10)

	// cmsg #2: ALG_SET_IV containing af_alg_iv{12, iv[12]}.
	e.MovRegImm64(amd64.R10, 32)
	e.MovDerefReg(amd64.RSP, 312, amd64.R10)
	e.MovRegImm32(amd64.R10, 279)
	e.MovDerefReg32(amd64.RSP, 320, amd64.R10)
	e.MovRegImm32(amd64.R10, 2)
	e.MovDerefReg32(amd64.RSP, 324, amd64.R10)
	e.MovRegImm32(amd64.R10, 12)
	e.MovDerefReg32(amd64.RSP, 328, amd64.R10)
	e.MovRegDeref(amd64.R11, amd64.R12, amd64ByteBufferData)
	for i := int32(0); i < 12; i++ {
		e.MovzxRegDeref8(amd64.R10, amd64.R11, i)
		e.MovDerefReg8(amd64.RSP, 332+i, amd64.R10)
	}

	// cmsg #3: ALG_SET_AEAD_ASSOCLEN.
	e.MovRegImm64(amd64.R10, 20)
	e.MovDerefReg(amd64.RSP, 344, amd64.R10)
	e.MovRegImm32(amd64.R10, 279)
	e.MovDerefReg32(amd64.RSP, 352, amd64.R10)
	e.MovRegImm32(amd64.R10, 4)
	e.MovDerefReg32(amd64.RSP, 356, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.R13, amd64ByteBufferLength)
	e.MovDerefReg32(amd64.RSP, 360, amd64.R10)

	// sendmsg(operationFD, &msghdr, 0)
	e.MovRegImm64(amd64.RAX, 46)
	e.MovRegDeref(amd64.RDI, amd64.RSP, 72)
	e.MovRegReg(amd64.RSI, amd64.RSP)
	e.AddRegImm32(amd64.RSI, 224)
	e.MovRegImm64(amd64.RDX, 0)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	sendFailed := len(e.Code)
	e.JccRel32(amd64.CondL, 0)

	// read() performs the AEAD operation. Decryption authentication failure is
	// returned as -EBADMSG and follows the same fail-closed path.
	e.MovRegImm64(amd64.RAX, 0)
	e.MovRegDeref(amd64.RDI, amd64.RSP, 72)
	e.MovRegDeref(amd64.R10, amd64.RSP, 48)
	e.MovRegDeref(amd64.RSI, amd64.R10, amd64ByteBufferData)
	e.MovRegDeref(amd64.RDX, amd64.RSP, 88)
	e.Syscall()
	e.MovRegDeref(amd64.R10, amd64.RSP, 88)
	e.CmpRegReg(amd64.RAX, amd64.R10)
	readFailed := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	// The kernel operation is complete. Close both AF_ALG descriptors before
	// allocating the user-visible result.
	e.MovRegImm64(amd64.RAX, 3)
	e.MovRegDeref(amd64.RDI, amd64.RSP, 72)
	e.Syscall()
	e.MovRegImm64(amd64.RAX, 3)
	e.MovRegDeref(amd64.RDI, amd64.RSP, 64)
	e.Syscall()
	e.MovRegImm64(amd64.R10, -1)
	e.MovDerefReg(amd64.RSP, 72, amd64.R10)
	e.MovDerefReg(amd64.RSP, 64, amd64.R10)
	// Strip the AAD prefix observed on AF_ALG AEAD output on Linux.
	e.MovRegDeref(amd64.R10, amd64.RSP, 80)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callResult := len(e.Code)
	e.CallRel32(int32(newOffset - (callResult + 5)))
	e.MovDerefReg(amd64.RSP, 56, amd64.RAX)
	e.MovRegReg(amd64.RDI, amd64.RAX)
	e.MovRegDeref(amd64.RSI, amd64.RSP, 48)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.R13, amd64ByteBufferLength)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.RSP, 80)
	e.Cvtsi2sd(amd64.XMM2, amd64.R10)
	copyResult := len(e.Code)
	e.CallRel32(int32(copyOffset - (copyResult + 5)))
	e.MovRegDeref(amd64.RAX, amd64.RSP, 56)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	successDone := len(e.Code)
	e.JmpRel32(0)

	rootedFailure := len(e.Code)
	for _, at := range []int{socketFailed, bindFailed, keyFailed, authSizeFailed, acceptFailed, sendFailed, readFailed} {
		patchJcc(at, rootedFailure)
	}
	// Close operation fd when it was created.
	e.MovRegDeref(amd64.RDI, amd64.RSP, 72)
	e.TestRegReg(amd64.RDI, amd64.RDI)
	skipOpClose := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	e.MovRegImm64(amd64.RAX, 3)
	e.Syscall()
	patchJcc(skipOpClose, len(e.Code))
	// Close parent fd when socket() succeeded.
	e.MovRegDeref(amd64.RDI, amd64.RSP, 64)
	e.TestRegReg(amd64.RDI, amd64.RDI)
	skipParentClose := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	e.MovRegImm64(amd64.RAX, 3)
	e.Syscall()
	patchJcc(skipParentClose, len(e.Code))

	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callRootedEmpty := len(e.Code)
	e.CallRel32(int32(newOffset - (callRootedEmpty + 5)))
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	failureDone := len(e.Code)
	e.JmpRel32(0)

	invalid := len(e.Code)
	patchJcc(invalidKey, invalid)
	patchJcc(invalidIV, invalid)
	if invalidCiphertext >= 0 {
		patchJcc(invalidCiphertext, invalid)
	}
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callInvalidEmpty := len(e.Code)
	e.CallRel32(int32(newOffset - (callInvalidEmpty + 5)))

	epilogue := len(e.Code)
	patchJmp(successDone, epilogue)
	patchJmp(failureDone, epilogue)
	e.AddRegImm32(amd64.RSP, 384)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
