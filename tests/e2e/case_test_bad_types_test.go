package e2e_test

import "testing"

func TestE2E_Bad_Type_Var_Decl(t *testing.T) {
	runBad(t, badCase{
		name: "type_mismatch_variable_declaration",
		source: `
let x: number = "this is not a number";
`,
		expectedCode: "TS2322",
		expectedSub:  "Type 'string' is not assignable to type 'number'",
	})
}

func TestE2E_Bad_Type_Var_Reassign(t *testing.T) {
	runBad(t, badCase{
		name: "type_mismatch_variable_reassignment",
		source: `
let count: number = 10;
count = "ten";
`,
		expectedCode: "TS2322",
		expectedSub:  "Type 'string' is not assignable to type 'number'",
	})
}

func TestE2E_Bad_Type_Return(t *testing.T) {
	runBad(t, badCase{
		name: "type_mismatch_function_return",
		source: `
function getNumber(): number {
	return "string instead of number";
}
`,
		expectedCode: "TS2322",
		expectedSub:  "Type 'string' is not assignable to return type 'number'",
	})
}

func TestE2E_Bad_Undeclared_Identifier(t *testing.T) {
	runBad(t, badCase{
		name: "undeclared_identifier_usage",
		source: `
console.log(totallyNonExistentVar);
`,
		expectedCode: "TS2304",
		expectedSub:  "Cannot find name 'totallyNonExistentVar'",
	})
}

func TestE2E_Bad_Duplicate_Symbol(t *testing.T) {
	runBad(t, badCase{
		name: "duplicate_variable_in_same_scope",
		source: `
let duplicateVar: number = 1;
let duplicateVar: string = "second";
`,
		expectedCode: "TS2300",
		expectedSub:  "duplicate symbol",
	})
}

func TestE2E_Bad_Call_Arg_Type(t *testing.T) {
	runBad(t, badCase{
		name: "type_mismatch_call_argument",
		source: `
function requireNumber(val: number): number {
	return val * 2;
}
requireNumber("not_a_number");
`,
		expectedCode: "TS2345",
		expectedSub:  "Argument of type 'string' is not assignable to parameter of type 'number'",
	})
}

func TestE2E_Bad_Call_Non_Callable(t *testing.T) {
	runBad(t, badCase{
		name: "call_non_callable_scalar",
		source: `
let nonCallable: number = 42;
nonCallable();
`,
		expectedCode: "TS2349",
		expectedSub:  "This expression is not callable",
	})
}

func TestE2E_Bad_Property_On_Number(t *testing.T) {
	runBad(t, badCase{
		name: "property_access_on_number",
		source: `
let num: number = 10;
let bad = num.nonExistentProp;
`,
		expectedCode: "TS2339",
		expectedSub:  "Property 'nonExistentProp' does not exist on type 'number'",
	})
}
