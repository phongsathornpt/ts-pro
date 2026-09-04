package token

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestLookup(t *testing.T) {
	if Lookup("function") != KwFunction {
		t.Errorf("Lookup(function) = %v, want KwFunction", Lookup("function"))
	}
	if Lookup("let") != KwLet {
		t.Errorf("Lookup(let) = %v, want KwLet", Lookup("let"))
	}
	if Lookup("myVariable") != Ident {
		t.Errorf("Lookup(myVariable) = %v, want Ident", Lookup("myVariable"))
	}
}

func TestKindProperties(t *testing.T) {
	if !KwReturn.IsKeyword() {
		t.Errorf("KwReturn should be a keyword")
	}
	if Number.IsKeyword() {
		t.Errorf("Number should not be a keyword")
	}
	if !Number.IsLiteral() {
		t.Errorf("Number should be a literal")
	}
	if Plus.IsLiteral() {
		t.Errorf("Plus should not be a literal")
	}
	if !Plus.IsOperator() {
		t.Errorf("Plus should be an operator")
	}
	if Number.IsOperator() {
		t.Errorf("Number should not be an operator")
	}
	if Plus.String() != "+" {
		t.Errorf("Plus.String() = %q, want +", Plus.String())
	}
	for k := Illegal; k <= Arrow; k++ {
		_ = k.String()
	}
	invalid := Kind(999)
	if invalid.String() != "Token(999)" {
		t.Errorf("invalid token string = %q", invalid.String())
	}

	tokWithText := Token{Kind: Ident, Span: source.Span{Start: 0, End: 3}, Text: "abc"}
	if tokWithText.String() != `IDENT("abc")` {
		t.Errorf("tokWithText.String() = %q", tokWithText.String())
	}
	tokNoText := Token{Kind: Plus, Span: source.Span{Start: 0, End: 1}}
	if tokNoText.String() != "+" {
		t.Errorf("tokNoText.String() = %q", tokNoText.String())
	}
}
