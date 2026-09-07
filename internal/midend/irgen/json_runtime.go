package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

const (
	jsonSkipWhitespaceFunc = "$json_skip_whitespace"
	jsonHexNibbleFunc      = "$json_hex_nibble"
	jsonParseHex4Func      = "$json_parse_hex4"
	jsonParseStringFunc    = "$json_parse_string"
	jsonParseValueFunc     = "$json_parse_value"
)

func (g *generator) jsonStateType() *types.ObjectType {
	if g.jsonParserStateType == nil {
		t := types.NewObject("$JSONParserState")
		t.AddField("text", types.TypeString, false)
		t.AddField("index", types.TypeNumber, false)
		t.AddField("ok", types.TypeBoolean, false)
		g.jsonParserStateType = t
	}
	return g.jsonParserStateType
}

func (g *generator) jsonStateField(state ir.Operand, name string, t types.Type) ir.Operand {
	offsets, _, _ := g.objectLayout(g.jsonStateType())
	res := g.currentFn.NewValue("json_state_"+name, t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
		Res: res, Obj: state, Field: name, Offset: offsets[name],
	})
	return res
}

func (g *generator) setJSONStateField(state ir.Operand, name string, value ir.Operand) {
	offsets, _, _ := g.objectLayout(g.jsonStateType())
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: state, Field: name, Offset: offsets[name], Val: value,
	})
}

func (g *generator) emitJSONGeneratedFunction(name string, returnType types.Type, build func(state ir.Operand)) {
	outerFn, outerBB := g.currentFn, g.currentBB
	outerLocals, outerProv, outerDirect := g.locals, g.localProvenance, g.localDirectCallee

	fn := ir.NewFunction(name, returnType)
	state := fn.NewValue("state", g.jsonStateType())
	fn.Params = append(fn.Params, state)
	g.currentFn = fn
	g.currentBB = fn.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	build(state)
	g.prog.Functions = append(g.prog.Functions, fn)

	g.currentFn, g.currentBB = outerFn, outerBB
	g.locals, g.localProvenance, g.localDirectCallee = outerLocals, outerProv, outerDirect
}

func (g *generator) emitJSONSkipWhitespace() {
	g.emitJSONGeneratedFunction(jsonSkipWhitespaceFunc, types.TypeVoid, func(state ir.Operand) {
		entry := g.currentBB
		cond := g.currentFn.NewBlock("json_ws_cond")
		body := g.currentFn.NewBlock("json_ws_body")
		advance := g.currentFn.NewBlock("json_ws_advance")
		done := g.currentFn.NewBlock("json_ws_done")
		entry.Terminator = &ir.JumpTerm{Target: cond}

		g.currentBB = cond
		text := g.jsonStateField(state, "text", types.TypeString)
		index := g.jsonStateField(state, "index", types.TypeNumber)
		length := g.urlStringLen(text)
		more := g.currentFn.NewValue("json_ws_more", types.TypeBoolean)
		cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
		cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: done}

		g.currentBB = body
		ch := g.urlStringByteAt(text, index)
		var isWS ir.Operand
		for i, code := range []float64{' ', '\t', '\n', '\r'} {
			eq := g.currentFn.NewValue("json_ws_eq", types.TypeBoolean)
			body.Instructions = append(body.Instructions, &ir.BinaryInst{Res: eq, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: code}})
			if i == 0 {
				isWS = eq
			} else {
				any := g.currentFn.NewValue("json_ws_any", types.TypeBoolean)
				body.Instructions = append(body.Instructions, &ir.BinaryInst{Res: any, Op: ir.OpOr, LHS: isWS, RHS: eq})
				isWS = any
			}
		}
		body.Terminator = &ir.BranchTerm{Cond: isWS, Then: advance, Else: done}

		g.currentBB = advance
		next := g.currentFn.NewValue("json_ws_next", types.TypeNumber)
		advance.Instructions = append(advance.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
		g.setJSONStateField(state, "index", next)
		advance.Terminator = &ir.JumpTerm{Target: cond}

		g.currentBB = done
		done.Terminator = &ir.ReturnTerm{}
	})
}

func (g *generator) emitJSONHexNibble() {
	g.emitJSONGeneratedFunction(jsonHexNibbleFunc, types.TypeNumber, func(state ir.Operand) {
		// This helper reuses state.index as the byte offset and returns the nibble
		// at that position, or -1 when the byte is not hexadecimal. The caller
		// controls index advancement so string escape parsing remains explicit.
		text := g.jsonStateField(state, "text", types.TypeString)
		index := g.jsonStateField(state, "index", types.TypeNumber)
		ch := g.urlStringByteAt(text, index)
		digitCheck := g.currentBB
		digit := g.currentFn.NewBlock("json_hex_digit")
		upperCheck := g.currentFn.NewBlock("json_hex_upper_check")
		upper := g.currentFn.NewBlock("json_hex_upper")
		lowerCheck := g.currentFn.NewBlock("json_hex_lower_check")
		lower := g.currentFn.NewBlock("json_hex_lower")
		invalid := g.currentFn.NewBlock("json_hex_invalid")

		ge0 := g.currentFn.NewValue("json_hex_ge0", types.TypeBoolean)
		le9 := g.currentFn.NewValue("json_hex_le9", types.TypeBoolean)
		isDigit := g.currentFn.NewValue("json_hex_digit_ok", types.TypeBoolean)
		digitCheck.Instructions = append(digitCheck.Instructions,
			&ir.BinaryInst{Res: ge0, Op: ir.OpGe, LHS: ch, RHS: ir.ConstNumber{Value: '0'}},
			&ir.BinaryInst{Res: le9, Op: ir.OpLe, LHS: ch, RHS: ir.ConstNumber{Value: '9'}},
			&ir.BinaryInst{Res: isDigit, Op: ir.OpAnd, LHS: ge0, RHS: le9},
		)
		digitCheck.Terminator = &ir.BranchTerm{Cond: isDigit, Then: digit, Else: upperCheck}
		digitVal := g.currentFn.NewValue("json_hex_digit_value", types.TypeNumber)
		digit.Instructions = append(digit.Instructions, &ir.BinaryInst{Res: digitVal, Op: ir.OpSub, LHS: ch, RHS: ir.ConstNumber{Value: '0'}})
		digit.Terminator = &ir.ReturnTerm{Val: digitVal}

		g.currentBB = upperCheck
		geA := g.currentFn.NewValue("json_hex_geA", types.TypeBoolean)
		leF := g.currentFn.NewValue("json_hex_leF", types.TypeBoolean)
		isUpper := g.currentFn.NewValue("json_hex_upper_ok", types.TypeBoolean)
		upperCheck.Instructions = append(upperCheck.Instructions,
			&ir.BinaryInst{Res: geA, Op: ir.OpGe, LHS: ch, RHS: ir.ConstNumber{Value: 'A'}},
			&ir.BinaryInst{Res: leF, Op: ir.OpLe, LHS: ch, RHS: ir.ConstNumber{Value: 'F'}},
			&ir.BinaryInst{Res: isUpper, Op: ir.OpAnd, LHS: geA, RHS: leF},
		)
		upperCheck.Terminator = &ir.BranchTerm{Cond: isUpper, Then: upper, Else: lowerCheck}
		upperVal := g.currentFn.NewValue("json_hex_upper_value", types.TypeNumber)
		upper.Instructions = append(upper.Instructions,
			&ir.BinaryInst{Res: upperVal, Op: ir.OpSub, LHS: ch, RHS: ir.ConstNumber{Value: 'A' - 10}},
		)
		upper.Terminator = &ir.ReturnTerm{Val: upperVal}

		g.currentBB = lowerCheck
		gea := g.currentFn.NewValue("json_hex_gea", types.TypeBoolean)
		lef := g.currentFn.NewValue("json_hex_lef", types.TypeBoolean)
		isLower := g.currentFn.NewValue("json_hex_lower_ok", types.TypeBoolean)
		lowerCheck.Instructions = append(lowerCheck.Instructions,
			&ir.BinaryInst{Res: gea, Op: ir.OpGe, LHS: ch, RHS: ir.ConstNumber{Value: 'a'}},
			&ir.BinaryInst{Res: lef, Op: ir.OpLe, LHS: ch, RHS: ir.ConstNumber{Value: 'f'}},
			&ir.BinaryInst{Res: isLower, Op: ir.OpAnd, LHS: gea, RHS: lef},
		)
		lowerCheck.Terminator = &ir.BranchTerm{Cond: isLower, Then: lower, Else: invalid}
		lowerVal := g.currentFn.NewValue("json_hex_lower_value", types.TypeNumber)
		lower.Instructions = append(lower.Instructions,
			&ir.BinaryInst{Res: lowerVal, Op: ir.OpSub, LHS: ch, RHS: ir.ConstNumber{Value: 'a' - 10}},
		)
		lower.Terminator = &ir.ReturnTerm{Val: lowerVal}
		invalid.Terminator = &ir.ReturnTerm{Val: ir.ConstNumber{Value: -1}}
		g.currentBB = invalid
	})
}

func (g *generator) emitJSONParseHex4() {
	g.emitJSONGeneratedFunction(jsonParseHex4Func, types.TypeNumber, func(state ir.Operand) {
		values := make([]ir.Operand, 0, 4)
		for i := 0; i < 4; i++ {
			step := g.currentBB
			nibble := g.currentFn.NewValue("json_hex4_nibble", types.TypeNumber)
			step.Instructions = append(step.Instructions, &ir.CallInst{
				Res: nibble, Callee: jsonHexNibbleFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()},
			})
			valid := g.currentFn.NewValue("json_hex4_valid", types.TypeBoolean)
			step.Instructions = append(step.Instructions, &ir.BinaryInst{Res: valid, Op: ir.OpGe, LHS: nibble, RHS: ir.ConstNumber{Value: 0}})
			okBB := g.currentFn.NewBlock("json_hex4_ok")
			failBB := g.currentFn.NewBlock("json_hex4_fail")
			step.Terminator = &ir.BranchTerm{Cond: valid, Then: okBB, Else: failBB}

			g.currentBB = failBB
			g.setJSONStateField(state, "ok", ir.ConstBool{Value: false})
			failBB.Terminator = &ir.ReturnTerm{Val: ir.ConstNumber{Value: -1}}

			g.currentBB = okBB
			index := g.jsonStateField(state, "index", types.TypeNumber)
			next := g.currentFn.NewValue("json_hex4_next", types.TypeNumber)
			okBB.Instructions = append(okBB.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
			g.setJSONStateField(state, "index", next)
			values = append(values, nibble)
		}

		v0 := g.currentFn.NewValue("json_hex4_v0", types.TypeNumber)
		v1 := g.currentFn.NewValue("json_hex4_v1", types.TypeNumber)
		v2 := g.currentFn.NewValue("json_hex4_v2", types.TypeNumber)
		v3 := g.currentFn.NewValue("json_hex4_v3", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.BinaryInst{Res: v0, Op: ir.OpMul, LHS: values[0], RHS: ir.ConstNumber{Value: 4096}},
			&ir.BinaryInst{Res: v1, Op: ir.OpMul, LHS: values[1], RHS: ir.ConstNumber{Value: 256}},
			&ir.BinaryInst{Res: v2, Op: ir.OpMul, LHS: values[2], RHS: ir.ConstNumber{Value: 16}},
		)
		s01 := g.currentFn.NewValue("json_hex4_s01", types.TypeNumber)
		s012 := g.currentFn.NewValue("json_hex4_s012", types.TypeNumber)
		cp := g.currentFn.NewValue("json_hex4_cp", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.BinaryInst{Res: s01, Op: ir.OpAdd, LHS: v0, RHS: v1},
			&ir.BinaryInst{Res: s012, Op: ir.OpAdd, LHS: s01, RHS: v2},
			&ir.BinaryInst{Res: v3, Op: ir.OpAdd, LHS: values[3], RHS: ir.ConstNumber{Value: 0}},
			&ir.BinaryInst{Res: cp, Op: ir.OpAdd, LHS: s012, RHS: v3},
		)
		g.currentBB.Terminator = &ir.ReturnTerm{Val: cp}
	})
}

func (g *generator) emitJSONParseString() {
	g.emitJSONGeneratedFunction(jsonParseStringFunc, types.TypeString, func(state ir.Operand) {
		entry := g.currentBB
		startIndex := g.jsonStateField(state, "index", types.TypeNumber)
		first := g.currentFn.NewValue("json_string_first", types.TypeNumber)
		entry.Instructions = append(entry.Instructions, &ir.BinaryInst{Res: first, Op: ir.OpAdd, LHS: startIndex, RHS: ir.ConstNumber{Value: 1}})
		g.setJSONStateField(state, "index", first)

		loop := g.currentFn.NewBlock("json_string_loop")
		scan := g.currentFn.NewBlock("json_string_scan")
		notQuote := g.currentFn.NewBlock("json_string_not_quote")
		controlCheck := g.currentFn.NewBlock("json_string_control_check")
		normal := g.currentFn.NewBlock("json_string_normal")
		escapeBounds := g.currentFn.NewBlock("json_string_escape_bounds")
		escapeRead := g.currentFn.NewBlock("json_string_escape_read")
		unicode := g.currentFn.NewBlock("json_string_unicode")
		unicodeValid := g.currentFn.NewBlock("json_string_unicode_valid")
		unicodeHighCheck := g.currentFn.NewBlock("json_string_unicode_high_check")
		unicodeHigh := g.currentFn.NewBlock("json_string_unicode_high")
		unicodeSingle := g.currentFn.NewBlock("json_string_unicode_single")
		unicodePairPrefix := g.currentFn.NewBlock("json_string_unicode_pair_prefix")
		unicodePairParse := g.currentFn.NewBlock("json_string_unicode_pair_parse")
		unicodePairValid := g.currentFn.NewBlock("json_string_unicode_pair_valid")
		unicodePairCombine := g.currentFn.NewBlock("json_string_unicode_pair_combine")
		unicodePairSeparate := g.currentFn.NewBlock("json_string_unicode_pair_separate")
		closeBB := g.currentFn.NewBlock("json_string_close")
		fail := g.currentFn.NewBlock("json_string_fail")
		entry.Terminator = &ir.JumpTerm{Target: loop}

		acc := g.currentFn.NewValue("json_string_acc", types.TypeString)
		loop.Phis = append(loop.Phis, &ir.PhiInst{Res: acc, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstString{Value: ""}}}})
		g.currentBB = loop
		text := g.jsonStateField(state, "text", types.TypeString)
		index := g.jsonStateField(state, "index", types.TypeNumber)
		length := g.urlStringLen(text)
		more := g.currentFn.NewValue("json_string_more", types.TypeBoolean)
		loop.Instructions = append(loop.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
		loop.Terminator = &ir.BranchTerm{Cond: more, Then: scan, Else: fail}

		g.currentBB = scan
		ch := g.urlStringByteAt(text, index)
		isQuote := g.currentFn.NewValue("json_string_quote", types.TypeBoolean)
		scan.Instructions = append(scan.Instructions, &ir.BinaryInst{Res: isQuote, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '"'}})
		scan.Terminator = &ir.BranchTerm{Cond: isQuote, Then: closeBB, Else: notQuote}

		g.currentBB = notQuote
		isSlash := g.currentFn.NewValue("json_string_slash", types.TypeBoolean)
		notQuote.Instructions = append(notQuote.Instructions, &ir.BinaryInst{Res: isSlash, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '\\'}})
		notQuote.Terminator = &ir.BranchTerm{Cond: isSlash, Then: escapeBounds, Else: controlCheck}

		g.currentBB = controlCheck
		isControl := g.currentFn.NewValue("json_string_control", types.TypeBoolean)
		controlCheck.Instructions = append(controlCheck.Instructions, &ir.BinaryInst{Res: isControl, Op: ir.OpLt, LHS: ch, RHS: ir.ConstNumber{Value: 0x20}})
		controlCheck.Terminator = &ir.BranchTerm{Cond: isControl, Then: fail, Else: normal}

		g.currentBB = normal
		normalNext := g.currentFn.NewValue("json_string_normal_next", types.TypeNumber)
		normal.Instructions = append(normal.Instructions, &ir.BinaryInst{Res: normalNext, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
		charText := g.urlStringSlice(text, index, normalNext)
		normalAcc := g.concatNativeStrings(acc, charText)
		g.setJSONStateField(state, "index", normalNext)
		normalEnd := g.currentBB
		normalEnd.Terminator = &ir.JumpTerm{Target: loop}
		loop.Phis[0].Incoming = append(loop.Phis[0].Incoming, ir.PhiIncoming{Block: normalEnd, Value: normalAcc})

		g.currentBB = closeBB
		closeNext := g.currentFn.NewValue("json_string_close_next", types.TypeNumber)
		closeBB.Instructions = append(closeBB.Instructions, &ir.BinaryInst{Res: closeNext, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
		g.setJSONStateField(state, "index", closeNext)
		closeBB.Terminator = &ir.ReturnTerm{Val: acc}

		g.currentBB = escapeBounds
		escapeIndex := g.currentFn.NewValue("json_string_escape_index", types.TypeNumber)
		escapeBounds.Instructions = append(escapeBounds.Instructions, &ir.BinaryInst{Res: escapeIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
		escapeInRange := g.currentFn.NewValue("json_string_escape_in_range", types.TypeBoolean)
		escapeBounds.Instructions = append(escapeBounds.Instructions, &ir.BinaryInst{Res: escapeInRange, Op: ir.OpLt, LHS: escapeIndex, RHS: length})
		escapeBounds.Terminator = &ir.BranchTerm{Cond: escapeInRange, Then: escapeRead, Else: fail}

		g.currentBB = escapeRead
		escapeChar := g.urlStringByteAt(text, escapeIndex)
		type simpleEscape struct {
			code float64
			text string
		}
		escapes := []simpleEscape{{'"', "\""}, {'\\', "\\"}, {'/', "/"}, {'b', "\b"}, {'f', "\f"}, {'n', "\n"}, {'r', "\r"}, {'t', "\t"}}
		check := escapeRead
		for i, esc := range escapes {
			g.currentBB = check
			match := g.currentFn.NewValue("json_string_escape_match", types.TypeBoolean)
			check.Instructions = append(check.Instructions, &ir.BinaryInst{Res: match, Op: ir.OpEq, LHS: escapeChar, RHS: ir.ConstNumber{Value: esc.code}})
			hit := g.currentFn.NewBlock("json_string_escape_hit")
			nextCheck := g.currentFn.NewBlock("json_string_escape_check")
			check.Terminator = &ir.BranchTerm{Cond: match, Then: hit, Else: nextCheck}

			g.currentBB = hit
			afterEscape := g.currentFn.NewValue("json_string_after_escape", types.TypeNumber)
			hit.Instructions = append(hit.Instructions, &ir.BinaryInst{Res: afterEscape, Op: ir.OpAdd, LHS: escapeIndex, RHS: ir.ConstNumber{Value: 1}})
			simpleAcc := g.concatNativeStrings(acc, ir.ConstString{Value: esc.text})
			g.setJSONStateField(state, "index", afterEscape)
			hitEnd := g.currentBB
			hitEnd.Terminator = &ir.JumpTerm{Target: loop}
			loop.Phis[0].Incoming = append(loop.Phis[0].Incoming, ir.PhiIncoming{Block: hitEnd, Value: simpleAcc})
			check = nextCheck
			if i == len(escapes)-1 {
				g.currentBB = check
				isUnicode := g.currentFn.NewValue("json_string_escape_unicode", types.TypeBoolean)
				check.Instructions = append(check.Instructions, &ir.BinaryInst{Res: isUnicode, Op: ir.OpEq, LHS: escapeChar, RHS: ir.ConstNumber{Value: 'u'}})
				check.Terminator = &ir.BranchTerm{Cond: isUnicode, Then: unicode, Else: fail}
			}
		}

		g.currentBB = unicode
		hexStart := g.currentFn.NewValue("json_string_hex_start", types.TypeNumber)
		unicode.Instructions = append(unicode.Instructions, &ir.BinaryInst{Res: hexStart, Op: ir.OpAdd, LHS: escapeIndex, RHS: ir.ConstNumber{Value: 1}})
		hexEnd := g.currentFn.NewValue("json_string_hex_end", types.TypeNumber)
		unicode.Instructions = append(unicode.Instructions, &ir.BinaryInst{Res: hexEnd, Op: ir.OpAdd, LHS: hexStart, RHS: ir.ConstNumber{Value: 4}})
		hexFits := g.currentFn.NewValue("json_string_hex_fits", types.TypeBoolean)
		unicode.Instructions = append(unicode.Instructions, &ir.BinaryInst{Res: hexFits, Op: ir.OpLe, LHS: hexEnd, RHS: length})
		unicode.Terminator = &ir.BranchTerm{Cond: hexFits, Then: unicodeValid, Else: fail}

		g.currentBB = unicodeValid
		g.setJSONStateField(state, "index", hexStart)
		codePoint := g.currentFn.NewValue("json_string_code_point", types.TypeNumber)
		unicodeValid.Instructions = append(unicodeValid.Instructions, &ir.CallInst{Res: codePoint, Callee: jsonParseHex4Func, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		stillOK := g.jsonStateField(state, "ok", types.TypeBoolean)
		unicodeValid.Terminator = &ir.BranchTerm{Cond: stillOK, Then: unicodeHighCheck, Else: fail}

		g.currentBB = unicodeHighCheck
		geHigh := g.currentFn.NewValue("json_string_ge_high_surrogate", types.TypeBoolean)
		leHigh := g.currentFn.NewValue("json_string_le_high_surrogate", types.TypeBoolean)
		isHigh := g.currentFn.NewValue("json_string_high_surrogate", types.TypeBoolean)
		unicodeHighCheck.Instructions = append(unicodeHighCheck.Instructions,
			&ir.BinaryInst{Res: geHigh, Op: ir.OpGe, LHS: codePoint, RHS: ir.ConstNumber{Value: 0xd800}},
			&ir.BinaryInst{Res: leHigh, Op: ir.OpLe, LHS: codePoint, RHS: ir.ConstNumber{Value: 0xdbff}},
			&ir.BinaryInst{Res: isHigh, Op: ir.OpAnd, LHS: geHigh, RHS: leHigh},
		)
		unicodeHighCheck.Terminator = &ir.BranchTerm{Cond: isHigh, Then: unicodeHigh, Else: unicodeSingle}

		g.currentBB = unicodeHigh
		afterHigh := g.jsonStateField(state, "index", types.TypeNumber)
		pairEnd := g.currentFn.NewValue("json_string_pair_end", types.TypeNumber)
		unicodeHigh.Instructions = append(unicodeHigh.Instructions, &ir.BinaryInst{Res: pairEnd, Op: ir.OpAdd, LHS: afterHigh, RHS: ir.ConstNumber{Value: 6}})
		pairFits := g.currentFn.NewValue("json_string_pair_fits", types.TypeBoolean)
		unicodeHigh.Instructions = append(unicodeHigh.Instructions, &ir.BinaryInst{Res: pairFits, Op: ir.OpLe, LHS: pairEnd, RHS: length})
		unicodeHigh.Terminator = &ir.BranchTerm{Cond: pairFits, Then: unicodePairPrefix, Else: unicodeSingle}

		g.currentBB = unicodePairPrefix
		slash := g.urlStringByteAt(text, afterHigh)
		uIndex := g.currentFn.NewValue("json_string_pair_u_index", types.TypeNumber)
		unicodePairPrefix.Instructions = append(unicodePairPrefix.Instructions, &ir.BinaryInst{Res: uIndex, Op: ir.OpAdd, LHS: afterHigh, RHS: ir.ConstNumber{Value: 1}})
		uChar := g.urlStringByteAt(text, uIndex)
		isSlash2 := g.currentFn.NewValue("json_string_pair_slash", types.TypeBoolean)
		isU2 := g.currentFn.NewValue("json_string_pair_u", types.TypeBoolean)
		isPrefix := g.currentFn.NewValue("json_string_pair_prefix_ok", types.TypeBoolean)
		unicodePairPrefix.Instructions = append(unicodePairPrefix.Instructions,
			&ir.BinaryInst{Res: isSlash2, Op: ir.OpEq, LHS: slash, RHS: ir.ConstNumber{Value: '\\'}},
			&ir.BinaryInst{Res: isU2, Op: ir.OpEq, LHS: uChar, RHS: ir.ConstNumber{Value: 'u'}},
			&ir.BinaryInst{Res: isPrefix, Op: ir.OpAnd, LHS: isSlash2, RHS: isU2},
		)
		unicodePairPrefix.Terminator = &ir.BranchTerm{Cond: isPrefix, Then: unicodePairParse, Else: unicodeSingle}

		g.currentBB = unicodePairParse
		lowStart := g.currentFn.NewValue("json_string_low_start", types.TypeNumber)
		unicodePairParse.Instructions = append(unicodePairParse.Instructions, &ir.BinaryInst{Res: lowStart, Op: ir.OpAdd, LHS: afterHigh, RHS: ir.ConstNumber{Value: 2}})
		g.setJSONStateField(state, "index", lowStart)
		low := g.currentFn.NewValue("json_string_low_surrogate", types.TypeNumber)
		unicodePairParse.Instructions = append(unicodePairParse.Instructions, &ir.CallInst{Res: low, Callee: jsonParseHex4Func, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		lowOK := g.jsonStateField(state, "ok", types.TypeBoolean)
		unicodePairParse.Terminator = &ir.BranchTerm{Cond: lowOK, Then: unicodePairValid, Else: fail}

		g.currentBB = unicodePairValid
		geLow := g.currentFn.NewValue("json_string_ge_low_surrogate", types.TypeBoolean)
		leLow := g.currentFn.NewValue("json_string_le_low_surrogate", types.TypeBoolean)
		isLow := g.currentFn.NewValue("json_string_low_surrogate_ok", types.TypeBoolean)
		unicodePairValid.Instructions = append(unicodePairValid.Instructions,
			&ir.BinaryInst{Res: geLow, Op: ir.OpGe, LHS: low, RHS: ir.ConstNumber{Value: 0xdc00}},
			&ir.BinaryInst{Res: leLow, Op: ir.OpLe, LHS: low, RHS: ir.ConstNumber{Value: 0xdfff}},
			&ir.BinaryInst{Res: isLow, Op: ir.OpAnd, LHS: geLow, RHS: leLow},
		)
		unicodePairValid.Terminator = &ir.BranchTerm{Cond: isLow, Then: unicodePairCombine, Else: unicodePairSeparate}

		g.currentBB = unicodePairCombine
		hiBase := g.currentFn.NewValue("json_string_hi_base", types.TypeNumber)
		lowBase := g.currentFn.NewValue("json_string_low_base", types.TypeNumber)
		hiScaled := g.currentFn.NewValue("json_string_hi_scaled", types.TypeNumber)
		pairSum := g.currentFn.NewValue("json_string_pair_sum", types.TypeNumber)
		combined := g.currentFn.NewValue("json_string_pair_code_point", types.TypeNumber)
		unicodePairCombine.Instructions = append(unicodePairCombine.Instructions,
			&ir.BinaryInst{Res: hiBase, Op: ir.OpSub, LHS: codePoint, RHS: ir.ConstNumber{Value: 0xd800}},
			&ir.BinaryInst{Res: lowBase, Op: ir.OpSub, LHS: low, RHS: ir.ConstNumber{Value: 0xdc00}},
			&ir.BinaryInst{Res: hiScaled, Op: ir.OpMul, LHS: hiBase, RHS: ir.ConstNumber{Value: 1024}},
			&ir.BinaryInst{Res: pairSum, Op: ir.OpAdd, LHS: hiScaled, RHS: lowBase},
			&ir.BinaryInst{Res: combined, Op: ir.OpAdd, LHS: pairSum, RHS: ir.ConstNumber{Value: 0x10000}},
		)
		pairText := g.currentFn.NewValue("json_string_pair_text", types.TypeString)
		unicodePairCombine.Instructions = append(unicodePairCombine.Instructions, &ir.CallInst{Res: pairText, Callee: "ts_utf8_from_code_point", Args: []ir.Operand{combined}, ParamTypes: []types.Type{types.TypeNumber}})
		pairAcc := g.concatNativeStrings(acc, pairText)
		pairEndBB := g.currentBB
		pairEndBB.Terminator = &ir.JumpTerm{Target: loop}
		loop.Phis[0].Incoming = append(loop.Phis[0].Incoming, ir.PhiIncoming{Block: pairEndBB, Value: pairAcc})

		g.currentBB = unicodePairSeparate
		hiText := g.currentFn.NewValue("json_string_high_text", types.TypeString)
		unicodePairSeparate.Instructions = append(unicodePairSeparate.Instructions, &ir.CallInst{Res: hiText, Callee: "ts_utf8_from_code_point", Args: []ir.Operand{codePoint}, ParamTypes: []types.Type{types.TypeNumber}})
		lowText := g.currentFn.NewValue("json_string_low_text", types.TypeString)
		unicodePairSeparate.Instructions = append(unicodePairSeparate.Instructions, &ir.CallInst{Res: lowText, Callee: "ts_utf8_from_code_point", Args: []ir.Operand{low}, ParamTypes: []types.Type{types.TypeNumber}})
		separateAcc := g.concatNativeStrings(g.concatNativeStrings(acc, hiText), lowText)
		separateEnd := g.currentBB
		separateEnd.Terminator = &ir.JumpTerm{Target: loop}
		loop.Phis[0].Incoming = append(loop.Phis[0].Incoming, ir.PhiIncoming{Block: separateEnd, Value: separateAcc})

		g.currentBB = unicodeSingle
		singleText := g.currentFn.NewValue("json_string_unicode_text", types.TypeString)
		unicodeSingle.Instructions = append(unicodeSingle.Instructions, &ir.CallInst{Res: singleText, Callee: "ts_utf8_from_code_point", Args: []ir.Operand{codePoint}, ParamTypes: []types.Type{types.TypeNumber}})
		singleAcc := g.concatNativeStrings(acc, singleText)
		singleEnd := g.currentBB
		singleEnd.Terminator = &ir.JumpTerm{Target: loop}
		loop.Phis[0].Incoming = append(loop.Phis[0].Incoming, ir.PhiIncoming{Block: singleEnd, Value: singleAcc})

		g.currentBB = fail
		g.setJSONStateField(state, "ok", ir.ConstBool{Value: false})
		fail.Terminator = &ir.ReturnTerm{Val: ir.ConstString{Value: ""}}
	})
}

func (g *generator) emitJSONParseValue() {
	g.emitJSONGeneratedFunction(jsonParseValueFunc, types.TypeAny, func(state ir.Operand) {
		entry := g.currentBB
		entry.Instructions = append(entry.Instructions, &ir.CallInst{Callee: jsonSkipWhitespaceFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		text := g.jsonStateField(state, "text", types.TypeString)
		index := g.jsonStateField(state, "index", types.TypeNumber)
		length := g.urlStringLen(text)
		more := g.currentFn.NewValue("json_value_more", types.TypeBoolean)
		entry.Instructions = append(entry.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
		dispatch := g.currentFn.NewBlock("json_value_dispatch")
		fail := g.currentFn.NewBlock("json_value_fail")
		entry.Terminator = &ir.BranchTerm{Cond: more, Then: dispatch, Else: fail}

		g.currentBB = dispatch
		ch := g.urlStringByteAt(text, index)
		stringBB := g.currentFn.NewBlock("json_value_string")
		objectCheck := g.currentFn.NewBlock("json_value_object_check")
		objectBB := g.currentFn.NewBlock("json_value_object")
		arrayCheck := g.currentFn.NewBlock("json_value_array_check")
		arrayBB := g.currentFn.NewBlock("json_value_array")
		trueCheck := g.currentFn.NewBlock("json_value_true_check")
		trueBB := g.currentFn.NewBlock("json_value_true")
		falseCheck := g.currentFn.NewBlock("json_value_false_check")
		falseBB := g.currentFn.NewBlock("json_value_false")
		nullCheck := g.currentFn.NewBlock("json_value_null_check")
		nullBB := g.currentFn.NewBlock("json_value_null")
		numberCheck := g.currentFn.NewBlock("json_value_number_check")
		numberBB := g.currentFn.NewBlock("json_value_number")

		isString := g.currentFn.NewValue("json_value_is_string", types.TypeBoolean)
		dispatch.Instructions = append(dispatch.Instructions, &ir.BinaryInst{Res: isString, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '"'}})
		dispatch.Terminator = &ir.BranchTerm{Cond: isString, Then: stringBB, Else: objectCheck}

		g.currentBB = objectCheck
		isObject := g.currentFn.NewValue("json_value_is_object", types.TypeBoolean)
		objectCheck.Instructions = append(objectCheck.Instructions, &ir.BinaryInst{Res: isObject, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '{'}})
		objectCheck.Terminator = &ir.BranchTerm{Cond: isObject, Then: objectBB, Else: arrayCheck}

		g.currentBB = arrayCheck
		isArray := g.currentFn.NewValue("json_value_is_array", types.TypeBoolean)
		arrayCheck.Instructions = append(arrayCheck.Instructions, &ir.BinaryInst{Res: isArray, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '['}})
		arrayCheck.Terminator = &ir.BranchTerm{Cond: isArray, Then: arrayBB, Else: trueCheck}

		g.currentBB = trueCheck
		isTrue := g.currentFn.NewValue("json_value_is_true", types.TypeBoolean)
		trueCheck.Instructions = append(trueCheck.Instructions, &ir.BinaryInst{Res: isTrue, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: 't'}})
		trueCheck.Terminator = &ir.BranchTerm{Cond: isTrue, Then: trueBB, Else: falseCheck}

		g.currentBB = falseCheck
		isFalse := g.currentFn.NewValue("json_value_is_false", types.TypeBoolean)
		falseCheck.Instructions = append(falseCheck.Instructions, &ir.BinaryInst{Res: isFalse, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: 'f'}})
		falseCheck.Terminator = &ir.BranchTerm{Cond: isFalse, Then: falseBB, Else: nullCheck}

		g.currentBB = nullCheck
		isNull := g.currentFn.NewValue("json_value_is_null", types.TypeBoolean)
		nullCheck.Instructions = append(nullCheck.Instructions, &ir.BinaryInst{Res: isNull, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: 'n'}})
		nullCheck.Terminator = &ir.BranchTerm{Cond: isNull, Then: nullBB, Else: numberCheck}

		g.currentBB = numberCheck
		isMinus := g.currentFn.NewValue("json_value_minus", types.TypeBoolean)
		geZero := g.currentFn.NewValue("json_value_ge_zero", types.TypeBoolean)
		leNine := g.currentFn.NewValue("json_value_le_nine", types.TypeBoolean)
		isDigit := g.currentFn.NewValue("json_value_digit", types.TypeBoolean)
		isNumber := g.currentFn.NewValue("json_value_number_start", types.TypeBoolean)
		numberCheck.Instructions = append(numberCheck.Instructions,
			&ir.BinaryInst{Res: isMinus, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '-'}},
			&ir.BinaryInst{Res: geZero, Op: ir.OpGe, LHS: ch, RHS: ir.ConstNumber{Value: '0'}},
			&ir.BinaryInst{Res: leNine, Op: ir.OpLe, LHS: ch, RHS: ir.ConstNumber{Value: '9'}},
			&ir.BinaryInst{Res: isDigit, Op: ir.OpAnd, LHS: geZero, RHS: leNine},
			&ir.BinaryInst{Res: isNumber, Op: ir.OpOr, LHS: isMinus, RHS: isDigit},
		)
		numberCheck.Terminator = &ir.BranchTerm{Cond: isNumber, Then: numberBB, Else: fail}

		// String value.
		g.currentBB = stringBB
		parsedString := g.currentFn.NewValue("json_value_string_value", types.TypeString)
		stringBB.Instructions = append(stringBB.Instructions, &ir.CallInst{Res: parsedString, Callee: jsonParseStringFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		stringOK := g.jsonStateField(state, "ok", types.TypeBoolean)
		stringReturn := g.currentFn.NewBlock("json_value_string_return")
		stringBB.Terminator = &ir.BranchTerm{Cond: stringOK, Then: stringReturn, Else: fail}
		g.currentBB = stringReturn
		boxedString := g.boxJSValue(parsedString, types.TypeString)
		stringReturn.Terminator = &ir.ReturnTerm{Val: boxedString}

		// Exact literal helper emitted inline for true/false/null.
		emitLiteral := func(bb *ir.BasicBlock, literal string, value ir.Operand, sourceType types.Type) {
			g.currentBB = bb
			start := g.jsonStateField(state, "index", types.TypeNumber)
			end := g.currentFn.NewValue("json_literal_end", types.TypeNumber)
			bb.Instructions = append(bb.Instructions, &ir.BinaryInst{Res: end, Op: ir.OpAdd, LHS: start, RHS: ir.ConstNumber{Value: float64(len(literal))}})
			textLen := g.urlStringLen(text)
			fits := g.currentFn.NewValue("json_literal_fits", types.TypeBoolean)
			bb.Instructions = append(bb.Instructions, &ir.BinaryInst{Res: fits, Op: ir.OpLe, LHS: end, RHS: textLen})
			compare := g.currentFn.NewBlock("json_literal_compare")
			ret := g.currentFn.NewBlock("json_literal_return")
			bb.Terminator = &ir.BranchTerm{Cond: fits, Then: compare, Else: fail}
			g.currentBB = compare
			piece := g.urlStringSlice(text, start, end)
			eq := g.urlStringEqual(piece, literal)
			compare.Terminator = &ir.BranchTerm{Cond: eq, Then: ret, Else: fail}
			g.currentBB = ret
			g.setJSONStateField(state, "index", end)
			if sourceType != nil && sourceType != types.TypeAny && sourceType.Kind() != types.KindNull {
				value = g.boxJSValue(value, sourceType)
			}
			ret.Terminator = &ir.ReturnTerm{Val: value}
		}
		emitLiteral(trueBB, "true", ir.ConstBool{Value: true}, types.TypeBoolean)
		emitLiteral(falseBB, "false", ir.ConstBool{Value: false}, types.TypeBoolean)
		emitLiteral(nullBB, "null", ir.ConstNull{}, types.TypeNull)

		// Number token scanning and validation.
		g.currentBB = numberBB
		numberStart := g.jsonStateField(state, "index", types.TypeNumber)
		numCond := g.currentFn.NewBlock("json_number_scan_cond")
		numBody := g.currentFn.NewBlock("json_number_scan_body")
		numAdvance := g.currentFn.NewBlock("json_number_scan_advance")
		numDone := g.currentFn.NewBlock("json_number_scan_done")
		numberBB.Terminator = &ir.JumpTerm{Target: numCond}
		g.currentBB = numCond
		numIndex := g.jsonStateField(state, "index", types.TypeNumber)
		numLen := g.urlStringLen(text)
		numMore := g.currentFn.NewValue("json_number_scan_more", types.TypeBoolean)
		numCond.Instructions = append(numCond.Instructions, &ir.BinaryInst{Res: numMore, Op: ir.OpLt, LHS: numIndex, RHS: numLen})
		numCond.Terminator = &ir.BranchTerm{Cond: numMore, Then: numBody, Else: numDone}
		g.currentBB = numBody
		numCh := g.urlStringByteAt(text, numIndex)
		var delimiter ir.Operand
		for i, code := range []float64{' ', '\t', '\n', '\r', ',', ']', '}'} {
			eq := g.currentFn.NewValue("json_number_delimiter_eq", types.TypeBoolean)
			numBody.Instructions = append(numBody.Instructions, &ir.BinaryInst{Res: eq, Op: ir.OpEq, LHS: numCh, RHS: ir.ConstNumber{Value: code}})
			if i == 0 {
				delimiter = eq
			} else {
				any := g.currentFn.NewValue("json_number_delimiter_any", types.TypeBoolean)
				numBody.Instructions = append(numBody.Instructions, &ir.BinaryInst{Res: any, Op: ir.OpOr, LHS: delimiter, RHS: eq})
				delimiter = any
			}
		}
		numBody.Terminator = &ir.BranchTerm{Cond: delimiter, Then: numDone, Else: numAdvance}
		g.currentBB = numAdvance
		numNext := g.currentFn.NewValue("json_number_scan_next", types.TypeNumber)
		numAdvance.Instructions = append(numAdvance.Instructions, &ir.BinaryInst{Res: numNext, Op: ir.OpAdd, LHS: numIndex, RHS: ir.ConstNumber{Value: 1}})
		g.setJSONStateField(state, "index", numNext)
		numAdvance.Terminator = &ir.JumpTerm{Target: numCond}
		g.currentBB = numDone
		numberEnd := g.jsonStateField(state, "index", types.TypeNumber)
		token := g.urlStringSlice(text, numberStart, numberEnd)
		validNumber := g.currentFn.NewValue("json_number_valid", types.TypeBoolean)
		numDone.Instructions = append(numDone.Instructions, &ir.CallInst{Res: validNumber, Callee: "ts_json_number_valid", Args: []ir.Operand{token}, ParamTypes: []types.Type{types.TypeString}})
		numReturn := g.currentFn.NewBlock("json_number_return")
		numDone.Terminator = &ir.BranchTerm{Cond: validNumber, Then: numReturn, Else: fail}
		g.currentBB = numReturn
		numberValue := g.currentFn.NewValue("json_number_value", types.TypeAny)
		numReturn.Instructions = append(numReturn.Instructions, &ir.CallInst{Res: numberValue, Callee: "ts_json_parse_scalar", Args: []ir.Operand{token}, ParamTypes: []types.Type{types.TypeString}})
		numReturn.Terminator = &ir.ReturnTerm{Val: numberValue}

		// Array parsing.
		g.currentBB = arrayBB
		arrayIndex := g.jsonStateField(state, "index", types.TypeNumber)
		afterOpenArray := g.currentFn.NewValue("json_array_after_open", types.TypeNumber)
		arrayBB.Instructions = append(arrayBB.Instructions, &ir.BinaryInst{Res: afterOpenArray, Op: ir.OpAdd, LHS: arrayIndex, RHS: ir.ConstNumber{Value: 1}})
		g.setJSONStateField(state, "index", afterOpenArray)
		arrayType := types.NewArray(types.TypeAny)
		array := g.currentFn.NewValue("json_dynamic_array", arrayType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{Res: array, ElemType: types.TypeAny, Length: ir.ConstNumber{Value: 0}})
		arrayLoop := g.currentFn.NewBlock("json_array_loop")
		arrayInspect := g.currentFn.NewBlock("json_array_inspect")
		arrayValue := g.currentFn.NewBlock("json_array_value")
		arrayValueOK := g.currentFn.NewBlock("json_array_value_ok")
		arrayDelimiter := g.currentFn.NewBlock("json_array_delimiter")
		arrayComma := g.currentFn.NewBlock("json_array_comma")
		arrayClose := g.currentFn.NewBlock("json_array_close")
		g.currentBB.Terminator = &ir.JumpTerm{Target: arrayLoop}
		g.currentBB = arrayLoop
		arrayLoop.Instructions = append(arrayLoop.Instructions, &ir.CallInst{Callee: jsonSkipWhitespaceFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		arrPos := g.jsonStateField(state, "index", types.TypeNumber)
		arrTextLen := g.urlStringLen(text)
		arrMore := g.currentFn.NewValue("json_array_more", types.TypeBoolean)
		arrayLoop.Instructions = append(arrayLoop.Instructions, &ir.BinaryInst{Res: arrMore, Op: ir.OpLt, LHS: arrPos, RHS: arrTextLen})
		arrayLoop.Terminator = &ir.BranchTerm{Cond: arrMore, Then: arrayInspect, Else: fail}
		g.currentBB = arrayInspect
		arrCh := g.urlStringByteAt(text, arrPos)
		isArrayClose := g.currentFn.NewValue("json_array_is_close", types.TypeBoolean)
		arrayInspect.Instructions = append(arrayInspect.Instructions, &ir.BinaryInst{Res: isArrayClose, Op: ir.OpEq, LHS: arrCh, RHS: ir.ConstNumber{Value: ']'}})
		arrayInspect.Terminator = &ir.BranchTerm{Cond: isArrayClose, Then: arrayClose, Else: arrayValue}
		g.currentBB = arrayValue
		item := g.currentFn.NewValue("json_array_item", types.TypeAny)
		arrayValue.Instructions = append(arrayValue.Instructions, &ir.CallInst{Res: item, Callee: jsonParseValueFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		itemOK := g.jsonStateField(state, "ok", types.TypeBoolean)
		arrayValue.Terminator = &ir.BranchTerm{Cond: itemOK, Then: arrayValueOK, Else: fail}
		g.currentBB = arrayValueOK
		g.pushArrayOperand(array, item)
		arrayValueOK.Instructions = append(arrayValueOK.Instructions, &ir.CallInst{Callee: jsonSkipWhitespaceFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		arrayValueOK.Terminator = &ir.JumpTerm{Target: arrayDelimiter}
		g.currentBB = arrayDelimiter
		arrDelimPos := g.jsonStateField(state, "index", types.TypeNumber)
		arrDelimLen := g.urlStringLen(text)
		arrDelimMore := g.currentFn.NewValue("json_array_delim_more", types.TypeBoolean)
		arrayDelimiter.Instructions = append(arrayDelimiter.Instructions, &ir.BinaryInst{Res: arrDelimMore, Op: ir.OpLt, LHS: arrDelimPos, RHS: arrDelimLen})
		arrDelimRead := g.currentFn.NewBlock("json_array_delim_read")
		arrayDelimiter.Terminator = &ir.BranchTerm{Cond: arrDelimMore, Then: arrDelimRead, Else: fail}
		g.currentBB = arrDelimRead
		arrDelimCh := g.urlStringByteAt(text, arrDelimPos)
		arrIsComma := g.currentFn.NewValue("json_array_is_comma", types.TypeBoolean)
		arrDelimRead.Instructions = append(arrDelimRead.Instructions, &ir.BinaryInst{Res: arrIsComma, Op: ir.OpEq, LHS: arrDelimCh, RHS: ir.ConstNumber{Value: ','}})
		arrCloseCheck := g.currentFn.NewBlock("json_array_close_check")
		arrDelimRead.Terminator = &ir.BranchTerm{Cond: arrIsComma, Then: arrayComma, Else: arrCloseCheck}
		g.currentBB = arrCloseCheck
		arrIsClose2 := g.currentFn.NewValue("json_array_is_close2", types.TypeBoolean)
		arrCloseCheck.Instructions = append(arrCloseCheck.Instructions, &ir.BinaryInst{Res: arrIsClose2, Op: ir.OpEq, LHS: arrDelimCh, RHS: ir.ConstNumber{Value: ']'}})
		arrCloseCheck.Terminator = &ir.BranchTerm{Cond: arrIsClose2, Then: arrayClose, Else: fail}
		g.currentBB = arrayComma
		afterComma := g.currentFn.NewValue("json_array_after_comma", types.TypeNumber)
		arrayComma.Instructions = append(arrayComma.Instructions, &ir.BinaryInst{Res: afterComma, Op: ir.OpAdd, LHS: arrDelimPos, RHS: ir.ConstNumber{Value: 1}})
		g.setJSONStateField(state, "index", afterComma)
		// After a comma a value is mandatory; jumping directly to parseValue
		// rejects the otherwise tempting-but-invalid trailing comma before ']'.
		arrayComma.Terminator = &ir.JumpTerm{Target: arrayValue}
		g.currentBB = arrayClose
		closePos := g.jsonStateField(state, "index", types.TypeNumber)
		afterCloseArray := g.currentFn.NewValue("json_array_after_close", types.TypeNumber)
		arrayClose.Instructions = append(arrayClose.Instructions, &ir.BinaryInst{Res: afterCloseArray, Op: ir.OpAdd, LHS: closePos, RHS: ir.ConstNumber{Value: 1}})
		g.setJSONStateField(state, "index", afterCloseArray)
		boxedArray := g.boxJSValue(array, arrayType)
		arrayClose.Terminator = &ir.ReturnTerm{Val: boxedArray}

		// Object parsing.
		g.currentBB = objectBB
		objectIndex := g.jsonStateField(state, "index", types.TypeNumber)
		afterOpenObject := g.currentFn.NewValue("json_object_after_open", types.TypeNumber)
		objectBB.Instructions = append(objectBB.Instructions, &ir.BinaryInst{Res: afterOpenObject, Op: ir.OpAdd, LHS: objectIndex, RHS: ir.ConstNumber{Value: 1}})
		g.setJSONStateField(state, "index", afterOpenObject)
		object := g.currentFn.NewValue("json_dynamic_object", types.TypeAny)
		objectBB.Instructions = append(objectBB.Instructions, &ir.CallInst{Res: object, Callee: "ts_dynamic_object_new"})
		objectLoop := g.currentFn.NewBlock("json_object_loop")
		objectInspect := g.currentFn.NewBlock("json_object_inspect")
		objectKey := g.currentFn.NewBlock("json_object_key")
		objectKeyOK := g.currentFn.NewBlock("json_object_key_ok")
		objectColonCheck := g.currentFn.NewBlock("json_object_colon_check")
		objectValue := g.currentFn.NewBlock("json_object_value")
		objectValueOK := g.currentFn.NewBlock("json_object_value_ok")
		objectDelimiter := g.currentFn.NewBlock("json_object_delimiter")
		objectComma := g.currentFn.NewBlock("json_object_comma")
		objectClose := g.currentFn.NewBlock("json_object_close")
		objectBB.Terminator = &ir.JumpTerm{Target: objectLoop}
		g.currentBB = objectLoop
		objectLoop.Instructions = append(objectLoop.Instructions, &ir.CallInst{Callee: jsonSkipWhitespaceFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		objPos := g.jsonStateField(state, "index", types.TypeNumber)
		objTextLen := g.urlStringLen(text)
		objMore := g.currentFn.NewValue("json_object_more", types.TypeBoolean)
		objectLoop.Instructions = append(objectLoop.Instructions, &ir.BinaryInst{Res: objMore, Op: ir.OpLt, LHS: objPos, RHS: objTextLen})
		objectLoop.Terminator = &ir.BranchTerm{Cond: objMore, Then: objectInspect, Else: fail}
		g.currentBB = objectInspect
		objCh := g.urlStringByteAt(text, objPos)
		isObjectClose := g.currentFn.NewValue("json_object_is_close", types.TypeBoolean)
		objectInspect.Instructions = append(objectInspect.Instructions, &ir.BinaryInst{Res: isObjectClose, Op: ir.OpEq, LHS: objCh, RHS: ir.ConstNumber{Value: '}'}})
		objectInspect.Terminator = &ir.BranchTerm{Cond: isObjectClose, Then: objectClose, Else: objectKey}
		g.currentBB = objectKey
		isKeyQuote := g.currentFn.NewValue("json_object_key_quote", types.TypeBoolean)
		objectKey.Instructions = append(objectKey.Instructions, &ir.BinaryInst{Res: isKeyQuote, Op: ir.OpEq, LHS: objCh, RHS: ir.ConstNumber{Value: '"'}})
		objectKey.Terminator = &ir.BranchTerm{Cond: isKeyQuote, Then: objectKeyOK, Else: fail}
		g.currentBB = objectKeyOK
		key := g.currentFn.NewValue("json_object_key_string", types.TypeString)
		objectKeyOK.Instructions = append(objectKeyOK.Instructions, &ir.CallInst{Res: key, Callee: jsonParseStringFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		keyOK := g.jsonStateField(state, "ok", types.TypeBoolean)
		objectKeyAfter := g.currentFn.NewBlock("json_object_key_after")
		objectKeyOK.Terminator = &ir.BranchTerm{Cond: keyOK, Then: objectKeyAfter, Else: fail}
		g.currentBB = objectKeyAfter
		objectKeyAfter.Instructions = append(objectKeyAfter.Instructions, &ir.CallInst{Callee: jsonSkipWhitespaceFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		objectKeyAfter.Terminator = &ir.JumpTerm{Target: objectColonCheck}
		g.currentBB = objectColonCheck
		colonPos := g.jsonStateField(state, "index", types.TypeNumber)
		colonLen := g.urlStringLen(text)
		colonMore := g.currentFn.NewValue("json_object_colon_more", types.TypeBoolean)
		objectColonCheck.Instructions = append(objectColonCheck.Instructions, &ir.BinaryInst{Res: colonMore, Op: ir.OpLt, LHS: colonPos, RHS: colonLen})
		colonRead := g.currentFn.NewBlock("json_object_colon_read")
		objectColonCheck.Terminator = &ir.BranchTerm{Cond: colonMore, Then: colonRead, Else: fail}
		g.currentBB = colonRead
		colonCh := g.urlStringByteAt(text, colonPos)
		isColon := g.currentFn.NewValue("json_object_is_colon", types.TypeBoolean)
		colonRead.Instructions = append(colonRead.Instructions, &ir.BinaryInst{Res: isColon, Op: ir.OpEq, LHS: colonCh, RHS: ir.ConstNumber{Value: ':'}})
		colonRead.Terminator = &ir.BranchTerm{Cond: isColon, Then: objectValue, Else: fail}
		g.currentBB = objectValue
		afterColon := g.currentFn.NewValue("json_object_after_colon", types.TypeNumber)
		objectValue.Instructions = append(objectValue.Instructions, &ir.BinaryInst{Res: afterColon, Op: ir.OpAdd, LHS: colonPos, RHS: ir.ConstNumber{Value: 1}})
		g.setJSONStateField(state, "index", afterColon)
		parsedValue := g.currentFn.NewValue("json_object_parsed_value", types.TypeAny)
		objectValue.Instructions = append(objectValue.Instructions, &ir.CallInst{Res: parsedValue, Callee: jsonParseValueFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		parsedOK := g.jsonStateField(state, "ok", types.TypeBoolean)
		objectValue.Terminator = &ir.BranchTerm{Cond: parsedOK, Then: objectValueOK, Else: fail}
		g.currentBB = objectValueOK
		setResult := g.currentFn.NewValue("json_object_set", types.TypeAny)
		objectValueOK.Instructions = append(objectValueOK.Instructions, &ir.CallInst{Res: setResult, Callee: "ts_dynamic_set", Args: []ir.Operand{object, key, parsedValue}, ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny}})
		objectValueOK.Instructions = append(objectValueOK.Instructions, &ir.CallInst{Callee: jsonSkipWhitespaceFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		objectValueOK.Terminator = &ir.JumpTerm{Target: objectDelimiter}
		g.currentBB = objectDelimiter
		objDelimPos := g.jsonStateField(state, "index", types.TypeNumber)
		objDelimLen := g.urlStringLen(text)
		objDelimMore := g.currentFn.NewValue("json_object_delim_more", types.TypeBoolean)
		objectDelimiter.Instructions = append(objectDelimiter.Instructions, &ir.BinaryInst{Res: objDelimMore, Op: ir.OpLt, LHS: objDelimPos, RHS: objDelimLen})
		objDelimRead := g.currentFn.NewBlock("json_object_delim_read")
		objectDelimiter.Terminator = &ir.BranchTerm{Cond: objDelimMore, Then: objDelimRead, Else: fail}
		g.currentBB = objDelimRead
		objDelimCh := g.urlStringByteAt(text, objDelimPos)
		objIsComma := g.currentFn.NewValue("json_object_is_comma", types.TypeBoolean)
		objDelimRead.Instructions = append(objDelimRead.Instructions, &ir.BinaryInst{Res: objIsComma, Op: ir.OpEq, LHS: objDelimCh, RHS: ir.ConstNumber{Value: ','}})
		objCloseCheck := g.currentFn.NewBlock("json_object_close_check")
		objDelimRead.Terminator = &ir.BranchTerm{Cond: objIsComma, Then: objectComma, Else: objCloseCheck}
		g.currentBB = objCloseCheck
		objIsClose2 := g.currentFn.NewValue("json_object_is_close2", types.TypeBoolean)
		objCloseCheck.Instructions = append(objCloseCheck.Instructions, &ir.BinaryInst{Res: objIsClose2, Op: ir.OpEq, LHS: objDelimCh, RHS: ir.ConstNumber{Value: '}'}})
		objCloseCheck.Terminator = &ir.BranchTerm{Cond: objIsClose2, Then: objectClose, Else: fail}
		g.currentBB = objectComma
		afterObjectComma := g.currentFn.NewValue("json_object_after_comma", types.TypeNumber)
		objectComma.Instructions = append(objectComma.Instructions, &ir.BinaryInst{Res: afterObjectComma, Op: ir.OpAdd, LHS: objDelimPos, RHS: ir.ConstNumber{Value: 1}})
		g.setJSONStateField(state, "index", afterObjectComma)
		objectComma.Instructions = append(objectComma.Instructions, &ir.CallInst{Callee: jsonSkipWhitespaceFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{g.jsonStateType()}})
		commaKeyPos := g.jsonStateField(state, "index", types.TypeNumber)
		commaKeyLen := g.urlStringLen(text)
		commaKeyMore := g.currentFn.NewValue("json_object_comma_key_more", types.TypeBoolean)
		objectComma.Instructions = append(objectComma.Instructions, &ir.BinaryInst{Res: commaKeyMore, Op: ir.OpLt, LHS: commaKeyPos, RHS: commaKeyLen})
		commaKeyRead := g.currentFn.NewBlock("json_object_comma_key_read")
		objectComma.Terminator = &ir.BranchTerm{Cond: commaKeyMore, Then: commaKeyRead, Else: fail}
		g.currentBB = commaKeyRead
		commaKeyCh := g.urlStringByteAt(text, commaKeyPos)
		commaKeyQuote := g.currentFn.NewValue("json_object_comma_key_quote", types.TypeBoolean)
		commaKeyRead.Instructions = append(commaKeyRead.Instructions, &ir.BinaryInst{Res: commaKeyQuote, Op: ir.OpEq, LHS: commaKeyCh, RHS: ir.ConstNumber{Value: '"'}})
		commaKeyRead.Terminator = &ir.BranchTerm{Cond: commaKeyQuote, Then: objectKeyOK, Else: fail}
		g.currentBB = objectClose
		objClosePos := g.jsonStateField(state, "index", types.TypeNumber)
		afterCloseObject := g.currentFn.NewValue("json_object_after_close", types.TypeNumber)
		objectClose.Instructions = append(objectClose.Instructions, &ir.BinaryInst{Res: afterCloseObject, Op: ir.OpAdd, LHS: objClosePos, RHS: ir.ConstNumber{Value: 1}})
		g.setJSONStateField(state, "index", afterCloseObject)
		objectClose.Terminator = &ir.ReturnTerm{Val: object}

		g.currentBB = fail
		g.setJSONStateField(state, "ok", ir.ConstBool{Value: false})
		fail.Terminator = &ir.ReturnTerm{Val: ir.ConstUndefined{}}
	})
}

func (g *generator) ensureRuntimeJSONParser() {
	if g.jsonParserEmitted {
		return
	}
	g.jsonParserEmitted = true
	_ = g.jsonStateType()
	g.emitJSONSkipWhitespace()
	g.emitJSONHexNibble()
	g.emitJSONParseHex4()
	g.emitJSONParseString()
	g.emitJSONParseValue()
}

func (g *generator) lowerRuntimeJSONParse(text ir.Operand) ir.Operand {
	g.ensureRuntimeJSONParser()
	t := g.jsonStateType()
	offsets, refMask, shape := g.objectLayout(t)
	state := g.currentFn.NewValue("json_parser_state", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: state, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: state, Field: "text", Offset: offsets["text"], Val: text},
		&ir.SetFieldInst{Obj: state, Field: "index", Offset: offsets["index"], Val: ir.ConstNumber{Value: 0}},
		&ir.SetFieldInst{Obj: state, Field: "ok", Offset: offsets["ok"], Val: ir.ConstBool{Value: true}},
	)
	value := g.currentFn.NewValue("json_runtime_value", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: value, Callee: jsonParseValueFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{t}})
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: jsonSkipWhitespaceFunc, Args: []ir.Operand{state}, ParamTypes: []types.Type{t}})
	ok := g.jsonStateField(state, "ok", types.TypeBoolean)
	index := g.jsonStateField(state, "index", types.TypeNumber)
	length := g.urlStringLen(text)
	atEnd := g.currentFn.NewValue("json_runtime_at_end", types.TypeBoolean)
	valid := g.currentFn.NewValue("json_runtime_valid", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: atEnd, Op: ir.OpEq, LHS: index, RHS: length},
		&ir.BinaryInst{Res: valid, Op: ir.OpAnd, LHS: ok, RHS: atEnd},
	)
	validBB := g.currentFn.NewBlock("json_runtime_valid")
	invalidBB := g.currentFn.NewBlock("json_runtime_invalid")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: valid, Then: validBB, Else: invalidBB}
	g.currentBB = invalidBB
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Unexpected token in JSON"}, ir.ConstString{Value: "SyntaxError"}))
	g.currentBB = validBB
	return value
}
