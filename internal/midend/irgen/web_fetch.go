package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
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

func (g *generator) lowerFetchCall(e *ast.CallExpr) ir.Operand {
	if len(e.Args) == 0 {
		return g.failExpr("fetch expects an input")
	}
	if g.semanticType(e.Args[0]) != types.TypeString {
		return g.failExpr("fetch currently requires a string URL while Request transport normalization is being completed")
	}
	invalid := g.currentFn.NewBlock("fetch_url_invalid")
	urlObj := g.lowerURLResolveAndParse(e.Args[0], nil, invalid)
	href, _ := g.lowerURLMember(urlObj, "href")
	scheme, _ := g.lowerURLMember(urlObj, "protocol")
	host, _ := g.lowerURLMember(urlObj, "hostname")
	port, _ := g.lowerURLMember(urlObj, "port")
	path, _ := g.lowerURLMember(urlObj, "pathname")
	search, _ := g.lowerURLMember(urlObj, "search")
	parseOK := g.currentBB
	g.currentBB = invalid
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Failed to parse URL from fetch input"}, ir.ConstString{Value: "TypeError"}))
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

	target := g.concatNativeStrings(path, search)
	hostHeader := g.concatNativeStrings(g.concatNativeStrings(host, ir.ConstString{Value: ":"}), port)
	requestText := g.concatNativeStrings(ir.ConstString{Value: "GET "}, target)
	requestText = g.concatNativeStrings(requestText, ir.ConstString{Value: " HTTP/1.1\r\nHost: "})
	requestText = g.concatNativeStrings(requestText, hostHeader)
	requestText = g.concatNativeStrings(requestText, ir.ConstString{Value: "\r\nConnection: close\r\nUser-Agent: ts-pro\r\n\r\n"})
	requestBuf := g.currentFn.NewValue("fetch_request_buffer", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: requestBuf, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{requestText}, ParamTypes: []types.Type{types.TypeString}})
	raw := g.currentFn.NewValue("fetch_raw_response", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: raw, Callee: "ts_net_http_request_loopback", Args: []ir.Operand{port, requestBuf}, ParamTypes: []types.Type{types.TypeString, g.semaResult.ByteBufferType}})
	status := g.lowerHTTPStatus(raw)
	offset := g.lowerHTTPBodyOffset(raw)
	length := g.currentFn.NewValue("fetch_raw_length", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{raw}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
	body := g.currentFn.NewValue("fetch_response_body", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: body, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{raw, offset, length}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
	res := g.newResponseObject(body, ir.ConstBool{Value: true}, g.newEmptyHeaders(), status, ir.ConstString{Value: ""}, ir.ConstString{Value: "default"}, href, ir.ConstBool{Value: false})
	return g.makeImmediatePromiseTask(res, g.semaResult.ResponseType, g.semaResult.ResponseType)
}
