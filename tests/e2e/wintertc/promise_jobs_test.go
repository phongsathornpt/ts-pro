package wintertc_test

import "testing"

func TestLinuxAMD64WinterTCPromiseJobsUseMicrotaskQueue(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_promise_jobs_microtask_priority",
		source: `
spawn((): void => { console.log("task"); });
const resolved = Promise.resolve(42);
console.log(join(resolved));
`,
		expected: "42\ntask\n",
	})
}

func TestLinuxAMD64WinterTCPromiseJobsStayFIFOWithQueueMicrotask(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_promise_jobs_microtask_fifo",
		source: `
queueMicrotask((): void => { console.log("queued-first"); });
const resolved = Promise.resolve(7);
queueMicrotask((): void => { console.log("queued-last"); });
console.log(join(resolved));
`,
		expected: "queued-first\n7\nqueued-last\n",
	})
}
