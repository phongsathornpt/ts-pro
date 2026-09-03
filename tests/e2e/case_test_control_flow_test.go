package e2e_test

import "testing"

func TestE2E_If_Else_Ladder(t *testing.T) {
	runHappy(t, happyCase{
		name: "control_flow_if_else_ladder",
		source: `
function classify(x: number): number {
	if (x < 0) {
		return 100;
	} else if (x == 0) {
		return 200;
	} else if (x < 10) {
		return 300;
	} else {
		return 400;
	}
}
console.log(classify(-5));
console.log(classify(0));
console.log(classify(7));
console.log(classify(50));
`,
		expected: "100\n200\n300\n400\n",
	})
}

func TestE2E_Ternary_Operator(t *testing.T) {
	runHappy(t, happyCase{
		name: "control_flow_ternary_operator",
		source: `
let val1 = 10 > 5 ? 42 : 99;
let val2 = 10 < 5 ? 42 : 99;
console.log(val1);
console.log(val2);
`,
		expected: "42\n99\n",
	})
}
