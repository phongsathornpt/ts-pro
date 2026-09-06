package parser

import (
	"fmt"

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

// --- Type Parsing ---
