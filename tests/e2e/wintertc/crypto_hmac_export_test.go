package wintertc_test

import "testing"

func TestSubtleCryptoHMACExportRaw(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_hmac_export_raw",
		source: `async function main(): Promise<void> {
  const original = new Uint8Array(3);
  original[0] = 10;
  original[1] = 20;
  original[2] = 30;

  const key = await crypto.subtle.importKey(
    "raw",
    original,
    { name: "HMAC", hash: "SHA-256" },
    true,
    ["sign", "verify"]
  );

  const exported = new Uint8Array(await crypto.subtle.exportKey("raw", key));
  console.log(exported.length);
  console.log(exported[0]);
  console.log(exported[1]);
  console.log(exported[2]);

  exported[0] = 99;
  const exportedAgain = new Uint8Array(await crypto.subtle.exportKey("raw", key));
  console.log(exportedAgain[0]);
}
main();
`,
		expected: "3\n10\n20\n30\n10\n",
	})
}

func TestSubtleCryptoHMACExportRejections(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_hmac_export_rejections",
		source: `async function main(): Promise<void> {
  const keyData = new Uint8Array(3);
  keyData[0] = 1;
  keyData[1] = 2;
  keyData[2] = 3;

  const hidden = await crypto.subtle.importKey(
    "raw",
    keyData,
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"]
  );

  try {
    await crypto.subtle.exportKey("raw", hidden);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  const visible = await crypto.subtle.importKey(
    "raw",
    keyData,
    { name: "HMAC", hash: "SHA-256" },
    true,
    ["sign"]
  );

  try {
    await crypto.subtle.exportKey("jwk", visible);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
main();
`,
		expected: "InvalidAccessError\nNotSupportedError\n",
	})
}
