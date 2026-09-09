package cases_test

import "testing"

func TestE2E_Arithmetic_Precedence(t *testing.T) {
	runHappy(t, happyCase{
		name: "arithmetic_precedence_and_modulo",
		source: `
let a = (10 + 20) * 3 - (40 / 4) % 7;
console.log(a);
`,
		expected: "87\n",
	})
}

func TestE2E_Unary_Negation_And_Not(t *testing.T) {
	runHappy(t, happyCase{
		name: "unary_negation_and_logical_not",
		source: `
let x = 42;
let neg = -x;
console.log(neg);
let zero = 0;
console.log(!zero);
let one = 1;
console.log(!one);
`,
		expected: "-42\ntrue\nfalse\n",
	})
}

func TestE2E_Boolean_Comparisons_And_Logic(t *testing.T) {
	runHappy(t, happyCase{
		name: "boolean_comparisons_and_logic",
		source: `
console.log(10 < 20);
console.log(20 <= 20);
console.log(30 > 50);
console.log(50 >= 50);
console.log(100 == 100);
console.log(100 != 200);
console.log(1 && 1);
console.log(1 && 0);
console.log(0 || 1);
console.log(0 || 0);
`,
		expected: "true\ntrue\nfalse\ntrue\ntrue\ntrue\n1\n0\n1\n0\n",
	})
}

func TestE2E_Logical_ShortCircuit_ValueSemantics(t *testing.T) {
	runHappy(t, happyCase{
		name: "logical_short_circuit_value_semantics",
		source: `
function side(): number {
  console.log(99);
  return 7;
}
console.log(0 && side());
console.log(1 || side());
console.log(1 && side());
console.log(0 || side());
`,
		expected: "0\n1\n99\n7\n99\n7\n",
	})
}
