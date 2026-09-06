package irgen

import (
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func isBuiltinURLPatternType(t types.Type) bool {
	if obj, ok := t.(*types.ObjectType); ok {
		return obj.Name == "$URLPattern"
	}
	return false
}

func (g *generator) lowerURLPatternField(pattern ir.Operand, field string) ir.Operand {
	t := g.semaResult.URLPatternType
	offsets, _, _ := g.objectLayout(t)
	fieldDef := t.Fields[field]
	res := g.currentFn.NewValue("url_pattern_field", fieldDef.Type)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
		Res: res, Obj: pattern, Field: field, Offset: offsets[field],
	})
	return res
}

func (g *generator) callDynamicSet(dyn, key, val ir.Operand) {
	boxed := val
	if !irJSValueType(val.Type()) {
		boxed = g.boxJSValue(val, val.Type())
	}
	set := g.currentFn.NewValue("dyn_set", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: set, Callee: "ts_dynamic_set",
		Args:       []ir.Operand{dyn, key, boxed},
		ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny},
	})
}

func (g *generator) newDynamicObject() ir.Operand {
	dyn := g.currentFn.NewValue("dynamic_obj", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: dyn, Callee: "ts_dynamic_object_new",
	})
	return dyn
}

func (g *generator) moveBlockToEnd(bb *ir.BasicBlock) {
	blocks := g.currentFn.Blocks
	for i, b := range blocks {
		if b == bb {
			g.currentFn.Blocks = append(append(blocks[:i], blocks[i+1:]...), bb)
			return
		}
	}
}

func parseURLPatternCompileTime(inputStr, baseStr string) (proto, user, pass, host, port, path, query, hash string, hasRegex bool, err error) {
	full := inputStr
	if !strings.Contains(full, "://") {
		if baseStr == "" {
			return "", "", "", "", "", "", "", "", false, err
		}
		if strings.HasPrefix(full, "/") {
			// Absolute path relative to base origin
			colonIdx := strings.Index(baseStr, "://")
			if colonIdx > 0 {
				afterScheme := baseStr[colonIdx+3:]
				slashIdx := strings.Index(afterScheme, "/")
				origin := baseStr
				if slashIdx >= 0 {
					origin = baseStr[:colonIdx+3+slashIdx]
				}
				full = origin + full
			} else {
				full = baseStr + full
			}
		} else if full == "*" {
			if strings.HasSuffix(baseStr, "/") {
				full = baseStr + "*"
			} else {
				full = baseStr + "/*"
			}
		} else {
			if strings.HasSuffix(baseStr, "/") {
				full = baseStr + full
			} else {
				full = baseStr + "/" + full
			}
		}
	}

	colonIdx := strings.Index(full, "://")
	if colonIdx <= 0 {
		return "", "", "", "", "", "", "", "", false, err
	}
	proto = full[:colonIdx]
	afterScheme := full[colonIdx+3:]

	user = "*"
	pass = "*"

	// Find authority and rest
	authEnd := len(afterScheme)
	for i := 0; i < len(afterScheme); i++ {
		b := afterScheme[i]
		if b == '/' || b == '?' || b == '#' {
			authEnd = i
			break
		}
	}
	authority := afterScheme[:authEnd]
	rest := afterScheme[authEnd:]

	if atIdx := strings.Index(authority, "@"); atIdx >= 0 {
		userinfo := authority[:atIdx]
		authority = authority[atIdx+1:]
		if colon := strings.Index(userinfo, ":"); colon >= 0 {
			user = userinfo[:colon]
			pass = userinfo[colon+1:]
		} else {
			user = userinfo
		}
	}

	// Host and port
	host = authority
	port = ""
	if lastColon := strings.LastIndex(authority, ":"); lastColon >= 0 {
		potentialPort := authority[lastColon+1:]
		isNumeric := len(potentialPort) > 0
		for i := 0; i < len(potentialPort); i++ {
			if potentialPort[i] < '0' || potentialPort[i] > '9' {
				isNumeric = false
				break
			}
		}
		if isNumeric {
			host = authority[:lastColon]
			port = potentialPort
		}
	}

	// Path
	path = "/*"
	query = "*"
	hash = "*"

	if len(rest) > 0 && rest[0] == '/' {
		pathEnd := len(rest)
		for i := 0; i < len(rest); i++ {
			if rest[i] == '?' {
				if i > 0 && rest[i-1] == '\\' {
					pathEnd = i - 1
					path = rest[:pathEnd]
					rest = rest[i:]
					break
				}
				pathEnd = i
				path = rest[:pathEnd]
				rest = rest[i:]
				break
			} else if rest[i] == '#' {
				pathEnd = i
				path = rest[:pathEnd]
				rest = rest[i:]
				break
			}
		}
		if path == "/*" {
			path = rest
			rest = ""
		}
	}

	if len(rest) > 0 && rest[0] == '?' {
		queryEnd := len(rest)
		for i := 1; i < len(rest); i++ {
			if rest[i] == '#' {
				queryEnd = i
				break
			}
		}
		query = rest[1:queryEnd]
		rest = rest[queryEnd:]
	}

	if len(rest) > 0 && rest[0] == '#' {
		hash = rest[1:]
	}

	for _, comp := range []string{proto, user, pass, host, port, path, query, hash} {
		if strings.Contains(comp, "(") {
			hasRegex = true
			break
		}
	}

	return proto, user, pass, host, port, path, query, hash, hasRegex, nil
}

func parseURLPatternInitCompileTime(fields map[string]string) (proto, user, pass, host, port, path, query, hash string, hasRegex bool) {
	proto = "*"
	user = "*"
	pass = "*"
	host = "*"
	port = "*"
	path = "*"
	query = "*"
	hash = "*"

	if baseStr, ok := fields["baseURL"]; ok && baseStr != "" {
		if bp, bu, bpass, bh, bport, bpath, _, _, _, err := parseURLPatternCompileTime(baseStr, ""); err == nil {
			proto = bp
			user = bu
			pass = bpass
			host = bh
			port = bport
			path = bpath
		}
	}

	if v, ok := fields["protocol"]; ok {
		proto = v
	}
	if v, ok := fields["username"]; ok {
		user = v
	}
	if v, ok := fields["password"]; ok {
		pass = v
	}
	if v, ok := fields["hostname"]; ok {
		host = v
	}
	if v, ok := fields["port"]; ok {
		port = v
	}
	if v, ok := fields["pathname"]; ok {
		path = v
	}
	if v, ok := fields["search"]; ok {
		query = v
	}
	if v, ok := fields["hash"]; ok {
		hash = v
	}

	for _, comp := range []string{proto, user, pass, host, port, path, query, hash} {
		if strings.Contains(comp, "(") {
			hasRegex = true
			break
		}
	}

	return proto, user, pass, host, port, path, query, hash, hasRegex
}

func (g *generator) lowerURLPatternAlloc(proto, user, pass, host, port, path, query, hash, hasRegex ir.Operand) ir.Operand {
	t := g.semaResult.URLPatternType
	offsets, refMask, shape := g.objectLayout(t)
	res := g.currentFn.NewValue("url_pattern", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})

	for field, val := range map[string]ir.Operand{
		"protocol": proto, "username": user, "password": pass,
		"hostname": host, "port": port, "pathname": path,
		"search": query, "hash": hash, "hasRegExpGroups": hasRegex,
	} {
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
			Obj: res, Field: field, Offset: offsets[field], Val: val,
		})
	}
	return res
}

func (g *generator) lowerURLPatternNew(e *ast.NewExpr) ir.Operand {
	if len(e.Args) == 0 {
		err := g.newWebError(ir.ConstString{Value: "Failed to construct URLPattern: expected 1 or 2 arguments"}, ir.ConstString{Value: "TypeError"})
		g.routeThrownValue(err)
		return g.nullRef(g.semaResult.URLPatternType)
	}

	// If 1st argument is Object literal
	if objLit, ok := e.Args[0].(*ast.ObjectLit); ok {
		if len(e.Args) > 1 {
			err := g.newWebError(ir.ConstString{Value: "Failed to construct URLPattern: baseURL cannot be passed with init object"}, ir.ConstString{Value: "TypeError"})
			g.routeThrownValue(err)
			return g.nullRef(g.semaResult.URLPatternType)
		}
		fields := make(map[string]string)
		for _, prop := range objLit.Properties {
			if strLit, ok := prop.Value.(*ast.StringLit); ok {
				fields[prop.Key] = strLit.Value
			}
		}
		proto, user, pass, host, port, path, query, hash, hasRegex := parseURLPatternInitCompileTime(fields)
		return g.lowerURLPatternAlloc(
			ir.ConstString{Value: proto}, ir.ConstString{Value: user}, ir.ConstString{Value: pass},
			ir.ConstString{Value: host}, ir.ConstString{Value: port}, ir.ConstString{Value: path},
			ir.ConstString{Value: query}, ir.ConstString{Value: hash}, ir.ConstBool{Value: hasRegex},
		)
	}

	// If 1st argument is String literal
	if strLit, ok := e.Args[0].(*ast.StringLit); ok {
		baseStr := ""
		if len(e.Args) > 1 {
			if baseLit, ok := e.Args[1].(*ast.StringLit); ok {
				baseStr = baseLit.Value
			}
		}
		proto, user, pass, host, port, path, query, hash, hasRegex, err := parseURLPatternCompileTime(strLit.Value, baseStr)
		if err != nil || (!strings.Contains(strLit.Value, "://") && baseStr == "") {
			webErr := g.newWebError(ir.ConstString{Value: "Failed to construct URLPattern: invalid pattern"}, ir.ConstString{Value: "TypeError"})
			g.routeThrownValue(webErr)
			return g.nullRef(g.semaResult.URLPatternType)
		}
		return g.lowerURLPatternAlloc(
			ir.ConstString{Value: proto}, ir.ConstString{Value: user}, ir.ConstString{Value: pass},
			ir.ConstString{Value: host}, ir.ConstString{Value: port}, ir.ConstString{Value: path},
			ir.ConstString{Value: query}, ir.ConstString{Value: hash}, ir.ConstBool{Value: hasRegex},
		)
	}

	// Dynamic argument:
	inputVal := g.lowerExpr(e.Args[0])
	var baseVal ir.Operand = ir.ConstString{Value: ""}
	if len(e.Args) > 1 {
		baseVal = g.lowerExpr(e.Args[1])
	}
	invalidBB := g.currentFn.NewBlock("url_pattern_invalid")
	validBB := g.currentFn.NewBlock("url_pattern_valid")

	var resolved ir.Operand = inputVal
	hasScheme := g.urlStringFindByte(inputVal, ir.ConstNumber{Value: ':'}, ir.ConstNumber{Value: 0}, g.urlStringLen(inputVal))
	isRel := g.currentFn.NewValue("url_pat_is_rel", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: isRel, Op: ir.OpLt, LHS: hasScheme, RHS: ir.ConstNumber{Value: 0},
	})

	relBB := g.currentFn.NewBlock("url_pat_rel")
	absBB := g.currentFn.NewBlock("url_pat_abs")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isRel, Then: relBB, Else: absBB}

	g.currentBB = relBB
	baseLen := g.urlStringLen(baseVal)
	noBase := g.currentFn.NewValue("url_pat_no_base", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: noBase, Op: ir.OpEq, LHS: baseLen, RHS: ir.ConstNumber{Value: 0},
	})
	resolveBB := g.currentFn.NewBlock("url_pat_resolve")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: noBase, Then: invalidBB, Else: resolveBB}

	g.currentBB = resolveBB
	baseRecord := g.lowerURLParseRecord(baseVal, invalidBB)
	resolvedRel := g.lowerURLResolveInput(inputVal, baseRecord)
	resolvedRelBB := g.currentBB
	resolvedRelBB.Terminator = &ir.JumpTerm{Target: validBB}

	g.currentBB = absBB
	absBB.Terminator = &ir.JumpTerm{Target: validBB}

	g.currentBB = validBB
	resolvedPhis := g.currentFn.NewValue("url_pat_resolved", types.TypeString)
	validBB.Phis = append(validBB.Phis, &ir.PhiInst{
		Res: resolvedPhis,
		Incoming: []ir.PhiIncoming{
			{Block: resolvedRelBB, Value: resolvedRel},
			{Block: absBB, Value: resolved},
		},
	})
	record := g.lowerURLParseRecord(resolvedPhis, invalidBB)
	recordBB := g.currentBB
	pProto := g.lowerURLField(record, "$scheme")
	pHost := g.lowerURLField(record, "$hostname")
	pPort := g.lowerURLField(record, "$port")
	pPath := g.lowerURLField(record, "$pathname")
	pQuery := g.lowerURLField(record, "$query")
	pHash := g.lowerURLField(record, "$fragment")

	res := g.lowerURLPatternAlloc(
		pProto, ir.ConstString{Value: "*"}, ir.ConstString{Value: "*"},
		pHost, pPort, pPath, pQuery, pHash, ir.ConstBool{Value: false},
	)
	doneBB := g.currentFn.NewBlock("url_pat_done")
	recordBB.Terminator = &ir.JumpTerm{Target: doneBB}

	g.currentBB = invalidBB
	err := g.newWebError(ir.ConstString{Value: "Failed to construct URLPattern: invalid input"}, ir.ConstString{Value: "TypeError"})
	g.routeThrownValue(err)

	g.currentBB = doneBB
	return res
}

func (g *generator) lowerURLPatternMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	if !isBuiltinURLPatternType(g.semanticType(mem.Object)) {
		return nil, false
	}
	patternObj := g.lowerExpr(mem.Object)
	switch mem.Property {
	case "test":
		return g.lowerURLPatternTest(patternObj, e), true
	case "exec":
		return g.lowerURLPatternExec(patternObj, e), true
	}
	return nil, false
}

func (g *generator) lowerURLPatternFindParamEnd(pattern, start, pLen ir.Operand) ir.Operand {
	condBB := g.currentFn.NewBlock("param_end_cond")
	bodyBB := g.currentFn.NewBlock("param_end_body")
	stepBB := g.currentFn.NewBlock("param_end_step")
	endFromCondBB := g.currentFn.NewBlock("param_end_from_cond")
	endFromDelimBB := g.currentFn.NewBlock("param_end_from_delim")
	endBB := g.currentFn.NewBlock("param_end_done")

	idx := g.currentFn.NewValue("param_end_idx", types.TypeNumber)
	nextIdx := g.currentFn.NewValue("param_end_next", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{
		Res: idx,
		Incoming: []ir.PhiIncoming{
			{Block: g.currentBB, Value: start},
			{Block: stepBB, Value: nextIdx},
		},
	})
	g.currentBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = condBB
	more := g.currentFn.NewValue("param_end_more", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: more, Op: ir.OpLt, LHS: idx, RHS: pLen,
	})
	g.currentBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: endFromCondBB}

	g.currentBB = endFromCondBB
	endFromCondBB.Terminator = &ir.JumpTerm{Target: endBB}

	g.currentBB = bodyBB
	ch := g.urlStringByteAt(pattern, idx)
	// Check if ch is delimiter: '/', '.', '(', '?', '*', '{', ':', '#', '\\'
	isSlash := g.currentFn.NewValue("p_slash", types.TypeBoolean)
	isDot := g.currentFn.NewValue("p_dot", types.TypeBoolean)
	isParen := g.currentFn.NewValue("p_paren", types.TypeBoolean)
	isQ := g.currentFn.NewValue("p_q", types.TypeBoolean)
	isStar := g.currentFn.NewValue("p_star", types.TypeBoolean)
	isHash := g.currentFn.NewValue("p_hash", types.TypeBoolean)

	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: isSlash, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '/'}},
		&ir.BinaryInst{Res: isDot, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '.'}},
		&ir.BinaryInst{Res: isParen, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '('}},
		&ir.BinaryInst{Res: isQ, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '?'}},
		&ir.BinaryInst{Res: isStar, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '*'}},
		&ir.BinaryInst{Res: isHash, Op: ir.OpEq, LHS: ch, RHS: ir.ConstNumber{Value: '#'}},
	)
	or1 := g.currentFn.NewValue("p_or1", types.TypeBoolean)
	or2 := g.currentFn.NewValue("p_or2", types.TypeBoolean)
	or3 := g.currentFn.NewValue("p_or3", types.TypeBoolean)
	or12 := g.currentFn.NewValue("p_or12", types.TypeBoolean)
	isDelim := g.currentFn.NewValue("p_is_delim", types.TypeBoolean)

	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: or1, Op: ir.OpOr, LHS: isSlash, RHS: isDot},
		&ir.BinaryInst{Res: or2, Op: ir.OpOr, LHS: isParen, RHS: isQ},
		&ir.BinaryInst{Res: or3, Op: ir.OpOr, LHS: isStar, RHS: isHash},
		&ir.BinaryInst{Res: or12, Op: ir.OpOr, LHS: or1, RHS: or2},
		&ir.BinaryInst{Res: isDelim, Op: ir.OpOr, LHS: or12, RHS: or3},
		&ir.BinaryInst{Res: nextIdx, Op: ir.OpAdd, LHS: idx, RHS: ir.ConstNumber{Value: 1}},
	)
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isDelim, Then: endFromDelimBB, Else: stepBB}

	g.currentBB = stepBB
	stepBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = endFromDelimBB
	endFromDelimBB.Terminator = &ir.JumpTerm{Target: endBB}

	g.currentBB = endBB
	finalIdx := g.currentFn.NewValue("param_end_res", types.TypeNumber)
	endBB.Phis = append(endBB.Phis, &ir.PhiInst{
		Res: finalIdx,
		Incoming: []ir.PhiIncoming{
			{Block: endFromCondBB, Value: pLen},
			{Block: endFromDelimBB, Value: idx},
		},
	})
	return finalIdx
}

func (g *generator) lowerURLPatternMatchSingleComponent(pattern, input ir.Operand, isPathname, isHostname, captureGroups bool) (ir.Operand, ir.Operand) {
	entryBB := g.currentBB
	groups := g.newDynamicObject()

	// 1. If pattern == "*"
	isStar := g.urlStringEqual(pattern, "*")
	starMatchBB := g.currentFn.NewBlock("comp_star_match")
	nonStarBB := g.currentFn.NewBlock("comp_non_star")
	entryBB.Terminator = &ir.BranchTerm{Cond: isStar, Then: starMatchBB, Else: nonStarBB}

	g.currentBB = starMatchBB
	if captureGroups {
		g.callDynamicSet(groups, ir.ConstString{Value: "0"}, input)
	}
	joinBB := g.currentFn.NewBlock("comp_join")
	starMatchBB.Terminator = &ir.JumpTerm{Target: joinBB}

	// 2. Exact match
	g.currentBB = nonStarBB
	isExact := g.currentFn.NewValue("comp_is_exact", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: isExact, Callee: "ts_string_eq", Args: []ir.Operand{pattern, input},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	exactMatchBB := g.currentFn.NewBlock("comp_exact_match")
	generalBB := g.currentFn.NewBlock("comp_general")
	nonStarBB.Terminator = &ir.BranchTerm{Cond: isExact, Then: exactMatchBB, Else: generalBB}

	g.currentBB = exactMatchBB
	exactMatchBB.Terminator = &ir.JumpTerm{Target: joinBB}

	// 3. General pattern match loop
	g.currentBB = generalBB
	pLen := g.urlStringLen(pattern)
	iLen := g.urlStringLen(input)

	mismatchBB := g.currentFn.NewBlock("comp_mismatch")
	matchBB := g.currentFn.NewBlock("comp_match")

	loopCondBB := g.currentFn.NewBlock("comp_loop_cond")
	loopBodyBB := g.currentFn.NewBlock("comp_loop_body")
	loopEndBB := g.currentFn.NewBlock("comp_loop_end")

	generalBB.Terminator = &ir.JumpTerm{Target: loopCondBB}

	pIdx := g.currentFn.NewValue("comp_p_idx", types.TypeNumber)
	iIdx := g.currentFn.NewValue("comp_i_idx", types.TypeNumber)

	g.currentBB = loopCondBB
	more := g.currentFn.NewValue("comp_more", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: more, Op: ir.OpLt, LHS: pIdx, RHS: pLen,
	})
	g.currentBB.Terminator = &ir.BranchTerm{Cond: more, Then: loopBodyBB, Else: loopEndBB}

	g.currentBB = loopBodyBB
	pChar := g.urlStringByteAt(pattern, pIdx)

	isPStar := g.currentFn.NewValue("comp_p_is_star", types.TypeBoolean)
	isPColon := g.currentFn.NewValue("comp_p_is_colon", types.TypeBoolean)
	isPParen := g.currentFn.NewValue("comp_p_is_paren", types.TypeBoolean)

	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: isPStar, Op: ir.OpEq, LHS: pChar, RHS: ir.ConstNumber{Value: '*'}},
		&ir.BinaryInst{Res: isPColon, Op: ir.OpEq, LHS: pChar, RHS: ir.ConstNumber{Value: ':'}},
		&ir.BinaryInst{Res: isPParen, Op: ir.OpEq, LHS: pChar, RHS: ir.ConstNumber{Value: '('}},
	)

	starBB := g.currentFn.NewBlock("comp_star_token")
	nonStarTokenBB := g.currentFn.NewBlock("comp_non_star_token")
	paramBB := g.currentFn.NewBlock("comp_param_token")
	nonParamTokenBB := g.currentFn.NewBlock("comp_non_param_token")
	parenBB := g.currentFn.NewBlock("comp_paren_token")
	litBB := g.currentFn.NewBlock("comp_lit_token")

	loopBodyBB.Terminator = &ir.BranchTerm{Cond: isPStar, Then: starBB, Else: nonStarTokenBB}
	nonStarTokenBB.Terminator = &ir.BranchTerm{Cond: isPColon, Then: paramBB, Else: nonParamTokenBB}
	nonParamTokenBB.Terminator = &ir.BranchTerm{Cond: isPParen, Then: parenBB, Else: litBB}

	// Case A: Literal
	g.currentBB = litBB
	canReadI := g.currentFn.NewValue("lit_can_read", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: canReadI, Op: ir.OpLt, LHS: iIdx, RHS: iLen,
	})
	litReadBB := g.currentFn.NewBlock("lit_read")
	litBB.Terminator = &ir.BranchTerm{Cond: canReadI, Then: litReadBB, Else: mismatchBB}

	g.currentBB = litReadBB
	iChar := g.urlStringByteAt(input, iIdx)
	charEq := g.currentFn.NewValue("lit_char_eq", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: charEq, Op: ir.OpEq, LHS: pChar, RHS: iChar,
	})
	litAdvBB := g.currentFn.NewBlock("lit_advance")
	litReadBB.Terminator = &ir.BranchTerm{Cond: charEq, Then: litAdvBB, Else: mismatchBB}

	g.currentBB = litAdvBB
	litNextP := g.currentFn.NewValue("lit_next_p", types.TypeNumber)
	litNextI := g.currentFn.NewValue("lit_next_i", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: litNextP, Op: ir.OpAdd, LHS: pIdx, RHS: ir.ConstNumber{Value: 1}},
		&ir.BinaryInst{Res: litNextI, Op: ir.OpAdd, LHS: iIdx, RHS: ir.ConstNumber{Value: 1}},
	)
	litAdvBB.Terminator = &ir.JumpTerm{Target: loopCondBB}

	// Case B: Wildcard '*'
	g.currentBB = starBB
	starP1 := g.currentFn.NewValue("star_p1", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: starP1, Op: ir.OpAdd, LHS: pIdx, RHS: ir.ConstNumber{Value: 1},
	})
	isStarEnd := g.currentFn.NewValue("star_is_end", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: isStarEnd, Op: ir.OpGe, LHS: starP1, RHS: pLen,
	})
	starEndBB := g.currentFn.NewBlock("star_end")
	starMidBB := g.currentFn.NewBlock("star_mid")
	starBB.Terminator = &ir.BranchTerm{Cond: isStarEnd, Then: starEndBB, Else: starMidBB}

	g.currentBB = starEndBB
	starCap := g.urlStringSlice(input, iIdx, iLen)
	if captureGroups {
		g.callDynamicSet(groups, ir.ConstString{Value: "0"}, starCap)
	}
	starEndAdvBB := g.currentFn.NewBlock("star_end_adv")
	starEndBB.Terminator = &ir.JumpTerm{Target: starEndAdvBB}
	g.currentBB = starEndAdvBB
	starEndAdvBB.Terminator = &ir.JumpTerm{Target: loopCondBB}

	g.currentBB = starMidBB
	nextDelimByte := g.urlStringByteAt(pattern, starP1)
	starFoundPos := g.urlStringFindByte(input, nextDelimByte, iIdx, iLen)
	starFoundOK := g.currentFn.NewValue("star_found_ok", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: starFoundOK, Op: ir.OpGe, LHS: starFoundPos, RHS: ir.ConstNumber{Value: 0},
	})
	starMidAdvBB := g.currentFn.NewBlock("star_mid_adv")
	starMidBB.Terminator = &ir.BranchTerm{Cond: starFoundOK, Then: starMidAdvBB, Else: mismatchBB}

	g.currentBB = starMidAdvBB
	starMidCap := g.urlStringSlice(input, iIdx, starFoundPos)
	if captureGroups {
		g.callDynamicSet(groups, ir.ConstString{Value: "0"}, starMidCap)
	}
	starMidAdvBB.Terminator = &ir.JumpTerm{Target: loopCondBB}

	// Case C: Named Parameter ':name'
	g.currentBB = paramBB
	pParamStart := g.currentFn.NewValue("p_param_start", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: pParamStart, Op: ir.OpAdd, LHS: pIdx, RHS: ir.ConstNumber{Value: 1},
	})
	pParamEnd := g.lowerURLPatternFindParamEnd(pattern, pParamStart, pLen)
	paramName := g.urlStringSlice(pattern, pParamStart, pParamEnd)

	// Delimiter byte in input:
	var delimByte ir.Operand = ir.ConstNumber{Value: -1}
	if isPathname {
		delimByte = ir.ConstNumber{Value: '/'}
	} else if isHostname {
		delimByte = ir.ConstNumber{Value: '.'}
	} else {
		hasParamDelim := g.currentFn.NewValue("has_p_delim", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
			Res: hasParamDelim, Op: ir.OpLt, LHS: pParamEnd, RHS: pLen,
		})
		delimFromPattern := g.urlStringByteAt(pattern, pParamEnd)
		delimJoin := g.currentFn.NewBlock("param_delim_join")
		delimThen := g.currentFn.NewBlock("param_delim_then")
		delimElse := g.currentFn.NewBlock("param_delim_else")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: hasParamDelim, Then: delimThen, Else: delimElse}

		delimThen.Terminator = &ir.JumpTerm{Target: delimJoin}
		delimElse.Terminator = &ir.JumpTerm{Target: delimJoin}

		g.currentBB = delimJoin
		delimPhi := g.currentFn.NewValue("p_delim_phi", types.TypeNumber)
		delimJoin.Phis = append(delimJoin.Phis, &ir.PhiInst{
			Res: delimPhi,
			Incoming: []ir.PhiIncoming{
				{Block: delimThen, Value: delimFromPattern},
				{Block: delimElse, Value: ir.ConstNumber{Value: -1}},
			},
		})
		delimByte = delimPhi
	}

	hasDelim := g.currentFn.NewValue("has_delim_gt", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: hasDelim, Op: ir.OpGe, LHS: delimByte, RHS: ir.ConstNumber{Value: 0},
	})
	findDelimBB := g.currentFn.NewBlock("param_find_delim")
	noDelimBB := g.currentFn.NewBlock("param_no_delim")
	delimJoinBB := g.currentFn.NewBlock("param_join_delim")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasDelim, Then: findDelimBB, Else: noDelimBB}

	g.currentBB = findDelimBB
	foundDelimPos := g.urlStringFindByte(input, delimByte, iIdx, iLen)
	hasFoundDelim := g.currentFn.NewValue("has_found_delim", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: hasFoundDelim, Op: ir.OpGe, LHS: foundDelimPos, RHS: ir.ConstNumber{Value: 0},
	})
	foundDelimOKBB := g.currentFn.NewBlock("param_delim_ok")
	foundDelimNoBB := g.currentFn.NewBlock("param_delim_no")
	findDelimBB.Terminator = &ir.BranchTerm{Cond: hasFoundDelim, Then: foundDelimOKBB, Else: foundDelimNoBB}

	foundDelimOKBB.Terminator = &ir.JumpTerm{Target: delimJoinBB}
	foundDelimNoBB.Terminator = &ir.JumpTerm{Target: delimJoinBB}
	noDelimBB.Terminator = &ir.JumpTerm{Target: delimJoinBB}

	g.currentBB = delimJoinBB
	endPos := g.currentFn.NewValue("param_end_pos", types.TypeNumber)
	delimJoinBB.Phis = append(delimJoinBB.Phis, &ir.PhiInst{
		Res: endPos,
		Incoming: []ir.PhiIncoming{
			{Block: foundDelimOKBB, Value: foundDelimPos},
			{Block: foundDelimNoBB, Value: iLen},
			{Block: noDelimBB, Value: iLen},
		},
	})

	paramCap := g.urlStringSlice(input, iIdx, endPos)
	paramCapLen := g.urlStringLen(paramCap)
	hasParamCap := g.currentFn.NewValue("has_param_cap", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: hasParamCap, Op: ir.OpGt, LHS: paramCapLen, RHS: ir.ConstNumber{Value: 0},
	})
	paramCheckBB := g.currentFn.NewBlock("param_check")
	delimJoinBB.Terminator = &ir.BranchTerm{Cond: hasParamCap, Then: paramCheckBB, Else: mismatchBB}

	// Check regex group if :name(\d+)
	g.currentBB = paramCheckBB
	hasParenGroup := g.currentFn.NewValue("has_paren_group", types.TypeBoolean)
	paramCanHaveParen := g.currentFn.NewValue("can_have_paren", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: paramCanHaveParen, Op: ir.OpLt, LHS: pParamEnd, RHS: pLen,
	})
	paramParenCheckBB := g.currentFn.NewBlock("param_paren_check")
	paramNoParenBB := g.currentFn.NewBlock("param_no_paren")
	paramCheckBB.Terminator = &ir.BranchTerm{Cond: paramCanHaveParen, Then: paramParenCheckBB, Else: paramNoParenBB}

	g.currentBB = paramParenCheckBB
	parenByte := g.urlStringByteAt(pattern, pParamEnd)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: hasParenGroup, Op: ir.OpEq, LHS: parenByte, RHS: ir.ConstNumber{Value: '('},
	})
	paramHasParenBB := g.currentFn.NewBlock("param_has_paren")
	paramParenCheckBB.Terminator = &ir.BranchTerm{Cond: hasParenGroup, Then: paramHasParenBB, Else: paramNoParenBB}

	g.currentBB = paramHasParenBB
	closeParenPos := g.urlStringFindByte(pattern, ir.ConstNumber{Value: ')'}, pParamEnd, pLen)
	parenNextP := g.currentFn.NewValue("paren_next_p", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: parenNextP, Op: ir.OpAdd, LHS: closeParenPos, RHS: ir.ConstNumber{Value: 1},
	})

	capDigitIdx := g.currentFn.NewValue("cap_d_idx", types.TypeNumber)
	capNextDigitIdx := g.currentFn.NewValue("cap_d_next", types.TypeNumber)
	digitCondBB := g.currentFn.NewBlock("param_digit_cond")
	digitBodyBB := g.currentFn.NewBlock("param_digit_body")
	digitStepBB := g.currentFn.NewBlock("param_digit_step")
	digitPassBB := g.currentFn.NewBlock("param_digit_pass")
	digitCondBB.Phis = append(digitCondBB.Phis, &ir.PhiInst{
		Res: capDigitIdx,
		Incoming: []ir.PhiIncoming{
			{Block: paramHasParenBB, Value: ir.ConstNumber{Value: 0}},
			{Block: digitStepBB, Value: capNextDigitIdx},
		},
	})
	paramHasParenBB.Terminator = &ir.JumpTerm{Target: digitCondBB}

	g.currentBB = digitCondBB
	moreDigits := g.currentFn.NewValue("more_digits", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: moreDigits, Op: ir.OpLt, LHS: capDigitIdx, RHS: paramCapLen,
	})
	digitCondBB.Terminator = &ir.BranchTerm{Cond: moreDigits, Then: digitBodyBB, Else: digitPassBB}

	g.currentBB = digitBodyBB
	bDigit := g.urlStringByteAt(paramCap, capDigitIdx)
	ge0 := g.currentFn.NewValue("ge0", types.TypeBoolean)
	le9 := g.currentFn.NewValue("le9", types.TypeBoolean)
	isDigit := g.currentFn.NewValue("is_digit", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: ge0, Op: ir.OpGe, LHS: bDigit, RHS: ir.ConstNumber{Value: '0'}},
		&ir.BinaryInst{Res: le9, Op: ir.OpLe, LHS: bDigit, RHS: ir.ConstNumber{Value: '9'}},
		&ir.BinaryInst{Res: isDigit, Op: ir.OpAnd, LHS: ge0, RHS: le9},
		&ir.BinaryInst{Res: capNextDigitIdx, Op: ir.OpAdd, LHS: capDigitIdx, RHS: ir.ConstNumber{Value: 1}},
	)
	digitBodyBB.Terminator = &ir.BranchTerm{Cond: isDigit, Then: digitStepBB, Else: mismatchBB}

	g.currentBB = digitStepBB
	digitStepBB.Terminator = &ir.JumpTerm{Target: digitCondBB}

	g.currentBB = digitPassBB
	paramParenJoinBB := g.currentFn.NewBlock("param_paren_join")
	digitPassBB.Terminator = &ir.JumpTerm{Target: paramParenJoinBB}
	paramNoParenBB.Terminator = &ir.JumpTerm{Target: paramParenJoinBB}

	g.currentBB = paramParenJoinBB
	actualNextP := g.currentFn.NewValue("actual_next_p", types.TypeNumber)
	paramParenJoinBB.Phis = append(paramParenJoinBB.Phis, &ir.PhiInst{
		Res: actualNextP,
		Incoming: []ir.PhiIncoming{
			{Block: digitPassBB, Value: parenNextP},
			{Block: paramNoParenBB, Value: pParamEnd},
		},
	})

	if captureGroups {
		g.callDynamicSet(groups, paramName, paramCap)
	}
	paramParenJoinBB.Terminator = &ir.JumpTerm{Target: loopCondBB}

	// Case D: Regex Group '(pattern)'
	g.currentBB = parenBB
	closeParen := g.urlStringFindByte(pattern, ir.ConstNumber{Value: ')'}, pIdx, pLen)
	hasCloseParen := g.currentFn.NewValue("has_close_paren", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: hasCloseParen, Op: ir.OpGe, LHS: closeParen, RHS: ir.ConstNumber{Value: 0},
	})
	parenCloseOKBB := g.currentFn.NewBlock("paren_close_ok")
	parenBB.Terminator = &ir.BranchTerm{Cond: hasCloseParen, Then: parenCloseOKBB, Else: mismatchBB}

	g.currentBB = parenCloseOKBB
	p1Paren := g.currentFn.NewValue("p1_paren", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: p1Paren, Op: ir.OpAdd, LHS: pIdx, RHS: ir.ConstNumber{Value: 1},
	})
	subPat := g.urlStringSlice(pattern, p1Paren, closeParen)
	// Check if subPat contains '|'
	pipePos := g.urlStringFindByte(subPat, ir.ConstNumber{Value: '|'}, ir.ConstNumber{Value: 0}, g.urlStringLen(subPat))
	hasPipe := g.currentFn.NewValue("has_pipe", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: hasPipe, Op: ir.OpGe, LHS: pipePos, RHS: ir.ConstNumber{Value: 0},
	})
	pipeBB := g.currentFn.NewBlock("paren_pipe")
	noPipeBB := g.currentFn.NewBlock("paren_no_pipe")
	parenMatchedBB := g.currentFn.NewBlock("paren_matched")
	parenCloseOKBB.Terminator = &ir.BranchTerm{Cond: hasPipe, Then: pipeBB, Else: noPipeBB}

	g.currentBB = pipeBB
	opt1 := g.urlStringSlice(subPat, ir.ConstNumber{Value: 0}, pipePos)
	p1Pipe := g.currentFn.NewValue("p1_pipe", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: p1Pipe, Op: ir.OpAdd, LHS: pipePos, RHS: ir.ConstNumber{Value: 1},
	})
	opt2 := g.urlStringSlice(subPat, p1Pipe, g.urlStringLen(subPat))
	opt1Len := g.urlStringLen(opt1)
	opt2Len := g.urlStringLen(opt2)

	// Check if input starting at iIdx matches opt1
	iOpt1End := g.currentFn.NewValue("i_opt1_end", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: iOpt1End, Op: ir.OpAdd, LHS: iIdx, RHS: opt1Len,
	})
	canMatchOpt1 := g.currentFn.NewValue("can_match_opt1", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: canMatchOpt1, Op: ir.OpLe, LHS: iOpt1End, RHS: iLen,
	})
	opt1CheckBB := g.currentFn.NewBlock("opt1_check")
	opt2CheckBB := g.currentFn.NewBlock("opt2_check")
	pipeBB.Terminator = &ir.BranchTerm{Cond: canMatchOpt1, Then: opt1CheckBB, Else: opt2CheckBB}

	g.currentBB = opt1CheckBB
	iOpt1Slice := g.urlStringSlice(input, iIdx, iOpt1End)
	opt1Eq := g.currentFn.NewValue("opt1_eq", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: opt1Eq, Callee: "ts_string_eq", Args: []ir.Operand{iOpt1Slice, opt1},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	opt1MatchBB := g.currentFn.NewBlock("opt1_match")
	opt1CheckBB.Terminator = &ir.BranchTerm{Cond: opt1Eq, Then: opt1MatchBB, Else: opt2CheckBB}

	opt1MatchBB.Terminator = &ir.JumpTerm{Target: parenMatchedBB}

	g.currentBB = opt2CheckBB
	iOpt2End := g.currentFn.NewValue("i_opt2_end", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: iOpt2End, Op: ir.OpAdd, LHS: iIdx, RHS: opt2Len,
	})
	canMatchOpt2 := g.currentFn.NewValue("can_match_opt2", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: canMatchOpt2, Op: ir.OpLe, LHS: iOpt2End, RHS: iLen,
	})
	opt2TestBB := g.currentFn.NewBlock("opt2_test")
	opt2CheckBB.Terminator = &ir.BranchTerm{Cond: canMatchOpt2, Then: opt2TestBB, Else: mismatchBB}

	g.currentBB = opt2TestBB
	iOpt2Slice := g.urlStringSlice(input, iIdx, iOpt2End)
	opt2Eq := g.currentFn.NewValue("opt2_eq", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: opt2Eq, Callee: "ts_string_eq", Args: []ir.Operand{iOpt2Slice, opt2},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	opt2MatchBB := g.currentFn.NewBlock("opt2_match")
	opt2TestBB.Terminator = &ir.BranchTerm{Cond: opt2Eq, Then: opt2MatchBB, Else: mismatchBB}

	opt2MatchBB.Terminator = &ir.JumpTerm{Target: parenMatchedBB}

	g.currentBB = noPipeBB
	subLen := g.urlStringLen(subPat)
	iSubEnd := g.currentFn.NewValue("i_sub_end", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: iSubEnd, Op: ir.OpAdd, LHS: iIdx, RHS: subLen,
	})
	canMatchSub := g.currentFn.NewValue("can_match_sub", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: canMatchSub, Op: ir.OpLe, LHS: iSubEnd, RHS: iLen,
	})
	noPipeTestBB := g.currentFn.NewBlock("no_pipe_test")
	noPipeBB.Terminator = &ir.BranchTerm{Cond: canMatchSub, Then: noPipeTestBB, Else: mismatchBB}

	g.currentBB = noPipeTestBB
	iSubSlice := g.urlStringSlice(input, iIdx, iSubEnd)
	subEq := g.currentFn.NewValue("sub_eq", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: subEq, Callee: "ts_string_eq", Args: []ir.Operand{iSubSlice, subPat},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	noPipeMatchBB := g.currentFn.NewBlock("no_pipe_match")
	noPipeTestBB.Terminator = &ir.BranchTerm{Cond: subEq, Then: noPipeMatchBB, Else: mismatchBB}

	noPipeMatchBB.Terminator = &ir.JumpTerm{Target: parenMatchedBB}

	g.currentBB = parenMatchedBB
	matchedParenStr := g.currentFn.NewValue("matched_paren_str", types.TypeString)
	matchedParenLen := g.currentFn.NewValue("matched_paren_len", types.TypeNumber)
	parenMatchedBB.Phis = append(parenMatchedBB.Phis,
		&ir.PhiInst{Res: matchedParenStr, Incoming: []ir.PhiIncoming{
			{Block: opt1MatchBB, Value: opt1},
			{Block: opt2MatchBB, Value: opt2},
			{Block: noPipeMatchBB, Value: subPat},
		}},
		&ir.PhiInst{Res: matchedParenLen, Incoming: []ir.PhiIncoming{
			{Block: opt1MatchBB, Value: opt1Len},
			{Block: opt2MatchBB, Value: opt2Len},
			{Block: noPipeMatchBB, Value: subLen},
		}},
	)
	if captureGroups {
		g.callDynamicSet(groups, ir.ConstString{Value: "0"}, matchedParenStr)
	}
	nextParenP := g.currentFn.NewValue("next_paren_p", types.TypeNumber)
	nextParenI := g.currentFn.NewValue("next_paren_i", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: nextParenP, Op: ir.OpAdd, LHS: closeParen, RHS: ir.ConstNumber{Value: 1}},
		&ir.BinaryInst{Res: nextParenI, Op: ir.OpAdd, LHS: iIdx, RHS: matchedParenLen},
	)
	parenMatchedBB.Terminator = &ir.JumpTerm{Target: loopCondBB}

	// Wiring Phis back into loopCondBB:
	loopCondBB.Phis = append(loopCondBB.Phis,
		&ir.PhiInst{Res: pIdx, Incoming: []ir.PhiIncoming{
			{Block: generalBB, Value: ir.ConstNumber{Value: 0}},
			{Block: litAdvBB, Value: litNextP},
			{Block: starEndAdvBB, Value: pLen},
			{Block: starMidAdvBB, Value: starP1},
			{Block: paramParenJoinBB, Value: actualNextP},
			{Block: parenMatchedBB, Value: nextParenP},
		}},
		&ir.PhiInst{Res: iIdx, Incoming: []ir.PhiIncoming{
			{Block: generalBB, Value: ir.ConstNumber{Value: 0}},
			{Block: litAdvBB, Value: litNextI},
			{Block: starEndAdvBB, Value: iLen},
			{Block: starMidAdvBB, Value: starFoundPos},
			{Block: paramParenJoinBB, Value: endPos},
			{Block: parenMatchedBB, Value: nextParenI},
		}},
	)

	// Loop End:
	g.currentBB = loopEndBB
	pDone := g.currentFn.NewValue("p_done", types.TypeBoolean)
	iDone := g.currentFn.NewValue("i_done", types.TypeBoolean)
	bothDone := g.currentFn.NewValue("both_done", types.TypeBoolean)

	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: pDone, Op: ir.OpGe, LHS: pIdx, RHS: pLen},
		&ir.BinaryInst{Res: iDone, Op: ir.OpGe, LHS: iIdx, RHS: iLen},
		&ir.BinaryInst{Res: bothDone, Op: ir.OpAnd, LHS: pDone, RHS: iDone},
	)
	loopEndBB.Terminator = &ir.BranchTerm{Cond: bothDone, Then: matchBB, Else: mismatchBB}

	g.moveBlockToEnd(matchBB)
	g.moveBlockToEnd(mismatchBB)
	g.moveBlockToEnd(joinBB)

	g.currentBB = matchBB
	matchBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = mismatchBB
	mismatchBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = joinBB
	matchedRes := g.currentFn.NewValue("comp_matched", types.TypeBoolean)
	groupsRes := g.currentFn.NewValue("comp_groups", types.TypeAny)

	joinBB.Phis = append(joinBB.Phis,
		&ir.PhiInst{Res: matchedRes, Incoming: []ir.PhiIncoming{
			{Block: starMatchBB, Value: ir.ConstBool{Value: true}},
			{Block: exactMatchBB, Value: ir.ConstBool{Value: true}},
			{Block: matchBB, Value: ir.ConstBool{Value: true}},
			{Block: mismatchBB, Value: ir.ConstBool{Value: false}},
		}},
		&ir.PhiInst{Res: groupsRes, Incoming: []ir.PhiIncoming{
			{Block: starMatchBB, Value: groups},
			{Block: exactMatchBB, Value: groups},
			{Block: matchBB, Value: groups},
			{Block: mismatchBB, Value: ir.ConstNull{}},
		}},
	)

	return matchedRes, groupsRes
}

func (g *generator) lowerURLPatternParseInput(patternObj ir.Operand, e *ast.CallExpr, onMismatch *ir.BasicBlock) (proto, user, pass, host, port, path, query, hash ir.Operand) {
	if len(e.Args) == 0 {
		return ir.ConstString{Value: ""}, ir.ConstString{Value: ""}, ir.ConstString{Value: ""},
			ir.ConstString{Value: ""}, ir.ConstString{Value: ""}, ir.ConstString{Value: ""},
			ir.ConstString{Value: ""}, ir.ConstString{Value: ""}
	}

	argType := g.semanticType(e.Args[0])

	// If argument is Object
	if _, ok := argType.(*types.ObjectType); ok && argType != g.semaResult.URLType {
		inputObj := g.lowerExpr(e.Args[0])
		mGet := func(field string) ir.Operand {
			res, ok := g.lowerWebIDLDictionaryMember(e.Args[0], inputObj, field, types.TypeString, ir.ConstString{Value: ""})
			if !ok {
				return ir.ConstString{Value: ""}
			}
			return res
		}
		return mGet("protocol"), mGet("username"), mGet("password"),
			mGet("hostname"), mGet("port"), mGet("pathname"),
			mGet("search"), mGet("hash")
	}

	// String or URL
	var baseExpr ast.Expr
	if len(e.Args) > 1 {
		baseExpr = e.Args[1]
	}
	rec := g.lowerURLResolveAndParse(e.Args[0], baseExpr, onMismatch)
	pProto := g.lowerURLField(rec, "$scheme")
	pHost := g.lowerURLField(rec, "$hostname")
	pPort := g.lowerURLField(rec, "$port")
	pPath := g.lowerURLField(rec, "$pathname")
	pQuery := g.lowerURLField(rec, "$query")
	pHash := g.lowerURLField(rec, "$fragment")

	return pProto, ir.ConstString{Value: ""}, ir.ConstString{Value: ""},
		pHost, pPort, pPath, pQuery, pHash
}

func (g *generator) lowerURLPatternTest(patternObj ir.Operand, e *ast.CallExpr) ir.Operand {
	mismatchBB := g.currentFn.NewBlock("url_pat_test_mismatch")
	matchBB := g.currentFn.NewBlock("url_pat_test_match")
	joinBB := g.currentFn.NewBlock("url_pat_test_join")

	inProto, inUser, inPass, inHost, inPort, inPath, inQuery, inHash := g.lowerURLPatternParseInput(patternObj, e, mismatchBB)

	patProto := g.lowerURLPatternField(patternObj, "protocol")
	patUser := g.lowerURLPatternField(patternObj, "username")
	patPass := g.lowerURLPatternField(patternObj, "password")
	patHost := g.lowerURLPatternField(patternObj, "hostname")
	patPort := g.lowerURLPatternField(patternObj, "port")
	patPath := g.lowerURLPatternField(patternObj, "pathname")
	patQuery := g.lowerURLPatternField(patternObj, "search")
	patHash := g.lowerURLPatternField(patternObj, "hash")

	// Match sequentially:
	mProto, _ := g.lowerURLPatternMatchSingleComponent(patProto, inProto, false, false, false)
	cProtoBB := g.currentFn.NewBlock("test_c_proto")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mProto, Then: cProtoBB, Else: mismatchBB}

	g.currentBB = cProtoBB
	mUser, _ := g.lowerURLPatternMatchSingleComponent(patUser, inUser, false, false, false)
	cUserBB := g.currentFn.NewBlock("test_c_user")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mUser, Then: cUserBB, Else: mismatchBB}

	g.currentBB = cUserBB
	mPass, _ := g.lowerURLPatternMatchSingleComponent(patPass, inPass, false, false, false)
	cPassBB := g.currentFn.NewBlock("test_c_pass")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mPass, Then: cPassBB, Else: mismatchBB}

	g.currentBB = cPassBB
	mHost, _ := g.lowerURLPatternMatchSingleComponent(patHost, inHost, false, true, false)
	cHostBB := g.currentFn.NewBlock("test_c_host")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mHost, Then: cHostBB, Else: mismatchBB}

	g.currentBB = cHostBB
	mPort, _ := g.lowerURLPatternMatchSingleComponent(patPort, inPort, false, false, false)
	cPortBB := g.currentFn.NewBlock("test_c_port")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mPort, Then: cPortBB, Else: mismatchBB}

	g.currentBB = cPortBB
	mPath, _ := g.lowerURLPatternMatchSingleComponent(patPath, inPath, true, false, false)
	cPathBB := g.currentFn.NewBlock("test_c_path")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mPath, Then: cPathBB, Else: mismatchBB}

	g.currentBB = cPathBB
	mQuery, _ := g.lowerURLPatternMatchSingleComponent(patQuery, inQuery, false, false, false)
	cQueryBB := g.currentFn.NewBlock("test_c_query")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mQuery, Then: cQueryBB, Else: mismatchBB}

	g.currentBB = cQueryBB
	mHash, _ := g.lowerURLPatternMatchSingleComponent(patHash, inHash, false, false, false)
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mHash, Then: matchBB, Else: mismatchBB}

	g.moveBlockToEnd(matchBB)
	g.moveBlockToEnd(mismatchBB)
	g.moveBlockToEnd(joinBB)

	g.currentBB = matchBB
	matchBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = mismatchBB
	mismatchBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = joinBB
	res := g.currentFn.NewValue("url_pat_test_res", types.TypeBoolean)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{
		Res: res,
		Incoming: []ir.PhiIncoming{
			{Block: matchBB, Value: ir.ConstBool{Value: true}},
			{Block: mismatchBB, Value: ir.ConstBool{Value: false}},
		},
	})
	return res
}

func (g *generator) lowerURLPatternAllocCompResult(input, groups ir.Operand) ir.Operand {
	t := g.semaResult.URLPatternComponentResultType
	offsets, refMask, shape := g.objectLayout(t)
	res := g.currentFn.NewValue("comp_res", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: res, Field: "input", Offset: offsets["input"], Val: input},
		&ir.SetFieldInst{Obj: res, Field: "groups", Offset: offsets["groups"], Val: groups},
	)
	return res
}

func (g *generator) lowerURLPatternExec(patternObj ir.Operand, e *ast.CallExpr) ir.Operand {
	mismatchBB := g.currentFn.NewBlock("url_pat_exec_mismatch")
	matchBB := g.currentFn.NewBlock("url_pat_exec_match")
	joinBB := g.currentFn.NewBlock("url_pat_exec_join")

	inProto, inUser, inPass, inHost, inPort, inPath, inQuery, inHash := g.lowerURLPatternParseInput(patternObj, e, mismatchBB)

	patProto := g.lowerURLPatternField(patternObj, "protocol")
	patUser := g.lowerURLPatternField(patternObj, "username")
	patPass := g.lowerURLPatternField(patternObj, "password")
	patHost := g.lowerURLPatternField(patternObj, "hostname")
	patPort := g.lowerURLPatternField(patternObj, "port")
	patPath := g.lowerURLPatternField(patternObj, "pathname")
	patQuery := g.lowerURLPatternField(patternObj, "search")
	patHash := g.lowerURLPatternField(patternObj, "hash")

	// Match sequentially with group capture:
	mProto, gProto := g.lowerURLPatternMatchSingleComponent(patProto, inProto, false, false, true)
	cProtoBB := g.currentFn.NewBlock("exec_c_proto")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mProto, Then: cProtoBB, Else: mismatchBB}

	g.currentBB = cProtoBB
	mUser, gUser := g.lowerURLPatternMatchSingleComponent(patUser, inUser, false, false, true)
	cUserBB := g.currentFn.NewBlock("exec_c_user")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mUser, Then: cUserBB, Else: mismatchBB}

	g.currentBB = cUserBB
	mPass, gPass := g.lowerURLPatternMatchSingleComponent(patPass, inPass, false, false, true)
	cPassBB := g.currentFn.NewBlock("exec_c_pass")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mPass, Then: cPassBB, Else: mismatchBB}

	g.currentBB = cPassBB
	mHost, gHost := g.lowerURLPatternMatchSingleComponent(patHost, inHost, false, true, true)
	cHostBB := g.currentFn.NewBlock("exec_c_host")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mHost, Then: cHostBB, Else: mismatchBB}

	g.currentBB = cHostBB
	mPort, gPort := g.lowerURLPatternMatchSingleComponent(patPort, inPort, false, false, true)
	cPortBB := g.currentFn.NewBlock("exec_c_port")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mPort, Then: cPortBB, Else: mismatchBB}

	g.currentBB = cPortBB
	mPath, gPath := g.lowerURLPatternMatchSingleComponent(patPath, inPath, true, false, true)
	cPathBB := g.currentFn.NewBlock("exec_c_path")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mPath, Then: cPathBB, Else: mismatchBB}

	g.currentBB = cPathBB
	mQuery, gQuery := g.lowerURLPatternMatchSingleComponent(patQuery, inQuery, false, false, true)
	cQueryBB := g.currentFn.NewBlock("exec_c_query")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mQuery, Then: cQueryBB, Else: mismatchBB}

	g.currentBB = cQueryBB
	mHash, gHash := g.lowerURLPatternMatchSingleComponent(patHash, inHash, false, false, true)
	g.currentBB.Terminator = &ir.BranchTerm{Cond: mHash, Then: matchBB, Else: mismatchBB}

	g.moveBlockToEnd(matchBB)
	g.moveBlockToEnd(mismatchBB)
	g.moveBlockToEnd(joinBB)

	g.currentBB = matchBB
	// Construct inputs array
	elemCount := 0
	if len(e.Args) > 0 {
		elemCount++
	}
	if len(e.Args) > 1 {
		elemCount++
	}
	inputsArr := g.currentFn.NewValue("exec_inputs", types.NewArray(types.TypeAny))
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: inputsArr, ElemType: types.TypeAny, Length: ir.ConstNumber{Value: float64(elemCount)},
	})
	if len(e.Args) > 0 {
		rawInput := g.lowerExpr(e.Args[0])
		boxedIn := rawInput
		if !irJSValueType(rawInput.Type()) {
			boxedIn = g.boxJSValue(rawInput, rawInput.Type())
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{
			Array: inputsArr, Index: ir.ConstNumber{Value: 0}, Val: boxedIn,
		})
	}
	if len(e.Args) > 1 {
		rawBase := g.lowerExpr(e.Args[1])
		boxedBase := rawBase
		if !irJSValueType(rawBase.Type()) {
			boxedBase = g.boxJSValue(rawBase, rawBase.Type())
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{
			Array: inputsArr, Index: ir.ConstNumber{Value: 1}, Val: boxedBase,
		})
	}

	resProto := g.lowerURLPatternAllocCompResult(inProto, gProto)
	resUser := g.lowerURLPatternAllocCompResult(inUser, gUser)
	resPass := g.lowerURLPatternAllocCompResult(inPass, gPass)
	resHost := g.lowerURLPatternAllocCompResult(inHost, gHost)
	resPort := g.lowerURLPatternAllocCompResult(inPort, gPort)
	resPath := g.lowerURLPatternAllocCompResult(inPath, gPath)
	resQuery := g.lowerURLPatternAllocCompResult(inQuery, gQuery)
	resHash := g.lowerURLPatternAllocCompResult(inHash, gHash)

	resT := g.semaResult.URLPatternResultType
	resOffsets, resRefMask, resShape := g.objectLayout(resT)
	resObj := g.currentFn.NewValue("url_pattern_result", resT)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: resObj, Shape: resShape, FieldCount: len(resOffsets), RefMask: resRefMask,
	})

	for field, val := range map[string]ir.Operand{
		"inputs": inputsArr, "protocol": resProto, "username": resUser,
		"password": resPass, "hostname": resHost, "port": resPort,
		"pathname": resPath, "search": resQuery, "hash": resHash,
	} {
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
			Obj: resObj, Field: field, Offset: resOffsets[field], Val: val,
		})
	}
	matchBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = mismatchBB
	mismatchBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = joinBB
	execRes := g.currentFn.NewValue("url_pat_exec_res", resT)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{
		Res: execRes,
		Incoming: []ir.PhiIncoming{
			{Block: matchBB, Value: resObj},
			{Block: mismatchBB, Value: ir.ConstNull{}},
		},
	})
	return execRes
}
