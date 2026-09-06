package parser

import (
	"fmt"
	"strconv"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func (p *Parser) parseImportDecl() *ast.ImportDecl {
	kw := p.advance()
	p.expect(token.LBrace)
	var specs []ast.ImportSpecifier
	for p.current().Kind != token.RBrace && p.current().Kind != token.EOF {
		start := p.cursor
		imported := p.expect(token.Ident)
		local := imported.Text
		if p.match(token.KwAs) {
			local = p.expect(token.Ident).Text
		}
		specs = append(specs, ast.ImportSpecifier{Imported: imported.Text, Local: local})
		if !p.match(token.Comma) {
			p.ensureProgress(start, "import specifier list")
			break
		}
		p.ensureProgress(start, "import specifier list")
	}
	p.expect(token.RBrace)
	p.expect(token.KwFrom)
	module := p.expect(token.String)
	p.match(token.Semicolon)
	return &ast.ImportDecl{
		SourceSpan: source.Span{Start: kw.Span.Start, End: module.Span.End},
		Module:     module.Text, Specifiers: specs,
	}
}

func (p *Parser) parseVarDecl() *ast.VarDeclStmt {
	kw := p.advance()
	start := kw.Span.Start
	var decls []ast.VarDeclarator
	for {
		if p.current().Kind == token.LBracket || p.current().Kind == token.LBrace {
			patternStart := p.current().Span.Start
			arrayPattern := p.match(token.LBracket)
			type binding struct {
				sourceName, targetName string
				span                   source.Span
			}
			var bindings []binding
			if arrayPattern {
				index := 0
				for p.current().Kind != token.RBracket && p.current().Kind != token.EOF {
					if p.match(token.Comma) {
						index++
						continue
					}
					name := p.expect(token.Ident)
					bindings = append(bindings, binding{sourceName: strconv.Itoa(index), targetName: name.Text, span: name.Span})
					index++
					if !p.match(token.Comma) {
						break
					}
				}
				p.expect(token.RBracket)
			} else {
				p.expect(token.LBrace)
				for p.current().Kind != token.RBrace && p.current().Kind != token.EOF {
					prop := p.expect(token.Ident)
					target := prop.Text
					if p.match(token.Colon) {
						target = p.expect(token.Ident).Text
					}
					bindings = append(bindings, binding{sourceName: prop.Text, targetName: target, span: prop.Span})
					if !p.match(token.Comma) {
						break
					}
				}
				p.expect(token.RBrace)
			}
			p.expect(token.Eq)
			init := p.parseExpression()
			tmp := fmt.Sprintf("__tspro_destruct_%d", p.destructCounter)
			p.destructCounter++
			decls = append(decls, ast.VarDeclarator{SourceSpan: source.Span{Start: patternStart, End: init.Span().End}, Name: tmp, Init: init})
			for _, bind := range bindings {
				base := &ast.IdentExpr{SourceSpan: bind.span, Name: tmp}
				var expr ast.Expr
				if arrayPattern {
					i, _ := strconv.Atoi(bind.sourceName)
					expr = &ast.IndexExpr{SourceSpan: bind.span, Target: base, Index: &ast.NumberLit{SourceSpan: bind.span, Value: float64(i)}}
				} else {
					expr = &ast.MemberExpr{SourceSpan: bind.span, Object: base, Property: bind.sourceName}
				}
				decls = append(decls, ast.VarDeclarator{SourceSpan: bind.span, Name: bind.targetName, Init: expr})
			}
		} else {
			nameTok := p.expect(token.Ident)
			var typeNode ast.TypeNode
			if p.match(token.Colon) {
				typeNode = p.parseType()
			}
			var init ast.Expr
			if p.match(token.Eq) {
				init = p.parseExpression()
			}
			end := nameTok.Span.End
			if init != nil {
				end = init.Span().End
			}
			decls = append(decls, ast.VarDeclarator{SourceSpan: source.Span{Start: nameTok.Span.Start, End: end}, Name: nameTok.Text, Type: typeNode, Init: init})
		}
		if !p.match(token.Comma) {
			break
		}
	}
	p.match(token.Semicolon)
	return &ast.VarDeclStmt{SourceSpan: source.Span{Start: start, End: p.current().Span.Start}, Kind: kw.Kind, Declarations: decls}
}

func (p *Parser) parseAsyncFunctionDecl() *ast.FunctionDecl {
	asyncTok := p.advance()
	if p.current().Kind != token.KwFunction {
		p.error(asyncTok.Span, "async is currently supported only on function declarations")
		return p.parseFunctionDecl()
	}
	fn := p.parseFunctionDecl()
	fn.IsAsync = true
	fn.SourceSpan.Start = asyncTok.Span.Start
	return fn
}

func (p *Parser) parseFunctionDecl() *ast.FunctionDecl {
	kw := p.advance() // consume 'function'
	nameTok := p.expect(token.Ident)
	typeParams := p.parseTypeParams()

	p.expect(token.LParen)
	params := p.parseParams()
	p.expect(token.RParen)

	var retType ast.TypeNode
	if p.match(token.Colon) {
		retType = p.parseType()
	}

	body := p.parseBlock()
	return &ast.FunctionDecl{
		SourceSpan: source.Span{Start: kw.Span.Start, End: body.Span().End},
		Name:       nameTok.Text,
		TypeParams: typeParams,
		Params:     params,
		ReturnType: retType,
		Body:       body,
	}
}

func (p *Parser) parseParams() []ast.Param {
	var params []ast.Param
	for p.current().Kind != token.RParen && p.current().Kind != token.EOF {
		loopStart := p.cursor
		start := p.current().Span.Start
		visibility := ""
		readonly := false
		for p.current().Kind == token.Ident {
			switch p.current().Text {
			case "public", "private", "protected":
				visibility = p.advance().Text
			case "readonly":
				readonly = true
				p.advance()
			default:
				goto paramModifiersDone
			}
		}
	paramModifiersDone:
		rest := p.match(token.DotDotDot)
		isThis := false
		var paramTok token.Token
		if p.current().Kind == token.KwThis {
			paramTok = p.advance()
			isThis = true
		} else {
			paramTok = p.expect(token.Ident)
		}
		optional := p.match(token.Question)
		var typeNode ast.TypeNode
		if p.match(token.Colon) {
			typeNode = p.parseType()
		}
		var defExpr ast.Expr
		if p.match(token.Eq) {
			defExpr = p.parseExpression()
		}
		params = append(params, ast.Param{
			SourceSpan: source.Span{Start: start, End: paramTok.Span.End},
			Name:       paramTok.Text, Type: typeNode, Optional: optional, Rest: rest, IsThis: isThis, Default: defExpr,
			Visibility: visibility, Readonly: readonly, IsParameterProperty: visibility != "" || readonly,
		})
		if !p.match(token.Comma) {
			p.ensureProgress(loopStart, "parameter list")
			break
		}
		p.ensureProgress(loopStart, "parameter list")
	}
	return params
}

func (p *Parser) parseEnumDecl() *ast.EnumDecl {
	kw := p.advance()
	name := p.expect(token.Ident)
	p.expect(token.LBrace)
	var members []ast.EnumMember
	for p.current().Kind != token.RBrace && p.current().Kind != token.EOF {
		start := p.cursor
		member := p.expect(token.Ident)
		var value ast.Expr
		if p.match(token.Eq) {
			value = p.parseExpression()
		}
		members = append(members, ast.EnumMember{SourceSpan: member.Span, Name: member.Text, Value: value})
		p.match(token.Comma)
		p.ensureProgress(start, "enum member")
	}
	rbrace := p.expect(token.RBrace)
	return &ast.EnumDecl{SourceSpan: source.Span{Start: kw.Span.Start, End: rbrace.Span.End}, Name: name.Text, Members: members}
}

func (p *Parser) parseClassDecl() *ast.ClassDecl {
	kw := p.advance()
	nameTok := p.expect(token.Ident)
	typeParams := p.parseTypeParams()

	extends := ""
	if p.match(token.KwExtends) {
		extTok := p.expect(token.Ident)
		extends = extTok.Text
	}
	var implements []ast.TypeNode
	if p.match(token.KwImplements) {
		for {
			implements = append(implements, p.parseType())
			if !p.match(token.Comma) {
				break
			}
		}
	}

	p.expect(token.LBrace)
	var fields []ast.ClassField
	var methods []ast.ClassMethod

	for p.current().Kind != token.RBrace && p.current().Kind != token.EOF {
		loopStart := p.cursor
		isStatic := false
		isOverride := false
		visibility := ""
		readonly := false
		for p.current().Kind == token.Ident {
			switch p.current().Text {
			case "static":
				isStatic = true
				p.advance()
			case "override":
				isOverride = true
				p.advance()
			case "public", "private", "protected":
				visibility = p.advance().Text
			case "readonly":
				readonly = true
				p.advance()
			default:
				goto classModifiersDone
			}
		}
	classModifiersDone:
		memberTok := p.expect(token.Ident)
		if p.match(token.LParen) {
			// Method
			params := p.parseParams()
			p.expect(token.RParen)
			var retType ast.TypeNode
			if p.match(token.Colon) {
				retType = p.parseType()
			}
			body := p.parseBlock()
			methods = append(methods, ast.ClassMethod{
				SourceSpan: source.Span{Start: memberTok.Span.Start, End: body.Span().End},
				Name:       memberTok.Text,
				Params:     params,
				ReturnType: retType,
				Body:       body,
				IsStatic:   isStatic, IsOverride: isOverride, Visibility: visibility,
			})
		} else {
			// Field
			var typeNode ast.TypeNode
			if p.match(token.Colon) {
				typeNode = p.parseType()
			}
			var init ast.Expr
			if p.match(token.Eq) {
				init = p.parseExpression()
			}
			p.match(token.Semicolon)
			fields = append(fields, ast.ClassField{
				SourceSpan: memberTok.Span,
				Name:       memberTok.Text,
				Type:       typeNode,
				Init:       init, IsStatic: isStatic, Visibility: visibility, Readonly: readonly,
			})
		}
		p.ensureProgress(loopStart, "class member")
	}
	rbrace := p.expect(token.RBrace)

	return &ast.ClassDecl{
		SourceSpan: source.Span{Start: kw.Span.Start, End: rbrace.Span.End},
		Name:       nameTok.Text,
		TypeParams: typeParams,
		Extends:    extends,
		Implements: implements,
		Fields:     fields,
		Methods:    methods,
	}
}

func (p *Parser) parseInterfaceDecl() *ast.InterfaceDecl {
	kw := p.advance()
	nameTok := p.expect(token.Ident)
	typeParams := p.parseTypeParams()

	p.expect(token.LBrace)
	var fields []ast.InterfaceField
	for p.current().Kind != token.RBrace && p.current().Kind != token.EOF {
		loopStart := p.cursor
		fieldTok := p.expectIdentifierName()
		optional := p.match(token.Question)
		p.expect(token.Colon)
		t := p.parseType()
		p.match(token.Semicolon)
		fields = append(fields, ast.InterfaceField{
			SourceSpan: fieldTok.Span,
			Name:       fieldTok.Text,
			Type:       t,
			Optional:   optional,
		})
		p.ensureProgress(loopStart, "interface field")
	}
	rbrace := p.expect(token.RBrace)
	return &ast.InterfaceDecl{
		SourceSpan: source.Span{Start: kw.Span.Start, End: rbrace.Span.End},
		Name:       nameTok.Text,
		TypeParams: typeParams,
		Fields:     fields,
	}
}

func (p *Parser) parseTypeAliasDecl() *ast.TypeAliasDecl {
	kw := p.advance()
	nameTok := p.expect(token.Ident)
	typeParams := p.parseTypeParams()
	p.expect(token.Eq)
	t := p.parseType()
	p.match(token.Semicolon)
	return &ast.TypeAliasDecl{
		SourceSpan: source.Span{Start: kw.Span.Start, End: t.Span().End},
		Name:       nameTok.Text,
		TypeParams: typeParams,
		Type:       t,
	}
}
