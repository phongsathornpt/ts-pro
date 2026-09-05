package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/frontend/lexer"
	"github.com/phongsathornpt/ts-pro/internal/support/diag"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

// Parser parses tokens into an AST.
type Parser struct {
	file            *source.File
	tokens          []token.Token
	cursor          int
	diagnostics     diag.DiagnosticList
	destructCounter int
}

// New creates a new Parser for the given source File.
func New(file *source.File) *Parser {
	toks, diags := lexer.TokenizeAll(file)
	return &Parser{
		file:        file,
		tokens:      toks,
		cursor:      0,
		diagnostics: diags,
	}
}

func (p *Parser) Diagnostics() diag.DiagnosticList {
	return p.diagnostics
}

func (p *Parser) current() token.Token {
	if p.cursor >= len(p.tokens) {
		return token.Token{Kind: token.EOF}
	}
	return p.tokens[p.cursor]
}

func (p *Parser) advance() token.Token {
	tok := p.current()
	if p.cursor < len(p.tokens) {
		p.cursor++
	}
	return tok
}

func (p *Parser) match(kind token.Kind) bool {
	if p.current().Kind == kind {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) expect(kind token.Kind) token.Token {
	tok := p.current()
	if tok.Kind == kind {
		p.advance()
		return tok
	}
	p.error(tok.Span, fmt.Sprintf("expected %s, got %s", kind, tok.Kind))
	if tok.Kind != token.EOF {
		p.advance()
	}
	return tok
}

func (p *Parser) error(span source.Span, msg string) {
	p.diagnostics = append(p.diagnostics, diag.Diagnostic{
		Span:     span,
		Code:     "TS1005",
		Message:  msg,
		Severity: diag.SeverityError,
	})
}

func (p *Parser) EnsureProgress(start int, context string) {
	if p.cursor != start || p.current().Kind == token.EOF {
		return
	}
	tok := p.current()
	p.error(tok.Span, fmt.Sprintf("parser made no progress while parsing %s at %s", context, tok.Kind))
	p.advance()
}

func (p *Parser) ensureProgress(start int, context string) {
	p.EnsureProgress(start, context)
}

// Parse parses the whole program.
func (p *Parser) Parse() (*ast.Program, diag.DiagnosticList) {
	startPos := p.current().Span.Start
	var stmts []ast.Stmt

	for p.current().Kind != token.EOF {
		loopStart := p.cursor
		stmt := p.parseStatement()
		stmts = append(stmts, stmt)
		p.ensureProgress(loopStart, "top-level statement")
	}

	endPos := p.current().Span.End
	return &ast.Program{
		SourceSpan: source.Span{Start: startPos, End: endPos},
		Statements: stmts,
	}, p.diagnostics
}

func (p *Parser) parseStatement() ast.Stmt {
	p.match(token.KwExport)
	switch p.current().Kind {
	case token.KwImport:
		return p.parseImportDecl()
	case token.KwLet, token.KwConst, token.KwVar:
		return p.parseVarDecl()
	case token.KwFunction:
		return p.parseFunctionDecl()
	case token.KwAsync:
		return p.parseAsyncFunctionDecl()
	case token.KwClass:
		return p.parseClassDecl()
	case token.KwEnum:
		return p.parseEnumDecl()
	case token.KwInterface:
		return p.parseInterfaceDecl()
	case token.KwType:
		return p.parseTypeAliasDecl()
	case token.KwReturn:
		return p.parseReturn()
	case token.KwThrow:
		return p.parseThrow()
	case token.KwTry:
		return p.parseTry()
	case token.KwIf:
		return p.parseIf()
	case token.KwWhile:
		return p.parseWhile()
	case token.KwDo:
		return p.parseDoWhile()
	case token.KwFor:
		return p.parseFor()
	case token.KwSwitch:
		return p.parseSwitch()
	case token.KwBreak:
		return p.parseBreak()
	case token.KwContinue:
		return p.parseContinue()
	case token.LBrace:
		return p.parseBlock()
	default:
		return p.parseExprStatement()
	}
}

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
					expr = &ast.IndexExpr{SourceSpan: bind.span, Target: base, Index: &ast.NumberLit{SourceSpan: bind.span, Value: float64(i), Raw: bind.sourceName}}
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
		fieldTok := p.expect(token.Ident)
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

func (p *Parser) parseBlock() *ast.BlockStmt {
	lbrace := p.expect(token.LBrace)
	var stmts []ast.Stmt
	for p.current().Kind != token.RBrace && p.current().Kind != token.EOF {
		loopStart := p.cursor
		stmt := p.parseStatement()
		stmts = append(stmts, stmt)
		p.ensureProgress(loopStart, "block statement")
	}
	rbrace := p.expect(token.RBrace)
	return &ast.BlockStmt{
		SourceSpan: source.Span{Start: lbrace.Span.Start, End: rbrace.Span.End},
		Statements: stmts,
	}
}

func (p *Parser) parseThrow() *ast.ThrowStmt {
	kw := p.advance()
	value := p.parseExpression()
	p.match(token.Semicolon)
	return &ast.ThrowStmt{SourceSpan: source.Span{Start: kw.Span.Start, End: value.Span().End}, Value: value}
}

func (p *Parser) parseTry() *ast.TryStmt {
	kw := p.advance()
	tryBlock := p.parseBlock()
	stmt := &ast.TryStmt{SourceSpan: source.Span{Start: kw.Span.Start, End: tryBlock.Span().End}, Try: tryBlock}
	if p.match(token.KwCatch) {
		p.expect(token.LParen)
		name := p.expect(token.Ident)
		stmt.CatchName = name.Text
		if p.match(token.Colon) {
			stmt.CatchType = p.parseType()
		}
		p.expect(token.RParen)
		stmt.Catch = p.parseBlock()
		stmt.SourceSpan.End = stmt.Catch.Span().End
	}
	if p.match(token.KwFinally) {
		stmt.Finally = p.parseBlock()
		stmt.SourceSpan.End = stmt.Finally.Span().End
	}
	if stmt.Catch == nil && stmt.Finally == nil {
		p.error(stmt.SourceSpan, "try must have catch or finally")
	}
	return stmt
}

func (p *Parser) parseReturn() *ast.ReturnStmt {
	kw := p.advance()
	var val ast.Expr
	if p.current().Kind != token.Semicolon && p.current().Kind != token.RBrace && p.current().Kind != token.EOF {
		val = p.parseExpression()
	}
	p.match(token.Semicolon)
	end := kw.Span.End
	if val != nil {
		end = val.Span().End
	}
	return &ast.ReturnStmt{
		SourceSpan: source.Span{Start: kw.Span.Start, End: end},
		Value:      val,
	}
}

func (p *Parser) parseIf() *ast.IfStmt {
	kw := p.advance()
	p.expect(token.LParen)
	cond := p.parseExpression()
	p.expect(token.RParen)
	thenStmt := p.parseStatement()
	var elseStmt ast.Stmt
	if p.match(token.KwElse) {
		elseStmt = p.parseStatement()
	}
	end := thenStmt.Span().End
	if elseStmt != nil {
		end = elseStmt.Span().End
	}
	return &ast.IfStmt{
		SourceSpan: source.Span{Start: kw.Span.Start, End: end},
		Cond:       cond,
		Then:       thenStmt,
		Else:       elseStmt,
	}
}

func (p *Parser) parseWhile() *ast.WhileStmt {
	kw := p.advance()
	p.expect(token.LParen)
	cond := p.parseExpression()
	p.expect(token.RParen)
	body := p.parseStatement()
	return &ast.WhileStmt{
		SourceSpan: source.Span{Start: kw.Span.Start, End: body.Span().End},
		Cond:       cond,
		Body:       body,
	}
}

func (p *Parser) parseDoWhile() *ast.DoWhileStmt {
	kw := p.advance()
	body := p.parseStatement()
	p.expect(token.KwWhile)
	p.expect(token.LParen)
	cond := p.parseExpression()
	rparen := p.expect(token.RParen)
	p.match(token.Semicolon)
	return &ast.DoWhileStmt{
		SourceSpan: source.Span{Start: kw.Span.Start, End: rparen.Span.End},
		Body:       body, Cond: cond,
	}
}

func (p *Parser) parseBreak() *ast.BreakStmt {
	tok := p.advance()
	p.match(token.Semicolon)
	return &ast.BreakStmt{SourceSpan: tok.Span}
}

func (p *Parser) parseContinue() *ast.ContinueStmt {
	tok := p.advance()
	p.match(token.Semicolon)
	return &ast.ContinueStmt{SourceSpan: tok.Span}
}

func (p *Parser) parseSwitch() *ast.SwitchStmt {
	kw := p.advance()
	p.expect(token.LParen)
	expr := p.parseExpression()
	p.expect(token.RParen)
	p.expect(token.LBrace)
	var cases []ast.SwitchCase
	for p.current().Kind != token.RBrace && p.current().Kind != token.EOF {
		loopStart := p.cursor
		var test ast.Expr
		start := p.current().Span.Start
		if p.match(token.KwCase) {
			test = p.parseExpression()
			p.expect(token.Colon)
		} else if p.match(token.KwDefault) {
			p.expect(token.Colon)
		} else {
			p.error(p.current().Span, "expected case or default in switch")
			p.advance()
			p.ensureProgress(loopStart, "switch clause")
			continue
		}
		var stmts []ast.Stmt
		for p.current().Kind != token.KwCase && p.current().Kind != token.KwDefault && p.current().Kind != token.RBrace && p.current().Kind != token.EOF {
			stmtStart := p.cursor
			if stmt := p.parseStatement(); stmt != nil {
				stmts = append(stmts, stmt)
			}
			p.ensureProgress(stmtStart, "switch clause statement")
		}
		end := p.current().Span.Start
		cases = append(cases, ast.SwitchCase{SourceSpan: source.Span{Start: start, End: end}, Test: test, Statements: stmts})
		p.ensureProgress(loopStart, "switch clause")
	}
	rbrace := p.expect(token.RBrace)
	return &ast.SwitchStmt{SourceSpan: source.Span{Start: kw.Span.Start, End: rbrace.Span.End}, Expr: expr, Cases: cases}
}

func (p *Parser) parseFor() ast.Stmt {
	kw := p.advance()
	p.expect(token.LParen)

	// Speculatively recognize the TypeScript for-of declaration form.
	declCursor := p.cursor
	declDiags := len(p.diagnostics)
	if p.current().Kind == token.KwLet || p.current().Kind == token.KwConst || p.current().Kind == token.KwVar {
		kind := p.advance().Kind
		if p.current().Kind == token.Ident {
			nameTok := p.advance()
			var typeNode ast.TypeNode
			if p.match(token.Colon) {
				typeNode = p.parseType()
			}
			if p.match(token.KwOf) {
				iterable := p.parseExpression()
				p.expect(token.RParen)
				body := p.parseStatement()
				return &ast.ForOfStmt{
					SourceSpan: source.Span{Start: kw.Span.Start, End: body.Span().End},
					Kind:       kind, Name: nameTok.Text, Type: typeNode, Iterable: iterable, Body: body,
				}
			}
		}
	}
	p.cursor = declCursor
	p.diagnostics = p.diagnostics[:declDiags]

	var init ast.Stmt
	if !p.match(token.Semicolon) {
		init = p.parseStatement()
	}
	var cond ast.Expr
	if p.current().Kind != token.Semicolon {
		cond = p.parseExpression()
	}
	p.expect(token.Semicolon)
	var post ast.Expr
	if p.current().Kind != token.RParen {
		post = p.parseExpression()
	}
	p.expect(token.RParen)
	body := p.parseStatement()
	return &ast.ForStmt{
		SourceSpan: source.Span{Start: kw.Span.Start, End: body.Span().End},
		Init:       init, Cond: cond, Post: post, Body: body,
	}
}

func (p *Parser) parseExprStatement() *ast.ExprStmt {
	expr := p.parseExpression()
	p.match(token.Semicolon)
	return &ast.ExprStmt{
		SourceSpan: expr.Span(),
		Expr:       expr,
	}
}

// --- Expression Parsing (Pratt Parser) ---

func (p *Parser) parseExpression() ast.Expr {
	return p.parseBinary(0)
}

func precedence(op token.Kind) int {
	switch op {
	case token.Eq, token.PlusEq, token.MinusEq, token.StarEq, token.SlashEq:
		return 1
	case token.Question:
		return 2
	case token.QuestionQuestion:
		return 3
	case token.PipePipe:
		return 4
	case token.AmpAmp:
		return 5
	case token.Pipe:
		return 6
	case token.Caret:
		return 7
	case token.Amp:
		return 8
	case token.EqEq, token.EqEqEq, token.BangEq, token.BangEqEq:
		return 9
	case token.Lt, token.LtEq, token.Gt, token.GtEq:
		return 10
	case token.Plus, token.Minus:
		return 11
	case token.Star, token.Slash, token.Percent:
		return 12
	default:
		return 0
	}
}

func (p *Parser) parseBinary(minPrec int) ast.Expr {
	left := p.parseUnary()

	for {
		op := p.current().Kind
		prec := precedence(op)
		if prec <= minPrec {
			break
		}
		p.advance() // consume op

		if op == token.Question {
			thenExpr := p.parseExpression()
			p.expect(token.Colon)
			elseExpr := p.parseBinary(prec - 1)
			left = &ast.TernaryExpr{
				SourceSpan: source.Span{Start: left.Span().Start, End: elseExpr.Span().End},
				Cond:       left,
				Then:       thenExpr,
				Else:       elseExpr,
			}
		} else if op == token.Eq || op == token.PlusEq || op == token.MinusEq || op == token.StarEq || op == token.SlashEq {
			right := p.parseBinary(prec - 1)
			left = &ast.AssignExpr{
				SourceSpan: source.Span{Start: left.Span().Start, End: right.Span().End},
				Left:       left,
				Op:         op,
				Right:      right,
			}
		} else {
			right := p.parseBinary(prec)
			left = &ast.BinaryExpr{
				SourceSpan: source.Span{Start: left.Span().Start, End: right.Span().End},
				Left:       left,
				Op:         op,
				Right:      right,
			}
		}
	}
	return left
}

func (p *Parser) parseUnary() ast.Expr {
	switch p.current().Kind {
	case token.KwAwait:
		tok := p.advance()
		target := p.parseUnary()
		return &ast.AwaitExpr{SourceSpan: source.Span{Start: tok.Span.Start, End: target.Span().End}, Target: target}
	case token.Bang, token.Minus, token.Plus, token.Tilde, token.PlusPlus, token.MinusMinus:
		opTok := p.advance()
		target := p.parseUnary()
		return &ast.UnaryExpr{
			SourceSpan: source.Span{Start: opTok.Span.Start, End: target.Span().End},
			Op:         opTok.Kind,
			Target:     target,
			Prefix:     true,
		}
	default:
		return p.parsePostfix()
	}
}

func (p *Parser) tryParseArrowExpr() (ast.Expr, bool) {
	startCursor := p.cursor
	startDiags := len(p.diagnostics)
	start := p.current().Span.Start

	p.advance() // consume '('
	params := p.parseParams()
	if p.current().Kind != token.RParen {
		p.cursor = startCursor
		p.diagnostics = p.diagnostics[:startDiags]
		return nil, false
	}
	p.advance()

	var retType ast.TypeNode
	if p.match(token.Colon) {
		retType = p.parseType()
	}
	if !p.match(token.Arrow) {
		p.cursor = startCursor
		p.diagnostics = p.diagnostics[:startDiags]
		return nil, false
	}

	var body ast.Node
	isExprBody := true
	if p.current().Kind == token.LBrace {
		body = p.parseBlock()
		isExprBody = false
	} else {
		body = p.parseExpression()
	}
	return &ast.ArrowFuncExpr{
		SourceSpan: source.Span{Start: start, End: body.Span().End},
		Params:     params, ReturnType: retType, Body: body, IsExprBody: isExprBody,
	}, true
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

func (p *Parser) parsePostfix() ast.Expr {
	expr := p.parsePrimary()

	for {
		switch p.current().Kind {
		case token.PlusPlus, token.MinusMinus:
			opTok := p.advance()
			expr = &ast.UnaryExpr{
				SourceSpan: source.Span{Start: expr.Span().Start, End: opTok.Span.End},
				Op:         opTok.Kind,
				Target:     expr,
				Prefix:     false,
			}
		case token.Bang:
			// TypeScript postfix non-null assertion is erased at runtime. Consume it
			// here so member/index/call postfix parsing can continue normally.
			p.advance()
		case token.Lt:
			typeArgs, ok := p.tryParseCallTypeArgs()
			if !ok {
				return expr
			}
			p.advance() // consume '('
			var args []ast.Expr
			for p.current().Kind != token.RParen && p.current().Kind != token.EOF {
				loopStart := p.cursor
				args = append(args, p.parseExpression())
				if !p.match(token.Comma) {
					p.ensureProgress(loopStart, "generic call arguments")
					break
				}
				p.ensureProgress(loopStart, "generic call arguments")
			}
			rparen := p.expect(token.RParen)
			expr = &ast.CallExpr{
				SourceSpan: source.Span{Start: expr.Span().Start, End: rparen.Span.End},
				Callee:     expr,
				TypeArgs:   typeArgs,
				Args:       args,
			}
		case token.LParen:
			// Call
			p.advance()
			var args []ast.Expr
			for p.current().Kind != token.RParen && p.current().Kind != token.EOF {
				loopStart := p.cursor
				args = append(args, p.parseExpression())
				if !p.match(token.Comma) {
					p.ensureProgress(loopStart, "call arguments")
					break
				}
				p.ensureProgress(loopStart, "call arguments")
			}
			rparen := p.expect(token.RParen)
			expr = &ast.CallExpr{
				SourceSpan: source.Span{Start: expr.Span().Start, End: rparen.Span.End},
				Callee:     expr,
				Args:       args,
			}
		case token.Dot:
			p.advance()
			propTok := p.expect(token.Ident)
			expr = &ast.MemberExpr{
				SourceSpan: source.Span{Start: expr.Span().Start, End: propTok.Span.End},
				Object:     expr,
				Property:   propTok.Text,
				Computed:   false,
			}
		case token.QuestionDot:
			p.advance()
			propTok := p.expect(token.Ident)
			expr = &ast.MemberExpr{
				SourceSpan: source.Span{Start: expr.Span().Start, End: propTok.Span.End},
				Object:     expr,
				Property:   propTok.Text,
				Computed:   false,
				Optional:   true,
			}
		case token.LBracket:
			p.advance()
			idx := p.parseExpression()
			rbracket := p.expect(token.RBracket)
			expr = &ast.IndexExpr{
				SourceSpan: source.Span{Start: expr.Span().Start, End: rbracket.Span.End},
				Target:     expr,
				Index:      idx,
			}
		default:
			return expr
		}
	}
}

func (p *Parser) parseTemplateLiteral(tok token.Token) ast.Expr {
	text := tok.Text
	if !strings.Contains(text, "${") {
		return &ast.StringLit{SourceSpan: tok.Span, Value: text}
	}
	parts := []ast.Expr{&ast.StringLit{SourceSpan: tok.Span, Value: ""}}
	for pos := 0; pos < len(text); {
		rel := strings.Index(text[pos:], "${")
		if rel < 0 {
			if pos < len(text) {
				parts = append(parts, &ast.StringLit{SourceSpan: tok.Span, Value: text[pos:]})
			}
			break
		}
		start := pos + rel
		if start > pos {
			parts = append(parts, &ast.StringLit{SourceSpan: tok.Span, Value: text[pos:start]})
		}
		i := start + 2
		depth := 1
		quote := byte(0)
		for i < len(text) && depth > 0 {
			ch := text[i]
			if quote != 0 {
				if ch == '\\' {
					i += 2
					continue
				}
				if ch == quote {
					quote = 0
				}
				i++
				continue
			}
			if ch == '\'' || ch == '"' || ch == '`' {
				quote = ch
				i++
				continue
			}
			switch ch {
			case '{':
				depth++
			case '}':
				depth--
			}
			i++
		}
		if depth != 0 {
			p.error(tok.Span, "unterminated template interpolation")
			return &ast.StringLit{SourceSpan: tok.Span, Value: text}
		}
		exprText := strings.TrimSpace(text[start+2 : i-1])
		fs := source.NewFileSet()
		file := fs.AddFile("<template-expression>", []byte(exprText))
		nested := New(file)
		expr := nested.parseExpression()
		if nested.diagnostics.HasErrors() || nested.current().Kind != token.EOF {
			p.error(tok.Span, "invalid template interpolation expression")
			return &ast.StringLit{SourceSpan: tok.Span, Value: text}
		}
		parts = append(parts, expr)
		pos = i
	}
	result := parts[0]
	for _, part := range parts[1:] {
		result = &ast.BinaryExpr{SourceSpan: tok.Span, Left: result, Op: token.Plus, Right: part}
	}
	return result
}

func (p *Parser) parseRegexLiteral(tok token.Token) ast.Expr {
	start := int(tok.Span.Start - p.file.Base)
	src := p.file.Src
	i := start + 1
	escaped, inClass := false, false
	endSlash := -1
	for i < len(src) {
		ch := src[i]
		if escaped {
			escaped = false
			i++
			continue
		}
		if ch == '\\' {
			escaped = true
			i++
			continue
		}
		if ch == '[' {
			inClass = true
			i++
			continue
		}
		if ch == ']' {
			inClass = false
			i++
			continue
		}
		if ch == '/' && !inClass {
			endSlash = i
			break
		}
		if ch == '\n' || ch == '\r' {
			break
		}
		i++
	}
	if endSlash < 0 {
		p.error(tok.Span, "unterminated regular expression literal")
		p.advance()
		return &ast.RegexLit{SourceSpan: tok.Span}
	}
	j := endSlash + 1
	for j < len(src) && ((src[j] >= 'a' && src[j] <= 'z') || (src[j] >= 'A' && src[j] <= 'Z')) {
		j++
	}
	endPos := p.file.Base + source.Pos(j)
	for p.current().Kind != token.EOF && p.current().Span.Start < endPos {
		p.advance()
	}
	return &ast.RegexLit{
		SourceSpan: source.Span{Start: tok.Span.Start, End: endPos},
		Pattern:    string(src[start+1 : endSlash]), Flags: string(src[endSlash+1 : j]),
	}
}

func (p *Parser) parsePrimary() ast.Expr {
	tok := p.current()

	switch tok.Kind {
	case token.Ident:
		p.advance()
		return &ast.IdentExpr{SourceSpan: tok.Span, Name: tok.Text}
	case token.Number:
		p.advance()
		val, _ := strconv.ParseFloat(tok.Text, 64)
		return &ast.NumberLit{SourceSpan: tok.Span, Value: val, Raw: tok.Text}
	case token.String:
		p.advance()
		return &ast.StringLit{SourceSpan: tok.Span, Value: tok.Text}
	case token.Slash:
		return p.parseRegexLiteral(tok)
	case token.TemplateNoSubst:
		p.advance()
		return p.parseTemplateLiteral(tok)
	case token.KwTrue:
		p.advance()
		return &ast.BoolLit{SourceSpan: tok.Span, Value: true}
	case token.KwFalse:
		p.advance()
		return &ast.BoolLit{SourceSpan: tok.Span, Value: false}
	case token.KwNull:
		p.advance()
		return &ast.NullLit{SourceSpan: tok.Span}
	case token.KwUndefined:
		p.advance()
		return &ast.UndefinedLit{SourceSpan: tok.Span}
	case token.KwFunction:
		start := p.advance().Span.Start
		name := ""
		if p.current().Kind == token.Ident {
			name = p.advance().Text
		}
		p.expect(token.LParen)
		params := p.parseParams()
		p.expect(token.RParen)
		var retType ast.TypeNode
		if p.match(token.Colon) {
			retType = p.parseType()
		}
		body := p.parseBlock()
		return &ast.FunctionExpr{SourceSpan: source.Span{Start: start, End: body.Span().End}, Name: name, Params: params, ReturnType: retType, Body: body}
	case token.KwThis:
		p.advance()
		return &ast.ThisExpr{SourceSpan: tok.Span}
	case token.KwSuper:
		p.advance()
		return &ast.SuperExpr{SourceSpan: tok.Span}
	case token.KwNew:
		start := p.advance().Span.Start
		classTok := p.expect(token.Ident)
		var typeArgs []ast.TypeNode
		if p.match(token.Lt) {
			typeArgs = p.parseTypeArgsAfterLt()
		}
		p.expect(token.LParen)
		var args []ast.Expr
		for p.current().Kind != token.RParen && p.current().Kind != token.EOF {
			loopStart := p.cursor
			args = append(args, p.parseExpression())
			if !p.match(token.Comma) {
				p.ensureProgress(loopStart, "new arguments")
				break
			}
			p.ensureProgress(loopStart, "new arguments")
		}
		rparen := p.expect(token.RParen)
		return &ast.NewExpr{SourceSpan: source.Span{Start: start, End: rparen.Span.End}, ClassName: classTok.Text, TypeArgs: typeArgs, Args: args}
	case token.LParen:
		if arrow, ok := p.tryParseArrowExpr(); ok {
			return arrow
		}
		p.advance()
		expr := p.parseExpression()
		p.expect(token.RParen)
		return expr
	case token.LBracket:
		// Array literal
		p.advance()
		var elements []ast.Expr
		for p.current().Kind != token.RBracket && p.current().Kind != token.EOF {
			loopStart := p.cursor
			if p.current().Kind == token.DotDotDot {
				start := p.advance().Span.Start
				value := p.parseExpression()
				elements = append(elements, &ast.SpreadExpr{SourceSpan: source.Span{Start: start, End: value.Span().End}, Value: value})
			} else {
				elements = append(elements, p.parseExpression())
			}
			if !p.match(token.Comma) {
				p.ensureProgress(loopStart, "array literal")
				break
			}
			p.ensureProgress(loopStart, "array literal")
		}
		rbracket := p.expect(token.RBracket)
		return &ast.ArrayLit{
			SourceSpan: source.Span{Start: tok.Span.Start, End: rbracket.Span.End},
			Elements:   elements,
		}
	case token.LBrace:
		// Object literal
		p.advance()
		var props []ast.PropertyAssignment
		for p.current().Kind != token.RBrace && p.current().Kind != token.EOF {
			loopStart := p.cursor
			if p.current().Kind == token.DotDotDot {
				start := p.advance().Span.Start
				val := p.parseExpression()
				props = append(props, ast.PropertyAssignment{
					SourceSpan: source.Span{Start: start, End: val.Span().End},
					Value:      val, Spread: true,
				})
			} else {
				var keyTok token.Token
				if p.current().Kind == token.Ident || p.current().Kind == token.String {
					keyTok = p.advance()
				} else {
					keyTok = p.expect(token.Ident)
				}
				p.expect(token.Colon)
				val := p.parseExpression()
				props = append(props, ast.PropertyAssignment{
					SourceSpan: source.Span{Start: keyTok.Span.Start, End: val.Span().End},
					Key:        keyTok.Text,
					Value:      val,
				})
			}
			if !p.match(token.Comma) {
				p.ensureProgress(loopStart, "object literal")
				break
			}
			p.ensureProgress(loopStart, "object literal")
		}
		rbrace := p.expect(token.RBrace)
		return &ast.ObjectLit{
			SourceSpan: source.Span{Start: tok.Span.Start, End: rbrace.Span.End},
			Properties: props,
		}
	default:
		p.error(tok.Span, fmt.Sprintf("unexpected token %s", tok.Kind))
		p.advance()
		return &ast.IdentExpr{SourceSpan: tok.Span, Name: "<error>"}
	}
}

// --- Type Parsing ---

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
			fieldTok := p.expect(token.Ident)
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
