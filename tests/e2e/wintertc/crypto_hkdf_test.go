package wintertc_test

import "testing"

func TestSubtleCryptoHKDFSHA256RFC5869(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_hkdf_sha256_rfc5869",
		source: `
function bytes(values: number[]): Uint8Array {
  const out = new Uint8Array(values.length);
  for (let i = 0; i < values.length; i = i + 1) out[i] = values[i];
  return out;
}

async function main(): Promise<void> {
  const ikm = new Uint8Array(22);
  for (let i = 0; i < ikm.length; i = i + 1) ikm[i] = 11;
  const salt = bytes([0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12]);
  const info = bytes([240, 241, 242, 243, 244, 245, 246, 247, 248, 249]);
  const key = await crypto.subtle.importKey("raw", ikm, { name: "HKDF" }, false, ["deriveBits"]);
  console.log(key.type);
  console.log(key.extractable);
  console.log(key.algorithm.name);
  console.log(key.usages.length);
  console.log(key.usages[0]);

  const result = new Uint8Array(await crypto.subtle.deriveBits({
    name: "HKDF",
    hash: "SHA-256",
    salt: salt,
    info: info,
  }, key, 336));
  console.log(result.length);
  for (let i = 0; i < result.length; i = i + 1) console.log(result[i]);
}
main();
`,
		expected: "secret\nfalse\nHKDF\n1\nderiveBits\n42\n60\n178\n95\n37\n250\n172\n213\n122\n144\n67\n79\n100\n208\n54\n47\n42\n45\n45\n10\n144\n207\n26\n90\n76\n93\n176\n45\n86\n236\n196\n197\n191\n52\n0\n114\n8\n213\n184\n135\n24\n88\n101\n",
	})
}

func TestSubtleCryptoHKDFRejections(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_hkdf_rejections",
		source: `
async function main(): Promise<void> {
  const ikm = new Uint8Array(4);
  const empty = new Uint8Array(0);

  try {
    await crypto.subtle.importKey("raw", ikm, { name: "HKDF" }, true, ["deriveBits"]);
    console.log("unexpected");
  } catch (err: any) { console.log(err.name); }

  try {
    await crypto.subtle.importKey("raw", ikm, { name: "HKDF" }, false, ["sign"]);
    console.log("unexpected");
  } catch (err: any) { console.log(err.name); }

  const key = await crypto.subtle.importKey("raw", ikm, { name: "HKDF" }, false, ["deriveBits"]);
  try {
    await crypto.subtle.deriveBits({ name: "HKDF", hash: "SHA-512", salt: empty, info: empty }, key, 256);
    console.log("unexpected");
  } catch (err: any) { console.log(err.name); }

  try {
    await crypto.subtle.deriveBits({ name: "HKDF", hash: "SHA-256", salt: empty, info: empty }, key, 7);
    console.log("unexpected");
  } catch (err: any) { console.log(err.name); }

  const deriveKeyOnly = await crypto.subtle.importKey("raw", ikm, { name: "HKDF" }, false, ["deriveKey"]);
  try {
    await crypto.subtle.deriveBits({ name: "HKDF", hash: "SHA-256", salt: empty, info: empty }, deriveKeyOnly, 256);
    console.log("unexpected");
  } catch (err: any) { console.log(err.name); }
}
main();
`,
		expected: "SyntaxError\nSyntaxError\nNotSupportedError\nOperationError\nInvalidAccessError\n",
	})
}
