package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64NetHTTPRequestIPv4 performs one HTTP/1.x exchange against a numeric IPv4 host.
// Reads are nonblocking and cooperatively yield so AbortSignal timers can cancel
// connect/write/read. DNS and TLS remain separate transport layers.
//
// ABI: RDI=host string, RSI=port string, RDX=request ByteBuffer, RCX=AbortSignal -> RAX=response ByteBuffer.
func emitAMD64NetHTTPRequestIPv4(e *amd64.Emitter, byteBufferNewOffset, byteBufferCopyOffset, taskYieldOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 64)
	e.MovRegReg(amd64.RBX, amd64.RDI)       // numeric IPv4 host string
	e.MovDerefReg(amd64.RSP, 16, amd64.RSI) // port string
	e.MovRegReg(amd64.R12, amd64.RDX)       // request buffer
	e.MovDerefReg(amd64.RSP, 24, amd64.RCX) // AbortSignal, survives cooperative yields

	// sockaddr_in family and zero padding. Host bytes are parsed directly into
	// sin_addr so the transport is not tied to 127.0.0.1.
	for off, value := range []byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0} {
		e.MovRegImm64(amd64.R11, int64(value))
		e.MovDerefReg8(amd64.RSP, int32(off), amd64.R11)
	}

	// Parse dotted-decimal IPv4: exactly four non-empty octets, each <= 255.
	e.MovRegDeref(amd64.R8, amd64.RBX, 0) // host length
	e.MovRegImm64(amd64.R9, 0)            // byte index
	e.MovRegImm64(amd64.R10, 0)           // octet index
	e.MovRegImm64(amd64.R13, 0)           // current octet value
	e.MovRegImm64(amd64.R14, 0)           // digits in current octet
	var invalidHost []int
	hostLoop := len(e.Code)
	e.CmpRegReg(amd64.R9, amd64.R8)
	hostEnd := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegReg(amd64.R11, amd64.RBX)
	e.AddRegImm32(amd64.R11, 8)
	e.AddRegReg(amd64.R11, amd64.R9)
	e.MovzxRegDeref8(amd64.RAX, amd64.R11, 0)
	e.CmpRegImm32(amd64.RAX, '.')
	dot := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.CmpRegImm32(amd64.RAX, '0')
	belowDigit := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	invalidHost = append(invalidHost, belowDigit)
	e.CmpRegImm32(amd64.RAX, '9')
	aboveDigit := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	invalidHost = append(invalidHost, aboveDigit)
	e.SubRegImm32(amd64.RAX, '0')
	e.MovRegImm64(amd64.R11, 10)
	e.ImulRegReg(amd64.R13, amd64.R11)
	e.AddRegReg(amd64.R13, amd64.RAX)
	e.CmpRegImm32(amd64.R13, 255)
	octetTooLarge := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	invalidHost = append(invalidHost, octetTooLarge)
	e.AddRegImm32(amd64.R14, 1)
	e.AddRegImm32(amd64.R9, 1)
	hostDigitBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(hostDigitBack, hostLoop)

	dotLabel := len(e.Code)
	patchJcc(dot, dotLabel)
	e.TestRegReg(amd64.R14, amd64.R14)
	emptyOctet := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	invalidHost = append(invalidHost, emptyOctet)
	e.CmpRegImm32(amd64.R10, 3)
	tooManyOctets := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	invalidHost = append(invalidHost, tooManyOctets)
	e.MovRegReg(amd64.R11, amd64.RSP)
	e.AddRegImm32(amd64.R11, 4)
	e.AddRegReg(amd64.R11, amd64.R10)
	e.MovDerefReg8(amd64.R11, 0, amd64.R13)
	e.AddRegImm32(amd64.R10, 1)
	e.MovRegImm64(amd64.R13, 0)
	e.MovRegImm64(amd64.R14, 0)
	e.AddRegImm32(amd64.R9, 1)
	hostDotBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(hostDotBack, hostLoop)

	hostEndLabel := len(e.Code)
	patchJcc(hostEnd, hostEndLabel)
	e.TestRegReg(amd64.R14, amd64.R14)
	missingLastOctet := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	invalidHost = append(invalidHost, missingLastOctet)
	e.CmpRegImm32(amd64.R10, 3)
	notFourOctets := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	invalidHost = append(invalidHost, notFourOctets)
	e.MovRegReg(amd64.R11, amd64.RSP)
	e.AddRegImm32(amd64.R11, 7)
	e.MovDerefReg8(amd64.R11, 0, amd64.R13)

	// Parse decimal port, defaulting empty to 80.
	e.MovRegDeref(amd64.RBX, amd64.RSP, 16)
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

	e.MovRegReg(amd64.R10, amd64.R13)
	e.ShrRegImm8(amd64.R10, 8)
	e.MovRegImm64(amd64.R11, 255)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RSP, 2, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.R13)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RSP, 3, amd64.R10)

	// Make the socket nonblocking before connect/write/read. Slow peers must not
	// pin the cooperative scheduler and starve AbortSignal timers.
	e.MovRegImm64(amd64.RAX, 72) // fcntl
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.MovRegImm64(amd64.RSI, 4)    // F_SETFL
	e.MovRegImm64(amd64.RDX, 2048) // O_NONBLOCK
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	fcntlFailed := len(e.Code)
	e.JccRel32(amd64.CondL, 0)

	connectLoop := len(e.Code)
	e.MovRegDeref(amd64.R10, amd64.RSP, 24)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64AbortSignalAborted)
	e.TestRegReg(amd64.R11, amd64.R11)
	abortedConnect := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	// connect(fd, &sockaddr, 16). EINPROGRESS/EALREADY are cooperative waits;
	// EISCONN means a retried nonblocking connect has completed.
	e.MovRegImm64(amd64.RAX, 42)
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.MovRegReg(amd64.RSI, amd64.RSP)
	e.MovRegImm64(amd64.RDX, 16)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	connectOK := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.CmpRegImm32(amd64.RAX, -106) // EISCONN
	connectIsConn := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	var connectRetry []int
	for _, errno := range []int32{-115, -114, -4} { // EINPROGRESS, EALREADY, EINTR
		e.CmpRegImm32(amd64.RAX, errno)
		at := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		connectRetry = append(connectRetry, at)
	}
	connectFailed := len(e.Code)
	e.JmpRel32(0)
	connectYield := len(e.Code)
	for _, at := range connectRetry {
		patchJcc(at, connectYield)
	}
	callConnectYield := len(e.Code)
	e.CallRel32(int32(taskYieldOffset - (callConnectYield + 5)))
	connectBack := len(e.Code)
	e.JmpRel32(int32(connectLoop - (connectBack + 5)))
	connected := len(e.Code)
	patchJcc(connectOK, connected)
	patchJcc(connectIsConn, connected)

	// Write the complete request. Partial writes and EAGAIN yield rather than
	// silently truncating the request body.
	e.MovRegImm64(amd64.R13, 0) // bytes written
	writeLoop := len(e.Code)
	e.MovRegDeref(amd64.R10, amd64.RSP, 24)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64AbortSignalAborted)
	e.TestRegReg(amd64.R11, amd64.R11)
	abortedWrite := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegDeref(amd64.R10, amd64.R12, amd64ByteBufferLength)
	e.CmpRegReg(amd64.R13, amd64.R10)
	writeComplete := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegImm64(amd64.RAX, 1)
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.MovRegDeref(amd64.RSI, amd64.R12, amd64ByteBufferData)
	e.AddRegReg(amd64.RSI, amd64.R13)
	e.MovRegReg(amd64.RDX, amd64.R10)
	e.SubRegReg(amd64.RDX, amd64.R13)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	writeProgress := len(e.Code)
	e.JccRel32(amd64.CondG, 0)
	var writeRetry []int
	e.CmpRegImm32(amd64.RAX, 0)
	writeZero := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	writeRetry = append(writeRetry, writeZero)
	for _, errno := range []int32{-11, -4} { // EAGAIN/EWOULDBLOCK, EINTR
		e.CmpRegImm32(amd64.RAX, errno)
		at := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		writeRetry = append(writeRetry, at)
	}
	writeFailed := len(e.Code)
	e.JmpRel32(0)
	writeYield := len(e.Code)
	for _, at := range writeRetry {
		patchJcc(at, writeYield)
	}
	callWriteYield := len(e.Code)
	e.CallRel32(int32(taskYieldOffset - (callWriteYield + 5)))
	writeRetryBack := len(e.Code)
	e.JmpRel32(int32(writeLoop - (writeRetryBack + 5)))
	writeProgressLabel := len(e.Code)
	patchJcc(writeProgress, writeProgressLabel)
	e.AddRegReg(amd64.R13, amd64.RAX)
	writeProgressBack := len(e.Code)
	e.JmpRel32(int32(writeLoop - (writeProgressBack + 5)))
	writeDone := len(e.Code)
	patchJcc(writeComplete, writeDone)

	// Start with 64KiB and grow geometrically while reading until EOF.
	e.MovRegImm64(amd64.R10, 64*1024)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callNew := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callNew + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RBX, amd64ByteBufferLength, amd64.R10)
	// Root the response buffer while scheduler yields can run arbitrary tasks/GC.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 32, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 40, amd64.R10)
	e.MovDerefReg(amd64.RSP, 48, amd64.RBX)
	e.MovRegReg(amd64.R10, amd64.RSP)
	e.AddRegImm32(amd64.R10, 32)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)

	e.MovRegImm64(amd64.R13, 0) // response bytes used
	readLoop := len(e.Code)
	// AbortSignal.aborted is a native boolean field. The caller still owns the
	// signal root while this runtime helper cooperatively suspends and resumes.
	e.MovRegDeref(amd64.R10, amd64.RSP, 24)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64AbortSignalAborted)
	e.TestRegReg(amd64.R11, amd64.R11)
	abortedRead := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	// Grow when the buffer is full. R12 is free after request write completion,
	// so it temporarily keeps the old rooted response during replacement.
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferCapacity)
	e.CmpRegReg(amd64.R13, amd64.R10)
	haveCapacity := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovRegReg(amd64.R12, amd64.RBX)
	e.AddRegReg(amd64.R10, amd64.R10)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callGrow := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callGrow + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	// The precise root must follow the replacement before any subsequent yield.
	e.MovDerefReg(amd64.RSP, 48, amd64.RBX)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegReg(amd64.RSI, amd64.R12)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.Cvtsi2sd(amd64.XMM2, amd64.R13)
	callCopy := len(e.Code)
	e.CallRel32(int32(byteBufferCopyOffset - (callCopy + 5)))
	e.MovDerefReg(amd64.RBX, amd64ByteBufferLength, amd64.R13)
	haveCapacityLabel := len(e.Code)
	patchJcc(haveCapacity, haveCapacityLabel)

	// read(fd, response.data+used, capacity-used)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferCapacity)
	e.MovRegReg(amd64.RDX, amd64.R10)
	e.SubRegReg(amd64.RDX, amd64.R13)
	e.MovRegImm64(amd64.RAX, 0)
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.MovRegDeref(amd64.RSI, amd64.RBX, amd64ByteBufferData)
	e.AddRegReg(amd64.RSI, amd64.R13)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	readProgress := len(e.Code)
	e.JccRel32(amd64.CondG, 0)
	readEOF := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	// EAGAIN/EWOULDBLOCK or EINTR: yield so timers/microtasks can run, then retry.
	e.CmpRegImm32(amd64.RAX, -11)
	wouldBlock := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.CmpRegImm32(amd64.RAX, -4)
	interrupted := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	// Other read failures return the bytes already received.
	readFailure := len(e.Code)
	e.JmpRel32(0)
	yieldLabel := len(e.Code)
	patchJcc(wouldBlock, yieldLabel)
	patchJcc(interrupted, yieldLabel)
	callYield := len(e.Code)
	e.CallRel32(int32(taskYieldOffset - (callYield + 5)))
	retryRead := len(e.Code)
	e.JmpRel32(int32(readLoop - (retryRead + 5)))
	readProgressLabel := len(e.Code)
	patchJcc(readProgress, readProgressLabel)
	e.AddRegReg(amd64.R13, amd64.RAX)
	e.MovDerefReg(amd64.RBX, amd64ByteBufferLength, amd64.R13)
	readMore := len(e.Code)
	e.JmpRel32(int32(readLoop - (readMore + 5)))
	readDone := len(e.Code)
	patchJcc(readEOF, readDone)
	patchJmp(readFailure, readDone)
	e.MovDerefReg(amd64.RBX, amd64ByteBufferLength, amd64.R13)
	responseCloseJump := len(e.Code)
	e.JmpRel32(0)
	abortedLabel := len(e.Code)
	patchJcc(abortedRead, abortedLabel)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RBX, amd64ByteBufferLength, amd64.R10)
	responseClose := len(e.Code)
	patchJmp(responseCloseJump, responseClose)
	// Pop the runtime-local precise root before returning to the caller frame.
	e.MovRegDeref(amd64.R10, amd64.RSP, 32)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovRegImm64(amd64.RAX, 3)
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.Syscall()
	e.MovRegReg(amd64.RAX, amd64.RBX)
	returnJump := len(e.Code)
	e.JmpRel32(0)

	// Failures/aborts before response allocation close the fd and return an empty buffer.
	failureWithFD := len(e.Code)
	patchJcc(fcntlFailed, failureWithFD)
	patchJcc(abortedConnect, failureWithFD)
	patchJmp(connectFailed, failureWithFD)
	patchJcc(abortedWrite, failureWithFD)
	patchJmp(writeFailed, failureWithFD)
	e.MovRegImm64(amd64.RAX, 3)
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.Syscall()
	failureNoFD := len(e.Code)
	for _, at := range invalidHost {
		patchJcc(at, failureNoFD)
	}
	patchJcc(socketFailed, failureNoFD)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callEmpty := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callEmpty + 5)))

	returnLabel := len(e.Code)
	patchJmp(returnJump, returnLabel)
	e.AddRegImm32(amd64.RSP, 64)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
