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
let msg = "hello\nworld"; /* block comment */
let x = 0x1F;
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
	if tokens[3].Kind != token.String || tokens[3].Text != "hello\nworld" {
		t.Errorf("expected string with escaped newline, got %v (%q)", tokens[3].Kind, tokens[3].Text)
	}
	if tokens[8].Kind != token.Number || tokens[8].Text != "0x1F" {
		t.Errorf("expected hex number 0x1F, got %v (%q)", tokens[8].Kind, tokens[8].Text)
	}
}
