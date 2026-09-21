package wintertc_test

import "testing"

func TestLinuxAMD64WinterTCUnhandledRejectionWithoutObservers(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_unhandled_rejection_no_observers",
		source: `
Promise.reject<string>("boom");
console.log("sync");
`,
		expected: "sync\n",
	})
}

func TestLinuxAMD64WinterTCUnhandledRejectionEventListener(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_unhandled_rejection_event_listener",
		source: `
globalThis.addEventListener("unhandledrejection", (event: any): void => {
  console.log(event.type);
  console.log(event.reason);
});
Promise.reject<string>("boom");
console.log("sync");
`,
		expected: "sync\nunhandledrejection\nboom\n",
	})
}

func TestLinuxAMD64WinterTCUnhandledRejectionHandler(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_unhandled_rejection_handler",
		source: `
globalThis.onunhandledrejection = (event: any): void => {
  console.log("handler:" + event.reason);
};
Promise.reject<string>("boom");
console.log("sync");
`,
		expected: "sync\nhandler:boom\n",
	})
}

func TestLinuxAMD64WinterTCHandledRejectionBeforeCheckpoint(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_handled_rejection_before_checkpoint",
		source: `
globalThis.addEventListener("unhandledrejection", (event: any): void => {
  console.log("unexpected:" + event.reason);
});
async function run(): Promise<void> {
  const promise = Promise.reject<string>("handled");
  try {
    await promise;
  } catch (error: any) {
    console.log("caught:" + error);
  }
}
join(run());
`,
		expected: "caught:handled\n",
	})
}

func TestLinuxAMD64WinterTCRejectionHandledAfterReport(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_rejection_handled_after_report",
		source: `
globalThis.addEventListener("unhandledrejection", (event: any): void => {
  console.log("unhandled:" + event.reason);
});
globalThis.addEventListener("rejectionhandled", (event: any): void => {
  console.log("handled:" + event.reason);
});
globalThis.onrejectionhandled = (event: any): void => {
  console.log("handler:" + event.reason);
};

const promise = Promise.reject<string>("late");
async function lateHandler(value: Promise<string>): Promise<void> {
  try {
    await value;
  } catch (error: any) {
    console.log("caught:" + error);
  }
}
lateHandler(promise);
`,
		expected: "unhandled:late\nhandled:late\nhandler:late\ncaught:late\n",
	})
}
