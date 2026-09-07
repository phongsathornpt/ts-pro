package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) urlStringByteAt(value, index ir.Operand) ir.Operand {
	res := g.currentFn.NewValue("url_string_byte", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_string_byte_at", Args: []ir.Operand{value, index},
		ParamTypes: []types.Type{types.TypeString, types.TypeNumber},
	})
	return res
}

func (g *generator) urlStringFindFirstDelimiter(value, start ir.Operand) ir.Operand {
	res := g.currentFn.NewValue("url_first_delimiter", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_string_find_first_of3", Args: []ir.Operand{value, start},
		ParamTypes: []types.Type{types.TypeString, types.TypeNumber},
	})
	return res
}

func (g *generator) lowerURLAllocRecord(scheme, hostname, port, pathname, query, fragment ir.Operand) ir.Operand {
	t := g.semaResult.URLType
	offsets, refMask, shape := g.objectLayout(t)
	res := g.currentFn.NewValue("url_record", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	for field, value := range map[string]ir.Operand{
		"$scheme": scheme, "$hostname": hostname, "$port": port,
		"$pathname": pathname, "$query": query, "$fragment": fragment,
	} {
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
			Obj: res, Field: field, Offset: offsets[field], Val: value,
		})
	}
	params := g.lowerURLSearchParamsNew(query)
	paramsOffsets, _, _ := g.objectLayout(g.semaResult.URLSearchParamsType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: res, Field: "$searchParams", Offset: offsets["$searchParams"], Val: params},
		&ir.SetFieldInst{Obj: params, Field: "$url", Offset: paramsOffsets["$url"], Val: res},
	)
	return res
}

func (g *generator) lowerURLField(url ir.Operand, field string) ir.Operand {
	t := g.semaResult.URLType
	offsets, _, _ := g.objectLayout(t)
	res := g.currentFn.NewValue("url_field", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
		Res: res, Obj: url, Field: field, Offset: offsets[field],
	})
	return res
}

func (g *generator) lowerURLParseRecord(input ir.Operand, invalid *ir.BasicBlock) ir.Operand {
	total := g.urlStringLen(input)
	start := g.currentBB
	schemeBB := g.currentFn.NewBlock("url_scheme")
	colon := g.urlStringFindByte(input, ir.ConstNumber{Value: ':'}, ir.ConstNumber{Value: 0}, total)
	hasScheme := g.currentFn.NewValue("url_has_scheme", types.TypeBoolean)
	start.Instructions = append(start.Instructions, &ir.BinaryInst{Res: hasScheme, Op: ir.OpGt, LHS: colon, RHS: ir.ConstNumber{Value: 0}})
	start.Terminator = &ir.BranchTerm{Cond: hasScheme, Then: schemeBB, Else: invalid}

	g.currentBB = schemeBB
	rawScheme := g.urlStringSlice(input, ir.ConstNumber{Value: 0}, colon)
	schemeLen := g.urlStringLen(rawScheme)
	len4 := g.currentFn.NewValue("url_scheme_len4", types.TypeBoolean)
	len5 := g.currentFn.NewValue("url_scheme_len5", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: len4, Op: ir.OpEq, LHS: schemeLen, RHS: ir.ConstNumber{Value: 4}},
		&ir.BinaryInst{Res: len5, Op: ir.OpEq, LHS: schemeLen, RHS: ir.ConstNumber{Value: 5}},
	)
	matchHTTP := g.currentFn.NewValue("url_http_match", types.TypeBoolean)
	matchHTTPS := g.currentFn.NewValue("url_https_match", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.CallInst{Res: matchHTTP, Callee: "ts_encoding_label_eq", Args: []ir.Operand{rawScheme, ir.ConstString{Value: "http"}}, ParamTypes: []types.Type{types.TypeString, types.TypeString}},
		&ir.CallInst{Res: matchHTTPS, Callee: "ts_encoding_label_eq", Args: []ir.Operand{rawScheme, ir.ConstString{Value: "https"}}, ParamTypes: []types.Type{types.TypeString, types.TypeString}},
	)

	httpOK := g.currentFn.NewValue("url_http_ok", types.TypeBoolean)
	httpsOK := g.currentFn.NewValue("url_https_ok", types.TypeBoolean)
	validScheme := g.currentFn.NewValue("url_scheme_valid", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: httpOK, Op: ir.OpAnd, LHS: len4, RHS: matchHTTP},
		&ir.BinaryInst{Res: httpsOK, Op: ir.OpAnd, LHS: len5, RHS: matchHTTPS},
		&ir.BinaryInst{Res: validScheme, Op: ir.OpOr, LHS: httpOK, RHS: httpsOK},
	)
	httpBB := g.currentFn.NewBlock("url_http")
	httpsBB := g.currentFn.NewBlock("url_https")
	schemeJoin := g.currentFn.NewBlock("url_scheme_join")
	schemeKind := g.currentFn.NewBlock("url_scheme_kind")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: validScheme, Then: schemeKind, Else: invalid}
	schemeKind.Terminator = &ir.BranchTerm{Cond: httpOK, Then: httpBB, Else: httpsBB}
	httpBB.Terminator = &ir.JumpTerm{Target: schemeJoin}
	httpsBB.Terminator = &ir.JumpTerm{Target: schemeJoin}
	canonicalScheme := g.currentFn.NewValue("url_scheme_canonical", types.TypeString)
	schemeJoin.Phis = append(schemeJoin.Phis, &ir.PhiInst{Res: canonicalScheme, Incoming: []ir.PhiIncoming{
		{Block: httpBB, Value: ir.ConstString{Value: "http"}},
		{Block: httpsBB, Value: ir.ConstString{Value: "https"}},
	}})

	g.currentBB = schemeJoin
	slash1Index := g.currentFn.NewValue("url_slash1_index", types.TypeNumber)
	slash2Index := g.currentFn.NewValue("url_slash2_index", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: slash1Index, Op: ir.OpAdd, LHS: colon, RHS: ir.ConstNumber{Value: 1}},
		&ir.BinaryInst{Res: slash2Index, Op: ir.OpAdd, LHS: colon, RHS: ir.ConstNumber{Value: 2}},
	)
	slash1 := g.urlStringByteAt(input, slash1Index)
	slash2 := g.urlStringByteAt(input, slash2Index)
	isSlash1 := g.currentFn.NewValue("url_slash1", types.TypeBoolean)
	isSlash2 := g.currentFn.NewValue("url_slash2", types.TypeBoolean)
	bothSlashes := g.currentFn.NewValue("url_slashes", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: isSlash1, Op: ir.OpEq, LHS: slash1, RHS: ir.ConstNumber{Value: '/'}},
		&ir.BinaryInst{Res: isSlash2, Op: ir.OpEq, LHS: slash2, RHS: ir.ConstNumber{Value: '/'}},
		&ir.BinaryInst{Res: bothSlashes, Op: ir.OpAnd, LHS: isSlash1, RHS: isSlash2},
	)
	authorityBB := g.currentFn.NewBlock("url_authority")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: bothSlashes, Then: authorityBB, Else: invalid}

	g.currentBB = authorityBB
	authorityStart := g.currentFn.NewValue("url_authority_start", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: authorityStart, Op: ir.OpAdd, LHS: colon, RHS: ir.ConstNumber{Value: 3}})
	authorityEnd := g.urlStringFindFirstDelimiter(input, authorityStart)
	hasAuthority := g.currentFn.NewValue("url_has_authority", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasAuthority, Op: ir.OpGt, LHS: authorityEnd, RHS: authorityStart})
	hostBB := g.currentFn.NewBlock("url_host")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasAuthority, Then: hostBB, Else: invalid}

	g.currentBB = hostBB
	// Bracketed IPv6 literals may contain many colons; only a colon after the
	// closing bracket separates the port. Ordinary hosts keep the first-colon path.
	firstHostByte := g.urlStringByteAt(input, authorityStart)
	isBracketedIPv6 := g.currentFn.NewValue("url_host_bracketed_ipv6", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: isBracketedIPv6, Op: ir.OpEq, LHS: firstHostByte, RHS: ir.ConstNumber{Value: '['}})
	regularPortBB := g.currentFn.NewBlock("url_regular_port_scan")
	ipv6PortBB := g.currentFn.NewBlock("url_ipv6_port_scan")
	portJoinBB := g.currentFn.NewBlock("url_port_scan_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isBracketedIPv6, Then: ipv6PortBB, Else: regularPortBB}

	g.currentBB = regularPortBB
	regularPortColon := g.urlStringFindByte(input, ir.ConstNumber{Value: ':'}, authorityStart, authorityEnd)
	regularPortEnd := g.currentBB
	regularPortEnd.Terminator = &ir.JumpTerm{Target: portJoinBB}

	g.currentBB = ipv6PortBB
	closeBracket := g.urlStringFindByte(input, ir.ConstNumber{Value: ']'}, authorityStart, authorityEnd)
	closeFound := g.currentFn.NewValue("url_ipv6_close_found", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: closeFound, Op: ir.OpGe, LHS: closeBracket, RHS: ir.ConstNumber{Value: 0}})
	ipv6CloseOK := g.currentFn.NewBlock("url_ipv6_close_ok")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: closeFound, Then: ipv6CloseOK, Else: invalid}

	g.currentBB = ipv6CloseOK
	afterBracket := g.currentFn.NewValue("url_ipv6_after_bracket", types.TypeNumber)
	hasIPv6Tail := g.currentFn.NewValue("url_ipv6_has_tail", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: afterBracket, Op: ir.OpAdd, LHS: closeBracket, RHS: ir.ConstNumber{Value: 1}},
		&ir.BinaryInst{Res: hasIPv6Tail, Op: ir.OpLt, LHS: afterBracket, RHS: authorityEnd},
	)
	ipv6TailBB := g.currentFn.NewBlock("url_ipv6_tail")
	ipv6NoPortBB := g.currentFn.NewBlock("url_ipv6_no_port")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasIPv6Tail, Then: ipv6TailBB, Else: ipv6NoPortBB}

	g.currentBB = ipv6TailBB
	tailByte := g.urlStringByteAt(input, afterBracket)
	tailIsColon := g.currentFn.NewValue("url_ipv6_tail_colon", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: tailIsColon, Op: ir.OpEq, LHS: tailByte, RHS: ir.ConstNumber{Value: ':'}})
	ipv6WithPortBB := g.currentFn.NewBlock("url_ipv6_with_port")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: tailIsColon, Then: ipv6WithPortBB, Else: invalid}
	ipv6WithPortBB.Terminator = &ir.JumpTerm{Target: portJoinBB}
	ipv6NoPortBB.Terminator = &ir.JumpTerm{Target: portJoinBB}

	g.currentBB = portJoinBB
	portColon := g.currentFn.NewValue("url_port_colon", types.TypeNumber)
	portJoinBB.Phis = append(portJoinBB.Phis, &ir.PhiInst{Res: portColon, Incoming: []ir.PhiIncoming{
		{Block: regularPortEnd, Value: regularPortColon},
		{Block: ipv6WithPortBB, Value: afterBracket},
		{Block: ipv6NoPortBB, Value: ir.ConstNumber{Value: -1}},
	}})
	hasPort := g.currentFn.NewValue("url_has_port", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasPort, Op: ir.OpGe, LHS: portColon, RHS: ir.ConstNumber{Value: 0}})
	withPortBB := g.currentFn.NewBlock("url_with_port")
	noPortBB := g.currentFn.NewBlock("url_no_port")
	hostJoin := g.currentFn.NewBlock("url_host_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasPort, Then: withPortBB, Else: noPortBB}

	g.currentBB = withPortBB
	hostWithPort := g.urlStringSlice(input, authorityStart, portColon)
	portStart := g.currentFn.NewValue("url_port_start", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: portStart, Op: ir.OpAdd, LHS: portColon, RHS: ir.ConstNumber{Value: 1}})
	portValue := g.urlStringSlice(input, portStart, authorityEnd)
	withPortEnd := g.currentBB
	withPortEnd.Terminator = &ir.JumpTerm{Target: hostJoin}

	g.currentBB = noPortBB
	hostNoPort := g.urlStringSlice(input, authorityStart, authorityEnd)
	noPortEnd := g.currentBB
	noPortEnd.Terminator = &ir.JumpTerm{Target: hostJoin}
	hostname := g.currentFn.NewValue("url_hostname", types.TypeString)
	port := g.currentFn.NewValue("url_port", types.TypeString)
	hostJoin.Phis = append(hostJoin.Phis,
		&ir.PhiInst{Res: hostname, Incoming: []ir.PhiIncoming{{Block: withPortEnd, Value: hostWithPort}, {Block: noPortEnd, Value: hostNoPort}}},
		&ir.PhiInst{Res: port, Incoming: []ir.PhiIncoming{{Block: withPortEnd, Value: portValue}, {Block: noPortEnd, Value: ir.ConstString{Value: ""}}}},
	)

	g.currentBB = hostJoin
	canonicalHostname := g.currentFn.NewValue("url_hostname_canonical", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: canonicalHostname, Callee: "ts_string_ascii_lower", Args: []ir.Operand{hostname}, ParamTypes: []types.Type{types.TypeString}})
	normalizedPort := g.lowerURLNormalizePort(canonicalScheme, port)
	hostLen := g.urlStringLen(canonicalHostname)
	hostNonEmpty := g.currentFn.NewValue("url_hostname_nonempty", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hostNonEmpty, Op: ir.OpGt, LHS: hostLen, RHS: ir.ConstNumber{Value: 0}})
	componentsBB := g.currentFn.NewBlock("url_components")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hostNonEmpty, Then: componentsBB, Else: invalid}

	g.currentBB = componentsBB
	queryAt := g.urlStringFindByte(input, ir.ConstNumber{Value: '?'}, authorityEnd, total)
	hashAt := g.urlStringFindByte(input, ir.ConstNumber{Value: '#'}, authorityEnd, total)
	queryFound := g.currentFn.NewValue("url_query_found", types.TypeBoolean)
	hashFound := g.currentFn.NewValue("url_hash_found", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: queryFound, Op: ir.OpGe, LHS: queryAt, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: hashFound, Op: ir.OpGe, LHS: hashAt, RHS: ir.ConstNumber{Value: 0}},
	)
	queryBeforeHash := g.currentFn.NewValue("url_query_before_hash", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: queryBeforeHash, Op: ir.OpLt, LHS: queryAt, RHS: hashAt})
	noHash := g.currentFn.NewValue("url_no_hash", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: noHash, Op: ir.OpEq, LHS: hashFound, RHS: ir.ConstBool{Value: false}})
	queryPositionOK := g.currentFn.NewValue("url_query_position_ok", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: queryPositionOK, Op: ir.OpOr, LHS: noHash, RHS: queryBeforeHash})
	queryValid := g.currentFn.NewValue("url_query_valid", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: queryValid, Op: ir.OpAnd, LHS: queryFound, RHS: queryPositionOK})

	pathQEndBB := g.currentFn.NewBlock("url_path_query_end")
	pathNoQBB := g.currentFn.NewBlock("url_path_no_query")
	pathHashEndBB := g.currentFn.NewBlock("url_path_hash_end")
	pathTotalEndBB := g.currentFn.NewBlock("url_path_total_end")
	pathEndJoin := g.currentFn.NewBlock("url_path_end_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: queryValid, Then: pathQEndBB, Else: pathNoQBB}
	pathQEndBB.Terminator = &ir.JumpTerm{Target: pathEndJoin}
	pathNoQBB.Terminator = &ir.BranchTerm{Cond: hashFound, Then: pathHashEndBB, Else: pathTotalEndBB}
	pathHashEndBB.Terminator = &ir.JumpTerm{Target: pathEndJoin}
	pathTotalEndBB.Terminator = &ir.JumpTerm{Target: pathEndJoin}
	pathEnd := g.currentFn.NewValue("url_path_end", types.TypeNumber)
	pathEndJoin.Phis = append(pathEndJoin.Phis, &ir.PhiInst{Res: pathEnd, Incoming: []ir.PhiIncoming{
		{Block: pathQEndBB, Value: queryAt},
		{Block: pathHashEndBB, Value: hashAt},
		{Block: pathTotalEndBB, Value: total},
	}})

	g.currentBB = pathEndJoin
	delim := g.urlStringByteAt(input, authorityEnd)
	hasPath := g.currentFn.NewValue("url_has_path", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasPath, Op: ir.OpEq, LHS: delim, RHS: ir.ConstNumber{Value: '/'}})
	pathSliceBB := g.currentFn.NewBlock("url_path_slice")
	pathDefaultBB := g.currentFn.NewBlock("url_path_default")
	pathJoin := g.currentFn.NewBlock("url_path_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasPath, Then: pathSliceBB, Else: pathDefaultBB}
	g.currentBB = pathSliceBB
	pathValue := g.urlStringSlice(input, authorityEnd, pathEnd)
	pathSliceEnd := g.currentBB
	pathSliceEnd.Terminator = &ir.JumpTerm{Target: pathJoin}
	pathDefaultBB.Terminator = &ir.JumpTerm{Target: pathJoin}
	pathname := g.currentFn.NewValue("url_pathname", types.TypeString)
	pathJoin.Phis = append(pathJoin.Phis, &ir.PhiInst{Res: pathname, Incoming: []ir.PhiIncoming{
		{Block: pathSliceEnd, Value: pathValue},
		{Block: pathDefaultBB, Value: ir.ConstString{Value: "/"}},
	}})

	g.currentBB = pathJoin
	queryBB := g.currentFn.NewBlock("url_query_value")
	queryEmptyBB := g.currentFn.NewBlock("url_query_empty")
	queryJoin := g.currentFn.NewBlock("url_query_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: queryValid, Then: queryBB, Else: queryEmptyBB}

	g.currentBB = queryBB
	queryStart := g.currentFn.NewValue("url_query_start", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: queryStart, Op: ir.OpAdd, LHS: queryAt, RHS: ir.ConstNumber{Value: 1}})
	queryHashBB := g.currentFn.NewBlock("url_query_hash_end")
	queryTotalBB := g.currentFn.NewBlock("url_query_total_end")
	queryEndJoin := g.currentFn.NewBlock("url_query_end_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hashFound, Then: queryHashBB, Else: queryTotalBB}
	queryHashBB.Terminator = &ir.JumpTerm{Target: queryEndJoin}
	queryTotalBB.Terminator = &ir.JumpTerm{Target: queryEndJoin}
	queryEnd := g.currentFn.NewValue("url_query_end", types.TypeNumber)
	queryEndJoin.Phis = append(queryEndJoin.Phis, &ir.PhiInst{Res: queryEnd, Incoming: []ir.PhiIncoming{
		{Block: queryHashBB, Value: hashAt},
		{Block: queryTotalBB, Value: total},
	}})
	g.currentBB = queryEndJoin
	queryValue := g.urlStringSlice(input, queryStart, queryEnd)
	queryValueEnd := g.currentBB
	queryValueEnd.Terminator = &ir.JumpTerm{Target: queryJoin}
	queryEmptyBB.Terminator = &ir.JumpTerm{Target: queryJoin}
	query := g.currentFn.NewValue("url_query", types.TypeString)
	queryJoin.Phis = append(queryJoin.Phis, &ir.PhiInst{Res: query, Incoming: []ir.PhiIncoming{
		{Block: queryValueEnd, Value: queryValue},
		{Block: queryEmptyBB, Value: ir.ConstString{Value: ""}},
	}})

	g.currentBB = queryJoin
	fragmentBB := g.currentFn.NewBlock("url_fragment_value")
	fragmentEmptyBB := g.currentFn.NewBlock("url_fragment_empty")
	fragmentJoin := g.currentFn.NewBlock("url_fragment_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hashFound, Then: fragmentBB, Else: fragmentEmptyBB}
	g.currentBB = fragmentBB
	fragmentStart := g.currentFn.NewValue("url_fragment_start", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: fragmentStart, Op: ir.OpAdd, LHS: hashAt, RHS: ir.ConstNumber{Value: 1}})
	fragmentValue := g.urlStringSlice(input, fragmentStart, total)
	fragmentValueEnd := g.currentBB
	fragmentValueEnd.Terminator = &ir.JumpTerm{Target: fragmentJoin}
	fragmentEmptyBB.Terminator = &ir.JumpTerm{Target: fragmentJoin}
	fragment := g.currentFn.NewValue("url_fragment", types.TypeString)
	fragmentJoin.Phis = append(fragmentJoin.Phis, &ir.PhiInst{Res: fragment, Incoming: []ir.PhiIncoming{
		{Block: fragmentValueEnd, Value: fragmentValue},
		{Block: fragmentEmptyBB, Value: ir.ConstString{Value: ""}},
	}})

	g.currentBB = fragmentJoin
	normalizedPathname := g.lowerURLNormalizePath(pathname)
	result := g.lowerURLAllocRecord(canonicalScheme, canonicalHostname, normalizedPort, normalizedPathname, query, fragment)
	return result
}

func (g *generator) lowerURLNew(input ir.Operand) ir.Operand {
	invalid := g.currentFn.NewBlock("url_invalid")
	result := g.lowerURLParseRecord(input, invalid)
	resultBB := g.currentBB

	g.currentBB = invalid
	err := g.newWebError(ir.ConstString{Value: "Invalid URL"}, ir.ConstString{Value: "TypeError"})
	g.routeThrownValue(err)
	g.currentBB = resultBB
	return result
}

func (g *generator) lowerURLHost(url ir.Operand) ir.Operand {
	hostname := g.lowerURLField(url, "$hostname")
	port := g.lowerURLField(url, "$port")
	isEmpty := g.currentFn.NewValue("url_port_empty", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: isEmpty, Callee: "ts_string_eq", Args: []ir.Operand{port, ir.ConstString{Value: ""}},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	emptyBB := g.currentFn.NewBlock("url_host_no_port")
	portBB := g.currentFn.NewBlock("url_host_with_port")
	joinBB := g.currentFn.NewBlock("url_host_join_value")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isEmpty, Then: emptyBB, Else: portBB}
	emptyBB.Terminator = &ir.JumpTerm{Target: joinBB}
	g.currentBB = portBB
	withPort := g.concatNativeStrings(g.concatNativeStrings(hostname, ir.ConstString{Value: ":"}), port)
	portEnd := g.currentBB
	portEnd.Terminator = &ir.JumpTerm{Target: joinBB}
	result := g.currentFn.NewValue("url_host_value", types.TypeString)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: emptyBB, Value: hostname}, {Block: portEnd, Value: withPort},
	}})
	g.currentBB = joinBB
	return result
}

func (g *generator) lowerURLPrefixedValue(value ir.Operand, prefix string) ir.Operand {
	isEmpty := g.currentFn.NewValue("url_component_empty", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: isEmpty, Callee: "ts_string_eq", Args: []ir.Operand{value, ir.ConstString{Value: ""}},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	emptyBB := g.currentFn.NewBlock("url_component_empty_value")
	valueBB := g.currentFn.NewBlock("url_component_prefixed_value")
	joinBB := g.currentFn.NewBlock("url_component_prefix_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isEmpty, Then: emptyBB, Else: valueBB}
	emptyBB.Terminator = &ir.JumpTerm{Target: joinBB}
	g.currentBB = valueBB
	prefixed := g.concatNativeStrings(ir.ConstString{Value: prefix}, value)
	valueEnd := g.currentBB
	valueEnd.Terminator = &ir.JumpTerm{Target: joinBB}
	result := g.currentFn.NewValue("url_prefixed_value", types.TypeString)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: emptyBB, Value: ir.ConstString{Value: ""}}, {Block: valueEnd, Value: prefixed},
	}})
	g.currentBB = joinBB
	return result
}

func (g *generator) lowerURLOrigin(url ir.Operand) ir.Operand {
	scheme := g.lowerURLField(url, "$scheme")
	host := g.lowerURLHost(url)
	return g.concatNativeStrings(g.concatNativeStrings(scheme, ir.ConstString{Value: "://"}), host)
}

func (g *generator) lowerURLHref(url ir.Operand) ir.Operand {
	origin := g.lowerURLOrigin(url)
	pathname := g.lowerURLField(url, "$pathname")
	query := g.lowerURLField(url, "$query")
	fragment := g.lowerURLField(url, "$fragment")
	search := g.lowerURLPrefixedValue(query, "?")
	hash := g.lowerURLPrefixedValue(fragment, "#")
	return g.concatNativeStrings(g.concatNativeStrings(g.concatNativeStrings(origin, pathname), search), hash)
}

func (g *generator) lowerURLMember(url ir.Operand, property string) (ir.Operand, bool) {
	switch property {
	case "href":
		return g.lowerURLHref(url), true
	case "origin":
		return g.lowerURLOrigin(url), true
	case "protocol":
		return g.concatNativeStrings(g.lowerURLField(url, "$scheme"), ir.ConstString{Value: ":"}), true
	case "host":
		return g.lowerURLHost(url), true
	case "hostname":
		return g.lowerURLField(url, "$hostname"), true
	case "port":
		return g.lowerURLField(url, "$port"), true
	case "pathname":
		return g.lowerURLField(url, "$pathname"), true
	case "search":
		return g.lowerURLPrefixedValue(g.lowerURLField(url, "$query"), "?"), true
	case "hash":
		return g.lowerURLPrefixedValue(g.lowerURLField(url, "$fragment"), "#"), true
	case "searchParams":
		offsets, _, _ := g.objectLayout(g.semaResult.URLType)
		params := g.currentFn.NewValue("url_search_params", g.semaResult.URLSearchParamsType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
			Res: params, Obj: url, Field: "$searchParams", Offset: offsets["$searchParams"],
		})
		return params, true
	}
	return nil, false
}

func (g *generator) lowerURLMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$URL" {
		return nil, false
	}
	if mem.Property != "toString" && mem.Property != "toJSON" {
		return nil, false
	}
	return g.lowerURLHref(g.lowerExpr(mem.Object)), true
}

func (g *generator) lowerURLNormalizePort(scheme, port ir.Operand) ir.Operand {
	isHTTP := g.currentFn.NewValue("url_port_http", types.TypeBoolean)
	isHTTPS := g.currentFn.NewValue("url_port_https", types.TypeBoolean)
	is80 := g.currentFn.NewValue("url_port_80", types.TypeBoolean)
	is443 := g.currentFn.NewValue("url_port_443", types.TypeBoolean)
	for _, spec := range []struct {
		res *ir.Value
		lhs ir.Operand
		rhs string
	}{
		{isHTTP, scheme, "http"}, {isHTTPS, scheme, "https"}, {is80, port, "80"}, {is443, port, "443"},
	} {
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: spec.res, Callee: "ts_string_eq", Args: []ir.Operand{spec.lhs, ir.ConstString{Value: spec.rhs}},
			ParamTypes: []types.Type{types.TypeString, types.TypeString},
		})
	}
	httpDefault := g.currentFn.NewValue("url_http_default_port", types.TypeBoolean)
	httpsDefault := g.currentFn.NewValue("url_https_default_port", types.TypeBoolean)
	isDefault := g.currentFn.NewValue("url_default_port", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: httpDefault, Op: ir.OpAnd, LHS: isHTTP, RHS: is80},
		&ir.BinaryInst{Res: httpsDefault, Op: ir.OpAnd, LHS: isHTTPS, RHS: is443},
		&ir.BinaryInst{Res: isDefault, Op: ir.OpOr, LHS: httpDefault, RHS: httpsDefault},
	)

	emptyBB := g.currentFn.NewBlock("url_default_port_empty")
	keepBB := g.currentFn.NewBlock("url_default_port_keep")
	joinBB := g.currentFn.NewBlock("url_default_port_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isDefault, Then: emptyBB, Else: keepBB}
	emptyBB.Terminator = &ir.JumpTerm{Target: joinBB}
	keepBB.Terminator = &ir.JumpTerm{Target: joinBB}
	result := g.currentFn.NewValue("url_normalized_port", types.TypeString)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: emptyBB, Value: ir.ConstString{Value: ""}},
		{Block: keepBB, Value: port},
	}})
	g.currentBB = joinBB
	return result
}

func (g *generator) urlStringFindLastByte(value ir.Operand, ch byte) ir.Operand {
	res := g.currentFn.NewValue("url_string_find_last", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_string_find_last_byte",
		Args:       []ir.Operand{value, ir.ConstNumber{Value: float64(ch)}},
		ParamTypes: []types.Type{types.TypeString, types.TypeNumber},
	})
	return res
}

func (g *generator) urlStringEqual(lhs ir.Operand, rhs string) ir.Operand {
	res := g.currentFn.NewValue("url_string_equal", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_string_eq",
		Args:       []ir.Operand{lhs, ir.ConstString{Value: rhs}},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	return res
}

func (g *generator) lowerURLNormalizePath(path ir.Operand) ir.Operand {
	segmentsType := types.NewArray(types.TypeString)
	segments := g.currentFn.NewValue("url_path_segments", segmentsType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: segments, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0},
	})
	total := g.urlStringLen(path)
	entry := g.currentBB
	condBB := g.currentFn.NewBlock("url_path_norm_cond")
	scanBB := g.currentFn.NewBlock("url_path_norm_scan")
	slashBB := g.currentFn.NewBlock("url_path_norm_slash")
	lastBB := g.currentFn.NewBlock("url_path_norm_last")
	segmentBB := g.currentFn.NewBlock("url_path_norm_segment")
	dotBB := g.currentFn.NewBlock("url_path_norm_dot")
	dotDotBB := g.currentFn.NewBlock("url_path_norm_dotdot")
	normalBB := g.currentFn.NewBlock("url_path_norm_normal")
	advanceBB := g.currentFn.NewBlock("url_path_norm_advance")
	serializeBB := g.currentFn.NewBlock("url_path_norm_serialize")
	entry.Terminator = &ir.JumpTerm{Target: condBB}

	offset := g.currentFn.NewValue("url_path_norm_offset", types.TypeNumber)
	nextOffset := g.currentFn.NewValue("url_path_norm_next_offset", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: offset, Incoming: []ir.PhiIncoming{
		{Block: entry, Value: ir.ConstNumber{Value: 1}},
		{Block: advanceBB, Value: nextOffset},
	}})
	more := g.currentFn.NewValue("url_path_norm_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{
		Res: more, Op: ir.OpLe, LHS: offset, RHS: total,
	})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: scanBB, Else: serializeBB}
	g.currentBB = scanBB
	slash := g.urlStringFindByte(path, ir.ConstNumber{Value: '/'}, offset, total)
	hasSlash := g.currentFn.NewValue("url_path_norm_has_slash", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: hasSlash, Op: ir.OpGe, LHS: slash, RHS: ir.ConstNumber{Value: 0},
	})
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasSlash, Then: slashBB, Else: lastBB}
	slashBB.Terminator = &ir.JumpTerm{Target: segmentBB}
	lastBB.Terminator = &ir.JumpTerm{Target: segmentBB}
	segmentEnd := g.currentFn.NewValue("url_path_norm_segment_end", types.TypeNumber)
	segmentBB.Phis = append(segmentBB.Phis, &ir.PhiInst{Res: segmentEnd, Incoming: []ir.PhiIncoming{
		{Block: slashBB, Value: slash}, {Block: lastBB, Value: total},
	}})

	g.currentBB = segmentBB
	segment := g.urlStringSlice(path, offset, segmentEnd)
	isDot := g.urlStringEqual(segment, ".")
	isDotDot := g.urlStringEqual(segment, "..")
	dotOrDotDot := g.currentFn.NewValue("url_path_norm_dot_kind", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: dotOrDotDot, Op: ir.OpOr, LHS: isDot, RHS: isDotDot,
	})
	dotKindBB := g.currentFn.NewBlock("url_path_norm_dot_kind")
	segmentBB.Terminator = &ir.BranchTerm{Cond: dotOrDotDot, Then: dotKindBB, Else: normalBB}
	dotKindBB.Terminator = &ir.BranchTerm{Cond: isDotDot, Then: dotDotBB, Else: dotBB}
	lastSegment := g.currentFn.NewValue("url_path_norm_last_segment", types.TypeBoolean)
	dotBB.Instructions = append(dotBB.Instructions, &ir.BinaryInst{
		Res: lastSegment, Op: ir.OpEq, LHS: segmentEnd, RHS: total,
	})
	dotTrailingBB := g.currentFn.NewBlock("url_path_norm_dot_trailing")
	dotBB.Terminator = &ir.BranchTerm{Cond: lastSegment, Then: dotTrailingBB, Else: advanceBB}
	g.currentBB = dotTrailingBB
	g.pushArrayOperand(segments, ir.ConstString{Value: ""})
	dotTrailingEnd := g.currentBB
	dotTrailingEnd.Terminator = &ir.JumpTerm{Target: advanceBB}

	segmentCount := g.currentFn.NewValue("url_path_norm_segment_count", types.TypeNumber)
	dotDotBB.Instructions = append(dotDotBB.Instructions, &ir.ArrayLengthInst{Res: segmentCount, Array: segments})
	hasPrevious := g.currentFn.NewValue("url_path_norm_has_previous", types.TypeBoolean)
	dotDotBB.Instructions = append(dotDotBB.Instructions, &ir.BinaryInst{
		Res: hasPrevious, Op: ir.OpGt, LHS: segmentCount, RHS: ir.ConstNumber{Value: 0},
	})
	popBB := g.currentFn.NewBlock("url_path_norm_pop")
	afterPopBB := g.currentFn.NewBlock("url_path_norm_after_pop")
	dotDotBB.Terminator = &ir.BranchTerm{Cond: hasPrevious, Then: popBB, Else: afterPopBB}
	popped := g.currentFn.NewValue("url_path_norm_popped", types.TypeString)
	popBB.Instructions = append(popBB.Instructions, &ir.ArrayPopInst{Res: popped, Array: segments})
	popBB.Terminator = &ir.JumpTerm{Target: afterPopBB}
	lastDotDot := g.currentFn.NewValue("url_path_norm_last_dotdot", types.TypeBoolean)
	afterPopBB.Instructions = append(afterPopBB.Instructions, &ir.BinaryInst{
		Res: lastDotDot, Op: ir.OpEq, LHS: segmentEnd, RHS: total,
	})
	dotDotTrailingBB := g.currentFn.NewBlock("url_path_norm_dotdot_trailing")
	afterPopBB.Terminator = &ir.BranchTerm{Cond: lastDotDot, Then: dotDotTrailingBB, Else: advanceBB}
	g.currentBB = dotDotTrailingBB
	g.pushArrayOperand(segments, ir.ConstString{Value: ""})
	dotDotTrailingEnd := g.currentBB
	dotDotTrailingEnd.Terminator = &ir.JumpTerm{Target: advanceBB}

	g.currentBB = normalBB
	g.pushArrayOperand(segments, segment)
	normalEnd := g.currentBB
	normalEnd.Terminator = &ir.JumpTerm{Target: advanceBB}

	advanceBB.Instructions = append(advanceBB.Instructions, &ir.BinaryInst{
		Res: nextOffset, Op: ir.OpAdd, LHS: segmentEnd, RHS: ir.ConstNumber{Value: 1},
	})
	advanceBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = serializeBB
	return g.lowerURLSerializePathSegments(segments)
}

func (g *generator) lowerURLSerializePathSegments(segments ir.Operand) ir.Operand {
	entry := g.currentBB
	length := g.currentFn.NewValue("url_path_segments_len", types.TypeNumber)
	entry.Instructions = append(entry.Instructions, &ir.ArrayLengthInst{Res: length, Array: segments})
	condBB := g.currentFn.NewBlock("url_path_serialize_cond")
	bodyBB := g.currentFn.NewBlock("url_path_serialize_body")
	firstBB := g.currentFn.NewBlock("url_path_serialize_first")
	restBB := g.currentFn.NewBlock("url_path_serialize_rest")
	nextBB := g.currentFn.NewBlock("url_path_serialize_next")
	doneBB := g.currentFn.NewBlock("url_path_serialize_done")
	entry.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("url_path_serialize_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("url_path_serialize_next_i", types.TypeNumber)
	acc := g.currentFn.NewValue("url_path_serialize_acc", types.TypeString)
	nextAcc := g.currentFn.NewValue("url_path_serialize_next_acc", types.TypeString)
	condBB.Phis = append(condBB.Phis,
		&ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}}},
		&ir.PhiInst{Res: acc, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstString{Value: "/"}}}},
	)
	more := g.currentFn.NewValue("url_path_serialize_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}
	g.currentBB = bodyBB
	segment := g.currentFn.NewValue("url_path_serialize_segment", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: segment, Array: segments, Index: index})
	isFirst := g.currentFn.NewValue("url_path_serialize_is_first", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{
		Res: isFirst, Op: ir.OpEq, LHS: index, RHS: ir.ConstNumber{Value: 0},
	})
	bodyBB.Terminator = &ir.BranchTerm{Cond: isFirst, Then: firstBB, Else: restBB}

	g.currentBB = firstBB
	firstAcc := g.concatNativeStrings(acc, segment)
	firstEnd := g.currentBB
	firstEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = restBB
	withSlash := g.concatNativeStrings(ir.ConstString{Value: "/"}, segment)
	restAcc := g.concatNativeStrings(acc, withSlash)
	restEnd := g.currentBB
	restEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	nextBB.Phis = append(nextBB.Phis, &ir.PhiInst{Res: nextAcc, Incoming: []ir.PhiIncoming{
		{Block: firstEnd, Value: firstAcc}, {Block: restEnd, Value: restAcc},
	}})
	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{
		Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1},
	})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}
	condBB.Phis[0].Incoming = append(condBB.Phis[0].Incoming, ir.PhiIncoming{Block: nextBB, Value: nextIndex})
	condBB.Phis[1].Incoming = append(condBB.Phis[1].Incoming, ir.PhiIncoming{Block: nextBB, Value: nextAcc})
	g.currentBB = doneBB
	return acc
}

func (g *generator) lowerURLResolveInput(input, base ir.Operand) ir.Operand {
	total := g.urlStringLen(input)
	colon := g.urlStringFindByte(input, ir.ConstNumber{Value: ':'}, ir.ConstNumber{Value: 0}, total)
	firstDelimiter := g.urlStringFindFirstDelimiter(input, ir.ConstNumber{Value: 0})
	colonPositive := g.currentFn.NewValue("url_resolve_colon_positive", types.TypeBoolean)
	colonBeforeDelimiter := g.currentFn.NewValue("url_resolve_colon_before_delimiter", types.TypeBoolean)
	hasScheme := g.currentFn.NewValue("url_resolve_has_scheme", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: colonPositive, Op: ir.OpGt, LHS: colon, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: colonBeforeDelimiter, Op: ir.OpLt, LHS: colon, RHS: firstDelimiter},
		&ir.BinaryInst{Res: hasScheme, Op: ir.OpAnd, LHS: colonPositive, RHS: colonBeforeDelimiter},
	)
	entry := g.currentBB
	absoluteBB := g.currentFn.NewBlock("url_resolve_absolute")
	relativeBB := g.currentFn.NewBlock("url_resolve_relative")
	joinBB := g.currentFn.NewBlock("url_resolve_join")
	entry.Terminator = &ir.BranchTerm{Cond: hasScheme, Then: absoluteBB, Else: relativeBB}
	absoluteBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = relativeBB
	origin := g.lowerURLOrigin(base)
	basePath := g.lowerURLField(base, "$pathname")
	baseQuery := g.lowerURLField(base, "$query")
	baseSearch := g.lowerURLPrefixedValue(baseQuery, "?")
	baseScheme := g.lowerURLField(base, "$scheme")
	first := g.urlStringByteAt(input, ir.ConstNumber{Value: 0})
	second := g.urlStringByteAt(input, ir.ConstNumber{Value: 1})
	isSlash := g.currentFn.NewValue("url_resolve_slash", types.TypeBoolean)
	isSecondSlash := g.currentFn.NewValue("url_resolve_second_slash", types.TypeBoolean)
	isDoubleSlash := g.currentFn.NewValue("url_resolve_double_slash", types.TypeBoolean)
	isQuery := g.currentFn.NewValue("url_resolve_query", types.TypeBoolean)
	isHash := g.currentFn.NewValue("url_resolve_hash", types.TypeBoolean)
	isEmpty := g.currentFn.NewValue("url_resolve_empty", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: isSlash, Op: ir.OpEq, LHS: first, RHS: ir.ConstNumber{Value: '/'}},
		&ir.BinaryInst{Res: isSecondSlash, Op: ir.OpEq, LHS: second, RHS: ir.ConstNumber{Value: '/'}},
		&ir.BinaryInst{Res: isDoubleSlash, Op: ir.OpAnd, LHS: isSlash, RHS: isSecondSlash},
		&ir.BinaryInst{Res: isQuery, Op: ir.OpEq, LHS: first, RHS: ir.ConstNumber{Value: '?'}},
		&ir.BinaryInst{Res: isHash, Op: ir.OpEq, LHS: first, RHS: ir.ConstNumber{Value: '#'}},
		&ir.BinaryInst{Res: isEmpty, Op: ir.OpEq, LHS: total, RHS: ir.ConstNumber{Value: 0}},
	)
	doubleSlashBB := g.currentFn.NewBlock("url_resolve_scheme_relative")
	slashCheckBB := g.currentFn.NewBlock("url_resolve_slash_check")
	rootSlashBB := g.currentFn.NewBlock("url_resolve_root_relative")
	queryCheckBB := g.currentFn.NewBlock("url_resolve_query_check")
	queryBB := g.currentFn.NewBlock("url_resolve_query_relative")
	hashCheckBB := g.currentFn.NewBlock("url_resolve_hash_check")
	hashBB := g.currentFn.NewBlock("url_resolve_hash_relative")
	emptyCheckBB := g.currentFn.NewBlock("url_resolve_empty_check")
	emptyBB := g.currentFn.NewBlock("url_resolve_empty_relative")
	pathBB := g.currentFn.NewBlock("url_resolve_path_relative")
	relativeJoin := g.currentFn.NewBlock("url_resolve_relative_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isDoubleSlash, Then: doubleSlashBB, Else: slashCheckBB}
	slashCheckBB.Terminator = &ir.BranchTerm{Cond: isSlash, Then: rootSlashBB, Else: queryCheckBB}
	queryCheckBB.Terminator = &ir.BranchTerm{Cond: isQuery, Then: queryBB, Else: hashCheckBB}
	hashCheckBB.Terminator = &ir.BranchTerm{Cond: isHash, Then: hashBB, Else: emptyCheckBB}
	emptyCheckBB.Terminator = &ir.BranchTerm{Cond: isEmpty, Then: emptyBB, Else: pathBB}

	g.currentBB = doubleSlashBB
	schemePrefix := g.concatNativeStrings(baseScheme, ir.ConstString{Value: ":"})
	schemeRelative := g.concatNativeStrings(schemePrefix, input)
	doubleSlashEnd := g.currentBB
	doubleSlashEnd.Terminator = &ir.JumpTerm{Target: relativeJoin}

	g.currentBB = rootSlashBB
	rootRelative := g.concatNativeStrings(origin, input)
	rootSlashEnd := g.currentBB
	rootSlashEnd.Terminator = &ir.JumpTerm{Target: relativeJoin}

	g.currentBB = queryBB
	queryBase := g.concatNativeStrings(origin, basePath)
	queryRelative := g.concatNativeStrings(queryBase, input)
	queryEnd := g.currentBB
	queryEnd.Terminator = &ir.JumpTerm{Target: relativeJoin}

	g.currentBB = hashBB
	hashBase := g.concatNativeStrings(g.concatNativeStrings(origin, basePath), baseSearch)
	hashRelative := g.concatNativeStrings(hashBase, input)
	hashEnd := g.currentBB
	hashEnd.Terminator = &ir.JumpTerm{Target: relativeJoin}

	g.currentBB = emptyBB
	emptyRelative := g.concatNativeStrings(g.concatNativeStrings(origin, basePath), baseSearch)
	emptyEnd := g.currentBB
	emptyEnd.Terminator = &ir.JumpTerm{Target: relativeJoin}

	g.currentBB = pathBB
	lastSlash := g.urlStringFindLastByte(basePath, '/')
	directoryEnd := g.currentFn.NewValue("url_resolve_directory_end", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: directoryEnd, Op: ir.OpAdd, LHS: lastSlash, RHS: ir.ConstNumber{Value: 1},
	})
	directory := g.urlStringSlice(basePath, ir.ConstNumber{Value: 0}, directoryEnd)
	pathBase := g.concatNativeStrings(origin, directory)
	pathRelative := g.concatNativeStrings(pathBase, input)
	pathEnd := g.currentBB
	pathEnd.Terminator = &ir.JumpTerm{Target: relativeJoin}

	resolvedRelative := g.currentFn.NewValue("url_resolved_relative", types.TypeString)
	relativeJoin.Phis = append(relativeJoin.Phis, &ir.PhiInst{Res: resolvedRelative, Incoming: []ir.PhiIncoming{
		{Block: doubleSlashEnd, Value: schemeRelative},
		{Block: rootSlashEnd, Value: rootRelative},
		{Block: queryEnd, Value: queryRelative},
		{Block: hashEnd, Value: hashRelative},
		{Block: emptyEnd, Value: emptyRelative},
		{Block: pathEnd, Value: pathRelative},
	}})
	relativeJoin.Terminator = &ir.JumpTerm{Target: joinBB}

	resolved := g.currentFn.NewValue("url_resolved_input", types.TypeString)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: resolved, Incoming: []ir.PhiIncoming{
		{Block: absoluteBB, Value: input},
		{Block: relativeJoin, Value: resolvedRelative},
	}})
	g.currentBB = joinBB
	return resolved
}

func (g *generator) lowerURLStripLeadingByte(value ir.Operand, ch byte) ir.Operand {
	total := g.urlStringLen(value)
	first := g.urlStringByteAt(value, ir.ConstNumber{Value: 0})
	matches := g.currentFn.NewValue("url_strip_prefix_match", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: matches, Op: ir.OpEq, LHS: first, RHS: ir.ConstNumber{Value: float64(ch)},
	})
	stripBB := g.currentFn.NewBlock("url_strip_prefix_strip")
	keepBB := g.currentFn.NewBlock("url_strip_prefix_keep")
	joinBB := g.currentFn.NewBlock("url_strip_prefix_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: matches, Then: stripBB, Else: keepBB}
	g.currentBB = stripBB
	stripped := g.urlStringSlice(value, ir.ConstNumber{Value: 1}, total)
	stripEnd := g.currentBB
	stripEnd.Terminator = &ir.JumpTerm{Target: joinBB}
	keepBB.Terminator = &ir.JumpTerm{Target: joinBB}
	result := g.currentFn.NewValue("url_stripped_value", types.TypeString)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: stripEnd, Value: stripped}, {Block: keepBB, Value: value},
	}})
	g.currentBB = joinBB
	return result
}

func (g *generator) lowerURLSetSearch(url, value ir.Operand) ir.Operand {
	query := g.lowerURLStripLeadingByte(value, '?')
	offsets, _, _ := g.objectLayout(g.semaResult.URLType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: url, Field: "$query", Offset: offsets["$query"], Val: query,
	})
	params := g.currentFn.NewValue("url_search_params_for_set", g.semaResult.URLSearchParamsType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
		Res: params, Obj: url, Field: "$searchParams", Offset: offsets["$searchParams"],
	})
	g.lowerURLSearchParamsReplace(params, query)
	return value
}

func (g *generator) lowerURLResolveAndParse(inputExpr, baseExpr ast.Expr, onInvalid *ir.BasicBlock) ir.Operand {
	input := g.lowerExpr(inputExpr)
	if baseExpr == nil {
		return g.lowerURLParseRecord(input, onInvalid)
	}
	baseType := g.semanticType(baseExpr)
	if obj, ok := baseType.(*types.ObjectType); ok && obj.Name == "$URL" {
		baseVal := g.lowerExpr(baseExpr)
		resolved := g.lowerURLResolveInput(input, baseVal)
		return g.lowerURLParseRecord(resolved, onInvalid)
	}
	baseStr := g.lowerExpr(baseExpr)
	baseRecord := g.lowerURLParseRecord(baseStr, onInvalid)
	resolved := g.lowerURLResolveInput(input, baseRecord)
	return g.lowerURLParseRecord(resolved, onInvalid)
}

func (g *generator) lowerURLStaticCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	ident, ok := mem.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "URL" {
		return nil, false
	}
	switch mem.Property {
	case "canParse":
		return g.lowerURLCanParse(e), true
	case "parse":
		return g.lowerURLParse(e), true
	}
	return nil, false
}

func (g *generator) lowerURLCanParse(e *ast.CallExpr) ir.Operand {
	invalidBB := g.currentFn.NewBlock("url_can_parse_invalid")
	var baseExpr ast.Expr
	if len(e.Args) > 1 {
		baseExpr = e.Args[1]
	}
	_ = g.lowerURLResolveAndParse(e.Args[0], baseExpr, invalidBB)
	successBB := g.currentBB
	joinBB := g.currentFn.NewBlock("url_can_parse_join")
	successBB.Terminator = &ir.JumpTerm{Target: joinBB}
	invalidBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = joinBB
	res := g.currentFn.NewValue("url_can_parse_res", types.TypeBoolean)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{
		{Block: successBB, Value: ir.ConstBool{Value: true}},
		{Block: invalidBB, Value: ir.ConstBool{Value: false}},
	}})
	return res
}

func (g *generator) lowerURLParse(e *ast.CallExpr) ir.Operand {
	invalidBB := g.currentFn.NewBlock("url_parse_invalid")
	var baseExpr ast.Expr
	if len(e.Args) > 1 {
		baseExpr = e.Args[1]
	}
	rec := g.lowerURLResolveAndParse(e.Args[0], baseExpr, invalidBB)
	successBB := g.currentBB
	joinBB := g.currentFn.NewBlock("url_parse_join")
	successBB.Terminator = &ir.JumpTerm{Target: joinBB}
	invalidBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = joinBB
	res := g.currentFn.NewValue("url_parse_res", g.semaResult.URLType)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{
		{Block: successBB, Value: rec},
		{Block: invalidBB, Value: ir.ConstNull{}},
	}})
	return res
}

func (g *generator) lowerURLSetHref(url, value ir.Operand) ir.Operand {
	invalid := g.currentFn.NewBlock("url_set_href_invalid")
	newRecord := g.lowerURLParseRecord(value, invalid)
	resultBB := g.currentBB

	offsets, _, _ := g.objectLayout(g.semaResult.URLType)
	for _, field := range []string{"$scheme", "$hostname", "$port", "$pathname", "$query", "$fragment"} {
		val := g.lowerURLField(newRecord, field)
		resultBB.Instructions = append(resultBB.Instructions, &ir.SetFieldInst{
			Obj: url, Field: field, Offset: offsets[field], Val: val,
		})
	}
	newQuery := g.lowerURLField(newRecord, "$query")
	params := g.currentFn.NewValue("url_search_params_for_href", g.semaResult.URLSearchParamsType)
	resultBB.Instructions = append(resultBB.Instructions, &ir.GetFieldInst{
		Res: params, Obj: url, Field: "$searchParams", Offset: offsets["$searchParams"],
	})
	g.currentBB = resultBB
	g.lowerURLSearchParamsReplace(params, newQuery)
	successEnd := g.currentBB

	g.currentBB = invalid
	err := g.newWebError(ir.ConstString{Value: "Invalid URL"}, ir.ConstString{Value: "TypeError"})
	g.routeThrownValue(err)

	g.currentBB = successEnd
	return value
}

func (g *generator) lowerURLSetHash(url, value ir.Operand) ir.Operand {
	fragment := g.lowerURLStripLeadingByte(value, '#')
	offsets, _, _ := g.objectLayout(g.semaResult.URLType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: url, Field: "$fragment", Offset: offsets["$fragment"], Val: fragment,
	})
	return value
}

func (g *generator) lowerURLSetPathname(url, value ir.Operand) ir.Operand {
	first := g.urlStringByteAt(value, ir.ConstNumber{Value: 0})
	isSlash := g.currentFn.NewValue("url_set_path_is_slash", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: isSlash, Op: ir.OpEq, LHS: first, RHS: ir.ConstNumber{Value: '/'},
	})
	slashBB := g.currentFn.NewBlock("url_set_path_slash")
	noSlashBB := g.currentFn.NewBlock("url_set_path_noslash")
	joinBB := g.currentFn.NewBlock("url_set_path_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isSlash, Then: slashBB, Else: noSlashBB}

	slashBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = noSlashBB
	prefixed := g.concatNativeStrings(ir.ConstString{Value: "/"}, value)
	noSlashEnd := g.currentBB
	noSlashEnd.Terminator = &ir.JumpTerm{Target: joinBB}

	pathVal := g.currentFn.NewValue("url_set_path_val", types.TypeString)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: pathVal, Incoming: []ir.PhiIncoming{
		{Block: slashBB, Value: value},
		{Block: noSlashEnd, Value: prefixed},
	}})
	g.currentBB = joinBB

	normalized := g.lowerURLNormalizePath(pathVal)
	offsets, _, _ := g.objectLayout(g.semaResult.URLType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: url, Field: "$pathname", Offset: offsets["$pathname"], Val: normalized,
	})
	return value
}

func (g *generator) lowerURLSetProtocol(url, value ir.Operand) ir.Operand {
	total := g.urlStringLen(value)
	hasChars := g.currentFn.NewValue("url_set_proto_has_chars", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: hasChars, Op: ir.OpGt, LHS: total, RHS: ir.ConstNumber{Value: 0},
	})
	lastIdx := g.currentFn.NewValue("url_set_proto_last_idx", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: lastIdx, Op: ir.OpSub, LHS: total, RHS: ir.ConstNumber{Value: 1},
	})
	lastByte := g.urlStringByteAt(value, lastIdx)
	isColon := g.currentFn.NewValue("url_set_proto_is_colon", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: isColon, Op: ir.OpEq, LHS: lastByte, RHS: ir.ConstNumber{Value: ':'},
	})
	stripColon := g.currentFn.NewValue("url_set_proto_strip_colon", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: stripColon, Op: ir.OpAnd, LHS: hasChars, RHS: isColon,
	})

	stripBB := g.currentFn.NewBlock("url_set_proto_strip")
	keepBB := g.currentFn.NewBlock("url_set_proto_keep")
	joinBB := g.currentFn.NewBlock("url_set_proto_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: stripColon, Then: stripBB, Else: keepBB}

	g.currentBB = stripBB
	stripped := g.urlStringSlice(value, ir.ConstNumber{Value: 0}, lastIdx)
	stripEnd := g.currentBB
	stripEnd.Terminator = &ir.JumpTerm{Target: joinBB}

	keepBB.Terminator = &ir.JumpTerm{Target: joinBB}

	schemeVal := g.currentFn.NewValue("url_set_proto_scheme_val", types.TypeString)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: schemeVal, Incoming: []ir.PhiIncoming{
		{Block: stripEnd, Value: stripped},
		{Block: keepBB, Value: value},
	}})
	g.currentBB = joinBB

	canonicalScheme := g.currentFn.NewValue("url_set_proto_canonical", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: canonicalScheme, Callee: "ts_string_ascii_lower", Args: []ir.Operand{schemeVal}, ParamTypes: []types.Type{types.TypeString},
	})
	isHTTP := g.urlStringEqual(canonicalScheme, "http")
	isHTTPS := g.urlStringEqual(canonicalScheme, "https")
	isValid := g.currentFn.NewValue("url_set_proto_valid", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: isValid, Op: ir.OpOr, LHS: isHTTP, RHS: isHTTPS,
	})

	setBB := g.currentFn.NewBlock("url_set_proto_set")
	doneBB := g.currentFn.NewBlock("url_set_proto_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isValid, Then: setBB, Else: doneBB}

	g.currentBB = setBB
	offsets, _, _ := g.objectLayout(g.semaResult.URLType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: url, Field: "$scheme", Offset: offsets["$scheme"], Val: canonicalScheme,
	})
	currentPort := g.lowerURLField(url, "$port")
	normalizedPort := g.lowerURLNormalizePort(canonicalScheme, currentPort)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: url, Field: "$port", Offset: offsets["$port"], Val: normalizedPort,
	})
	setEnd := g.currentBB
	setEnd.Terminator = &ir.JumpTerm{Target: doneBB}

	g.currentBB = doneBB
	return value
}

func (g *generator) lowerURLSetPort(url, value ir.Operand) ir.Operand {
	total := g.urlStringLen(value)
	isEmpty := g.currentFn.NewValue("url_set_port_empty", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: isEmpty, Op: ir.OpEq, LHS: total, RHS: ir.ConstNumber{Value: 0},
	})
	emptyBB := g.currentFn.NewBlock("url_set_port_is_empty")
	nonEmptyBB := g.currentFn.NewBlock("url_set_port_non_empty")
	doneBB := g.currentFn.NewBlock("url_set_port_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isEmpty, Then: emptyBB, Else: nonEmptyBB}

	g.currentBB = emptyBB
	offsets, _, _ := g.objectLayout(g.semaResult.URLType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: url, Field: "$port", Offset: offsets["$port"], Val: ir.ConstString{Value: ""},
	})
	emptyBB.Terminator = &ir.JumpTerm{Target: doneBB}

	g.currentBB = nonEmptyBB
	entryBB := g.currentBB
	condBB := g.currentFn.NewBlock("url_set_port_cond")
	bodyBB := g.currentFn.NewBlock("url_set_port_body")
	stepBB := g.currentFn.NewBlock("url_set_port_step")
	checkBB := g.currentFn.NewBlock("url_set_port_check")
	entryBB.Terminator = &ir.JumpTerm{Target: condBB}

	idx := g.currentFn.NewValue("url_set_port_i", types.TypeNumber)
	acc := g.currentFn.NewValue("url_set_port_acc", types.TypeNumber)
	allDigits := g.currentFn.NewValue("url_set_port_all_digits", types.TypeBoolean)

	nextIdx := g.currentFn.NewValue("url_set_port_next_i", types.TypeNumber)
	nextAcc := g.currentFn.NewValue("url_set_port_next_acc", types.TypeNumber)
	nextAllDigits := g.currentFn.NewValue("url_set_port_next_all_digits", types.TypeBoolean)

	condBB.Phis = append(condBB.Phis,
		&ir.PhiInst{Res: idx, Incoming: []ir.PhiIncoming{{Block: entryBB, Value: ir.ConstNumber{Value: 0}}}},
		&ir.PhiInst{Res: acc, Incoming: []ir.PhiIncoming{{Block: entryBB, Value: ir.ConstNumber{Value: 0}}}},
		&ir.PhiInst{Res: allDigits, Incoming: []ir.PhiIncoming{{Block: entryBB, Value: ir.ConstBool{Value: true}}}},
	)
	hasMore := g.currentFn.NewValue("url_set_port_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{
		Res: hasMore, Op: ir.OpLt, LHS: idx, RHS: total,
	})
	condBB.Terminator = &ir.BranchTerm{Cond: hasMore, Then: bodyBB, Else: checkBB}

	g.currentBB = bodyBB
	ch := g.urlStringByteAt(value, idx)
	ge0 := g.currentFn.NewValue("url_set_port_ge0", types.TypeBoolean)
	le9 := g.currentFn.NewValue("url_set_port_le9", types.TypeBoolean)
	isDig := g.currentFn.NewValue("url_set_port_is_dig", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.BinaryInst{Res: ge0, Op: ir.OpGe, LHS: ch, RHS: ir.ConstNumber{Value: '0'}},
		&ir.BinaryInst{Res: le9, Op: ir.OpLe, LHS: ch, RHS: ir.ConstNumber{Value: '9'}},
		&ir.BinaryInst{Res: isDig, Op: ir.OpAnd, LHS: ge0, RHS: le9},
	)
	curAll := g.currentFn.NewValue("url_set_port_cur_all", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{
		Res: curAll, Op: ir.OpAnd, LHS: allDigits, RHS: isDig,
	})
	digitVal := g.currentFn.NewValue("url_set_port_dig_val", types.TypeNumber)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{
		Res: digitVal, Op: ir.OpSub, LHS: ch, RHS: ir.ConstNumber{Value: '0'},
	})
	accTimes10 := g.currentFn.NewValue("url_set_port_acc10", types.TypeNumber)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{
		Res: accTimes10, Op: ir.OpMul, LHS: acc, RHS: ir.ConstNumber{Value: 10},
	})
	curAcc := g.currentFn.NewValue("url_set_port_cur_acc", types.TypeNumber)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{
		Res: curAcc, Op: ir.OpAdd, LHS: accTimes10, RHS: digitVal,
	})
	curIdx := g.currentFn.NewValue("url_set_port_cur_idx", types.TypeNumber)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{
		Res: curIdx, Op: ir.OpAdd, LHS: idx, RHS: ir.ConstNumber{Value: 1},
	})
	bodyBB.Terminator = &ir.JumpTerm{Target: stepBB}

	stepBB.Instructions = append(stepBB.Instructions,
		&ir.BinaryInst{Res: nextIdx, Op: ir.OpAdd, LHS: curIdx, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: nextAcc, Op: ir.OpAdd, LHS: curAcc, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: nextAllDigits, Op: ir.OpAnd, LHS: curAll, RHS: ir.ConstBool{Value: true}},
	)
	stepBB.Terminator = &ir.JumpTerm{Target: condBB}
	condBB.Phis[0].Incoming = append(condBB.Phis[0].Incoming, ir.PhiIncoming{Block: stepBB, Value: nextIdx})
	condBB.Phis[1].Incoming = append(condBB.Phis[1].Incoming, ir.PhiIncoming{Block: stepBB, Value: nextAcc})
	condBB.Phis[2].Incoming = append(condBB.Phis[2].Incoming, ir.PhiIncoming{Block: stepBB, Value: nextAllDigits})

	g.currentBB = checkBB
	le65535 := g.currentFn.NewValue("url_set_port_le65535", types.TypeBoolean)
	checkBB.Instructions = append(checkBB.Instructions, &ir.BinaryInst{
		Res: le65535, Op: ir.OpLe, LHS: acc, RHS: ir.ConstNumber{Value: 65535},
	})
	portValid := g.currentFn.NewValue("url_set_port_valid", types.TypeBoolean)
	checkBB.Instructions = append(checkBB.Instructions, &ir.BinaryInst{
		Res: portValid, Op: ir.OpAnd, LHS: allDigits, RHS: le65535,
	})
	applyBB := g.currentFn.NewBlock("url_set_port_apply")
	checkBB.Terminator = &ir.BranchTerm{Cond: portValid, Then: applyBB, Else: doneBB}

	g.currentBB = applyBB
	scheme := g.lowerURLField(url, "$scheme")
	normalizedPort := g.lowerURLNormalizePort(scheme, value)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: url, Field: "$port", Offset: offsets["$port"], Val: normalizedPort,
	})
	applyEnd := g.currentBB
	applyEnd.Terminator = &ir.JumpTerm{Target: doneBB}

	g.currentBB = doneBB
	return value
}

func (g *generator) lowerURLSetHostname(url, value ir.Operand) ir.Operand {
	canonical := g.currentFn.NewValue("url_set_hostname_canonical", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: canonical, Callee: "ts_string_ascii_lower", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeString},
	})
	total := g.urlStringLen(canonical)
	nonEmpty := g.currentFn.NewValue("url_set_hostname_nonempty", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: nonEmpty, Op: ir.OpGt, LHS: total, RHS: ir.ConstNumber{Value: 0},
	})
	slash := g.urlStringFindByte(canonical, ir.ConstNumber{Value: '/'}, ir.ConstNumber{Value: 0}, total)
	colon := g.urlStringFindByte(canonical, ir.ConstNumber{Value: ':'}, ir.ConstNumber{Value: 0}, total)
	query := g.urlStringFindByte(canonical, ir.ConstNumber{Value: '?'}, ir.ConstNumber{Value: 0}, total)
	hash := g.urlStringFindByte(canonical, ir.ConstNumber{Value: '#'}, ir.ConstNumber{Value: 0}, total)
	atSign := g.urlStringFindByte(canonical, ir.ConstNumber{Value: '@'}, ir.ConstNumber{Value: 0}, total)
	space := g.urlStringFindByte(canonical, ir.ConstNumber{Value: ' '}, ir.ConstNumber{Value: 0}, total)

	noSlash := g.currentFn.NewValue("url_set_hostname_noslash", types.TypeBoolean)
	noColon := g.currentFn.NewValue("url_set_hostname_nocolon", types.TypeBoolean)
	noQuery := g.currentFn.NewValue("url_set_hostname_noquery", types.TypeBoolean)
	noHash := g.currentFn.NewValue("url_set_hostname_nohash", types.TypeBoolean)
	noAt := g.currentFn.NewValue("url_set_hostname_noat", types.TypeBoolean)
	noSpace := g.currentFn.NewValue("url_set_hostname_nospace", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: noSlash, Op: ir.OpLt, LHS: slash, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: noColon, Op: ir.OpLt, LHS: colon, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: noQuery, Op: ir.OpLt, LHS: query, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: noHash, Op: ir.OpLt, LHS: hash, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: noAt, Op: ir.OpLt, LHS: atSign, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: noSpace, Op: ir.OpLt, LHS: space, RHS: ir.ConstNumber{Value: 0}},
	)
	ok1 := g.currentFn.NewValue("url_set_hostname_ok1", types.TypeBoolean)
	ok2 := g.currentFn.NewValue("url_set_hostname_ok2", types.TypeBoolean)
	ok3 := g.currentFn.NewValue("url_set_hostname_ok3", types.TypeBoolean)
	ok4 := g.currentFn.NewValue("url_set_hostname_ok4", types.TypeBoolean)
	ok5 := g.currentFn.NewValue("url_set_hostname_ok5", types.TypeBoolean)
	isValid := g.currentFn.NewValue("url_set_hostname_valid", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: ok1, Op: ir.OpAnd, LHS: nonEmpty, RHS: noSlash},
		&ir.BinaryInst{Res: ok2, Op: ir.OpAnd, LHS: noColon, RHS: noQuery},
		&ir.BinaryInst{Res: ok3, Op: ir.OpAnd, LHS: ok1, RHS: ok2},
		&ir.BinaryInst{Res: ok4, Op: ir.OpAnd, LHS: ok3, RHS: noHash},
		&ir.BinaryInst{Res: ok5, Op: ir.OpAnd, LHS: ok4, RHS: noAt},
		&ir.BinaryInst{Res: isValid, Op: ir.OpAnd, LHS: ok5, RHS: noSpace},
	)

	setBB := g.currentFn.NewBlock("url_set_hostname_apply")
	doneBB := g.currentFn.NewBlock("url_set_hostname_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isValid, Then: setBB, Else: doneBB}

	g.currentBB = setBB
	offsets, _, _ := g.objectLayout(g.semaResult.URLType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: url, Field: "$hostname", Offset: offsets["$hostname"], Val: canonical,
	})
	setBB.Terminator = &ir.JumpTerm{Target: doneBB}

	g.currentBB = doneBB
	return value
}

func (g *generator) lowerURLSetHost(url, value ir.Operand) ir.Operand {
	total := g.urlStringLen(value)
	colon := g.urlStringFindByte(value, ir.ConstNumber{Value: ':'}, ir.ConstNumber{Value: 0}, total)
	hasColon := g.currentFn.NewValue("url_set_host_has_colon", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: hasColon, Op: ir.OpGe, LHS: colon, RHS: ir.ConstNumber{Value: 0},
	})
	withColonBB := g.currentFn.NewBlock("url_set_host_colon")
	noColonBB := g.currentFn.NewBlock("url_set_host_nocolon")
	joinBB := g.currentFn.NewBlock("url_set_host_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasColon, Then: withColonBB, Else: noColonBB}

	g.currentBB = withColonBB
	hostPart := g.urlStringSlice(value, ir.ConstNumber{Value: 0}, colon)
	portStart := g.currentFn.NewValue("url_set_host_port_start", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: portStart, Op: ir.OpAdd, LHS: colon, RHS: ir.ConstNumber{Value: 1},
	})
	portPart := g.urlStringSlice(value, portStart, total)
	g.lowerURLSetHostname(url, hostPart)
	g.lowerURLSetPort(url, portPart)
	withColonEnd := g.currentBB
	withColonEnd.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = noColonBB
	g.lowerURLSetHostname(url, value)
	offsets, _, _ := g.objectLayout(g.semaResult.URLType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: url, Field: "$port", Offset: offsets["$port"], Val: ir.ConstString{Value: ""},
	})
	noColonEnd := g.currentBB
	noColonEnd.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = joinBB
	return value
}

func (g *generator) lowerURLMemberAssignment(url ir.Operand, property string, rhs ir.Operand) ir.Operand {
	switch property {
	case "href":
		return g.lowerURLSetHref(url, rhs)
	case "protocol":
		return g.lowerURLSetProtocol(url, rhs)
	case "host":
		return g.lowerURLSetHost(url, rhs)
	case "hostname":
		return g.lowerURLSetHostname(url, rhs)
	case "port":
		return g.lowerURLSetPort(url, rhs)
	case "pathname":
		return g.lowerURLSetPathname(url, rhs)
	case "search":
		return g.lowerURLSetSearch(url, rhs)
	case "hash":
		return g.lowerURLSetHash(url, rhs)
	case "origin", "searchParams":
		return g.failExpr("URL.%s is read-only", property)
	default:
		return g.failExpr("URL.%s setter is not implemented yet", property)
	}
}
