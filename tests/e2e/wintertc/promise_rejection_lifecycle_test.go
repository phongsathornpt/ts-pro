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
globalThis.addEventListener("unhandledrejection", (event: PromiseRejectionEvent): void => {
  console.log("listener-called");
});
Promise.reject<string>("boom");
console.log("sync");
`,
		expected: "sync\nlistener-called\n",
	})
}

func TestLinuxAMD64WinterTCUnhandledRejectionHandler(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_unhandled_rejection_handler",
		source: `
globalThis.onunhandledrejection = (event: PromiseRejectionEvent): void => {
  console.log("handler-called");
};
Promise.reject<string>("boom");
console.log("sync");
`,
		expected: "sync\nhandler-called\n",
	})
}

func TestLinuxAMD64WinterTCHandledRejectionBeforeCheckpoint(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_handled_rejection_before_checkpoint",
		source: `
globalThis.addEventListener("unhandledrejection", (event: PromiseRejectionEvent): void => {
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
globalThis.addEventListener("unhandledrejection", (event: PromiseRejectionEvent): void => {
  console.log("unhandled:" + event.reason);
});
globalThis.addEventListener("rejectionhandled", (event: PromiseRejectionEvent): void => {
  console.log("handled:" + event.reason);
});
globalThis.onrejectionhandled = (event: PromiseRejectionEvent): void => {
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

func TestLinuxAMD64WinterTCTaskNestedIndirectCallback(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_task_nested_indirect_callback",
		source: `
const callback = (value: string): void => { console.log(value); };
spawn((): void => { callback("nested"); });
console.log("sync");
`,
		expected: "sync\nnested\n",
	})
}

func TestLinuxAMD64WinterTCGlobalEventDispatchFromTask(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_global_event_dispatch_from_task",
		source: `
globalThis.addEventListener("probe", (event: Event): void => {
  console.log(event.type);
});
spawn((): void => {
  globalThis.dispatchEvent(new Event("probe"));
});
console.log("sync");
`,
		expected: "sync\nprobe\n",
	})
}
