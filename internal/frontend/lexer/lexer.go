package lexer

import (
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/support/diag"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

// Lexer scans UTF-8 TypeScript source into tokens.
type Lexer struct {
	file        *source.File
	src         []byte
	offset      int  // byte offset in file.Src
	readOffset  int  // reading offset (position after current rune)
	ch          rune // current character
	diagnostics diag.DiagnosticList
}

// New creates a new Lexer for the given source File.
func New(file *source.File) *Lexer {
	l := &Lexer{
		file: file,
		src:  file.Src,
	}
	l.nextChar()
	return l
}

func (l *Lexer) Diagnostics() diag.DiagnosticList {
	return l.diagnostics
}

func (l *Lexer) nextChar() {
	if l.readOffset >= len(l.src) {
		l.offset = len(l.src)
		l.ch = -1 // EOF
		return
	}
	l.offset = l.readOffset
	r, size := rune(l.src[l.readOffset]), 1
	if r >= utf8.RuneSelf {
		r, size = utf8.DecodeRune(l.src[l.readOffset:])
	}
	l.readOffset += size
	l.ch = r
}

func (l *Lexer) peek() rune {
	if l.readOffset >= len(l.src) {
		return -1
	}
	r := rune(l.src[l.readOffset])
	if r >= utf8.RuneSelf {
		r, _ = utf8.DecodeRune(l.src[l.readOffset:])
	}
	return r
}

func (l *Lexer) currentPos() source.Pos {
	return l.file.Base + source.Pos(l.offset)
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' || l.ch == '\n' {
		l.nextChar()
	}
}

// Next returns the next token in the source stream.
func (l *Lexer) Next() token.Token {
	l.skipWhitespace()

	startPos := l.currentPos()

	if l.ch == -1 {
		return token.Token{
			Kind: token.EOF,
			Span: source.Span{Start: startPos, End: startPos},
		}
	}

	ch := l.ch

	// Comments
	if ch == '/' {
		if l.peek() == '/' {
			start := l.offset
			l.nextChar() // consume '/'
			l.nextChar() // consume '/'
			for l.ch != '\n' && l.ch != -1 {
				l.nextChar()
			}
			return token.Token{
				Kind: token.Comment,
				Span: source.Span{Start: startPos, End: l.currentPos()},
				Text: string(l.src[start:l.offset]),
			}
		} else if l.peek() == '*' {
			start := l.offset
			l.nextChar() // consume '/'
			l.nextChar() // consume '*'
			for {
				if l.ch == -1 {
					l.diagnostics = append(l.diagnostics, diag.Diagnostic{
						Span:     source.Span{Start: startPos, End: l.currentPos()},
						Code:     "TS1002",
						Message:  "Unterminated multi-line comment.",
						Severity: diag.SeverityError,
					})
					break
				}
				if l.ch == '*' && l.peek() == '/' {
					l.nextChar()
					l.nextChar()
					break
				}
				l.nextChar()
			}
			return token.Token{
				Kind: token.Comment,
				Span: source.Span{Start: startPos, End: l.currentPos()},
				Text: string(l.src[start:l.offset]),
			}
		}
	}

	// Identifiers and Keywords
	if isIdentStart(ch) {
		start := l.offset
		for isIdentPart(l.ch) {
			l.nextChar()
		}
		text := string(l.src[start:l.offset])
		kind := token.Lookup(text)
		return token.Token{
			Kind: kind,
			Span: source.Span{Start: startPos, End: l.currentPos()},
			Text: text,
		}
	}

	// Numeric literals
	if unicode.IsDigit(ch) || (ch == '.' && unicode.IsDigit(l.peek())) {
		return l.scanNumber(startPos)
	}

	// String literals
	if ch == '"' || ch == '\'' {
		return l.scanString(startPos, ch)
	}

	// Template literal (no substitution for simple backticks)
	if ch == '`' {
		return l.scanTemplate(startPos)
	}

	// Operators and Delimiters
	l.nextChar()
	endPos := l.currentPos()
	span := source.Span{Start: startPos, End: endPos}

	switch ch {
	case '+':
		if l.ch == '+' {
			l.nextChar()
			return token.Token{Kind: token.PlusPlus, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		if l.ch == '=' {
			l.nextChar()
			return token.Token{Kind: token.PlusEq, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Plus, Span: span}
	case '-':
		if l.ch == '-' {
			l.nextChar()
			return token.Token{Kind: token.MinusMinus, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		if l.ch == '=' {
			l.nextChar()
			return token.Token{Kind: token.MinusEq, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Minus, Span: span}
	case '*':
		if l.ch == '*' {
			l.nextChar()
			return token.Token{Kind: token.StarStar, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		if l.ch == '=' {
			l.nextChar()
			return token.Token{Kind: token.StarEq, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Star, Span: span}
	case '/':
		if l.ch == '=' {
			l.nextChar()
			return token.Token{Kind: token.SlashEq, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Slash, Span: span}
	case '%':
		if l.ch == '=' {
			l.nextChar()
			return token.Token{Kind: token.PercentEq, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Percent, Span: span}
	case '=':
		if l.ch == '=' {
			l.nextChar()
			if l.ch == '=' {
				l.nextChar()
				return token.Token{Kind: token.EqEqEq, Span: source.Span{Start: startPos, End: l.currentPos()}}
			}
			return token.Token{Kind: token.EqEq, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		if l.ch == '>' {
			l.nextChar()
			return token.Token{Kind: token.Arrow, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Eq, Span: span}
	case '!':
		if l.ch == '=' {
			l.nextChar()
			if l.ch == '=' {
				l.nextChar()
				return token.Token{Kind: token.BangEqEq, Span: source.Span{Start: startPos, End: l.currentPos()}}
			}
			return token.Token{Kind: token.BangEq, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Bang, Span: span}
	case '<':
		if l.ch == '<' {
			l.nextChar()
			return token.Token{Kind: token.LtLt, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		if l.ch == '=' {
			l.nextChar()
			return token.Token{Kind: token.LtEq, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Lt, Span: span}
	case '>':
		if l.ch == '>' {
			l.nextChar()
			if l.ch == '>' {
				l.nextChar()
				return token.Token{Kind: token.GtGtGt, Span: source.Span{Start: startPos, End: l.currentPos()}}
			}
			return token.Token{Kind: token.GtGt, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		if l.ch == '=' {
			l.nextChar()
			return token.Token{Kind: token.GtEq, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Gt, Span: span}
	case '&':
		if l.ch == '&' {
			l.nextChar()
			return token.Token{Kind: token.AmpAmp, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Amp, Span: span}
	case '|':
		if l.ch == '|' {
			l.nextChar()
			return token.Token{Kind: token.PipePipe, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Pipe, Span: span}
	case '^':
		return token.Token{Kind: token.Caret, Span: span}
	case '~':
		return token.Token{Kind: token.Tilde, Span: span}
	case '?':
		if l.ch == '?' {
			l.nextChar()
			return token.Token{Kind: token.QuestionQuestion, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		if l.ch == '.' {
			l.nextChar()
			return token.Token{Kind: token.QuestionDot, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Question, Span: span}
	case ':':
		return token.Token{Kind: token.Colon, Span: span}
	case ';':
		return token.Token{Kind: token.Semicolon, Span: span}
	case ',':
		return token.Token{Kind: token.Comma, Span: span}
	case '.':
		if l.ch == '.' && l.peek() == '.' {
			l.nextChar()
			l.nextChar()
			return token.Token{Kind: token.DotDotDot, Span: source.Span{Start: startPos, End: l.currentPos()}}
		}
		return token.Token{Kind: token.Dot, Span: span}
	case '(':
		return token.Token{Kind: token.LParen, Span: span}
	case ')':
		return token.Token{Kind: token.RParen, Span: span}
	case '{':
		return token.Token{Kind: token.LBrace, Span: span}
	case '}':
		return token.Token{Kind: token.RBrace, Span: span}
	case '[':
		return token.Token{Kind: token.LBracket, Span: span}
	case ']':
		return token.Token{Kind: token.RBracket, Span: span}
	default:
		return token.Token{
			Kind: token.Illegal,
			Span: span,
			Text: string(ch),
		}
	}
}

func (l *Lexer) scanNumber(startPos source.Pos) token.Token {
	start := l.offset
	// Check for hex 0x, bin 0b, oct 0o
	if l.ch == '0' {
		p := l.peek()
		if p == 'x' || p == 'X' {
			l.nextChar() // '0'
			l.nextChar() // 'x'
			for isHexDigit(l.ch) {
				l.nextChar()
			}
			text := string(l.src[start:l.offset])
			return token.Token{Kind: token.Number, Span: source.Span{Start: startPos, End: l.currentPos()}, Text: text}
		} else if p == 'b' || p == 'B' {
			l.nextChar()
			l.nextChar()
			for l.ch == '0' || l.ch == '1' {
				l.nextChar()
			}
			text := string(l.src[start:l.offset])
			return token.Token{Kind: token.Number, Span: source.Span{Start: startPos, End: l.currentPos()}, Text: text}
		}
	}

	for unicode.IsDigit(l.ch) {
		l.nextChar()
	}
	if l.ch == '.' && unicode.IsDigit(l.peek()) {
		l.nextChar() // '.'
		for unicode.IsDigit(l.ch) {
			l.nextChar()
		}
	}
	if l.ch == 'e' || l.ch == 'E' {
		l.nextChar()
		if l.ch == '+' || l.ch == '-' {
			l.nextChar()
		}
		for unicode.IsDigit(l.ch) {
			l.nextChar()
		}
	}
	text := string(l.src[start:l.offset])
	return token.Token{Kind: token.Number, Span: source.Span{Start: startPos, End: l.currentPos()}, Text: text}
}

func (l *Lexer) scanString(startPos source.Pos, quote rune) token.Token {
	l.nextChar() // consume opening quote
	var buf []rune
	for l.ch != quote && l.ch != -1 && l.ch != '\n' {
		if l.ch == '\\' {
			l.nextChar()
			switch l.ch {
			case 'n':
				buf = append(buf, '\n')
			case 'r':
				buf = append(buf, '\r')
			case 't':
				buf = append(buf, '\t')
			case '\\':
				buf = append(buf, '\\')
			case '\'':
				buf = append(buf, '\'')
			case '"':
				buf = append(buf, '"')
			default:
				buf = append(buf, l.ch)
			}
		} else {
			buf = append(buf, l.ch)
		}
		l.nextChar()
	}

	if l.ch != quote {
		l.diagnostics = append(l.diagnostics, diag.Diagnostic{
			Span:     source.Span{Start: startPos, End: l.currentPos()},
			Code:     "TS1002",
			Message:  "Unterminated string literal.",
			Severity: diag.SeverityError,
		})
	} else {
		l.nextChar() // consume closing quote
	}

	return token.Token{
		Kind: token.String,
		Span: source.Span{Start: startPos, End: l.currentPos()},
		Text: string(buf),
	}
}

func (l *Lexer) scanTemplate(startPos source.Pos) token.Token {
	l.nextChar() // consume '`'
	var buf []rune
	for l.ch != '`' && l.ch != -1 {
		if l.ch == '\\' {
			l.nextChar()
			buf = append(buf, l.ch)
		} else {
			buf = append(buf, l.ch)
		}
		l.nextChar()
	}
	if l.ch == '`' {
		l.nextChar()
	} else {
		l.diagnostics = append(l.diagnostics, diag.Diagnostic{
			Span:     source.Span{Start: startPos, End: l.currentPos()},
			Code:     "TS1002",
			Message:  "Unterminated template literal.",
			Severity: diag.SeverityError,
		})
	}
	return token.Token{
		Kind: token.TemplateNoSubst,
		Span: source.Span{Start: startPos, End: l.currentPos()},
		Text: string(buf),
	}
}

func isIdentStart(ch rune) bool {
	return ch == '_' || ch == '$' || unicode.IsLetter(ch)
}

func isIdentPart(ch rune) bool {
	return ch == '_' || ch == '$' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

func isHexDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// TokenizeAll drains the lexer and returns all non-comment tokens ending with EOF.
func TokenizeAll(file *source.File) ([]token.Token, diag.DiagnosticList) {
	lex := New(file)
	var list []token.Token
	for {
		tok := lex.Next()
		if tok.Kind == token.Comment {
			continue
		}
		list = append(list, tok)
		if tok.Kind == token.EOF {
			break
		}
	}
	return list, lex.Diagnostics()
}

// ParseFloat converts token text to float64 value.
func ParseFloat(text string) (float64, error) {
	return strconv.ParseFloat(text, 64)
}
