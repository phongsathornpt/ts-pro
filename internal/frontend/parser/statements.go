package parser

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

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
