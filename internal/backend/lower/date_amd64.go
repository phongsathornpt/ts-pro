package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	amd64DateYear = iota
	amd64DateMonth
	amd64DateDay
	amd64DateHours
	amd64DateMinutes
	amd64DateSeconds
)

func emitAMD64DateFromNumber(e *amd64.Emitter) { e.Cvttsd2si(amd64.RAX, amd64.XMM0); e.Ret() }

func emitAMD64DateNow(e *amd64.Emitter) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 16)
	e.MovRegImm64(amd64.RAX, 228)
	e.MovRegImm64(amd64.RDI, 0)
	e.MovRegReg(amd64.RSI, amd64.RSP)
	e.Syscall()
	e.MovRegDeref(amd64.R11, amd64.RSP, 0)
	e.MovRegImm64(amd64.R10, 1000)
	e.ImulRegReg(amd64.R11, amd64.R10)
	e.MovRegDeref(amd64.RAX, amd64.RSP, 8)
	e.Cqo()
	e.MovRegImm64(amd64.R10, 1000000)
	e.IdivReg(amd64.R10)
	e.AddRegReg(amd64.RAX, amd64.R11)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)
	e.MovRegReg(amd64.RSP, amd64.RBP)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64DateLeap(e *amd64.Emitter, year, out amd64.Register) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.MovRegImm64(out, 0)
	e.MovRegReg(amd64.RAX, year)
	e.Cqo()
	e.MovRegImm64(amd64.R11, 4)
	e.IdivReg(amd64.R11)
	e.TestRegReg(amd64.RDX, amd64.RDX)
	notLeap4 := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.RAX, year)
	e.Cqo()
	e.MovRegImm64(amd64.R11, 100)
	e.IdivReg(amd64.R11)
	e.TestRegReg(amd64.RDX, amd64.RDX)
	leap100 := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.RAX, year)
	e.Cqo()
	e.MovRegImm64(amd64.R11, 400)
	e.IdivReg(amd64.R11)
	e.TestRegReg(amd64.RDX, amd64.RDX)
	notLeap400 := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	leap := len(e.Code)
	patchJcc(leap100, leap)
	e.MovRegImm64(out, 1)
	doneJump := len(e.Code)
	e.JmpRel32(0)
	notLeap := len(e.Code)
	patchJcc(notLeap4, notLeap)
	patchJcc(notLeap400, notLeap)
	done := len(e.Code)
	patchJmp(doneJump, done)
}

func emitAMD64DateGetPart(e *amd64.Emitter, part int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	retNum := func(reg amd64.Register) { e.Cvtsi2sd(amd64.XMM0, reg); e.Ret() }
	div := func(value amd64.Register, divisor int64, q, r amd64.Register) {
		e.MovRegReg(amd64.RAX, value)
		e.Cqo()
		e.MovRegImm64(amd64.R11, divisor)
		e.IdivReg(amd64.R11)
		if q != amd64.RAX {
			e.MovRegReg(q, amd64.RAX)
		}
		if r != amd64.RDX {
			e.MovRegReg(r, amd64.RDX)
		}
	}
	div(amd64.RDI, 86400000, amd64.R8, amd64.R9)
	switch part {
	case amd64DateHours:
		div(amd64.R9, 3600000, amd64.R8, amd64.R10)
		retNum(amd64.R8)
		return
	case amd64DateMinutes:
		div(amd64.R9, 3600000, amd64.R8, amd64.R9)
		div(amd64.R9, 60000, amd64.R8, amd64.R10)
		retNum(amd64.R8)
		return
	case amd64DateSeconds:
		div(amd64.R9, 60000, amd64.R8, amd64.R9)
		div(amd64.R9, 1000, amd64.R8, amd64.R10)
		retNum(amd64.R8)
		return
	}

	e.MovRegImm64(amd64.R9, 1970)
	yearLoop := len(e.Code)
	emitAMD64DateLeap(e, amd64.R9, amd64.R10)
	e.MovRegImm64(amd64.R11, 365)
	e.AddRegReg(amd64.R11, amd64.R10)
	e.CmpRegReg(amd64.R8, amd64.R11)
	yearDoneJump := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.SubRegReg(amd64.R8, amd64.R11)
	e.AddRegImm32(amd64.R9, 1)
	yearBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(yearBack, yearLoop)
	yearDone := len(e.Code)
	patchJcc(yearDoneJump, yearDone)
	if part == amd64DateYear {
		retNum(amd64.R9)
		return
	}

	// Recompute leap flag for the resolved year because year-loop comparisons clobber scratch state.
	emitAMD64DateLeap(e, amd64.R9, amd64.R10)
	e.MovRegImm64(amd64.RCX, 0)
	monthLoop := len(e.Code)
	e.MovRegImm64(amd64.R11, 31)
	e.CmpRegImm32(amd64.RCX, 1)
	febJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	var thirtyJumps []int
	for _, m := range []int32{3, 5, 8, 10} {
		e.CmpRegImm32(amd64.RCX, m)
		at := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		thirtyJumps = append(thirtyJumps, at)
	}
	readyJump := len(e.Code)
	e.JmpRel32(0)
	feb := len(e.Code)
	patchJcc(febJump, feb)
	e.MovRegImm64(amd64.R11, 28)
	e.AddRegReg(amd64.R11, amd64.R10)
	febReady := len(e.Code)
	e.JmpRel32(0)
	thirty := len(e.Code)
	for _, at := range thirtyJumps {
		patchJcc(at, thirty)
	}
	e.MovRegImm64(amd64.R11, 30)
	ready := len(e.Code)
	patchJmp(readyJump, ready)
	patchJmp(febReady, ready)
	e.CmpRegReg(amd64.R8, amd64.R11)
	monthDoneJump := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.SubRegReg(amd64.R8, amd64.R11)
	e.AddRegImm32(amd64.RCX, 1)
	monthBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(monthBack, monthLoop)
	monthDone := len(e.Code)
	patchJcc(monthDoneJump, monthDone)
	if part == amd64DateMonth {
		retNum(amd64.RCX)
		return
	}
	e.AddRegImm32(amd64.R8, 1)
	retNum(amd64.R8)
}

func emitAMD64DateToISO(e *amd64.Emitter, allocOffset int, getters [6]int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegImm64(amd64.RDI, 32)
	callAlloc := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAlloc + 5)))
	e.MovRegReg(amd64.R12, amd64.RAX)
	e.MovRegImm64(amd64.RAX, 24)
	e.MovDerefReg(amd64.R12, 0, amd64.RAX)
	writeByte := func(off int32, ch byte) {
		e.MovRegImm64(amd64.RAX, int64(ch))
		e.MovDerefReg8(amd64.R12, 8+off, amd64.RAX)
	}
	callGetter := func(offset int) {
		e.MovRegReg(amd64.RDI, amd64.RBX)
		at := len(e.Code)
		e.CallRel32(int32(offset - (at + 5)))
		e.Cvttsd2si(amd64.R13, amd64.XMM0)
	}
	writeDigits := func(off int32, divs []int64) {
		e.MovRegReg(amd64.R14, amd64.R13)
		for i, d := range divs {
			e.MovRegReg(amd64.RAX, amd64.R14)
			e.Cqo()
			e.MovRegImm64(amd64.R10, d)
			e.IdivReg(amd64.R10)
			e.AddRegImm32(amd64.RAX, 48)
			e.MovDerefReg8(amd64.R12, 8+off+int32(i), amd64.RAX)
			e.MovRegReg(amd64.R14, amd64.RDX)
		}
	}
	callGetter(getters[amd64DateYear])
	writeDigits(0, []int64{1000, 100, 10, 1})
	writeByte(4, '-')
	callGetter(getters[amd64DateMonth])
	e.AddRegImm32(amd64.R13, 1)
	writeDigits(5, []int64{10, 1})
	writeByte(7, '-')
	callGetter(getters[amd64DateDay])
	writeDigits(8, []int64{10, 1})
	writeByte(10, 'T')
	callGetter(getters[amd64DateHours])
	writeDigits(11, []int64{10, 1})
	writeByte(13, ':')
	callGetter(getters[amd64DateMinutes])
	writeDigits(14, []int64{10, 1})
	writeByte(16, ':')
	callGetter(getters[amd64DateSeconds])
	writeDigits(17, []int64{10, 1})
	writeByte(19, '.')
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.Cqo()
	e.MovRegImm64(amd64.R10, 1000)
	e.IdivReg(amd64.R10)
	e.MovRegReg(amd64.R13, amd64.RDX)
	writeDigits(20, []int64{100, 10, 1})
	writeByte(23, 'Z')
	e.MovRegReg(amd64.RAX, amd64.R12)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
