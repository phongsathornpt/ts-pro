package token

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

// Kind represents the lexical category of a token.
type Kind uint16

const (
	Illegal Kind = iota
	EOF
	Comment

	// Literals & Identifiers
	Ident
	Number
	String
	TemplateNoSubst
	TemplateHead
	TemplateMiddle
	TemplateTail

	// Keywords
	KwLet
	KwConst
	KwVar
	KwFunction
	KwReturn
	KwIf
	KwElse
	KwWhile
	KwDo
	KwFor
	KwSwitch
	KwCase
	KwDefault
	KwBreak
	KwContinue
	KwClass
	KwEnum
	KwInterface
	KwType
	KwExtends
	KwImplements
	KwNew
	KwThis
	KwSuper
	KwImport
	KwExport
	KwFrom
	KwAs
	KwTrue
	KwFalse
	KwNull
	KwUndefined
	KwAsync
	KwAwait
	KwTry
	KwCatch
	KwFinally
	KwThrow
	KwOf
	KwIn

	// Operators & Delimiters
	Plus             // +
	Minus            // -
	Star             // *
	Slash            // /
	Percent          // %
	StarStar         // **
	PlusPlus         // ++
	MinusMinus       // --
	Bang             // !
	Tilde            // ~
	Amp              // &
	Pipe             // |
	Caret            // ^
	LtLt             // <<
	GtGt             // >>
	GtGtGt           // >>>
	AmpAmp           // &&
	PipePipe         // ||
	QuestionQuestion // ??
	QuestionDot      // ?.
	Eq               // =
	EqEq             // ==
	EqEqEq           // ===
	BangEq           // !=
	BangEqEq         // !==
	Lt               // <
	LtEq             // <=
	Gt               // >
	GtEq             // >=
	PlusEq           // +=
	MinusEq          // -=
	StarEq           // *=
	SlashEq          // /=
	PercentEq        // %=
	LParen           // (
	RParen           // )
	LBrace           // {
	RBrace           // }
	LBracket         // [
	RBracket         // ]
	Semicolon        // ;
	Comma            // ,
	Dot              // .
	DotDotDot        // ...
	Question         // ?
	Colon            // :
	Arrow            // =>
)

var tokens = [...]string{
	Illegal: "ILLEGAL",
	EOF:     "EOF",
	Comment: "COMMENT",

	Ident:           "IDENT",
	Number:          "NUMBER",
	String:          "STRING",
	TemplateNoSubst: "TEMPLATE",
	TemplateHead:    "TEMPLATE_HEAD",
	TemplateMiddle:  "TEMPLATE_MIDDLE",
	TemplateTail:    "TEMPLATE_TAIL",

	KwLet:        "let",
	KwConst:      "const",
	KwVar:        "var",
	KwFunction:   "function",
	KwReturn:     "return",
	KwIf:         "if",
	KwElse:       "else",
	KwWhile:      "while",
	KwDo:         "do",
	KwFor:        "for",
	KwSwitch:     "switch",
	KwCase:       "case",
	KwDefault:    "default",
	KwBreak:      "break",
	KwContinue:   "continue",
	KwClass:      "class",
	KwEnum:       "enum",
	KwInterface:  "interface",
	KwType:       "type",
	KwExtends:    "extends",
	KwImplements: "implements",
	KwNew:        "new",
	KwThis:       "this",
	KwSuper:      "super",
	KwImport:     "import",
	KwExport:     "export",
	KwFrom:       "from",
	KwAs:         "as",
	KwTrue:       "true",
	KwFalse:      "false",
	KwNull:       "null",
	KwUndefined:  "undefined",
	KwAsync:      "async",
	KwAwait:      "await",
	KwTry:        "try",
	KwCatch:      "catch",
	KwFinally:    "finally",
	KwThrow:      "throw",
	KwOf:         "of",
	KwIn:         "in",

	Plus:             "+",
	Minus:            "-",
	Star:             "*",
	Slash:            "/",
	Percent:          "%",
	StarStar:         "**",
	PlusPlus:         "++",
	MinusMinus:       "--",
	Bang:             "!",
	Tilde:            "~",
	Amp:              "&",
	Pipe:             "|",
	Caret:            "^",
	LtLt:             "<<",
	GtGt:             ">>",
	GtGtGt:           ">>>",
	AmpAmp:           "&&",
	PipePipe:         "||",
	QuestionQuestion: "??",
	QuestionDot:      "?.",
	Eq:               "=",
	EqEq:             "==",
	EqEqEq:           "===",
	BangEq:           "!=",
	BangEqEq:         "!==",
	Lt:               "<",
	LtEq:             "<=",
	Gt:               ">",
	GtEq:             ">=",
	PlusEq:           "+=",
	MinusEq:          "-=",
	StarEq:           "*=",
	SlashEq:          "/=",
	PercentEq:        "%=",
	LParen:           "(",
	RParen:           ")",
	LBrace:           "{",
	RBrace:           "}",
	LBracket:         "[",
	RBracket:         "]",
	Semicolon:        ";",
	Comma:            ",",
	Dot:              ".",
	DotDotDot:        "...",
	Question:         "?",
	Colon:            ":",
	Arrow:            "=>",
}

func (k Kind) String() string {
	if int(k) < len(tokens) && tokens[k] != "" {
		return tokens[k]
	}
	return fmt.Sprintf("Token(%d)", k)
}

// IsKeyword reports whether the token is a reserved keyword.
func (k Kind) IsKeyword() bool {
	return k >= KwLet && k <= KwIn
}

// IsLiteral reports whether the token is a literal.
func (k Kind) IsLiteral() bool {
	return k >= Ident && k <= TemplateTail
}

// IsOperator reports whether the token is an operator.
func (k Kind) IsOperator() bool {
	return k >= Plus && k <= Arrow
}

var keywords = map[string]Kind{
	"let":        KwLet,
	"const":      KwConst,
	"var":        KwVar,
	"function":   KwFunction,
	"return":     KwReturn,
	"if":         KwIf,
	"else":       KwElse,
	"while":      KwWhile,
	"do":         KwDo,
	"for":        KwFor,
	"switch":     KwSwitch,
	"case":       KwCase,
	"default":    KwDefault,
	"break":      KwBreak,
	"continue":   KwContinue,
	"class":      KwClass,
	"enum":       KwEnum,
	"interface":  KwInterface,
	"type":       KwType,
	"extends":    KwExtends,
	"implements": KwImplements,
	"new":        KwNew,
	"this":       KwThis,
	"super":      KwSuper,
	"import":     KwImport,
	"export":     KwExport,
	"from":       KwFrom,
	"as":         KwAs,
	"true":       KwTrue,
	"false":      KwFalse,
	"null":       KwNull,
	"undefined":  KwUndefined,
	"async":      KwAsync,
	"await":      KwAwait,
	"try":        KwTry,
	"catch":      KwCatch,
	"finally":    KwFinally,
	"throw":      KwThrow,
	"of":         KwOf,
	"in":         KwIn,
}

// Lookup maps an identifier name to its keyword Kind if reserved, or Ident.
func Lookup(ident string) Kind {
	if k, ok := keywords[ident]; ok {
		return k
	}
	return Ident
}

// Token represents a scanned token with its kind, span, and optional literal string.
type Token struct {
	Kind Kind
	Span source.Span
	Text string
}

func (t Token) String() string {
	if t.Text != "" {
		return fmt.Sprintf("%s(%q)", t.Kind, t.Text)
	}
	return t.Kind.String()
}
