package wintertc_test

import "testing"

func TestLinuxAMD64WinterTCStructuredCloneDeepCopy(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_structured_clone_deep_copy",
		source: `
const source = {
  nested: { value: 1 },
  list: [1, 2, 3],
};
const clone = structuredClone(source);
console.log(source === clone);
console.log(source.nested === clone.nested);
console.log(source.list === clone.list);
clone.nested.value = 9;
clone.list[0] = 7;
console.log(source.nested.value);
console.log(clone.nested.value);
console.log(source.list[0]);
console.log(clone.list[0]);
`,
		expected: "false\nfalse\nfalse\n1\n9\n1\n7\n",
	})
}
