package token

import (
	"testing"
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
	if !Number.IsLiteral() {
		t.Errorf("Number should be a literal")
	}
	if !Plus.IsOperator() {
		t.Errorf("Plus should be an operator")
	}
	if Plus.String() != "+" {
		t.Errorf("Plus.String() = %q, want +", Plus.String())
	}
}
