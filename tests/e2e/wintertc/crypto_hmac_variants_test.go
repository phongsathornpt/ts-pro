package wintertc_test

import "testing"

func TestSubtleCryptoHMACHashVariants(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_hmac_hash_variants",
		source: `async function main(): Promise<void> {
  const encoder = new TextEncoder();
  const keyData = encoder.encode("key");
  const data = encoder.encode("abc");

  const sha1Key = await crypto.subtle.importKey(
    "raw", keyData, { name: "HMAC", hash: "sHa-1" }, false, ["sign", "verify"]
  );
  const sha1 = new Uint8Array(await crypto.subtle.sign("HMAC", sha1Key, data));
  console.log(sha1Key.algorithm.hash.name);
  console.log(sha1.length);
  console.log(sha1[0]);
  console.log(sha1[19]);
  console.log(await crypto.subtle.verify("hmac", sha1Key, sha1, data));

  const sha384Key = await crypto.subtle.importKey(
    "raw", keyData, { name: "HMAC", hash: { name: "ShA-384" } }, false, ["sign", "verify"]
  );
  const sha384 = new Uint8Array(await crypto.subtle.sign("HMAC", sha384Key, data));
  console.log(sha384Key.algorithm.hash.name);
  console.log(sha384.length);
  console.log(sha384[0]);
  console.log(sha384[47]);
  console.log(await crypto.subtle.verify("HMAC", sha384Key, sha384.buffer, data.buffer));

  const sha512Key = await crypto.subtle.importKey(
    "raw", keyData, { name: "HMAC", hash: "SHA-512" }, false, ["sign", "verify"]
  );
  const sha512 = new Uint8Array(await crypto.subtle.sign({ name: "HMAC" }, sha512Key, data));
  console.log(sha512Key.algorithm.hash.name);
  console.log(sha512.length);
  console.log(sha512[0]);
  console.log(sha512[63]);
  console.log(await crypto.subtle.verify({ name: "hmac" }, sha512Key, sha512, data));

  try {
    await crypto.subtle.importKey(
      "raw", keyData, { name: "HMAC", hash: "SHA-3" }, false, ["sign"]
    );
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
main();
`,
		expected: "SHA-1\n20\n79\n252\ntrue\nSHA-384\n48\n48\n56\ntrue\nSHA-512\n64\n57\n122\ntrue\nNotSupportedError\n",
	})
}
