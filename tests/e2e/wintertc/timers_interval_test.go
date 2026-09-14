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
