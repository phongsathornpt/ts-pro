package e2e_test

import "testing"

func TestE2E_Strings_Basic(t *testing.T) {
	runHappy(t, happyCase{
		name: "strings_concatenation_basic",
		source: `
let greeting = "Hello, ";
let target = "World!";
console.log(greeting + target);
`,
		expected: "Hello, World!\n",
	})
}

func TestE2E_Strings_Multi_Segment(t *testing.T) {
	runHappy(t, happyCase{
		name: "strings_multi_segment_and_empty",
		source: `
let empty = "";
let full = empty + "Pure " + "Go " + "TypeScript " + "Native" + empty;
console.log(full);
`,
		expected: "Pure Go TypeScript Native\n",
	})
}

func TestE2E_Strings_Function_Args(t *testing.T) {
	runHappy(t, happyCase{
		name: "strings_function_arguments_and_return",
		source: `
function wrap(s: string): string {
	return "[" + s + "]";
}
console.log(wrap("TS-Pro"));
`,
		expected: "[TS-Pro]\n",
	})
}
