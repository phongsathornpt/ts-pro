package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64PrintValV2 formats the binary64 value in XMM0 using a normalized
// 17-significant-digit decimal path. It follows ECMAScript's fixed/scientific
// display thresholds and preserves NaN, infinities, and signed zero.
func emitAMD64PrintValV2(e *amd64.Emitter) {
	var candidateCalls []int
	emitCandidateCall := func() {
		at := len(e.Code)
		e.CallRel32(0)
		candidateCalls = append(candidateCalls, at)
	}
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}
	emitByte := func(ptr amd64.Register, ch byte) {
		e.MovRegImm64(amd64.RDX, int64(ch))
		e.MovDerefReg8(ptr, 0, amd64.RDX)
		e.AddRegImm32(ptr, 1)
	}
	emitCopy := func(src, count amd64.Register) {
		loop := len(e.Code)
		e.MovzxRegDeref8(amd64.RAX, src, 0)
		e.MovDerefReg8(amd64.R8, 0, amd64.RAX)
		e.AddRegImm32(src, 1)
		e.AddRegImm32(amd64.R8, 1)
		e.SubRegImm32(count, 1)
		back := len(e.Code)
		e.JccRel32(amd64.CondNE, 0)
		patchJcc(back, loop)
	}
	emitZeroes := func(count amd64.Register) {
		e.TestRegReg(count, count)
		done := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		loop := len(e.Code)
		emitByte(amd64.R8, '0')
		e.SubRegImm32(count, 1)
		back := len(e.Code)
		e.JccRel32(amd64.CondNE, 0)
		patchJcc(back, loop)
		patchJcc(done, len(e.Code))
	}
	emitReturn := func() {
		e.MovRegReg(amd64.RDX, amd64.R8)
		e.SubRegReg(amd64.RDX, amd64.R9)
		e.MovRegImm64(amd64.RDI, 1)
		e.MovRegReg(amd64.RSI, amd64.R9)
		e.MovRegImm64(amd64.RAX, 1)
		e.Syscall()
		e.MovRegReg(amd64.RSP, amd64.RBP)
		e.Pop(amd64.RBP)
		e.Ret()
	}

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 256)

	// Keep raw IEEE-754 bits in RAX for special-value/sign handling.
	e.MovQRegXMM(amd64.RAX, amd64.XMM0)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, int64(0x7ff0000000000000))
	e.AndRegReg(amd64.R10, amd64.R11)
	e.CmpRegReg(amd64.R10, amd64.R11)
	specialJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// Output buffer is [RBP-224, RBP-96); decimal digits use [RBP-64, ...].
	e.MovRegReg(amd64.R8, amd64.RBP)
	e.SubRegImm32(amd64.R8, 224)
	e.MovRegReg(amd64.R9, amd64.R8)

	// Emit sign first, including -0, then clear it from the working payload.
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, int64(-0x8000000000000000))
	e.AndRegReg(amd64.R10, amd64.R11)
	e.TestRegReg(amd64.R10, amd64.R10)
	noSign := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitByte(amd64.R8, '-')
	patchJcc(noSign, len(e.Code))

	e.MovRegImm64(amd64.R11, int64(0x7fffffffffffffff))
	e.AndRegReg(amd64.RAX, amd64.R11)
	e.MovQXMMReg(amd64.XMM0, amd64.RAX)
	e.MovDerefReg(amd64.RBP, -8, amd64.RAX) // original absolute F64 bits

	// Zero is special because the normalization loops require a positive value.
	e.XorPD(amd64.XMM1, amd64.XMM1)
	e.Ucomisd(amd64.XMM0, amd64.XMM1)
	nonZero := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	emitByte(amd64.R8, '0')
	emitByte(amd64.R8, '\n')
	emitReturn()
	patchJcc(nonZero, len(e.Code))

	// Exact integers through 2^53 have a unique decimal integer spelling. Keep
	// them off the normalized floating path so arithmetic results such as 21 or
	// 6765 never grow decimal noise from repeated scaling.
	e.MovRegImm64(amd64.R11, int64(0x4340000000000000)) // 2^53
	e.MovQXMMReg(amd64.XMM4, amd64.R11)
	e.Ucomisd(amd64.XMM0, amd64.XMM4)
	notSafeMagnitude := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	e.Cvttsd2si(amd64.RAX, amd64.XMM0)
	e.Cvtsi2sd(amd64.XMM1, amd64.RAX)
	e.Ucomisd(amd64.XMM0, amd64.XMM1)
	notExactInteger := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	// Build the exact integer backwards below RBP and copy it to the output.
	e.MovRegReg(amd64.RSI, amd64.RBP)
	e.SubRegImm32(amd64.RSI, 1)
	e.MovRegImm64(amd64.R11, 0)
	integerFastLoop := len(e.Code)
	e.MovRegImm64(amd64.R10, 10)
	e.Cqo()
	e.IdivReg(amd64.R10)
	e.AddRegImm32(amd64.RDX, '0')
	e.SubRegImm32(amd64.RSI, 1)
	e.MovDerefReg8(amd64.RSI, 0, amd64.RDX)
	e.AddRegImm32(amd64.R11, 1)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	integerFastBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(integerFastBack, integerFastLoop)
	emitCopy(amd64.RSI, amd64.R11)
	emitByte(amd64.R8, '\n')
	emitReturn()

	floatingPath := len(e.Code)
	patchJcc(notSafeMagnitude, floatingPath)
	patchJcc(notExactInteger, floatingPath)

	// Normalize XMM0 to [1,10), tracking the base-10 exponent in RCX.
	e.MovRegImm64(amd64.R11, int64(0x4024000000000000)) // 10.0
	e.MovQXMMReg(amd64.XMM2, amd64.R11)
	e.MovRegImm64(amd64.R11, int64(0x3ff0000000000000)) // 1.0
	e.MovQXMMReg(amd64.XMM3, amd64.R11)
	e.MovRegImm64(amd64.RCX, 0)

	e.Ucomisd(amd64.XMM0, amd64.XMM2)
	checkSmall := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	largeLoop := len(e.Code)
	e.DivSD(amd64.XMM0, amd64.XMM2)
	e.AddRegImm32(amd64.RCX, 1)
	e.Ucomisd(amd64.XMM0, amd64.XMM2)
	largeBack := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	patchJcc(largeBack, largeLoop)
	largeDone := len(e.Code)
	e.JmpRel32(0)

	smallCheckLabel := len(e.Code)
	patchJcc(checkSmall, smallCheckLabel)
	e.Ucomisd(amd64.XMM0, amd64.XMM3)
	smallDone := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	smallLoop := len(e.Code)
	e.MulSD(amd64.XMM0, amd64.XMM2)
	e.SubRegImm32(amd64.RCX, 1)
	e.Ucomisd(amd64.XMM0, amd64.XMM3)
	smallBack := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	patchJcc(smallBack, smallLoop)

	normalized := len(e.Code)
	patchJmp(largeDone, normalized)
	patchJcc(smallDone, normalized)

	// Generate 17 significant digits plus one guard digit.
	e.MovRegReg(amd64.RSI, amd64.RBP)
	e.SubRegImm32(amd64.RSI, 64)
	e.MovRegImm64(amd64.RDI, 18)
	digitLoop := len(e.Code)
	e.Cvttsd2si(amd64.RAX, amd64.XMM0)
	// Clamp tiny extraction drift into [0,9].
	e.CmpRegImm32(amd64.RAX, 0)
	digitNonNegative := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.RAX, 0)
	patchJcc(digitNonNegative, len(e.Code))
	e.CmpRegImm32(amd64.RAX, 9)
	digitAtMostNine := len(e.Code)
	e.JccRel32(amd64.CondLE, 0)
	e.MovRegImm64(amd64.RAX, 9)
	patchJcc(digitAtMostNine, len(e.Code))
	e.MovRegReg(amd64.RDX, amd64.RAX)
	e.AddRegImm32(amd64.RDX, '0')
	e.MovDerefReg8(amd64.RSI, 0, amd64.RDX)
	e.AddRegImm32(amd64.RSI, 1)
	e.Cvtsi2sd(amd64.XMM1, amd64.RAX)
	e.SubSD(amd64.XMM0, amd64.XMM1)
	e.MulSD(amd64.XMM0, amd64.XMM2)
	e.SubRegImm32(amd64.RDI, 1)
	digitBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(digitBack, digitLoop)

	// Round the 17th digit using the guard digit.
	e.MovzxRegDeref8(amd64.RAX, amd64.RBP, -47)
	e.CmpRegImm32(amd64.RAX, '5')
	noRound := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovRegReg(amd64.RSI, amd64.RBP)
	e.SubRegImm32(amd64.RSI, 48)
	e.MovRegImm64(amd64.RDI, 17)
	carryLoop := len(e.Code)
	e.MovzxRegDeref8(amd64.RAX, amd64.RSI, 0)
	e.CmpRegImm32(amd64.RAX, '9')
	incrementDigit := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.RAX, '0')
	e.MovDerefReg8(amd64.RSI, 0, amd64.RAX)
	e.SubRegImm32(amd64.RSI, 1)
	e.SubRegImm32(amd64.RDI, 1)
	carryBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(carryBack, carryLoop)
	// 9.999... rounded to 10.000... advances the decimal exponent.
	e.MovRegImm64(amd64.RAX, '1')
	e.MovDerefReg8(amd64.RBP, -64, amd64.RAX)
	e.AddRegImm32(amd64.RCX, 1)
	roundDoneJump := len(e.Code)
	e.JmpRel32(0)
	incrementLabel := len(e.Code)
	patchJcc(incrementDigit, incrementLabel)
	e.AddRegImm32(amd64.RAX, 1)
	e.MovDerefReg8(amd64.RSI, 0, amd64.RAX)
	roundDone := len(e.Code)
	patchJmp(roundDoneJump, roundDone)
	patchJcc(noRound, roundDone)

	// Search shortest digit counts from 1 through 17. For each count the local
	// verifier records every decimal that rounds back to the original binary64
	// and retains the one with the smallest exact x87 distance.
	e.MovDerefReg(amd64.RBP, -32, amd64.RCX) // decimal exponent
	e.MovRegImm64(amd64.RAX, 0)
	e.MovDerefReg(amd64.RBP, -72, amd64.RAX) // prefix
	e.MovDerefReg(amd64.RBP, -80, amd64.RAX) // digit count
	e.MovRegReg(amd64.RSI, amd64.RBP)
	e.SubRegImm32(amd64.RSI, 64)
	searchLoop := len(e.Code)
	e.MovRegDeref(amd64.RAX, amd64.RBP, -72)
	e.MovRegImm64(amd64.R10, 10)
	e.ImulRegReg(amd64.RAX, amd64.R10)
	e.MovzxRegDeref8(amd64.RDX, amd64.RSI, 0)
	e.SubRegImm32(amd64.RDX, '0')
	e.AddRegReg(amd64.RAX, amd64.RDX)
	e.MovDerefReg(amd64.RBP, -72, amd64.RAX)
	e.AddRegImm32(amd64.RSI, 1)
	e.MovDerefReg(amd64.RBP, -88, amd64.RSI)
	e.MovRegDeref(amd64.R11, amd64.RBP, -80)
	e.AddRegImm32(amd64.R11, 1)
	e.MovDerefReg(amd64.RBP, -80, amd64.R11)
	// Reset best candidate for this significant-digit count.
	e.MovRegImm64(amd64.RAX, 0)
	e.MovDerefReg(amd64.RBP, -104, amd64.RAX)

	for _, delta := range []int32{0, -1, 1, -2, 2, -3, 3, -4, 4, -5, 5, -6, 6, -7, 7, -8, 8} {
		e.MovRegDeref(amd64.RDI, amd64.RBP, -72)
		if delta > 0 {
			e.AddRegImm32(amd64.RDI, delta)
		}
		if delta < 0 {
			e.SubRegImm32(amd64.RDI, -delta)
		}
		e.MovRegDeref(amd64.RSI, amd64.RBP, -80)
		emitCandidateCall()
	}

	// If any candidate at this digit count round-tripped, this is the shortest
	// count by construction. Use the closest candidate selected by the helper.
	e.MovRegDeref(amd64.RAX, amd64.RBP, -104)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	foundCandidateJump := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegDeref(amd64.RSI, amd64.RBP, -88)
	e.MovRegDeref(amd64.R11, amd64.RBP, -80)
	e.CmpRegImm32(amd64.R11, 17)
	searchBack := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	patchJcc(searchBack, searchLoop)
	noCandidateJump := len(e.Code)
	e.JmpRel32(0)

	foundCandidate := len(e.Code)
	patchJcc(foundCandidateJump, foundCandidate)
	e.MovRegDeref(amd64.RDI, amd64.RBP, -112)
	// Rewrite the significant-digit buffer from the selected exact integer.
	e.MovRegDeref(amd64.R11, amd64.RBP, -80)
	e.MovRegReg(amd64.RAX, amd64.RDI)
	e.MovRegReg(amd64.RSI, amd64.RBP)
	e.SubRegImm32(amd64.RSI, 64)
	e.AddRegReg(amd64.RSI, amd64.R11)
	rewriteLoop := len(e.Code)
	e.MovRegImm64(amd64.R10, 10)
	e.Cqo()
	e.IdivReg(amd64.R10)
	e.AddRegImm32(amd64.RDX, '0')
	e.SubRegImm32(amd64.RSI, 1)
	e.MovDerefReg8(amd64.RSI, 0, amd64.RDX)
	e.SubRegImm32(amd64.R11, 1)
	rewriteBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(rewriteBack, rewriteLoop)
	e.MovRegDeref(amd64.RDI, amd64.RBP, -80)
	e.MovRegDeref(amd64.RCX, amd64.RBP, -32)
	candidateDoneJump := len(e.Code)
	e.JmpRel32(0)

	// A 17-digit round-tripping representation always exists for binary64. The
	// fallback remains defensive if a future candidate-search change regresses.
	noCandidate := len(e.Code)
	patchJmp(noCandidateJump, noCandidate)
	e.MovRegImm64(amd64.RDI, 17)
	e.MovRegDeref(amd64.RCX, amd64.RBP, -32)
	digitsReady := len(e.Code)
	patchJmp(candidateDoneJump, digitsReady)

	// ECMAScript switches to exponential notation at exponent >= 21 or <= -7.
	e.CmpRegImm32(amd64.RCX, 21)
	toScientificHigh := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.CmpRegImm32(amd64.RCX, -7)
	toScientificLow := len(e.Code)
	e.JccRel32(amd64.CondLE, 0)

	// Fixed notation. decimalPos = exponent + 1.
	e.MovRegReg(amd64.R10, amd64.RCX)
	e.AddRegImm32(amd64.R10, 1)
	e.CmpRegImm32(amd64.R10, 0)
	fixedLeadingZero := len(e.Code)
	e.JccRel32(amd64.CondLE, 0)
	e.CmpRegReg(amd64.R10, amd64.RDI)
	fixedInteger := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)

	// Decimal point falls inside the significant digits.
	e.MovRegReg(amd64.RSI, amd64.RBP)
	e.SubRegImm32(amd64.RSI, 64)
	e.MovRegReg(amd64.R11, amd64.R10)
	emitCopy(amd64.RSI, amd64.R11)
	emitByte(amd64.R8, '.')
	e.MovRegReg(amd64.R11, amd64.RDI)
	e.SubRegReg(amd64.R11, amd64.R10)
	emitCopy(amd64.RSI, amd64.R11)
	fixedDoneJump := len(e.Code)
	e.JmpRel32(0)

	// 0.00...digits form for exponents -1 through -6.
	leadingZeroLabel := len(e.Code)
	patchJcc(fixedLeadingZero, leadingZeroLabel)
	emitByte(amd64.R8, '0')
	emitByte(amd64.R8, '.')
	e.MovRegReg(amd64.R11, amd64.R10)
	e.NegReg(amd64.R11)
	emitZeroes(amd64.R11)
	e.MovRegReg(amd64.RSI, amd64.RBP)
	e.SubRegImm32(amd64.RSI, 64)
	e.MovRegReg(amd64.R11, amd64.RDI)
	emitCopy(amd64.RSI, amd64.R11)
	leadingDoneJump := len(e.Code)
	e.JmpRel32(0)

	// Integer-like fixed form: digits followed by required zeroes.
	integerLabel := len(e.Code)
	patchJcc(fixedInteger, integerLabel)
	e.MovRegReg(amd64.RSI, amd64.RBP)
	e.SubRegImm32(amd64.RSI, 64)
	e.MovRegReg(amd64.R11, amd64.RDI)
	emitCopy(amd64.RSI, amd64.R11)
	e.MovRegReg(amd64.R11, amd64.R10)
	e.SubRegReg(amd64.R11, amd64.RDI)
	emitZeroes(amd64.R11)

	fixedDone := len(e.Code)
	patchJmp(fixedDoneJump, fixedDone)
	patchJmp(leadingDoneJump, fixedDone)
	emitByte(amd64.R8, '\n')
	emitReturn()

	// Scientific notation: one leading digit, optional fraction, then e±N.
	scientific := len(e.Code)
	patchJcc(toScientificHigh, scientific)
	patchJcc(toScientificLow, scientific)
	e.MovRegReg(amd64.RSI, amd64.RBP)
	e.SubRegImm32(amd64.RSI, 64)
	e.MovzxRegDeref8(amd64.RAX, amd64.RSI, 0)
	e.MovDerefReg8(amd64.R8, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 1)
	e.AddRegImm32(amd64.RSI, 1)
	e.CmpRegImm32(amd64.RDI, 1)
	noSciFraction := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitByte(amd64.R8, '.')
	e.MovRegReg(amd64.R11, amd64.RDI)
	e.SubRegImm32(amd64.R11, 1)
	emitCopy(amd64.RSI, amd64.R11)
	patchJcc(noSciFraction, len(e.Code))
	emitByte(amd64.R8, 'e')

	e.MovRegReg(amd64.RAX, amd64.RCX)
	e.CmpRegImm32(amd64.RAX, 0)
	expNegative := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	emitByte(amd64.R8, '+')
	expSignDone := len(e.Code)
	e.JmpRel32(0)
	expNegativeLabel := len(e.Code)
	patchJcc(expNegative, expNegativeLabel)
	emitByte(amd64.R8, '-')
	e.NegReg(amd64.RAX)
	patchJmp(expSignDone, len(e.Code))

	// Build exponent digits backwards below RBP.
	e.MovRegReg(amd64.RSI, amd64.RBP)
	e.SubRegImm32(amd64.RSI, 1)
	e.MovRegImm64(amd64.R11, 0)
	expLoop := len(e.Code)
	e.MovRegImm64(amd64.R10, 10)
	e.Cqo()
	e.IdivReg(amd64.R10)
	e.AddRegImm32(amd64.RDX, '0')
	e.SubRegImm32(amd64.RSI, 1)
	e.MovDerefReg8(amd64.RSI, 0, amd64.RDX)
	e.AddRegImm32(amd64.R11, 1)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	expBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(expBack, expLoop)
	emitCopy(amd64.RSI, amd64.R11)
	emitByte(amd64.R8, '\n')
	emitReturn()

	// NaN and infinities preserve ECMAScript spellings.
	special := len(e.Code)
	patchJcc(specialJump, special)
	e.MovRegReg(amd64.R8, amd64.RBP)
	e.SubRegImm32(amd64.R8, 224)
	e.MovRegReg(amd64.R9, amd64.R8)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, int64(0x000fffffffffffff))
	e.AndRegReg(amd64.R10, amd64.R11)
	e.TestRegReg(amd64.R10, amd64.R10)
	isNaN := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, int64(-0x8000000000000000))
	e.AndRegReg(amd64.R10, amd64.R11)
	e.TestRegReg(amd64.R10, amd64.R10)
	infPositive := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitByte(amd64.R8, '-')
	patchJcc(infPositive, len(e.Code))
	for _, ch := range []byte("Infinity") {
		emitByte(amd64.R8, ch)
	}
	emitByte(amd64.R8, '\n')
	emitReturn()

	nan := len(e.Code)
	patchJcc(isNaN, nan)
	for _, ch := range []byte("NaN") {
		emitByte(amd64.R8, ch)
	}
	emitByte(amd64.R8, '\n')
	emitReturn()

	// Local candidate parser used only by the shortening search above. It
	// reconstructs candidate * 10^(exp-(digits-1)) and compares raw F64 bits.
	candidateHelper := len(e.Code)
	// Candidate significands fit in signed 64-bit. x87 loads them exactly,
	// scales in 80-bit precision, and rounds to binary64 only for the roundtrip
	// check. The still-live extended value then supplies an exact distance.
	e.MovDerefReg(amd64.RBP, -240, amd64.RDI)
	e.MovRegImm64(amd64.RAX, int64(0x4024000000000000)) // 10.0
	e.MovDerefReg(amd64.RBP, -232, amd64.RAX)
	e.FildDeref64(amd64.RBP, -240)
	e.MovRegDeref(amd64.R10, amd64.RBP, -32)
	e.SubRegReg(amd64.R10, amd64.RSI)
	e.AddRegImm32(amd64.R10, 1)
	e.CmpRegImm32(amd64.R10, 0)
	candidateStoreZero := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	candidateNegative := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	candidateMulLoop := len(e.Code)
	e.FmulDeref64(amd64.RBP, -232)
	e.SubRegImm32(amd64.R10, 1)
	candidateMulBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(candidateMulBack, candidateMulLoop)
	candidateScaleDone := len(e.Code)
	e.JmpRel32(0)
	candidateNegativeLabel := len(e.Code)
	patchJcc(candidateNegative, candidateNegativeLabel)
	e.NegReg(amd64.R10)
	candidateDivLoop := len(e.Code)
	e.FdivDeref64(amd64.RBP, -232)
	e.SubRegImm32(amd64.R10, 1)
	candidateDivBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(candidateDivBack, candidateDivLoop)
	candidateScaled := len(e.Code)
	patchJcc(candidateStoreZero, candidateScaled)
	patchJmp(candidateScaleDone, candidateScaled)

	// Duplicate the exact candidate: one copy rounds to binary64 for the
	// roundtrip test, while the other remains extended for distance ranking.
	e.FldST0()
	e.FstpDeref64(amd64.RBP, -248)
	e.MovRegDeref(amd64.R11, amd64.RBP, -248)
	e.MovRegDeref(amd64.RAX, amd64.RBP, -8)
	e.CmpRegReg(amd64.R11, amd64.RAX)
	candidateNoMatch := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	// ST0 = |candidateExact - originalExact|.
	e.FsubDeref64(amd64.RBP, -8)
	e.Fabs()
	e.MovRegDeref(amd64.RAX, amd64.RBP, -104)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	candidateFirst := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// Compare current distance (ST0) with best distance stored as 80-bit.
	e.FldDeref80(amd64.RBP, -128) // ST0=best, ST1=current
	e.FcomipST1()                 // compare best vs current, pop best
	candidateBetter := len(e.Code)
	e.JccRel32(amd64.CondA, 0) // best > current
	// Existing best wins. Pop current distance without changing best state.
	e.FstpDeref80(amd64.RBP, -144)
	candidateMatchedReturn := len(e.Code)
	e.JmpRel32(0)

	candidateFirstLabel := len(e.Code)
	patchJcc(candidateFirst, candidateFirstLabel)
	candidateBetterLabel := len(e.Code)
	patchJcc(candidateBetter, candidateBetterLabel)
	e.FstpDeref80(amd64.RBP, -128)
	e.MovDerefReg(amd64.RBP, -112, amd64.RDI)
	e.MovRegImm64(amd64.RAX, 1)
	e.MovDerefReg(amd64.RBP, -104, amd64.RAX)
	candidateMatched := len(e.Code)
	patchJmp(candidateMatchedReturn, candidateMatched)
	e.MovRegImm64(amd64.RAX, 1)
	e.Ret()

	candidateNoMatchLabel := len(e.Code)
	patchJcc(candidateNoMatch, candidateNoMatchLabel)
	// Pop the exact candidate left in ST0.
	e.FstpDeref80(amd64.RBP, -144)
	e.MovRegImm64(amd64.RAX, 0)
	e.Ret()

	for _, at := range candidateCalls {
		rel := int32(candidateHelper - (at + 5))
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(rel))
	}
}
