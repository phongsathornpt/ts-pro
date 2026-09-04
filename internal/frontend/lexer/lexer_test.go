package lexer

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestLexerBasic(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("test.ts", []byte("function fib(n: number): number { return n <= 1 ? n : fib(n - 1) + fib(n - 2); }"))

	tokens, diags := TokenizeAll(f)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	expectedKinds := []token.Kind{
		token.KwFunction,
		token.Ident,
		token.LParen,
		token.Ident,
		token.Colon,
		token.Ident,
		token.RParen,
		token.Colon,
		token.Ident,
		token.LBrace,
		token.KwReturn,
		token.Ident,
		token.LtEq,
		token.Number,
		token.Question,
		token.Ident,
		token.Colon,
		token.Ident,
		token.LParen,
		token.Ident,
		token.Minus,
		token.Number,
		token.RParen,
		token.Plus,
		token.Ident,
		token.LParen,
		token.Ident,
		token.Minus,
		token.Number,
		token.RParen,
		token.Semicolon,
		token.RBrace,
		token.EOF,
	}

	if len(tokens) != len(expectedKinds) {
		t.Fatalf("expected %d tokens, got %d", len(expectedKinds), len(tokens))
	}

	for i, exp := range expectedKinds {
		if tokens[i].Kind != exp {
			t.Errorf("token %d: got %v (%q), want %v", i, tokens[i].Kind, tokens[i].Text, exp)
		}
	}
}

func TestLexerStringsAndComments(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("test2.ts", []byte(`// comment line
let msg = "hello\nworld\r\t\\\'\""; /* block comment */
let x = 0x1F;
let b = 0b1010;
let f = 3.14e-2;
let dotF = .5;
`))

	tokens, diags := TokenizeAll(f)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if len(tokens) < 7 {
		t.Fatalf("expected at least 7 tokens, got %d", len(tokens))
	}
	if tokens[0].Kind != token.KwLet {
		t.Errorf("expected KwLet, got %v", tokens[0].Kind)
	}
	if tokens[2].Kind != token.Eq {
		t.Errorf("expected Eq, got %v", tokens[2].Kind)
	}
}

func TestLexerAllOperatorsAndPunctuation(t *testing.T) {
	src := []byte(`
+ ++ += - -- -= * ** *= / /= % %=
! ~ & && &= | || |= ^ ^=
<< >> >>> = == === != !==
< <= > >= ( ) { } [ ] ; , . ... ? ?? ?. : =>
`)
	fs := source.NewFileSet()
	f := fs.AddFile("ops.ts", src)
	tokens, _ := TokenizeAll(f)
	if len(tokens) < 30 {
		t.Fatalf("too few tokens parsed: %d", len(tokens))
	}
}

func TestLexerErrorsAndEdgeCases(t *testing.T) {
	fs := source.NewFileSet()

	// Unterminated block comment
	f1 := fs.AddFile("err1.ts", []byte(`/* unterminated block`))
	_, d1 := TokenizeAll(f1)
	if !d1.HasErrors() {
		t.Errorf("expected TS1002 diagnostic for unterminated block comment")
	}

	// Unterminated string
	f2 := fs.AddFile("err2.ts", []byte(`"unterminated string`))
	_, d2 := TokenizeAll(f2)
	if !d2.HasErrors() {
		t.Errorf("expected diagnostic for unterminated string")
	}

	// Single quote string
	f3 := fs.AddFile("str.ts", []byte(`'single quoted'`))
	toks3, d3 := TokenizeAll(f3)
	if d3.HasErrors() || len(toks3) < 1 || toks3[0].Text != "single quoted" {
		t.Errorf("expected single quote string, got %v, diags: %v", toks3, d3)
	}

	// Template literals
	f4 := fs.AddFile("tpl.ts", []byte("`hello ${world}`"))
	toks4, _ := TokenizeAll(f4)
	if len(toks4) < 1 {
		t.Errorf("expected template token")
	}
}
