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

func TestLinuxAMD64WinterTCStructuredClonePreservesSharedReferences(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_structured_clone_shared_reference",
		source: `
const shared = { value: 1 };
const source = { a: shared, b: shared };
const clone = structuredClone(source);
console.log(clone !== source);
console.log(clone.a !== shared);
console.log(clone.a === clone.b);
clone.a.value = 7;
console.log(clone.b.value);
console.log(shared.value);
`,
		expected: "true\ntrue\ntrue\n7\n1\n",
	})
}

func TestLinuxAMD64WinterTCStructuredClonePreservesCycles(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_structured_clone_cycle",
		source: `
const source: any = { value: 1 };
source.self = source;
const clone: any = structuredClone(source);
console.log(clone !== source);
console.log(clone.self === clone);
console.log(clone.value);
clone.value = 9;
console.log(source.value);
console.log(clone.self.value);
`,
		expected: "true\ntrue\n1\n1\n9\n",
	})
}

func TestLinuxAMD64WinterTCStructuredCloneArrayBufferCopiesBytes(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_structured_clone_array_buffer",
		source: `
const source = new ArrayBuffer(3);
const sourceView = new Uint8Array(source);
sourceView[0] = 1;
sourceView[1] = 2;
sourceView[2] = 3;
const clone = structuredClone(source);
const cloneView = new Uint8Array(clone);
console.log(clone !== source);
console.log(clone.byteLength);
console.log(cloneView[0]);
console.log(cloneView[1]);
console.log(cloneView[2]);
cloneView[0] = 9;
console.log(sourceView[0]);
console.log(cloneView[0]);
`,
		expected: "true\n3\n1\n2\n3\n1\n9\n",
	})
}
