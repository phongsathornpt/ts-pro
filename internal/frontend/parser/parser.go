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

func (p *Parser) expectIdentifierName() token.Token {
	tok := p.current()
	if tok.Kind == token.Ident || tok.Kind.IsKeyword() {
		p.advance()
		return tok
	}
	p.error(tok.Span, fmt.Sprintf("expected identifier name, got %s", tok.Kind))
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
			propTok := p.expectIdentifierName()
			expr = &ast.MemberExpr{
				SourceSpan: source.Span{Start: expr.Span().Start, End: propTok.Span.End},
				Object:     expr,
				Property:   propTok.Text,
			}
		case token.QuestionDot:
			p.advance()
			propTok := p.expectIdentifierName()
			expr = &ast.MemberExpr{
				SourceSpan: source.Span{Start: expr.Span().Start, End: propTok.Span.End},
				Object:     expr,
				Property:   propTok.Text,
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
		return &ast.NumberLit{SourceSpan: tok.Span, Value: val}
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
				if p.current().Kind == token.Ident || p.current().Kind == token.String || p.current().Kind.IsKeyword() {
					keyTok = p.advance()
				} else {
					keyTok = p.expectIdentifierName()
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
