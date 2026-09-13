package wintertc_test

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/tests/e2e/harness"
)

func TestSubtleCryptoHMACSHA256(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_hmac_sha256",
		source: `async function main(): Promise<void> {
  const encoder = new TextEncoder();
  const key = await crypto.subtle.importKey(
    "raw",
    encoder.encode("key"),
    { name: "hMaC", hash: "sHa-256" },
    false,
    ["sign", "verify"]
  );

  console.log(key.type);
  console.log(key.extractable);
  console.log(key.algorithm.name);
  console.log(key.algorithm.hash.name);
  console.log(key.algorithm.length);
  console.log(key.usages.length);
  console.log(key.usages[0]);
  console.log(key.usages[1]);

  const data = encoder.encode("abc");
  const signature = new Uint8Array(await crypto.subtle.sign({ name: "HMAC" }, key, data));
  console.log(signature.length);
  for (let i = 0; i < signature.length; i = i + 1) {
    console.log(signature[i]);
  }

  console.log(await crypto.subtle.verify("hmac", key, signature, data));
  signature[0] = signature[0] + 1;
  console.log(await crypto.subtle.verify("HMAC", key, signature.buffer, data.buffer));
}
main();
`,
		expected: "secret\nfalse\nHMAC\nSHA-256\n24\n2\nsign\nverify\n32\n156\n25\n110\n50\n220\n1\n117\n248\n111\n75\n28\n184\n146\n137\n214\n97\n157\n230\n190\n230\n153\n228\n195\n120\n230\n131\n9\n237\n151\n161\n166\n171\ntrue\nfalse\n",
	})
}

func TestSubtleCryptoHMACRejections(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_hmac_rejections",
		source: `async function main(): Promise<void> {
  const encoder = new TextEncoder();
  const keyData = encoder.encode("key");
  const data = encoder.encode("abc");

  try {
    await crypto.subtle.importKey("raw", keyData, { name: "HMAC", hash: "SHA-512" }, false, ["sign"]);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  try {
    await crypto.subtle.importKey("raw", new Uint8Array(0), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  try {
    await crypto.subtle.importKey("raw", keyData, { name: "HMAC", hash: "SHA-256" }, false, ["encrypt"]);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  const signOnly = await crypto.subtle.importKey("raw", keyData, { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
  const signature = await crypto.subtle.sign("HMAC", signOnly, data);
  console.log(new Uint8Array(signature).length);
  try {
    await crypto.subtle.verify("HMAC", signOnly, signature, data);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
main();
`,
		expected: "NotSupportedError\nDataError\nSyntaxError\n32\nInvalidAccessError\n",
	})
}

func TestSubtleCryptoHMACRejectsInvalidKeyType(t *testing.T) {
	harness.RunBad(t, harness.BadCase{
		Name:         "subtle_crypto_hmac_invalid_key",
		Source:       `crypto.subtle.sign("HMAC", "not-a-key", new Uint8Array(1));`,
		ExpectedCode: "TS2345",
		ExpectedSub:  "crypto.subtle.sign argument \"key\"",
	})
}
