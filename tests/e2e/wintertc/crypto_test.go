package wintertc_test

import "testing"

func TestCryptoGetRandomValues(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "crypto_get_random_values",
		source: `const backing = new Uint8Array(36);
backing[0] = 11;
backing[1] = 22;
backing[34] = 33;
backing[35] = 44;

const view = backing.subarray(2, 34);
const same = crypto.getRandomValues(view);
console.log(same === view);
console.log(backing[0]);
console.log(backing[1]);
console.log(backing[34]);
console.log(backing[35]);

let changed = false;
for (let i = 2; i < 34; i = i + 1) {
  if (backing[i] !== 0) {
    changed = true;
  }
}
console.log(changed);

try {
  crypto.getRandomValues(new Uint8Array(65537));
  console.log("unexpected");
} catch (err: any) {
  console.log(err.name);
}
`,
		expected: "true\n11\n22\n33\n44\ntrue\nQuotaExceededError\n",
	})
}
