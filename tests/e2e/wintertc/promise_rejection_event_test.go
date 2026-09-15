package wintertc_test

import "testing"

func TestLinuxAMD64WinterTCPromiseRejectionEvent(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_promise_rejection_event",
		source: `
const promise: any = "promise-token";
const event = new PromiseRejectionEvent("unhandledrejection", {
  promise,
  reason: "boom",
  cancelable: true,
});
console.log(event.type);
console.log(event.promise);
console.log(event.reason);
console.log(event.cancelable);
console.log(event.defaultPrevented);
`,
		expected: "unhandledrejection\npromise-token\nboom\ntrue\nfalse\n",
	})
}
