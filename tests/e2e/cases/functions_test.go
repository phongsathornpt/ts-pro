package cases_test

import "testing"

func TestE2E_Multi_Param_ABI(t *testing.T) {
	runHappy(t, happyCase{
		name: "multi_param_abi_passing",
		source: `
function sum6(a: number, b: number, c: number, d: number, e: number, f: number): number {
	return a + b + c + d + e + f;
}
console.log(sum6(1, 2, 3, 4, 5, 6));
`,
		expected: "21\n",
	})
}

func TestE2E_Scope_Reassignment(t *testing.T) {
	runHappy(t, happyCase{
		name: "scope_variable_reassignment",
		source: `
let x = 100;
x = x + 50;
console.log(x);
`,
		expected: "150\n",
	})
}

func TestE2E_Top_Level_Execution(t *testing.T) {
	runHappy(t, happyCase{
		name: "top_level_sequential_execution",
		source: `
console.log("Starting pipeline...");
let val = 12345;
console.log(val);
console.log("Pipeline finished.");
`,
		expected: "Starting pipeline...\n12345\nPipeline finished.\n",
	})
}
