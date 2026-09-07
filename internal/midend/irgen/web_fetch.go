package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"strings"
)

func (g *generator) lowerHTTPStatus(raw ir.Operand) ir.Operand {
	get := func(index float64) ir.Operand {
		v := g.currentFn.NewValue("http_status_byte", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{raw, ir.ConstNumber{Value: index}}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber},
		})
		return v
	}
	d1, d2, d3 := get(9), get(10), get(11)
	for _, d := range []ir.Operand{d1, d2, d3} {
		v := g.currentFn.NewValue("http_status_digit", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: v, Op: ir.OpSub, LHS: d, RHS: ir.ConstNumber{Value: 48}})
		switch d {
		case d1:
			d1 = v
		case d2:
			d2 = v
		default:
			d3 = v
		}
	}
	hundreds := g.currentFn.NewValue("http_status_hundreds", types.TypeNumber)
	tens := g.currentFn.NewValue("http_status_tens", types.TypeNumber)
	partial := g.currentFn.NewValue("http_status_partial", types.TypeNumber)
	status := g.currentFn.NewValue("http_status", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: hundreds, Op: ir.OpMul, LHS: d1, RHS: ir.ConstNumber{Value: 100}},
		&ir.BinaryInst{Res: tens, Op: ir.OpMul, LHS: d2, RHS: ir.ConstNumber{Value: 10}},
		&ir.BinaryInst{Res: partial, Op: ir.OpAdd, LHS: hundreds, RHS: tens},
		&ir.BinaryInst{Res: status, Op: ir.OpAdd, LHS: partial, RHS: d3},
	)
	return status
}

func (g *generator) lowerHTTPBodyOffset(raw ir.Operand) ir.Operand {
	length := g.currentFn.NewValue("http_raw_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{raw}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
	pre := g.currentBB
	cond := g.currentFn.NewBlock("http_body_scan_cond")
	body := g.currentFn.NewBlock("http_body_scan_body")
	match := g.currentFn.NewBlock("http_body_scan_match")
	next := g.currentFn.NewBlock("http_body_scan_next")
	exhausted := g.currentFn.NewBlock("http_body_scan_exhausted")
	join := g.currentFn.NewBlock("http_body_scan_join")
	pre.Terminator = &ir.JumpTerm{Target: cond}
	i := g.currentFn.NewValue("http_body_scan_i", types.TypeNumber)
	nextI := g.currentFn.NewValue("http_body_scan_next_i", types.TypeNumber)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: i, Incoming: []ir.PhiIncoming{{Block: pre, Value: ir.ConstNumber{Value: 0}}, {Block: next, Value: nextI}}})
	limit := g.currentFn.NewValue("http_body_scan_limit", types.TypeNumber)
	more := g.currentFn.NewValue("http_body_scan_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions,
		&ir.BinaryInst{Res: limit, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 3}},
		&ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: limit, RHS: length},
	)
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: exhausted}

	bytes := make([]ir.Operand, 4)
	for n := 0; n < 4; n++ {
		idx := ir.Operand(i)
		if n > 0 {
			v := g.currentFn.NewValue("http_body_scan_idx", types.TypeNumber)
			body.Instructions = append(body.Instructions, &ir.BinaryInst{Res: v, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: float64(n)}})
			idx = v
		}
		v := g.currentFn.NewValue("http_body_scan_byte", types.TypeNumber)
		body.Instructions = append(body.Instructions, &ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{raw, idx}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber}})
		bytes[n] = v
	}
	checks := make([]ir.Operand, 4)
	want := []float64{13, 10, 13, 10}
	for n := range checks {
		v := g.currentFn.NewValue("http_body_scan_eq", types.TypeBoolean)
		body.Instructions = append(body.Instructions, &ir.BinaryInst{Res: v, Op: ir.OpEq, LHS: bytes[n], RHS: ir.ConstNumber{Value: want[n]}})
		checks[n] = v
	}
	and1 := g.currentFn.NewValue("http_body_scan_and1", types.TypeBoolean)
	and2 := g.currentFn.NewValue("http_body_scan_and2", types.TypeBoolean)
	all := g.currentFn.NewValue("http_body_scan_all", types.TypeBoolean)
	body.Instructions = append(body.Instructions,
		&ir.BinaryInst{Res: and1, Op: ir.OpAnd, LHS: checks[0], RHS: checks[1]},
		&ir.BinaryInst{Res: and2, Op: ir.OpAnd, LHS: checks[2], RHS: checks[3]},
		&ir.BinaryInst{Res: all, Op: ir.OpAnd, LHS: and1, RHS: and2},
	)
	body.Terminator = &ir.BranchTerm{Cond: all, Then: match, Else: next}
	next.Instructions = append(next.Instructions, &ir.BinaryInst{Res: nextI, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 1}})
	next.Terminator = &ir.JumpTerm{Target: cond}
	found := g.currentFn.NewValue("http_body_offset_found", types.TypeNumber)
	match.Instructions = append(match.Instructions, &ir.BinaryInst{Res: found, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 4}})
	match.Terminator = &ir.JumpTerm{Target: join}
	exhausted.Terminator = &ir.JumpTerm{Target: join}
	g.currentBB = join
	res := g.currentFn.NewValue("http_body_offset", types.TypeNumber)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{{Block: match, Value: found}, {Block: exhausted, Value: length}}})
	return res
}

func (g *generator) lowerHTTPResponseHeaders(raw, bodyOffset ir.Operand) ir.Operand {
	headers := g.newEmptyHeaders()
	headerBytes := g.currentFn.NewValue("http_header_bytes", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: headerBytes, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{raw, ir.ConstNumber{Value: 0}, bodyOffset},
		ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber},
	})
	headerText := g.currentFn.NewValue("http_header_text", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: headerText, Callee: "ts_byte_buffer_to_utf8_string", Args: []ir.Operand{headerBytes}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
	})
	total := g.urlStringLen(headerText)
	firstLF := g.urlStringFindByte(headerText, ir.ConstNumber{Value: 10}, ir.ConstNumber{Value: 0}, total)
	start := g.currentFn.NewValue("http_header_start", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: start, Op: ir.OpAdd, LHS: firstLF, RHS: ir.ConstNumber{Value: 1}})

	pre := g.currentBB
	cond := g.currentFn.NewBlock("http_headers_parse_cond")
	body := g.currentFn.NewBlock("http_headers_parse_body")
	appendBB := g.currentFn.NewBlock("http_headers_parse_append")
	done := g.currentFn.NewBlock("http_headers_parse_done")
	pre.Terminator = &ir.JumpTerm{Target: cond}
	pos := g.currentFn.NewValue("http_headers_pos", types.TypeNumber)
	nextPos := g.currentFn.NewValue("http_headers_next_pos", types.TypeNumber)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: pos, Incoming: []ir.PhiIncoming{{Block: pre, Value: start}, {Block: appendBB, Value: nextPos}}})
	more := g.currentFn.NewValue("http_headers_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: pos, RHS: total})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: done}

	g.currentBB = body
	lineLF := g.urlStringFindByte(headerText, ir.ConstNumber{Value: 10}, pos, total)
	lineEnd := g.currentFn.NewValue("http_header_line_end", types.TypeNumber)
	body.Instructions = append(body.Instructions, &ir.BinaryInst{Res: lineEnd, Op: ir.OpSub, LHS: lineLF, RHS: ir.ConstNumber{Value: 1}})
	colon := g.urlStringFindByte(headerText, ir.ConstNumber{Value: 58}, pos, lineEnd)
	colonFound := g.currentFn.NewValue("http_header_colon_found", types.TypeBoolean)
	colonBeforeEnd := g.currentFn.NewValue("http_header_colon_before_end", types.TypeBoolean)
	hasColon := g.currentFn.NewValue("http_header_has_colon", types.TypeBoolean)
	body = g.currentBB
	body.Instructions = append(body.Instructions,
		&ir.BinaryInst{Res: colonFound, Op: ir.OpGe, LHS: colon, RHS: pos},
		&ir.BinaryInst{Res: colonBeforeEnd, Op: ir.OpLt, LHS: colon, RHS: lineEnd},
		&ir.BinaryInst{Res: hasColon, Op: ir.OpAnd, LHS: colonFound, RHS: colonBeforeEnd},
	)
	body.Terminator = &ir.BranchTerm{Cond: hasColon, Then: appendBB, Else: done}

	g.currentBB = appendBB
	name := g.urlStringSlice(headerText, pos, colon)
	valueStart := g.currentFn.NewValue("http_header_value_start", types.TypeNumber)
	appendBB.Instructions = append(appendBB.Instructions, &ir.BinaryInst{Res: valueStart, Op: ir.OpAdd, LHS: colon, RHS: ir.ConstNumber{Value: 1}})
	value := g.urlStringSlice(headerText, valueStart, lineEnd)
	g.lowerHeadersAppendDirect(headers, name, value)
	appendBB = g.currentBB
	appendBB.Instructions = append(appendBB.Instructions, &ir.BinaryInst{Res: nextPos, Op: ir.OpAdd, LHS: lineLF, RHS: ir.ConstNumber{Value: 1}})
	appendBB.Terminator = &ir.JumpTerm{Target: cond}
	cond.Phis[0].Incoming[1].Block = appendBB
	g.currentBB = done
	return headers
}

func (g *generator) serializeFetchHeaders(headers ir.Operand) ir.Operand {
	entries := g.headersEntries(headers)
	length := g.currentFn.NewValue("fetch_headers_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})
	pre := g.currentBB
	cond := g.currentFn.NewBlock("fetch_headers_cond")
	body := g.currentFn.NewBlock("fetch_headers_body")
	done := g.currentFn.NewBlock("fetch_headers_done")
	pre.Terminator = &ir.JumpTerm{Target: cond}
	idx := g.currentFn.NewValue("fetch_headers_i", types.TypeNumber)
	nextIdx := g.currentFn.NewValue("fetch_headers_next_i", types.TypeNumber)
	acc := g.currentFn.NewValue("fetch_headers_acc", types.TypeString)
	cond.Phis = append(cond.Phis,
		&ir.PhiInst{Res: idx, Incoming: []ir.PhiIncoming{{Block: pre, Value: ir.ConstNumber{Value: 0}}, {Block: body, Value: nextIdx}}},
		&ir.PhiInst{Res: acc, Incoming: []ir.PhiIncoming{{Block: pre, Value: ir.ConstString{Value: ""}}}},
	)
	more := g.currentFn.NewValue("fetch_headers_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: idx, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: done}

	name := g.currentFn.NewValue("fetch_header_name", types.TypeString)
	valueIndex := g.currentFn.NewValue("fetch_header_value_i", types.TypeNumber)
	value := g.currentFn.NewValue("fetch_header_value", types.TypeString)
	body.Instructions = append(body.Instructions,
		&ir.GetElementInst{Res: name, Array: entries, Index: idx},
		&ir.BinaryInst{Res: valueIndex, Op: ir.OpAdd, LHS: idx, RHS: ir.ConstNumber{Value: 1}},
		&ir.GetElementInst{Res: value, Array: entries, Index: valueIndex},
	)
	g.currentBB = body
	line := g.concatNativeStrings(name, ir.ConstString{Value: ": "})
	line = g.concatNativeStrings(line, value)
	line = g.concatNativeStrings(line, ir.ConstString{Value: "\r\n"})
	computed := g.concatNativeStrings(acc, line)
	body = g.currentBB
	body.Instructions = append(body.Instructions,
		&ir.BinaryInst{Res: nextIdx, Op: ir.OpAdd, LHS: idx, RHS: ir.ConstNumber{Value: 2}},
	)
	body.Terminator = &ir.JumpTerm{Target: cond}
	cond.Phis[0].Incoming[1].Block = body
	cond.Phis[1].Incoming = append(cond.Phis[1].Incoming, ir.PhiIncoming{Block: body, Value: computed})
	g.currentBB = done
	return acc
}

func (g *generator) buildFetchWireRequest(method, host, port, path, search, headers, body ir.Operand) ir.Operand {
	target := g.concatNativeStrings(path, search)
	hostHeader := g.concatNativeStrings(g.concatNativeStrings(host, ir.ConstString{Value: ":"}), port)
	requestText := g.concatNativeStrings(method, ir.ConstString{Value: " "})
	requestText = g.concatNativeStrings(requestText, target)
	requestText = g.concatNativeStrings(requestText, ir.ConstString{Value: " HTTP/1.1\r\nHost: "})
	requestText = g.concatNativeStrings(requestText, hostHeader)
	requestText = g.concatNativeStrings(requestText, ir.ConstString{Value: "\r\nConnection: close\r\nUser-Agent: ts-pro\r\n"})
	requestText = g.concatNativeStrings(requestText, g.serializeFetchHeaders(headers))
	bodyLen := g.currentFn.NewValue("fetch_body_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: bodyLen, Callee: "ts_byte_buffer_len", Args: []ir.Operand{body}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
	bodyLenText := g.currentFn.NewValue("fetch_body_len_text", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: bodyLenText, Callee: "ts_number_to_string", Args: []ir.Operand{bodyLen}, ParamTypes: []types.Type{types.TypeNumber}})
	requestText = g.concatNativeStrings(requestText, ir.ConstString{Value: "Content-Length: "})
	requestText = g.concatNativeStrings(requestText, bodyLenText)
	requestText = g.concatNativeStrings(requestText, ir.ConstString{Value: "\r\n\r\n"})

	headerBuf := g.currentFn.NewValue("fetch_header_buffer", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: headerBuf, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{requestText}, ParamTypes: []types.Type{types.TypeString}})
	headerLen := g.currentFn.NewValue("fetch_header_len", types.TypeNumber)
	totalLen := g.currentFn.NewValue("fetch_wire_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.CallInst{Res: headerLen, Callee: "ts_byte_buffer_len", Args: []ir.Operand{headerBuf}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}},
		&ir.BinaryInst{Res: totalLen, Op: ir.OpAdd, LHS: headerLen, RHS: bodyLen},
	)
	wire := g.currentFn.NewValue("fetch_wire_buffer", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.CallInst{Res: wire, Callee: "ts_byte_buffer_new", Args: []ir.Operand{totalLen}, ParamTypes: []types.Type{types.TypeNumber}},
		&ir.CallInst{Callee: "ts_byte_buffer_copy", Args: []ir.Operand{wire, headerBuf, ir.ConstNumber{Value: 0}, ir.ConstNumber{Value: 0}, headerLen}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber, types.TypeNumber}},
		&ir.CallInst{Callee: "ts_byte_buffer_copy", Args: []ir.Operand{wire, body, headerLen, ir.ConstNumber{Value: 0}, bodyLen}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber, types.TypeNumber}},
	)
	return wire
}

func (g *generator) lowerFetchRound(href, method, headers, body, signal ir.Operand) (ir.Operand, ir.Operand, ir.Operand, ir.Operand, ir.Operand) {
	invalid := g.currentFn.NewBlock("fetch_url_invalid")
	urlObj := g.lowerURLParseRecord(href, invalid)
	canonicalHref, _ := g.lowerURLMember(urlObj, "href")
	scheme, _ := g.lowerURLMember(urlObj, "protocol")
	host, _ := g.lowerURLMember(urlObj, "hostname")
	port, _ := g.lowerURLMember(urlObj, "port")
	path, _ := g.lowerURLMember(urlObj, "pathname")
	search, _ := g.lowerURLMember(urlObj, "search")
	parseOK := g.currentBB
	g.currentBB = invalid
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Failed to parse URL from fetch request"}, ir.ConstString{Value: "TypeError"}))
	g.currentBB = parseOK

	isHTTP := g.urlStringEqual(scheme, "http:")
	isLoopback := g.urlStringEqual(host, "127.0.0.1")
	allowed := g.currentFn.NewValue("fetch_transport_allowed", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: allowed, Op: ir.OpAnd, LHS: isHTTP, RHS: isLoopback})
	transportOK := g.currentFn.NewBlock("fetch_transport_ok")
	transportErr := g.currentFn.NewBlock("fetch_transport_err")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: allowed, Then: transportOK, Else: transportErr}
	g.currentBB = transportErr
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "fetch transport currently supports loopback HTTP only"}, ir.ConstString{Value: "TypeError"}))
	g.currentBB = transportOK

	aborted := g.currentFn.NewValue("fetch_signal_aborted", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: aborted, Callee: "ts_abort_signal_aborted", Args: []ir.Operand{signal}})
	fetchAbort := g.currentFn.NewBlock("fetch_aborted")
	fetchDispatch := g.currentFn.NewBlock("fetch_dispatch")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: aborted, Then: fetchAbort, Else: fetchDispatch}
	g.currentBB = fetchAbort
	reason := g.currentFn.NewValue("fetch_abort_reason", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: reason, Callee: "ts_abort_signal_reason", Args: []ir.Operand{signal}})
	g.routeThrownValue(reason)
	g.currentBB = fetchDispatch

	requestBuf := g.buildFetchWireRequest(method, host, port, path, search, headers, body)
	raw := g.currentFn.NewValue("fetch_raw_response", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: raw, Callee: "ts_net_http_request_loopback", Args: []ir.Operand{port, requestBuf}, ParamTypes: []types.Type{types.TypeString, g.semaResult.ByteBufferType}})
	status := g.lowerHTTPStatus(raw)
	offset := g.lowerHTTPBodyOffset(raw)
	length := g.currentFn.NewValue("fetch_raw_length", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{raw}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
	responseBody := g.currentFn.NewValue("fetch_response_body", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: responseBody, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{raw, offset, length}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
	responseHeaders := g.lowerHTTPResponseHeaders(raw, offset)
	return urlObj, canonicalHref, status, responseHeaders, responseBody
}

func (g *generator) lowerFetchRedirectStatus(status ir.Operand) ir.Operand {
	var combined ir.Operand
	for i, code := range []float64{301, 302, 303, 307, 308} {
		eq := g.currentFn.NewValue("fetch_redirect_status", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: eq, Op: ir.OpEq, LHS: status, RHS: ir.ConstNumber{Value: code}})
		if i == 0 {
			combined = eq
		} else {
			next := g.currentFn.NewValue("fetch_redirect_status_any", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpOr, LHS: combined, RHS: eq})
			combined = next
		}
	}
	return combined
}

func (g *generator) lowerFetchRedirectMethodBody(status, method, body ir.Operand) (ir.Operand, ir.Operand) {
	is301 := g.currentFn.NewValue("fetch_redirect_301", types.TypeBoolean)
	is302 := g.currentFn.NewValue("fetch_redirect_302", types.TypeBoolean)
	is303 := g.currentFn.NewValue("fetch_redirect_303", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: is301, Op: ir.OpEq, LHS: status, RHS: ir.ConstNumber{Value: 301}},
		&ir.BinaryInst{Res: is302, Op: ir.OpEq, LHS: status, RHS: ir.ConstNumber{Value: 302}},
		&ir.BinaryInst{Res: is303, Op: ir.OpEq, LHS: status, RHS: ir.ConstNumber{Value: 303}},
	)
	isPost := g.urlStringEqual(method, "POST")
	isGet := g.urlStringEqual(method, "GET")
	isHead := g.urlStringEqual(method, "HEAD")
	is301or302 := g.currentFn.NewValue("fetch_redirect_301_302", types.TypeBoolean)
	post301or302 := g.currentFn.NewValue("fetch_redirect_post_301_302", types.TypeBoolean)
	getOrHead := g.currentFn.NewValue("fetch_redirect_get_head", types.TypeBoolean)
	notGetOrHead := g.currentFn.NewValue("fetch_redirect_not_get_head", types.TypeBoolean)
	rewrite303 := g.currentFn.NewValue("fetch_redirect_rewrite_303", types.TypeBoolean)
	rewrite := g.currentFn.NewValue("fetch_redirect_rewrite_method", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: is301or302, Op: ir.OpOr, LHS: is301, RHS: is302},
		&ir.BinaryInst{Res: post301or302, Op: ir.OpAnd, LHS: is301or302, RHS: isPost},
		&ir.BinaryInst{Res: getOrHead, Op: ir.OpOr, LHS: isGet, RHS: isHead},
		&ir.BinaryInst{Res: notGetOrHead, Op: ir.OpEq, LHS: getOrHead, RHS: ir.ConstBool{Value: false}},
		&ir.BinaryInst{Res: rewrite303, Op: ir.OpAnd, LHS: is303, RHS: notGetOrHead},
		&ir.BinaryInst{Res: rewrite, Op: ir.OpOr, LHS: post301or302, RHS: rewrite303},
	)
	pre := g.currentBB
	rewriteBB := g.currentFn.NewBlock("fetch_redirect_rewrite")
	preserveBB := g.currentFn.NewBlock("fetch_redirect_preserve")
	joinBB := g.currentFn.NewBlock("fetch_redirect_method_join")
	pre.Terminator = &ir.BranchTerm{Cond: rewrite, Then: rewriteBB, Else: preserveBB}
	g.currentBB = rewriteBB
	empty := g.emptyByteBuffer()
	rewriteEnd := g.currentBB
	rewriteEnd.Terminator = &ir.JumpTerm{Target: joinBB}
	preserveBB.Terminator = &ir.JumpTerm{Target: joinBB}
	g.currentBB = joinBB
	outMethod := g.currentFn.NewValue("fetch_redirect_method", types.TypeString)
	outBody := g.currentFn.NewValue("fetch_redirect_body", g.semaResult.ByteBufferType)
	joinBB.Phis = append(joinBB.Phis,
		&ir.PhiInst{Res: outMethod, Incoming: []ir.PhiIncoming{{Block: rewriteEnd, Value: ir.ConstString{Value: "GET"}}, {Block: preserveBB, Value: method}}},
		&ir.PhiInst{Res: outBody, Incoming: []ir.PhiIncoming{{Block: rewriteEnd, Value: empty}, {Block: preserveBB, Value: body}}},
	)
	return outMethod, outBody
}

func (g *generator) lowerFetchCall(e *ast.CallExpr) ir.Operand {
	if len(e.Args) == 0 {
		return g.failExpr("fetch expects an input")
	}

	var href, method, headers, body, signal ir.Operand
	inputType := g.semanticType(e.Args[0])
	if obj, ok := inputType.(*types.ObjectType); ok && obj.Name == "$Request" {
		req := g.lowerExpr(e.Args[0])
		g.ensureRequestBodyUnused(req)
		href = g.requestField(req, "url", types.TypeString)
		method = g.requestField(req, "method", types.TypeString)
		headers = g.cloneHeaders(g.requestField(req, "headers", g.semaResult.HeadersType))
		body = g.copyByteBuffer(g.requestField(req, "$bodyData", g.semaResult.ByteBufferType))
		signal = g.requestField(req, "signal", g.semaResult.AbortSignalType)
		g.setRequestField(req, "bodyUsed", ir.ConstBool{Value: true})
	} else if inputType == types.TypeString {
		invalid := g.currentFn.NewBlock("fetch_input_url_invalid")
		urlObj := g.lowerURLResolveAndParse(e.Args[0], nil, invalid)
		href, _ = g.lowerURLMember(urlObj, "href")
		parseOK := g.currentBB
		g.currentBB = invalid
		g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Failed to parse URL from fetch input"}, ir.ConstString{Value: "TypeError"}))
		g.currentBB = parseOK
		method = ir.ConstString{Value: "GET"}
		headers = g.newEmptyHeaders()
		body = g.emptyByteBuffer()
		freshSignal := g.currentFn.NewValue("fetch_signal", g.semaResult.AbortSignalType)
		signal = freshSignal
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: freshSignal, Callee: "ts_abort_signal_new"})
	} else {
		return g.failExpr("fetch input must be a string or Request")
	}

	if len(e.Args) > 1 {
		if lit, ok := e.Args[1].(*ast.ObjectLit); ok {
			if ex := objectLiteralProperty(lit, "method"); ex != nil {
				method = g.coerceStringType(g.semanticType(ex), g.lowerExpr(ex))
				if c, ok := method.(ir.ConstString); ok {
					method = ir.ConstString{Value: strings.ToUpper(c.Value)}
				}
			}
			if ex := objectLiteralProperty(lit, "headers"); ex != nil {
				headers = g.lowerHeadersNew(&ast.NewExpr{ClassName: "Headers", Args: []ast.Expr{ex}})
			}
			if ex := objectLiteralProperty(lit, "body"); ex != nil {
				body, _ = g.lowerRequestBody(ex)
			}
			if ex := objectLiteralProperty(lit, "signal"); ex != nil {
				signal = g.lowerExpr(ex)
			}
		}
	}

	urlObj, finalHref, status, responseHeaders, responseBody := g.lowerFetchRound(href, method, headers, body, signal)
	isRedirect := g.lowerFetchRedirectStatus(status)
	hasLocation := g.lowerHeadersHas(responseHeaders, ir.ConstString{Value: "location"})
	shouldRedirect := g.currentFn.NewValue("fetch_should_redirect", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: shouldRedirect, Op: ir.OpAnd, LHS: isRedirect, RHS: hasLocation})
	redirectBB := g.currentFn.NewBlock("fetch_redirect_follow")
	directBB := g.currentFn.NewBlock("fetch_redirect_direct")
	joinBB := g.currentFn.NewBlock("fetch_redirect_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: shouldRedirect, Then: redirectBB, Else: directBB}

	directBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = redirectBB
	location := g.lowerHeadersGet(responseHeaders, ir.ConstString{Value: "location"})
	resolved := g.lowerURLResolveInput(location, urlObj)
	redirectInvalid := g.currentFn.NewBlock("fetch_redirect_url_invalid")
	redirectURL := g.lowerURLParseRecord(resolved, redirectInvalid)
	redirectHref, _ := g.lowerURLMember(redirectURL, "href")
	redirectParseOK := g.currentBB
	g.currentBB = redirectInvalid
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Invalid redirect URL"}, ir.ConstString{Value: "TypeError"}))
	g.currentBB = redirectParseOK
	redirectMethod, redirectBody := g.lowerFetchRedirectMethodBody(status, method, body)
	_, followedHref, followedStatus, followedHeaders, followedBody := g.lowerFetchRound(redirectHref, redirectMethod, headers, redirectBody, signal)
	followEnd := g.currentBB
	followEnd.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = joinBB
	outHref := g.currentFn.NewValue("fetch_final_href", types.TypeString)
	outStatus := g.currentFn.NewValue("fetch_final_status", types.TypeNumber)
	outHeaders := g.currentFn.NewValue("fetch_final_headers", g.semaResult.HeadersType)
	outBody := g.currentFn.NewValue("fetch_final_body", g.semaResult.ByteBufferType)
	redirected := g.currentFn.NewValue("fetch_redirected", types.TypeBoolean)
	joinBB.Phis = append(joinBB.Phis,
		&ir.PhiInst{Res: outHref, Incoming: []ir.PhiIncoming{{Block: directBB, Value: finalHref}, {Block: followEnd, Value: followedHref}}},
		&ir.PhiInst{Res: outStatus, Incoming: []ir.PhiIncoming{{Block: directBB, Value: status}, {Block: followEnd, Value: followedStatus}}},
		&ir.PhiInst{Res: outHeaders, Incoming: []ir.PhiIncoming{{Block: directBB, Value: responseHeaders}, {Block: followEnd, Value: followedHeaders}}},
		&ir.PhiInst{Res: outBody, Incoming: []ir.PhiIncoming{{Block: directBB, Value: responseBody}, {Block: followEnd, Value: followedBody}}},
		&ir.PhiInst{Res: redirected, Incoming: []ir.PhiIncoming{{Block: directBB, Value: ir.ConstBool{Value: false}}, {Block: followEnd, Value: ir.ConstBool{Value: true}}}},
	)
	res := g.newResponseObject(outBody, ir.ConstBool{Value: true}, outHeaders, outStatus, ir.ConstString{Value: ""}, ir.ConstString{Value: "default"}, outHref, redirected)
	return g.makeImmediatePromiseTask(res, g.semaResult.ResponseType, g.semaResult.ResponseType)
}
