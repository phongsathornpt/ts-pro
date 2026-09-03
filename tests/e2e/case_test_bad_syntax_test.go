package e2e_test

import "testing"

func TestE2E_Bad_Syntax_Missing_Paren(t *testing.T) {
	runBad(t, badCase{
		name: "syntax_missing_closing_paren",
		source: `
function calc(x: number {
	return x;
}
`,
		expectedCode: "TS1005",
		expectedSub:  "expected )",
	})
}

func TestE2E_Bad_Syntax_Unexpected_Token(t *testing.T) {
	runBad(t, badCase{
		name: "syntax_unexpected_token_in_expression",
		source: `
let x = + * 5;
`,
		expectedCode: "TS1005",
		expectedSub:  "unexpected token",
	})
}

func TestE2E_Bad_Syntax_Unclosed_Brace(t *testing.T) {
	runBad(t, badCase{
		name: "syntax_unclosed_brace",
		source: `
function broken() {
	let a = 10;
`,
		expectedCode: "TS1005",
		expectedSub:  "expected }",
	})
}

func TestE2E_Bad_Syntax_Missing_Colon_Ternary(t *testing.T) {
	runBad(t, badCase{
		name: "syntax_missing_colon_in_ternary",
		source: `
let res = true ? 1;
`,
		expectedCode: "TS1005",
		expectedSub:  "expected :",
	})
}
