package e2e_test

import "testing"

func TestLinuxAMD64OptionalFieldTernaryMissingBranch(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "optional_field_ternary_missing",
		source: `
const wrapped: { inner?: { value: number } } = false ? { inner: { value: 16 } } : {};
console.log(wrapped.inner?.value ?? 26);
`,
		expected: "26\n",
	})
}

func TestLinuxAMD64OptionalFieldTernaryPresentBranch(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "optional_field_ternary_present",
		source: `
const wrapped: { inner?: { value: number } } = true ? { inner: { value: 16 } } : {};
console.log(wrapped.inner?.value ?? 26);
`,
		expected: "16\n",
	})
}
