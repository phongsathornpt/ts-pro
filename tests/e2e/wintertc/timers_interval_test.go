package wintertc_test

import "testing"

func TestLinuxAMD64WinterTCSetIntervalRepeatsAndCancels(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_set_interval",
		source: `
let calls = 0;
let id: number = 0;
id = setInterval((): void => {
  calls = calls + 1;
  console.log(calls);
  if (calls === 3) {
    clearInterval(id);
  }
}, 0);
`,
		expected: "1\n2\n3\n",
	})
}

func TestLinuxAMD64WinterTCClearIntervalBeforeFirstTick(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_clear_interval_before_first_tick",
		source: `
const id = setInterval((): void => { console.log("unexpected"); }, 50);
clearInterval(id);
clearInterval(id);
console.log("cleared");
`,
		expected: "cleared\n",
	})
}

func TestLinuxAMD64WinterTCClearIntervalStaleHandle(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_clear_interval_stale_handle",
		source: `
let id: number = 0;
id = setInterval((): void => {
  console.log("tick");
  clearInterval(id);
  setTimeout((): void => {
    clearInterval(id);
    console.log("stale");
  }, 0);
}, 0);
`,
		expected: "tick\nstale\n",
	})
}
