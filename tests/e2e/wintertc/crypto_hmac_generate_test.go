package wintertc_test

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/tests/e2e/harness"
)

func TestSubtleCryptoHMACGenerateKey(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_hmac_generate_key",
		source: `async function main(): Promise<void> {
  const sha256 = await crypto.subtle.generateKey(
    { name: "hMaC", hash: "sHa-256" },
    true,
    ["sign", "verify"]
  );
  console.log(sha256.type);
  console.log(sha256.extractable);
  console.log(sha256.algorithm.name);
  console.log(sha256.algorithm.hash.name);
  console.log(sha256.algorithm.length);
  console.log(sha256.usages.length);

  const raw256 = new Uint8Array(await crypto.subtle.exportKey("raw", sha256));
  console.log(raw256.length);

  const data = new TextEncoder().encode("generated-key");
  const signature = await crypto.subtle.sign("HMAC", sha256, data);
  console.log(new Uint8Array(signature).length);
  console.log(await crypto.subtle.verify("HMAC", sha256, signature, data));

  const sha512 = await crypto.subtle.generateKey(
    { name: "HMAC", hash: { name: "SHA-512" } },
    false,
    ["sign"]
  );
  console.log(sha512.algorithm.length);

  const partial = await crypto.subtle.generateKey(
    { name: "HMAC", hash: "SHA-256", length: 17 },
    true,
    ["sign"]
  );
  console.log(partial.algorithm.length);
  const partialRaw = new Uint8Array(await crypto.subtle.exportKey("raw", partial));
  console.log(partialRaw.length);
  console.log(partialRaw[2] % 128);
}
main();
`,
		expected: "secret\ntrue\nHMAC\nSHA-256\n512\n2\n64\n32\ntrue\n1024\n17\n3\n0\n",
	})
}

func TestSubtleCryptoHMACGenerateKeyRejections(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_hmac_generate_key_rejections",
		source: `async function main(): Promise<void> {
  try {
    await crypto.subtle.generateKey({ name: "HMAC", hash: "SHA-256", length: 0 }, false, ["sign"]);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  try {
    await crypto.subtle.generateKey({ name: "HMAC", hash: "SHA-256" }, false, ["encrypt"]);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  try {
    await crypto.subtle.generateKey({ name: "HMAC", hash: "SHA-3" }, false, ["sign"]);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
main();
`,
		expected: "OperationError\nSyntaxError\nNotSupportedError\n",
	})
}

func TestSubtleCryptoHMACGenerateKeyRejectsInvalidAlgorithmType(t *testing.T) {
	harness.RunBad(t, harness.BadCase{
		Name:         "subtle_crypto_hmac_generate_key_invalid_algorithm",
		Source:       `crypto.subtle.generateKey("HMAC", false, ["sign"]);`,
		ExpectedCode: "TS2345",
		ExpectedSub:  "crypto.subtle.generateKey argument \"algorithm\"",
	})
}
