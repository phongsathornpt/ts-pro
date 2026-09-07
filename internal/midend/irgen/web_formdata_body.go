package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerBodyFormData(data, headers ir.Operand) ir.Operand {
	hasContentType := g.lowerHeadersHas(headers, ir.ConstString{Value: "content-type"})
	inspect := g.currentFn.NewBlock("body_form_data_inspect_type")
	fail := g.currentFn.NewBlock("body_form_data_type_error")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasContentType, Then: inspect, Else: fail}

	g.currentBB = inspect
	contentType := g.lowerHeadersGet(headers, ir.ConstString{Value: "content-type"})
	lower := g.currentFn.NewValue("body_form_data_type_lower", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: lower, Callee: "ts_string_ascii_lower", Args: []ir.Operand{contentType}, ParamTypes: []types.Type{types.TypeString}})
	urlencoded := g.urlStringFind(lower, ir.ConstString{Value: "application/x-www-form-urlencoded"}, ir.ConstNumber{Value: 0})
	isURLEncoded := g.currentFn.NewValue("body_form_data_is_urlencoded", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: isURLEncoded, Op: ir.OpEq, LHS: urlencoded, RHS: ir.ConstNumber{Value: 0}})
	urlBB := g.currentFn.NewBlock("body_form_data_urlencoded")
	multiCheck := g.currentFn.NewBlock("body_form_data_multipart_check")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isURLEncoded, Then: urlBB, Else: multiCheck}

	g.currentBB = urlBB
	returnURL := g.lowerBodyURLEncodedFormData(data, headers)
	urlDone := g.currentBB

	g.currentBB = multiCheck
	multipart := g.urlStringFind(lower, ir.ConstString{Value: "multipart/form-data"}, ir.ConstNumber{Value: 0})
	isMultipart := g.currentFn.NewValue("body_form_data_is_multipart", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: isMultipart, Op: ir.OpEq, LHS: multipart, RHS: ir.ConstNumber{Value: 0}})
	multiBB := g.currentFn.NewBlock("body_form_data_multipart")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isMultipart, Then: multiBB, Else: fail}

	g.currentBB = multiBB
	returnMulti := g.lowerBodyMultipartFormData(data, contentType, lower, fail)
	multiDone := g.currentBB

	g.currentBB = fail
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Body MIME type is not supported by formData()"}, ir.ConstString{Value: "TypeError"}))

	join := g.currentFn.NewBlock("body_form_data_dispatch_join")
	if urlDone.Terminator == nil {
		urlDone.Terminator = &ir.JumpTerm{Target: join}
	}
	if multiDone.Terminator == nil {
		multiDone.Terminator = &ir.JumpTerm{Target: join}
	}
	g.currentBB = join
	result := g.currentFn.NewValue("body_form_data_dispatch_result", returnURL.Type())
	join.Phis = append(join.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{{Block: urlDone, Value: returnURL}, {Block: multiDone, Value: returnMulti}}})
	return result
}

func (g *generator) multipartBoundary(contentType, lower ir.Operand, fail *ir.BasicBlock) ir.Operand {
	marker := ir.ConstString{Value: "boundary="}
	at := g.urlStringFind(lower, marker, ir.ConstNumber{Value: 0})
	found := g.currentFn.NewValue("multipart_boundary_found", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: found, Op: ir.OpGe, LHS: at, RHS: ir.ConstNumber{Value: 0}})
	foundBB := g.currentFn.NewBlock("multipart_boundary_found")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: found, Then: foundBB, Else: fail}
	g.currentBB = foundBB
	start := g.currentFn.NewValue("multipart_boundary_start", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: start, Op: ir.OpAdd, LHS: at, RHS: ir.ConstNumber{Value: 9}})
	total := g.urlStringLen(contentType)
	quote := g.urlStringByteAt(contentType, start)
	quoted := g.currentFn.NewValue("multipart_boundary_quoted", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: quoted, Op: ir.OpEq, LHS: quote, RHS: ir.ConstNumber{Value: '"'}})
	quotedBB := g.currentFn.NewBlock("multipart_boundary_quoted")
	plainBB := g.currentFn.NewBlock("multipart_boundary_plain")
	join := g.currentFn.NewBlock("multipart_boundary_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: quoted, Then: quotedBB, Else: plainBB}

	g.currentBB = quotedBB
	qStart := g.currentFn.NewValue("multipart_boundary_qstart", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: qStart, Op: ir.OpAdd, LHS: start, RHS: ir.ConstNumber{Value: 1}})
	qEnd := g.urlStringFindByte(contentType, ir.ConstNumber{Value: '"'}, qStart, total)
	qOK := g.currentFn.NewValue("multipart_boundary_qend_ok", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: qOK, Op: ir.OpGe, LHS: qEnd, RHS: ir.ConstNumber{Value: 0}})
	qSliceBB := g.currentFn.NewBlock("multipart_boundary_qslice")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: qOK, Then: qSliceBB, Else: fail}
	g.currentBB = qSliceBB
	qBoundary := g.urlStringSlice(contentType, qStart, qEnd)
	qSliceBB.Terminator = &ir.JumpTerm{Target: join}

	g.currentBB = plainBB
	semi := g.urlStringFindByte(contentType, ir.ConstNumber{Value: ';'}, start, total)
	hasSemi := g.currentFn.NewValue("multipart_boundary_has_semi", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasSemi, Op: ir.OpGe, LHS: semi, RHS: ir.ConstNumber{Value: 0}})
	semiBB := g.currentFn.NewBlock("multipart_boundary_semi")
	endBB := g.currentFn.NewBlock("multipart_boundary_to_end")
	plainJoin := g.currentFn.NewBlock("multipart_boundary_plain_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasSemi, Then: semiBB, Else: endBB}
	semiBB.Terminator = &ir.JumpTerm{Target: plainJoin}
	endBB.Terminator = &ir.JumpTerm{Target: plainJoin}
	g.currentBB = plainJoin
	plainEnd := g.currentFn.NewValue("multipart_boundary_plain_end", types.TypeNumber)
	plainJoin.Phis = append(plainJoin.Phis, &ir.PhiInst{Res: plainEnd, Incoming: []ir.PhiIncoming{{Block: semiBB, Value: semi}, {Block: endBB, Value: total}}})
	pBoundary := g.urlStringSlice(contentType, start, plainEnd)
	plainJoin.Terminator = &ir.JumpTerm{Target: join}

	g.currentBB = join
	boundary := g.currentFn.NewValue("multipart_boundary", types.TypeString)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: boundary, Incoming: []ir.PhiIncoming{{Block: qSliceBB, Value: qBoundary}, {Block: plainJoin, Value: pBoundary}}})
	length := g.urlStringLen(boundary)
	nonEmpty := g.currentFn.NewValue("multipart_boundary_nonempty", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: nonEmpty, Op: ir.OpGt, LHS: length, RHS: ir.ConstNumber{Value: 0}})
	ok := g.currentFn.NewBlock("multipart_boundary_ok")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: nonEmpty, Then: ok, Else: fail}
	g.currentBB = ok
	return boundary
}

func (g *generator) multipartRequiredQuotedParam(raw, lower ir.Operand, marker string, fail *ir.BasicBlock) ir.Operand {
	pos := g.urlStringFind(lower, ir.ConstString{Value: marker}, ir.ConstNumber{Value: 0})
	found := g.currentFn.NewValue("multipart_param_found", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: found, Op: ir.OpGe, LHS: pos, RHS: ir.ConstNumber{Value: 0}})
	okBB := g.currentFn.NewBlock("multipart_param_found")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: found, Then: okBB, Else: fail}
	g.currentBB = okBB
	start := g.currentFn.NewValue("multipart_param_start", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: start, Op: ir.OpAdd, LHS: pos, RHS: ir.ConstNumber{Value: float64(len(marker))}})
	end := g.urlStringFindByte(raw, ir.ConstNumber{Value: '"'}, start, g.urlStringLen(raw))
	hasEnd := g.currentFn.NewValue("multipart_param_end_found", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasEnd, Op: ir.OpGe, LHS: end, RHS: ir.ConstNumber{Value: 0}})
	sliceBB := g.currentFn.NewBlock("multipart_param_slice")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasEnd, Then: sliceBB, Else: fail}
	g.currentBB = sliceBB
	return g.urlStringSlice(raw, start, end)
}

func (g *generator) multipartPartContentType(raw, lower ir.Operand) ir.Operand {
	marker := "content-type:"
	pos := g.urlStringFind(lower, ir.ConstString{Value: marker}, ir.ConstNumber{Value: 0})
	found := g.currentFn.NewValue("multipart_part_type_found", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: found, Op: ir.OpGe, LHS: pos, RHS: ir.ConstNumber{Value: 0}})
	foundBB := g.currentFn.NewBlock("multipart_part_type_present")
	missingBB := g.currentFn.NewBlock("multipart_part_type_missing")
	join := g.currentFn.NewBlock("multipart_part_type_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: found, Then: foundBB, Else: missingBB}

	g.currentBB = foundBB
	start0 := g.currentFn.NewValue("multipart_part_type_start0", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: start0, Op: ir.OpAdd, LHS: pos, RHS: ir.ConstNumber{Value: float64(len(marker))}})
	first := g.urlStringByteAt(raw, start0)
	isSpace := g.currentFn.NewValue("multipart_part_type_space", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: isSpace, Op: ir.OpEq, LHS: first, RHS: ir.ConstNumber{Value: ' '}})
	spaceBB := g.currentFn.NewBlock("multipart_part_type_skip_space")
	noSpaceBB := g.currentFn.NewBlock("multipart_part_type_no_space")
	startJoin := g.currentFn.NewBlock("multipart_part_type_start_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isSpace, Then: spaceBB, Else: noSpaceBB}
	start1 := g.currentFn.NewValue("multipart_part_type_start1", types.TypeNumber)
	spaceBB.Instructions = append(spaceBB.Instructions, &ir.BinaryInst{Res: start1, Op: ir.OpAdd, LHS: start0, RHS: ir.ConstNumber{Value: 1}})
	spaceBB.Terminator = &ir.JumpTerm{Target: startJoin}
	noSpaceBB.Terminator = &ir.JumpTerm{Target: startJoin}
	g.currentBB = startJoin
	start := g.currentFn.NewValue("multipart_part_type_start", types.TypeNumber)
	startJoin.Phis = append(startJoin.Phis, &ir.PhiInst{Res: start, Incoming: []ir.PhiIncoming{{Block: spaceBB, Value: start1}, {Block: noSpaceBB, Value: start0}}})
	total := g.urlStringLen(raw)
	lineEnd := g.urlStringFindByte(raw, ir.ConstNumber{Value: '\r'}, start, total)
	hasLineEnd := g.currentFn.NewValue("multipart_part_type_line_end", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasLineEnd, Op: ir.OpGe, LHS: lineEnd, RHS: ir.ConstNumber{Value: 0}})
	lineBB := g.currentFn.NewBlock("multipart_part_type_line")
	endBB := g.currentFn.NewBlock("multipart_part_type_to_end")
	valueJoin := g.currentFn.NewBlock("multipart_part_type_value_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasLineEnd, Then: lineBB, Else: endBB}
	lineBB.Terminator = &ir.JumpTerm{Target: valueJoin}
	endBB.Terminator = &ir.JumpTerm{Target: valueJoin}
	g.currentBB = valueJoin
	end := g.currentFn.NewValue("multipart_part_type_end", types.TypeNumber)
	valueJoin.Phis = append(valueJoin.Phis, &ir.PhiInst{Res: end, Incoming: []ir.PhiIncoming{{Block: lineBB, Value: lineEnd}, {Block: endBB, Value: total}}})
	value := g.urlStringSlice(raw, start, end)
	normalized := g.lowerNormalizeMimeType(value)
	valueEnd := g.currentBB
	valueEnd.Terminator = &ir.JumpTerm{Target: join}

	missingBB.Terminator = &ir.JumpTerm{Target: join}
	g.currentBB = join
	result := g.currentFn.NewValue("multipart_part_type", types.TypeString)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{{Block: valueEnd, Value: normalized}, {Block: missingBB, Value: ir.ConstString{Value: ""}}}})
	return result
}

func (g *generator) lowerBodyMultipartFormData(data, contentType, lowerContentType ir.Operand, fail *ir.BasicBlock) ir.Operand {
	boundary := g.multipartBoundary(contentType, lowerContentType, fail)
	delimiter := g.concatNativeStrings(ir.ConstString{Value: "--"}, boundary)
	boundaryMarker := g.concatNativeStrings(ir.ConstString{Value: "\r\n"}, delimiter)
	delimiterLen := g.urlStringLen(delimiter)

	raw := g.currentFn.NewValue("multipart_raw", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: raw, Callee: "ts_byte_buffer_to_utf8_string", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
	first := g.urlStringFind(raw, delimiter, ir.ConstNumber{Value: 0})
	atStart := g.currentFn.NewValue("multipart_starts_with_boundary", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: atStart, Op: ir.OpEq, LHS: first, RHS: ir.ConstNumber{Value: 0}})
	startBB := g.currentFn.NewBlock("multipart_start")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: atStart, Then: startBB, Else: fail}
	g.currentBB = startBB
	initialCursor := g.currentFn.NewValue("multipart_initial_cursor", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: initialCursor, Op: ir.OpAdd, LHS: first, RHS: delimiterLen})

	fd := g.lowerFormDataNew(nil)
	entries := g.formDataEntries(fd)
	entry := g.currentBB
	loop := g.currentFn.NewBlock("multipart_loop")
	inspect := g.currentFn.NewBlock("multipart_inspect")
	headersBB := g.currentFn.NewBlock("multipart_headers")
	fileBB := g.currentFn.NewBlock("multipart_file_part")
	textBB := g.currentFn.NewBlock("multipart_text_part")
	nextBB := g.currentFn.NewBlock("multipart_next_part")
	done := g.currentFn.NewBlock("multipart_done")
	entry.Terminator = &ir.JumpTerm{Target: loop}

	cursor := g.currentFn.NewValue("multipart_cursor", types.TypeNumber)
	nextCursor := g.currentFn.NewValue("multipart_next_cursor", types.TypeNumber)
	loop.Phis = append(loop.Phis, &ir.PhiInst{Res: cursor, Incoming: []ir.PhiIncoming{{Block: entry, Value: initialCursor}, {Block: nextBB, Value: nextCursor}}})
	b0 := g.urlStringByteAtInBlock(raw, cursor, loop)
	cursor1 := g.currentFn.NewValue("multipart_cursor1", types.TypeNumber)
	loop.Instructions = append(loop.Instructions, &ir.BinaryInst{Res: cursor1, Op: ir.OpAdd, LHS: cursor, RHS: ir.ConstNumber{Value: 1}})
	b1 := g.urlStringByteAtInBlock(raw, cursor1, loop)
	isDash0 := g.currentFn.NewValue("multipart_dash0", types.TypeBoolean)
	isDash1 := g.currentFn.NewValue("multipart_dash1", types.TypeBoolean)
	isFinal := g.currentFn.NewValue("multipart_final", types.TypeBoolean)
	loop.Instructions = append(loop.Instructions,
		&ir.BinaryInst{Res: isDash0, Op: ir.OpEq, LHS: b0, RHS: ir.ConstNumber{Value: '-'}},
		&ir.BinaryInst{Res: isDash1, Op: ir.OpEq, LHS: b1, RHS: ir.ConstNumber{Value: '-'}},
		&ir.BinaryInst{Res: isFinal, Op: ir.OpAnd, LHS: isDash0, RHS: isDash1},
	)
	loop.Terminator = &ir.BranchTerm{Cond: isFinal, Then: done, Else: inspect}

	g.currentBB = inspect
	cr := g.currentFn.NewValue("multipart_cr", types.TypeBoolean)
	lf := g.currentFn.NewValue("multipart_lf", types.TypeBoolean)
	framing := g.currentFn.NewValue("multipart_crlf", types.TypeBoolean)
	inspect.Instructions = append(inspect.Instructions,
		&ir.BinaryInst{Res: cr, Op: ir.OpEq, LHS: b0, RHS: ir.ConstNumber{Value: '\r'}},
		&ir.BinaryInst{Res: lf, Op: ir.OpEq, LHS: b1, RHS: ir.ConstNumber{Value: '\n'}},
		&ir.BinaryInst{Res: framing, Op: ir.OpAnd, LHS: cr, RHS: lf},
	)
	inspect.Terminator = &ir.BranchTerm{Cond: framing, Then: headersBB, Else: fail}

	g.currentBB = headersBB
	headerStart := g.currentFn.NewValue("multipart_header_start", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: headerStart, Op: ir.OpAdd, LHS: cursor, RHS: ir.ConstNumber{Value: 2}})
	headerEnd := g.urlStringFind(raw, ir.ConstString{Value: "\r\n\r\n"}, headerStart)
	hasHeaderEnd := g.currentFn.NewValue("multipart_header_end_found", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasHeaderEnd, Op: ir.OpGe, LHS: headerEnd, RHS: ir.ConstNumber{Value: 0}})
	headerParse := g.currentFn.NewBlock("multipart_header_parse")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasHeaderEnd, Then: headerParse, Else: fail}
	g.currentBB = headerParse
	headerRaw := g.urlStringSlice(raw, headerStart, headerEnd)
	headerLower := g.currentFn.NewValue("multipart_header_lower", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: headerLower, Callee: "ts_string_ascii_lower", Args: []ir.Operand{headerRaw}, ParamTypes: []types.Type{types.TypeString}})
	name := g.multipartRequiredQuotedParam(headerRaw, headerLower, "name=\"", fail)
	bodyStart := g.currentFn.NewValue("multipart_body_start", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: bodyStart, Op: ir.OpAdd, LHS: headerEnd, RHS: ir.ConstNumber{Value: 4}})
	nextBoundary := g.urlStringFind(raw, boundaryMarker, bodyStart)
	hasNextBoundary := g.currentFn.NewValue("multipart_next_boundary_found", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasNextBoundary, Op: ir.OpGe, LHS: nextBoundary, RHS: ir.ConstNumber{Value: 0}})
	partReady := g.currentFn.NewBlock("multipart_part_ready")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasNextBoundary, Then: partReady, Else: fail}
	g.currentBB = partReady
	filenameAt := g.urlStringFind(headerLower, ir.ConstString{Value: "filename=\""}, ir.ConstNumber{Value: 0})
	hasFilename := g.currentFn.NewValue("multipart_has_filename", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasFilename, Op: ir.OpGe, LHS: filenameAt, RHS: ir.ConstNumber{Value: 0}})
	partReady.Terminator = &ir.BranchTerm{Cond: hasFilename, Then: fileBB, Else: textBB}

	g.currentBB = fileBB
	filename := g.multipartRequiredQuotedParam(headerRaw, headerLower, "filename=\"", fail)
	partType := g.multipartPartContentType(headerRaw, headerLower)
	fileSize := g.currentFn.NewValue("multipart_file_size", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: fileSize, Op: ir.OpSub, LHS: nextBoundary, RHS: bodyStart})
	fileData := g.currentFn.NewValue("multipart_file_data", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: fileData, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{data, bodyStart, nextBoundary}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
	now := g.currentFn.NewValue("multipart_file_now", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: now, Callee: "ts_date_now"})
	file := g.newFileObject(fileData, fileSize, partType, filename, now, ir.ConstString{Value: ""})
	g.pushArrayOperand(entries, g.boxJSValue(name, types.TypeString))
	g.pushArrayOperand(entries, g.boxJSValue(file, g.semaResult.FileType))
	fileEnd := g.currentBB
	fileEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = textBB
	textSize := g.currentFn.NewValue("multipart_text_size", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: textSize, Op: ir.OpSub, LHS: nextBoundary, RHS: bodyStart})
	textValue := g.currentFn.NewValue("multipart_text_value", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: textValue, Callee: "ts_utf8_sanitize", Args: []ir.Operand{data, bodyStart, textSize, ir.ConstBool{Value: false}}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber, types.TypeBoolean}})
	g.pushArrayOperand(entries, g.boxJSValue(name, types.TypeString))
	g.pushArrayOperand(entries, g.boxJSValue(textValue, types.TypeString))
	textEnd := g.currentBB
	textEnd.Terminator = &ir.JumpTerm{Target: nextBB}

	g.currentBB = nextBB
	afterCRLF := g.currentFn.NewValue("multipart_after_crlf", types.TypeNumber)
	nextBB.Instructions = append(nextBB.Instructions,
		&ir.BinaryInst{Res: afterCRLF, Op: ir.OpAdd, LHS: nextBoundary, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: nextCursor, Op: ir.OpAdd, LHS: afterCRLF, RHS: delimiterLen},
	)
	nextBB.Terminator = &ir.JumpTerm{Target: loop}

	g.currentBB = done
	return g.makeImmediatePromiseTask(fd, g.semaResult.FormDataType, g.semaResult.FormDataType)
}

func (g *generator) urlStringByteAtInBlock(value, index ir.Operand, bb *ir.BasicBlock) ir.Operand {
	res := g.currentFn.NewValue("multipart_string_byte", types.TypeNumber)
	bb.Instructions = append(bb.Instructions, &ir.CallInst{Res: res, Callee: "ts_string_byte_at", Args: []ir.Operand{value, index}, ParamTypes: []types.Type{types.TypeString, types.TypeNumber}})
	return res
}
