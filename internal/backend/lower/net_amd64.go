package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64NetHTTPRequestLoopback performs one blocking HTTP/1.x exchange against
// 127.0.0.1. It is deliberately a narrow transport primitive for the deterministic
// fetch integration harness; DNS, TLS, redirects, streaming and cancellation live
// above/beside this primitive rather than being smuggled into one giant syscall blob.
//
// ABI: RDI=port string, RSI=request ByteBuffer -> RAX=response ByteBuffer.
func emitAMD64NetHTTPRequestLoopback(e *amd64.Emitter, byteBufferNewOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 32)
	e.MovRegReg(amd64.RBX, amd64.RDI) // port string
	e.MovRegReg(amd64.R12, amd64.RSI) // request buffer

	// Parse decimal port, defaulting empty to 80.
	e.MovRegDeref(amd64.R10, amd64.RBX, 0)
	e.MovRegImm64(amd64.R13, 0)
	e.MovRegImm64(amd64.R14, 0)
	e.TestRegReg(amd64.R10, amd64.R10)
	portLoopStartJump := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.R13, 80)
	portDoneJump := len(e.Code)
	e.JmpRel32(0)
	portLoop := len(e.Code)
	patchJcc(portLoopStartJump, portLoop)
	e.CmpRegReg(amd64.R14, amd64.R10)
	portParsed := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegReg(amd64.R11, amd64.RBX)
	e.AddRegImm32(amd64.R11, 8)
	e.AddRegReg(amd64.R11, amd64.R14)
	e.MovzxRegDeref8(amd64.R11, amd64.R11, 0)
	e.SubRegImm32(amd64.R11, 48)
	e.MovRegImm64(amd64.RAX, 10)
	e.ImulRegReg(amd64.R13, amd64.RAX)
	e.AddRegReg(amd64.R13, amd64.R11)
	e.AddRegImm32(amd64.R14, 1)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, portLoop)
	portReady := len(e.Code)
	patchJcc(portParsed, portReady)
	patchJmp(portDoneJump, portReady)

	// socket(AF_INET, SOCK_STREAM, 0)
	e.MovRegImm64(amd64.RAX, 41)
	e.MovRegImm64(amd64.RDI, 2)
	e.MovRegImm64(amd64.RSI, 1)
	e.MovRegImm64(amd64.RDX, 0)
	e.Syscall()
	e.MovRegReg(amd64.R14, amd64.RAX) // fd
	e.TestRegReg(amd64.R14, amd64.R14)
	socketFailed := len(e.Code)
	e.JccRel32(amd64.CondL, 0)

	// sockaddr_in on stack: AF_INET, big-endian port, 127.0.0.1.
	for off, value := range []byte{2, 0, 0, 0, 127, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0} {
		e.MovRegImm64(amd64.R11, int64(value))
		e.MovDerefReg8(amd64.RSP, int32(off), amd64.R11)
	}
	e.MovRegReg(amd64.R10, amd64.R13)
	e.ShrRegImm8(amd64.R10, 8)
	e.MovRegImm64(amd64.R11, 255)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RSP, 2, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.R13)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RSP, 3, amd64.R10)

	// connect(fd, &sockaddr, 16)
	e.MovRegImm64(amd64.RAX, 42)
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.MovRegReg(amd64.RSI, amd64.RSP)
	e.MovRegImm64(amd64.RDX, 16)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	connectFailed := len(e.Code)
	e.JccRel32(amd64.CondL, 0)

	// write(fd, request.data, request.length)
	e.MovRegImm64(amd64.RAX, 1)
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.MovRegDeref(amd64.RSI, amd64.R12, amd64ByteBufferData)
	e.MovRegDeref(amd64.RDX, amd64.R12, amd64ByteBufferLength)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	writeFailed := len(e.Code)
	e.JccRel32(amd64.CondL, 0)

	// Allocate a bounded receive buffer. The transport layer will become streaming
	// before general network fetch conformance is claimed.
	e.MovRegImm64(amd64.R10, 64*1024)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callNew := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callNew + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)

	// read(fd, response.data, 64KiB)
	e.MovRegImm64(amd64.RAX, 0)
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.MovRegDeref(amd64.RSI, amd64.RBX, amd64ByteBufferData)
	e.MovRegImm64(amd64.RDX, 64*1024)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	readOK := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.RAX, 0)
	patchJcc(readOK, len(e.Code))
	e.MovDerefReg(amd64.RBX, amd64ByteBufferLength, amd64.RAX)
	closeAndReturn := len(e.Code)
	e.MovRegImm64(amd64.RAX, 3)
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.Syscall()
	e.MovRegReg(amd64.RAX, amd64.RBX)
	returnJump := len(e.Code)
	e.JmpRel32(0)

	// Failures after socket creation close the fd and return an empty buffer.
	failureWithFD := len(e.Code)
	patchJcc(connectFailed, failureWithFD)
	patchJcc(writeFailed, failureWithFD)
	e.MovRegImm64(amd64.RAX, 3)
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.Syscall()
	failureNoFD := len(e.Code)
	patchJcc(socketFailed, failureNoFD)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callEmpty := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callEmpty + 5)))

	returnLabel := len(e.Code)
	patchJmp(returnJump, returnLabel)
	_ = closeAndReturn
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
