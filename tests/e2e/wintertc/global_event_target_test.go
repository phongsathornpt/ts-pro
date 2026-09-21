package wintertc_test

import "testing"

func TestLinuxAMD64WinterTCGlobalEventTarget(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_global_event_target",
		source: `
let calls = 0;
const listener = (event: Event): void => {
  calls = calls + 1;
  console.log(event.type);
};

globalThis.addEventListener("probe", listener);
console.log(globalThis.dispatchEvent(new Event("probe")));
globalThis.removeEventListener("probe", listener);
console.log(globalThis.dispatchEvent(new Event("probe")));
console.log(calls);
`,
		expected: "probe\ntrue\ntrue\n1\n",
	})
}
