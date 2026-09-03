package e2e_test

import "testing"

func TestE2E_While_Countdown(t *testing.T) {
	runHappy(t, happyCase{
		name: "loops_while_countdown",
		source: `
let count = 5;
let sum = 0;
while (count > 0) {
	sum = sum + count;
	count--;
}
console.log(sum);
`,
		expected: "15\n",
	})
}

func TestE2E_For_Accumulator(t *testing.T) {
	runHappy(t, happyCase{
		name: "loops_for_accumulator",
		source: `
let total = 0;
for (let i = 1; i <= 10; i++) {
	total = total + i;
}
console.log(total);
`,
		expected: "55\n",
	})
}

func TestE2E_Nested_Loops_Matrix(t *testing.T) {
	runHappy(t, happyCase{
		name: "loops_nested_multiplication_matrix",
		source: `
let total = 0;
for (let i = 1; i <= 3; i++) {
	for (let j = 1; j <= 4; j++) {
		total = total + (i * j);
	}
}
console.log(total);
`,
		expected: "60\n",
	})
}
