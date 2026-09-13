package wintertc_test

import "testing"

func TestSubtleCryptoHMACImportPartialBitLength(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_hmac_partial_bit_length",
		source: `async function main(): Promise<void> {
  const keyData = new Uint8Array(3);
  keyData[0] = 171;
  keyData[1] = 205;
  keyData[2] = 239;
  const algorithm = { name: "HMAC", hash: "SHA-256", length: 17 };
  const key = await crypto.subtle.importKey("raw", keyData, algorithm, false, ["sign", "verify"]);
  const data = new TextEncoder().encode("abc");
  const signature = new Uint8Array(await crypto.subtle.sign("HMAC", key, data));

  console.log(key.algorithm.length);
  console.log(keyData[2]);
  console.log(signature.length);
  console.log(signature[0]);
  console.log(signature[1]);
  console.log(signature[2]);
  console.log(signature[3]);
  console.log(signature[31]);
  console.log(await crypto.subtle.verify("HMAC", key, signature, data));

  const masked = new Uint8Array(3);
  masked[0] = 171;
  masked[1] = 205;
  masked[2] = 128;
  const maskedKey = await crypto.subtle.importKey(
    "raw", masked, { name: "HMAC", hash: "SHA-256" }, false, ["verify"]
  );
  console.log(await crypto.subtle.verify("HMAC", maskedKey, signature, data));
}
main();
`,
		expected: "17\n239\n32\n232\n53\n14\n10\n178\ntrue\ntrue\n",
	})
}

func TestSubtleCryptoHMACImportLengthValidation(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_hmac_length_validation",
		source: `async function main(): Promise<void> {
  const keyData = new Uint8Array(3);
  keyData[0] = 171;
  keyData[1] = 205;
  keyData[2] = 239;

  try {
    await crypto.subtle.importKey("raw", keyData, { name: "HMAC", hash: "SHA-256", length: 0 }, false, ["sign"]);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  try {
    await crypto.subtle.importKey("raw", keyData, { name: "HMAC", hash: "SHA-256", length: 25 }, false, ["sign"]);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  try {
    await crypto.subtle.importKey("raw", keyData, { name: "HMAC", hash: "SHA-256", length: 16 }, false, ["sign"]);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  try {
    await crypto.subtle.importKey("raw", keyData, { name: "HMAC", hash: "SHA-256", length: 23.5 }, false, ["sign"]);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  const full = await crypto.subtle.importKey(
    "raw", keyData, { name: "HMAC", hash: "SHA-256", length: 24 }, false, ["sign"]
  );
  console.log(full.algorithm.length);

  const longKey = new Uint8Array(65);
  try {
    await crypto.subtle.importKey(
      "raw", longKey, { name: "HMAC", hash: "SHA-256", length: 519 }, false, ["sign"]
    );
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
main();
`,
		expected: "DataError\nDataError\nDataError\nDataError\n24\nNotSupportedError\n",
	})
}
