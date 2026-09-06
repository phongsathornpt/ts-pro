package parser

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func (p *Parser) parseTypeParams() []string {
	if !p.match(token.Lt) {
		return nil
	}
	var params []string
	for p.current().Kind != token.Gt && p.current().Kind != token.EOF {
		start := p.cursor
		name := p.expect(token.Ident)
		if name.Kind == token.Ident {
			params = append(params, name.Text)
		}
		if !p.match(token.Comma) {
			p.ensureProgress(start, "type parameter list")
			break
		}
		p.ensureProgress(start, "type parameter list")
	}
	p.expect(token.Gt)
	return params
}

func (p *Parser) parseTypeArgsAfterLt() []ast.TypeNode {
	var args []ast.TypeNode
	for p.current().Kind != token.Gt && p.current().Kind != token.EOF {
		start := p.cursor
		args = append(args, p.parseType())
		if !p.match(token.Comma) {
			p.ensureProgress(start, "type argument list")
			break
		}
		p.ensureProgress(start, "type argument list")
	}
	p.expect(token.Gt)
	return args
}

func (p *Parser) tryParseCallTypeArgs() ([]ast.TypeNode, bool) {
	savedCursor := p.cursor
	savedDiagLen := len(p.diagnostics)
	p.advance()
	args := p.parseTypeArgsAfterLt()
	if p.current().Kind != token.LParen || len(args) == 0 {
		p.cursor = savedCursor
		p.diagnostics = p.diagnostics[:savedDiagLen]
		return nil, false
	}
	return args, true
}

func (p *Parser) tryParseFunctionType() (ast.TypeNode, bool) {
	startCursor := p.cursor
	startDiags := len(p.diagnostics)
	start := p.current().Span.Start
	p.advance() // consume '('
	// A nested '(' immediately after the opening paren means this is type
	// grouping such as (() => number), not a function parameter list.
	if p.current().Kind == token.LParen {
		p.cursor = startCursor
		p.diagnostics = p.diagnostics[:startDiags]
		return nil, false
	}
	params := p.parseParams()
	if p.current().Kind != token.RParen {
		p.cursor = startCursor
		p.diagnostics = p.diagnostics[:startDiags]
		return nil, false
	}
	p.advance()
	if !p.match(token.Arrow) {
		p.cursor = startCursor
		p.diagnostics = p.diagnostics[:startDiags]
		return nil, false
	}
	ret := p.parseType()
	return &ast.FunctionTypeNode{
		SourceSpan: source.Span{Start: start, End: ret.Span().End},
		Params:     params, ReturnType: ret,
	}, true
}

func (p *Parser) parseType() ast.TypeNode {
	base := p.parsePrimaryType()
	if p.match(token.Pipe) {
		types := []ast.TypeNode{base, p.parseType()}
		return &ast.UnionTypeNode{
			SourceSpan: source.Span{Start: base.Span().Start, End: types[len(types)-1].Span().End},
			Types:      types,
		}
	}
	return base
}

func (p *Parser) parsePrimaryType() ast.TypeNode {
	tok := p.current()
	var node ast.TypeNode
	switch tok.Kind {
	case token.Ident:
		p.advance()
		switch tok.Text {
		case "number", "string", "boolean", "void", "any", "never", "unknown":
			node = &ast.PrimitiveTypeNode{SourceSpan: tok.Span, Kind: tok.Text}
		default:
			ref := &ast.TypeRefNode{SourceSpan: tok.Span, Name: tok.Text}
			if p.match(token.Lt) {
				ref.TypeArgs = p.parseTypeArgsAfterLt()
				if len(ref.TypeArgs) > 0 {
					ref.SourceSpan.End = ref.TypeArgs[len(ref.TypeArgs)-1].Span().End
				}
			}
			node = ref
		}
	case token.KwUndefined:
		p.advance()
		node = &ast.PrimitiveTypeNode{SourceSpan: tok.Span, Kind: "undefined"}
	case token.KwNull:
		p.advance()
		node = &ast.PrimitiveTypeNode{SourceSpan: tok.Span, Kind: "null"}
	case token.LBracket:
		lbracket := p.advance()
		var elems []ast.TypeNode
		for p.current().Kind != token.RBracket && p.current().Kind != token.EOF {
			loopStart := p.cursor
			elems = append(elems, p.parseType())
			if !p.match(token.Comma) {
				p.ensureProgress(loopStart, "tuple type")
				break
			}
			p.ensureProgress(loopStart, "tuple type")
		}
		rbracket := p.expect(token.RBracket)
		node = &ast.TupleTypeNode{SourceSpan: source.Span{Start: lbracket.Span.Start, End: rbracket.Span.End}, Elements: elems}
	case token.LParen:
		if fnType, ok := p.tryParseFunctionType(); ok {
			node = fnType
		} else {
			p.advance()
			inner := p.parseType()
			p.expect(token.RParen)
			// Parentheses are type-level grouping only; retain the inner node while
			// allowing suffixes such as (number | string)[].
			node = inner
		}
	case token.LBrace:
		lbrace := p.advance()
		var fields []ast.InterfaceField
		for p.current().Kind != token.RBrace && p.current().Kind != token.EOF {
			loopStart := p.cursor
			fieldTok := p.expectIdentifierName()
			optional := p.match(token.Question)
			p.expect(token.Colon)
			fieldType := p.parseType()
			p.match(token.Semicolon)
			p.match(token.Comma)
			fields = append(fields, ast.InterfaceField{SourceSpan: fieldTok.Span, Name: fieldTok.Text, Type: fieldType, Optional: optional})
			p.ensureProgress(loopStart, "object type field")
		}
		rbrace := p.expect(token.RBrace)
		node = &ast.ObjectTypeNode{SourceSpan: source.Span{Start: lbrace.Span.Start, End: rbrace.Span.End}, Fields: fields}
	default:
		p.error(tok.Span, fmt.Sprintf("expected type annotation, got %s", tok.Kind))
		p.advance()
		return &ast.PrimitiveTypeNode{SourceSpan: tok.Span, Kind: "any"}
	}
	for p.match(token.LBracket) {
		rbracket := p.expect(token.RBracket)
		node = &ast.ArrayTypeNode{
			SourceSpan: source.Span{Start: node.Span().Start, End: rbracket.Span.End},
			ElemType:   node,
		}
	}
	return node
}
