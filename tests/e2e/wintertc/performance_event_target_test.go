package wintertc_test

import "testing"

func TestLinuxAMD64WinterTCPerformanceEventTarget(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_performance_event_target",
		source: `
let calls = 0;
const listener = (event: Event): void => {
  calls = calls + 1;
  console.log(event.type);
  console.log(event.target === performance);
  console.log(event.currentTarget === performance);
};

performance.addEventListener("measure", listener);
console.log(performance.dispatchEvent(new Event("measure")));
performance.removeEventListener("measure", listener);
console.log(performance.dispatchEvent(new Event("measure")));
console.log(calls);
`,
		expected: "measure\ntrue\ntrue\ntrue\ntrue\n1\n",
	})
}
