package irgen

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) headersEntries(headers ir.Operand) ir.Operand {
	t := g.semaResult.HeadersType
	offsets, _, _ := g.objectLayout(t)
	entriesType := types.NewArray(types.TypeString)
	res := g.currentFn.NewValue("headers_entries", entriesType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
		Res: res, Obj: headers, Field: "$entries", Offset: offsets["$entries"],
	})
	return res
}

func isValidHeaderName(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		if (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
			continue
		}
		switch b {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func (g *generator) validateHeaderName(name ir.Operand) {
	if c, ok := name.(ir.ConstString); ok {
		if !isValidHeaderName(c.Value) {
			errBB := g.currentFn.NewBlock("hdr_name_const_err")
			nextBB := g.currentFn.NewBlock("hdr_name_const_ok")
			g.currentBB.Terminator = &ir.JumpTerm{Target: errBB}
			g.currentBB = errBB
			err := g.newWebError(ir.ConstString{Value: "Invalid header name"}, ir.ConstString{Value: "TypeError"})
			g.routeThrownValue(err)
			g.currentBB = nextBB
		}
		return
	}

	total := g.urlStringLen(name)
	entryBB := g.currentBB
	isZero := g.currentFn.NewValue("hdr_name_is_zero", types.TypeBoolean)
	entryBB.Instructions = append(entryBB.Instructions, &ir.BinaryInst{
		Res: isZero, Op: ir.OpEq, LHS: total, RHS: ir.ConstNumber{Value: 0},
	})
	chkCondBB := g.currentFn.NewBlock("hdr_name_chk_cond")
	chkBodyBB := g.currentFn.NewBlock("hdr_name_chk_body")
	chkSpecialBB := g.currentFn.NewBlock("hdr_name_chk_special")
	chkNextBB := g.currentFn.NewBlock("hdr_name_chk_next")
	errBB := g.currentFn.NewBlock("hdr_name_chk_err")
	doneBB := g.currentFn.NewBlock("hdr_name_chk_done")

	entryBB.Terminator = &ir.BranchTerm{Cond: isZero, Then: errBB, Else: chkCondBB}

	i := g.currentFn.NewValue("hdr_name_chk_i", types.TypeNumber)
	nextI := g.currentFn.NewValue("hdr_name_chk_next_i", types.TypeNumber)
	chkCondBB.Phis = append(chkCondBB.Phis, &ir.PhiInst{
		Res: i,
		Incoming: []ir.PhiIncoming{
			{Block: entryBB, Value: ir.ConstNumber{Value: 0}},
			{Block: chkNextBB, Value: nextI},
		},
	})
	more := g.currentFn.NewValue("hdr_name_chk_more", types.TypeBoolean)
	chkCondBB.Instructions = append(chkCondBB.Instructions, &ir.BinaryInst{
		Res: more, Op: ir.OpLt, LHS: i, RHS: total,
	})
	chkCondBB.Terminator = &ir.BranchTerm{Cond: more, Then: chkBodyBB, Else: doneBB}

	g.currentBB = chkBodyBB
	b := g.urlStringByteAt(name, i)

	isDigitLo := g.currentFn.NewValue("is_dig_lo", types.TypeBoolean)
	isDigitHi := g.currentFn.NewValue("is_dig_hi", types.TypeBoolean)
	isDigit := g.currentFn.NewValue("is_digit", types.TypeBoolean)
	chkBodyBB.Instructions = append(chkBodyBB.Instructions,
		&ir.BinaryInst{Res: isDigitLo, Op: ir.OpGe, LHS: b, RHS: ir.ConstNumber{Value: 0x30}},
		&ir.BinaryInst{Res: isDigitHi, Op: ir.OpLe, LHS: b, RHS: ir.ConstNumber{Value: 0x39}},
		&ir.BinaryInst{Res: isDigit, Op: ir.OpAnd, LHS: isDigitLo, RHS: isDigitHi},
	)

	isLowerLo := g.currentFn.NewValue("is_low_lo", types.TypeBoolean)
	isLowerHi := g.currentFn.NewValue("is_low_hi", types.TypeBoolean)
	isLower := g.currentFn.NewValue("is_lower", types.TypeBoolean)
	chkBodyBB.Instructions = append(chkBodyBB.Instructions,
		&ir.BinaryInst{Res: isLowerLo, Op: ir.OpGe, LHS: b, RHS: ir.ConstNumber{Value: 0x61}},
		&ir.BinaryInst{Res: isLowerHi, Op: ir.OpLe, LHS: b, RHS: ir.ConstNumber{Value: 0x7A}},
		&ir.BinaryInst{Res: isLower, Op: ir.OpAnd, LHS: isLowerLo, RHS: isLowerHi},
	)

	isUpperLo := g.currentFn.NewValue("is_up_lo", types.TypeBoolean)
	isUpperHi := g.currentFn.NewValue("is_up_hi", types.TypeBoolean)
	isUpper := g.currentFn.NewValue("is_upper", types.TypeBoolean)
	chkBodyBB.Instructions = append(chkBodyBB.Instructions,
		&ir.BinaryInst{Res: isUpperLo, Op: ir.OpGe, LHS: b, RHS: ir.ConstNumber{Value: 0x41}},
		&ir.BinaryInst{Res: isUpperHi, Op: ir.OpLe, LHS: b, RHS: ir.ConstNumber{Value: 0x5A}},
		&ir.BinaryInst{Res: isUpper, Op: ir.OpAnd, LHS: isUpperLo, RHS: isUpperHi},
	)

	isAlnum1 := g.currentFn.NewValue("is_alnum1", types.TypeBoolean)
	isAlnum := g.currentFn.NewValue("is_alnum", types.TypeBoolean)
	chkBodyBB.Instructions = append(chkBodyBB.Instructions,
		&ir.BinaryInst{Res: isAlnum1, Op: ir.OpOr, LHS: isDigit, RHS: isLower},
		&ir.BinaryInst{Res: isAlnum, Op: ir.OpOr, LHS: isAlnum1, RHS: isUpper},
	)
	chkBodyBB.Terminator = &ir.BranchTerm{Cond: isAlnum, Then: chkNextBB, Else: chkSpecialBB}

	g.currentBB = chkSpecialBB
	specials := []int{0x21, 0x23, 0x24, 0x25, 0x26, 0x27, 0x2A, 0x2B, 0x2D, 0x2E, 0x5E, 0x5F, 0x60, 0x7C, 0x7E}
	var specMatches []ir.Operand
	for idx, code := range specials {
		m := g.currentFn.NewValue(fmt.Sprintf("hdr_spec_%d", idx), types.TypeBoolean)
		chkSpecialBB.Instructions = append(chkSpecialBB.Instructions, &ir.BinaryInst{
			Res: m, Op: ir.OpEq, LHS: b, RHS: ir.ConstNumber{Value: float64(code)},
		})
		specMatches = append(specMatches, m)
	}
	anySpec := specMatches[0]
	for idx, m := range specMatches[1:] {
		comb := g.currentFn.NewValue(fmt.Sprintf("hdr_spec_comb_%d", idx), types.TypeBoolean)
		chkSpecialBB.Instructions = append(chkSpecialBB.Instructions, &ir.BinaryInst{
			Res: comb, Op: ir.OpOr, LHS: anySpec, RHS: m,
		})
		anySpec = comb
	}
	chkSpecialBB.Terminator = &ir.BranchTerm{Cond: anySpec, Then: chkNextBB, Else: errBB}

	g.currentBB = errBB
	err := g.newWebError(ir.ConstString{Value: "Invalid header name"}, ir.ConstString{Value: "TypeError"})
	g.routeThrownValue(err)

	chkNextBB.Instructions = append(chkNextBB.Instructions, &ir.BinaryInst{
		Res: nextI, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 1},
	})
	chkNextBB.Terminator = &ir.JumpTerm{Target: chkCondBB}

	g.currentBB = doneBB
}

func isHttpWhitespaceByte(b byte) bool {
	return b == 0x20 || b == 0x09 || b == 0x0A || b == 0x0D
}

func trimHttpWhitespaceString(s string) string {
	start := 0
	end := len(s)
	for start < end && isHttpWhitespaceByte(s[start]) {
		start++
	}
	for end > start && isHttpWhitespaceByte(s[end-1]) {
		end--
	}
	return s[start:end]
}

func (g *generator) lowerIsHttpWhitespace(b ir.Operand) ir.Operand {
	is20 := g.currentFn.NewValue("is_sp", types.TypeBoolean)
	is09 := g.currentFn.NewValue("is_tab", types.TypeBoolean)
	is0A := g.currentFn.NewValue("is_lf", types.TypeBoolean)
	is0D := g.currentFn.NewValue("is_cr", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: is20, Op: ir.OpEq, LHS: b, RHS: ir.ConstNumber{Value: 0x20}},
		&ir.BinaryInst{Res: is09, Op: ir.OpEq, LHS: b, RHS: ir.ConstNumber{Value: 0x09}},
		&ir.BinaryInst{Res: is0A, Op: ir.OpEq, LHS: b, RHS: ir.ConstNumber{Value: 0x0A}},
		&ir.BinaryInst{Res: is0D, Op: ir.OpEq, LHS: b, RHS: ir.ConstNumber{Value: 0x0D}},
	)
	or1 := g.currentFn.NewValue("is_ws1", types.TypeBoolean)
	or2 := g.currentFn.NewValue("is_ws2", types.TypeBoolean)
	or3 := g.currentFn.NewValue("is_ws3", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: or1, Op: ir.OpOr, LHS: is20, RHS: is09},
		&ir.BinaryInst{Res: or2, Op: ir.OpOr, LHS: is0A, RHS: is0D},
		&ir.BinaryInst{Res: or3, Op: ir.OpOr, LHS: or1, RHS: or2},
	)
	return or3
}

func (g *generator) trimHttpWhitespace(val ir.Operand) ir.Operand {
	if c, ok := val.(ir.ConstString); ok {
		return ir.ConstString{Value: trimHttpWhitespaceString(c.Value)}
	}
	total := g.urlStringLen(val)
	entryBB := g.currentBB

	leadCond := g.currentFn.NewBlock("hdr_trim_lead_cond")
	leadBody := g.currentFn.NewBlock("hdr_trim_lead_body")
	leadNext := g.currentFn.NewBlock("hdr_trim_lead_next")
	trailInit := g.currentFn.NewBlock("hdr_trim_trail_init")

	entryBB.Terminator = &ir.JumpTerm{Target: leadCond}

	startPhi := g.currentFn.NewValue("hdr_trim_start", types.TypeNumber)
	nextStart := g.currentFn.NewValue("hdr_trim_next_start", types.TypeNumber)
	leadCond.Phis = append(leadCond.Phis, &ir.PhiInst{
		Res: startPhi,
		Incoming: []ir.PhiIncoming{
			{Block: entryBB, Value: ir.ConstNumber{Value: 0}},
			{Block: leadNext, Value: nextStart},
		},
	})
	hasChars := g.currentFn.NewValue("hdr_trim_lead_has", types.TypeBoolean)
	leadCond.Instructions = append(leadCond.Instructions, &ir.BinaryInst{
		Res: hasChars, Op: ir.OpLt, LHS: startPhi, RHS: total,
	})
	leadCond.Terminator = &ir.BranchTerm{Cond: hasChars, Then: leadBody, Else: trailInit}

	g.currentBB = leadBody
	leadByte := g.urlStringByteAt(val, startPhi)
	leadIsWs := g.lowerIsHttpWhitespace(leadByte)
	leadBody.Terminator = &ir.BranchTerm{Cond: leadIsWs, Then: leadNext, Else: trailInit}

	leadNext.Instructions = append(leadNext.Instructions, &ir.BinaryInst{
		Res: nextStart, Op: ir.OpAdd, LHS: startPhi, RHS: ir.ConstNumber{Value: 1},
	})
	leadNext.Terminator = &ir.JumpTerm{Target: leadCond}

	trailCond := g.currentFn.NewBlock("hdr_trim_trail_cond")
	trailBody := g.currentFn.NewBlock("hdr_trim_trail_body")
	trailNext := g.currentFn.NewBlock("hdr_trim_trail_next")
	sliceBB := g.currentFn.NewBlock("hdr_trim_slice")

	trailInit.Terminator = &ir.JumpTerm{Target: trailCond}

	endPhi := g.currentFn.NewValue("hdr_trim_end", types.TypeNumber)
	nextEnd := g.currentFn.NewValue("hdr_trim_next_end", types.TypeNumber)
	trailCond.Phis = append(trailCond.Phis, &ir.PhiInst{
		Res: endPhi,
		Incoming: []ir.PhiIncoming{
			{Block: trailInit, Value: total},
			{Block: trailNext, Value: nextEnd},
		},
	})
	moreTrail := g.currentFn.NewValue("hdr_trim_trail_more", types.TypeBoolean)
	trailCond.Instructions = append(trailCond.Instructions, &ir.BinaryInst{
		Res: moreTrail, Op: ir.OpGt, LHS: endPhi, RHS: startPhi,
	})
	trailCond.Terminator = &ir.BranchTerm{Cond: moreTrail, Then: trailBody, Else: sliceBB}

	g.currentBB = trailBody
	trailPrevIndex := g.currentFn.NewValue("hdr_trim_trail_prev_i", types.TypeNumber)
	trailBody.Instructions = append(trailBody.Instructions, &ir.BinaryInst{
		Res: trailPrevIndex, Op: ir.OpSub, LHS: endPhi, RHS: ir.ConstNumber{Value: 1},
	})
	trailByte := g.urlStringByteAt(val, trailPrevIndex)
	trailIsWs := g.lowerIsHttpWhitespace(trailByte)
	trailBody.Terminator = &ir.BranchTerm{Cond: trailIsWs, Then: trailNext, Else: sliceBB}

	trailNext.Instructions = append(trailNext.Instructions, &ir.BinaryInst{
		Res: nextEnd, Op: ir.OpSub, LHS: endPhi, RHS: ir.ConstNumber{Value: 1},
	})
	trailNext.Terminator = &ir.JumpTerm{Target: trailCond}

	g.currentBB = sliceBB
	return g.urlStringSlice(val, startPhi, endPhi)
}

func (g *generator) validateHeaderValue(val ir.Operand) {
	if c, ok := val.(ir.ConstString); ok {
		for i := 0; i < len(c.Value); i++ {
			ch := c.Value[i]
			if ch == 0x00 || ch == 0x0A || ch == 0x0D {
				errBB := g.currentFn.NewBlock("hdr_val_const_err")
				nextBB := g.currentFn.NewBlock("hdr_val_const_ok")
				g.currentBB.Terminator = &ir.JumpTerm{Target: errBB}
				g.currentBB = errBB
				err := g.newWebError(ir.ConstString{Value: "Invalid header value"}, ir.ConstString{Value: "TypeError"})
				g.routeThrownValue(err)
				g.currentBB = nextBB
				return
			}
		}
		return
	}

	total := g.urlStringLen(val)
	entryBB := g.currentBB
	condBB := g.currentFn.NewBlock("hdr_val_chk_cond")
	bodyBB := g.currentFn.NewBlock("hdr_val_chk_body")
	nextBB := g.currentFn.NewBlock("hdr_val_chk_next")
	errBB := g.currentFn.NewBlock("hdr_val_chk_err")
	doneBB := g.currentFn.NewBlock("hdr_val_chk_done")

	entryBB.Terminator = &ir.JumpTerm{Target: condBB}

	i := g.currentFn.NewValue("hdr_val_chk_i", types.TypeNumber)
	nextI := g.currentFn.NewValue("hdr_val_chk_next_i", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{
		Res: i,
		Incoming: []ir.PhiIncoming{
			{Block: entryBB, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextI},
		},
	})
	more := g.currentFn.NewValue("hdr_val_chk_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{
		Res: more, Op: ir.OpLt, LHS: i, RHS: total,
	})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	b := g.urlStringByteAt(val, i)
	is00 := g.currentFn.NewValue("val_is_00", types.TypeBoolean)
	is0A := g.currentFn.NewValue("val_is_0a", types.TypeBoolean)
	is0D := g.currentFn.NewValue("val_is_0d", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.BinaryInst{Res: is00, Op: ir.OpEq, LHS: b, RHS: ir.ConstNumber{Value: 0x00}},
		&ir.BinaryInst{Res: is0A, Op: ir.OpEq, LHS: b, RHS: ir.ConstNumber{Value: 0x0A}},
		&ir.BinaryInst{Res: is0D, Op: ir.OpEq, LHS: b, RHS: ir.ConstNumber{Value: 0x0D}},
	)
	bad1 := g.currentFn.NewValue("val_bad1", types.TypeBoolean)
	bad2 := g.currentFn.NewValue("val_bad2", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.BinaryInst{Res: bad1, Op: ir.OpOr, LHS: is00, RHS: is0A},
		&ir.BinaryInst{Res: bad2, Op: ir.OpOr, LHS: bad1, RHS: is0D},
	)
	bodyBB.Terminator = &ir.BranchTerm{Cond: bad2, Then: errBB, Else: nextBB}

	g.currentBB = errBB
	err := g.newWebError(ir.ConstString{Value: "Invalid header value"}, ir.ConstString{Value: "TypeError"})
	g.routeThrownValue(err)

	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{
		Res: nextI, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 1},
	})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = doneBB
}

func (g *generator) stringAsciiLower(s ir.Operand) ir.Operand {
	if c, ok := s.(ir.ConstString); ok {
		return ir.ConstString{Value: strings.ToLower(c.Value)}
	}
	res := g.currentFn.NewValue("lower_str", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_string_ascii_lower", Args: []ir.Operand{s},
		ParamTypes: []types.Type{types.TypeString},
	})
	return res
}

func (g *generator) stringAsciiUpper(s ir.Operand) ir.Operand {
	if c, ok := s.(ir.ConstString); ok {
		return ir.ConstString{Value: strings.ToUpper(c.Value)}
	}
	res := g.currentFn.NewValue("upper_str", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_string_ascii_upper", Args: []ir.Operand{s},
		ParamTypes: []types.Type{types.TypeString},
	})
	return res
}

func (g *generator) lowerHeadersAppendDirect(headers, name, value ir.Operand) {
	g.validateHeaderName(name)
	trimmed := g.trimHttpWhitespace(value)
	g.validateHeaderValue(trimmed)
	lowerName := g.stringAsciiLower(name)
	entries := g.headersEntries(headers)
	g.pushArrayOperand(entries, lowerName)
	g.pushArrayOperand(entries, trimmed)
}

func (g *generator) lowerHeadersInit(headers ir.Operand, initExpr ast.Expr) {
	if initExpr == nil {
		return
	}
	if objLit, ok := initExpr.(*ast.ObjectLit); ok {
		for _, prop := range objLit.Properties {
			if !prop.Spread {
				keyStr := ir.ConstString{Value: prop.Key}
				valOperand := g.lowerExpr(prop.Value)
				valStr := g.coerceStringType(g.semanticType(prop.Value), valOperand)
				g.lowerHeadersAppendDirect(headers, keyStr, valStr)
			}
		}
		return
	}
	if arrLit, ok := initExpr.(*ast.ArrayLit); ok {
		for _, elem := range arrLit.Elements {
			if pairLit, ok := elem.(*ast.ArrayLit); ok && len(pairLit.Elements) >= 2 {
				keyOperand := g.lowerExpr(pairLit.Elements[0])
				keyStr := g.coerceStringType(g.semanticType(pairLit.Elements[0]), keyOperand)
				valOperand := g.lowerExpr(pairLit.Elements[1])
				valStr := g.coerceStringType(g.semanticType(pairLit.Elements[1]), valOperand)
				g.lowerHeadersAppendDirect(headers, keyStr, valStr)
			}
		}
		return
	}

	initVal := g.lowerExpr(initExpr)
	initType := g.semanticType(initExpr)
	if objType, ok := initType.(*types.ObjectType); ok && objType.Name == "$Headers" {
		srcEntries := g.headersEntries(initVal)
		dstEntries := g.headersEntries(headers)
		srcLen := g.currentFn.NewValue("src_entries_len", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: srcLen, Array: srcEntries})

		preBB := g.currentBB
		condBB := g.currentFn.NewBlock("hdr_copy_cond")
		bodyBB := g.currentFn.NewBlock("hdr_copy_body")
		doneBB := g.currentFn.NewBlock("hdr_copy_done")
		preBB.Terminator = &ir.JumpTerm{Target: condBB}

		idx := g.currentFn.NewValue("hdr_copy_i", types.TypeNumber)
		nextIdx := g.currentFn.NewValue("hdr_copy_next_i", types.TypeNumber)
		condBB.Phis = append(condBB.Phis, &ir.PhiInst{
			Res: idx,
			Incoming: []ir.PhiIncoming{
				{Block: preBB, Value: ir.ConstNumber{Value: 0}},
				{Block: bodyBB, Value: nextIdx},
			},
		})
		more := g.currentFn.NewValue("hdr_copy_more", types.TypeBoolean)
		condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: idx, RHS: srcLen})
		condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

		g.currentBB = bodyBB
		item := g.currentFn.NewValue("hdr_copy_item", types.TypeString)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: item, Array: srcEntries, Index: idx})
		g.pushArrayOperand(dstEntries, item)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{Res: nextIdx, Op: ir.OpAdd, LHS: idx, RHS: ir.ConstNumber{Value: 1}})
		bodyBB.Terminator = &ir.JumpTerm{Target: condBB}

		g.currentBB = doneBB
		return
	}
	if arrType, ok := initType.(*types.ArrayType); ok {
		arrLen := g.currentFn.NewValue("hdr_init_arr_len", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: arrLen, Array: initVal})

		preBB := g.currentBB
		condBB := g.currentFn.NewBlock("hdr_init_arr_cond")
		bodyBB := g.currentFn.NewBlock("hdr_init_arr_body")
		doneBB := g.currentFn.NewBlock("hdr_init_arr_done")
		preBB.Terminator = &ir.JumpTerm{Target: condBB}

		idx := g.currentFn.NewValue("hdr_init_arr_i", types.TypeNumber)
		nextIdx := g.currentFn.NewValue("hdr_init_arr_next_i", types.TypeNumber)
		condBB.Phis = append(condBB.Phis, &ir.PhiInst{
			Res: idx,
			Incoming: []ir.PhiIncoming{
				{Block: preBB, Value: ir.ConstNumber{Value: 0}},
				{Block: bodyBB, Value: nextIdx},
			},
		})
		more := g.currentFn.NewValue("hdr_init_arr_more", types.TypeBoolean)
		condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: idx, RHS: arrLen})
		condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

		g.currentBB = bodyBB
		pair := g.currentFn.NewValue("hdr_init_pair", arrType.Elem)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: pair, Array: initVal, Index: idx})
		kVal := g.currentFn.NewValue("hdr_init_pair_k", types.TypeString)
		vVal := g.currentFn.NewValue("hdr_init_pair_v", types.TypeString)
		bodyBB.Instructions = append(bodyBB.Instructions,
			&ir.GetElementInst{Res: kVal, Array: pair, Index: ir.ConstNumber{Value: 0}},
			&ir.GetElementInst{Res: vVal, Array: pair, Index: ir.ConstNumber{Value: 1}},
		)
		g.lowerHeadersAppendDirect(headers, kVal, vVal)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{Res: nextIdx, Op: ir.OpAdd, LHS: idx, RHS: ir.ConstNumber{Value: 1}})
		bodyBB.Terminator = &ir.JumpTerm{Target: condBB}

		g.currentBB = doneBB
		return
	}
	if objType, ok := initType.(*types.ObjectType); ok {
		offsets, _, _ := g.objectLayout(objType)
		for _, field := range objType.FieldOrder {
			fieldVal := g.currentFn.NewValue("hdr_init_fld", objType.Fields[field].Type)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
				Res: fieldVal, Obj: initVal, Field: field, Offset: offsets[field],
			})
			valStr := g.coerceStringType(objType.Fields[field].Type, fieldVal)
			g.lowerHeadersAppendDirect(headers, ir.ConstString{Value: field}, valStr)
		}
	}
}

func (g *generator) lowerHeadersNew(e *ast.NewExpr) ir.Operand {
	t := g.semaResult.HeadersType
	offsets, refMask, shape := g.objectLayout(t)
	headers := g.currentFn.NewValue("headers", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: headers, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	entriesType := types.NewArray(types.TypeString)
	entries := g.currentFn.NewValue("headers_entries_init", entriesType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: entries, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0},
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: headers, Field: "$entries", Offset: offsets["$entries"], Val: entries,
	})
	if len(e.Args) > 0 {
		g.lowerHeadersInit(headers, e.Args[0])
	}
	return headers
}

func (g *generator) lowerHeadersGet(headers, name ir.Operand) ir.Operand {
	g.validateHeaderName(name)
	lowerName := g.stringAsciiLower(name)
	entries := g.headersEntries(headers)

	startBB := g.currentBB
	length := g.currentFn.NewValue("hdr_get_len", types.TypeNumber)
	startBB.Instructions = append(startBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})

	condBB := g.currentFn.NewBlock("hdr_get_cond")
	bodyBB := g.currentFn.NewBlock("hdr_get_body")
	matchBB := g.currentFn.NewBlock("hdr_get_match")
	firstBB := g.currentFn.NewBlock("hdr_get_first")
	joinBB := g.currentFn.NewBlock("hdr_get_join")
	nextBB := g.currentFn.NewBlock("hdr_get_next")
	doneBB := g.currentFn.NewBlock("hdr_get_done")

	startBB.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("hdr_get_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("hdr_get_next_i", types.TypeNumber)
	acc := g.currentFn.NewValue("hdr_get_acc", types.TypeString)
	nextAcc := g.currentFn.NewValue("hdr_get_next_acc", types.TypeString)
	found := g.currentFn.NewValue("hdr_get_found", types.TypeBoolean)
	nextFound := g.currentFn.NewValue("hdr_get_next_found", types.TypeBoolean)

	condBB.Phis = append(condBB.Phis,
		&ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: startBB, Value: ir.ConstNumber{Value: 0}}, {Block: nextBB, Value: nextIndex}}},
		&ir.PhiInst{Res: acc, Incoming: []ir.PhiIncoming{{Block: startBB, Value: ir.ConstString{Value: ""}}, {Block: nextBB, Value: nextAcc}}},
		&ir.PhiInst{Res: found, Incoming: []ir.PhiIncoming{{Block: startBB, Value: ir.ConstBool{Value: false}}, {Block: nextBB, Value: nextFound}}},
	)
	more := g.currentFn.NewValue("hdr_get_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	key := g.currentFn.NewValue("hdr_get_key", types.TypeString)
	valIdx := g.currentFn.NewValue("hdr_get_val_i", types.TypeNumber)
	val := g.currentFn.NewValue("hdr_get_val", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.GetElementInst{Res: key, Array: entries, Index: index},
		&ir.BinaryInst{Res: valIdx, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}},
		&ir.GetElementInst{Res: val, Array: entries, Index: valIdx},
	)
	equal := g.currentFn.NewValue("hdr_get_eq", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
		Res: equal, Callee: "ts_string_eq", Args: []ir.Operand{key, lowerName},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	bodyBB.Terminator = &ir.BranchTerm{Cond: equal, Then: matchBB, Else: nextBB}

	g.currentBB = matchBB
	matchBB.Terminator = &ir.BranchTerm{Cond: found, Then: joinBB, Else: firstBB}

	g.currentBB = firstBB
	firstEnd := g.currentBB
	firstEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = joinBB
	joinedVal := g.concatNativeStrings(g.concatNativeStrings(acc, ir.ConstString{Value: ", "}), val)
	joinEnd := g.currentBB
	joinEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = nextBB
	nextBB.Phis = append(nextBB.Phis,
		&ir.PhiInst{Res: nextAcc, Incoming: []ir.PhiIncoming{
			{Block: bodyBB, Value: acc},
			{Block: firstEnd, Value: val},
			{Block: joinEnd, Value: joinedVal},
		}},
		&ir.PhiInst{Res: nextFound, Incoming: []ir.PhiIncoming{
			{Block: bodyBB, Value: found},
			{Block: firstEnd, Value: ir.ConstBool{Value: true}},
			{Block: joinEnd, Value: ir.ConstBool{Value: true}},
		}},
	)
	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{
		Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2},
	})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = doneBB
	resBB := g.currentFn.NewBlock("hdr_get_res")
	retNullBB := g.currentFn.NewBlock("hdr_get_ret_null")
	retStrBB := g.currentFn.NewBlock("hdr_get_ret_str")
	doneBB.Terminator = &ir.BranchTerm{Cond: found, Then: retStrBB, Else: retNullBB}

	retNullBB.Terminator = &ir.JumpTerm{Target: resBB}
	retStrBB.Terminator = &ir.JumpTerm{Target: resBB}

	g.currentBB = resBB
	res := g.currentFn.NewValue("hdr_get_result", types.NewUnion(types.TypeString, types.TypeNull))
	resBB.Phis = append(resBB.Phis, &ir.PhiInst{
		Res: res,
		Incoming: []ir.PhiIncoming{
			{Block: retNullBB, Value: ir.ConstNull{}},
			{Block: retStrBB, Value: acc},
		},
	})
	return res
}

func (g *generator) lowerHeadersHas(headers, name ir.Operand) ir.Operand {
	g.validateHeaderName(name)
	lowerName := g.stringAsciiLower(name)
	entries := g.headersEntries(headers)

	startBB := g.currentBB
	length := g.currentFn.NewValue("hdr_has_len", types.TypeNumber)
	startBB.Instructions = append(startBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})

	condBB := g.currentFn.NewBlock("hdr_has_cond")
	bodyBB := g.currentFn.NewBlock("hdr_has_body")
	nextBB := g.currentFn.NewBlock("hdr_has_next")
	doneBB := g.currentFn.NewBlock("hdr_has_done")
	retBB := g.currentFn.NewBlock("hdr_has_ret")

	startBB.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("hdr_has_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("hdr_has_next_i", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{
		Res: index, Incoming: []ir.PhiIncoming{
			{Block: startBB, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		},
	})
	more := g.currentFn.NewValue("hdr_has_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	key := g.currentFn.NewValue("hdr_has_key", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: key, Array: entries, Index: index})
	equal := g.currentFn.NewValue("hdr_has_eq", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
		Res: equal, Callee: "ts_string_eq", Args: []ir.Operand{key, lowerName},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	bodyBB.Terminator = &ir.BranchTerm{Cond: equal, Then: retBB, Else: nextBB}

	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{
		Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2},
	})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}

	doneBB.Terminator = &ir.JumpTerm{Target: retBB}

	g.currentBB = retBB
	res := g.currentFn.NewValue("hdr_has_res", types.TypeBoolean)
	retBB.Phis = append(retBB.Phis, &ir.PhiInst{
		Res: res,
		Incoming: []ir.PhiIncoming{
			{Block: bodyBB, Value: ir.ConstBool{Value: true}},
			{Block: doneBB, Value: ir.ConstBool{Value: false}},
		},
	})
	return res
}

func (g *generator) lowerHeadersDelete(headers, name ir.Operand) {
	g.validateHeaderName(name)
	lowerName := g.stringAsciiLower(name)
	entries := g.headersEntries(headers)

	startBB := g.currentBB
	entriesType := types.NewArray(types.TypeString)
	nextEntries := g.currentFn.NewValue("hdr_del_entries", entriesType)
	startBB.Instructions = append(startBB.Instructions, &ir.AllocArrayInst{
		Res: nextEntries, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0},
	})
	length := g.currentFn.NewValue("hdr_del_len", types.TypeNumber)
	startBB.Instructions = append(startBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})

	condBB := g.currentFn.NewBlock("hdr_del_cond")
	bodyBB := g.currentFn.NewBlock("hdr_del_body")
	keepBB := g.currentFn.NewBlock("hdr_del_keep")
	nextBB := g.currentFn.NewBlock("hdr_del_next")
	doneBB := g.currentFn.NewBlock("hdr_del_done")

	startBB.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("hdr_del_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("hdr_del_next_i", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{
		Res: index,
		Incoming: []ir.PhiIncoming{
			{Block: startBB, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		},
	})
	more := g.currentFn.NewValue("hdr_del_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	key := g.currentFn.NewValue("hdr_del_k", types.TypeString)
	valIdx := g.currentFn.NewValue("hdr_del_val_i", types.TypeNumber)
	val := g.currentFn.NewValue("hdr_del_v", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.GetElementInst{Res: key, Array: entries, Index: index},
		&ir.BinaryInst{Res: valIdx, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}},
		&ir.GetElementInst{Res: val, Array: entries, Index: valIdx},
	)
	equal := g.currentFn.NewValue("hdr_del_eq", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
		Res: equal, Callee: "ts_string_eq", Args: []ir.Operand{key, lowerName},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	bodyBB.Terminator = &ir.BranchTerm{Cond: equal, Then: nextBB, Else: keepBB}

	g.currentBB = keepBB
	g.pushArrayOperand(nextEntries, key)
	g.pushArrayOperand(nextEntries, val)
	keepEnd := g.currentBB
	keepEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = nextBB
	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{
		Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2},
	})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = doneBB
	offsets, _, _ := g.objectLayout(g.semaResult.HeadersType)
	doneBB.Instructions = append(doneBB.Instructions, &ir.SetFieldInst{
		Obj: headers, Field: "$entries", Offset: offsets["$entries"], Val: nextEntries,
	})
}

func (g *generator) lowerHeadersSet(headers, name, value ir.Operand) {
	g.validateHeaderName(name)
	trimmed := g.trimHttpWhitespace(value)
	g.validateHeaderValue(trimmed)
	lowerName := g.stringAsciiLower(name)
	entries := g.headersEntries(headers)

	startBB := g.currentBB
	entriesType := types.NewArray(types.TypeString)
	nextEntries := g.currentFn.NewValue("hdr_set_entries", entriesType)
	startBB.Instructions = append(startBB.Instructions, &ir.AllocArrayInst{
		Res: nextEntries, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0},
	})
	length := g.currentFn.NewValue("hdr_set_len", types.TypeNumber)
	startBB.Instructions = append(startBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})

	condBB := g.currentFn.NewBlock("hdr_set_cond")
	bodyBB := g.currentFn.NewBlock("hdr_set_body")
	matchBB := g.currentFn.NewBlock("hdr_set_match")
	firstBB := g.currentFn.NewBlock("hdr_set_first")
	copyBB := g.currentFn.NewBlock("hdr_set_copy")
	nextBB := g.currentFn.NewBlock("hdr_set_next")
	doneBB := g.currentFn.NewBlock("hdr_set_done")
	appendBB := g.currentFn.NewBlock("hdr_set_append")
	storeBB := g.currentFn.NewBlock("hdr_set_store")

	startBB.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("hdr_set_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("hdr_set_next_i", types.TypeNumber)
	found := g.currentFn.NewValue("hdr_set_found", types.TypeBoolean)
	nextFound := g.currentFn.NewValue("hdr_set_next_found", types.TypeBoolean)

	condBB.Phis = append(condBB.Phis,
		&ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: startBB, Value: ir.ConstNumber{Value: 0}}, {Block: nextBB, Value: nextIndex}}},
		&ir.PhiInst{Res: found, Incoming: []ir.PhiIncoming{{Block: startBB, Value: ir.ConstBool{Value: false}}, {Block: nextBB, Value: nextFound}}},
	)
	more := g.currentFn.NewValue("hdr_set_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	key := g.currentFn.NewValue("hdr_set_k", types.TypeString)
	valIdx := g.currentFn.NewValue("hdr_set_val_i", types.TypeNumber)
	val := g.currentFn.NewValue("hdr_set_v", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.GetElementInst{Res: key, Array: entries, Index: index},
		&ir.BinaryInst{Res: valIdx, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}},
		&ir.GetElementInst{Res: val, Array: entries, Index: valIdx},
	)
	equal := g.currentFn.NewValue("hdr_set_eq", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
		Res: equal, Callee: "ts_string_eq", Args: []ir.Operand{key, lowerName},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	bodyBB.Terminator = &ir.BranchTerm{Cond: equal, Then: matchBB, Else: copyBB}

	g.currentBB = copyBB
	g.pushArrayOperand(nextEntries, key)
	g.pushArrayOperand(nextEntries, val)
	copyEnd := g.currentBB
	copyEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = matchBB
	matchBB.Terminator = &ir.BranchTerm{Cond: found, Then: nextBB, Else: firstBB}

	g.currentBB = firstBB
	g.pushArrayOperand(nextEntries, lowerName)
	g.pushArrayOperand(nextEntries, trimmed)
	firstEnd := g.currentBB
	firstEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = nextBB
	nextBB.Phis = append(nextBB.Phis, &ir.PhiInst{
		Res: nextFound,
		Incoming: []ir.PhiIncoming{
			{Block: copyEnd, Value: found},
			{Block: matchBB, Value: found},
			{Block: firstEnd, Value: ir.ConstBool{Value: true}},
		},
	})
	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{
		Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2},
	})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = doneBB
	doneBB.Terminator = &ir.BranchTerm{Cond: found, Then: storeBB, Else: appendBB}

	g.currentBB = appendBB
	g.pushArrayOperand(nextEntries, lowerName)
	g.pushArrayOperand(nextEntries, trimmed)
	appendEnd := g.currentBB
	appendEnd.Terminator = &ir.JumpTerm{Target: storeBB}

	g.currentBB = storeBB
	offsets, _, _ := g.objectLayout(g.semaResult.HeadersType)
	storeBB.Instructions = append(storeBB.Instructions, &ir.SetFieldInst{
		Obj: headers, Field: "$entries", Offset: offsets["$entries"], Val: nextEntries,
	})
}

func (g *generator) lowerHeadersGetSetCookie(headers ir.Operand) ir.Operand {
	entries := g.headersEntries(headers)
	resultType := types.NewArray(types.TypeString)
	cookies := g.currentFn.NewValue("hdr_cookies", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: cookies, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0},
	})

	startBB := g.currentBB
	length := g.currentFn.NewValue("hdr_cookies_len", types.TypeNumber)
	startBB.Instructions = append(startBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})

	condBB := g.currentFn.NewBlock("hdr_cookies_cond")
	bodyBB := g.currentFn.NewBlock("hdr_cookies_body")
	matchBB := g.currentFn.NewBlock("hdr_cookies_match")
	nextBB := g.currentFn.NewBlock("hdr_cookies_next")
	doneBB := g.currentFn.NewBlock("hdr_cookies_done")

	startBB.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("hdr_cookies_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("hdr_cookies_next_i", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{
		Res: index,
		Incoming: []ir.PhiIncoming{
			{Block: startBB, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		},
	})
	more := g.currentFn.NewValue("hdr_cookies_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	key := g.currentFn.NewValue("hdr_cookies_k", types.TypeString)
	valIdx := g.currentFn.NewValue("hdr_cookies_val_i", types.TypeNumber)
	val := g.currentFn.NewValue("hdr_cookies_v", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.GetElementInst{Res: key, Array: entries, Index: index},
		&ir.BinaryInst{Res: valIdx, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}},
		&ir.GetElementInst{Res: val, Array: entries, Index: valIdx},
	)
	equal := g.currentFn.NewValue("hdr_cookies_eq", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
		Res: equal, Callee: "ts_string_eq", Args: []ir.Operand{key, ir.ConstString{Value: "set-cookie"}},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	bodyBB.Terminator = &ir.BranchTerm{Cond: equal, Then: matchBB, Else: nextBB}

	g.currentBB = matchBB
	g.pushArrayOperand(cookies, val)
	matchEnd := g.currentBB
	matchEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = nextBB
	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{
		Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2},
	})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = doneBB
	return cookies
}

func (g *generator) lowerHeadersNormalizedPairs(headers ir.Operand) ir.Operand {
	entries := g.headersEntries(headers)

	startBB := g.currentBB
	entriesLen := g.currentFn.NewValue("hdr_pairs_entries_len", types.TypeNumber)
	startBB.Instructions = append(startBB.Instructions, &ir.ArrayLengthInst{Res: entriesLen, Array: entries})

	uniqueNamesType := types.NewArray(types.TypeString)
	uniqueNames := g.currentFn.NewValue("hdr_unique_names", uniqueNamesType)
	startBB.Instructions = append(startBB.Instructions, &ir.AllocArrayInst{
		Res: uniqueNames, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0},
	})

	// Step 1: Collect unique names in order of appearance
	uniqCond := g.currentFn.NewBlock("hdr_uniq_cond")
	uniqBody := g.currentFn.NewBlock("hdr_uniq_body")
	searchCond := g.currentFn.NewBlock("hdr_search_cond")
	searchBody := g.currentFn.NewBlock("hdr_search_body")
	searchNext := g.currentFn.NewBlock("hdr_search_next")
	searchDone := g.currentFn.NewBlock("hdr_search_done")
	uniqAppend := g.currentFn.NewBlock("hdr_uniq_append")
	uniqNext := g.currentFn.NewBlock("hdr_uniq_next")
	sortInit := g.currentFn.NewBlock("hdr_sort_init")

	startBB.Terminator = &ir.JumpTerm{Target: uniqCond}

	eIdx := g.currentFn.NewValue("hdr_uniq_e", types.TypeNumber)
	nextEIdx := g.currentFn.NewValue("hdr_uniq_next_e", types.TypeNumber)
	uniqCond.Phis = append(uniqCond.Phis, &ir.PhiInst{
		Res: eIdx,
		Incoming: []ir.PhiIncoming{
			{Block: startBB, Value: ir.ConstNumber{Value: 0}},
			{Block: uniqNext, Value: nextEIdx},
		},
	})
	moreEntries := g.currentFn.NewValue("hdr_uniq_more", types.TypeBoolean)
	uniqCond.Instructions = append(uniqCond.Instructions, &ir.BinaryInst{
		Res: moreEntries, Op: ir.OpLt, LHS: eIdx, RHS: entriesLen,
	})
	uniqCond.Terminator = &ir.BranchTerm{Cond: moreEntries, Then: uniqBody, Else: sortInit}

	g.currentBB = uniqBody
	eKey := g.currentFn.NewValue("hdr_uniq_key", types.TypeString)
	uniqBody.Instructions = append(uniqBody.Instructions, &ir.GetElementInst{Res: eKey, Array: entries, Index: eIdx})
	uniqBody.Terminator = &ir.JumpTerm{Target: searchCond}

	uIdx := g.currentFn.NewValue("hdr_search_u", types.TypeNumber)
	nextUIdx := g.currentFn.NewValue("hdr_search_next_u", types.TypeNumber)
	searchCond.Phis = append(searchCond.Phis, &ir.PhiInst{
		Res: uIdx,
		Incoming: []ir.PhiIncoming{
			{Block: uniqBody, Value: ir.ConstNumber{Value: 0}},
			{Block: searchNext, Value: nextUIdx},
		},
	})
	uniqLen := g.currentFn.NewValue("hdr_uniq_len", types.TypeNumber)
	searchCond.Instructions = append(searchCond.Instructions, &ir.ArrayLengthInst{Res: uniqLen, Array: uniqueNames})
	moreUniq := g.currentFn.NewValue("hdr_search_more", types.TypeBoolean)
	searchCond.Instructions = append(searchCond.Instructions, &ir.BinaryInst{Res: moreUniq, Op: ir.OpLt, LHS: uIdx, RHS: uniqLen})
	searchCond.Terminator = &ir.BranchTerm{Cond: moreUniq, Then: searchBody, Else: searchDone}

	g.currentBB = searchBody
	uKey := g.currentFn.NewValue("hdr_search_ukey", types.TypeString)
	searchBody.Instructions = append(searchBody.Instructions, &ir.GetElementInst{Res: uKey, Array: uniqueNames, Index: uIdx})
	uEq := g.currentFn.NewValue("hdr_search_ueq", types.TypeBoolean)
	searchBody.Instructions = append(searchBody.Instructions, &ir.CallInst{
		Res: uEq, Callee: "ts_string_eq", Args: []ir.Operand{eKey, uKey},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	searchBody.Terminator = &ir.BranchTerm{Cond: uEq, Then: uniqNext, Else: searchNext}

	g.currentBB = searchNext
	searchNext.Instructions = append(searchNext.Instructions, &ir.BinaryInst{
		Res: nextUIdx, Op: ir.OpAdd, LHS: uIdx, RHS: ir.ConstNumber{Value: 1},
	})
	searchNext.Terminator = &ir.JumpTerm{Target: searchCond}

	searchDone.Terminator = &ir.JumpTerm{Target: uniqAppend}

	g.currentBB = uniqAppend
	g.pushArrayOperand(uniqueNames, eKey)
	appendEnd := g.currentBB
	appendEnd.Terminator = &ir.JumpTerm{Target: uniqNext}

	g.currentBB = uniqNext
	uniqNext.Instructions = append(uniqNext.Instructions, &ir.BinaryInst{
		Res: nextEIdx, Op: ir.OpAdd, LHS: eIdx, RHS: ir.ConstNumber{Value: 2},
	})
	uniqNext.Terminator = &ir.JumpTerm{Target: uniqCond}

	// Step 2: Sort uniqueNames in ascending UTF-16 code unit order.
	outerCond := g.currentFn.NewBlock("hdr_sort_outer_cond")
	outerBody := g.currentFn.NewBlock("hdr_sort_outer_body")
	innerCond := g.currentFn.NewBlock("hdr_sort_inner_cond")
	innerBody := g.currentFn.NewBlock("hdr_sort_inner_body")
	shiftBB := g.currentFn.NewBlock("hdr_sort_shift")
	insertBB := g.currentFn.NewBlock("hdr_sort_insert")
	outerNext := g.currentFn.NewBlock("hdr_sort_outer_next")
	buildInit := g.currentFn.NewBlock("hdr_build_init")

	sortInit.Terminator = &ir.JumpTerm{Target: outerCond}

	sortI := g.currentFn.NewValue("hdr_sort_i", types.TypeNumber)
	nextSortI := g.currentFn.NewValue("hdr_sort_next_i", types.TypeNumber)
	outerCond.Phis = append(outerCond.Phis, &ir.PhiInst{
		Res: sortI,
		Incoming: []ir.PhiIncoming{
			{Block: sortInit, Value: ir.ConstNumber{Value: 1}},
			{Block: outerNext, Value: nextSortI},
		},
	})
	totalUniq := g.currentFn.NewValue("hdr_sort_len", types.TypeNumber)
	outerCond.Instructions = append(outerCond.Instructions, &ir.ArrayLengthInst{Res: totalUniq, Array: uniqueNames})
	moreSort := g.currentFn.NewValue("hdr_sort_more", types.TypeBoolean)
	outerCond.Instructions = append(outerCond.Instructions, &ir.BinaryInst{Res: moreSort, Op: ir.OpLt, LHS: sortI, RHS: totalUniq})
	outerCond.Terminator = &ir.BranchTerm{Cond: moreSort, Then: outerBody, Else: buildInit}

	g.currentBB = outerBody
	keyName := g.currentFn.NewValue("hdr_sort_key_name", types.TypeString)
	outerBody.Instructions = append(outerBody.Instructions, &ir.GetElementInst{Res: keyName, Array: uniqueNames, Index: sortI})
	outerBody.Terminator = &ir.JumpTerm{Target: innerCond}

	sortJ := g.currentFn.NewValue("hdr_sort_j", types.TypeNumber)
	prevJ := g.currentFn.NewValue("hdr_sort_prev_j", types.TypeNumber)
	innerCond.Phis = append(innerCond.Phis, &ir.PhiInst{
		Res: sortJ,
		Incoming: []ir.PhiIncoming{
			{Block: outerBody, Value: sortI},
			{Block: shiftBB, Value: prevJ},
		},
	})
	hasPrev := g.currentFn.NewValue("hdr_sort_has_prev", types.TypeBoolean)
	innerCond.Instructions = append(innerCond.Instructions, &ir.BinaryInst{Res: hasPrev, Op: ir.OpGt, LHS: sortJ, RHS: ir.ConstNumber{Value: 0}})
	innerCond.Terminator = &ir.BranchTerm{Cond: hasPrev, Then: innerBody, Else: insertBB}

	g.currentBB = innerBody
	prevIndex := g.currentFn.NewValue("hdr_sort_prev_i", types.TypeNumber)
	innerBody.Instructions = append(innerBody.Instructions, &ir.BinaryInst{Res: prevIndex, Op: ir.OpSub, LHS: sortJ, RHS: ir.ConstNumber{Value: 1}})
	prevName := g.currentFn.NewValue("hdr_sort_prev_name", types.TypeString)
	innerBody.Instructions = append(innerBody.Instructions, &ir.GetElementInst{Res: prevName, Array: uniqueNames, Index: prevIndex})

	prevBox := g.boxJSValue(prevName, types.TypeString)
	keyBox := g.boxJSValue(keyName, types.TypeString)
	greater := g.currentFn.NewValue("hdr_sort_greater", types.TypeBoolean)
	innerBody.Instructions = append(innerBody.Instructions, &ir.CallInst{
		Res: greater, Callee: "ts_js_gt", Args: []ir.Operand{prevBox, keyBox}, ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
	})
	innerBody.Terminator = &ir.BranchTerm{Cond: greater, Then: shiftBB, Else: insertBB}

	g.currentBB = shiftBB
	shiftBB.Instructions = append(shiftBB.Instructions,
		&ir.SetElementInst{Array: uniqueNames, Index: sortJ, Val: prevName},
		&ir.BinaryInst{Res: prevJ, Op: ir.OpSub, LHS: sortJ, RHS: ir.ConstNumber{Value: 1}},
	)
	shiftBB.Terminator = &ir.JumpTerm{Target: innerCond}

	g.currentBB = insertBB
	insertBB.Instructions = append(insertBB.Instructions, &ir.SetElementInst{Array: uniqueNames, Index: sortJ, Val: keyName})
	insertEnd := g.currentBB
	insertEnd.Terminator = &ir.JumpTerm{Target: outerNext}

	g.currentBB = outerNext
	outerNext.Instructions = append(outerNext.Instructions, &ir.BinaryInst{Res: nextSortI, Op: ir.OpAdd, LHS: sortI, RHS: ir.ConstNumber{Value: 1}})
	outerNext.Terminator = &ir.JumpTerm{Target: outerCond}

	// Step 3: Build pairs array
	g.currentBB = buildInit
	pairType := types.NewArray(types.TypeString)
	pairsType := types.NewArray(pairType)
	pairs := g.currentFn.NewValue("hdr_pairs", pairsType)
	buildInit.Instructions = append(buildInit.Instructions, &ir.AllocArrayInst{
		Res: pairs, ElemType: pairType, Length: ir.ConstNumber{Value: 0},
	})

	namesCond := g.currentFn.NewBlock("hdr_names_cond")
	namesBody := g.currentFn.NewBlock("hdr_names_body")
	cookieBB := g.currentFn.NewBlock("hdr_names_cookie")
	regularBB := g.currentFn.NewBlock("hdr_names_regular")
	namesNext := g.currentFn.NewBlock("hdr_names_next")
	buildDone := g.currentFn.NewBlock("hdr_build_done")

	buildInit.Terminator = &ir.JumpTerm{Target: namesCond}

	nIdx := g.currentFn.NewValue("hdr_names_i", types.TypeNumber)
	nextNIdx := g.currentFn.NewValue("hdr_names_next_i", types.TypeNumber)
	namesCond.Phis = append(namesCond.Phis, &ir.PhiInst{
		Res: nIdx,
		Incoming: []ir.PhiIncoming{
			{Block: buildInit, Value: ir.ConstNumber{Value: 0}},
			{Block: namesNext, Value: nextNIdx},
		},
	})
	namesLen := g.currentFn.NewValue("hdr_names_len", types.TypeNumber)
	namesCond.Instructions = append(namesCond.Instructions, &ir.ArrayLengthInst{Res: namesLen, Array: uniqueNames})
	moreNames := g.currentFn.NewValue("hdr_names_more", types.TypeBoolean)
	namesCond.Instructions = append(namesCond.Instructions, &ir.BinaryInst{Res: moreNames, Op: ir.OpLt, LHS: nIdx, RHS: namesLen})
	namesCond.Terminator = &ir.BranchTerm{Cond: moreNames, Then: namesBody, Else: buildDone}

	g.currentBB = namesBody
	currentName := g.currentFn.NewValue("hdr_names_name", types.TypeString)
	namesBody.Instructions = append(namesBody.Instructions, &ir.GetElementInst{Res: currentName, Array: uniqueNames, Index: nIdx})
	isCookie := g.currentFn.NewValue("hdr_names_is_cookie", types.TypeBoolean)
	namesBody.Instructions = append(namesBody.Instructions, &ir.CallInst{
		Res: isCookie, Callee: "ts_string_eq", Args: []ir.Operand{currentName, ir.ConstString{Value: "set-cookie"}},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	namesBody.Terminator = &ir.BranchTerm{Cond: isCookie, Then: cookieBB, Else: regularBB}

	// set-cookie: yield each entry as separate pair
	g.currentBB = cookieBB
	cCond := g.currentFn.NewBlock("hdr_cookie_cond")
	cBody := g.currentFn.NewBlock("hdr_cookie_body")
	cMatch := g.currentFn.NewBlock("hdr_cookie_match")
	cNext := g.currentFn.NewBlock("hdr_cookie_next")
	cDone := g.currentFn.NewBlock("hdr_cookie_done")

	cookieBB.Terminator = &ir.JumpTerm{Target: cCond}

	cIdx := g.currentFn.NewValue("hdr_cookie_i", types.TypeNumber)
	nextCIdx := g.currentFn.NewValue("hdr_cookie_next_i", types.TypeNumber)
	cCond.Phis = append(cCond.Phis, &ir.PhiInst{
		Res: cIdx,
		Incoming: []ir.PhiIncoming{
			{Block: cookieBB, Value: ir.ConstNumber{Value: 0}},
			{Block: cNext, Value: nextCIdx},
		},
	})
	moreCookies := g.currentFn.NewValue("hdr_cookie_more", types.TypeBoolean)
	cCond.Instructions = append(cCond.Instructions, &ir.BinaryInst{Res: moreCookies, Op: ir.OpLt, LHS: cIdx, RHS: entriesLen})
	cCond.Terminator = &ir.BranchTerm{Cond: moreCookies, Then: cBody, Else: cDone}

	g.currentBB = cBody
	cKey := g.currentFn.NewValue("hdr_cookie_k", types.TypeString)
	cValI := g.currentFn.NewValue("hdr_cookie_val_i", types.TypeNumber)
	cVal := g.currentFn.NewValue("hdr_cookie_v", types.TypeString)
	cBody.Instructions = append(cBody.Instructions,
		&ir.GetElementInst{Res: cKey, Array: entries, Index: cIdx},
		&ir.BinaryInst{Res: cValI, Op: ir.OpAdd, LHS: cIdx, RHS: ir.ConstNumber{Value: 1}},
		&ir.GetElementInst{Res: cVal, Array: entries, Index: cValI},
	)
	cEq := g.currentFn.NewValue("hdr_cookie_eq", types.TypeBoolean)
	cBody.Instructions = append(cBody.Instructions, &ir.CallInst{
		Res: cEq, Callee: "ts_string_eq", Args: []ir.Operand{cKey, ir.ConstString{Value: "set-cookie"}},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	cBody.Terminator = &ir.BranchTerm{Cond: cEq, Then: cMatch, Else: cNext}

	g.currentBB = cMatch
	cPair := g.currentFn.NewValue("hdr_cookie_pair", pairType)
	cMatch.Instructions = append(cMatch.Instructions,
		&ir.AllocArrayInst{Res: cPair, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 2}},
		&ir.SetElementInst{Array: cPair, Index: ir.ConstNumber{Value: 0}, Val: ir.ConstString{Value: "set-cookie"}},
		&ir.SetElementInst{Array: cPair, Index: ir.ConstNumber{Value: 1}, Val: cVal},
	)
	g.pushArrayOperand(pairs, cPair)
	cMatchEnd := g.currentBB
	cMatchEnd.Terminator = &ir.JumpTerm{Target: cNext}

	g.currentBB = cNext
	cNext.Instructions = append(cNext.Instructions, &ir.BinaryInst{Res: nextCIdx, Op: ir.OpAdd, LHS: cIdx, RHS: ir.ConstNumber{Value: 2}})
	cNext.Terminator = &ir.JumpTerm{Target: cCond}

	cDone.Terminator = &ir.JumpTerm{Target: namesNext}

	// Regular headers: combine all values with ", "
	g.currentBB = regularBB
	rCond := g.currentFn.NewBlock("hdr_reg_cond")
	rBody := g.currentFn.NewBlock("hdr_reg_body")
	rMatch := g.currentFn.NewBlock("hdr_reg_match")
	rFirst := g.currentFn.NewBlock("hdr_reg_first")
	rJoin := g.currentFn.NewBlock("hdr_reg_join")
	rNext := g.currentFn.NewBlock("hdr_reg_next")
	rDone := g.currentFn.NewBlock("hdr_reg_done")

	regularBB.Terminator = &ir.JumpTerm{Target: rCond}

	rIdx := g.currentFn.NewValue("hdr_reg_i", types.TypeNumber)
	nextRIdx := g.currentFn.NewValue("hdr_reg_next_i", types.TypeNumber)
	rAcc := g.currentFn.NewValue("hdr_reg_acc", types.TypeString)
	nextRAcc := g.currentFn.NewValue("hdr_reg_next_acc", types.TypeString)
	rFound := g.currentFn.NewValue("hdr_reg_found", types.TypeBoolean)
	nextRFound := g.currentFn.NewValue("hdr_reg_next_found", types.TypeBoolean)

	rCond.Phis = append(rCond.Phis,
		&ir.PhiInst{Res: rIdx, Incoming: []ir.PhiIncoming{{Block: regularBB, Value: ir.ConstNumber{Value: 0}}, {Block: rNext, Value: nextRIdx}}},
		&ir.PhiInst{Res: rAcc, Incoming: []ir.PhiIncoming{{Block: regularBB, Value: ir.ConstString{Value: ""}}, {Block: rNext, Value: nextRAcc}}},
		&ir.PhiInst{Res: rFound, Incoming: []ir.PhiIncoming{{Block: regularBB, Value: ir.ConstBool{Value: false}}, {Block: rNext, Value: nextRFound}}},
	)
	moreReg := g.currentFn.NewValue("hdr_reg_more", types.TypeBoolean)
	rCond.Instructions = append(rCond.Instructions, &ir.BinaryInst{Res: moreReg, Op: ir.OpLt, LHS: rIdx, RHS: entriesLen})
	rCond.Terminator = &ir.BranchTerm{Cond: moreReg, Then: rBody, Else: rDone}

	g.currentBB = rBody
	rKey := g.currentFn.NewValue("hdr_reg_k", types.TypeString)
	rValI := g.currentFn.NewValue("hdr_reg_val_i", types.TypeNumber)
	rVal := g.currentFn.NewValue("hdr_reg_v", types.TypeString)
	rBody.Instructions = append(rBody.Instructions,
		&ir.GetElementInst{Res: rKey, Array: entries, Index: rIdx},
		&ir.BinaryInst{Res: rValI, Op: ir.OpAdd, LHS: rIdx, RHS: ir.ConstNumber{Value: 1}},
		&ir.GetElementInst{Res: rVal, Array: entries, Index: rValI},
	)
	rEq := g.currentFn.NewValue("hdr_reg_eq", types.TypeBoolean)
	rBody.Instructions = append(rBody.Instructions, &ir.CallInst{
		Res: rEq, Callee: "ts_string_eq", Args: []ir.Operand{rKey, currentName},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	rBody.Terminator = &ir.BranchTerm{Cond: rEq, Then: rMatch, Else: rNext}

	g.currentBB = rMatch
	rMatch.Terminator = &ir.BranchTerm{Cond: rFound, Then: rJoin, Else: rFirst}

	g.currentBB = rFirst
	rFirstEnd := g.currentBB
	rFirstEnd.Terminator = &ir.JumpTerm{Target: rNext}

	g.currentBB = rJoin
	rJoined := g.concatNativeStrings(g.concatNativeStrings(rAcc, ir.ConstString{Value: ", "}), rVal)
	rJoinEnd := g.currentBB
	rJoinEnd.Terminator = &ir.JumpTerm{Target: rNext}

	g.currentBB = rNext
	rNext.Phis = append(rNext.Phis,
		&ir.PhiInst{Res: nextRAcc, Incoming: []ir.PhiIncoming{
			{Block: rBody, Value: rAcc},
			{Block: rFirstEnd, Value: rVal},
			{Block: rJoinEnd, Value: rJoined},
		}},
		&ir.PhiInst{Res: nextRFound, Incoming: []ir.PhiIncoming{
			{Block: rBody, Value: rFound},
			{Block: rFirstEnd, Value: ir.ConstBool{Value: true}},
			{Block: rJoinEnd, Value: ir.ConstBool{Value: true}},
		}},
	)
	rNext.Instructions = append(rNext.Instructions, &ir.BinaryInst{Res: nextRIdx, Op: ir.OpAdd, LHS: rIdx, RHS: ir.ConstNumber{Value: 2}})
	rNext.Terminator = &ir.JumpTerm{Target: rCond}

	g.currentBB = rDone
	rPair := g.currentFn.NewValue("hdr_reg_pair", pairType)
	rDone.Instructions = append(rDone.Instructions,
		&ir.AllocArrayInst{Res: rPair, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 2}},
		&ir.SetElementInst{Array: rPair, Index: ir.ConstNumber{Value: 0}, Val: currentName},
		&ir.SetElementInst{Array: rPair, Index: ir.ConstNumber{Value: 1}, Val: rAcc},
	)
	g.pushArrayOperand(pairs, rPair)
	rDoneEnd := g.currentBB
	rDoneEnd.Terminator = &ir.JumpTerm{Target: namesNext}

	g.currentBB = namesNext
	namesNext.Instructions = append(namesNext.Instructions, &ir.BinaryInst{Res: nextNIdx, Op: ir.OpAdd, LHS: nIdx, RHS: ir.ConstNumber{Value: 1}})
	namesNext.Terminator = &ir.JumpTerm{Target: namesCond}

	g.currentBB = buildDone
	return pairs
}

func (g *generator) lowerHeadersEntries(headers ir.Operand) ir.Operand {
	return g.lowerHeadersNormalizedPairs(headers)
}

func (g *generator) lowerHeadersKeys(headers ir.Operand) ir.Operand {
	pairs := g.lowerHeadersNormalizedPairs(headers)
	resultType := types.NewArray(types.TypeString)
	keys := g.currentFn.NewValue("hdr_keys", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: keys, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0},
	})

	startBB := g.currentBB
	length := g.currentFn.NewValue("hdr_keys_len", types.TypeNumber)
	startBB.Instructions = append(startBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: pairs})

	condBB := g.currentFn.NewBlock("hdr_keys_cond")
	bodyBB := g.currentFn.NewBlock("hdr_keys_body")
	nextBB := g.currentFn.NewBlock("hdr_keys_next")
	doneBB := g.currentFn.NewBlock("hdr_keys_done")

	startBB.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("hdr_keys_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("hdr_keys_next_i", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{
		Res: index,
		Incoming: []ir.PhiIncoming{
			{Block: startBB, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		},
	})
	more := g.currentFn.NewValue("hdr_keys_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	pairType := types.NewArray(types.TypeString)
	pair := g.currentFn.NewValue("hdr_keys_pair", pairType)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: pair, Array: pairs, Index: index})
	key := g.currentFn.NewValue("hdr_keys_k", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: key, Array: pair, Index: ir.ConstNumber{Value: 0}})
	g.pushArrayOperand(keys, key)
	bodyEnd := g.currentBB
	bodyEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = nextBB
	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = doneBB
	return keys
}

func (g *generator) lowerHeadersValues(headers ir.Operand) ir.Operand {
	pairs := g.lowerHeadersNormalizedPairs(headers)
	resultType := types.NewArray(types.TypeString)
	vals := g.currentFn.NewValue("hdr_values", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: vals, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0},
	})

	startBB := g.currentBB
	length := g.currentFn.NewValue("hdr_values_len", types.TypeNumber)
	startBB.Instructions = append(startBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: pairs})

	condBB := g.currentFn.NewBlock("hdr_values_cond")
	bodyBB := g.currentFn.NewBlock("hdr_values_body")
	nextBB := g.currentFn.NewBlock("hdr_values_next")
	doneBB := g.currentFn.NewBlock("hdr_values_done")

	startBB.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("hdr_values_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("hdr_values_next_i", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{
		Res: index,
		Incoming: []ir.PhiIncoming{
			{Block: startBB, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		},
	})
	more := g.currentFn.NewValue("hdr_values_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	pairType := types.NewArray(types.TypeString)
	pair := g.currentFn.NewValue("hdr_values_pair", pairType)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: pair, Array: pairs, Index: index})
	val := g.currentFn.NewValue("hdr_values_v", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: val, Array: pair, Index: ir.ConstNumber{Value: 1}})
	g.pushArrayOperand(vals, val)
	bodyEnd := g.currentBB
	bodyEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = nextBB
	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = doneBB
	return vals
}

func (g *generator) lowerHeadersForEach(headers, cb, thisArg ir.Operand) {
	pairs := g.lowerHeadersNormalizedPairs(headers)
	startBB := g.currentBB
	length := g.currentFn.NewValue("hdr_each_len", types.TypeNumber)
	startBB.Instructions = append(startBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: pairs})

	condBB := g.currentFn.NewBlock("hdr_each_cond")
	bodyBB := g.currentFn.NewBlock("hdr_each_body")
	nextBB := g.currentFn.NewBlock("hdr_each_next")
	doneBB := g.currentFn.NewBlock("hdr_each_done")

	startBB.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("hdr_each_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("hdr_each_next_i", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{
		Res: index,
		Incoming: []ir.PhiIncoming{
			{Block: startBB, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		},
	})
	more := g.currentFn.NewValue("hdr_each_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	pairType := types.NewArray(types.TypeString)
	pair := g.currentFn.NewValue("hdr_each_pair", pairType)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: pair, Array: pairs, Index: index})
	k := g.currentFn.NewValue("hdr_each_k", types.TypeString)
	v := g.currentFn.NewValue("hdr_each_v", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.GetElementInst{Res: k, Array: pair, Index: ir.ConstNumber{Value: 0}},
		&ir.GetElementInst{Res: v, Array: pair, Index: ir.ConstNumber{Value: 1}},
	)
	boxedHeaders := g.boxJSValue(headers, g.semaResult.HeadersType)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.IndirectCallInst{
		Closure:    cb,
		ThisArg:    thisArg,
		Args:       []ir.Operand{v, k, boxedHeaders},
		ParamTypes: []types.Type{types.TypeString, types.TypeString, types.TypeAny},
	})
	bodyEnd := g.currentBB
	bodyEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = nextBB
	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = doneBB
}

func (g *generator) lowerHeadersMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$Headers" {
		return nil, false
	}
	headers := g.lowerExpr(mem.Object)
	switch mem.Property {
	case "append":
		name := g.coerceStringType(g.semanticType(e.Args[0]), g.lowerExpr(e.Args[0]))
		val := g.coerceStringType(g.semanticType(e.Args[1]), g.lowerExpr(e.Args[1]))
		g.lowerHeadersAppendDirect(headers, name, val)
		return ir.ConstUndefined{}, true
	case "delete":
		name := g.coerceStringType(g.semanticType(e.Args[0]), g.lowerExpr(e.Args[0]))
		g.lowerHeadersDelete(headers, name)
		return ir.ConstUndefined{}, true
	case "get":
		name := g.coerceStringType(g.semanticType(e.Args[0]), g.lowerExpr(e.Args[0]))
		return g.lowerHeadersGet(headers, name), true
	case "getSetCookie":
		return g.lowerHeadersGetSetCookie(headers), true
	case "has":
		name := g.coerceStringType(g.semanticType(e.Args[0]), g.lowerExpr(e.Args[0]))
		return g.lowerHeadersHas(headers, name), true
	case "set":
		name := g.coerceStringType(g.semanticType(e.Args[0]), g.lowerExpr(e.Args[0]))
		val := g.coerceStringType(g.semanticType(e.Args[1]), g.lowerExpr(e.Args[1]))
		g.lowerHeadersSet(headers, name, val)
		return ir.ConstUndefined{}, true
	case "keys":
		return g.lowerHeadersKeys(headers), true
	case "values":
		return g.lowerHeadersValues(headers), true
	case "entries":
		return g.lowerHeadersEntries(headers), true
	case "forEach":
		cb := g.lowerExpr(e.Args[0])
		var thisArg ir.Operand
		if fnType, ok := g.semanticType(e.Args[0]).(*types.FunctionType); ok && fnType.This != nil && len(e.Args) > 1 {
			thisArg = g.lowerExpr(e.Args[1])
		}
		g.lowerHeadersForEach(headers, cb, thisArg)
		return ir.ConstUndefined{}, true
	}
	return nil, false
}
